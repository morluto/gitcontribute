package evidence

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

type fakeRepo struct {
	defs     map[string]*ValidationDefinition
	runs     map[string]*ValidationRun
	evidence map[string]*Evidence
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		defs:     make(map[string]*ValidationDefinition),
		runs:     make(map[string]*ValidationRun),
		evidence: make(map[string]*Evidence),
	}
}

func (r *fakeRepo) SaveValidationDefinition(_ context.Context, d *ValidationDefinition) error {
	r.defs[d.ID] = d
	return nil
}

func (r *fakeRepo) GetValidationDefinition(_ context.Context, id string) (*ValidationDefinition, error) {
	d, ok := r.defs[id]
	if !ok {
		return nil, ErrNotFound
	}
	return d, nil
}

func (r *fakeRepo) SaveValidationRun(_ context.Context, run *ValidationRun) error {
	r.runs[run.ID] = run
	return nil
}

func (r *fakeRepo) GetValidationRun(_ context.Context, id string) (*ValidationRun, error) {
	run, ok := r.runs[id]
	if !ok {
		return nil, ErrNotFound
	}
	return run, nil
}

func (r *fakeRepo) SaveEvidence(_ context.Context, e *Evidence) error {
	r.evidence[e.ID] = e
	return nil
}

func (r *fakeRepo) ListEvidence(_ context.Context, filter EvidenceFilter) ([]*Evidence, error) {
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
	svc := NewService(newFakeRepo(), &fakeRunner{})
	err := svc.DefineValidation(context.Background(), &ValidationDefinition{
		WorkingDir: "/tmp",
	})
	if !errors.Is(err, ErrMissingCommand) {
		t.Fatalf("expected ErrMissingCommand, got %v", err)
	}
}

func TestDefineValidationRequiresWorkspace(t *testing.T) {
	svc := NewService(newFakeRepo(), &fakeRunner{})
	err := svc.DefineValidation(context.Background(), &ValidationDefinition{
		Command: []string{"go", "test"},
	})
	if !errors.Is(err, ErrMissingWorkspace) {
		t.Fatalf("expected ErrMissingWorkspace, got %v", err)
	}
}

func TestRunValidation(t *testing.T) {
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
	svc := NewService(newFakeRepo(), &fakeRunner{})
	def := &ValidationDefinition{ID: "def-1", Command: []string{"go", "test"}, WorkingDir: "/tmp"}
	_ = svc.DefineValidation(context.Background(), def)
	_, err := svc.RunValidation(context.Background(), def.ID, RunKind("other"))
	if !errors.Is(err, ErrMissingRunKind) {
		t.Fatalf("expected ErrMissingRunKind, got %v", err)
	}
}

func TestCreateEvidenceValidates(t *testing.T) {
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
	repo := newFakeRepo()
	svc := NewService(repo, &fakeRunner{})
	_ = repo.SaveValidationRun(context.Background(), &ValidationRun{ID: "base", DefinitionID: "a", Kind: RunKindBase})
	_ = repo.SaveValidationRun(context.Background(), &ValidationRun{ID: "candidate", DefinitionID: "b", Kind: RunKindCandidate})
	if _, err := svc.CompareValidation(context.Background(), "base", "candidate"); !errors.Is(err, ErrInvalidComparison) {
		t.Fatalf("comparison error = %v, want ErrInvalidComparison", err)
	}
}

func TestRunValidationUsesKindSpecificWorkspace(t *testing.T) {
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

func TestEvidenceWithSourceRef(t *testing.T) {
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
