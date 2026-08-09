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
