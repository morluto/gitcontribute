package app

import (
	"context"
	"errors"
	"fmt"
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
	return evSvc.ListEvidence(ctx, evidence.EvidenceFilter{OpportunityID: opportunityID})
}

func (s *Service) workspaceDiff(ctx context.Context, workspaceID string, inv *investigation.Investigation) (string, error) {
	c, err := s.openCorpus(ctx)
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

func draftResult(kind, opportunityID, title, body string, renderedAt time.Time) *cli.DraftResult {
	return &cli.DraftResult{
		OpportunityID: opportunityID,
		Kind:          kind,
		Title:         title,
		Body:          body,
		RenderedAt:    formatTime(renderedAt),
	}
}
