package evidence

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/google/uuid"
)

const (
	maxEnvironmentVariables  = 64
	defaultValidationTimeout = 30 * time.Minute
	maxValidationTimeout     = 24 * time.Hour
)

var environmentNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Service manages validation definitions, runs, evidence, and base-vs-candidate comparisons.
type Service struct {
	repo      Repository
	runner    Runner
	mcpRunner Runner
}

// NewService returns an EvidenceService backed by repo and runner.
func NewService(repo Repository, runner Runner) *Service {
	return &Service{repo: repo, runner: runner, mcpRunner: NewMCPStdioRunner()}
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
	if d.Timeout < 0 || d.Timeout > maxValidationTimeout {
		return ErrInvalidTimeout
	}
	if d.Timeout == 0 {
		d.Timeout = defaultValidationTimeout
	}
	if d.Protocol != "" && d.Protocol != ValidationProtocolMCPStdio {
		return fmt.Errorf("unsupported validation protocol %q", d.Protocol)
	}
	if d.Protocol == "" && d.ReadinessTimeout != 0 {
		return errors.New("readiness timeout requires a declared protocol adapter")
	}
	if d.Protocol == ValidationProtocolMCPStdio {
		if d.ReadinessTimeout == 0 {
			d.ReadinessTimeout = 30 * time.Second
		}
		if d.ReadinessTimeout < 0 || d.ReadinessTimeout > d.Timeout {
			return errors.New("readiness timeout must be positive and no greater than validation timeout")
		}
	}
	if d.MaxOutputBytes < 0 || d.MaxOutputBytes > maxOutputBytes {
		return ErrInvalidOutputLimit
	}
	env, err := normalizeEnvironmentAllowlist(d.Env)
	if err != nil {
		return err
	}
	d.Env = env
	if err := validateObservationContract(d.Observation); err != nil {
		return err
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
	return s.runDefinition(ctx, def, kind, def.Timeout, 0)
}

func (s *Service) runDefinition(ctx context.Context, def *ValidationDefinition, kind RunKind, timeout, sampleInterval time.Duration) (*ValidationRun, error) {
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

	if timeout <= 0 || timeout > maxValidationTimeout {
		return nil, ErrInvalidTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var maxOutput int64 = defaultMaxOutputBytes
	if def.MaxOutputBytes > 0 {
		maxOutput = def.MaxOutputBytes
	}

	runner := s.runner
	if def.Protocol == ValidationProtocolMCPStdio {
		runner = s.mcpRunner
	}
	if runner == nil {
		return nil, errors.New("validation runner is unavailable")
	}
	result, err := runner.Run(runCtx, RunRequest{
		Args:             def.Command,
		Dir:              workingDir,
		Env:              resolveEnvironment(def.Env),
		MaxOutputBytes:   maxOutput,
		SampleInterval:   sampleInterval,
		ReadinessTimeout: def.ReadinessTimeout,
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.New("validation runner returned no result")
	}

	run := &ValidationRun{
		ID:              uuid.NewString(),
		DefinitionID:    def.ID,
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
		Process:         result.Process,
		Phases:          result.Phases,
		TimeoutPhase:    result.TimeoutPhase,
		FailurePhase:    result.FailurePhase,
		Resources:       result.Resources,
		Cleanup:         result.Cleanup,
	}
	run.ObservationStatus, run.Observations = evaluateObservations(ctx, def.Observation, kind, workingDir, result.Stdout, result.Stderr, maxOutput)
	saveCtx := ctx
	saveCancel := func() {}
	if ctx.Err() != nil {
		saveCtx, saveCancel = context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	}
	defer saveCancel()
	if err := s.repo.SaveValidationRun(saveCtx, run); err != nil {
		return nil, err
	}
	return run, nil
}

func normalizeEnvironmentAllowlist(names []string) ([]string, error) {
	if len(names) > maxEnvironmentVariables {
		return nil, fmt.Errorf("%w: at most %d variable names are allowed", ErrInvalidEnvironment, maxEnvironmentVariables)
	}
	if len(names) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(names))
	out := make([]string, 0, len(names))
	for _, name := range names {
		if !environmentNamePattern.MatchString(name) {
			return nil, fmt.Errorf("%w: %q is not a variable name", ErrInvalidEnvironment, name)
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out, nil
}

func resolveEnvironment(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	env := make([]string, 0, len(names))
	for _, name := range names {
		if value, exists := os.LookupEnv(name); exists {
			env = append(env, name+"="+value)
		}
	}
	return env
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
	if err := ValidateEvidence(e); err != nil {
		return err
	}
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	return s.repo.SaveEvidence(ctx, e)
}

// ValidateEvidence validates portable evidence fields without persisting them.
func ValidateEvidence(e *Evidence) error {
	if e == nil {
		return fmt.Errorf("evidence: evidence is nil")
	}
	if !isValidEvidenceType(e.Type) {
		return fmt.Errorf("%w: %q", ErrInvalidEvidenceType, e.Type)
	}
	if !isValidRelation(e.Relation) {
		return fmt.Errorf("%w: %q", ErrInvalidRelation, e.Relation)
	}
	provenance, err := NormalizeSourceRevisions(e.SourceProvenance)
	if err != nil {
		return fmt.Errorf("invalid source provenance: %w", err)
	}
	e.SourceProvenance = provenance
	return nil
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
