package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/facets"
	"github.com/morluto/gitcontribute/internal/github"
)

// Thread facets that selective hydration can retrieve.
const (
	FacetIssueComments    = facets.IssueComments
	FacetPRDetails        = facets.PRDetails
	FacetPRReviews        = facets.PRReviews
	FacetPRReviewComments = facets.PRReviewComments
	FacetPRChecks         = facets.PRChecks
	FacetPRReviewThreads  = facets.PRReviewThreads
	FacetPRMergeState     = facets.PRMergeState
	FacetPRMergeQueue     = facets.PRMergeQueue
	FacetPRClosingIssues  = facets.PRClosingIssues
	FacetPRFiles          = facets.PRFiles
	FacetIssueTimeline    = facets.IssueTimeline
)

const maxHydrationPages = 100

// HydrateResult reports the outcome of hydrating a thread.
type HydrateResult struct {
	Repo     contracts.RepoRef
	Number   int
	Kind     string
	Facets   []HydratedFacet
	Pages    int
	Requests int
	Capped   bool
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
	// Kind selects the exact issue or pull request when a number is ambiguous.
	Kind string
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
func (s *Service) HydrateThread(ctx context.Context, repo contracts.RepoRef, number int, opts HydrateOptions) (*HydrateResult, error) {
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
		_ = corpus.RetryBusy(cleanupCtx, func(ctx context.Context) error {
			return c.FailRun(ctx, run.ID, hydrateErr.Error())
		})
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

	var thread *corpus.Thread
	if opts.Kind == "" {
		thread, err = c.GetThreadByNumber(ctx, repoProjection.ID, number)
	} else {
		if opts.Kind != corpus.ThreadKindIssue && opts.Kind != corpus.ThreadKindPullRequest {
			hydrateErr = fmt.Errorf("thread kind must be issue or pull_request")
			return nil, hydrateErr
		}
		thread, err = c.GetThread(ctx, repoProjection.ID, opts.Kind, number)
	}
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
		case FacetIssueTimeline:
			facetResult, err = f.hydrateIssueTimeline()
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
	if err := corpus.RetryBusy(ctx, func(ctx context.Context) error {
		return c.FinishRun(ctx, run.ID, string(statsPayload))
	}); err != nil {
		hydrateErr = err
		return nil, hydrateErr
	}

	result.Message = fmt.Sprintf("hydrated %d facets for %s#%d", len(result.Facets), ref, number)
	return result, nil
}

func selectFacets(kind string, requested []string) ([]string, error) {
	defaults := facets.DefaultFor(kind)
	if len(defaults) == 0 {
		return nil, fmt.Errorf("unknown thread kind %q", kind)
	}

	if len(requested) == 0 {
		return defaults, nil
	}
	allowed := facets.SelectableFor(kind)

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

func (f *facetRunner) hydrateIssueTimeline() (HydratedFacet, error) {
	reader, ok := f.reader.(github.IssueTimelineReader)
	if !ok {
		return HydratedFacet{}, errors.New("GitHub reader does not support issue timelines")
	}
	expectedSequence, err := f.facetBaseline(FacetIssueTimeline)
	if err != nil {
		return HydratedFacet{}, err
	}
	opts := github.PageOptions{Page: 1, PerPage: 100}
	var total, pages int
	var complete bool
	var pageObservations []corpus.FacetObservationInput
	sourceUpdatedAt := f.thread.SourceUpdatedAt
	var events []github.IssueTimelineEvent
	for pages < f.maxPages {
		if err := f.ctx.Err(); err != nil {
			return HydratedFacet{}, err
		}
		res, err := reader.ListIssueTimeline(f.ctx, f.ref.Owner, f.ref.Repo, f.thread.Number, opts)
		if err != nil {
			return HydratedFacet{}, err
		}
		pages++
		pageUpdatedAt := sourceUpdatedAt
		for _, event := range res.Items {
			if event.CreatedAt.After(pageUpdatedAt) {
				pageUpdatedAt = event.CreatedAt
			}
		}
		payload, err := json.Marshal(res.Items)
		if err != nil {
			return HydratedFacet{}, fmt.Errorf("marshal issue timeline: %w", err)
		}
		pageObservations = append(pageObservations, corpus.FacetObservationInput{
			SourceUpdatedAt: pageUpdatedAt,
			Payload:         string(payload),
			SearchText:      issueTimelineSearchText(res.Items),
		})
		events = append(events, res.Items...)
		total += len(res.Items)
		if pageUpdatedAt.After(sourceUpdatedAt) {
			sourceUpdatedAt = pageUpdatedAt
		}
		if !res.Page.HasNext {
			complete = true
			break
		}
		opts.Page = res.Page.NextPage
	}
	if !complete {
		if _, err := corpus.RetryBusyValue(f.ctx, func(ctx context.Context) (bool, error) {
			return f.c.AdvanceFacetCAS(ctx, f.repoID, &f.threadID, FacetIssueTimeline, sourceUpdatedAt, false, f.runID, expectedSequence)
		}); err != nil {
			return HydratedFacet{}, err
		}
		return HydratedFacet{Facet: FacetIssueTimeline, Count: total, Pages: pages, Complete: false}, nil
	}
	collapseFacetSearchText(pageObservations)
	applied, err := corpus.RetryBusyValue(f.ctx, func(ctx context.Context) (bool, error) {
		return f.c.ApplyFacetObservationSetCAS(ctx, f.repoID, &f.threadID, FacetIssueTimeline, sourceUpdatedAt, pageObservations, true, f.runID, expectedSequence)
	})
	if err != nil {
		return HydratedFacet{}, err
	}
	if !applied {
		return HydratedFacet{Facet: FacetIssueTimeline, Count: total, Pages: pages, Complete: true}, nil
	}
	coverage, err := f.c.GetCoverage(f.ctx, f.repoID, &f.threadID, FacetIssueTimeline)
	if err != nil {
		return HydratedFacet{}, err
	}
	if coverage == nil || !coverage.Complete || !coverage.SourceUpdatedAt.Equal(sourceUpdatedAt.Truncate(time.Second)) {
		// A newer stored snapshot won the stale-write comparison. Do not attach
		// this older derivation to that snapshot's observation identities.
		return HydratedFacet{Facet: FacetIssueTimeline, Count: total, Pages: pages, Complete: true}, nil
	}
	if err := f.persistTimelineResolution(events, sourceUpdatedAt); err != nil {
		return HydratedFacet{}, err
	}
	return HydratedFacet{Facet: FacetIssueTimeline, Count: total, Pages: pages, Complete: true}, nil
}

func (f *facetRunner) persistTimelineResolution(events []github.IssueTimelineEvent, sourceUpdatedAt time.Time) error {
	kind, summary := "", ""
	selectedCommit := ""
	if f.thread.StateReason == "not_planned" {
		kind, summary = "not_planned", "GitHub records this issue as closed without planned work."
	}
	for _, event := range events {
		if event.Event == "closed" && event.CommitID != "" {
			kind, summary = "fixed_by_commit", "GitHub records an explicit closing commit: "+event.CommitID
			selectedCommit = event.CommitID
		}
	}
	if kind == "" {
		return nil
	}
	var refs []corpus.ObservationRef
	if selectedCommit == "" {
		observation, err := f.c.GetThreadObservationRevision(f.ctx, f.threadID, f.thread.SourceUpdatedAt, f.thread.ObservationSequence)
		if err != nil {
			return err
		}
		refs = []corpus.ObservationRef{{Kind: "thread", ID: observation.ID}}
	} else {
		observations, _, err := f.c.ListFacetObservationsBounded(f.ctx, f.repoID, &f.threadID, FacetIssueTimeline, 100)
		if err != nil {
			return err
		}
		for _, observation := range observations {
			var page []github.IssueTimelineEvent
			if err := json.Unmarshal([]byte(observation.Payload), &page); err != nil {
				return fmt.Errorf("decode issue timeline provenance: %w", err)
			}
			for _, event := range page {
				if event.Event == "closed" && event.CommitID == selectedCommit {
					refs = append(refs, corpus.ObservationRef{Kind: "facet", ID: observation.ID})
					break
				}
			}
		}
		if len(refs) == 0 {
			return errors.New("closing commit timeline observation is unavailable")
		}
	}
	_, err := corpus.RetryBusyValue(f.ctx, func(ctx context.Context) (*corpus.ResolutionRecord, error) {
		return f.c.SaveResolutionRecord(ctx, corpus.ResolutionRecord{ThreadID: f.threadID, Kind: kind, Summary: summary, RuleVersion: "resolution.v1", SourceUpdatedAt: sourceUpdatedAt, SourceObservationRefs: refs})
	})
	return err
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

type paginatedFacetSpec[T any] struct {
	facet          string
	marshalContext string
	fetch          func(github.PageOptions) (github.ListResult[T], error)
	latest         func([]T) time.Time
	searchText     func([]T) string
}

func hydratePaginatedFacet[T any](f *facetRunner, spec paginatedFacetSpec[T]) (HydratedFacet, error) {
	expectedSequence, err := f.facetBaseline(spec.facet)
	if err != nil {
		return HydratedFacet{}, err
	}
	opts := github.PageOptions{Page: 1, PerPage: 100}
	var total, pages int
	var complete bool
	var pageObservations []corpus.FacetObservationInput
	sourceUpdatedAt := f.thread.SourceUpdatedAt
	for pages < f.maxPages {
		if err := f.ctx.Err(); err != nil {
			return HydratedFacet{}, err
		}
		res, err := spec.fetch(opts)
		if err != nil {
			return HydratedFacet{}, err
		}
		pages++
		pageUpdated := spec.latest(res.Items)
		if pageUpdated.IsZero() {
			pageUpdated = f.thread.SourceUpdatedAt
		}
		if pageUpdated.After(sourceUpdatedAt) {
			sourceUpdatedAt = pageUpdated
		}
		payload, err := json.Marshal(res.Items)
		if err != nil {
			return HydratedFacet{}, fmt.Errorf("marshal %s: %w", spec.marshalContext, err)
		}
		pageObservations = append(pageObservations, corpus.FacetObservationInput{
			SourceUpdatedAt: pageUpdated,
			Payload:         string(payload),
			SearchText:      spec.searchText(res.Items),
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
	if !complete {
		if _, err := corpus.RetryBusyValue(f.ctx, func(ctx context.Context) (bool, error) {
			return f.c.AdvanceFacetCAS(ctx, f.repoID, &f.threadID, spec.facet, sourceUpdatedAt, false, f.runID, expectedSequence)
		}); err != nil {
			return HydratedFacet{}, err
		}
		return HydratedFacet{Facet: spec.facet, Count: total, Pages: pages, Complete: false}, nil
	}
	collapseFacetSearchText(pageObservations)
	if _, err := corpus.RetryBusyValue(f.ctx, func(ctx context.Context) (bool, error) {
		return f.c.ApplyFacetObservationSetCAS(ctx, f.repoID, &f.threadID, spec.facet, sourceUpdatedAt, pageObservations, true, f.runID, expectedSequence)
	}); err != nil {
		return HydratedFacet{}, err
	}
	return HydratedFacet{Facet: spec.facet, Count: total, Pages: pages, Complete: true}, nil
}

func (f *facetRunner) facetBaseline(facet string) (int64, error) {
	coverage, err := f.c.GetCoverage(f.ctx, f.repoID, &f.threadID, facet)
	if err != nil {
		return 0, err
	}
	if coverage == nil {
		return 0, nil
	}
	return coverage.ObservationSequence, nil
}

func (f *facetRunner) hydrateIssueComments() (HydratedFacet, error) {
	return hydratePaginatedFacet(f, paginatedFacetSpec[github.IssueComment]{
		facet:          FacetIssueComments,
		marshalContext: "issue comments",
		fetch: func(opts github.PageOptions) (github.ListResult[github.IssueComment], error) {
			return f.reader.ListIssueComments(f.ctx, f.ref.Owner, f.ref.Repo, f.thread.Number, opts)
		},
		latest:     latestFromIssueComments,
		searchText: issueCommentsSearchText,
	})
}

func (f *facetRunner) hydratePullRequestDetails() (HydratedFacet, error) {
	expectedSequence, err := f.facetBaseline(FacetPRDetails)
	if err != nil {
		return HydratedFacet{}, err
	}
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
	applied, err := corpus.RetryBusyValue(f.ctx, func(ctx context.Context) (bool, error) {
		return f.c.ApplyFacetObservationSetCAS(ctx, f.repoID, &f.threadID, FacetPRDetails, updatedAt, pages, true, f.runID, expectedSequence)
	})
	if err != nil {
		return HydratedFacet{}, err
	}
	if !applied {
		return HydratedFacet{Facet: FacetPRDetails, Count: 1, Pages: 1, Complete: true}, nil
	}
	projection := *f.thread
	projection.State = pr.State
	projection.Title = pr.Title
	projection.Body = pr.Body
	projection.Draft = pr.Draft
	projection.Locked = pr.Locked
	projection.Author = pr.Author
	projection.AuthorAssociation = pr.AuthorAssociation
	projection.Labels = append([]string(nil), pr.Labels...)
	projection.Assignees = append([]string(nil), pr.Assignees...)
	projection.Milestone = pr.Milestone
	projection.Merged = pr.Merged
	projection.MergedKnown = true
	projection.SourceUpdatedAt = updatedAt
	if !pr.CreatedAt.IsZero() {
		projection.SourceCreatedAt = pr.CreatedAt
	}
	if pr.ClosedAt != nil {
		projection.ClosedAt = *pr.ClosedAt
	} else {
		projection.ClosedAt = time.Time{}
	}
	if pr.MergedAt != nil {
		projection.MergedAt = *pr.MergedAt
	} else {
		projection.MergedAt = time.Time{}
	}
	stored, err := corpus.RetryBusyValue(f.ctx, func(ctx context.Context) (*corpus.Thread, error) {
		return f.c.UpsertThread(ctx, projection, string(payload))
	})
	if err != nil {
		return HydratedFacet{}, fmt.Errorf("project pr details: %w", err)
	}
	*f.thread = *stored

	return HydratedFacet{Facet: FacetPRDetails, Count: 1, Pages: 1, Complete: true}, nil
}

func (f *facetRunner) hydratePullRequestReviews() (HydratedFacet, error) {
	return hydratePaginatedFacet(f, paginatedFacetSpec[github.Review]{
		facet:          FacetPRReviews,
		marshalContext: "pr reviews",
		fetch: func(opts github.PageOptions) (github.ListResult[github.Review], error) {
			return f.reader.ListPullRequestReviews(f.ctx, f.ref.Owner, f.ref.Repo, f.thread.Number, opts)
		},
		latest:     latestFromReviews,
		searchText: pullRequestReviewsSearchText,
	})
}

func (f *facetRunner) hydratePullRequestReviewComments() (HydratedFacet, error) {
	return hydratePaginatedFacet(f, paginatedFacetSpec[github.ReviewComment]{
		facet:          FacetPRReviewComments,
		marshalContext: "pr review comments",
		fetch: func(opts github.PageOptions) (github.ListResult[github.ReviewComment], error) {
			return f.reader.ListPullRequestComments(f.ctx, f.ref.Owner, f.ref.Repo, f.thread.Number, opts)
		},
		latest:     latestFromReviewComments,
		searchText: reviewCommentsSearchText,
	})
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

func issueCommentsSearchText(items []github.IssueComment) string {
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = appendSearchLine(lines, item.Author, item.Body)
	}
	return strings.Join(lines, "\n")
}

func pullRequestReviewsSearchText(items []github.Review) string {
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = appendSearchLine(lines, item.Author, item.State, item.Body)
	}
	return strings.Join(lines, "\n")
}

func reviewCommentsSearchText(items []github.ReviewComment) string {
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = appendSearchLine(lines, item.Author, item.Path, item.Body)
	}
	return strings.Join(lines, "\n")
}

func issueTimelineSearchText(items []github.IssueTimelineEvent) string {
	lines := make([]string, 0, len(items))
	for _, item := range items {
		fields := []string{item.Event, item.Actor, item.CommitID, item.SourceOwner, item.SourceRepository}
		if item.SourceNumber > 0 {
			fields = append(fields, strconv.Itoa(item.SourceNumber))
			if item.SourceIsPullRequest {
				fields = append(fields, "pull request")
			} else {
				fields = append(fields, "issue")
			}
		}
		lines = appendSearchLine(lines, fields...)
	}
	return strings.Join(lines, "\n")
}

func appendSearchLine(lines []string, fields ...string) []string {
	line := strings.TrimSpace(strings.Join(fields, " "))
	if line == "" {
		return lines
	}
	return append(lines, line)
}

func collapseFacetSearchText(pages []corpus.FacetObservationInput) {
	if len(pages) == 0 {
		return
	}
	texts := make([]string, 0, len(pages))
	for i := range pages {
		if pages[i].SearchText != "" {
			texts = append(texts, pages[i].SearchText)
		}
		pages[i].SearchText = ""
	}
	pages[0].SearchText = strings.Join(texts, "\n")
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
