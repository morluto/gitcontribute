package app

import (
	"context"
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func TestEnsureCoverageRejectsThreadFacetsForRepositoryTarget(t *testing.T) {
	_, err := (&MCPReader{}).EnsureCoverage(context.Background(), mcpcontract.EnsureCoverageInput{
		Target: mcpcontract.CoverageTarget{
			Type:       mcpcontract.CoverageTargetRepository,
			Repository: mcpcontract.RepositoryRef{Owner: "acme", Repo: "rocket"},
		},
		Facets: []string{"issue_comments"},
	})
	if err == nil || !strings.Contains(err.Error(), "exact-thread") {
		t.Fatalf("repository coverage with facets error = %v", err)
	}
}

func TestParseCoverageTargetRejectsMixedVariantsAndOwnsCanonicalIdentity(t *testing.T) {
	t.Parallel()
	thread := &mcpcontract.ExactCoverageThread{Kind: "issue", Number: 7}
	for _, target := range []mcpcontract.CoverageTarget{
		{Type: mcpcontract.CoverageTargetRepository, Repository: mcpcontract.RepositoryRef{Owner: "acme", Repo: "rocket"}, Thread: thread},
		{Type: mcpcontract.CoverageTargetExactThread, Repository: mcpcontract.RepositoryRef{Owner: "acme", Repo: "rocket"}},
		{Type: mcpcontract.CoverageTargetExactThread, Repository: mcpcontract.RepositoryRef{Owner: "acme", Repo: "rocket"}, Thread: &mcpcontract.ExactCoverageThread{Kind: "both", Number: 7}},
	} {
		if _, _, err := parseCoverageTarget(target); err == nil {
			t.Fatalf("parseCoverageTarget accepted mixed target: %+v", target)
		}
	}

	target := mcpcontract.CoverageTarget{
		Type:       mcpcontract.CoverageTargetExactThread,
		Repository: mcpcontract.RepositoryRef{Owner: " acme ", Repo: " rocket "},
		Thread:     thread,
	}
	parsed, normalized, err := parseCoverageTarget(target)
	if err != nil {
		t.Fatal(err)
	}
	thread.Kind = "pull_request"
	kind, number, exact := parsed.thread()
	if !exact || kind != "issue" || number != 7 || normalized.Repository.Owner != "acme" || normalized.Thread.Kind != "issue" {
		t.Fatalf("parsed target = %s#%d exact=%v, normalized=%+v", kind, number, exact, normalized)
	}
}
