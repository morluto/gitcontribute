package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpserver"
)

type authoredHeaderReader struct {
	listRequests     int
	prDetailRequests int
	now              time.Time
}

func (r *authoredHeaderReader) GetRepository(context.Context, string, string) (github.Repository, github.RateInfo, error) {
	return github.Repository{Owner: "owner", Name: "repo", NodeID: "R_repo", DefaultBranch: "main", UpdatedAt: r.now}, github.RateInfo{}, nil
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

	out, err := svc.syncAuthoredPullRequests(ctx, mcpserver.SyncAuthoredPullRequestsInput{State: "open", Limit: 2, MaxRequests: 20}, func(string, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if reader.listRequests != 0 || reader.prDetailRequests != 0 {
		t.Fatalf("redundant reads: issue lists=%d PR details=%d", reader.listRequests, reader.prDetailRequests)
	}
	if out["requests"] != 3 || out["planned_requests"] != 11 || out["status"] != "complete" {
		t.Fatalf("result = %+v", out)
	}
	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := c.GetRepository(ctx, "owner", "repo")
	if err != nil || repo == nil {
		t.Fatalf("repository = %+v, %v", repo, err)
	}
	threads, err := c.ListThreadsFiltered(ctx, repo.ID, "pull_request", "open", 10)
	if err != nil || len(threads) != 2 || threads[0].Number != 3 || threads[1].Number != 2 {
		t.Fatalf("threads = %+v, %v", threads, err)
	}
}

func TestAuthoredPullRequestMinimumBudgetMakesSyncProgress(t *testing.T) {
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
	minimum := syncFixedRequestCost() + 2
	out, err := svc.syncAuthoredPullRequests(ctx, mcpserver.SyncAuthoredPullRequestsInput{State: "open", Limit: 2, MaxRequests: minimum}, func(string, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	repositories, ok := out["repositories"].([]map[string]any)
	if !ok || len(repositories) != 1 || repositories[0]["status"] != "complete" || out["planned_requests"] != minimum || out["status"] != "complete" {
		t.Fatalf("minimum-budget result = %+v", out)
	}
}

func TestSyncThreadsBatchPlansBudgetBeforeNetworkAccess(t *testing.T) {
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := svc.syncThreadsBatch(context.Background(), mcpserver.SyncThreadsInput{
		Selection: "repositories", Repositories: []mcpserver.RepositoryRef{{Owner: "owner", Repo: "repo"}},
		LimitPerRepository: 100, MaxRequests: syncFixedRequestCost(),
	}, func(string, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	items, ok := out["items"].([]map[string]any)
	if !ok || len(items) != 1 || items[0]["reason"] != "request_budget_exceeded" || out["status"] != "partial" || out["requests"] != 0 {
		t.Fatalf("result = %+v", out)
	}
}

func TestNormalizeSyncBatchMaxRequestsBoundsInput(t *testing.T) {
	if got, err := normalizeSyncBatchMaxRequests(0); err != nil || got != defaultSyncBatchMaxRequests {
		t.Fatalf("default = %d, %v", got, err)
	}
	for _, value := range []int{syncFixedRequestCost() - 1, defaultSyncBatchMaxRequests + 1} {
		if _, err := normalizeSyncBatchMaxRequests(value); err == nil {
			t.Fatalf("budget %d unexpectedly accepted", value)
		}
	}
}
