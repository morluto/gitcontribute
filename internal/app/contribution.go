package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/contribution"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/investigation"
)

const maxPreparedDiffBytes = 1 << 20

// PrepareIssue renders and stores an issue draft for an opportunity.
func (s *Service) PrepareIssue(ctx context.Context, opportunityID string, opts cli.PrepareIssueOptions) (*cli.DraftResult, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}

	opp, inv, err := s.loadOpportunityAndRepo(ctx, c, opportunityID)
	if err != nil {
		return nil, err
	}

	allEvidence, err := s.loadOpportunityEvidence(ctx, c, opportunityID)
	if err != nil {
		return nil, err
	}

	guidance := opts.Guidance
	if guidance == "" {
		guidance, _, err = (&corpusReader{s: s}).ReadContributionGuidance(ctx, inv.Repo)
		if err != nil && !errors.Is(err, errRepositoryNotFound) {
			return nil, fmt.Errorf("read contribution guidance: %w", err)
		}
	}

	svc := contribution.NewService(c)
	draft, err := svc.PrepareIssue(ctx, contribution.IssueInput{
		Opportunity: opp,
		Evidence:    allEvidence,
		Guidance:    guidance,
		Repo:        inv.Repo,
		Success:     opts.Success,
	})
	if err != nil {
		return nil, err
	}

	return draftResult("issue", draft.OpportunityID, draft.Title, draft.Body, draft.RenderedAt), nil
}

// PreparePullRequest renders and stores a pull request draft for an opportunity.
func (s *Service) PreparePullRequest(ctx context.Context, opportunityID string, opts cli.PreparePROptions) (*cli.DraftResult, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}

	opp, inv, err := s.loadOpportunityAndRepo(ctx, c, opportunityID)
	if err != nil {
		return nil, err
	}

	if opts.Approach == "" {
		return nil, fmt.Errorf("%w: approach is required", contribution.ErrMissingApproach)
	}

	changes := opts.Changes
	if changes == "" && opts.WorkspaceID != "" {
		diff, err := s.workspaceDiff(ctx, opts.WorkspaceID, inv)
		if err != nil {
			return nil, fmt.Errorf("read workspace diff: %w", err)
		}
		if len(diff) > maxPreparedDiffBytes {
			return nil, fmt.Errorf("workspace diff exceeds %d bytes; provide a bounded --changes summary", maxPreparedDiffBytes)
		}
		changes = strings.TrimSpace(diff)
	}

	guidance := opts.Guidance
	if guidance == "" {
		guidance, _, err = (&corpusReader{s: s}).ReadContributionGuidance(ctx, inv.Repo)
		if err != nil && !errors.Is(err, errRepositoryNotFound) {
			return nil, fmt.Errorf("read contribution guidance: %w", err)
		}
	}

	allEvidence, err := s.loadOpportunityEvidence(ctx, c, opportunityID)
	if err != nil {
		return nil, err
	}

	svc := contribution.NewService(c)
	draft, err := svc.PreparePullRequest(ctx, contribution.PullRequestInput{
		Opportunity:   opp,
		Evidence:      allEvidence,
		Guidance:      guidance,
		Repo:          inv.Repo,
		Approach:      opts.Approach,
		Changes:       changes,
		Compatibility: opts.Compatibility,
		Limitations:   opts.Limitations,
		LinkedIssue:   opts.LinkedIssue,
	})
	if err != nil {
		return nil, err
	}

	return draftResult("pull_request", draft.OpportunityID, draft.Title, draft.Body, draft.RenderedAt), nil
}

func (s *Service) loadOpportunityAndRepo(ctx context.Context, c *corpus.Corpus, opportunityID string) (*investigation.Opportunity, *investigation.Investigation, error) {
	invSvc := investigation.NewService(c, c)
	opp, err := invSvc.GetOpportunity(ctx, opportunityID)
	if err != nil {
		return nil, nil, mapInvestigationError(err)
	}
	inv, err := invSvc.GetInvestigation(ctx, opp.InvestigationID)
	if err != nil {
		return nil, nil, mapInvestigationError(err)
	}
	return opp, inv, nil
}

func (s *Service) loadOpportunityEvidence(ctx context.Context, c *corpus.Corpus, opportunityID string) ([]*evidence.Evidence, error) {
	evSvc := evidence.NewService(c, evidence.NewExecRunner())
	items, err := evSvc.ListEvidence(ctx, evidence.EvidenceFilter{OpportunityID: opportunityID})
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.ValidationRunID == "" {
			continue
		}
		run, err := c.GetValidationRun(ctx, item.ValidationRunID)
		if err != nil {
			return nil, fmt.Errorf("read validation run %q for evidence %q: %w", item.ValidationRunID, item.ID, err)
		}
		for _, observation := range run.Observations {
			if observation.Status != evidence.ObservationMatched {
				continue
			}
			item.Description += fmt.Sprintf(" Matched observation %q", observation.Name)
			if observation.Excerpt != "" {
				item.Description += ": " + observation.Excerpt
			}
			item.Description += "."
		}
	}
	return items, nil
}

func (s *Service) workspaceDiff(ctx context.Context, workspaceID string, inv *investigation.Investigation) (string, error) {
	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return "", err
	}
	ws, err := c.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	if inv == nil || ws.InvestigationID != inv.ID ||
		!strings.EqualFold(ws.RepoOwner, inv.Repo.Owner) ||
		!strings.EqualFold(ws.RepoName, inv.Repo.Repo) {
		return "", errors.New("workspace does not belong to the opportunity investigation and repository")
	}
	mgr, err := s.workspaceManager(ctx)
	if err != nil {
		return "", err
	}
	hasUntracked, err := mgr.HasUntrackedByPath(ctx, ws.Path)
	if err != nil {
		return "", err
	}
	if hasUntracked {
		return "", errors.New("workspace has untracked files; stage them or provide an explicit --changes summary")
	}
	return mgr.DiffByPath(ctx, ws.Path, ws.BaseSHA)
}

// PrepareReviewReportInput scopes a review report to an opportunity and/or workspace.
type PrepareReviewReportInput struct {
	OpportunityID string
	WorkspaceID   string
}

// EvidenceSummary counts evidence by relation.
type EvidenceSummary struct {
	Supporting    int `json:"supporting"`
	Contradicting int `json:"contradicting"`
	Inconclusive  int `json:"inconclusive"`
	Stale         int `json:"stale"`
	Invalid       int `json:"invalid"`
	Total         int `json:"total"`
}

// ReviewReport is a read-only preparation artifact with collision findings,
// complete diff metadata, and a suggested review order.
type ReviewReport struct {
	OpportunityID        string               `json:"opportunity_id,omitempty"`
	WorkspaceID          string               `json:"workspace_id,omitempty"`
	Repo                 cli.RepoRef          `json:"repo"`
	OpportunityStatus    string               `json:"opportunity_status,omitempty"`
	CollisionStatus      string               `json:"collision_status,omitempty"`
	CollisionFindings    []evidence.Evidence  `json:"collision_findings"`
	DiffMetadata         *WorkspaceDiffResult `json:"diff_metadata,omitempty"`
	EvidenceSummary      EvidenceSummary      `json:"evidence_summary"`
	SuggestedReviewOrder []ReviewStep         `json:"suggested_review_order"`
	RenderedAt           time.Time            `json:"rendered_at"`
}

// PrepareReviewReport assembles a review report for an opportunity and/or workspace.
func (s *Service) PrepareReviewReport(ctx context.Context, input PrepareReviewReportInput) (*ReviewReport, error) {
	if input.OpportunityID == "" && input.WorkspaceID == "" {
		return nil, errors.New("opportunity or workspace is required")
	}

	report := &ReviewReport{
		OpportunityID: input.OpportunityID,
		WorkspaceID:   input.WorkspaceID,
		RenderedAt:    time.Now().UTC(),
	}

	var inv *investigation.Investigation
	var opp *investigation.Opportunity
	var opportunityEvidence []*evidence.Evidence
	if input.OpportunityID != "" {
		var err error
		opp, inv, err = s.loadOpportunityAndInvestigation(ctx, input.OpportunityID)
		if err != nil {
			return nil, err
		}
		report.OpportunityStatus = string(opp.Status)
		report.CollisionStatus = string(opp.CollisionStatus)
		report.Repo = cli.RepoRef{Owner: inv.Repo.Owner, Repo: inv.Repo.Repo}

		opportunityEvidence, err = s.evidenceForOpportunity(ctx, input.OpportunityID)
		if err != nil {
			return nil, err
		}
		report.EvidenceSummary = summarizeEvidence(opportunityEvidence)

		collisions, err := s.CheckOpportunityCollisions(ctx, input.OpportunityID, defaultCollisionLimit)
		if err != nil {
			return nil, err
		}
		report.CollisionFindings = collisions.Findings
	}

	if input.WorkspaceID != "" {
		c, err := s.openReadOnlyCorpus(ctx)
		if err != nil {
			return nil, err
		}
		ws, err := c.GetWorkspace(ctx, input.WorkspaceID)
		if err != nil {
			return nil, mapWorkspaceError(err)
		}
		if inv != nil && (ws.InvestigationID != inv.ID ||
			!strings.EqualFold(ws.RepoOwner, inv.Repo.Owner) ||
			!strings.EqualFold(ws.RepoName, inv.Repo.Repo)) {
			return nil, errors.New("workspace does not belong to the opportunity investigation and repository")
		}
		diff, err := s.WorkspaceDiff(ctx, input.WorkspaceID)
		if err != nil {
			return nil, err
		}
		report.Repo = diff.Repo
		report.DiffMetadata = diff
	}

	if report.DiffMetadata != nil && len(report.DiffMetadata.ReviewOrder) > 0 {
		report.SuggestedReviewOrder = report.DiffMetadata.ReviewOrder
	} else if opp != nil {
		report.SuggestedReviewOrder = reviewOrderFromEvidence(opportunityEvidence)
	}

	return report, nil
}

func (s *Service) loadOpportunityAndInvestigation(ctx context.Context, opportunityID string) (*investigation.Opportunity, *investigation.Investigation, error) {
	invSvc, err := s.readInvestigationSvc(ctx)
	if err != nil {
		return nil, nil, err
	}
	opp, err := invSvc.GetOpportunity(ctx, opportunityID)
	if err != nil {
		return nil, nil, mapInvestigationError(err)
	}
	inv, err := invSvc.GetInvestigation(ctx, opp.InvestigationID)
	if err != nil {
		return nil, nil, mapInvestigationError(err)
	}
	return opp, inv, nil
}

func (s *Service) evidenceForOpportunity(ctx context.Context, opportunityID string) ([]*evidence.Evidence, error) {
	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return nil, err
	}
	evSvc := evidence.NewService(c, evidence.NewExecRunner())
	return evSvc.ListEvidence(ctx, evidence.EvidenceFilter{OpportunityID: opportunityID})
}

func summarizeEvidence(items []*evidence.Evidence) EvidenceSummary {
	var s EvidenceSummary
	for _, e := range items {
		if e == nil {
			continue
		}
		s.Total++
		switch e.Relation {
		case evidence.RelationSupporting:
			s.Supporting++
		case evidence.RelationContradicting:
			s.Contradicting++
		case evidence.RelationInconclusive:
			s.Inconclusive++
		case evidence.RelationStale:
			s.Stale++
		case evidence.RelationInvalid:
			s.Invalid++
		}
	}
	return s
}

func reviewOrderFromEvidence(items []*evidence.Evidence) []ReviewStep {
	steps := make([]ReviewStep, 0, len(items))
	for _, e := range items {
		if e == nil {
			continue
		}
		priority, rationale := evidenceReviewPriority(e)
		steps = append(steps, ReviewStep{
			Path:      e.Description,
			Priority:  priority,
			Rationale: rationale,
		})
	}
	sort.Slice(steps, func(i, j int) bool {
		if steps[i].Priority != steps[j].Priority {
			return steps[i].Priority < steps[j].Priority
		}
		return steps[i].Path < steps[j].Path
	})
	return steps
}

func evidenceReviewPriority(e *evidence.Evidence) (int, string) {
	switch e.Relation {
	case evidence.RelationSupporting:
		return 0, "supporting evidence first"
	case evidence.RelationContradicting:
		return 2, "contradicting evidence before approval"
	case evidence.RelationInconclusive:
		return 3, "inconclusive evidence to resolve"
	case evidence.RelationStale:
		return 4, "stale evidence to refresh"
	case evidence.RelationInvalid:
		return 5, "invalid evidence to remove"
	default:
		return 6, "other evidence"
	}
}

func diffMatchesOpportunity(diff *WorkspaceDiffResult, inv *investigation.Investigation, opp *investigation.Opportunity) bool {
	if diff == nil || inv == nil || opp == nil {
		return false
	}
	return diff.Repo.Owner == inv.Repo.Owner && diff.Repo.Repo == inv.Repo.Repo
}

func draftResult(kind, opportunityID, title, body string, renderedAt time.Time) *cli.DraftResult {
	return &cli.DraftResult{
		OpportunityID: opportunityID,
		Kind:          kind,
		Title:         title,
		Body:          body,
		RenderedAt:    formatTime(renderedAt),
	}
}
