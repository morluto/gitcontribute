package app

import (
	"context"
	"fmt"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
)

// HydrateRepositoryOptions controls repository-wide thread hydration.
type HydrateRepositoryOptions struct {
	Facets   []string
	MaxPages int
	State    string
	Numbers  []int
}

// HydrateRepository hydrates selected facets for threads in a repository.
// It is explicit, bounded, cancellation-aware, and aggregates per-thread results.
func (s *Service) HydrateRepository(ctx context.Context, repo cli.RepoRef, opts HydrateRepositoryOptions) (*HydrateResult, error) {
	ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Repo}
	if err := ref.Validate(); err != nil {
		return nil, err
	}

	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}

	repoProjection, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return nil, fmt.Errorf("get repository: %w", err)
	}
	if repoProjection == nil {
		return nil, fmt.Errorf("%w: %s", errRepositoryNotFound, ref)
	}

	maxPages := opts.MaxPages
	if maxPages <= 0 {
		maxPages = 50
	}
	if maxPages > maxHydrationPages {
		return nil, fmt.Errorf("max pages cannot exceed %d", maxHydrationPages)
	}

	threads, err := c.ListThreads(ctx, repoProjection.ID, "", 10000)
	if err != nil {
		return nil, err
	}

	wantNumbers := make(map[int]struct{}, len(opts.Numbers))
	for _, n := range opts.Numbers {
		if n > 0 {
			wantNumbers[n] = struct{}{}
		}
	}

	result := &HydrateResult{
		Repo:   repo,
		Facets: make([]HydratedFacet, 0),
	}

	for _, t := range threads {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if len(wantNumbers) > 0 {
			if _, ok := wantNumbers[t.Number]; !ok {
				continue
			}
		}
		if opts.State != "" && opts.State != "all" && t.State != opts.State {
			continue
		}

		facets := applicableFacets(t.Kind, opts.Facets)
		if len(opts.Facets) > 0 && len(facets) == 0 {
			continue
		}

		hr, err := s.HydrateThread(ctx, repo, t.Number, HydrateOptions{
			Facets:   facets,
			MaxPages: maxPages,
		})
		if err != nil {
			return nil, fmt.Errorf("hydrate %s#%d: %w", ref, t.Number, err)
		}
		result.Pages += hr.Pages
		result.Requests += hr.Requests
		result.Facets = append(result.Facets, hr.Facets...)
	}

	result.Message = fmt.Sprintf("hydrated repository %s (%d requests, %d pages)", ref, result.Requests, result.Pages)
	return result, nil
}

func applicableFacets(kind string, requested []string) []string {
	var allowed []string
	switch kind {
	case corpus.ThreadKindIssue:
		allowed = issueFacets
	case corpus.ThreadKindPullRequest:
		allowed = pullRequestFacets
	default:
		return nil
	}

	if len(requested) == 0 {
		return allowed
	}

	allowedSet := make(map[string]struct{}, len(allowed))
	for _, f := range allowed {
		allowedSet[f] = struct{}{}
	}

	out := make([]string, 0, len(requested))
	seen := make(map[string]struct{}, len(requested))
	for _, f := range requested {
		if _, ok := allowedSet[f]; !ok {
			continue
		}
		if _, ok := seen[f]; ok {
			continue
		}
		seen[f] = struct{}{}
		out = append(out, f)
	}
	return out
}
