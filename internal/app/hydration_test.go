package app

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/github"
)

type fakeHydrationReader struct {
	issueCommentsPages    [][]github.IssueComment
	issueCommentsCalls    int
	prDetails             github.PullRequestDetails
	prReviewsPages        [][]github.Review
	prReviewsCalls        int
	prReviewCommentsPages [][]github.ReviewComment
	prReviewCommentsCalls int
	issueTimelinePages    [][]github.IssueTimelineEvent
	issueTimelineCalls    int
	failAfterIssueCalls   int
	failWith              error
}

func (f *fakeHydrationReader) ListIssueTimeline(_ context.Context, _, _ string, _ int, opts github.PageOptions) (github.ListResult[github.IssueTimelineEvent], error) {
	idx := f.issueTimelineCalls
	if idx >= len(f.issueTimelinePages) {
		return github.ListResult[github.IssueTimelineEvent]{Page: github.PageInfo{Page: opts.Page, PerPage: opts.PerPage}}, nil
	}
	f.issueTimelineCalls++
	page := github.PageInfo{Page: opts.Page, PerPage: opts.PerPage}
	if f.issueTimelineCalls < len(f.issueTimelinePages) {
		page.HasNext, page.NextPage = true, opts.Page+1
	}
	return github.ListResult[github.IssueTimelineEvent]{Items: f.issueTimelinePages[idx], Page: page}, nil
}

func (f *fakeHydrationReader) GetRepository(ctx context.Context, owner, name string) (github.Repository, github.RateInfo, error) {
	return github.Repository{Owner: owner, Name: name, NodeID: "R_1", UpdatedAt: time.Now()}, github.RateInfo{}, nil
}

func (f *fakeHydrationReader) ListIssues(ctx context.Context, owner, name string, opts github.ListIssueOptions) (github.ListResult[github.Issue], error) {
	return github.ListResult[github.Issue]{}, nil
}

func (f *fakeHydrationReader) ListIssueComments(ctx context.Context, owner, name string, issueNumber int, opts github.PageOptions) (github.ListResult[github.IssueComment], error) {
	if f.failWith != nil && f.issueCommentsCalls >= f.failAfterIssueCalls {
		return github.ListResult[github.IssueComment]{}, f.failWith
	}
	idx := f.issueCommentsCalls
	if idx >= len(f.issueCommentsPages) {
		idx = len(f.issueCommentsPages) - 1
	}
	f.issueCommentsCalls++
	page := github.PageInfo{Page: opts.Page, PerPage: opts.PerPage}
	if f.issueCommentsCalls < len(f.issueCommentsPages) {
		page.HasNext = true
		page.NextPage = opts.Page + 1
	}
	return github.ListResult[github.IssueComment]{Items: f.issueCommentsPages[idx], Page: page}, nil
}

func (f *fakeHydrationReader) GetPullRequestDetails(ctx context.Context, owner, name string, number int) (github.PullRequestDetails, github.RateInfo, error) {
	if f.failWith != nil {
		return github.PullRequestDetails{}, github.RateInfo{}, f.failWith
	}
	return f.prDetails, github.RateInfo{}, nil
}

func (f *fakeHydrationReader) ListPullRequestReviews(ctx context.Context, owner, name string, number int, opts github.PageOptions) (github.ListResult[github.Review], error) {
	if f.failWith != nil && f.prReviewsCalls >= f.failAfterIssueCalls {
		return github.ListResult[github.Review]{}, f.failWith
	}
	idx := f.prReviewsCalls
	if idx >= len(f.prReviewsPages) {
		idx = len(f.prReviewsPages) - 1
	}
	f.prReviewsCalls++
	page := github.PageInfo{Page: opts.Page, PerPage: opts.PerPage}
	if f.prReviewsCalls < len(f.prReviewsPages) {
		page.HasNext = true
		page.NextPage = opts.Page + 1
	}
	return github.ListResult[github.Review]{Items: f.prReviewsPages[idx], Page: page}, nil
}

func (f *fakeHydrationReader) ListPullRequestComments(ctx context.Context, owner, name string, number int, opts github.PageOptions) (github.ListResult[github.ReviewComment], error) {
	if f.failWith != nil && f.prReviewCommentsCalls >= f.failAfterIssueCalls {
		return github.ListResult[github.ReviewComment]{}, f.failWith
	}
	idx := f.prReviewCommentsCalls
	if idx >= len(f.prReviewCommentsPages) {
		idx = len(f.prReviewCommentsPages) - 1
	}
	f.prReviewCommentsCalls++
	page := github.PageInfo{Page: opts.Page, PerPage: opts.PerPage}
	if f.prReviewCommentsCalls < len(f.prReviewCommentsPages) {
		page.HasNext = true
		page.NextPage = opts.Page + 1
	}
	return github.ListResult[github.ReviewComment]{Items: f.prReviewCommentsPages[idx], Page: page}, nil
}

func seedRepoAndThread(t *testing.T, svc *Service, kind string, number int) (*corpus.Repository, *corpus.Thread) {
	t.Helper()
	ctx := context.Background()
	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	repo := corpus.Repository{
		Owner:           "owner",
		Name:            "repo",
		ExternalID:      "R_1",
		SourceUpdatedAt: now,
	}
	repoProjection, err := c.UpsertRepository(ctx, repo, "{}")
	if err != nil {
		t.Fatalf("upsert repository: %v", err)
	}
	thread := corpus.Thread{
		RepositoryID:    repoProjection.ID,
		Kind:            kind,
		Number:          number,
		State:           "open",
		Title:           "test thread",
		SourceUpdatedAt: now,
	}
	threadProjection, err := c.UpsertThread(ctx, thread, "{}")
	if err != nil {
		t.Fatalf("upsert thread: %v", err)
	}
	return repoProjection, threadProjection
}

func TestHydrateIssueCommentsPaginatesAndRecordsCoverage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()

	repo, thread := seedRepoAndThread(t, svc, corpus.ThreadKindIssue, 1)
	reader := &fakeHydrationReader{
		issueCommentsPages: [][]github.IssueComment{
			{
				{ID: 1, Body: "first", UpdatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
				{ID: 2, Body: "second", UpdatedAt: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)},
			},
			{
				{ID: 3, Body: "third", UpdatedAt: time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC)},
			},
		},
	}
	svc.SetGitHubReader(reader)

	result, err := svc.HydrateThread(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, 1, HydrateOptions{Facets: []string{FacetIssueComments}})
	if err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	if len(result.Facets) != 1 {
		t.Fatalf("facets = %d, want 1", len(result.Facets))
	}
	if result.Facets[0].Count != 3 {
		t.Fatalf("count = %d, want 3", result.Facets[0].Count)
	}
	if result.Facets[0].Pages != 2 {
		t.Fatalf("pages = %d, want 2", result.Facets[0].Pages)
	}
	if !result.Facets[0].Complete {
		t.Fatal("expected complete")
	}
	if reader.issueCommentsCalls != 2 {
		t.Fatalf("issue comment calls = %d, want 2", reader.issueCommentsCalls)
	}

	c, _ := svc.openCorpus(ctx)
	obs, err := c.ListFacetObservations(ctx, repo.ID, &thread.ID, FacetIssueComments)
	if err != nil {
		t.Fatalf("list facet observations: %v", err)
	}
	if len(obs) != 2 {
		t.Fatalf("observations = %d, want 2", len(obs))
	}
	cov, err := c.GetCoverage(ctx, repo.ID, &thread.ID, FacetIssueComments)
	if err != nil {
		t.Fatalf("get coverage: %v", err)
	}
	if cov == nil || !cov.Complete {
		t.Fatal("expected complete coverage")
	}
	search, err := svc.Search(ctx, "first third", cli.SearchOptions{Kind: "issues", Limit: 10})
	if err != nil {
		t.Fatalf("search hydrated comments: %v", err)
	}
	if len(search.Matches) != 1 || search.Matches[0].Number != 1 || search.Matches[0].MatchSource != FacetIssueComments || !strings.Contains(search.Matches[0].MatchExcerpt, "first") {
		t.Fatalf("hydrated comment search = %+v", search)
	}
	if search.Matches[0].Freshness != "2024-01-04T00:00:00Z" {
		t.Fatalf("hydrated comment freshness = %q", search.Matches[0].Freshness)
	}
	if reader.issueCommentsCalls != 2 {
		t.Fatalf("offline search made GitHub reads: calls=%d", reader.issueCommentsCalls)
	}
	explanation, err := svc.ExplainMatch(ctx, "first third", search.Matches[0])
	if err != nil {
		t.Fatalf("explain hydrated comment match: %v", err)
	}
	if !slices.ContainsFunc(explanation.Reasons, func(reason string) bool { return strings.Contains(reason, "query in issue_comments") }) {
		t.Fatalf("hydrated comment explanation = %+v", explanation)
	}
}

func TestHydratePullRequestFacets(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()

	repo, thread := seedRepoAndThread(t, svc, corpus.ThreadKindPullRequest, 2)
	reader := &fakeHydrationReader{
		issueCommentsPages: [][]github.IssueComment{
			{{ID: 5, Body: "main conversation", UpdatedAt: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)}},
		},
		prDetails: github.PullRequestDetails{
			Number:    2,
			Title:     "Add feature",
			UpdatedAt: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
		},
		prReviewsPages: [][]github.Review{
			{{ID: 10, State: "APPROVED", Body: "architectural approval", SubmittedAt: time.Date(2024, 2, 2, 0, 0, 0, 0, time.UTC)}},
		},
		prReviewCommentsPages: [][]github.ReviewComment{
			{{ID: 20, Body: "nit", UpdatedAt: time.Date(2024, 2, 3, 0, 0, 0, 0, time.UTC)}},
		},
	}
	svc.SetGitHubReader(reader)

	result, err := svc.HydrateThread(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, 2, HydrateOptions{})
	if err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	if result.Kind != corpus.ThreadKindPullRequest {
		t.Fatalf("kind = %q, want pull_request", result.Kind)
	}
	if len(result.Facets) != 4 {
		t.Fatalf("facets = %d, want 4", len(result.Facets))
	}

	c, _ := svc.openCorpus(ctx)
	for _, facet := range []string{FacetIssueComments, FacetPRDetails, FacetPRReviews, FacetPRReviewComments} {
		obs, err := c.ListFacetObservations(ctx, repo.ID, &thread.ID, facet)
		if err != nil {
			t.Fatalf("list observations for %s: %v", facet, err)
		}
		if len(obs) == 0 {
			t.Fatalf("missing observations for %s", facet)
		}
		cov, err := c.GetCoverage(ctx, repo.ID, &thread.ID, facet)
		if err != nil {
			t.Fatalf("get coverage for %s: %v", facet, err)
		}
		if cov == nil || !cov.Complete {
			t.Fatalf("expected complete coverage for %s", facet)
		}
	}
	projected, err := c.GetThread(ctx, repo.ID, corpus.ThreadKindPullRequest, thread.Number)
	if err != nil || projected == nil || !projected.MergedKnown || projected.Merged {
		t.Fatalf("projected PR merge state = %+v, %v", projected, err)
	}
	for query, source := range map[string]string{"architectural approval": FacetPRReviews, "nit": FacetPRReviewComments} {
		search, err := svc.Search(ctx, query, cli.SearchOptions{Kind: "prs", Limit: 10})
		if err != nil || len(search.Matches) != 1 || search.Matches[0].MatchSource != source {
			t.Fatalf("search %q = %+v, err=%v", query, search, err)
		}
	}
}

func TestHydratePullRequestDetailsDoesNotProjectStaleSnapshot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()

	repo, thread := seedRepoAndThread(t, svc, corpus.ThreadKindPullRequest, 2)
	thread.Merged = true
	thread.MergedKnown = true
	thread.MergedAt = thread.SourceUpdatedAt
	stored, err := svc.corpus.UpsertThread(ctx, *thread, `{"Merged":true}`)
	if err != nil {
		t.Fatalf("store known projection: %v", err)
	}
	newerAt := stored.SourceUpdatedAt.Add(2 * time.Hour)
	newerPayload, err := json.Marshal(github.PullRequestDetails{Number: 2, Merged: true, UpdatedAt: newerAt})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.ApplyFacetObservationSet(ctx, repo.ID, &stored.ID, FacetPRDetails, newerAt, []corpus.FacetObservationInput{{SourceUpdatedAt: newerAt, Payload: string(newerPayload)}}, true, 0); err != nil {
		t.Fatalf("store newer detail facet: %v", err)
	}

	reader := &fakeHydrationReader{prDetails: github.PullRequestDetails{
		Number: 2, Merged: false, UpdatedAt: stored.SourceUpdatedAt.Add(time.Hour),
	}}
	svc.SetGitHubReader(reader)
	if _, err := svc.HydrateThread(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, 2, HydrateOptions{Facets: []string{FacetPRDetails}}); err != nil {
		t.Fatalf("hydrate stale details: %v", err)
	}

	projected, err := svc.corpus.GetThread(ctx, repo.ID, corpus.ThreadKindPullRequest, 2)
	if err != nil {
		t.Fatal(err)
	}
	if projected == nil || !projected.MergedKnown || !projected.Merged || !projected.MergedAt.Equal(stored.MergedAt) {
		t.Fatalf("stale detail response replaced known projection: %+v", projected)
	}
	observations, err := svc.corpus.ListFacetObservations(ctx, repo.ID, &stored.ID, FacetPRDetails)
	if err != nil || len(observations) != 1 || observations[0].Payload != string(newerPayload) {
		t.Fatalf("newer detail facet was replaced: observations=%+v err=%v", observations, err)
	}
}

func TestHydratePullRequestReviewsAtPageCapPreservesCompleteSnapshot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()
	repo, thread := seedRepoAndThread(t, svc, corpus.ThreadKindPullRequest, 2)
	at := thread.SourceUpdatedAt
	oldPayload, _ := json.Marshal([]github.Review{{ID: 1, State: "APPROVED", SubmittedAt: at}})
	if err := svc.corpus.ApplyFacetObservationSet(ctx, repo.ID, &thread.ID, FacetPRReviews, at, []corpus.FacetObservationInput{{SourceUpdatedAt: at, Payload: string(oldPayload)}}, true, 0); err != nil {
		t.Fatal(err)
	}
	reader := &fakeHydrationReader{prReviewsPages: [][]github.Review{{{ID: 2, State: "COMMENTED", SubmittedAt: at.Add(time.Minute)}}, {{ID: 3, State: "APPROVED", SubmittedAt: at.Add(2 * time.Minute)}}}}
	svc.SetGitHubReader(reader)
	result, err := svc.HydrateThread(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, 2, HydrateOptions{Facets: []string{FacetPRReviews}, MaxPages: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Facets) != 1 || result.Facets[0].Complete {
		t.Fatalf("result = %+v", result)
	}
	observations, err := svc.corpus.ListFacetObservations(ctx, repo.ID, &thread.ID, FacetPRReviews)
	if err != nil || len(observations) != 1 || observations[0].Payload != string(oldPayload) {
		t.Fatalf("preserved observations = %+v err=%v", observations, err)
	}
	coverage, err := svc.corpus.GetCoverage(ctx, repo.ID, &thread.ID, FacetPRReviews)
	if err != nil || coverage == nil || coverage.Complete {
		t.Fatalf("coverage = %+v err=%v", coverage, err)
	}
}

func TestHydrateBoundsPagination(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()

	repo, thread := seedRepoAndThread(t, svc, corpus.ThreadKindIssue, 1)
	pages := make([][]github.IssueComment, 10)
	for i := range pages {
		pages[i] = []github.IssueComment{{ID: int64(i + 1), UpdatedAt: time.Date(2024, 1, 1, 0, 0, int(i), 0, time.UTC)}}
	}
	reader := &fakeHydrationReader{issueCommentsPages: pages}
	svc.SetGitHubReader(reader)

	result, err := svc.HydrateThread(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, 1, HydrateOptions{Facets: []string{FacetIssueComments}, MaxPages: 3})
	if err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	if result.Facets[0].Pages != 3 {
		t.Fatalf("pages = %d, want 3", result.Facets[0].Pages)
	}
	if result.Facets[0].Complete {
		t.Fatal("expected incomplete due to max pages bound")
	}

	c, _ := svc.openCorpus(ctx)
	cov, err := c.GetCoverage(ctx, repo.ID, &thread.ID, FacetIssueComments)
	if err != nil {
		t.Fatalf("get coverage: %v", err)
	}
	if cov.Complete {
		t.Fatal("coverage should not be complete when bounded")
	}
}

func TestHydrateRejectsExcessivePagination(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()
	seedRepoAndThread(t, svc, corpus.ThreadKindIssue, 1)
	svc.SetGitHubReader(&fakeHydrationReader{})

	_, err := svc.HydrateThread(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, 1, HydrateOptions{MaxPages: maxHydrationPages + 1})
	if err == nil || err.Error() != "max pages cannot exceed 100" {
		t.Fatalf("expected maximum pagination error, got %v", err)
	}
}

func TestSelectFacetsDeduplicatesRequestedFacets(t *testing.T) {
	t.Parallel()
	got, err := selectFacets(corpus.ThreadKindPullRequest, []string{FacetPRDetails, FacetPRDetails, FacetPRReviews})
	if err != nil {
		t.Fatalf("select facets: %v", err)
	}
	want := []string{FacetPRDetails, FacetPRReviews}
	if !slices.Equal(got, want) {
		t.Fatalf("facets = %v, want %v", got, want)
	}
}

func TestHydrateIssueTimelinePersistsExplicitClosingCommitResolution(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()
	repo, thread := seedRepoAndThread(t, svc, corpus.ThreadKindIssue, 1)
	now := thread.SourceUpdatedAt.Add(time.Hour)
	reader := &fakeHydrationReader{issueTimelinePages: [][]github.IssueTimelineEvent{
		{{ID: 1, Event: "cross-referenced", CreatedAt: now.Add(-time.Minute), SourceNumber: 9, SourceIsPullRequest: true}},
		{{ID: 2, Event: "closed", CommitID: "abc123", CreatedAt: now}},
	}}
	svc.SetGitHubReader(reader)

	result, err := svc.HydrateThread(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, 1, HydrateOptions{Facets: []string{FacetIssueTimeline}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Facets) != 1 || !result.Facets[0].Complete || result.Facets[0].Count != 2 || result.Facets[0].Pages != 2 {
		t.Fatalf("timeline result = %+v", result)
	}
	resolution, err := svc.corpus.GetResolutionRecord(ctx, thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resolution == nil || resolution.Kind != "fixed_by_commit" || resolution.RuleVersion != "resolution.v1" || len(resolution.SourceObservationRefs) != 1 {
		t.Fatalf("resolution = %+v", resolution)
	}
	coverage, err := svc.corpus.GetCoverage(ctx, repo.ID, &thread.ID, FacetIssueTimeline)
	if err != nil || coverage == nil || !coverage.Complete {
		t.Fatalf("coverage = %+v, err = %v", coverage, err)
	}
	search, err := svc.Search(ctx, "cross-referenced abc123", cli.SearchOptions{Kind: "issues", Limit: 10})
	if err != nil || len(search.Matches) != 1 || search.Matches[0].MatchSource != FacetIssueTimeline {
		t.Fatalf("timeline search = %+v, err=%v", search, err)
	}
}

func TestHydrateCancellation(t *testing.T) {
	t.Parallel()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()
	seedRepoAndThread(t, svc, corpus.ThreadKindIssue, 1)

	reader := &fakeHydrationReader{}
	svc.SetGitHubReader(reader)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.HydrateThread(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, 1, HydrateOptions{Facets: []string{FacetIssueComments}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestHydrateRecordsRunFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()
	_, _ = seedRepoAndThread(t, svc, corpus.ThreadKindIssue, 1)

	reader := &fakeHydrationReader{
		issueCommentsPages: [][]github.IssueComment{{{ID: 1}}},
		failWith:           errors.New("injected failure"),
	}
	svc.SetGitHubReader(reader)

	_, err := svc.HydrateThread(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, 1, HydrateOptions{Facets: []string{FacetIssueComments}})
	if err == nil || err.Error() != "hydrate issue_comments: injected failure" {
		t.Fatalf("unexpected error: %v", err)
	}

	c, _ := svc.openCorpus(ctx)
	runs, err := c.ListRuns(ctx, 10)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	var found bool
	for _, r := range runs {
		if r.Kind == "hydrate" {
			found = true
			if r.Status != corpus.RunStatusFailed {
				t.Fatalf("run status = %q, want failed", r.Status)
			}
		}
	}
	if !found {
		t.Fatal("missing hydrate run")
	}
}

func TestHydrateRequiresSyncedRepositoryAndThread(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()

	_, err := svc.HydrateThread(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, 1, HydrateOptions{})
	if err == nil || err.Error() != "repository owner/repo has not been synced" {
		t.Fatalf("expected missing repo error, got %v", err)
	}

	repo := corpus.Repository{Owner: "owner", Name: "repo", ExternalID: "R_1", SourceUpdatedAt: time.Now()}
	c, _ := svc.openCorpus(ctx)
	if _, err := c.UpsertRepository(ctx, repo, "{}"); err != nil {
		t.Fatalf("seed repo: %v", err)
	}

	_, err = svc.HydrateThread(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, 1, HydrateOptions{})
	if err == nil || err.Error() != "thread owner/repo#1 has not been synced" {
		t.Fatalf("expected missing thread error, got %v", err)
	}
}

func TestHydrateRejectsInapplicableFacets(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()
	seedRepoAndThread(t, svc, corpus.ThreadKindIssue, 1)

	reader := &fakeHydrationReader{}
	svc.SetGitHubReader(reader)

	_, err := svc.HydrateThread(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, 1, HydrateOptions{Facets: []string{FacetPRDetails}})
	if err == nil || err.Error() != `facet "pr_details" is not applicable to issue threads` {
		t.Fatalf("expected facet error, got %v", err)
	}
}

func TestHydrateIssueCommentsInterruptPage2RetainsOldData(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()

	repo, thread := seedRepoAndThread(t, svc, corpus.ThreadKindIssue, 1)

	oldTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	oldReader := &fakeHydrationReader{
		issueCommentsPages: [][]github.IssueComment{
			{{ID: 1, Body: "old comment", UpdatedAt: oldTime}},
		},
	}
	svc.SetGitHubReader(oldReader)
	if _, err := svc.HydrateThread(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, 1, HydrateOptions{Facets: []string{FacetIssueComments}}); err != nil {
		t.Fatalf("seed hydrate: %v", err)
	}

	newTime := time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)
	newReader := &fakeHydrationReader{
		issueCommentsPages: [][]github.IssueComment{
			{{ID: 2, Body: "new page one", UpdatedAt: newTime}},
			{{ID: 3, Body: "new page two", UpdatedAt: newTime.Add(24 * time.Hour)}},
		},
		failAfterIssueCalls: 1,
		failWith:            errors.New("page 2 failure"),
	}
	svc.SetGitHubReader(newReader)
	_, err := svc.HydrateThread(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, 1, HydrateOptions{Facets: []string{FacetIssueComments}})
	if err == nil || err.Error() != "hydrate issue_comments: page 2 failure" {
		t.Fatalf("expected page 2 failure, got %v", err)
	}

	c, _ := svc.openCorpus(ctx)
	obs, err := c.ListFacetObservations(ctx, repo.ID, &thread.ID, FacetIssueComments)
	if err != nil {
		t.Fatalf("list facet observations: %v", err)
	}
	if len(obs) != 1 {
		t.Fatalf("observations = %d, want 1", len(obs))
	}
	var got []github.IssueComment
	if err := json.Unmarshal([]byte(obs[0].Payload), &got); err != nil {
		t.Fatalf("unmarshal old observation: %v", err)
	}
	if len(got) != 1 || got[0].ID != 1 || got[0].Body != "old comment" {
		t.Fatalf("old observation retained = %+v", got)
	}

	cov, err := c.GetCoverage(ctx, repo.ID, &thread.ID, FacetIssueComments)
	if err != nil {
		t.Fatalf("get coverage: %v", err)
	}
	if cov == nil || !cov.Complete {
		t.Fatal("expected old complete coverage to remain")
	}
	if !cov.SourceUpdatedAt.Equal(oldTime) {
		t.Fatalf("coverage source updated at = %v, want %v", cov.SourceUpdatedAt, oldTime)
	}
	oldSearch, err := svc.Search(ctx, "old comment", cli.SearchOptions{Kind: "issues", Limit: 10})
	if err != nil || len(oldSearch.Matches) != 1 {
		t.Fatalf("preserved comment search = %+v, err=%v", oldSearch, err)
	}
	partialSearch, err := svc.Search(ctx, "new page one", cli.SearchOptions{Kind: "issues", Limit: 10})
	if err != nil || len(partialSearch.Matches) != 0 {
		t.Fatalf("partial comment page became searchable: %+v, err=%v", partialSearch, err)
	}
}

func TestHydrateIssueCommentsSuccessfulReplacement(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()

	repo, thread := seedRepoAndThread(t, svc, corpus.ThreadKindIssue, 1)

	oldTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	oldReader := &fakeHydrationReader{
		issueCommentsPages: [][]github.IssueComment{
			{{ID: 1, Body: "old comment", UpdatedAt: oldTime}},
		},
	}
	svc.SetGitHubReader(oldReader)
	if _, err := svc.HydrateThread(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, 1, HydrateOptions{Facets: []string{FacetIssueComments}}); err != nil {
		t.Fatalf("first hydrate: %v", err)
	}

	newTime := time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)
	newReader := &fakeHydrationReader{
		issueCommentsPages: [][]github.IssueComment{
			{{ID: 2, Body: "new page one", UpdatedAt: newTime}},
			{{ID: 3, Body: "new page two", UpdatedAt: newTime.Add(24 * time.Hour)}},
		},
	}
	svc.SetGitHubReader(newReader)
	result, err := svc.HydrateThread(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, 1, HydrateOptions{Facets: []string{FacetIssueComments}})
	if err != nil {
		t.Fatalf("second hydrate: %v", err)
	}
	if result.Facets[0].Count != 2 {
		t.Fatalf("count = %d, want 2", result.Facets[0].Count)
	}
	if result.Facets[0].Pages != 2 {
		t.Fatalf("pages = %d, want 2", result.Facets[0].Pages)
	}
	if !result.Facets[0].Complete {
		t.Fatal("expected complete")
	}

	c, _ := svc.openCorpus(ctx)
	obs, err := c.ListFacetObservations(ctx, repo.ID, &thread.ID, FacetIssueComments)
	if err != nil {
		t.Fatalf("list facet observations: %v", err)
	}
	if len(obs) != 2 {
		t.Fatalf("observations = %d, want 2", len(obs))
	}

	var last []github.IssueComment
	if err := json.Unmarshal([]byte(obs[len(obs)-1].Payload), &last); err != nil {
		t.Fatalf("unmarshal last observation: %v", err)
	}
	if len(last) != 1 || last[0].ID != 3 {
		t.Fatalf("last observation = %+v, want new page two", last)
	}

	cov, err := c.GetCoverage(ctx, repo.ID, &thread.ID, FacetIssueComments)
	if err != nil {
		t.Fatalf("get coverage: %v", err)
	}
	if cov == nil || !cov.Complete {
		t.Fatal("expected complete coverage")
	}
	wantLatest := newTime.Add(24 * time.Hour)
	if !cov.SourceUpdatedAt.Equal(wantLatest) {
		t.Fatalf("coverage source updated at = %v, want %v", cov.SourceUpdatedAt, wantLatest)
	}
}

func TestHydrateRepositoryExactNumbersAreNotLimitedByList(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()
	seedRepoAndThread(t, svc, corpus.ThreadKindIssue, 1)
	seedRepoAndThread(t, svc, corpus.ThreadKindIssue, 2)
	svc.SetGitHubReader(&fakeHydrationReader{issueCommentsPages: [][]github.IssueComment{
		{{ID: 1, UpdatedAt: time.Now()}},
	}})

	result, err := svc.HydrateRepository(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, HydrateRepositoryOptions{Numbers: []int{2}})
	if err != nil {
		t.Fatalf("hydrate repository: %v", err)
	}
	if len(result.Facets) != 1 || result.Facets[0].Facet != FacetIssueComments {
		t.Fatalf("expected one issue_comments facet, got %+v", result.Facets)
	}
}

func TestHydrateRepositoryExactNumberMissingReturnsError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()
	seedRepoAndThread(t, svc, corpus.ThreadKindIssue, 1)
	svc.SetGitHubReader(&fakeHydrationReader{})

	_, err := svc.HydrateRepository(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, HydrateRepositoryOptions{Numbers: []int{99}})
	if err == nil || !strings.Contains(err.Error(), "has not been synced") {
		t.Fatalf("expected missing thread error, got %v", err)
	}
}

func TestHydrateRepositoryUnknownFacetErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()
	seedRepoAndThread(t, svc, corpus.ThreadKindIssue, 1)
	svc.SetGitHubReader(&fakeHydrationReader{})

	_, err := svc.HydrateRepository(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, HydrateRepositoryOptions{Facets: []string{"unknown"}})
	if err == nil || !strings.Contains(err.Error(), `unknown facet "unknown"`) {
		t.Fatalf("expected unknown facet error, got %v", err)
	}
}

func TestHydrateRepositorySkipsKnownInapplicableFacets(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()
	seedRepoAndThread(t, svc, corpus.ThreadKindIssue, 1)
	seedRepoAndThread(t, svc, corpus.ThreadKindPullRequest, 2)
	svc.SetGitHubReader(&fakeHydrationReader{
		prDetails: github.PullRequestDetails{Number: 2, Title: "Add feature", UpdatedAt: time.Now()},
	})

	result, err := svc.HydrateRepository(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, HydrateRepositoryOptions{Facets: []string{FacetPRDetails}})
	if err != nil {
		t.Fatalf("hydrate repository: %v", err)
	}
	if len(result.Facets) != 1 || result.Facets[0].Facet != FacetPRDetails {
		t.Fatalf("expected one pr_details facet, got %+v", result.Facets)
	}
}

func TestHydrateRepositoryRejectsInvalidExactNumber(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()
	seedRepoAndThread(t, svc, corpus.ThreadKindIssue, 1)
	svc.SetGitHubReader(&fakeHydrationReader{})

	_, err := svc.HydrateRepository(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, HydrateRepositoryOptions{Numbers: []int{0}})
	if err == nil || !strings.Contains(err.Error(), "must be positive") {
		t.Fatalf("expected invalid thread number error, got %v", err)
	}
}
