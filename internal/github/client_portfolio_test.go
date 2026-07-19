package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestGetAuthenticatedIdentity(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v3/user" {
			http.NotFound(w, r)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		setRateHeaders(w.Header())
		writeJSON(w, map[string]any{
			"login":   "morluto",
			"id":      1234,
			"node_id": "U_1234",
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, StaticTokenSource("test-token"))
	identity, rate, err := client.GetAuthenticatedIdentity(context.Background())
	if err != nil {
		t.Fatalf("GetAuthenticatedIdentity: %v", err)
	}

	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-token")
	}
	want := Identity{Login: "morluto", ID: 1234, NodeID: "U_1234"}
	if diff := cmp.Diff(want, identity); diff != "" {
		t.Errorf("identity mismatch (-want +got):\n%s", diff)
	}
	if rate.Limit != 5000 || rate.Remaining != 4999 || rate.Used != 1 {
		t.Errorf("unexpected rate metadata: %+v", rate)
	}
}

func TestSearchAuthoredPullRequestsBuildsQueryAndExtractsRepository(t *testing.T) {
	updatedAfter := time.Date(2026, time.July, 1, 3, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	tests := []struct {
		name         string
		opts         AuthoredPullRequestSearchOptions
		wantQuery    string
		wantPage     int
		wantPerPage  int
		wantHasNext  bool
		responseLink string
	}{
		{
			name: "filters state and UTC update date",
			opts: AuthoredPullRequestSearchOptions{
				Login:        "morluto",
				State:        "open",
				UpdatedAfter: updatedAfter,
				PageOptions:  PageOptions{Page: 3, PerPage: 25},
			},
			wantQuery:    "is:pr author:morluto is:open updated:>=2026-06-30",
			wantPage:     3,
			wantPerPage:  25,
			wantHasNext:  true,
			responseLink: `<https://api.github.com/search/issues?page=4&per_page=25>; rel="next"`,
		},
		{
			name: "all state omits state and update qualifiers",
			opts: AuthoredPullRequestSearchOptions{
				Login:       "morluto",
				State:       "all",
				PageOptions: PageOptions{Page: 1, PerPage: 50},
			},
			wantQuery:   "is:pr author:morluto",
			wantPage:    1,
			wantPerPage: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/v3/search/issues" {
					http.NotFound(w, r)
					return
				}
				query := r.URL.Query()
				if got := query.Get("q"); got != tt.wantQuery {
					t.Errorf("q = %q, want %q", got, tt.wantQuery)
				}
				if got := query.Get("sort"); got != "updated" {
					t.Errorf("sort = %q, want updated", got)
				}
				if got := query.Get("order"); got != "desc" {
					t.Errorf("order = %q, want desc", got)
				}
				if got := query.Get("page"); got != strconv.Itoa(tt.wantPage) {
					t.Errorf("page = %q, want %d", got, tt.wantPage)
				}
				if got := query.Get("per_page"); got != strconv.Itoa(tt.wantPerPage) {
					t.Errorf("per_page = %q, want %d", got, tt.wantPerPage)
				}

				setRateHeaders(w.Header())
				if tt.responseLink != "" {
					w.Header().Set("Link", tt.responseLink)
				}
				writeJSON(w, map[string]any{
					"total_count":        17,
					"incomplete_results": true,
					"items": []any{map[string]any{
						"id":             99,
						"node_id":        "PR_99",
						"number":         42,
						"title":          "Fix scheduler cleanup",
						"state":          "open",
						"repository_url": "https://api.github.com/repos/lab/runtime",
						"html_url":       "https://github.com/lab/runtime/pull/42",
						"user":           map[string]any{"login": "morluto"},
						"pull_request": map[string]any{
							"url":      "https://api.github.com/repos/lab/runtime/pulls/42",
							"html_url": "https://github.com/lab/runtime/pull/42",
						},
					}},
				})
			}))
			defer srv.Close()

			client := newTestClient(t, srv, nil)
			result, err := client.SearchAuthoredPullRequests(context.Background(), tt.opts)
			if err != nil {
				t.Fatalf("SearchAuthoredPullRequests: %v", err)
			}
			if result.Total != 17 || !result.Incomplete {
				t.Errorf("search summary = total %d incomplete %t", result.Total, result.Incomplete)
			}
			if result.Page.Page != tt.wantPage || result.Page.PerPage != tt.wantPerPage || result.Page.HasNext != tt.wantHasNext {
				t.Errorf("page = %+v, want page=%d per_page=%d has_next=%t", result.Page, tt.wantPage, tt.wantPerPage, tt.wantHasNext)
			}
			if len(result.Items) != 1 {
				t.Fatalf("got %d items, want 1", len(result.Items))
			}
			item := result.Items[0]
			if item.RepositoryOwner != "lab" || item.RepositoryName != "runtime" {
				t.Errorf("repository = %q/%q, want lab/runtime", item.RepositoryOwner, item.RepositoryName)
			}
			if item.Kind != ThreadKindPullRequest || item.Number != 42 {
				t.Errorf("item = kind %q number %d, want pull_request #42", item.Kind, item.Number)
			}
		})
	}
}
