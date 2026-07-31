package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGetPullRequestFeedbackPreservesChannelsAndThreadState(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/repos/acme/project/pulls/7":
			writeJSON(w, map[string]any{"number": 7, "updated_at": "2026-07-30T10:00:00Z", "head": map[string]any{"sha": "head-7"}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/repos/acme/project/issues/7/comments":
			writeJSON(w, []any{map[string]any{"id": 11, "body": "top", "user": map[string]any{"login": "alice"}, "created_at": "2026-07-30T10:01:00Z", "updated_at": "2026-07-30T10:01:00Z"}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/repos/acme/project/pulls/7/reviews":
			writeJSON(w, []any{map[string]any{"id": 12, "state": "CHANGES_REQUESTED", "body": "review", "commit_id": "head-7", "user": map[string]any{"login": "bob"}, "submitted_at": "2026-07-30T10:02:00Z"}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/repos/acme/project/pulls/7/comments":
			writeJSON(w, []any{map[string]any{"id": 13, "node_id": "C13", "body": "inline", "path": "main.go", "line": 8, "commit_id": "head-7", "in_reply_to_id": 10, "user": map[string]any{"login": "carol"}, "created_at": "2026-07-30T10:03:00Z", "updated_at": "2026-07-30T10:03:00Z"}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v3/graphql":
			comments := map[string]any{"totalCount": 1, "nodes": []any{
				map[string]any{"id": "C13", "databaseId": 13, "body": "inline", "outdated": true, "replyTo": map[string]any{"databaseId": 10}, "author": map[string]any{"login": "carol"}, "commit": map[string]any{"oid": "head-7"}},
			}}
			threads := map[string]any{"totalCount": 2, "pageInfo": map[string]any{"hasNextPage": false}, "nodes": []any{
				map[string]any{"id": "T1", "isResolved": false, "isOutdated": true, "path": "main.go", "line": 8, "comments": comments},
				map[string]any{"id": "T2", "isResolved": true, "comments": map[string]any{"totalCount": 0, "nodes": []any{}}},
			}}
			pullRequest := map[string]any{
				"headRefOid": "head-7", "updatedAt": "2026-07-30T10:00:00Z",
				"reviewThreads": threads,
			}
			writeJSON(w, map[string]any{"data": map[string]any{"repository": map[string]any{"pullRequest": pullRequest}}})
		default:
			http.Error(w, r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer srv.Close()

	got, err := newTestClient(t, srv, nil).GetPullRequestFeedback(context.Background(), "acme", "project", 7, PullRequestFeedbackOptions{
		Channels: []string{"issue_comments", "submitted_reviews", "inline_comments", "review_threads"}, ThreadState: "unresolved", MaxItemsPerChannel: 10,
	}, NewRequestBudget(5))
	if err != nil {
		t.Fatal(err)
	}
	if requests != 5 || got.HeadSHA != "head-7" || got.Header.Number != 7 || got.Header.HeadSHA != "head-7" || got.Header.UpdatedAt.IsZero() || len(got.IssueComments) != 1 || len(got.Reviews) != 1 || len(got.InlineComments) != 1 || len(got.ReviewThreads) != 1 {
		t.Fatalf("requests=%d feedback=%+v", requests, got)
	}
	thread := got.ReviewThreads[0]
	if !thread.Outdated || thread.Resolved || thread.Comments[0].InReplyToID != 10 || !thread.Comments[0].Outdated {
		t.Fatalf("thread topology not preserved: %+v", thread)
	}
}

func TestGetPullRequestFeedbackEnforcesTotalRequestBudget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/pulls/7") {
			writeJSON(w, map[string]any{"updated_at": "2026-07-30T10:00:00Z", "head": map[string]any{"sha": "head-7"}})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/issues/7/comments") {
			writeJSON(w, []any{map[string]any{"id": 1, "body": "preserved"}})
			return
		}
		t.Fatalf("unexpected request after budget was exhausted: %s", r.URL.Path)
	}))
	defer srv.Close()
	budget := NewRequestBudget(2)
	got, err := newTestClient(t, srv, nil).GetPullRequestFeedback(context.Background(), "acme", "project", 7, PullRequestFeedbackOptions{
		Channels: []string{"issue_comments", "submitted_reviews"}, ThreadState: "all", MaxItemsPerChannel: 10,
	}, budget)
	if !errors.Is(err, ErrRequestBudgetExhausted) || budget.Completed() != 2 || len(got.IssueComments) != 1 || !got.Coverage["issue_comments"].Complete {
		t.Fatalf("err=%v completed=%d", err, budget.Completed())
	}
}

func TestGetPullRequestCINormalizesProvidersAndBoundsLogs(t *testing.T) {
	var baseURL string
	logUsedConfiguredTransport := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/repos/acme/project/pulls/7":
			writeJSON(w, map[string]any{"head": map[string]any{"sha": "head-7"}, "updated_at": "2026-07-30T10:00:00Z"})
		case "/api/v3/repos/acme/project/commits/head-7/status":
			writeJSON(w, map[string]any{"state": "failure", "statuses": []any{map[string]any{"context": "external-ci", "state": "failure", "target_url": "https://ci.example/1"}}})
		case "/api/v3/repos/acme/project/commits/head-7/check-runs":
			writeJSON(w, map[string]any{"total_count": 1, "check_runs": []any{map[string]any{"name": "lint", "status": "completed", "conclusion": "failure"}}})
		case "/api/v3/repos/acme/project/actions/runs":
			if r.URL.Query().Get("head_sha") != "head-7" {
				t.Fatalf("head_sha=%q", r.URL.Query().Get("head_sha"))
			}
			writeJSON(w, map[string]any{"total_count": 1, "workflow_runs": []any{map[string]any{"id": 21, "name": "test", "status": "completed", "conclusion": "failure", "run_attempt": 2}}})
		case "/api/v3/repos/acme/project/actions/runs/21/jobs":
			job := map[string]any{
				"id": 31, "name": "unit", "status": "completed", "conclusion": "failure",
				"steps": []any{map[string]any{"name": "go test", "conclusion": "failure"}},
			}
			writeJSON(w, map[string]any{"total_count": 1, "jobs": []any{job}})
		case "/api/v3/repos/acme/project/actions/jobs/31/logs":
			w.Header().Set("Location", baseURL+"/job-log")
			w.WriteHeader(http.StatusFound)
		case "/job-log":
			if authorization := r.Header.Get("Authorization"); authorization != "" {
				t.Fatalf("job-log download leaked authorization header %q", authorization)
			}
			fmt.Fprint(w, "0123456789")
		default:
			http.Error(w, r.URL.Path, http.StatusNotFound)
		}
	}))
	baseURL = srv.URL
	defer srv.Close()

	baseTransport := http.DefaultTransport
	client, err := NewClient(Config{
		BaseURL: srv.URL, UploadURL: srv.URL, TokenSource: StaticTokenSource("secret-token"),
		Limiter: noopLimiter{}, Retry: &RetryConfig{MaxAttempts: 1},
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/job-log" {
				logUsedConfiguredTransport = true
			}
			return baseTransport.RoundTrip(req)
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	budget := NewRequestBudget(7)
	got, err := client.GetPullRequestCI(context.Background(), "acme", "project", 7, CIFailureOptions{
		MaxRuns: 5, MaxJobsPerRun: 5, MaxLogBytes: 5, Logs: "failures_only",
	}, budget)
	if err != nil {
		t.Fatal(err)
	}
	if budget.Completed() != 7 || got.HeadSHA != "head-7" || len(got.Statuses) != 1 || len(got.CheckRuns) != 1 || len(got.Runs) != 1 {
		t.Fatalf("requests=%d ci=%+v", budget.Completed(), got)
	}
	if got.SourceUpdatedAt.IsZero() {
		t.Fatal("CI snapshot omitted the authoritative pull-request revision")
	}
	log := got.Runs[0].Jobs[0].Log
	if log == nil || log.Body != "01234" || !log.Truncated {
		t.Fatalf("log=%+v", log)
	}
	if !logUsedConfiguredTransport {
		t.Fatal("signed log download did not use the configured transport")
	}
	var persisted map[string]any
	data, _ := json.Marshal(got)
	if err := json.Unmarshal(data, &persisted); err != nil || persisted["workflow_runs"] == nil {
		t.Fatalf("persisted JSON is not resource-readable: %s (%v)", data, err)
	}
}
