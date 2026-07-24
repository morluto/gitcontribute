package evidence

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/morluto/gitcontribute/internal/domain"
)

type fakeRepo struct {
	mu       sync.Mutex
	defs     map[string]*ValidationDefinition
	runs     map[string]*ValidationRun
	groups   map[string]*ValidationRunGroup
	evidence map[string]*Evidence
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		defs:     make(map[string]*ValidationDefinition),
		runs:     make(map[string]*ValidationRun),
		groups:   make(map[string]*ValidationRunGroup),
		evidence: make(map[string]*Evidence),
	}
}

func (r *fakeRepo) SaveValidationDefinition(_ context.Context, d *ValidationDefinition) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defs[d.ID] = d
	return nil
}

func (r *fakeRepo) GetValidationDefinition(_ context.Context, id string) (*ValidationDefinition, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.defs[id]
	if !ok {
		return nil, ErrNotFound
	}
	return d, nil
}

func (r *fakeRepo) SaveValidationRun(_ context.Context, run *ValidationRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[run.ID] = run
	return nil
}

func (r *fakeRepo) GetValidationRun(_ context.Context, id string) (*ValidationRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[id]
	if !ok {
		return nil, ErrNotFound
	}
	return run, nil
}

func (r *fakeRepo) SaveValidationRunGroup(_ context.Context, group *ValidationRunGroup) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.groups[group.ID] = group
	return nil
}

func (r *fakeRepo) GetValidationRunGroup(_ context.Context, id string) (*ValidationRunGroup, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	group, ok := r.groups[id]
	if !ok {
		return nil, ErrNotFound
	}
	return group, nil
}

func (r *fakeRepo) SaveEvidence(_ context.Context, e *Evidence) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.evidence[e.ID] = e
	return nil
}

func (r *fakeRepo) ListEvidence(_ context.Context, filter EvidenceFilter) ([]*Evidence, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*Evidence
	for _, e := range r.evidence {
		if filter.OpportunityID != "" && e.OpportunityID != filter.OpportunityID {
			continue
		}
		if filter.HypothesisID != "" && e.HypothesisID != filter.HypothesisID {
			continue
		}
		if filter.InvestigationID != "" && e.InvestigationID != filter.InvestigationID {
			continue
		}
		if filter.Relation != "" && e.Relation != filter.Relation {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

type fakeRunner struct {
	result *RunResult
	err    error
}

func (f *fakeRunner) Run(_ context.Context, _ RunRequest) (*RunResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func TestDefineValidationRequiresCommand(t *testing.T) {
	t.Parallel()
	svc := NewService(newFakeRepo(), &fakeRunner{})
	err := svc.DefineValidation(context.Background(), &ValidationDefinition{
		WorkingDir: "/tmp",
	})
	if !errors.Is(err, ErrMissingCommand) {
		t.Fatalf("expected ErrMissingCommand, got %v", err)
	}
}

func TestDefineValidationRequiresWorkspace(t *testing.T) {
	t.Parallel()
	svc := NewService(newFakeRepo(), &fakeRunner{})
	err := svc.DefineValidation(context.Background(), &ValidationDefinition{
		Command: []string{"go", "test"},
	})
	if !errors.Is(err, ErrMissingWorkspace) {
		t.Fatalf("expected ErrMissingWorkspace, got %v", err)
	}
}

func TestRunValidation(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	runner := &fakeRunner{
		result: &RunResult{
			ExitCode:       1,
			Stdout:         "fail\n",
			Stderr:         "",
			Classification: RunClassificationFailing,
		},
	}
	svc := NewService(repo, runner)

	def := &ValidationDefinition{
		ID:         "def-1",
		Name:       "test",
		Command:    []string{"go", "test"},
		WorkingDir: "/tmp",
	}
	if err := svc.DefineValidation(context.Background(), def); err != nil {
		t.Fatalf("define: %v", err)
	}

	run, err := svc.RunValidation(context.Background(), def.ID, RunKindBase)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if run.Kind != RunKindBase {
		t.Fatalf("kind: got %q, want base", run.Kind)
	}
	if run.Classification != RunClassificationFailing {
		t.Fatalf("classification: got %q, want failing", run.Classification)
	}
	if run.Stdout != "fail\n" {
		t.Fatalf("stdout: got %q", run.Stdout)
	}
}

func TestRunValidationUnknownKind(t *testing.T) {
	t.Parallel()
	svc := NewService(newFakeRepo(), &fakeRunner{})
	def := &ValidationDefinition{ID: "def-1", Command: []string{"go", "test"}, WorkingDir: "/tmp"}
	_ = svc.DefineValidation(context.Background(), def)
	_, err := svc.RunValidation(context.Background(), def.ID, RunKind("other"))
	if !errors.Is(err, ErrMissingRunKind) {
		t.Fatalf("expected ErrMissingRunKind, got %v", err)
	}
}

func TestCreateEvidenceValidates(t *testing.T) {
	t.Parallel()
	svc := NewService(newFakeRepo(), &fakeRunner{})
	err := svc.CreateEvidence(context.Background(), &Evidence{
		Type:     "not-a-type",
		Relation: RelationSupporting,
	})
	if !errors.Is(err, ErrInvalidEvidenceType) {
		t.Fatalf("expected ErrInvalidEvidenceType, got %v", err)
	}
}

func TestCompareValidation(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	svc := NewService(repo, &fakeRunner{})

	base := &ValidationRun{ID: "base", DefinitionID: "def", Kind: RunKindBase, ExitCode: 1, Classification: RunClassificationFailing}
	candidate := &ValidationRun{ID: "candidate", DefinitionID: "def", Kind: RunKindCandidate, ExitCode: 0, Classification: RunClassificationPassing}
	_ = repo.SaveValidationRun(context.Background(), base)
	_ = repo.SaveValidationRun(context.Background(), candidate)

	cmp, err := svc.CompareValidation(context.Background(), base.ID, candidate.ID)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if cmp.Classification != ComparisonFixed {
		t.Fatalf("classification: got %q, want fixed", cmp.Classification)
	}
}

func TestCompareValidationRejectsDifferentDefinitions(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	svc := NewService(repo, &fakeRunner{})
	_ = repo.SaveValidationRun(context.Background(), &ValidationRun{ID: "base", DefinitionID: "a", Kind: RunKindBase})
	_ = repo.SaveValidationRun(context.Background(), &ValidationRun{ID: "candidate", DefinitionID: "b", Kind: RunKindCandidate})
	if _, err := svc.CompareValidation(context.Background(), "base", "candidate"); !errors.Is(err, ErrInvalidComparison) {
		t.Fatalf("comparison error = %v, want ErrInvalidComparison", err)
	}
}

func TestRunValidationUsesKindSpecificWorkspace(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	runner := &capturingRunner{result: &RunResult{Classification: RunClassificationPassing}}
	svc := NewService(repo, runner)
	def := &ValidationDefinition{ID: "def", Command: []string{"test"}, BaseWorkingDir: "/base", CandidateDir: "/candidate"}
	if err := svc.DefineValidation(context.Background(), def); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RunValidation(context.Background(), def.ID, RunKindCandidate); err != nil {
		t.Fatal(err)
	}
	if runner.request.Dir != "/candidate" {
		t.Fatalf("runner dir = %q, want candidate workspace", runner.request.Dir)
	}
}

type capturingRunner struct {
	request RunRequest
	result  *RunResult
}

func (r *capturingRunner) Run(_ context.Context, request RunRequest) (*RunResult, error) {
	r.request = request
	return r.result, nil
}

type deadlineRunner struct {
	hadDeadline bool
	remaining   time.Duration
}

func (r *deadlineRunner) Run(ctx context.Context, _ RunRequest) (*RunResult, error) {
	deadline, ok := ctx.Deadline()
	r.hadDeadline = ok
	if ok {
		r.remaining = time.Until(deadline)
	}
	now := time.Now()
	return &RunResult{StartedAt: now, CompletedAt: now, Classification: RunClassificationPassing}, nil
}

func TestRunValidationRejectsDefinitionWithoutNormalizedTimeout(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	repo.defs["invalid"] = &ValidationDefinition{ID: "invalid", Command: []string{"test"}, WorkingDir: "/tmp"}
	_, err := NewService(repo, &deadlineRunner{}).RunValidation(context.Background(), "invalid", RunKindBase)
	if !errors.Is(err, ErrInvalidTimeout) {
		t.Fatalf("RunValidation error = %v, want ErrInvalidTimeout", err)
	}
}

func TestDefineValidationStoresKindAndBounds(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	svc := NewService(repo, &fakeRunner{})
	def := &ValidationDefinition{
		Command:        []string{"go", "test"},
		WorkingDir:     "/tmp/ws",
		Kind:           "test",
		Timeout:        2 * time.Minute,
		MaxOutputBytes: 8192,
	}
	if err := svc.DefineValidation(context.Background(), def); err != nil {
		t.Fatal(err)
	}
	stored, ok := repo.defs[def.ID]
	if !ok {
		t.Fatal("definition was not stored")
	}
	if stored.Kind != "test" {
		t.Fatalf("kind = %q, want test", stored.Kind)
	}
	if stored.Timeout != 2*time.Minute {
		t.Fatalf("timeout = %v, want 2m", stored.Timeout)
	}
	if stored.MaxOutputBytes != 8192 {
		t.Fatalf("max output = %d, want 8192", stored.MaxOutputBytes)
	}
}

func TestDefineValidationRejectsEnvironmentValues(t *testing.T) {
	svc := NewService(newFakeRepo(), &fakeRunner{})
	err := svc.DefineValidation(context.Background(), &ValidationDefinition{
		Command:    []string{"go", "test"},
		WorkingDir: "/tmp/ws",
		Env:        []string{"GITHUB_TOKEN=secret"},
	})
	if !errors.Is(err, ErrInvalidEnvironment) {
		t.Fatalf("DefineValidation error = %v, want ErrInvalidEnvironment", err)
	}
}

func TestDefineValidationRejectsInvalidBounds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		def     *ValidationDefinition
		wantErr error
	}{
		{
			name:    "negative timeout",
			def:     &ValidationDefinition{Command: []string{"test"}, WorkingDir: "/tmp", Timeout: -time.Second},
			wantErr: ErrInvalidTimeout,
		},
		{
			name:    "excessive timeout",
			def:     &ValidationDefinition{Command: []string{"test"}, WorkingDir: "/tmp", Timeout: maxValidationTimeout + time.Second},
			wantErr: ErrInvalidTimeout,
		},
		{
			name:    "oversized output",
			def:     &ValidationDefinition{Command: []string{"test"}, WorkingDir: "/tmp", MaxOutputBytes: maxOutputBytes + 1},
			wantErr: ErrInvalidOutputLimit,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewService(newFakeRepo(), &fakeRunner{}).DefineValidation(context.Background(), tt.def)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("DefineValidation error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefineValidationAppliesSafeTimeoutDefault(t *testing.T) {
	t.Parallel()
	def := &ValidationDefinition{Command: []string{"test"}, WorkingDir: "/tmp"}
	if err := NewService(newFakeRepo(), &fakeRunner{}).DefineValidation(context.Background(), def); err != nil {
		t.Fatal(err)
	}
	if def.Timeout != defaultValidationTimeout {
		t.Fatalf("default timeout = %v, want %v", def.Timeout, defaultValidationTimeout)
	}
}

func TestRunValidationResolvesEnvironmentAllowlistAtExecution(t *testing.T) {
	t.Setenv("GITCONTRIBUTE_ALLOWED_TEST", "current-value")
	repo := newFakeRepo()
	runner := &capturingRunner{result: &RunResult{Classification: RunClassificationPassing}}
	svc := NewService(repo, runner)
	def := &ValidationDefinition{
		ID:         "def",
		Command:    []string{"test"},
		WorkingDir: "/tmp/ws",
		Env:        []string{"GITCONTRIBUTE_ALLOWED_TEST", "GITCONTRIBUTE_MISSING_TEST", "GITCONTRIBUTE_ALLOWED_TEST"},
	}
	if err := svc.DefineValidation(context.Background(), def); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"GITCONTRIBUTE_ALLOWED_TEST", "GITCONTRIBUTE_MISSING_TEST"}, def.Env); diff != "" {
		t.Fatalf("stored allowlist mismatch (-want +got):\n%s", diff)
	}
	if _, err := svc.RunValidation(context.Background(), def.ID, RunKindBase); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"GITCONTRIBUTE_ALLOWED_TEST=current-value"}, runner.request.Env); diff != "" {
		t.Fatalf("execution environment mismatch (-want +got):\n%s", diff)
	}
}

func TestRunValidationInheritsEnvironmentWithEmptyAllowlist(t *testing.T) {
	repo := newFakeRepo()
	runner := &capturingRunner{result: &RunResult{Classification: RunClassificationPassing}}
	svc := NewService(repo, runner)
	def := &ValidationDefinition{ID: "def", Command: []string{"test"}, WorkingDir: "/tmp/ws"}
	if err := svc.DefineValidation(context.Background(), def); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RunValidation(context.Background(), def.ID, RunKindBase); err != nil {
		t.Fatal(err)
	}
	if runner.request.Env != nil {
		t.Fatalf("execution environment = %#v, want nil for inherited environment", runner.request.Env)
	}
}

func TestRunValidationPassesOutputBound(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	runner := &capturingRunner{result: &RunResult{Classification: RunClassificationPassing}}
	svc := NewService(repo, runner)
	def := &ValidationDefinition{ID: "def", Command: []string{"go", "test"}, WorkingDir: "/tmp/ws", MaxOutputBytes: 1024}
	if err := svc.DefineValidation(context.Background(), def); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RunValidation(context.Background(), def.ID, RunKindBase); err != nil {
		t.Fatal(err)
	}
	if runner.request.MaxOutputBytes != 1024 {
		t.Fatalf("max output bytes = %d, want 1024", runner.request.MaxOutputBytes)
	}
}

func TestDefineValidationRequiresDeclaredProtocolForReadinessDeadline(t *testing.T) {
	t.Parallel()
	svc := NewService(newFakeRepo(), &fakeRunner{})
	withoutProtocol := &ValidationDefinition{Command: []string{"server"}, WorkingDir: "/tmp", ReadinessTimeout: time.Second}
	if err := svc.DefineValidation(context.Background(), withoutProtocol); err == nil {
		t.Fatal("readiness deadline without protocol was accepted")
	}
	withProtocol := &ValidationDefinition{Command: []string{"server"}, WorkingDir: "/tmp", Protocol: ValidationProtocolMCPStdio}
	if err := svc.DefineValidation(context.Background(), withProtocol); err != nil {
		t.Fatal(err)
	}
	if withProtocol.ReadinessTimeout != 30*time.Second {
		t.Fatalf("readiness timeout = %s", withProtocol.ReadinessTimeout)
	}
}

func TestEvidenceWithSourceRef(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	svc := NewService(repo, &fakeRunner{})
	e := &Evidence{
		ID:          "ev-1",
		Type:        EvidenceTypeManualObservation,
		Relation:    RelationSupporting,
		Description: "observed panic",
		SourceRefs: []domain.SourceRef{
			{Source: "manual", URL: "file://note.txt", ObservedAt: time.Now().UTC()},
		},
	}
	err := svc.CreateEvidence(context.Background(), e)
	if err != nil {
		t.Fatalf("create evidence: %v", err)
	}
	if e.ID == "" {
		t.Fatal("expected ID assigned")
	}
}
