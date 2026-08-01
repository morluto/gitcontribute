package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestListPullRequestsUsesAllStatePaginationAndFiltersIssueMarkers(t *testing.T) {
	var pages []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v3/repos/acme/rocket/issues" {
			http.NotFound(w, r)
			return
		}
		query := r.URL.Query()
		if query.Get("state") != "all" || query.Get("sort") != "updated" || query.Get("direction") != "desc" {
			t.Fatalf("unexpected discovery query: %s", r.URL.RawQuery)
		}
		page, _ := strconv.Atoi(query.Get("page"))
		pages = append(pages, page)
		if page == 1 {
			w.Header().Set("Link", `<http://`+r.Host+r.URL.Path+`?page=2&per_page=2>; rel="next"`)
			writeJSON(w, []map[string]any{
				{"id": 1, "number": 3, "title": "issue", "state": "open", "user": map[string]any{"login": "issue-author"}},
				{"id": 2, "number": 7, "title": "open PR", "state": "open", "user": map[string]any{"login": "alice"}, "pull_request": map[string]any{"url": "https://api.github.com/repos/acme/rocket/pulls/7"}},
			})
			return
		}
		writeJSON(w, []map[string]any{
			{"id": 3, "number": 8, "title": "merged PR", "state": "closed", "user": map[string]any{"login": "bob"}, "pull_request": map[string]any{"url": "https://api.github.com/repos/acme/rocket/pulls/8"}},
		})
	}))
	defer srv.Close()

	got, err := newTestClient(t, srv, nil).ListPullRequests(context.Background(), "acme", "rocket", PullRequestListOptions{
		State: "all", Sort: "updated", Direction: "desc", PageOptions: PageOptions{Page: 1, PerPage: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 || got.Items[0].Number != 7 || got.Items[0].Kind != ThreadKindPullRequest {
		t.Fatalf("page one pull requests = %+v", got.Items)
	}
	if !got.Page.HasNext || got.Page.NextPage != 2 || len(pages) != 1 || pages[0] != 1 {
		t.Fatalf("page one pagination = %+v requests=%v", got.Page, pages)
	}

	second, err := newTestClient(t, srv, nil).ListPullRequests(context.Background(), "acme", "rocket", PullRequestListOptions{
		State: "all", Sort: "updated", Direction: "desc", PageOptions: PageOptions{Page: got.Page.NextPage, PerPage: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Items[0].Number != 8 || second.Items[0].State != "closed" || second.Page.HasNext {
		t.Fatalf("page two pull requests = %+v page=%+v", second.Items, second.Page)
	}
	if len(pages) != 2 || pages[1] != 2 {
		t.Fatalf("provider pages = %v", pages)
	}
}
