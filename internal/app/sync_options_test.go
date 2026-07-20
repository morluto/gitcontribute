package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/config"
)

func TestSyncWithOptionsPassesStateAndSinceAndMarksPartialCoverage(t *testing.T) {
	base := &testServer{owner: "octocat", repo: "test"}
	var gotState, gotSince string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v3/repos/octocat/test/issues" {
			gotState = r.URL.Query().Get("state")
			gotSince = r.URL.Query().Get("since")
			w.Header().Set("Content-Type", "application/json")
			setAppRateHeaders(w.Header())
			_ = writeAppJSON(w, base.issuePayload()[:1])
			return
		}
		base.handler(w, r)
	}))
	defer srv.Close()

	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()
	since := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	result, err := svc.SyncWithOptions(context.Background(), cli.RepoRef{Owner: "octocat", Repo: "test"}, SyncOptions{
		State: "open", Since: since, MaxPages: 2,
	})
	if err != nil {
		t.Fatalf("SyncWithOptions: %v", err)
	}
	if result.Updated != 1 {
		t.Fatalf("updated = %d, want 1", result.Updated)
	}
	if result.Requests != syncFixedRequestCost()+1 || result.Capped {
		t.Fatalf("request accounting = %+v", result)
	}
	if gotState != "open" || gotSince != since.Format(time.RFC3339) {
		t.Fatalf("query state=%q since=%q", gotState, gotSince)
	}

	c, err := svc.openCorpus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := c.GetRepository(context.Background(), "octocat", "test")
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := c.GetCoverage(context.Background(), repo.ID, nil, "threads")
	if err != nil {
		t.Fatal(err)
	}
	if coverage == nil || coverage.Complete {
		t.Fatalf("partial sync coverage = %+v, want incomplete", coverage)
	}
}

func TestSyncWithOptionsRefreshesExactNumbersDeterministically(t *testing.T) {
	base := &testServer{owner: "octocat", repo: "test"}
	var gotNumbers []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for i, issue := range base.issuePayload() {
			path := fmt.Sprintf("/api/v3/repos/octocat/test/issues/%d", i+1)
			if r.URL.Path == path {
				gotNumbers = append(gotNumbers, fmt.Sprint(i+1))
				w.Header().Set("Content-Type", "application/json")
				setAppRateHeaders(w.Header())
				_ = writeAppJSON(w, issue)
				return
			}
		}
		if r.URL.Path == "/api/v3/repos/octocat/test/issues" {
			t.Fatal("exact refresh unexpectedly listed repository issues")
		}
		base.handler(w, r)
	}))
	defer srv.Close()

	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()
	result, err := svc.SyncWithOptions(context.Background(), cli.RepoRef{Owner: "octocat", Repo: "test"}, SyncOptions{
		Numbers: []int{2, 1, 2},
	})
	if err != nil {
		t.Fatalf("SyncWithOptions: %v", err)
	}
	if result.Updated != 2 {
		t.Fatalf("updated = %d, want 2", result.Updated)
	}
	if result.Requests != syncFixedRequestCost()+2 || result.Capped {
		t.Fatalf("request accounting = %+v", result)
	}
	if !reflect.DeepEqual(gotNumbers, []string{"1", "2"}) {
		t.Fatalf("exact requests = %v, want [1 2]", gotNumbers)
	}
}

func TestSyncWithOptionsDoesNotHydratePullRequestDetails(t *testing.T) {
	base := &testServer{owner: "octocat", repo: "test"}
	prDetailRequests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v3/repos/octocat/test/pulls/2" {
			prDetailRequests++
		}
		base.handler(w, r)
	}))
	defer srv.Close()

	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()
	result, err := svc.SyncWithOptions(context.Background(), cli.RepoRef{Owner: "octocat", Repo: "test"}, SyncOptions{MaxRequests: syncFixedRequestCost() + 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated != 2 || result.Requests != syncFixedRequestCost()+1 || prDetailRequests != 0 {
		t.Fatalf("result=%+v PR detail requests=%d", result, prDetailRequests)
	}
	c, err := svc.openCorpus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := c.GetRepository(context.Background(), "octocat", "test")
	if err != nil {
		t.Fatal(err)
	}
	pr, err := c.GetThread(context.Background(), repo.ID, "pull_request", 2)
	if err != nil || pr == nil {
		t.Fatalf("stored PR = %+v, %v", pr, err)
	}
	coverage, err := c.GetCoverage(context.Background(), repo.ID, &pr.ID, FacetPRDetails)
	if err != nil || coverage != nil {
		t.Fatalf("implicit PR details coverage = %+v, %v", coverage, err)
	}
}

func TestSyncWithOptionsPreservesPreviouslyObservedPullRequestMergeState(t *testing.T) {
	base := &testServer{owner: "octocat", repo: "test"}
	srv := httptest.NewServer(http.HandlerFunc(base.handler))
	defer srv.Close()

	ctx := context.Background()
	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()
	if _, err := svc.SyncWithOptions(ctx, cli.RepoRef{Owner: "octocat", Repo: "test"}, SyncOptions{MaxRequests: syncFixedRequestCost() + 1}); err != nil {
		t.Fatal(err)
	}
	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := c.GetRepository(ctx, "octocat", "test")
	if err != nil {
		t.Fatal(err)
	}
	pr, err := c.GetThread(ctx, repo.ID, "pull_request", 2)
	if err != nil || pr == nil {
		t.Fatalf("stored PR = %+v, %v", pr, err)
	}
	pr.Merged = true
	pr.MergedAt = pr.SourceUpdatedAt
	if _, err := c.UpsertThread(ctx, *pr, `{"source":"previous-pr-details"}`); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.SyncWithOptions(ctx, cli.RepoRef{Owner: "octocat", Repo: "test"}, SyncOptions{MaxRequests: syncFixedRequestCost() + 1}); err != nil {
		t.Fatal(err)
	}
	pr, err = c.GetThread(ctx, repo.ID, "pull_request", 2)
	if err != nil || pr == nil || !pr.Merged || pr.MergedAt.IsZero() {
		t.Fatalf("merge state after header sync = %+v, %v", pr, err)
	}
}

func TestSyncWithOptionsStopsBeforeRequestBudgetIsExceeded(t *testing.T) {
	base := &testServer{owner: "octocat", repo: "test"}
	pageTwoRequests := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v3/repos/octocat/test/issues" {
			if r.URL.Query().Get("page") == "2" {
				pageTwoRequests++
			}
			w.Header().Set("Content-Type", "application/json")
			setAppRateHeaders(w.Header())
			w.Header().Set("Link", "<"+srv.URL+"/api/v3/repos/octocat/test/issues?page=2>; rel=\"next\"")
			_ = writeAppJSON(w, base.issuePayload()[:1])
			return
		}
		base.handler(w, r)
	}))
	defer srv.Close()

	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()
	result, err := svc.SyncWithOptions(context.Background(), cli.RepoRef{Owner: "octocat", Repo: "test"}, SyncOptions{MaxPages: 5, MaxRequests: syncFixedRequestCost() + 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Requests != syncFixedRequestCost()+1 || !result.Capped || pageTwoRequests != 0 {
		t.Fatalf("result=%+v page two requests=%d", result, pageTwoRequests)
	}
}

func TestSyncWithOptionsRejectsExactSelectionOverRequestBudgetBeforeIO(t *testing.T) {
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.SyncWithOptions(context.Background(), cli.RepoRef{Owner: "octocat", Repo: "test"}, SyncOptions{
		Numbers: []int{1, 2}, MaxRequests: syncFixedRequestCost() + 1,
	})
	if err == nil || !strings.Contains(err.Error(), "exact thread selection requires") {
		t.Fatalf("budget error = %v", err)
	}
}

func TestSyncWithOptionsRejectsUnboundedPageLimit(t *testing.T) {
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.SyncWithOptions(context.Background(), cli.RepoRef{Owner: "octocat", Repo: "test"}, SyncOptions{MaxPages: 1001})
	if err == nil {
		t.Fatal("expected max-pages validation error")
	}
}

func TestSyncWithOptionsRejectsInvalidRequestBudgets(t *testing.T) {
	for _, maxRequests := range []int{-1, syncFixedRequestCost() - 1, maxSyncRequests + 1} {
		if _, err := normalizeSyncOptions(SyncOptions{MaxRequests: maxRequests}); err == nil {
			t.Fatalf("max requests %d unexpectedly accepted", maxRequests)
		}
	}
}

func TestSyncWithOptionsRejectsConflictingExactFilters(t *testing.T) {
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.SyncWithOptions(context.Background(), cli.RepoRef{Owner: "octocat", Repo: "test"}, SyncOptions{
		State: "open", Numbers: []int{1},
	})
	if err == nil {
		t.Fatal("expected conflicting filter validation error")
	}
}

func setAppRateHeaders(h http.Header) {
	h.Set("X-Ratelimit-Limit", "5000")
	h.Set("X-Ratelimit-Remaining", "4999")
	h.Set("X-Ratelimit-Reset", fmt.Sprint(time.Now().Add(time.Hour).Unix()))
}

func writeAppJSON(w http.ResponseWriter, value any) error {
	return json.NewEncoder(w).Encode(value)
}
