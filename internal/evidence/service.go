package evidence

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Service manages validation definitions, runs, evidence, and base-vs-candidate comparisons.
type Service struct {
	repo   Repository
	runner Runner
}

// NewService returns an EvidenceService backed by repo and runner.
func NewService(repo Repository, runner Runner) *Service {
	return &Service{repo: repo, runner: runner}
}

// DefineValidation validates and stores a validation definition.
func (s *Service) DefineValidation(ctx context.Context, d *ValidationDefinition) error {
	if d == nil {
		return fmt.Errorf("evidence: validation definition is nil")
	}
	if len(d.Command) == 0 {
		return ErrMissingCommand
	}
	if d.WorkingDir == "" && (d.BaseWorkingDir == "" || d.CandidateDir == "") {
		return ErrMissingWorkspace
	}
	if d.ID == "" {
		d.ID = uuid.NewString()
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now().UTC()
	}
	return s.repo.SaveValidationDefinition(ctx, d)
}

// RunValidation executes the definition and records a bounded run.
func (s *Service) RunValidation(ctx context.Context, defID string, kind RunKind) (*ValidationRun, error) {
	if kind != RunKindBase && kind != RunKindCandidate {
		return nil, ErrMissingRunKind
	}
	def, err := s.repo.GetValidationDefinition(ctx, defID)
	if err != nil {
		return nil, err
	}
	workingDir := def.WorkingDir
	if kind == RunKindBase && def.BaseWorkingDir != "" {
		workingDir = def.BaseWorkingDir
	}
	if kind == RunKindCandidate && def.CandidateDir != "" {
		workingDir = def.CandidateDir
	}
	if workingDir == "" {
		return nil, ErrMissingWorkspace
	}

	runCtx := ctx
	if def.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, def.Timeout)
		defer cancel()
	}

	var maxOutput int64 = defaultMaxOutputBytes
	if def.MaxOutputBytes > 0 {
		maxOutput = def.MaxOutputBytes
	}

	result, err := s.runner.Run(runCtx, RunRequest{
		Args:           def.Command,
		Dir:            workingDir,
		Env:            def.Env,
		MaxOutputBytes: maxOutput,
	})
	if err != nil {
		return nil, err
	}

	run := &ValidationRun{
		ID:              uuid.NewString(),
		DefinitionID:    defID,
		InvestigationID: def.InvestigationID,
		HypothesisID:    def.HypothesisID,
		OpportunityID:   def.OpportunityID,
		Kind:            kind,
		StartedAt:       result.StartedAt,
		CompletedAt:     result.CompletedAt,
		ExitCode:        result.ExitCode,
		Stdout:          result.Stdout,
		Stderr:          result.Stderr,
		Truncated:       result.Truncated,
		Error:           result.Error,
		Classification:  result.Classification,
	}
	if err := s.repo.SaveValidationRun(ctx, run); err != nil {
		return nil, err
	}
	return run, nil
}

// CompareValidation loads two runs and classifies their relationship.
func (s *Service) CompareValidation(ctx context.Context, baseRunID, candidateRunID string) (*ComparisonResult, error) {
	base, err := s.repo.GetValidationRun(ctx, baseRunID)
	if err != nil {
		return nil, fmt.Errorf("load base run: %w", err)
	}
	candidate, err := s.repo.GetValidationRun(ctx, candidateRunID)
	if err != nil {
		return nil, fmt.Errorf("load candidate run: %w", err)
	}
	if base.DefinitionID == "" || base.DefinitionID != candidate.DefinitionID ||
		base.InvestigationID != candidate.InvestigationID ||
		base.HypothesisID != candidate.HypothesisID ||
		base.OpportunityID != candidate.OpportunityID {
		return nil, ErrInvalidComparison
	}
	return Compare(base, candidate)
}

// CreateEvidence validates and stores an evidence item.
func (s *Service) CreateEvidence(ctx context.Context, e *Evidence) error {
	if e == nil {
		return fmt.Errorf("evidence: evidence is nil")
	}
	if !isValidEvidenceType(e.Type) {
		return fmt.Errorf("%w: %q", ErrInvalidEvidenceType, e.Type)
	}
	if !isValidRelation(e.Relation) {
		return fmt.Errorf("%w: %q", ErrInvalidRelation, e.Relation)
	}
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	return s.repo.SaveEvidence(ctx, e)
}

// ListEvidence returns stored evidence matching the filter.
func (s *Service) ListEvidence(ctx context.Context, filter EvidenceFilter) ([]*Evidence, error) {
	return s.repo.ListEvidence(ctx, filter)
}

func isValidEvidenceType(t EvidenceType) bool {
	switch t {
	case EvidenceTypeBaseFailingRegression,
		EvidenceTypeCandidatePassingRegression,
		EvidenceTypeMinimalReproduction,
		EvidenceTypeBenchmark,
		EvidenceTypeProfiler,
		EvidenceTypeInvariantViolation,
		EvidenceTypeCompatibilityMatrix,
		EvidenceTypeStaticAnalysis,
		EvidenceTypeManualObservation,
		EvidenceTypeGitHubSource:
		return true
	}
	return false
}

func isValidRelation(r Relation) bool {
	switch r {
	case RelationSupporting, RelationContradicting, RelationInconclusive, RelationStale, RelationInvalid:
		return true
	}
	return false
}
