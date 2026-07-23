package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/shlex"
	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/workspace"
)

// DefineValidation stores a validation definition for an investigation.
func (s *Service) DefineValidation(ctx context.Context, investigationID string, opts cli.DefineValidationOptions) (*cli.ValidationResult, error) {
	invSvc, err := s.writeInvestigationSvc(ctx)
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
	if opts.WorkspaceID == "" && opts.BaseWorkspaceID == "" && opts.CandidateWorkspaceID == "" && opts.WorkingDir == "" && (opts.BaseWorkingDir == "" || opts.CandidateDir == "") {
		return nil, errors.New("validation working directory is required")
	}

	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.resolveValidationWorkspaces(ctx, c, inv.ID, &opts); err != nil {
		return nil, err
	}

	def := &evidence.ValidationDefinition{
		InvestigationID:      inv.ID,
		Name:                 opts.Kind,
		Kind:                 opts.Kind,
		Command:              command,
		WorkingDir:           opts.WorkingDir,
		BaseWorkingDir:       opts.BaseWorkingDir,
		CandidateDir:         opts.CandidateDir,
		WorkspaceID:          opts.WorkspaceID,
		BaseWorkspaceID:      opts.BaseWorkspaceID,
		CandidateWorkspaceID: opts.CandidateWorkspaceID,
		Env:                  opts.Env,
		Timeout:              opts.Timeout,
		MaxOutputBytes:       opts.MaxOutputBytes,
		Observation:          observationContractToEvidence(opts.Observation),
	}

	evSvc := evidence.NewService(c, evidence.NewExecRunner())
	if err := evSvc.DefineValidation(ctx, def); err != nil {
		return nil, err
	}

	return validationResult(def), nil
}

// ShowValidation returns a stored validation definition without executing it.
func (s *Service) ShowValidation(ctx context.Context, id string) (*cli.ValidationResult, error) {
	c, err := s.openReadOnlyCorpus(ctx)
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
	def, err := c.GetValidationDefinition(ctx, id)
	if err != nil {
		return nil, mapEvidenceError(err)
	}
	workspaceID := def.WorkspaceID
	if runKind == evidence.RunKindBase && def.BaseWorkspaceID != "" {
		workspaceID = def.BaseWorkspaceID
	}
	if runKind == evidence.RunKindCandidate && def.CandidateWorkspaceID != "" {
		workspaceID = def.CandidateWorkspaceID
	}
	var before workspace.Snapshot
	var beforeErr error
	var managedWorkspace *workspace.Workspace
	if workspaceID != "" {
		managedWorkspace, beforeErr = c.GetWorkspace(ctx, workspaceID)
		if beforeErr == nil {
			var manager *workspace.Manager
			manager, beforeErr = s.workspaceReader()
			if beforeErr == nil {
				before, beforeErr = manager.SnapshotByPath(ctx, managedWorkspace.Path, managedWorkspace.BaseSHA, managedWorkspace.MergeBase)
			}
		}
	}
	evSvc := evidence.NewService(c, evidence.NewExecRunner())
	run, err := evSvc.RunValidation(ctx, id, runKind)
	if err != nil {
		return nil, mapEvidenceError(err)
	}
	if err := bindValidationWorkspace(ctx, s, c, run, managedWorkspace, before, beforeErr); err != nil {
		return nil, err
	}
	return validationRunResult(run), nil
}

func (s *Service) resolveValidationWorkspaces(ctx context.Context, c *corpus.Corpus, investigationID string, opts *cli.DefineValidationOptions) error {
	if opts.WorkspaceID == "" && opts.BaseWorkspaceID == "" && opts.CandidateWorkspaceID == "" {
		return nil
	}
	manager, err := s.workspaceReader()
	if err != nil {
		return fmt.Errorf("open managed workspaces: %w", err)
	}
	resolve := func(id string) (*workspace.Workspace, error) {
		item, err := c.GetWorkspace(ctx, id)
		if err != nil {
			return nil, mapWorkspaceError(err)
		}
		if item.InvestigationID != investigationID {
			return nil, errors.New("workspace does not belong to the validation investigation")
		}
		if err := manager.ValidateWorkspacePath(item.Path); err != nil {
			return nil, fmt.Errorf("workspace %q path is not managed: %w", id, err)
		}
		return item, nil
	}
	if opts.WorkspaceID != "" {
		if opts.BaseWorkspaceID != "" || opts.CandidateWorkspaceID != "" || opts.WorkingDir != "" || opts.BaseWorkingDir != "" || opts.CandidateDir != "" {
			return errors.New("workspace-id cannot be combined with other workspace selectors")
		}
		item, err := resolve(opts.WorkspaceID)
		if err != nil {
			return mapWorkspaceError(err)
		}
		opts.WorkingDir = item.Path
		return nil
	}
	if opts.BaseWorkspaceID != "" || opts.CandidateWorkspaceID != "" {
		if opts.BaseWorkspaceID == "" || opts.CandidateWorkspaceID == "" || opts.WorkingDir != "" || opts.BaseWorkingDir != "" || opts.CandidateDir != "" {
			return errors.New("base-workspace-id and candidate-workspace-id must be provided together without directory selectors")
		}
		base, err := resolve(opts.BaseWorkspaceID)
		if err != nil {
			return mapWorkspaceError(err)
		}
		candidate, err := resolve(opts.CandidateWorkspaceID)
		if err != nil {
			return mapWorkspaceError(err)
		}
		opts.BaseWorkingDir, opts.CandidateDir = base.Path, candidate.Path
	}
	return nil
}

func bindValidationWorkspace(ctx context.Context, service *Service, c *corpus.Corpus, run *evidence.ValidationRun, managed *workspace.Workspace, before workspace.Snapshot, beforeErr error) error {
	run.WorkspaceBindingStatus = "unavailable"
	switch {
	case beforeErr != nil:
		run.WorkspaceBindingReason = "capture pre-run workspace snapshot: " + beforeErr.Error()
	case managed == nil:
		run.WorkspaceBindingReason = "validation did not declare a managed workspace"
	default:
		run.WorkspaceSnapshotBefore = before.SHA256
		manager, err := service.workspaceReader()
		if err != nil {
			run.WorkspaceBindingReason = "open workspace reader after validation: " + err.Error()
			break
		}
		snapshotCtx, snapshotCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		after, err := manager.SnapshotByPath(snapshotCtx, managed.Path, managed.BaseSHA, managed.MergeBase)
		snapshotCancel()
		if err != nil {
			run.WorkspaceBindingReason = "capture post-run workspace snapshot: " + err.Error()
			break
		}
		run.WorkspaceSnapshotAfter = after.SHA256
		switch {
		case !before.Complete || !after.Complete:
			run.WorkspaceBindingStatus = "incomplete"
			run.WorkspaceBindingReason = "workspace snapshot contains explicitly unbound content"
		case before.SHA256 != after.SHA256:
			run.WorkspaceBindingStatus = "changed"
			run.WorkspaceBindingReason = "workspace changed while validation was running"
		default:
			run.WorkspaceBindingStatus = "bound"
			run.WorkspaceBindingReason = "pre-run and post-run workspace identities match"
		}
	}
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := c.SaveValidationRun(saveCtx, run); err != nil {
		return fmt.Errorf("save validation workspace binding: %w", err)
	}
	return nil
}

// CompareValidation compares a base validation run with a candidate validation run.
func (s *Service) CompareValidation(ctx context.Context, baseRunID, candidateRunID string) (*cli.ValidationComparisonResult, error) {
	c, err := s.openReadOnlyCorpus(ctx)
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
	invSvc, err := s.readInvestigationSvc(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := invSvc.GetInvestigation(ctx, investigationID); err != nil {
		return nil, mapInvestigationError(err)
	}

	c, err := s.openReadOnlyCorpus(ctx)
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
		out[i], err = evidenceItemResult(ctx, c, e)
		if err != nil {
			return nil, fmt.Errorf("evaluate evidence %q: %w", e.ID, err)
		}
	}

	return &cli.EvidenceResult{
		InvestigationID: investigationID,
		Evidence:        out,
	}, nil
}

// RecordEvidenceInput carries a new evidence item for an investigation,
// hypothesis, or opportunity.
type RecordEvidenceInput struct {
	InvestigationID  string
	HypothesisID     string
	OpportunityID    string
	Type             string
	Relation         string
	Description      string
	SourceRefs       []domain.SourceRef
	SourceProvenance []evidence.SourceRevision
}

// RecordEvidence stores an evidence item scoped to its parent workflow.
func (s *Service) RecordEvidence(ctx context.Context, input RecordEvidenceInput) (*evidence.Evidence, error) {
	if strings.TrimSpace(input.Description) == "" {
		return nil, errors.New("evidence description is required")
	}

	invSvc, err := s.writeInvestigationSvc(ctx)
	if err != nil {
		return nil, err
	}

	var investigationID, hypothesisID, opportunityID string
	switch {
	case input.OpportunityID != "":
		o, err := invSvc.GetOpportunity(ctx, input.OpportunityID)
		if err != nil {
			return nil, mapInvestigationError(err)
		}
		opportunityID = o.ID
		hypothesisID = o.HypothesisID
		investigationID = o.InvestigationID
	case input.HypothesisID != "":
		h, err := invSvc.GetHypothesis(ctx, input.HypothesisID)
		if err != nil {
			return nil, mapInvestigationError(err)
		}
		hypothesisID = h.ID
		investigationID = h.InvestigationID
	case input.InvestigationID != "":
		investigationID = input.InvestigationID
	default:
		return nil, errors.New("an investigation, hypothesis, or opportunity scope is required")
	}
	inv, err := invSvc.GetInvestigation(ctx, investigationID)
	if err != nil {
		return nil, mapInvestigationError(err)
	}

	sourceRefs := append([]domain.SourceRef(nil), input.SourceRefs...)
	provenance := append([]evidence.SourceRevision(nil), input.SourceProvenance...)
	if len(provenance) == 0 && evidence.EvidenceType(input.Type) == evidence.EvidenceTypeGitHubSource && inv.ThreadBaseline != nil {
		provenance = []evidence.SourceRevision{sourceRevisionFromThreadBaseline(*inv.ThreadBaseline)}
		if len(sourceRefs) == 0 {
			sourceRefs = []domain.SourceRef{inv.ThreadBaseline.Source}
		}
	}

	e := &evidence.Evidence{
		InvestigationID:  investigationID,
		HypothesisID:     hypothesisID,
		OpportunityID:    opportunityID,
		Type:             evidence.EvidenceType(input.Type),
		Relation:         evidence.Relation(input.Relation),
		Description:      strings.TrimSpace(input.Description),
		SourceRefs:       sourceRefs,
		SourceProvenance: provenance,
	}

	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	evSvc := evidence.NewService(c, evidence.NewExecRunner())
	if err := evSvc.CreateEvidence(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

func validationResult(def *evidence.ValidationDefinition) *cli.ValidationResult {
	timeout := ""
	if def.Timeout > 0 {
		timeout = def.Timeout.String()
	}
	return &cli.ValidationResult{
		ID:                   def.ID,
		InvestigationID:      def.InvestigationID,
		Kind:                 def.Kind,
		Command:              def.Command,
		WorkingDir:           def.WorkingDir,
		BaseWorkingDir:       def.BaseWorkingDir,
		CandidateDir:         def.CandidateDir,
		WorkspaceID:          def.WorkspaceID,
		BaseWorkspaceID:      def.BaseWorkspaceID,
		CandidateWorkspaceID: def.CandidateWorkspaceID,
		Env:                  append([]string(nil), def.Env...),
		Timeout:              timeout,
		MaxOutputBytes:       def.MaxOutputBytes,
		Observation:          observationContractToCLI(def.Observation),
		CreatedAt:            formatTime(def.CreatedAt),
	}
}

func validationRunResult(run *evidence.ValidationRun) *cli.ValidationRunResult {
	return &cli.ValidationRunResult{
		ID:                      run.ID,
		DefinitionID:            run.DefinitionID,
		InvestigationID:         run.InvestigationID,
		Kind:                    string(run.Kind),
		ExitCode:                run.ExitCode,
		Stdout:                  run.Stdout,
		Stderr:                  run.Stderr,
		Truncated:               run.Truncated,
		Error:                   run.Error,
		Classification:          string(run.Classification),
		ObservationStatus:       string(run.ObservationStatus),
		Observations:            observationResultsToCLI(run.Observations),
		StartedAt:               formatTime(run.StartedAt),
		CompletedAt:             formatTime(run.CompletedAt),
		WorkspaceSnapshotBefore: run.WorkspaceSnapshotBefore,
		WorkspaceSnapshotAfter:  run.WorkspaceSnapshotAfter,
		WorkspaceBindingStatus:  run.WorkspaceBindingStatus,
		WorkspaceBindingReason:  run.WorkspaceBindingReason,
	}
}

func observationContractToEvidence(contract *cli.ValidationObservationContract) *evidence.ObservationContract {
	if contract == nil {
		return nil
	}
	return &evidence.ObservationContract{
		Intent:    contract.Intent,
		Base:      expectedObservationsToEvidence(contract.Base),
		Candidate: expectedObservationsToEvidence(contract.Candidate),
	}
}

func expectedObservationsToEvidence(items []cli.ValidationExpectedObservation) []evidence.ExpectedObservation {
	out := make([]evidence.ExpectedObservation, len(items))
	for i, item := range items {
		out[i] = evidence.ExpectedObservation{
			Name: item.Name, Source: evidence.ObservationSource(item.Source),
			Matcher: evidence.ObservationMatcher(item.Matcher), Pattern: item.Pattern,
			Occurrence: evidence.ObservationOccurrence(item.Occurrence),
			Path:       item.Path,
		}
	}
	return out
}

func observationContractToCLI(contract *evidence.ObservationContract) *cli.ValidationObservationContract {
	if contract == nil {
		return nil
	}
	return &cli.ValidationObservationContract{
		Intent:    contract.Intent,
		Base:      expectedObservationsToCLI(contract.Base),
		Candidate: expectedObservationsToCLI(contract.Candidate),
	}
}

func expectedObservationsToCLI(items []evidence.ExpectedObservation) []cli.ValidationExpectedObservation {
	out := make([]cli.ValidationExpectedObservation, len(items))
	for i, item := range items {
		out[i] = cli.ValidationExpectedObservation{
			Name: item.Name, Source: string(item.Source), Matcher: string(item.Matcher),
			Pattern: item.Pattern, Occurrence: string(item.Occurrence), Path: item.Path,
		}
	}
	return out
}

func observationResultsToCLI(items []evidence.ObservationResult) []cli.ValidationObservationResult {
	out := make([]cli.ValidationObservationResult, len(items))
	for i, item := range items {
		out[i] = cli.ValidationObservationResult{
			ValidationExpectedObservation: expectedObservationsToCLI([]evidence.ExpectedObservation{item.ExpectedObservation})[0],
			Status:                        string(item.Status), Excerpt: item.Excerpt, Error: item.Error,
		}
	}
	return out
}

func mapEvidenceError(err error) error {
	if errors.Is(err, evidence.ErrNotFound) {
		return cli.NewCLIError(cli.ExitNotFound, err)
	}
	return err
}
