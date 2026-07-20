package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
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
	FacetPRChecks         = "pr_checks"
	FacetPRReviewThreads  = "pr_review_threads"
	FacetPRMergeState     = "pr_merge_state"
	FacetPRMergeQueue     = "pr_merge_queue"
	FacetPRClosingIssues  = "pr_closing_issues"
	FacetPRFiles          = "pr_files"
	FacetIssueTimeline    = "issue_timeline"
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
	// Timeline history is intentionally opt-in because it can be much larger
	// than the default hydration set.
	allowed = append(append([]string(nil), allowed...), FacetIssueTimeline)

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
		if err := f.c.AdvanceFacet(f.ctx, f.repoID, &f.threadID, FacetIssueTimeline, sourceUpdatedAt, false, f.runID); err != nil {
			return HydratedFacet{}, err
		}
		return HydratedFacet{Facet: FacetIssueTimeline, Count: total, Pages: pages, Complete: false}, nil
	}
	collapseFacetSearchText(pageObservations)
	if err := f.c.ApplyFacetObservationSet(f.ctx, f.repoID, &f.threadID, FacetIssueTimeline, sourceUpdatedAt, pageObservations, true, f.runID); err != nil {
		return HydratedFacet{}, err
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
	_, err := f.c.SaveResolutionRecord(f.ctx, corpus.ResolutionRecord{ThreadID: f.threadID, Kind: kind, Summary: summary, RuleVersion: "resolution.v1", SourceUpdatedAt: sourceUpdatedAt, SourceObservationRefs: refs})
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
			SearchText:      issueCommentsSearchText(res.Items),
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
		if err := f.c.AdvanceFacet(f.ctx, f.repoID, &f.threadID, FacetIssueComments, sourceUpdatedAt, false, f.runID); err != nil {
			return HydratedFacet{}, err
		}
		return HydratedFacet{Facet: FacetIssueComments, Count: total, Pages: pages, Complete: false}, nil
	}
	collapseFacetSearchText(pageObservations)
	if err := f.c.ApplyFacetObservationSet(f.ctx, f.repoID, &f.threadID, FacetIssueComments, sourceUpdatedAt, pageObservations, true, f.runID); err != nil {
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
	applied, err := f.c.ApplyFacetObservationSetIfNewer(f.ctx, f.repoID, &f.threadID, FacetPRDetails, updatedAt, pages, true, f.runID)
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
	stored, err := f.c.UpsertThread(f.ctx, projection, string(payload))
	if err != nil {
		return HydratedFacet{}, fmt.Errorf("project pr details: %w", err)
	}
	*f.thread = *stored

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
			SearchText:      pullRequestReviewsSearchText(res.Items),
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
		if err := f.c.AdvanceFacet(f.ctx, f.repoID, &f.threadID, FacetPRReviews, sourceUpdatedAt, false, f.runID); err != nil {
			return HydratedFacet{}, err
		}
		return HydratedFacet{Facet: FacetPRReviews, Count: total, Pages: pages, Complete: false}, nil
	}
	collapseFacetSearchText(pageObservations)
	if err := f.c.ApplyFacetObservationSet(f.ctx, f.repoID, &f.threadID, FacetPRReviews, sourceUpdatedAt, pageObservations, true, f.runID); err != nil {
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
			SearchText:      reviewCommentsSearchText(res.Items),
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
		if err := f.c.AdvanceFacet(f.ctx, f.repoID, &f.threadID, FacetPRReviewComments, sourceUpdatedAt, false, f.runID); err != nil {
			return HydratedFacet{}, err
		}
		return HydratedFacet{Facet: FacetPRReviewComments, Count: total, Pages: pages, Complete: false}, nil
	}
	collapseFacetSearchText(pageObservations)
	if err := f.c.ApplyFacetObservationSet(f.ctx, f.repoID, &f.threadID, FacetPRReviewComments, sourceUpdatedAt, pageObservations, true, f.runID); err != nil {
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
