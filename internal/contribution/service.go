package contribution

import (
	"context"
	"fmt"
)

// Service prepares contribution drafts and persists them through a narrow Repository.
type Service struct {
	repo     Repository
	renderer *Renderer
}

// NewService returns a ContributionService backed by repo and renderer.
func NewService(repo Repository) *Service {
	return &Service{repo: repo, renderer: NewRenderer()}
}

// PrepareIssue renders an issue draft and stores it.
func (s *Service) PrepareIssue(ctx context.Context, in IssueInput) (*IssueDraft, error) {
	draft, err := s.renderer.RenderIssue(in)
	if err != nil {
		return nil, fmt.Errorf("render issue: %w", err)
	}
	if err := s.repo.SaveIssueDraft(ctx, draft); err != nil {
		return nil, fmt.Errorf("save issue draft: %w", err)
	}
	return draft, nil
}

// PreparePullRequest renders a pull request draft and stores it.
func (s *Service) PreparePullRequest(ctx context.Context, in PullRequestInput) (*PullRequestDraft, error) {
	draft, err := s.renderer.RenderPullRequest(in)
	if err != nil {
		return nil, fmt.Errorf("render pull request: %w", err)
	}
	if err := s.repo.SavePullRequestDraft(ctx, draft); err != nil {
		return nil, fmt.Errorf("save pull request draft: %w", err)
	}
	return draft, nil
}
