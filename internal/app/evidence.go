package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/shlex"
	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/failure"
	"github.com/morluto/gitcontribute/internal/investigation"
	"github.com/morluto/gitcontribute/internal/workspace"
)

// DefineValidation stores a validation definition for an investigation.
func (s *Service) DefineValidation(ctx context.Context, investigationID string, opts contracts.DefineValidationOptions) (*contracts.ValidationResult, error) {
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
		Protocol:             evidence.ValidationProtocol(opts.Protocol),
		ReadinessTimeout:     opts.ReadinessTimeout,
	}

	evSvc := evidence.NewService(c, evidence.NewExecRunner())
	if err := evSvc.DefineValidation(ctx, def); err != nil {
		return nil, err
	}

	return validationResult(def), nil
}

// ShowValidation returns a stored validation definition without executing it.
func (s *Service) ShowValidation(ctx context.Context, id string) (*contracts.ValidationResult, error) {
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
func (s *Service) RunValidation(ctx context.Context, id string, opts contracts.RunValidationOptions) (*contracts.ValidationRunResult, error) {
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

func (s *Service) resolveValidationWorkspaces(ctx context.Context, c *corpus.Corpus, investigationID string, opts *contracts.DefineValidationOptions) error {
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

// RunValidationGroup executes a bounded repeat/stress validation group.
func (s *Service) RunValidationGroup(ctx context.Context, id string, opts contracts.RepeatValidationOptions) (*contracts.ValidationRunGroupResult, error) {
	if !opts.Execute {
		return nil, evidence.ErrExecutionNotAuthorized
	}
	kinds := make([]evidence.RunKind, len(opts.Kinds))
	for index, kind := range opts.Kinds {
		kinds[index] = evidence.RunKind(kind)
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	group, err := evidence.NewService(c, evidence.NewExecRunner()).RunValidationGroup(ctx, id, evidence.RepeatValidationOptions{
		Kinds: kinds, RunCount: opts.RunCount, Concurrency: opts.Concurrency,
		PerRunTimeout: opts.PerRunTimeout, OverallTimeout: opts.OverallTimeout, SampleInterval: opts.SampleInterval,
	})
	if err != nil {
		return nil, mapEvidenceError(err)
	}
	return validationRunGroupResult(group), nil
}

// CompareValidation compares a base validation run with a candidate validation run.
func (s *Service) CompareValidation(ctx context.Context, baseRunID, candidateRunID string) (*contracts.ValidationComparisonResult, error) {
	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return nil, err
	}
	evSvc := evidence.NewService(c, evidence.NewExecRunner())
	result, err := evSvc.CompareValidation(ctx, baseRunID, candidateRunID)
	if err != nil {
		return nil, err
	}
	return &contracts.ValidationComparisonResult{
		Base:           validationRunResult(result.Base),
		Candidate:      validationRunResult(result.Candidate),
		Classification: string(result.Classification),
		Explanation:    result.Explanation,
	}, nil
}

// AttachValidationReceipt imports a structured external receipt without
// executing its declared command.
func (s *Service) AttachValidationReceipt(ctx context.Context, receipt contracts.ExternalValidationReceipt) (*contracts.ValidationRunResult, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	invSvc := investigation.NewService(c, c)
	if _, err := invSvc.GetInvestigation(ctx, receipt.InvestigationID); err != nil {
		return nil, mapInvestigationError(err)
	}
	if receipt.OpportunityID != "" {
		opportunity, err := invSvc.GetOpportunity(ctx, receipt.OpportunityID)
		if err != nil {
			return nil, mapInvestigationError(err)
		}
		if opportunity.InvestigationID != receipt.InvestigationID {
			return nil, errors.New("external receipt opportunity does not belong to its investigation")
		}
	}
	run, err := evidence.NewService(c, nil).AttachExternalReceipt(ctx, evidence.ExternalReceipt{
		SchemaVersion: receipt.SchemaVersion, Producer: receipt.Producer, ReceiptSHA256: receipt.ReceiptSHA256,
		ValidationID:    receipt.ValidationID,
		InvestigationID: receipt.InvestigationID, OpportunityID: receipt.OpportunityID, Kind: evidence.RunKind(receipt.Kind),
		Repository: receipt.Repository, Revision: receipt.Revision, ArtifactSHA256: receipt.ArtifactSHA256,
		Provider: receipt.Provider, ExternalRunID: receipt.ExternalRunID, Command: receipt.Command, WorkingDir: receipt.WorkingDir,
		Environment: receipt.Environment, Artifacts: receipt.Artifacts, StartedAt: receipt.StartedAt, CompletedAt: receipt.CompletedAt,
		ExitCode: receipt.ExitCode, Classification: evidence.RunClassification(receipt.Classification),
		Stdout: receipt.Stdout, Stderr: receipt.Stderr, Truncated: receipt.Truncated,
		Limitations: receipt.Limitations, Incomplete: receipt.Incomplete,
	})
	if err != nil {
		return nil, err
	}
	return validationRunResult(run), nil
}

// ShowEvidence returns the evidence packet for an investigation.
func (s *Service) ShowEvidence(ctx context.Context, investigationID string) (*contracts.EvidenceResult, error) {
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

	out := make([]contracts.EvidenceItem, len(items))
	for i, e := range items {
		out[i], err = evidenceItemResult(ctx, c, e)
		if err != nil {
			return nil, fmt.Errorf("evaluate evidence %q: %w", e.ID, err)
		}
	}

	return &contracts.EvidenceResult{
		InvestigationID: investigationID,
		Evidence:        out,
	}, nil
}

// RecordEvidence stores an evidence item scoped to its parent workflow.
func (s *Service) RecordEvidence(ctx context.Context, input contracts.RecordEvidenceInput) (*evidence.Evidence, error) {
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

func validationResult(def *evidence.ValidationDefinition) *contracts.ValidationResult {
	timeout := ""
	if def.Timeout > 0 {
		timeout = def.Timeout.String()
	}
	readinessTimeout := ""
	if def.ReadinessTimeout > 0 {
		readinessTimeout = def.ReadinessTimeout.String()
	}
	return &contracts.ValidationResult{
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
		Protocol:             string(def.Protocol),
		ReadinessTimeout:     readinessTimeout,
		CreatedAt:            formatTime(def.CreatedAt),
	}
}

func validationRunResult(run *evidence.ValidationRun) *contracts.ValidationRunResult {
	result := &contracts.ValidationRunResult{
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
		Process:                 validationProcessIdentity(run.Process),
		Phases:                  validationPhases(run.Phases),
		TimeoutPhase:            run.TimeoutPhase,
		FailurePhase:            run.FailurePhase,
		Resources:               validationResources(run.Resources),
		Cleanup:                 validationCleanup(run.Cleanup),
		ExecutionOrigin:         run.ExecutionOrigin,
	}
	if run.External != nil {
		result.External = &contracts.ExternalValidationProvenance{
			SchemaVersion: run.External.SchemaVersion, Producer: run.External.Producer, ReceiptSHA256: run.External.ReceiptSHA256,
			ValidationID: run.External.ValidationID,
			Repository:   run.External.Repository, Revision: run.External.Revision, ArtifactSHA256: run.External.ArtifactSHA256,
			Provider: run.External.Provider, ExternalRunID: run.External.ExternalRunID, Command: append([]string(nil), run.External.Command...),
			WorkingDir: run.External.WorkingDir, Environment: run.External.Environment, Artifacts: run.External.Artifacts,
			Limitations: append([]string(nil), run.External.Limitations...), Incomplete: run.External.Incomplete,
		}
	}
	return result
}

func validationRunGroupResult(group *evidence.ValidationRunGroup) *contracts.ValidationRunGroupResult {
	result := &contracts.ValidationRunGroupResult{
		ID: group.ID, DefinitionID: group.DefinitionID, InvestigationID: group.InvestigationID,
		ConfigurationSHA256: group.ConfigurationSHA256, RequestedRuns: group.RequestedRuns, CompletedRuns: group.CompletedRuns,
		Concurrency: group.Concurrency, PerRunTimeout: group.PerRunTimeout.String(), OverallTimeout: group.OverallTimeout.String(),
		SampleInterval: group.SampleInterval.String(), Classification: string(group.Classification),
		StartedAt: formatTime(group.StartedAt), CompletedAt: formatTime(group.CompletedAt),
	}
	for _, attempt := range group.Attempts {
		result.Attempts = append(result.Attempts, contracts.ValidationAttemptResult{
			Index: attempt.Index, Kind: string(attempt.Kind), RunID: attempt.RunID,
			StartedAt: formatTime(attempt.StartedAt), CompletedAt: formatTime(attempt.CompletedAt), ExitCode: attempt.ExitCode,
			Classification: string(attempt.Classification), ObservationStatus: string(attempt.ObservationStatus),
			TimeoutPhase: attempt.TimeoutPhase, FailurePhase: attempt.FailurePhase,
			Error: attempt.Error, Process: validationProcessIdentity(attempt.Process),
			Phases:    validationPhases(attempt.Phases),
			Resources: validationResources(attempt.Resources), Cleanup: validationCleanup(attempt.Cleanup),
		})
	}
	for _, aggregate := range group.Aggregates {
		result.Aggregates = append(result.Aggregates, contracts.ValidationAggregateResult{
			Kind: string(aggregate.Kind), Requested: aggregate.Requested, Completed: aggregate.Completed,
			Passing: aggregate.Passing, Failing: aggregate.Failing, Inconclusive: aggregate.Inconclusive,
			Cancelled: aggregate.Cancelled, Classification: string(aggregate.Classification),
			ResourceClassification: aggregate.ResourceClassification,
		})
	}
	if group.Comparison != nil {
		result.Comparison = &contracts.ValidationGroupComparisonResult{Classification: string(group.Comparison.Classification), Explanation: group.Comparison.Explanation}
	}
	return result
}

func validationPhases(value evidence.RunPhases) contracts.ValidationRunPhases {
	return contracts.ValidationRunPhases{
		SpawnStartedAt: formatTime(value.SpawnStartedAt), ProcessStartedAt: formatTime(value.ProcessStartedAt),
		InitializedAt: formatTime(value.InitializedAt), ToolsListedAt: formatTime(value.ToolsListedAt),
		FirstResponseAt: formatTime(value.FirstResponseAt), ExecutionEndedAt: formatTime(value.ExecutionEndedAt),
		ShutdownStartedAt: formatTime(value.ShutdownStartedAt), ShutdownCheckedAt: formatTime(value.ShutdownCheckedAt),
	}
}

func validationProcessIdentity(value evidence.ProcessIdentity) contracts.ValidationProcessIdentity {
	return contracts.ValidationProcessIdentity{PID: value.PID, CreateTimeUnixMilli: value.CreateTimeUnixMilli}
}

func validationResources(value evidence.ResourceTelemetry) contracts.ValidationResourceTelemetry {
	return contracts.ValidationResourceTelemetry{
		Provider: value.Provider, Platform: value.Platform, SampleInterval: value.SampleInterval.String(), SampleCount: value.SampleCount,
		CPUTimeMillis:              contracts.ValidationInt64Metric{Value: value.CPUTimeMillis.Value, UnavailableReason: value.CPUTimeMillis.UnavailableReason},
		PeakRSSBytes:               contracts.ValidationUint64Metric{Value: value.PeakRSSBytes.Value, UnavailableReason: value.PeakRSSBytes.UnavailableReason},
		PeakChildCount:             contracts.ValidationInt64Metric{Value: value.PeakChildCount.Value, UnavailableReason: value.PeakChildCount.UnavailableReason},
		SamplerOverheadNanoseconds: value.SamplerOverheadNanoseconds,
	}
}

func validationCleanup(value evidence.CleanupResult) contracts.ValidationCleanupResult {
	result := contracts.ValidationCleanupResult{Status: value.Status, Reason: value.Reason, CheckedAt: formatTime(value.CheckedAt)}
	for _, survivor := range value.Survivors {
		result.Survivors = append(result.Survivors, validationProcessIdentity(survivor))
	}
	return result
}

func observationContractToEvidence(contract *contracts.ValidationObservationContract) *evidence.ObservationContract {
	if contract == nil {
		return nil
	}
	return &evidence.ObservationContract{
		Intent:    contract.Intent,
		Base:      expectedObservationsToEvidence(contract.Base),
		Candidate: expectedObservationsToEvidence(contract.Candidate),
	}
}

func expectedObservationsToEvidence(items []contracts.ValidationExpectedObservation) []evidence.ExpectedObservation {
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

func observationContractToCLI(contract *evidence.ObservationContract) *contracts.ValidationObservationContract {
	if contract == nil {
		return nil
	}
	return &contracts.ValidationObservationContract{
		Intent:    contract.Intent,
		Base:      expectedObservationsToCLI(contract.Base),
		Candidate: expectedObservationsToCLI(contract.Candidate),
	}
}

func expectedObservationsToCLI(items []evidence.ExpectedObservation) []contracts.ValidationExpectedObservation {
	out := make([]contracts.ValidationExpectedObservation, len(items))
	for i, item := range items {
		out[i] = contracts.ValidationExpectedObservation{
			Name: item.Name, Source: string(item.Source), Matcher: string(item.Matcher),
			Pattern: item.Pattern, Occurrence: string(item.Occurrence), Path: item.Path,
		}
	}
	return out
}

func observationResultsToCLI(items []evidence.ObservationResult) []contracts.ValidationObservationResult {
	out := make([]contracts.ValidationObservationResult, len(items))
	for i, item := range items {
		out[i] = contracts.ValidationObservationResult{
			ValidationExpectedObservation: expectedObservationsToCLI([]evidence.ExpectedObservation{item.ExpectedObservation})[0],
			Status:                        string(item.Status), Excerpt: item.Excerpt, Error: item.Error,
		}
	}
	return out
}

func mapEvidenceError(err error) error {
	if errors.Is(err, evidence.ErrNotFound) {
		return failure.NotFound(err)
	}
	return err
}
