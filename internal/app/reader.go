package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/dossier"
)

var errRepositoryNotFound = errors.New("repository not found")

type corpusReader struct {
	s *Service
}

var _ dossier.Reader = (*corpusReader)(nil)

func (r *corpusReader) ReadRepository(ctx context.Context, ref domain.RepoRef) (domain.Repository, []domain.SourceRef, error) {
	c, err := r.s.openCorpus(ctx)
	if err != nil {
		return domain.Repository{}, nil, err
	}
	repo, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return domain.Repository{}, nil, fmt.Errorf("get repository: %w", err)
	}
	if repo == nil {
		return domain.Repository{}, nil, fmt.Errorf("%w: %s", errRepositoryNotFound, ref)
	}

	threads, err := c.ListThreads(ctx, repo.ID, "", 0)
	if err != nil {
		return domain.Repository{}, nil, fmt.Errorf("list threads: %w", err)
	}
	counts := countThreads(threads)

	domainRepo := corpusRepoToDomain(ref, repo)
	domainRepo.OpenIssueCount = counts.openIssues
	domainRepo.ClosedIssueCount = counts.closedIssues
	domainRepo.OpenPullRequestCount = counts.openPRs
	domainRepo.MergedPullRequestCount = counts.mergedPRs
	domainRepo.ClosedUnmergedPullRequestCount = counts.closedUnmergedPRs

	if snap, err := c.LatestCodeSnapshot(ctx, ref); err != nil {
		return domain.Repository{}, nil, fmt.Errorf("latest code snapshot: %w", err)
	} else if snap != nil {
		domainRepo.CommitSHA = snap.CommitSHA
	}

	sourceRef := domain.SourceRef{
		Source:     "github:rest",
		URL:        fmt.Sprintf("https://api.github.com/repos/%s", ref),
		ObservedAt: repo.SourceUpdatedAt,
		AsOf:       repo.SourceUpdatedAt,
	}
	return domainRepo, []domain.SourceRef{sourceRef}, nil
}

func (r *corpusReader) ReadThreads(ctx context.Context, ref domain.RepoRef, q dossier.ThreadQuery) ([]domain.Thread, []domain.SourceRef, error) {
	c, err := r.s.openCorpus(ctx)
	if err != nil {
		return nil, nil, err
	}
	repo, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return nil, nil, fmt.Errorf("get repository: %w", err)
	}
	if repo == nil {
		return nil, nil, fmt.Errorf("%w: %s", errRepositoryNotFound, ref)
	}

	kind := string(q.Kind)
	threads, err := c.ListThreads(ctx, repo.ID, kind, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("list threads: %w", err)
	}

	var out []domain.Thread
	var latest time.Time
	for _, t := range threads {
		if q.State != "" && t.State != string(q.State) {
			continue
		}
		if q.Merged != nil && t.Kind == corpus.ThreadKindPullRequest {
			if t.Merged != *q.Merged {
				continue
			}
		}
		dt := corpusThreadToDomain(ref, t)
		out = append(out, dt)
		if t.SourceUpdatedAt.After(latest) {
			latest = t.SourceUpdatedAt
		}
	}

	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}

	sourceRef := domain.SourceRef{
		Source:     "github:rest",
		URL:        fmt.Sprintf("https://api.github.com/repos/%s/issues?state=all", ref),
		ObservedAt: latest,
		AsOf:       latest,
	}
	return out, []domain.SourceRef{sourceRef}, nil
}

func (r *corpusReader) ReadCoverage(ctx context.Context, ref domain.RepoRef) (domain.Coverage, error) {
	c, err := r.s.openCorpus(ctx)
	if err != nil {
		return domain.Coverage{}, err
	}
	repo, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return domain.Coverage{}, fmt.Errorf("get repository: %w", err)
	}
	if repo == nil {
		return domain.Coverage{}, fmt.Errorf("%w: %s", errRepositoryNotFound, ref)
	}

	covs, err := c.ListCoverage(ctx, repo.ID, nil)
	if err != nil {
		return domain.Coverage{}, fmt.Errorf("list coverage: %w", err)
	}

	asOf := repo.SourceUpdatedAt
	facets := make([]domain.FacetCoverage, 0, len(covs))
	for _, cov := range covs {
		if cov.SourceUpdatedAt.After(asOf) {
			asOf = cov.SourceUpdatedAt
		}
		status := domain.Fresh
		if !cov.Complete {
			status = domain.Stale
		}
		facets = append(facets, domain.FacetCoverage{
			Facet:     cov.Facet,
			Present:   true,
			Complete:  cov.Complete,
			Freshness: domain.Freshness{Status: status, AsOf: cov.SourceUpdatedAt},
			Count:     0,
		})
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}
	return domain.Coverage{AsOf: asOf, Facets: facets}, nil
}

func (r *corpusReader) ReadContributionGuidance(ctx context.Context, ref domain.RepoRef) (string, []domain.SourceRef, error) {
	c, err := r.s.openCorpus(ctx)
	if err != nil {
		return "", nil, err
	}
	repo, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return "", nil, fmt.Errorf("get repository: %w", err)
	}
	if repo == nil {
		return "", nil, fmt.Errorf("%w: %s", errRepositoryNotFound, ref)
	}
	// Contribution guidance files are not fetched in the first vertical slice,
	// so no source reference is claimed here.
	return "", nil, nil
}

type threadCounts struct {
	openIssues        int
	closedIssues      int
	openPRs           int
	mergedPRs         int
	closedUnmergedPRs int
}

func countThreads(threads []corpus.Thread) threadCounts {
	var c threadCounts
	for _, t := range threads {
		switch t.Kind {
		case corpus.ThreadKindIssue:
			if t.State == "open" {
				c.openIssues++
			} else {
				c.closedIssues++
			}
		case corpus.ThreadKindPullRequest:
			if t.State == "open" {
				c.openPRs++
			} else if t.Merged {
				c.mergedPRs++
			} else {
				c.closedUnmergedPRs++
			}
		}
	}
	return c
}

func corpusRepoToDomain(ref domain.RepoRef, repo *corpus.Repository) domain.Repository {
	dr := domain.Repository{
		RepoRef:       ref,
		ID:            repo.ID,
		Description:   repo.Description,
		Topics:        repo.Topics,
		License:       repo.License,
		DefaultBranch: repo.DefaultBranch,
		Archived:      repo.Archived,
		Fork:          repo.Fork,
		Stars:         repo.Stars,
		Watchers:      repo.Watchers,
		Forks:         repo.Forks,
		CreatedAt:     repo.SourceCreatedAt,
		UpdatedAt:     repo.SourceUpdatedAt,
	}
	if repo.Language != "" {
		dr.Languages = []string{repo.Language}
	}
	return dr
}

func corpusThreadToDomain(ref domain.RepoRef, t corpus.Thread) domain.Thread {
	dt := domain.Thread{
		Repo:      ref,
		ID:        t.ID,
		Kind:      domain.ThreadKind(t.Kind),
		Number:    t.Number,
		Title:     t.Title,
		Body:      t.Body,
		Author:    t.Author,
		State:     domain.ThreadState(t.State),
		Labels:    t.Labels,
		CreatedAt: t.SourceCreatedAt,
		UpdatedAt: t.SourceUpdatedAt,
		ClosedAt:  t.ClosedAt,
	}
	if t.Kind == corpus.ThreadKindPullRequest {
		dt.PullRequest = &domain.PullRequestDetails{
			Merged:   t.Merged,
			MergedAt: t.MergedAt,
		}
	}
	return dt
}

func firstLanguage(languages []string) string {
	if len(languages) == 0 {
		return ""
	}
	return languages[0]
}

func coverageNames(cov domain.Coverage) []string {
	out := make([]string, 0, len(cov.Facets))
	for _, f := range cov.Facets {
		out = append(out, f.Facet)
	}
	return out
}
