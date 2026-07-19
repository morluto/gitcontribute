package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/zalando/go-keyring"
)

const testOwner = "octocat"
const testRepo = "hello-world"

type noopLimiter struct{}

func (noopLimiter) WaitN(ctx context.Context, n int) error { return nil }

type countingLimiter struct {
	calls int
	err   error
}

func (l *countingLimiter) WaitN(ctx context.Context, n int) error {
	l.calls++
	return l.err
}

func newTestClient(t *testing.T, srv *httptest.Server, ts TokenSource) *Client {
	t.Helper()
	client, err := NewClient(Config{
		BaseURL:     srv.URL,
		UploadURL:   srv.URL,
		TokenSource: ts,
		Limiter:     noopLimiter{},
		Retry:       &RetryConfig{MaxAttempts: 1},
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func repoPayload(id int64, name, owner string) map[string]any {
	return map[string]any{
		"id":                id,
		"node_id":           "R_" + strconv.FormatInt(id, 10),
		"name":              name,
		"full_name":         owner + "/" + name,
		"owner":             map[string]any{"login": owner, "id": 1},
		"private":           false,
		"fork":              false,
		"archived":          false,
		"is_template":       false,
		"default_branch":    "main",
		"html_url":          "https://github.com/" + owner + "/" + name,
		"description":       "test repo",
		"stargazers_count":  42,
		"watchers_count":    7,
		"forks_count":       3,
		"open_issues_count": 5,
		"language":          "Go",
		"license":           map[string]any{"name": "MIT", "spdx_id": "MIT"},
		"topics":            []string{"go", "test"},
		"created_at":        "2020-01-01T00:00:00Z",
		"updated_at":        "2024-01-01T00:00:00Z",
		"pushed_at":         "2024-06-01T00:00:00Z",
	}
}

func setRateHeaders(h http.Header) {
	h.Set("X-Ratelimit-Limit", "5000")
	h.Set("X-Ratelimit-Remaining", "4999")
	h.Set("X-Ratelimit-Used", "1")
	h.Set("X-Ratelimit-Reset", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))
}

func TestRepositoryLookup(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v3/repos/"+testOwner+"/"+testRepo {
			http.NotFound(w, r)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		setRateHeaders(w.Header())
		writeJSON(w, repoPayload(123, testRepo, testOwner))
	}))
	defer srv.Close()

	client := newTestClient(t, srv, StaticTokenSource("test-token"))
	repo, rate, err := client.GetRepository(context.Background(), testOwner, testRepo)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}

	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization header = %q, want \"Bearer test-token\"", gotAuth)
	}

	want := Repository{
		ID:            123,
		NodeID:        "R_123",
		Owner:         testOwner,
		Name:          testRepo,
		FullName:      testOwner + "/" + testRepo,
		Description:   "test repo",
		DefaultBranch: "main",
		HTMLURL:       "https://github.com/" + testOwner + "/" + testRepo,
		Private:       false,
		Fork:          false,
		Archived:      false,
		IsTemplate:    false,
		Stars:         42,
		Watchers:      7,
		Forks:         3,
		OpenIssues:    5,
		Language:      "Go",
		License:       "MIT",
		Topics:        []string{"go", "test"},
	}
	if diff := cmp.Diff(want, repo, cmpopts.IgnoreFields(Repository{}, "CreatedAt", "UpdatedAt", "PushedAt")); diff != "" {
		t.Errorf("repository mismatch (-want +got):\n%s", diff)
	}

	if rate.Limit != 5000 || rate.Remaining != 4999 || rate.Used != 1 {
		t.Errorf("unexpected rate metadata: %+v", rate)
	}
}

func TestListIssueTimelineConvertsExplicitSourceAndPagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/repos/octocat/hello-world/issues/7/timeline" || r.URL.Query().Get("page") != "1" {
			http.NotFound(w, r)
			return
		}
		setRateHeaders(w.Header())
		w.Header().Set("Link", `<https://api.github.com/repositories/1/issues/7/timeline?page=2>; rel="next"`)
		writeJSON(w, []any{map[string]any{
			"id": 42, "event": "cross-referenced", "actor": map[string]any{"login": "alice"}, "created_at": "2024-06-01T00:00:00Z",
			"source": map[string]any{"issue": map[string]any{"number": 9, "pull_request": map[string]any{"url": "https://api.github.test/pulls/9"}, "repository": map[string]any{"name": "other", "owner": map[string]any{"login": "acme"}}}},
		}})
	}))
	defer srv.Close()
	client := newTestClient(t, srv, StaticTokenSource(""))
	result, err := client.ListIssueTimeline(context.Background(), testOwner, testRepo, 7, PageOptions{Page: 1, PerPage: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || !result.Page.HasNext {
		t.Fatalf("result = %+v", result)
	}
	event := result.Items[0]
	if event.ID != 42 || event.Actor != "alice" || event.SourceOwner != "acme" || event.SourceRepository != "other" || event.SourceNumber != 9 || !event.SourceIsPullRequest {
		t.Fatalf("event = %+v", event)
	}
}

func TestRepositorySearchPreservesCountIncompleteAndPagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/search/repositories" || r.URL.Query().Get("q") != "language:go" {
			http.NotFound(w, r)
			return
		}
		setRateHeaders(w.Header())
		w.Header().Set("Link", `<https://api.github.com/search/repositories?page=2>; rel="next"`)
		writeJSON(w, map[string]any{
			"total_count": 1200, "incomplete_results": true,
			"items": []any{repoPayload(123, testRepo, testOwner)},
		})
	}))
	defer srv.Close()
	client := newTestClient(t, srv, StaticTokenSource(""))
	result, err := client.SearchRepositories(context.Background(), RepositorySearchOptions{
		Query: "language:go", PageOptions: PageOptions{Page: 1, PerPage: 100},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1200 || !result.Incomplete || len(result.Items) != 1 || !result.Page.HasNext {
		t.Fatalf("search result = %+v", result)
	}
}

func TestAuthRedaction(t *testing.T) {
	token := "super-secret-token-12345"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Errorf("Authorization = %q, want Bearer %s", got, token)
		}
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(w, map[string]any{"message": "boom"})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, StaticTokenSource(token))
	_, _, err := client.GetRepository(context.Background(), testOwner, testRepo)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), token) {
		t.Errorf("error string leaked token: %v", err)
	}
}

func TestIssueVsPRClassification(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/repos/"+testOwner+"/"+testRepo+"/issues" {
			http.NotFound(w, r)
			return
		}
		setRateHeaders(w.Header())
		writeJSON(w, []map[string]any{
			{
				"id":      1,
				"node_id": "I_1",
				"number":  1,
				"title":   "bug",
				"state":   "open",
				"user":    map[string]any{"login": "alice"},
			},
			{
				"id":           2,
				"node_id":      "PR_2",
				"number":       2,
				"title":        "feature",
				"state":        "open",
				"user":         map[string]any{"login": "bob"},
				"pull_request": map[string]any{"url": "https://api.github.com/repos/octocat/hello-world/pulls/2", "html_url": "https://github.com/octocat/hello-world/pull/2"},
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, nil)
	res, err := client.ListIssues(context.Background(), testOwner, testRepo, ListIssueOptions{})
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(res.Items) != 2 {
		t.Fatalf("got %d issues, want 2", len(res.Items))
	}
	if res.Items[0].Kind != ThreadKindIssue {
		t.Errorf("first issue kind = %q, want issue", res.Items[0].Kind)
	}
	if res.Items[1].Kind != ThreadKindPullRequest {
		t.Errorf("second issue kind = %q, want pull_request", res.Items[1].Kind)
	}
	if res.Items[1].PullRequestURL != "https://github.com/octocat/hello-world/pull/2" {
		t.Errorf("PullRequestURL = %q", res.Items[1].PullRequestURL)
	}
}

func TestPagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/repos/"+testOwner+"/"+testRepo+"/issues" {
			http.NotFound(w, r)
			return
		}
		setRateHeaders(w.Header())
		w.Header().Set("Link", `<`+r.URL.Path+`?page=1&per_page=5>; rel="first", <`+r.URL.Path+`?page=1&per_page=5>; rel="prev", <`+r.URL.Path+`?page=3&per_page=5>; rel="next", <`+r.URL.Path+`?page=10&per_page=5>; rel="last"`)
		writeJSON(w, []map[string]any{
			{"id": 3, "number": 3, "title": "page two", "state": "open", "user": map[string]any{"login": "carol"}},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, nil)
	res, err := client.ListIssues(context.Background(), testOwner, testRepo, ListIssueOptions{PageOptions: PageOptions{Page: 2, PerPage: 5}})
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}

	want := PageInfo{
		Page:      2,
		PerPage:   5,
		PrevPage:  1,
		FirstPage: 1,
		NextPage:  3,
		LastPage:  10,
		HasPrev:   true,
		HasFirst:  true,
		HasNext:   true,
		HasLast:   true,
	}
	if diff := cmp.Diff(want, res.Page); diff != "" {
		t.Errorf("page info mismatch (-want +got):\n%s", diff)
	}
}

func TestIssueComments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/repos/"+testOwner+"/"+testRepo+"/issues/1/comments" {
			http.NotFound(w, r)
			return
		}
		setRateHeaders(w.Header())
		writeJSON(w, []map[string]any{
			{"id": 101, "node_id": "IC_101", "body": "thanks", "user": map[string]any{"login": "alice"}, "created_at": "2024-01-01T00:00:00Z", "updated_at": "2024-01-01T00:00:00Z", "html_url": "https://github.com/octocat/hello-world/issues/1#issuecomment-101"},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, nil)
	res, err := client.ListIssueComments(context.Background(), testOwner, testRepo, 1, PageOptions{})
	if err != nil {
		t.Fatalf("ListIssueComments: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("got %d comments, want 1", len(res.Items))
	}
	if res.Items[0].Body != "thanks" || res.Items[0].Author != "alice" {
		t.Errorf("unexpected comment: %+v", res.Items[0])
	}
}

func TestPullRequestDetails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/repos/"+testOwner+"/"+testRepo+"/pulls/2" {
			http.NotFound(w, r)
			return
		}
		setRateHeaders(w.Header())
		writeJSON(w, map[string]any{
			"id":            2,
			"node_id":       "PR_2",
			"number":        2,
			"state":         "open",
			"title":         "feature",
			"body":          "body",
			"draft":         false,
			"merged":        false,
			"user":          map[string]any{"login": "bob"},
			"head":          map[string]any{"ref": "feature-branch", "sha": "abc123"},
			"base":          map[string]any{"ref": "main", "sha": "def456"},
			"commits":       3,
			"additions":     10,
			"deletions":     2,
			"changed_files": 1,
			"html_url":      "https://github.com/octocat/hello-world/pull/2",
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, nil)
	pr, _, err := client.GetPullRequestDetails(context.Background(), testOwner, testRepo, 2)
	if err != nil {
		t.Fatalf("GetPullRequestDetails: %v", err)
	}
	if pr.Number != 2 || pr.HeadRef != "feature-branch" || pr.BaseRef != "main" {
		t.Errorf("unexpected PR details: %+v", pr)
	}
	if pr.HeadSHA != "abc123" || pr.BaseSHA != "def456" {
		t.Errorf("unexpected head/base SHAs: %+v", pr)
	}
}

func TestPullRequestReviews(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/repos/"+testOwner+"/"+testRepo+"/pulls/2/reviews" {
			http.NotFound(w, r)
			return
		}
		setRateHeaders(w.Header())
		writeJSON(w, []map[string]any{
			{"id": 201, "node_id": "R_201", "state": "APPROVED", "body": "lgtm", "user": map[string]any{"login": "reviewer"}, "commit_id": "abc123"},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, nil)
	res, err := client.ListPullRequestReviews(context.Background(), testOwner, testRepo, 2, PageOptions{})
	if err != nil {
		t.Fatalf("ListPullRequestReviews: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].State != "APPROVED" {
		t.Fatalf("unexpected reviews: %+v", res.Items)
	}
}

func TestPullRequestComments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/repos/"+testOwner+"/"+testRepo+"/pulls/2/comments" {
			http.NotFound(w, r)
			return
		}
		setRateHeaders(w.Header())
		writeJSON(w, []map[string]any{
			{"id": 301, "node_id": "RC_301", "body": "nit", "path": "main.go", "user": map[string]any{"login": "reviewer"}, "commit_id": "abc123", "position": 7},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, nil)
	res, err := client.ListPullRequestComments(context.Background(), testOwner, testRepo, 2, PageOptions{})
	if err != nil {
		t.Fatalf("ListPullRequestComments: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].Path != "main.go" {
		t.Fatalf("unexpected comments: %+v", res.Items)
	}
}

func TestCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for cancelled context")
	}))
	defer srv.Close()

	client := newTestClient(t, srv, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.ListIssues(ctx, testOwner, testRepo, ListIssueOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}

func TestPrimaryRateLimitError(t *testing.T) {
	reset := time.Now().Add(time.Hour).Unix()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Ratelimit-Limit", "60")
		w.Header().Set("X-Ratelimit-Remaining", "0")
		w.Header().Set("X-Ratelimit-Reset", strconv.FormatInt(reset, 10))
		w.WriteHeader(http.StatusForbidden)
		writeJSON(w, map[string]any{"message": "API rate limit exceeded"})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, nil)
	_, _, err := client.GetRepository(context.Background(), testOwner, testRepo)
	if err == nil {
		t.Fatal("expected error")
	}

	var prl *PrimaryRateLimitError
	if !errors.As(err, &prl) {
		t.Fatalf("got %T, want *PrimaryRateLimitError: %v", err, err)
	}
	if prl.Rate.Remaining != 0 || prl.Message != "API rate limit exceeded" {
		t.Errorf("unexpected primary rate limit error: %+v", prl)
	}
}

func TestSecondaryRateLimitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusForbidden)
		writeJSON(w, map[string]any{
			"message":           "You have exceeded a secondary rate limit.",
			"documentation_url": "https://docs.github.com/rest/overview/resources-in-the-rest-api#abuse-rate-limits",
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, nil)
	start := time.Now()
	_, _, err := client.GetRepository(context.Background(), testOwner, testRepo)
	if time.Since(start) > 100*time.Millisecond {
		t.Errorf("rate limit handling slept in test (%v)", time.Since(start))
	}
	if err == nil {
		t.Fatal("expected error")
	}

	var srl *SecondaryRateLimitError
	if !errors.As(err, &srl) {
		t.Fatalf("got %T, want *SecondaryRateLimitError: %v", err, err)
	}
	if srl.RetryAfter != time.Second {
		t.Errorf("RetryAfter = %v, want 1s", srl.RetryAfter)
	}
}

func TestPermanentAccessErrorsAreTyped(t *testing.T) {
	tests := []struct {
		name   string
		status int
		check  func(error) bool
	}{
		{
			name: "unauthorized", status: http.StatusUnauthorized,
			check: func(err error) bool { var target *AccessDeniedError; return errors.As(err, &target) },
		},
		{
			name: "forbidden", status: http.StatusForbidden,
			check: func(err error) bool { var target *AccessDeniedError; return errors.As(err, &target) },
		},
		{
			name: "gone", status: http.StatusGone,
			check: func(err error) bool { var target *GoneError; return errors.As(err, &target) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				writeJSON(w, map[string]any{"message": tt.name})
			}))
			defer srv.Close()
			client := newTestClient(t, srv, nil)
			_, _, err := client.GetRepository(context.Background(), testOwner, testRepo)
			if err == nil || !tt.check(err) {
				t.Fatalf("error = %T %v", err, err)
			}
		})
	}
}

func TestRateLimiterTransport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, repoPayload(1, testRepo, testOwner))
	}))
	defer srv.Close()

	lim := &countingLimiter{}
	client, err := NewClient(Config{
		BaseURL:     srv.URL,
		UploadURL:   srv.URL,
		TokenSource: nil,
		Limiter:     lim,
		Retry: &RetryConfig{
			MaxAttempts: 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = client.GetRepository(context.Background(), testOwner, testRepo)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if lim.calls != 1 {
		t.Errorf("Limiter.WaitN called %d times, want 1", lim.calls)
	}

	lim.err = errors.New("rate limiter rejected")
	_, _, err = client.GetRepository(context.Background(), testOwner, testRepo)
	if !errors.Is(err, lim.err) {
		t.Fatalf("got %v, want %v", err, lim.err)
	}
}

func TestTokenResolution(t *testing.T) {
	t.Run("explicit", func(t *testing.T) {
		src := NewTokenSource("explicit-token", "GITHUB_TOKEN", nil)
		tok, err := src.Token(context.Background())
		if err != nil || tok != "explicit-token" {
			t.Fatalf("got %q, %v", tok, err)
		}
	})

	t.Run("env", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "env-token")
		src := NewTokenSource("", "GITHUB_TOKEN", nil)
		tok, err := src.Token(context.Background())
		if err != nil || tok != "env-token" {
			t.Fatalf("got %q, %v", tok, err)
		}
	})

	t.Run("gh", func(t *testing.T) {
		runner := fakeRunner{out: "gh-token\n"}
		src := NewTokenSource("", "GITHUB_TOKEN", runner)
		tok, err := src.Token(context.Background())
		if err != nil || tok != "gh-token" {
			t.Fatalf("got %q, %v", tok, err)
		}
	})

	t.Run("missing", func(t *testing.T) {
		src := NewTokenSource("", "NOT_SET_ENV_VAR", nil)
		tok, err := src.Token(context.Background())
		if !errors.Is(err, ErrNoToken) || tok != "" {
			t.Fatalf("got %q, %v", tok, err)
		}
	})

	t.Run("required missing", func(t *testing.T) {
		src := RequireToken(StaticTokenSource(""))
		tok, err := src.Token(context.Background())
		if !errors.Is(err, ErrRequiredToken) || errors.Is(err, ErrNoToken) || tok != "" {
			t.Fatalf("got %q, %v", tok, err)
		}
	})

	t.Run("required present", func(t *testing.T) {
		src := RequireToken(StaticTokenSource("required-token"))
		tok, err := src.Token(context.Background())
		if err != nil || tok != "required-token" {
			t.Fatalf("got %q, %v", tok, err)
		}
	})

	t.Run("keyring", func(t *testing.T) {
		var service, account string
		src := &keyringTokenSource{
			account: "github.com",
			get: func(gotService, gotAccount string) (string, error) {
				service, account = gotService, gotAccount
				return " keyring-token\n", nil
			},
		}
		tok, err := src.Token(context.Background())
		if err != nil || tok != "keyring-token" {
			t.Fatalf("got %q, %v", tok, err)
		}
		if service != KeyringService || account != "github.com" {
			t.Fatalf("lookup = %q/%q, want %q/github.com", service, account, KeyringService)
		}
	})

	t.Run("keyring missing", func(t *testing.T) {
		src := &keyringTokenSource{
			account: "missing",
			get:     func(string, string) (string, error) { return "", keyring.ErrNotFound },
		}
		tok, err := src.Token(context.Background())
		if !errors.Is(err, ErrNoToken) || tok != "" {
			t.Fatalf("got %q, %v", tok, err)
		}
	})

	t.Run("keyring backend failure", func(t *testing.T) {
		backendErr := errors.New("backend unavailable")
		src := &keyringTokenSource{
			account: "github.com",
			get:     func(string, string) (string, error) { return "", backendErr },
		}
		_, err := src.Token(context.Background())
		if !errors.Is(err, backendErr) || !strings.Contains(err.Error(), "keyring") {
			t.Fatalf("got %v, want wrapped keyring backend error", err)
		}
	})

	t.Run("keyring canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		called := false
		src := &keyringTokenSource{
			account: "github.com",
			get: func(string, string) (string, error) {
				called = true
				return "token", nil
			},
		}
		_, err := src.Token(ctx)
		if !errors.Is(err, context.Canceled) || called {
			t.Fatalf("got err=%v called=%t, want canceled without lookup", err, called)
		}
	})
}

type fakeRunner struct {
	out string
	err error
}

func (f fakeRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	return f.out, f.err
}

func TestNoTokenIsAllowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected Authorization header for unauthenticated request")
		}
		writeJSON(w, repoPayload(1, testRepo, testOwner))
	}))
	defer srv.Close()

	client := newTestClient(t, srv, nil)
	_, _, err := client.GetRepository(context.Background(), testOwner, testRepo)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
}
