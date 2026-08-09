package app

import (
	"context"
	"testing"

	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func TestSearchContributionsReportsMissingActorCoverage(t *testing.T) {
	t.Parallel()
	reader := &MCPReader{Service: newSearchTestService(t)}
	out, err := reader.SearchContributions(context.Background(), mcpcontract.SearchContributionsInput{
		Actors: []string{"github:node:U_missing"},
		From:   "2025-01-01T00:00:00Z",
		To:     "2025-02-01T00:00:00Z",
		Limit:  10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 0 || len(out.Coverage) != 1 || out.Coverage[0].ActorID != "github:node:U_missing" || out.Coverage[0].Facet.Reason != "actor_not_indexed" {
		t.Fatalf("missing actor result = %+v", out)
	}
}

func TestActorSelectorsNormalizeWhitespaceBeforeDuplicateDetection(t *testing.T) {
	t.Parallel()
	_, _, err := parseActorSelectors([]mcpcontract.ActorSelector{
		{Type: "login", Login: "alice"},
		{Type: "login", Login: " Alice "},
	})
	if err == nil {
		t.Fatal("equivalent login selectors were not rejected as duplicates")
	}
	selectors, normalized, err := parseActorSelectors([]mcpcontract.ActorSelector{{Type: "node_id", NodeID: " U_1 "}})
	if err != nil {
		t.Fatal(err)
	}
	if got := selectors[0].key(); got != "U_1" || normalized[0].NodeID != "U_1" {
		t.Fatalf("normalized node selector = %q, wire = %+v", got, normalized[0])
	}
}
