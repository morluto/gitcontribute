package app

import (
	"context"
	"fmt"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
)

const hydrateRepoThreadLimit = 10000

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

	if err := validateRequestedFacets(opts.Facets); err != nil {
		return nil, err
	}

	result := &HydrateResult{
		Repo:   repo,
		Facets: make([]HydratedFacet, 0),
	}

	var threads []*corpus.Thread

	if len(opts.Numbers) > 0 {
		seen := make(map[int]struct{}, len(opts.Numbers))
		for _, n := range opts.Numbers {
			if n <= 0 {
				return nil, fmt.Errorf("thread number must be positive: %d", n)
			}
			if _, ok := seen[n]; ok {
				continue
			}
			seen[n] = struct{}{}
			thread, err := c.GetThreadByNumber(ctx, repoProjection.ID, n)
			if err != nil {
				return nil, fmt.Errorf("get thread %s#%d: %w", ref, n, err)
			}
			if thread == nil {
				return nil, fmt.Errorf("thread %s#%d has not been synced", ref, n)
			}
			threads = append(threads, thread)
		}
	} else {
		listed, err := c.ListThreads(ctx, repoProjection.ID, "", hydrateRepoThreadLimit)
		if err != nil {
			return nil, err
		}
		threads = make([]*corpus.Thread, len(listed))
		for i := range listed {
			threads[i] = &listed[i]
		}
		if len(threads) == hydrateRepoThreadLimit {
			total, err := c.CountThreadsFiltered(ctx, repoProjection.ID, "", "")
			if err != nil {
				return nil, err
			}
			result.Capped = total > len(threads)
		}
	}

	for _, t := range threads {
		if err := ctx.Err(); err != nil {
			return nil, err
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

	if result.Capped {
		result.Message = fmt.Sprintf("hydrated repository %s (%d requests, %d pages, capped at %d threads)", ref, result.Requests, result.Pages, hydrateRepoThreadLimit)
	} else {
		result.Message = fmt.Sprintf("hydrated repository %s (%d requests, %d pages)", ref, result.Requests, result.Pages)
	}
	return result, nil
}

func validateRequestedFacets(requested []string) error {
	if len(requested) == 0 {
		return nil
	}
	known := make(map[string]struct{}, len(issueFacets)+len(pullRequestFacets))
	for _, f := range issueFacets {
		known[f] = struct{}{}
	}
	for _, f := range pullRequestFacets {
		known[f] = struct{}{}
	}
	for _, f := range requested {
		if _, ok := known[f]; !ok {
			return fmt.Errorf("unknown facet %q", f)
		}
	}
	return nil
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
