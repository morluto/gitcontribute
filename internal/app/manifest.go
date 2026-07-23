package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/contribution"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/manifest"
	"github.com/morluto/gitcontribute/internal/workspace"
)

const manifestFacetStaleAfter = 14 * 24 * time.Hour

// ManifestOptions selects optional local workspace and exact stored PR inputs.
type ManifestOptions struct {
	WorkspaceID string
	PullRequest *ManifestPullRequest
}

// ManifestPullRequest identifies one exact stored pull request.
type ManifestPullRequest struct {
	Owner  string
	Repo   string
	Number int
}

// ContributionManifest assembles and persists one bounded local evidence statement.
func (s *Service) ContributionManifest(ctx context.Context, opportunityID string, opts ManifestOptions) (*manifest.Statement, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	opp, inv, err := s.loadOpportunityAndRepo(ctx, c, opportunityID)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	predicate := manifest.Predicate{
		GeneratedAt: now,
		Repository:  manifest.RepositoryIdentity{Owner: inv.Repo.Owner, Repo: inv.Repo.Repo, CommitSHA: inv.CommitSHA},
		Opportunity: manifest.OpportunityRecord{
			ID: opp.ID, InvestigationID: opp.InvestigationID, HypothesisID: opp.HypothesisID,
			ProblemStatement: opp.ProblemStatement, Scope: opp.Scope, Impact: opp.Impact,
			Status: string(opp.Status), SourceRefs: append([]domain.SourceRef(nil), opp.SourceRefs...),
		},
	}
	if err := s.addManifestWorkspace(ctx, c, inv.ID, inv.Repo.Owner, inv.Repo.Repo, opts.WorkspaceID, &predicate); err != nil {
		return nil, err
	}
	if err := addManifestValidations(ctx, c, &predicate); err != nil {
		return nil, err
	}
	if err := addManifestEvidence(ctx, c, &predicate); err != nil {
		return nil, err
	}
	if err := s.addManifestReadiness(ctx, opp.ID, &predicate); err != nil {
		return nil, err
	}
	pullRequestRef := ""
	if opts.PullRequest != nil {
		pullRequestRef, err = s.addManifestPullRequest(ctx, c, inv.Repo.Owner, inv.Repo.Repo, *opts.PullRequest, now, &predicate)
		if err != nil {
			return nil, err
		}
	} else {
		predicate.Completeness = append(predicate.Completeness, manifest.CompletenessFacet{Facet: "pull_request", Status: "not_requested", Reason: "no exact pull request was selected"})
	}
	if err := addManifestDrafts(ctx, c, opp.ID, &predicate); err != nil {
		return nil, err
	}
	predicate.Status = "complete"
	if len(predicate.Gaps) > 0 {
		predicate.Status = "incomplete"
	}
	sortManifestPredicate(&predicate)
	statement, err := manifest.Finalize(predicate)
	if err != nil {
		return nil, err
	}
	if err := c.SaveContributionManifest(ctx, &statement, opts.WorkspaceID, pullRequestRef); err != nil {
		return nil, err
	}
	return &statement, nil
}

// ExportManifest generates, persists, and renders one local JSON statement.
func (s *Service) ExportManifest(ctx context.Context, opportunityID string, opts cli.ManifestExportOptions) (*cli.ExportResult, error) {
	manifestOpts := ManifestOptions{WorkspaceID: opts.WorkspaceID}
	if opts.PullRequest != nil {
		manifestOpts.PullRequest = &ManifestPullRequest{Owner: opts.PullRequest.Owner, Repo: opts.PullRequest.Repo, Number: opts.PullRequest.Number}
	}
	statement, err := s.ContributionManifest(ctx, opportunityID, manifestOpts)
	if err != nil {
		return nil, err
	}
	payload, err := json.MarshalIndent(statement, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode contribution manifest: %w", err)
	}
	return &cli.ExportResult{Kind: "manifest", Format: "json", Content: string(payload)}, nil
}

func (s *Service) addManifestWorkspace(ctx context.Context, c *corpus.Corpus, investigationID, owner, repo, workspaceID string, predicate *manifest.Predicate) error {
	if workspaceID == "" {
		predicate.Completeness = append(predicate.Completeness, manifest.CompletenessFacet{Facet: "workspace", Status: "not_requested", Reason: "no workspace was selected"})
		return nil
	}
	item, err := c.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return mapWorkspaceError(err)
	}
	if item.InvestigationID != investigationID || !strings.EqualFold(item.RepoOwner, owner) || !strings.EqualFold(item.RepoName, repo) {
		return fmt.Errorf("%w: workspace %q belongs to another investigation or repository", manifest.ErrIdentityMismatch, workspaceID)
	}
	manager, err := s.workspaceReader()
	if err != nil {
		return err
	}
	snapshot, err := manager.SnapshotByPath(ctx, item.Path, item.BaseSHA, item.MergeBase)
	if err != nil {
		return fmt.Errorf("snapshot workspace %q: %w", workspaceID, err)
	}
	predicate.Workspace = &snapshot
	status, reason := "complete", "workspace content is fully digest-bound"
	if !snapshot.Complete {
		status, reason = "incomplete", "workspace snapshot has explicitly unbound content"
		for _, gap := range snapshot.Gaps {
			predicate.Gaps = append(predicate.Gaps, manifest.Gap{Code: gap.Code, Facet: "workspace", Reason: gap.Reason})
		}
	}
	predicate.Completeness = append(predicate.Completeness, manifest.CompletenessFacet{Facet: "workspace", Status: status, Reason: reason})
	return nil
}

func addManifestValidations(ctx context.Context, c *corpus.Corpus, predicate *manifest.Predicate) error {
	definitions, err := c.ListValidationDefinitions(ctx, predicate.Opportunity.ID)
	if err != nil {
		return err
	}
	runs, err := c.ListValidationRuns(ctx, predicate.Opportunity.ID)
	if err != nil {
		return err
	}
	byID := make(map[string]*evidence.ValidationDefinition, len(definitions))
	runCount := make(map[string]int, len(definitions))
	selectedRuns := latestValidationRuns(runs)
	for _, definition := range definitions {
		byID[definition.ID] = definition
	}
	for _, run := range runs {
		runCount[run.DefinitionID]++
		definition := byID[run.DefinitionID]
		if definition == nil {
			predicate.Gaps = append(predicate.Gaps, manifest.Gap{Code: "validation_definition_missing", Facet: "validations", Reason: "run " + run.ID + " has no stored definition"})
			continue
		}
		record, gaps, err := buildManifestValidation(definition, run, predicate.Workspace, selectedRuns[run.DefinitionID+"\x00"+string(run.Kind)] == run.ID)
		if err != nil {
			return err
		}
		predicate.Gaps = append(predicate.Gaps, gaps...)
		predicate.Validations = append(predicate.Validations, record)
	}
	for _, definition := range definitions {
		if runCount[definition.ID] == 0 {
			predicate.Gaps = append(predicate.Gaps, manifest.Gap{Code: "validation_run_missing", Facet: "validations", Reason: "definition " + definition.ID + " has no stored run"})
		}
	}
	status, reason := "complete", "all stored validation runs have compatible workspace bindings and observations"
	if len(definitions) == 0 {
		status, reason = "unknown", "no validation definitions are stored"
		predicate.Gaps = append(predicate.Gaps, manifest.Gap{Code: "validations_missing", Facet: "validations", Reason: reason})
	} else if len(runs) == 0 {
		status, reason = "unknown", "no validation runs are stored"
		predicate.Gaps = append(predicate.Gaps, manifest.Gap{Code: "validations_missing", Facet: "validations", Reason: reason})
	} else if hasManifestGap(predicate.Gaps, "validations") {
		status, reason = "incomplete", "one or more validation claims are stale, unknown, or unverified"
	}
	predicate.Completeness = append(predicate.Completeness, manifest.CompletenessFacet{Facet: "validations", Status: status, Reason: reason})
	return nil
}

func buildManifestValidation(definition *evidence.ValidationDefinition, run *evidence.ValidationRun, current *workspace.Snapshot, selected bool) (manifest.ValidationRecord, []manifest.Gap, error) {
	commandDigest, err := digestJSON(definition.Command)
	if err != nil {
		return manifest.ValidationRecord{}, nil, err
	}
	executionDigest, err := digestJSON(validationExecutionContract{
		Command: definition.Command, WorkingDir: definition.WorkingDir, BaseWorkingDir: definition.BaseWorkingDir,
		CandidateDir: definition.CandidateDir, WorkspaceID: definition.WorkspaceID, BaseWorkspaceID: definition.BaseWorkspaceID,
		CandidateWorkspaceID: definition.CandidateWorkspaceID, Env: definition.Env, Timeout: definition.Timeout,
		MaxOutputBytes: definition.MaxOutputBytes, Observation: definition.Observation,
	})
	if err != nil {
		return manifest.ValidationRecord{}, nil, err
	}
	record := manifest.ValidationRecord{
		DefinitionID: definition.ID, RunID: run.ID, Kind: string(run.Kind), Command: append([]string(nil), definition.Command...),
		CommandSHA256: commandDigest, ExecutionContractSHA256: executionDigest, EnvironmentAllowlist: append([]string(nil), definition.Env...),
		Timeout: definition.Timeout.String(), MaxOutputBytes: definition.MaxOutputBytes, Observation: definition.Observation,
		Classification: string(run.Classification), ObservationStatus: string(run.ObservationStatus),
		Observations: append([]evidence.ObservationResult(nil), run.Observations...), StartedAt: run.StartedAt, CompletedAt: run.CompletedAt,
		WorkspaceSnapshotBefore: run.WorkspaceSnapshotBefore, WorkspaceSnapshotAfter: run.WorkspaceSnapshotAfter,
		WorkspaceBindingStatus: run.WorkspaceBindingStatus, Selected: selected,
	}
	record.WorkspaceCompatibility, record.CompatibilityReason = validationWorkspaceCompatibility(run, current)
	if !selected {
		return record, nil, nil
	}
	var gaps []manifest.Gap
	if record.WorkspaceCompatibility != "compatible" {
		gaps = append(gaps, manifest.Gap{Code: "validation_workspace_" + record.WorkspaceCompatibility, Facet: "validations", Reason: "run " + run.ID + ": " + record.CompatibilityReason})
	}
	if run.Classification == evidence.RunClassificationError || run.Classification == evidence.RunClassificationCancelled ||
		(run.Kind == evidence.RunKindCandidate && run.Classification != evidence.RunClassificationPassing) {
		gaps = append(gaps, manifest.Gap{Code: "validation_outcome_" + string(run.Classification), Facet: "validations", Reason: "run " + run.ID + " cannot support the candidate claim"})
	}
	if definition.Observation != nil && run.ObservationStatus != evidence.ObservationMatched {
		gaps = append(gaps, manifest.Gap{Code: "validation_observation_unverified", Facet: "validations", Reason: "run " + run.ID + " did not match its expected observation contract"})
	}
	return record, gaps, nil
}

func latestValidationRuns(runs []*evidence.ValidationRun) map[string]string {
	selected := make(map[string]*evidence.ValidationRun)
	for _, run := range runs {
		key := run.DefinitionID + "\x00" + string(run.Kind)
		current := selected[key]
		if current == nil || run.CompletedAt.After(current.CompletedAt) || (run.CompletedAt.Equal(current.CompletedAt) && run.ID > current.ID) {
			selected[key] = run
		}
	}
	ids := make(map[string]string, len(selected))
	for key, run := range selected {
		ids[key] = run.ID
	}
	return ids
}

type validationExecutionContract struct {
	Command              []string
	WorkingDir           string
	BaseWorkingDir       string
	CandidateDir         string
	WorkspaceID          string
	BaseWorkspaceID      string
	CandidateWorkspaceID string
	Env                  []string
	Timeout              time.Duration
	MaxOutputBytes       int64
	Observation          *evidence.ObservationContract
}

func validationWorkspaceCompatibility(run *evidence.ValidationRun, current *workspace.Snapshot) (string, string) {
	if run.WorkspaceBindingStatus != "bound" {
		status := run.WorkspaceBindingStatus
		if status == "" {
			status = "unknown"
		}
		return status, run.WorkspaceBindingReason
	}
	if run.Kind != evidence.RunKindCandidate {
		return "compatible", "base validation has a stable pre/post workspace identity"
	}
	if current == nil {
		return "unknown", "no current candidate workspace was selected for comparison"
	}
	if run.WorkspaceSnapshotBefore != current.SHA256 {
		return "stale", "current candidate workspace identity differs from the validated identity"
	}
	return "compatible", "current candidate workspace matches the validated identity"
}

func addManifestEvidence(ctx context.Context, c *corpus.Corpus, predicate *manifest.Predicate) error {
	items, err := c.ListEvidence(ctx, evidence.EvidenceFilter{OpportunityID: predicate.Opportunity.ID})
	if err != nil {
		return err
	}
	freshness := evidence.NewFreshnessEvaluator(c)
	for _, item := range items {
		assessment, err := freshness.Evaluate(ctx, item)
		if err != nil {
			return err
		}
		record := manifest.EvidenceRecord{
			ID: item.ID, Type: string(item.Type), Relation: string(item.Relation), Description: item.Description,
			ValidationRunID: item.ValidationRunID, SourceRefs: append([]domain.SourceRef(nil), item.SourceRefs...),
			SourceProvenance: append([]evidence.SourceRevision(nil), item.SourceProvenance...),
			Freshness:        string(assessment.Status), FreshnessReason: assessment.Reason,
		}
		predicate.Evidence = append(predicate.Evidence, record)
		if assessment.Status == evidence.FreshnessStale || assessment.Status == evidence.FreshnessUnknown {
			predicate.Gaps = append(predicate.Gaps, manifest.Gap{Code: "evidence_" + string(assessment.Status), Facet: "evidence", Reason: "evidence " + item.ID + ": " + assessment.Reason})
		}
	}
	status, reason := "complete", "all evidence is fresh or local-only"
	if len(items) == 0 {
		status, reason = "unknown", "no evidence is scoped to the opportunity"
		predicate.Gaps = append(predicate.Gaps, manifest.Gap{Code: "evidence_missing", Facet: "evidence", Reason: reason})
	} else if hasManifestGap(predicate.Gaps, "evidence") {
		status, reason = "incomplete", "some evidence is stale or has unknown freshness"
	}
	predicate.Completeness = append(predicate.Completeness, manifest.CompletenessFacet{Facet: "evidence", Status: status, Reason: reason})
	return nil
}

func (s *Service) addManifestReadiness(ctx context.Context, opportunityID string, predicate *manifest.Predicate) error {
	report, err := s.OpportunityReadiness(ctx, opportunityID)
	if err != nil {
		return err
	}
	predicate.Readiness = manifest.ReadinessRecord{RuleSetVersion: report.RuleSetVersion, Status: report.Status, EvaluatedAt: report.EvaluatedAt}
	for _, check := range report.Checks {
		predicate.Readiness.Checks = append(predicate.Readiness.Checks, manifest.ReadinessCheck{
			RuleID: check.RuleID, RuleVersion: check.RuleVersion, Status: check.Status,
			Summary: check.Summary, EvidenceRefs: append([]string(nil), check.EvidenceRefs...),
		})
		if check.Status == readinessUnknown || check.Status == readinessBlock {
			predicate.Gaps = append(predicate.Gaps, manifest.Gap{Code: "readiness_" + check.Status, Facet: "readiness", Reason: check.RuleID + ": " + check.Summary})
		}
	}
	status, reason := "complete", "readiness has no blocking or unknown checks"
	if hasManifestGap(predicate.Gaps, "readiness") {
		status, reason = "incomplete", "readiness includes blocking or unknown checks"
	}
	predicate.Completeness = append(predicate.Completeness, manifest.CompletenessFacet{Facet: "readiness", Status: status, Reason: reason})
	return nil
}

func (s *Service) addManifestPullRequest(ctx context.Context, c *corpus.Corpus, owner, repo string, selector ManifestPullRequest, now time.Time, predicate *manifest.Predicate) (string, error) {
	if !strings.EqualFold(selector.Owner, owner) || !strings.EqualFold(selector.Repo, repo) || selector.Number <= 0 {
		return "", fmt.Errorf("%w: pull request does not match the opportunity repository", manifest.ErrIdentityMismatch)
	}
	storedRepo, err := c.GetRepository(ctx, selector.Owner, selector.Repo)
	if err != nil {
		return "", err
	}
	thread, err := c.GetThread(ctx, storedRepo.ID, corpus.ThreadKindPullRequest, selector.Number)
	if err != nil {
		return "", err
	}
	item, err := portfolioItem(ctx, c, corpus.PortfolioPullRequest{Owner: storedRepo.Owner, Repo: storedRepo.Name, Thread: *thread}, now)
	if err != nil {
		return "", err
	}
	if predicate.Workspace != nil && item.HeadSHA != "" && item.HeadSHA != predicate.Workspace.HeadSHA {
		return "", fmt.Errorf("%w: pull request head %s differs from workspace head %s", manifest.ErrIdentityMismatch, item.HeadSHA, predicate.Workspace.HeadSHA)
	}
	record := manifest.PullRequestRecord{
		Owner: item.Owner, Repo: item.Repo, Number: item.Number, State: item.State,
		HeadSHA: item.HeadSHA, BaseSHA: item.BaseSHA, ChecksStatus: item.ChecksStatus,
		ReviewDecision: item.ReviewDecision, UnresolvedReviewThreads: item.UnresolvedReviewThreads,
		MergeStateStatus: item.MergeStateStatus, MergeQueueState: item.MergeQueueState,
		Attention: item.Attention, SourceUpdatedAt: item.SourceUpdatedAt,
	}
	complete := true
	for _, facet := range item.Facets {
		status := facet.Status
		if status == "complete" {
			updatedAt, parseErr := time.Parse(time.RFC3339, facet.UpdatedAt)
			switch {
			case parseErr != nil:
				status = "unknown"
			case updatedAt.After(now.Add(5 * time.Minute)):
				status = "unknown"
			case now.Sub(updatedAt) > manifestFacetStaleAfter:
				status = "stale"
			}
		}
		record.Facets = append(record.Facets, manifest.FacetStatus{Facet: facet.Facet, Status: status, UpdatedAt: facet.UpdatedAt})
		if status != "complete" {
			complete = false
			predicate.Gaps = append(predicate.Gaps, manifest.Gap{Code: "pull_request_facet_" + status, Facet: "pull_request", Reason: facet.Facet + " is " + status})
		}
	}
	predicate.PullRequest = &record
	status, reason := "complete", "all requested pull-request health facets are complete and current"
	if !complete {
		status, reason = "incomplete", "one or more pull-request health facets are missing, stale, or incomplete"
	}
	predicate.Completeness = append(predicate.Completeness, manifest.CompletenessFacet{Facet: "pull_request", Status: status, Reason: reason})
	return fmt.Sprintf("%s/%s#%d", item.Owner, item.Repo, item.Number), nil
}

func addManifestDrafts(ctx context.Context, c *corpus.Corpus, opportunityID string, predicate *manifest.Predicate) error {
	if draft, err := c.GetIssueDraft(ctx, opportunityID); err == nil {
		predicate.Drafts = append(predicate.Drafts, manifest.DraftRecord{Kind: "issue", Title: draft.Title, RenderedAt: draft.RenderedAt, ManifestID: draft.ManifestID})
	} else if !errors.Is(err, contribution.ErrNotFound) {
		return err
	}
	if draft, err := c.GetPullRequestDraft(ctx, opportunityID); err == nil {
		predicate.Drafts = append(predicate.Drafts, manifest.DraftRecord{Kind: "pull_request", Title: draft.Title, RenderedAt: draft.RenderedAt, ManifestID: draft.ManifestID})
	} else if !errors.Is(err, contribution.ErrNotFound) {
		return err
	}
	return nil
}

func sortManifestPredicate(predicate *manifest.Predicate) {
	sort.Slice(predicate.Validations, func(i, j int) bool {
		if !predicate.Validations[i].StartedAt.Equal(predicate.Validations[j].StartedAt) {
			return predicate.Validations[i].StartedAt.Before(predicate.Validations[j].StartedAt)
		}
		return predicate.Validations[i].RunID < predicate.Validations[j].RunID
	})
	sort.Slice(predicate.Evidence, func(i, j int) bool { return predicate.Evidence[i].ID < predicate.Evidence[j].ID })
	sort.Slice(predicate.Drafts, func(i, j int) bool { return predicate.Drafts[i].Kind < predicate.Drafts[j].Kind })
	sort.Slice(predicate.Completeness, func(i, j int) bool { return predicate.Completeness[i].Facet < predicate.Completeness[j].Facet })
	sort.Slice(predicate.Gaps, func(i, j int) bool {
		if predicate.Gaps[i].Facet != predicate.Gaps[j].Facet {
			return predicate.Gaps[i].Facet < predicate.Gaps[j].Facet
		}
		if predicate.Gaps[i].Code != predicate.Gaps[j].Code {
			return predicate.Gaps[i].Code < predicate.Gaps[j].Code
		}
		return predicate.Gaps[i].Reason < predicate.Gaps[j].Reason
	})
}

func hasManifestGap(gaps []manifest.Gap, facet string) bool {
	for _, gap := range gaps {
		if gap.Facet == facet {
			return true
		}
	}
	return false
}

func digestJSON(value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode manifest digest input: %w", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}
