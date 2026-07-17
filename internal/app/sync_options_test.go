package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
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
	if !reflect.DeepEqual(gotNumbers, []string{"1", "2"}) {
		t.Fatalf("exact requests = %v, want [1 2]", gotNumbers)
	}
}

func TestSyncWithOptionsRejectsUnboundedPageLimit(t *testing.T) {
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test")
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.SyncWithOptions(context.Background(), cli.RepoRef{Owner: "octocat", Repo: "test"}, SyncOptions{MaxPages: 1001})
	if err == nil {
		t.Fatal("expected max-pages validation error")
	}
}

func TestSyncWithOptionsRejectsConflictingExactFilters(t *testing.T) {
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test")
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
