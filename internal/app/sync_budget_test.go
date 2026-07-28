package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

type authoredHeaderReader struct {
	listRequests     int
	prDetailRequests int
	now              time.Time
}

func (r *authoredHeaderReader) GetRepository(context.Context, string, string) (github.Repository, github.RateInfo, error) {
	return github.Repository{Owner: "owner", Name: "repo", NodeID: "R_repo", DefaultBranch: "main", UpdatedAt: r.now}, github.RateInfo{}, nil
}

func (*authoredHeaderReader) GetRepositoryFile(_ context.Context, _, _, path string) (github.RepositoryFile, github.RateInfo, error) {
	return github.RepositoryFile{}, github.RateInfo{}, &github.NotFoundError{Resource: path}
}

func (r *authoredHeaderReader) ListIssues(context.Context, string, string, github.ListIssueOptions) (github.ListResult[github.Issue], error) {
	r.listRequests++
	return github.ListResult[github.Issue]{}, errors.New("unexpected issue list")
}

func (*authoredHeaderReader) ListIssueComments(context.Context, string, string, int, github.PageOptions) (github.ListResult[github.IssueComment], error) {
	return github.ListResult[github.IssueComment]{}, errors.New("unexpected issue comments")
}

func (r *authoredHeaderReader) GetPullRequestDetails(context.Context, string, string, int) (github.PullRequestDetails, github.RateInfo, error) {
	r.prDetailRequests++
	return github.PullRequestDetails{}, github.RateInfo{}, errors.New("unexpected pull-request details")
}

func (*authoredHeaderReader) ListPullRequestReviews(context.Context, string, string, int, github.PageOptions) (github.ListResult[github.Review], error) {
	return github.ListResult[github.Review]{}, errors.New("unexpected pull-request reviews")
}

func (*authoredHeaderReader) ListPullRequestComments(context.Context, string, string, int, github.PageOptions) (github.ListResult[github.ReviewComment], error) {
	return github.ListResult[github.ReviewComment]{}, errors.New("unexpected pull-request comments")
}

func (*authoredHeaderReader) GetAuthenticatedIdentity(context.Context) (github.Identity, github.RateInfo, error) {
	return github.Identity{Login: "contributor", ID: 1}, github.RateInfo{}, nil
}

func (r *authoredHeaderReader) SearchAuthoredPullRequests(context.Context, github.AuthoredPullRequestSearchOptions) (github.AuthoredPullRequestSearchResult, error) {
	return github.AuthoredPullRequestSearchResult{Items: []github.Issue{
		{RepositoryOwner: "owner", RepositoryName: "repo", Kind: github.ThreadKindPullRequest, Number: 2, State: "open", Title: "first", CreatedAt: r.now, UpdatedAt: r.now},
		{RepositoryOwner: "owner", RepositoryName: "repo", Kind: github.ThreadKindPullRequest, Number: 3, State: "open", Title: "second", CreatedAt: r.now, UpdatedAt: r.now},
	}}, nil
}

func TestAuthoredPullRequestSyncReusesSearchHeadersWithoutNPlusOne(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	if _, err := svc.Init(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	reader := &authoredHeaderReader{now: now}
	svc.SetGitHubReader(reader)

	out, err := svc.syncAuthoredPullRequests(ctx, mcpcontract.SyncAuthoredPullRequestsInput{State: "open", Limit: 2, MaxRequests: 20}, func(string, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if reader.listRequests != 0 || reader.prDetailRequests != 0 {
		t.Fatalf("redundant reads: issue lists=%d PR details=%d", reader.listRequests, reader.prDetailRequests)
	}
	if out.Requests != 2 || out.PlannedRequests != 2 || out.Status != "complete" {
		t.Fatalf("result = %+v", out)
	}
	if len(out.PullRequestTargets) != 2 || out.PullRequestTargets[0].Number != 2 || out.PullRequestTargets[1].Number != 3 {
		t.Fatalf("discovered status targets = %+v", out.PullRequestTargets)
	}
	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := c.GetRepository(ctx, "owner", "repo")
	if err != nil || repo == nil {
		t.Fatalf("repository = %+v, %v", repo, err)
	}
	if !repo.SourceUpdatedAt.IsZero() {
		t.Fatalf("authored search timestamp leaked into repository metadata ordering: %v", repo.SourceUpdatedAt)
	}
	if _, err := svc.RepositoryContextSync(ctx, contracts.RepoRef{Owner: "owner", Repo: "repo"}, 0); err != nil {
		t.Fatalf("repository context sync after authored discovery: %v", err)
	}
	repo, err = c.GetRepository(ctx, "owner", "repo")
	if err != nil || repo == nil || repo.ExternalID != "R_repo" || !repo.SourceUpdatedAt.Equal(now) {
		t.Fatalf("repository context did not replace authored identity: %+v, %v", repo, err)
	}
	threads, err := c.ListThreadsFiltered(ctx, repo.ID, "pull_request", "open", 10)
	if err != nil || len(threads) != 2 || threads[0].Number != 3 || threads[1].Number != 2 {
		t.Fatalf("threads = %+v, %v", threads, err)
	}
}

func TestAuthoredPullRequestMinimumBudgetMakesSyncProgress(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	if _, err := svc.Init(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	svc.SetGitHubReader(&authoredHeaderReader{now: now})
	minimum := 2
	out, err := svc.syncAuthoredPullRequests(ctx, mcpcontract.SyncAuthoredPullRequestsInput{State: "open", Limit: 2, MaxRequests: minimum}, func(string, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Repositories) != 1 || out.Repositories[0].Status != "complete" || out.PlannedRequests != minimum || out.Status != "complete" {
		t.Fatalf("minimum-budget result = %+v", out)
	}
}

func TestSyncThreadsBatchReportsMissingRepositoryWithoutNetworkAccess(t *testing.T) {
	t.Parallel()
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	out, err := svc.syncThreadsBatch(context.Background(), mcpcontract.SyncThreadsInput{
		Selection: "repositories", Repositories: []mcpcontract.RepositoryRef{{Owner: "owner", Repo: "repo"}},
		LimitPerRepository: 100, MaxRequests: 1,
	}, func(string, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	items, ok := out["items"].([]map[string]any)
	if !ok || len(items) != 1 || items[0]["reason"] != "repository_not_indexed" || out["status"] != "partial" || out["requests"] != 0 {
		t.Fatalf("result = %+v", out)
	}
}

func TestSyncThreadsBatchThreadTotalCountsRequestedThreads(t *testing.T) {
	t.Parallel()
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	out, err := svc.syncThreadsBatch(context.Background(), mcpcontract.SyncThreadsInput{
		Selection: "threads",
		Threads: []mcpcontract.ThreadRef{
			{Owner: "owner", Repo: "repo", Number: 1},
			{Owner: "owner", Repo: "repo", Number: 2},
		},
		LimitPerRepository: 100,
		MaxRequests:        1,
	}, func(string, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	items, ok := out["items"].([]map[string]any)
	if !ok || len(items) != 2 || out["total"] != 2 || out["completed"] != 0 || out["status"] != "partial" {
		t.Fatalf("thread-mode result = %+v", out)
	}
}
