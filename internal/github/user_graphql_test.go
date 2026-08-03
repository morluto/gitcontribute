package github

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSearchUsersReturnsIdentityOnlyPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/search/users" || r.URL.Query().Get("q") != "language:go" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, map[string]any{"total_count": 1, "incomplete_results": false, "items": []any{map[string]any{"login": "octocat", "id": 1, "node_id": "U_1", "type": "User", "avatar_url": "https://example/avatar"}}})
	}))
	defer srv.Close()

	result, err := newTestClient(t, srv, StaticTokenSource("")).SearchUsers(context.Background(), UserSearchOptions{Query: "language:go", Sort: "followers", Order: "desc", Page: 1, PerPage: 20})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || len(result.Items) != 1 || result.Items[0].NodeID != "U_1" || result.Items[0].Login != "octocat" {
		t.Fatalf("unexpected search result: %+v", result)
	}
}

func TestUserRepositoryRelationshipsUseGitHubRESTValues(t *testing.T) {
	t.Parallel()
	for product, want := range map[string]string{"owned": "owner", "affiliated": "member"} {
		t.Run(product, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v3/users/octocat/repos" || r.URL.Query().Get("type") != want {
					t.Errorf("request = %s?%s, want type=%s", r.URL.Path, r.URL.RawQuery, want)
				}
				writeJSON(w, []any{})
			}))
			defer srv.Close()
			if _, err := newTestClient(t, srv, StaticTokenSource("")).ListUserRepositories(context.Background(), "octocat", UserRepositoryOptions{Relationship: product}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUserOrganizationsUsesEnterpriseGraphQLEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/graphql" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, map[string]any{"data": map[string]any{"user": map[string]any{"organizations": map[string]any{"nodes": []any{map[string]any{"id": "O_1", "login": "acme"}}, "pageInfo": map[string]any{"hasNextPage": false, "endCursor": "cursor-1"}}}}})
	}))
	defer srv.Close()

	result, err := newTestClient(t, srv, StaticTokenSource("")).ListUserOrganizations(context.Background(), "octocat", CursorPageOptions{First: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Login != "acme" || result.Page.EndCursor != "cursor-1" {
		t.Fatalf("unexpected organization result: %+v", result)
	}
}

func TestGitHubGraphQLEndpointUsesPublicAndEnterpriseLayouts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		base string
		want string
	}{
		{base: "https://api.github.com/", want: "https://api.github.com/graphql"},
		{base: "https://github.example/api/v3/", want: "https://github.example/api/graphql"},
		{base: "https://api.github.example/api/v3/", want: "https://api.github.example/api/graphql"},
	}
	for _, test := range tests {
		got, err := githubGraphQLURL(test.base)
		if err != nil {
			t.Fatalf("githubGraphQLURL(%q): %v", test.base, err)
		}
		if got != test.want {
			t.Errorf("githubGraphQLURL(%q) = %q, want %q", test.base, got, test.want)
		}
	}
}

func TestNewClientUsesPublicGraphQLEndpointByDefault(t *testing.T) {
	t.Parallel()
	client, err := NewClient(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if client.graphQLURL != "https://api.github.com/graphql" {
		t.Fatalf("default GraphQL URL = %q", client.graphQLURL)
	}
}

func TestUserContributionsPreservesRestrictedAggregate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		query := string(body)
		if strings.Contains(query, "issueContributions(first:100,maxRepositories") || !strings.Contains(query, "commitContributionsByRepository(maxRepositories") {
			t.Errorf("invalid contribution query: %s", query)
		}
		writeJSON(w, map[string]any{"data": map[string]any{"user": map[string]any{"contributionsCollection": map[string]any{
			"startedAt": "2025-01-01T00:00:00Z", "endedAt": "2025-02-01T00:00:00Z", "hasAnyRestrictedContributions": true,
			"restrictedContributionsCount": 3, "totalCommitContributions": 4, "totalIssueContributions": 1, "totalPullRequestContributions": 2, "totalPullRequestReviewContributions": 5, "totalRepositoryContributions": 1,
			"contributionCalendar":            map[string]any{"weeks": []any{}},
			"commitContributionsByRepository": []any{}, "issueContributionsByRepository": []any{}, "pullRequestContributionsByRepository": []any{}, "pullRequestReviewContributionsByRepository": []any{}, "repositoryContributions": map[string]any{"nodes": []any{}},
		}}}})
	}))
	defer srv.Close()

	result, err := newTestClient(t, srv, StaticTokenSource("")).GetUserContributions(context.Background(), "octocat", UserContributionOptions{From: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC), MaxRepositories: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.RestrictedContributions != 3 || result.TotalPullRequestReviews != 5 {
		t.Fatalf("unexpected contribution aggregate: %+v", result)
	}
}

func TestUserContributionsAreIncompleteAtRepositoryGroupCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		group := func(id string) map[string]any {
			return map[string]any{"repository": map[string]any{"id": id, "nameWithOwner": "acme/" + id}, "contributions": map[string]any{"nodes": []any{}, "pageInfo": map[string]any{"hasNextPage": false}}}
		}
		writeJSON(w, map[string]any{"data": map[string]any{"user": map[string]any{"contributionsCollection": map[string]any{
			"startedAt": "2025-01-01T00:00:00Z", "endedAt": "2025-02-01T00:00:00Z",
			"contributionCalendar": map[string]any{"weeks": []any{}}, "commitContributionsByRepository": []any{group("one"), group("two")},
			"issueContributions": map[string]any{"nodes": []any{}, "pageInfo": map[string]any{}}, "pullRequestContributions": map[string]any{"nodes": []any{}, "pageInfo": map[string]any{}},
			"pullRequestReviewContributions": map[string]any{"nodes": []any{}, "pageInfo": map[string]any{}}, "repositoryContributions": map[string]any{"nodes": []any{}, "pageInfo": map[string]any{}},
		}}}})
	}))
	defer srv.Close()
	result, err := newTestClient(t, srv, StaticTokenSource("")).GetUserContributions(context.Background(), "octocat", UserContributionOptions{From: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC), MaxRepositories: 2})
	if err != nil {
		t.Fatal(err)
	}
	if result.Complete {
		t.Fatal("repository-group cap was reported as complete")
	}
}

func TestUserPinnedItemsReadsProfileShowcase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "itemShowcase") || strings.Contains(string(body), "profileItemShowcase") {
			t.Errorf("invalid showcase query: %s", body)
		}
		writeJSON(w, map[string]any{
			"data": map[string]any{"user": map[string]any{
				"itemShowcase": map[string]any{
					"hasPinnedItems": true,
					"items": map[string]any{
						"nodes":    []any{map[string]any{"__typename": "Repository", "id": "R_1", "name": "ml", "owner": map[string]any{"login": "acme"}}},
						"pageInfo": map[string]any{"hasNextPage": false},
					},
				},
			}},
		})
	}))
	defer srv.Close()

	result, err := newTestClient(t, srv, StaticTokenSource("")).GetUserPinnedItems(context.Background(), "octocat", 6)
	if err != nil {
		t.Fatal(err)
	}
	if result.ShowcaseKind != "pinned" || len(result.Items) != 1 || result.Items[0].RepositoryOwner != "acme" {
		t.Fatalf("unexpected pinned items: %+v", result)
	}
}
