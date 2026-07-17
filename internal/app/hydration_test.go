package app

import (
	"context"
	"errors"
	"slices"
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
	failAfterIssueCalls   int
	failWith              error
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
}

func TestHydratePullRequestFacets(t *testing.T) {
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
			{{ID: 10, State: "APPROVED", SubmittedAt: time.Date(2024, 2, 2, 0, 0, 0, 0, time.UTC)}},
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
}

func TestHydrateBoundsPagination(t *testing.T) {
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
	got, err := selectFacets(corpus.ThreadKindPullRequest, []string{FacetPRDetails, FacetPRDetails, FacetPRReviews})
	if err != nil {
		t.Fatalf("select facets: %v", err)
	}
	want := []string{FacetPRDetails, FacetPRReviews}
	if !slices.Equal(got, want) {
		t.Fatalf("facets = %v, want %v", got, want)
	}
}

func TestHydrateCancellation(t *testing.T) {
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
