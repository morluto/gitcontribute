package app

import (
	"context"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/investigation"
	"github.com/morluto/gitcontribute/internal/mcpserver"
)

func TestMCPReaderSearchCodeIntegration(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer("owner", "repo")
	defer srv.Close()

	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()

	if _, _, err := svc.corpus.StoreCodeSnapshot(ctx, domain.RepoRef{Owner: "owner", Repo: "repo"}, codeindex.Snapshot{
		RepoPath: "/repo", Commit: "abc123", CreatedAt: time.Now().UTC(), TotalBytes: 25,
		Documents: []codeindex.Document{{Path: "parser.go", Content: "func searchableParser() {}", Bytes: 25, LanguageHint: "go"}},
	}); err != nil {
		t.Fatalf("store code snapshot: %v", err)
	}

	reader := svc.MCPReader()
	out, err := reader.SearchCode(ctx, mcpserver.SearchCodeInput{Query: "searchableParser", Limit: 10})
	if err != nil {
		t.Fatalf("search code: %v", err)
	}
	if out.Total != 1 || len(out.Matches) != 1 {
		t.Fatalf("unexpected output: %+v", out)
	}
	match := out.Matches[0]
	if match.Repo != "owner/repo" || match.Commit != "abc123" || match.Path != "parser.go" {
		t.Fatalf("unexpected match: %+v", match)
	}
	if match.Snippet != "func searchableParser() {}" {
		t.Fatalf("unexpected snippet: %q", match.Snippet)
	}
}

func TestMCPReaderInvestigationWorkflow(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer("owner", "repo")
	defer srv.Close()

	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()

	invSvc := investigation.NewService(svc.corpus, svc.corpus)
	evSvc := evidence.NewService(svc.corpus, nil)

	ref := domain.RepoRef{Owner: "owner", Repo: "repo"}
	inv, err := invSvc.StartInvestigation(ctx, ref, "deadbeef", "")
	if err != nil {
		t.Fatalf("start investigation: %v", err)
	}
	hyp, err := invSvc.RecordHypothesis(ctx, inv.ID, "panic", "parser panics on empty input", investigation.CategoryBug, []domain.SourceRef{
		{Source: "github", URL: "https://github.com/owner/repo/issues/1"},
	})
	if err != nil {
		t.Fatalf("record hypothesis: %v", err)
	}
	opp, err := invSvc.PromoteOpportunity(ctx, hyp.ID, "panic on empty input", "parser", "crash", "small", 0.75)
	if err != nil {
		t.Fatalf("promote opportunity: %v", err)
	}
	if err := evSvc.CreateEvidence(ctx, &evidence.Evidence{
		InvestigationID: inv.ID,
		OpportunityID:   opp.ID,
		Type:            evidence.EvidenceTypeMinimalReproduction,
		Relation:        evidence.RelationSupporting,
		Description:     "base branch crashes with attached input",
	}); err != nil {
		t.Fatalf("create evidence: %v", err)
	}

	reader := svc.MCPReader()

	invOut, err := reader.Investigation(ctx, mcpserver.InvestigationInput{ID: inv.ID})
	if err != nil {
		t.Fatalf("get investigation: %v", err)
	}
	if invOut.ID != inv.ID || invOut.Owner != "owner" || invOut.Repo != "repo" || invOut.Status != "open" {
		t.Fatalf("unexpected investigation output: %+v", invOut)
	}
	if len(invOut.Hypotheses) != 1 || invOut.Hypotheses[0].ID != hyp.ID {
		t.Fatalf("unexpected hypotheses: %+v", invOut.Hypotheses)
	}

	listOut, err := reader.ListOpportunities(ctx, mcpserver.ListOpportunitiesInput{InvestigationID: inv.ID, Limit: 10})
	if err != nil {
		t.Fatalf("list opportunities: %v", err)
	}
	if listOut.Total != 1 || len(listOut.Opportunities) != 1 || listOut.Opportunities[0].ID != opp.ID {
		t.Fatalf("unexpected opportunities: %+v", listOut)
	}

	oppOut, err := reader.Opportunity(ctx, mcpserver.OpportunityInput{ID: opp.ID})
	if err != nil {
		t.Fatalf("get opportunity: %v", err)
	}
	if oppOut.ID != opp.ID || oppOut.InvestigationID != inv.ID || oppOut.Confidence != 0.75 {
		t.Fatalf("unexpected opportunity output: %+v", oppOut)
	}
	if len(oppOut.SourceRefs) != 1 || oppOut.SourceRefs[0].URL != "https://github.com/owner/repo/issues/1" {
		t.Fatalf("unexpected opportunity source refs: %+v", oppOut.SourceRefs)
	}
	if len(oppOut.EvidenceIDs) != 1 {
		t.Fatalf("unexpected evidence ids: %+v", oppOut.EvidenceIDs)
	}

	evOut, err := reader.Evidence(ctx, mcpserver.EvidenceInput{OpportunityID: opp.ID, Limit: 10})
	if err != nil {
		t.Fatalf("get evidence: %v", err)
	}
	if evOut.Total != 1 || len(evOut.Evidence) != 1 || evOut.Evidence[0].Relation != "supporting" {
		t.Fatalf("unexpected evidence: %+v", evOut)
	}

	evByInv, err := reader.Evidence(ctx, mcpserver.EvidenceInput{InvestigationID: inv.ID, Relation: "supporting", Limit: 10})
	if err != nil {
		t.Fatalf("get evidence by investigation: %v", err)
	}
	if evByInv.Total != 1 {
		t.Fatalf("unexpected evidence by investigation: %+v", evByInv)
	}
}
