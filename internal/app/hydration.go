package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
)

// Thread facets that selective hydration can retrieve.
const (
	FacetIssueComments    = "issue_comments"
	FacetPRDetails        = "pr_details"
	FacetPRReviews        = "pr_reviews"
	FacetPRReviewComments = "pr_review_comments"
)

var issueFacets = []string{FacetIssueComments}
var pullRequestFacets = []string{FacetIssueComments, FacetPRDetails, FacetPRReviews, FacetPRReviewComments}

const maxHydrationPages = 100

// HydrateResult reports the outcome of hydrating a thread.
type HydrateResult struct {
	Repo     cli.RepoRef
	Number   int
	Kind     string
	Facets   []HydratedFacet
	Pages    int
	Requests int
	Message  string
}

// HydratedFacet reports coverage and counts for one hydrated facet.
type HydratedFacet struct {
	Facet    string
	Count    int
	Pages    int
	Complete bool
}

// HydrateOptions controls selective thread hydration.
type HydrateOptions struct {
	// Facets lists the facets to retrieve. An empty list hydrates all facets
	// applicable to the thread kind.
	Facets []string
	// MaxPages bounds pagination per facet. Zero defaults to 50.
	MaxPages int
}

// HydrateThread fetches the requested facets for an issue or pull request and
// stores immutable facet observations. It is explicit, bounded, paginated,
// cancellation-aware, and records independent facet coverage plus run
// completion/failure statistics.
func (s *Service) HydrateThread(ctx context.Context, repo cli.RepoRef, number int, opts HydrateOptions) (*HydrateResult, error) {
	ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Repo}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	if number <= 0 {
		return nil, errors.New("thread number must be positive")
	}

	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	reader, err := s.githubReader()
	if err != nil {
		return nil, err
	}

	run, err := c.StartRun(ctx, "hydrate")
	if err != nil {
		return nil, err
	}
	var hydrateErr error
	defer func() {
		if hydrateErr == nil {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = c.FailRun(cleanupCtx, run.ID, hydrateErr.Error())
	}()

	repoProjection, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil {
		hydrateErr = fmt.Errorf("get repository: %w", err)
		return nil, hydrateErr
	}
	if repoProjection == nil {
		hydrateErr = fmt.Errorf("repository %s has not been synced", ref)
		return nil, hydrateErr
	}

	thread, err := c.GetThreadByNumber(ctx, repoProjection.ID, number)
	if err != nil {
		hydrateErr = fmt.Errorf("get thread: %w", err)
		return nil, hydrateErr
	}
	if thread == nil {
		hydrateErr = fmt.Errorf("thread %s#%d has not been synced", ref, number)
		return nil, hydrateErr
	}

	if err := c.RecordRunEvent(ctx, run.ID, "info", fmt.Sprintf("hydrating %s#%d (%s)", ref, number, thread.Kind)); err != nil {
		hydrateErr = err
		return nil, hydrateErr
	}

	facets, err := selectFacets(thread.Kind, opts.Facets)
	if err != nil {
		hydrateErr = err
		return nil, hydrateErr
	}

	maxPages := opts.MaxPages
	if maxPages <= 0 {
		maxPages = 50
	}
	if maxPages > maxHydrationPages {
		hydrateErr = fmt.Errorf("max pages cannot exceed %d", maxHydrationPages)
		return nil, hydrateErr
	}

	result := &HydrateResult{
		Repo:   repo,
		Number: number,
		Kind:   thread.Kind,
		Facets: make([]HydratedFacet, 0, len(facets)),
	}

	for _, facet := range facets {
		if err := ctx.Err(); err != nil {
			hydrateErr = err
			return nil, hydrateErr
		}

		f := &facetRunner{
			ctx:      ctx,
			c:        c,
			reader:   reader,
			ref:      ref,
			thread:   thread,
			repoID:   repoProjection.ID,
			threadID: thread.ID,
			runID:    run.ID,
			maxPages: maxPages,
		}

		var facetResult HydratedFacet
		switch facet {
		case FacetIssueComments:
			facetResult, err = f.hydrateIssueComments()
		case FacetPRDetails:
			facetResult, err = f.hydratePullRequestDetails()
		case FacetPRReviews:
			facetResult, err = f.hydratePullRequestReviews()
		case FacetPRReviewComments:
			facetResult, err = f.hydratePullRequestReviewComments()
		default:
			hydrateErr = fmt.Errorf("unknown facet %q", facet)
			return nil, hydrateErr
		}
		if err != nil {
			hydrateErr = fmt.Errorf("hydrate %s: %w", facet, err)
			return nil, hydrateErr
		}

		result.Facets = append(result.Facets, facetResult)
		result.Pages += facetResult.Pages
		result.Requests += facetResult.Pages
	}

	statsPayload, _ := json.Marshal(map[string]any{
		"facets":   len(result.Facets),
		"pages":    result.Pages,
		"requests": result.Requests,
	})
	if err := c.FinishRun(ctx, run.ID, string(statsPayload)); err != nil {
		hydrateErr = err
		return nil, hydrateErr
	}

	result.Message = fmt.Sprintf("hydrated %d facets for %s#%d", len(result.Facets), ref, number)
	return result, nil
}

func selectFacets(kind string, requested []string) ([]string, error) {
	var allowed []string
	switch kind {
	case corpus.ThreadKindIssue:
		allowed = issueFacets
	case corpus.ThreadKindPullRequest:
		allowed = pullRequestFacets
	default:
		return nil, fmt.Errorf("unknown thread kind %q", kind)
	}

	if len(requested) == 0 {
		return allowed, nil
	}

	allowedSet := make(map[string]struct{}, len(allowed))
	for _, f := range allowed {
		allowedSet[f] = struct{}{}
	}

	out := make([]string, 0, len(requested))
	seen := make(map[string]struct{}, len(requested))
	for _, f := range requested {
		if _, ok := allowedSet[f]; !ok {
			return nil, fmt.Errorf("facet %q is not applicable to %s threads", f, kind)
		}
		if _, ok := seen[f]; ok {
			continue
		}
		seen[f] = struct{}{}
		out = append(out, f)
	}
	return out, nil
}

type facetRunner struct {
	ctx      context.Context
	c        *corpus.Corpus
	reader   github.Reader
	ref      domain.RepoRef
	thread   *corpus.Thread
	repoID   int64
	threadID int64
	runID    int64
	maxPages int
}

func (f *facetRunner) hydrateIssueComments() (HydratedFacet, error) {
	opts := github.PageOptions{Page: 1, PerPage: 100}
	var total, pages int
	var complete bool
	var pageObservations []corpus.FacetObservationInput
	sourceUpdatedAt := f.thread.SourceUpdatedAt

	for {
		if pages >= f.maxPages {
			break
		}
		if err := f.ctx.Err(); err != nil {
			return HydratedFacet{}, err
		}

		res, err := f.reader.ListIssueComments(f.ctx, f.ref.Owner, f.ref.Repo, f.thread.Number, opts)
		if err != nil {
			return HydratedFacet{}, err
		}
		pages++

		pageUpdated := latestFromIssueComments(res.Items)
		if pageUpdated.IsZero() {
			pageUpdated = f.thread.SourceUpdatedAt
		}
		if pageUpdated.After(sourceUpdatedAt) {
			sourceUpdatedAt = pageUpdated
		}
		payload, err := json.Marshal(res.Items)
		if err != nil {
			return HydratedFacet{}, fmt.Errorf("marshal issue comments: %w", err)
		}
		pageObservations = append(pageObservations, corpus.FacetObservationInput{
			SourceUpdatedAt: pageUpdated,
			Payload:         string(payload),
		})
		total += len(res.Items)

		if !res.Page.HasNext {
			complete = true
			break
		}
		opts.Page = res.Page.NextPage
	}

	if err := f.ctx.Err(); err != nil {
		return HydratedFacet{}, err
	}
	if err := f.c.ApplyFacetObservationSet(f.ctx, f.repoID, &f.threadID, FacetIssueComments, sourceUpdatedAt, pageObservations, complete, f.runID); err != nil {
		return HydratedFacet{}, err
	}

	return HydratedFacet{Facet: FacetIssueComments, Count: total, Pages: pages, Complete: complete}, nil
}

func (f *facetRunner) hydratePullRequestDetails() (HydratedFacet, error) {
	pr, _, err := f.reader.GetPullRequestDetails(f.ctx, f.ref.Owner, f.ref.Repo, f.thread.Number)
	if err != nil {
		return HydratedFacet{}, err
	}

	payload, err := json.Marshal(pr)
	if err != nil {
		return HydratedFacet{}, fmt.Errorf("marshal pr details: %w", err)
	}
	updatedAt := pr.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = f.thread.SourceUpdatedAt
	}

	pages := []corpus.FacetObservationInput{{SourceUpdatedAt: updatedAt, Payload: string(payload)}}
	if err := f.c.ApplyFacetObservationSet(f.ctx, f.repoID, &f.threadID, FacetPRDetails, updatedAt, pages, true, f.runID); err != nil {
		return HydratedFacet{}, err
	}

	return HydratedFacet{Facet: FacetPRDetails, Count: 1, Pages: 1, Complete: true}, nil
}

func (f *facetRunner) hydratePullRequestReviews() (HydratedFacet, error) {
	opts := github.PageOptions{Page: 1, PerPage: 100}
	var total, pages int
	var complete bool
	var pageObservations []corpus.FacetObservationInput
	sourceUpdatedAt := f.thread.SourceUpdatedAt

	for {
		if pages >= f.maxPages {
			break
		}
		if err := f.ctx.Err(); err != nil {
			return HydratedFacet{}, err
		}

		res, err := f.reader.ListPullRequestReviews(f.ctx, f.ref.Owner, f.ref.Repo, f.thread.Number, opts)
		if err != nil {
			return HydratedFacet{}, err
		}
		pages++

		pageUpdated := latestFromReviews(res.Items)
		if pageUpdated.IsZero() {
			pageUpdated = f.thread.SourceUpdatedAt
		}
		if pageUpdated.After(sourceUpdatedAt) {
			sourceUpdatedAt = pageUpdated
		}
		payload, err := json.Marshal(res.Items)
		if err != nil {
			return HydratedFacet{}, fmt.Errorf("marshal pr reviews: %w", err)
		}
		pageObservations = append(pageObservations, corpus.FacetObservationInput{
			SourceUpdatedAt: pageUpdated,
			Payload:         string(payload),
		})
		total += len(res.Items)

		if !res.Page.HasNext {
			complete = true
			break
		}
		opts.Page = res.Page.NextPage
	}

	if err := f.ctx.Err(); err != nil {
		return HydratedFacet{}, err
	}
	if err := f.c.ApplyFacetObservationSet(f.ctx, f.repoID, &f.threadID, FacetPRReviews, sourceUpdatedAt, pageObservations, complete, f.runID); err != nil {
		return HydratedFacet{}, err
	}

	return HydratedFacet{Facet: FacetPRReviews, Count: total, Pages: pages, Complete: complete}, nil
}

func (f *facetRunner) hydratePullRequestReviewComments() (HydratedFacet, error) {
	opts := github.PageOptions{Page: 1, PerPage: 100}
	var total, pages int
	var complete bool
	var pageObservations []corpus.FacetObservationInput
	sourceUpdatedAt := f.thread.SourceUpdatedAt

	for {
		if pages >= f.maxPages {
			break
		}
		if err := f.ctx.Err(); err != nil {
			return HydratedFacet{}, err
		}

		res, err := f.reader.ListPullRequestComments(f.ctx, f.ref.Owner, f.ref.Repo, f.thread.Number, opts)
		if err != nil {
			return HydratedFacet{}, err
		}
		pages++

		pageUpdated := latestFromReviewComments(res.Items)
		if pageUpdated.IsZero() {
			pageUpdated = f.thread.SourceUpdatedAt
		}
		if pageUpdated.After(sourceUpdatedAt) {
			sourceUpdatedAt = pageUpdated
		}
		payload, err := json.Marshal(res.Items)
		if err != nil {
			return HydratedFacet{}, fmt.Errorf("marshal pr review comments: %w", err)
		}
		pageObservations = append(pageObservations, corpus.FacetObservationInput{
			SourceUpdatedAt: pageUpdated,
			Payload:         string(payload),
		})
		total += len(res.Items)

		if !res.Page.HasNext {
			complete = true
			break
		}
		opts.Page = res.Page.NextPage
	}

	if err := f.ctx.Err(); err != nil {
		return HydratedFacet{}, err
	}
	if err := f.c.ApplyFacetObservationSet(f.ctx, f.repoID, &f.threadID, FacetPRReviewComments, sourceUpdatedAt, pageObservations, complete, f.runID); err != nil {
		return HydratedFacet{}, err
	}

	return HydratedFacet{Facet: FacetPRReviewComments, Count: total, Pages: pages, Complete: complete}, nil
}

func latestFromIssueComments(items []github.IssueComment) time.Time {
	var latest time.Time
	for _, c := range items {
		if c.UpdatedAt.After(latest) {
			latest = c.UpdatedAt
		}
	}
	return latest
}

func latestFromReviews(items []github.Review) time.Time {
	var latest time.Time
	for _, r := range items {
		if r.SubmittedAt.After(latest) {
			latest = r.SubmittedAt
		}
	}
	return latest
}

func latestFromReviewComments(items []github.ReviewComment) time.Time {
	var latest time.Time
	for _, c := range items {
		if c.UpdatedAt.After(latest) {
			latest = c.UpdatedAt
		}
	}
	return latest
}
