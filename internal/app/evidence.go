package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/shlex"
	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/evidence"
)

// DefineValidation stores a validation definition for an investigation.
func (s *Service) DefineValidation(ctx context.Context, investigationID string, opts cli.DefineValidationOptions) (*cli.ValidationResult, error) {
	invSvc, err := s.investigationSvc(ctx)
	if err != nil {
		return nil, err
	}
	inv, err := invSvc.GetInvestigation(ctx, investigationID)
	if err != nil {
		return nil, mapInvestigationError(err)
	}

	command, err := shlex.Split(opts.Command)
	if err != nil {
		return nil, fmt.Errorf("parse validation command: %w", err)
	}
	if len(command) == 0 {
		return nil, errors.New("validation command is required")
	}
	if opts.WorkingDir == "" && (opts.BaseWorkingDir == "" || opts.CandidateDir == "") {
		return nil, errors.New("validation working directory is required")
	}

	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}

	def := &evidence.ValidationDefinition{
		InvestigationID: inv.ID,
		Name:            opts.Kind,
		Kind:            opts.Kind,
		Command:         command,
		WorkingDir:      opts.WorkingDir,
		BaseWorkingDir:  opts.BaseWorkingDir,
		CandidateDir:    opts.CandidateDir,
		Env:             opts.Env,
		Timeout:         opts.Timeout,
		MaxOutputBytes:  opts.MaxOutputBytes,
	}

	evSvc := evidence.NewService(c, evidence.NewExecRunner())
	if err := evSvc.DefineValidation(ctx, def); err != nil {
		return nil, err
	}

	return validationResult(def), nil
}

// ShowValidation returns a stored validation definition without executing it.
func (s *Service) ShowValidation(ctx context.Context, id string) (*cli.ValidationResult, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	def, err := c.GetValidationDefinition(ctx, id)
	if err != nil {
		return nil, mapEvidenceError(err)
	}
	return validationResult(def), nil
}

// RunValidation executes a stored validation definition against the base or candidate workspace.
func (s *Service) RunValidation(ctx context.Context, id string, opts cli.RunValidationOptions) (*cli.ValidationRunResult, error) {
	if !opts.Execute {
		return nil, evidence.ErrExecutionNotAuthorized
	}
	runKind := evidence.RunKind(opts.Kind)
	if runKind != evidence.RunKindBase && runKind != evidence.RunKindCandidate {
		return nil, fmt.Errorf("invalid run kind %q: must be base or candidate", opts.Kind)
	}

	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	evSvc := evidence.NewService(c, evidence.NewExecRunner())
	run, err := evSvc.RunValidation(ctx, id, runKind)
	if err != nil {
		return nil, mapEvidenceError(err)
	}
	return validationRunResult(run), nil
}

// CompareValidation compares a base validation run with a candidate validation run.
func (s *Service) CompareValidation(ctx context.Context, baseRunID, candidateRunID string) (*cli.ValidationComparisonResult, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	evSvc := evidence.NewService(c, evidence.NewExecRunner())
	result, err := evSvc.CompareValidation(ctx, baseRunID, candidateRunID)
	if err != nil {
		return nil, err
	}
	return &cli.ValidationComparisonResult{
		Base:           validationRunResult(result.Base),
		Candidate:      validationRunResult(result.Candidate),
		Classification: string(result.Classification),
		Explanation:    result.Explanation,
	}, nil
}

// ShowEvidence returns the evidence packet for an investigation.
func (s *Service) ShowEvidence(ctx context.Context, investigationID string) (*cli.EvidenceResult, error) {
	invSvc, err := s.investigationSvc(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := invSvc.GetInvestigation(ctx, investigationID); err != nil {
		return nil, mapInvestigationError(err)
	}

	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	evSvc := evidence.NewService(c, evidence.NewExecRunner())
	items, err := evSvc.ListEvidence(ctx, evidence.EvidenceFilter{InvestigationID: investigationID})
	if err != nil {
		return nil, err
	}

	out := make([]cli.EvidenceItem, len(items))
	for i, e := range items {
		out[i] = cli.EvidenceItem{
			ID:              e.ID,
			Type:            string(e.Type),
			Relation:        string(e.Relation),
			Description:     e.Description,
			ValidationRunID: e.ValidationRunID,
			OpportunityID:   e.OpportunityID,
			CreatedAt:       formatTime(e.CreatedAt),
		}
	}

	return &cli.EvidenceResult{
		InvestigationID: investigationID,
		Evidence:        out,
	}, nil
}

func validationResult(def *evidence.ValidationDefinition) *cli.ValidationResult {
	timeout := ""
	if def.Timeout > 0 {
		timeout = def.Timeout.String()
	}
	return &cli.ValidationResult{
		ID:              def.ID,
		InvestigationID: def.InvestigationID,
		Kind:            def.Kind,
		Command:         def.Command,
		WorkingDir:      def.WorkingDir,
		BaseWorkingDir:  def.BaseWorkingDir,
		CandidateDir:    def.CandidateDir,
		Env:             append([]string(nil), def.Env...),
		Timeout:         timeout,
		MaxOutputBytes:  def.MaxOutputBytes,
		CreatedAt:       formatTime(def.CreatedAt),
	}
}

func validationRunResult(run *evidence.ValidationRun) *cli.ValidationRunResult {
	return &cli.ValidationRunResult{
		ID:              run.ID,
		DefinitionID:    run.DefinitionID,
		InvestigationID: run.InvestigationID,
		Kind:            string(run.Kind),
		ExitCode:        run.ExitCode,
		Stdout:          run.Stdout,
		Stderr:          run.Stderr,
		Truncated:       run.Truncated,
		Error:           run.Error,
		Classification:  string(run.Classification),
		StartedAt:       formatTime(run.StartedAt),
		CompletedAt:     formatTime(run.CompletedAt),
	}
}

func mapEvidenceError(err error) error {
	if errors.Is(err, evidence.ErrNotFound) {
		return cli.NewCLIError(cli.ExitNotFound, err)
	}
	return err
}
