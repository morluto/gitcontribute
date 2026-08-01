package github

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetRepositoryFileDecodesContentAndContainsVendorTypes(t *testing.T) {
	const content = "We accept pull requests for help-wanted issues."
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		switch r.URL.Path {
		case "/api/v3/repos/" + testOwner + "/" + testRepo + "/commits/HEAD":
			setRateHeaders(w.Header())
			writeJSON(w, map[string]any{"sha": "commit-sha"})
		case "/api/v3/repos/" + testOwner + "/" + testRepo + "/contents/.github/CONTRIBUTING.md":
			if got := r.URL.Query().Get("ref"); got != "commit-sha" {
				t.Errorf("content ref = %q, want commit-sha", got)
			}
			setRateHeaders(w.Header())
			writeJSON(w, map[string]any{
				"type": "file", "path": ".github/CONTRIBUTING.md", "sha": "abc123",
				"html_url": "https://github.com/octocat/hello-world/blob/main/.github/CONTRIBUTING.md",
				"encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte(content)),
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := newTestClient(t, srv, StaticTokenSource(""))
	file, rate, err := client.GetRepositoryFile(context.Background(), testOwner, testRepo, ".github/CONTRIBUTING.md")
	if err != nil {
		t.Fatal(err)
	}
	if file.Path != ".github/CONTRIBUTING.md" || file.BlobSHA != "abc123" || file.CommitSHA != "commit-sha" || file.RequestedRef != "HEAD" || file.ResolvedRef != "commit-sha" || file.Content != content {
		t.Fatalf("file = %+v", file)
	}
	if rate.Limit != 5000 || rate.Remaining != 4999 {
		t.Fatalf("rate = %+v", rate)
	}
}

func TestGetRepositoryFileClassifiesMissingPath(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	client := newTestClient(t, srv, StaticTokenSource(""))

	_, _, err := client.GetRepositoryFile(context.Background(), testOwner, testRepo, "CONTRIBUTING.md")
	if _, ok := err.(*NotFoundError); !ok {
		t.Fatalf("error = %T %v, want *NotFoundError", err, err)
	}
}
