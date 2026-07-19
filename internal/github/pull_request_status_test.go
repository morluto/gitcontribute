package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestGetPullRequestStatusConvertsHealthAndReportsPartialCoverage(t *testing.T) {
	var request graphQLRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v3/graphql" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		checkContexts := map[string]any{
			"totalCount": 2,
			"nodes": []any{
				map[string]any{"__typename": "CheckRun", "name": "test", "status": "COMPLETED", "conclusion": "FAILURE", "detailsUrl": "https://github.com/acme/project/actions/1"},
				map[string]any{"__typename": "StatusContext", "context": "lint", "state": "SUCCESS", "targetUrl": "https://ci.example/lint", "createdAt": "2026-07-19T08:15:00Z"},
			},
			"pageInfo": map[string]any{"hasNextPage": false, "endCursor": "checks-end"},
		}
		commitNode := map[string]any{"commit": map[string]any{"statusCheckRollup": map[string]any{"contexts": checkContexts}}}
		pullRequest := map[string]any{
			"id": "PR_20", "updatedAt": "2026-07-19T08:30:00Z", "headRefOid": "abc123",
			"mergeStateStatus": "BLOCKED", "mergeable": "MERGEABLE",
			"mergeQueueEntry":         map[string]any{"id": "MQE_1", "state": "QUEUED", "position": 3, "enqueuedAt": "2026-07-19T08:00:00Z", "estimatedTimeToMerge": 90000},
			"closingIssuesReferences": map[string]any{"totalCount": 1, "nodes": []any{map[string]any{"id": "I_7", "number": 7, "url": "https://github.com/acme/project/issues/7", "repository": map[string]any{"nameWithOwner": "acme/project"}}}, "pageInfo": map[string]any{"hasNextPage": false, "endCursor": "issues-end"}},
			"files":                   map[string]any{"totalCount": 3, "nodes": []any{map[string]any{"path": "internal/one.go", "changeType": "MODIFIED", "additions": 4, "deletions": 2}, map[string]any{"path": "internal/two.go", "changeType": "ADDED", "additions": 8, "deletions": 0}}, "pageInfo": map[string]any{"hasNextPage": true, "endCursor": "files-next"}},
			"reviewThreads":           map[string]any{"totalCount": 2, "nodes": []any{map[string]any{"id": "RT_1", "isResolved": false, "isOutdated": false, "path": "internal/one.go", "line": 12}}, "pageInfo": map[string]any{"hasNextPage": true, "endCursor": "threads-next"}},
			"commits":                 map[string]any{"nodes": []any{commitNode}},
		}
		writeJSON(w, map[string]any{"data": map[string]any{"repository": map[string]any{"pullRequest": pullRequest}}})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, nil)
	result, err := client.GetPullRequestStatus(context.Background(), "acme", "project", 20, PullRequestStatusOptions{PageSize: 500, MaxPages: 1})
	if err != nil {
		t.Fatalf("GetPullRequestStatus: %v", err)
	}
	if request.Variables["first"] != float64(maxPullRequestStatusPageSize) {
		t.Errorf("first = %#v, want %d", request.Variables["first"], maxPullRequestStatusPageSize)
	}
	if result.HeadSHA != "abc123" || result.MergeState.MergeStateStatus != "BLOCKED" || !result.MergeState.MergeableKnown {
		t.Errorf("status identity/merge = %+v", result)
	}
	if result.MergeQueue == nil || result.MergeQueue.Position != 3 || result.MergeQueue.EstimatedTimeToMergeSeconds == nil || *result.MergeQueue.EstimatedTimeToMergeSeconds != 90000 {
		t.Errorf("merge queue = %+v", result.MergeQueue)
	}
	if !result.Checks.Coverage.Complete || len(result.Checks.Items) != 2 || result.Checks.Items[1].Name != "lint" || result.Checks.Items[1].Status != "SUCCESS" {
		t.Errorf("checks = %+v", result.Checks)
	}
	if result.ReviewThreads.Coverage.Complete || result.ReviewThreads.Coverage.Total != 2 || result.ReviewThreads.Coverage.EndCursor != "threads-next" {
		t.Errorf("review thread coverage = %+v", result.ReviewThreads.Coverage)
	}
	if result.Files.Coverage.Complete || result.Files.Coverage.Fetched != 2 || result.Files.Coverage.Total != 3 {
		t.Errorf("file coverage = %+v", result.Files.Coverage)
	}
	if !result.ClosingIssues.Coverage.Complete || result.ClosingIssues.Items[0].RepositoryFullName != "acme/project" {
		t.Errorf("closing issues = %+v", result.ClosingIssues)
	}
}

func TestGetPullRequestStatusPaginatesFacetsToCompletion(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var request graphQLRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		files := map[string]any{"totalCount": 2, "nodes": []any{map[string]any{"path": "one.go"}}, "pageInfo": map[string]any{"hasNextPage": true, "endCursor": "files-1"}}
		if request.Variables["filesAfter"] == "files-1" {
			files = map[string]any{"totalCount": 2, "nodes": []any{map[string]any{"path": "two.go"}}, "pageInfo": map[string]any{"hasNextPage": false, "endCursor": "files-2"}}
		}
		pullRequest := map[string]any{
			"id": "PR_20", "updatedAt": "2026-07-19T08:30:00Z", "headRefOid": "abc123", "mergeStateStatus": "CLEAN", "mergeable": "MERGEABLE",
			"closingIssuesReferences": emptyConnection(), "files": files, "reviewThreads": emptyConnection(), "commits": map[string]any{"nodes": []any{}},
		}
		writeJSON(w, map[string]any{"data": map[string]any{"repository": map[string]any{"pullRequest": pullRequest}}})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, nil)
	result, err := client.GetPullRequestStatus(context.Background(), "acme", "project", 20, PullRequestStatusOptions{PageSize: 1, MaxPages: 2})
	if err != nil {
		t.Fatalf("GetPullRequestStatus: %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
	if !result.Files.Coverage.Complete || result.Files.Coverage.Fetched != 2 || len(result.Files.Items) != 2 || result.Files.Items[1].Path != "two.go" {
		t.Errorf("files = %+v", result.Files)
	}
}

func TestGetPullRequestStatusKeepsNullAndUnknownMergeabilityUnknown(t *testing.T) {
	for _, mergeable := range []any{nil, "UNKNOWN"} {
		name := "UNKNOWN"
		if mergeable == nil {
			name = "null"
		}
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				pullRequest := map[string]any{
					"id": "PR_20", "updatedAt": "2026-07-19T08:30:00Z", "headRefOid": "abc123", "mergeStateStatus": "UNKNOWN", "mergeable": mergeable,
					"closingIssuesReferences": emptyConnection(), "files": emptyConnection(), "reviewThreads": emptyConnection(), "commits": map[string]any{"nodes": []any{}},
				}
				writeJSON(w, map[string]any{"data": map[string]any{"repository": map[string]any{"pullRequest": pullRequest}}})
			}))
			defer srv.Close()
			client := newTestClient(t, srv, nil)
			result, err := client.GetPullRequestStatus(context.Background(), "acme", "project", 20, PullRequestStatusOptions{})
			if err != nil {
				t.Fatalf("GetPullRequestStatus: %v", err)
			}
			if result.MergeState.MergeableKnown {
				t.Errorf("mergeability = %+v, want unknown", result.MergeState)
			}
			if !result.MergeStateCoverage.Complete {
				t.Errorf("coverage = %+v, want observed scalar", result.MergeStateCoverage)
			}
		})
	}
}

func TestGetPullRequestStatusClassifiesGraphQLRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Ratelimit-Limit", "5000")
		w.Header().Set("X-Ratelimit-Remaining", "0")
		w.Header().Set("X-Ratelimit-Reset", strconv.FormatInt(time.Now().Add(time.Minute).Unix(), 10))
		writeJSON(w, map[string]any{"errors": []any{map[string]any{"type": "RATE_LIMITED", "message": "API rate limit exceeded"}}})
	}))
	defer srv.Close()
	client := newTestClient(t, srv, nil)
	_, err := client.GetPullRequestStatus(context.Background(), "acme", "project", 20, PullRequestStatusOptions{})
	var rateErr *PrimaryRateLimitError
	if !errors.As(err, &rateErr) || rateErr.RetryAfter <= 0 {
		t.Fatalf("rate error = %T %v", err, err)
	}
}

func emptyConnection() map[string]any {
	return map[string]any{"totalCount": 0, "nodes": []any{}, "pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil}}
}
