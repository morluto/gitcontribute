package evidence

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type sequenceRunner struct {
	mu      sync.Mutex
	results []*RunResult
	errors  []error
	delay   time.Duration
	index   int
}

func (r *sequenceRunner) Run(ctx context.Context, _ RunRequest) (*RunResult, error) {
	if r.delay > 0 {
		timer := time.NewTimer(r.delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return &RunResult{ExitCode: -1, StartedAt: time.Now().UTC(), CompletedAt: time.Now().UTC(), Classification: RunClassificationCancelled, Error: ctx.Err().Error()}, nil
		case <-timer.C:
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	index := r.index
	r.index++
	if index < len(r.errors) && r.errors[index] != nil {
		return nil, r.errors[index]
	}
	if index >= len(r.results) {
		return nil, errors.New("missing scripted result")
	}
	return r.results[index], nil
}

func repeatFixture(t *testing.T, runner Runner, observation *ObservationContract) (*Service, *fakeRepo, string) {
	t.Helper()
	repo := newFakeRepo()
	definition := &ValidationDefinition{
		ID: "def-repeat", InvestigationID: "inv-1", Command: []string{"test"}, WorkingDir: t.TempDir(),
		Timeout: time.Second, MaxOutputBytes: 1024, Observation: observation,
	}
	if err := repo.SaveValidationDefinition(context.Background(), definition); err != nil {
		t.Fatal(err)
	}
	return NewService(repo, runner), repo, definition.ID
}

func runResult(classification RunClassification) *RunResult {
	now := time.Now().UTC()
	exitCode := 0
	if classification == RunClassificationFailing {
		exitCode = 1
	}
	zero64, zeroU64 := int64(0), uint64(0)
	return &RunResult{
		ExitCode: exitCode, Classification: classification, StartedAt: now, CompletedAt: now,
		Resources: ResourceTelemetry{CPUTimeMillis: Int64Metric{Value: &zero64}, PeakRSSBytes: Uint64Metric{Value: &zeroU64}, PeakChildCount: Int64Metric{Value: &zero64}},
		Cleanup:   CleanupResult{Status: "clean"},
	}
}

func TestRunValidationGroupClassifiesOneOffFailureAsFlaky(t *testing.T) {
	runner := &sequenceRunner{results: []*RunResult{runResult(RunClassificationPassing), runResult(RunClassificationFailing), runResult(RunClassificationPassing)}}
	svc, repo, id := repeatFixture(t, runner, nil)
	group, err := svc.RunValidationGroup(context.Background(), id, RepeatValidationOptions{Kinds: []RunKind{RunKindCandidate}, RunCount: 3, Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	if group.Classification != RunGroupFlaky || group.CompletedRuns != 3 || len(group.Attempts) != 3 {
		t.Fatalf("group = %+v", group)
	}
	if repo.groups[group.ID] == nil || len(repo.runs) != 3 {
		t.Fatalf("group or attempts were not persisted")
	}
}

func TestRunValidationGroupKeepsResultsAfterMalformedAttempt(t *testing.T) {
	runner := &sequenceRunner{
		results: []*RunResult{nil, runResult(RunClassificationPassing), runResult(RunClassificationPassing)},
	}
	svc, _, id := repeatFixture(t, runner, nil)
	group, err := svc.RunValidationGroup(context.Background(), id, RepeatValidationOptions{Kinds: []RunKind{RunKindCandidate}, RunCount: 3, Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(group.Attempts) != 3 || group.CompletedRuns != 2 || group.Classification != RunGroupInconclusive || group.Attempts[0].RunID != "" {
		t.Fatalf("group = %+v", group)
	}
}

func TestRunValidationGroupCancellationReturnsPartialResults(t *testing.T) {
	runner := &sequenceRunner{delay: 100 * time.Millisecond, results: []*RunResult{runResult(RunClassificationPassing)}}
	svc, _, id := repeatFixture(t, runner, nil)
	group, err := svc.RunValidationGroup(context.Background(), id, RepeatValidationOptions{
		Kinds: []RunKind{RunKindCandidate}, RunCount: 5, Concurrency: 1,
		PerRunTimeout: time.Second, OverallTimeout: 25 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(group.Attempts) >= 5 || group.Classification != RunGroupCancelled {
		t.Fatalf("group = %+v", group)
	}
}

func TestRunValidationGroupComparisonRejectsUnrelatedCandidateError(t *testing.T) {
	contract := &ObservationContract{
		Intent:    "candidate removes expected symptom",
		Base:      []ExpectedObservation{{Name: "symptom", Source: ObservationStderr, Matcher: ObservationExact, Pattern: "expected symptom", Occurrence: ObservationPresent}},
		Candidate: []ExpectedObservation{{Name: "symptom absent", Source: ObservationStderr, Matcher: ObservationExact, Pattern: "expected symptom", Occurrence: ObservationAbsent}},
	}
	base := runResult(RunClassificationFailing)
	base.Stderr = "expected symptom"
	candidate := runResult(RunClassificationError)
	candidate.Stderr, candidate.Error = "unrelated startup failure", "start failed"
	runner := &sequenceRunner{results: []*RunResult{base, candidate}}
	svc, _, id := repeatFixture(t, runner, contract)
	group, err := svc.RunValidationGroup(context.Background(), id, RepeatValidationOptions{Kinds: []RunKind{RunKindBase, RunKindCandidate}, RunCount: 1, Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	if group.Comparison == nil || group.Comparison.Classification != ComparisonInconclusive || group.Aggregates[1].Classification != RunGroupInconclusive {
		t.Fatalf("group = %+v", group)
	}
}
