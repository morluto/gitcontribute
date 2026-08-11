package app

import (
	"context"
	"errors"
	"testing"

	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func TestActorFacetItemsPreserveDurableJSONShapes(t *testing.T) {
	t.Parallel()
	base := actorFacetCountSuccess{actorID: "github:node:U1", login: "alice", items: 2, complete: false}
	tests := []struct {
		name     string
		item     actorFacetItem
		expected string
	}{
		{
			name:     "count",
			item:     successfulActorFacetItem("alice", base),
			expected: `{"key":"alice","status":"complete","actor_id":"github:node:U1","login":"alice","items":2,"complete":false}`,
		},
		{
			name:     "pinned items",
			item:     successfulActorFacetItem("alice", actorPinnedItemsSuccess{actorFacetCountSuccess: base, showcaseKind: "repository"}),
			expected: `{"key":"alice","status":"complete","actor_id":"github:node:U1","login":"alice","items":2,"complete":false,"showcase_kind":"repository"}`,
		},
		{
			name:     "repositories",
			item:     successfulActorFacetItem("alice", actorRepositoriesSuccess{actorFacetCountSuccess: base, relationship: "owned"}),
			expected: `{"key":"alice","status":"complete","actor_id":"github:node:U1","login":"alice","items":2,"complete":false,"relationship":"owned"}`,
		},
		{
			name:     "contributions",
			item:     successfulActorFacetItem("alice", actorContributionsSuccess{actorFacetCountSuccess: base, from: "2026-01-01T00:00:00Z", to: "2026-02-01T00:00:00Z"}),
			expected: `{"key":"alice","status":"complete","actor_id":"github:node:U1","login":"alice","items":2,"complete":false,"from":"2026-01-01T00:00:00Z","to":"2026-02-01T00:00:00Z"}`,
		},
		{
			name:     "failure",
			item:     failedActorFacetItem("alice", mcpcontract.BatchItemRetryable, "rate_limited", "wait", 0),
			expected: `{"key":"alice","status":"retryable","reason":"rate_limited","message":"wait","retry_after_ms":0}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertJSONDocumentEqual(t, test.item, test.expected)
		})
	}
}

func TestRunActorFacetItemsPreservesOrderAndPartialStatus(t *testing.T) {
	t.Parallel()
	selectors := []parsedActorSelector{actorLogin("Alice"), actorLogin("Bob")}
	reports := 0
	result, err := (&MCPReader{}).runActorFacetItems(context.Background(), selectors, "social_accounts", func(string, string) error {
		reports++
		return nil
	}, func(selector parsedActorSelector) (actorFacetSuccess, error) {
		if selector.key() == "bob" {
			return nil, errors.New("provider failed")
		}
		return actorFacetCountSuccess{actorID: "github:node:U1", login: "Alice", items: 1, complete: true}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != batchOperationPartial || result.Completed != 1 || result.Total != 2 || reports != 3 {
		t.Fatalf("result = %+v, reports = %d", result, reports)
	}
	assertJSONDocumentEqual(t, result, `{
		"status":"partial","items":[
			{"key":"alice","status":"complete","actor_id":"github:node:U1","login":"Alice","items":1,"complete":true},
			{"key":"bob","status":"failed","reason":"request_failed","message":"provider failed","retry_after_ms":0}
		],"completed":1,"total":2
	}`)
}

func TestActorProfileItemsPreserveDurableJSONShapes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		item     actorProfileItem
		expected string
	}{
		{
			name:     "success",
			item:     successfulActorProfileItem("alice", "github:node:U1", "Alice"),
			expected: `{"key":"alice","status":"complete","actor_id":"github:node:U1","login":"Alice"}`,
		},
		{
			name:     "unresolved selector",
			item:     unresolvedActorProfileItem("U1", "actor_login_unknown", "node ID is not stored"),
			expected: `{"key":"U1","status":"unavailable","reason":"actor_login_unknown","message":"node ID is not stored"}`,
		},
		{
			name:     "provider failure",
			item:     failedActorProfileItem("alice", mcpcontract.BatchItemRetryable, "rate_limited", "wait", 0),
			expected: `{"key":"alice","status":"retryable","reason":"rate_limited","message":"wait","retry_after_ms":0}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertJSONDocumentEqual(t, test.item, test.expected)
		})
	}
}
