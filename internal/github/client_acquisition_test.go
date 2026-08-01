package github

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchThreadsPreservesProviderQueryFiltersPaginationAndRate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/search/issues" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query().Get("q")
		for _, part := range []string{"repo:" + testOwner + "/" + testRepo, "needle", "is:pr", "is:open"} {
			if !strings.Contains(q, part) {
				t.Errorf("provider query %q does not contain %q", q, part)
			}
		}
		if r.URL.Query().Get("sort") != "updated" || r.URL.Query().Get("order") != "asc" || r.URL.Query().Get("page") != "2" || r.URL.Query().Get("per_page") != "3" {
			t.Errorf("request query = %v", r.URL.Query())
		}
		setRateHeaders(w.Header())
		w.Header().Set("Link", `<https://api.github.com/search/issues?page=3>; rel="next"`)
		writeJSON(w, map[string]any{
			"total_count": 12, "incomplete_results": true,
			"items": []any{map[string]any{
				"id": 41, "node_id": "PR_41", "number": 7, "title": "needle", "state": "open",
				"repository_url": "https://api.github.com/repos/" + testOwner + "/" + testRepo,
				"html_url":       "https://github.com/" + testOwner + "/" + testRepo + "/pull/7",
				"pull_request":   map[string]any{"html_url": "https://github.com/" + testOwner + "/" + testRepo + "/pull/7"},
				"created_at":     "2026-07-01T00:00:00Z", "updated_at": "2026-07-02T00:00:00Z",
			}},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, StaticTokenSource(""))
	result, err := client.SearchThreads(context.Background(), ThreadSearchOptions{
		Owner: testOwner, Repo: testRepo, Query: "needle", Kind: ThreadKindPullRequest, State: "open", Sort: "updated", Order: "asc",
		PageOptions: PageOptions{Page: 2, PerPage: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 12 || !result.Incomplete || !result.Page.HasNext || result.Page.NextPage != 3 || len(result.Items) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if result.Items[0].Kind != ThreadKindPullRequest || result.Items[0].RepositoryOwner != testOwner || result.Items[0].RepositoryName != testRepo {
		t.Fatalf("item = %+v", result.Items[0])
	}
	if result.Rate.Limit != 5000 || result.Rate.Remaining != 4999 {
		t.Fatalf("rate = %+v", result.Rate)
	}
}

func TestRepositoryFileAtRefSeparatesCommitAndBlobSHA(t *testing.T) {
	const content = "package example\n"
	var gotRef string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/repos/" + testOwner + "/" + testRepo + "/commits/main":
			writeJSON(w, map[string]any{"sha": "commit-sha"})
		case "/api/v3/repos/" + testOwner + "/" + testRepo + "/contents/internal/example.go":
			gotRef = r.URL.Query().Get("ref")
			writeJSON(w, map[string]any{
				"type": "file", "path": "internal/example.go", "sha": "blob-sha", "html_url": "https://github.com/example",
				"encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte(content)),
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := newTestClient(t, srv, StaticTokenSource(""))
	file, _, err := client.GetRepositoryFileAtRef(context.Background(), testOwner, testRepo, "internal/example.go", "main")
	if err != nil {
		t.Fatal(err)
	}
	if gotRef != "commit-sha" {
		t.Fatalf("content ref = %q, want commit-sha", gotRef)
	}
	if file.CommitSHA != "commit-sha" || file.BlobSHA != "blob-sha" || file.RequestedRef != "main" || file.ResolvedRef != "commit-sha" || file.Content != content {
		t.Fatalf("file = %+v", file)
	}
}

func TestReadSourceFilesPreservesOrderRangesAndBoundedStatuses(t *testing.T) {
	const readme = "one\ntwo\nthree\n"
	const big = "0123456789"
	var contentRefs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/repos/" + testOwner + "/" + testRepo + "/commits/main":
			writeJSON(w, map[string]any{"sha": "commit-sha"})
		case "/api/v3/repos/" + testOwner + "/" + testRepo + "/contents/README.md":
			contentRefs = append(contentRefs, r.URL.Query().Get("ref"))
			writeJSON(w, map[string]any{"type": "file", "path": "README.md", "sha": "readme-blob", "encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte(readme))})
		case "/api/v3/repos/" + testOwner + "/" + testRepo + "/contents/missing.md":
			http.NotFound(w, r)
		case "/api/v3/repos/" + testOwner + "/" + testRepo + "/contents/big.txt":
			contentRefs = append(contentRefs, r.URL.Query().Get("ref"))
			writeJSON(w, map[string]any{"type": "file", "path": "big.txt", "sha": "big-blob", "encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte(big))})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := newTestClient(t, srv, StaticTokenSource(""))
	result, err := client.ReadSourceFiles(context.Background(), testOwner, testRepo, "main", []SourceFileRequest{
		{Path: "README.md", StartLine: 2, EndLine: 2}, {Path: "missing.md"}, {Path: "big.txt"},
	}, SourceFileReadOptions{PerFileBytes: 32, TotalBytes: 8})
	if err != nil {
		t.Fatal(err)
	}
	if result.Resolution.CommitSHA != "commit-sha" || len(result.Items) != 3 || result.TotalBytes != 4 {
		t.Fatalf("result = %+v", result)
	}
	if item := result.Items[0]; item.Status != "complete" || item.File.BlobSHA != "readme-blob" || item.File.Content != "two\n" || item.StartLine != 2 || item.EndLine != 2 || item.Bytes != 4 {
		t.Fatalf("readme item = %+v", item)
	}
	if result.Items[1].Status != "not_found" || result.Items[2].Status != "too_large" {
		t.Fatalf("bounded statuses = %+v", result.Items)
	}
	for _, ref := range contentRefs {
		if ref != "commit-sha" {
			t.Fatalf("content request ref = %q", ref)
		}
	}
}

func TestReadSourceFilesClassifiesMalformedContentAsFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/repos/" + testOwner + "/" + testRepo + "/commits/main":
			writeJSON(w, map[string]any{"sha": "commit-sha"})
		case "/api/v3/repos/" + testOwner + "/" + testRepo + "/contents/broken.txt":
			writeJSON(w, map[string]any{"type": "file", "path": "broken.txt", "sha": "blob-sha", "encoding": "base64", "content": "not base64!"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := newTestClient(t, srv, StaticTokenSource(""))
	result, err := client.ReadSourceFiles(context.Background(), testOwner, testRepo, "main", []SourceFileRequest{{Path: "broken.txt"}}, SourceFileReadOptions{PerFileBytes: 100, TotalBytes: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Status != "failed" {
		t.Fatalf("result = %+v", result)
	}
}

func TestReadSourceFilesRejectsInvalidLimitsBeforeResolvingRef(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client := newTestClient(t, srv, StaticTokenSource(""))
	_, err := client.ReadSourceFiles(context.Background(), testOwner, testRepo, "main", []SourceFileRequest{{Path: "README.md"}}, SourceFileReadOptions{PerFileBytes: 0, TotalBytes: 100})
	if err == nil || !strings.Contains(err.Error(), "byte limits must be positive") {
		t.Fatalf("error = %v", err)
	}
	if requests != 0 {
		t.Fatalf("invalid limits triggered %d GitHub requests", requests)
	}
}
