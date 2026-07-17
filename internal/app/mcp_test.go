package app

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/corpus"
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

func TestDecodeJobJSONPreservesStructuredValues(t *testing.T) {
	object, err := decodeJobJSON("request", `{"owner":"acme","limit":20}`)
	if err != nil {
		t.Fatalf("decode object: %v", err)
	}
	fields, ok := object.(map[string]any)
	if !ok || fields["owner"] != "acme" || fields["limit"] != float64(20) {
		t.Fatalf("decoded object = %#v", object)
	}

	array, err := decodeJobJSON("result", `["one","two"]`)
	if err != nil {
		t.Fatalf("decode array: %v", err)
	}
	items, ok := array.([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("decoded array = %#v", array)
	}

	if _, err := decodeJobJSON("result", `{broken`); err == nil {
		t.Fatal("invalid persisted job JSON was accepted")
	}
}

func TestMCPReaderRepositorySearchDoesNotFallBackFromMissingExactRepository(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	if _, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{
		Owner: "owner", Name: "present", Description: "searchable repository",
	}, `{}`); err != nil {
		t.Fatalf("store repository: %v", err)
	}

	out, err := svc.MCPReader().SearchRepositories(ctx, mcpserver.SearchRepositoriesInput{
		Owner: "owner", Repo: "missing", Query: "searchable", Limit: 10,
	})
	if err != nil {
		t.Fatalf("search repositories: %v", err)
	}
	if out.Total != 0 || len(out.Matches) != 0 {
		t.Fatalf("missing exact repository returned global matches: %+v", out)
	}
}

func TestMCPReaderExplainRejectsNonMatchingThreadAndRepository(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{
		Owner: "owner", Name: "repo", Description: "parser utilities",
	}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.corpus.UpsertThread(ctx, corpus.Thread{
		RepositoryID: repo.ID, Kind: corpus.ThreadKindIssue, Number: 1,
		State: "open", Title: "parser panic", Body: "reproduction",
		SourceUpdatedAt: time.Now().UTC(),
	}, `{}`); err != nil {
		t.Fatal(err)
	}
	reader := svc.MCPReader()
	for _, input := range []mcpserver.ExplainMatchInput{
		{Owner: "owner", Repo: "repo", Kind: "issue", Number: 1, Query: "unrelated"},
		{Owner: "owner", Repo: "repo", Kind: "repo", Query: "unrelated"},
	} {
		if _, err := reader.ExplainMatch(ctx, input); !errors.Is(err, mcpserver.ErrNotFound) {
			t.Fatalf("ExplainMatch(%+v) error = %v, want ErrNotFound", input, err)
		}
	}
}

func TestMCPReaderExplainCodeRejectsDifferentRequestedPath(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	ref := domain.RepoRef{Owner: "owner", Repo: "repo"}
	if _, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: ref.Owner, Name: ref.Repo}, `{}`); err != nil {
		t.Fatalf("store repository: %v", err)
	}
	if _, _, err := svc.corpus.StoreCodeSnapshot(ctx, ref, codeindex.Snapshot{
		RepoPath: "/repo", Commit: "abc123", CreatedAt: time.Now().UTC(), TotalBytes: 25,
		Documents: []codeindex.Document{{Path: "parser.go", Content: "func searchableParser() {}", Bytes: 25, LanguageHint: "go"}},
	}); err != nil {
		t.Fatalf("store code snapshot: %v", err)
	}

	_, err := svc.MCPReader().ExplainMatch(ctx, mcpserver.ExplainMatchInput{
		Owner: ref.Owner, Repo: ref.Repo, Kind: "code", Query: "searchableParser", Path: "missing.go", Limit: 10,
	})
	if !errors.Is(err, mcpserver.ErrNotFound) {
		t.Fatalf("explain different path error = %v, want ErrNotFound", err)
	}
}

func TestMCPReaderExplainCodeExactPathNotOnFirstSearchPage(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	ref := domain.RepoRef{Owner: "owner", Repo: "repo"}
	if _, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: ref.Owner, Name: ref.Repo}, `{}`); err != nil {
		t.Fatalf("store repository: %v", err)
	}

	docs := make([]codeindex.Document, 0, 26)
	for i := range 25 {
		docs = append(docs, codeindex.Document{
			Path:    fmt.Sprintf("file%02d.go", i),
			Content: "func searchableParser() {}",
			Bytes:   25,
		})
	}
	docs = append(docs, codeindex.Document{
		Path:    "target.go",
		Content: "func searchableParser() {}",
		Bytes:   25,
	})
	if _, _, err := svc.corpus.StoreCodeSnapshot(ctx, ref, codeindex.Snapshot{
		RepoPath: "/repo", Commit: "abc123", CreatedAt: time.Now().UTC(), TotalBytes: 650,
		Documents: docs,
	}); err != nil {
		t.Fatalf("store code snapshot: %v", err)
	}

	// Confirm the target is not on the first search page.
	searchOut, err := svc.MCPReader().SearchCode(ctx, mcpserver.SearchCodeInput{
		Owner: ref.Owner, Repo: ref.Repo, Query: "searchableParser", Limit: 20,
	})
	if err != nil {
		t.Fatalf("search code: %v", err)
	}
	for _, m := range searchOut.Matches {
		if m.Path == "target.go" {
			t.Fatal("target.go unexpectedly on first search page")
		}
	}

	out, err := svc.MCPReader().ExplainMatch(ctx, mcpserver.ExplainMatchInput{
		Owner: ref.Owner, Repo: ref.Repo, Kind: "code", Query: "searchableParser", Path: "target.go",
	})
	if err != nil {
		t.Fatalf("explain match: %v", err)
	}
	if out.Path != "target.go" || out.Commit != "abc123" || out.Score <= 0 {
		t.Fatalf("unexpected explain output: %+v", out)
	}
}

func TestMCPReaderExplainCodeRejectsNonMatchingQuery(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	ref := domain.RepoRef{Owner: "owner", Repo: "repo"}
	if _, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: ref.Owner, Name: ref.Repo}, `{}`); err != nil {
		t.Fatalf("store repository: %v", err)
	}
	if _, _, err := svc.corpus.StoreCodeSnapshot(ctx, ref, codeindex.Snapshot{
		RepoPath: "/repo", Commit: "abc123", CreatedAt: time.Now().UTC(), TotalBytes: 20,
		Documents: []codeindex.Document{{Path: "parser.go", Content: "func unrelated() {}", Bytes: 20}},
	}); err != nil {
		t.Fatalf("store code snapshot: %v", err)
	}

	_, err := svc.MCPReader().ExplainMatch(ctx, mcpserver.ExplainMatchInput{
		Owner: ref.Owner, Repo: ref.Repo, Kind: "code", Query: "searchableParser", Path: "parser.go",
	})
	if !errors.Is(err, mcpserver.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for non-matching query, got %v", err)
	}
}

func TestMCPReaderExplainCodeRejectsWrongCommit(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	ref := domain.RepoRef{Owner: "owner", Repo: "repo"}
	if _, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: ref.Owner, Name: ref.Repo}, `{}`); err != nil {
		t.Fatalf("store repository: %v", err)
	}
	if _, _, err := svc.corpus.StoreCodeSnapshot(ctx, ref, codeindex.Snapshot{
		RepoPath: "/repo", Commit: "abc123", CreatedAt: time.Now().UTC(), TotalBytes: 25,
		Documents: []codeindex.Document{{Path: "parser.go", Content: "func searchableParser() {}", Bytes: 25}},
	}); err != nil {
		t.Fatalf("store code snapshot: %v", err)
	}

	_, err := svc.MCPReader().ExplainMatch(ctx, mcpserver.ExplainMatchInput{
		Owner: ref.Owner, Repo: ref.Repo, Kind: "code", Path: "parser.go", Commit: "deadbeef",
	})
	if !errors.Is(err, mcpserver.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for wrong commit, got %v", err)
	}

	out, err := svc.MCPReader().ExplainMatch(ctx, mcpserver.ExplainMatchInput{
		Owner: ref.Owner, Repo: ref.Repo, Kind: "code", Path: "parser.go", Commit: "abc123",
	})
	if err != nil {
		t.Fatalf("explain match: %v", err)
	}
	if out.Commit != "abc123" {
		t.Fatalf("unexpected commit: %s", out.Commit)
	}
}

func TestMCPReaderExplainThreadRejectsDifferentRequestedKind(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: "owner", Name: "repo"}, `{}`)
	if err != nil {
		t.Fatalf("store repository: %v", err)
	}
	if _, err := svc.corpus.UpsertThread(ctx, corpus.Thread{
		RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 1,
		State: "open", Title: "searchable change", SourceUpdatedAt: time.Now().UTC(),
	}, `{}`); err != nil {
		t.Fatalf("store pull request: %v", err)
	}

	_, err = svc.MCPReader().ExplainMatch(ctx, mcpserver.ExplainMatchInput{
		Owner: "owner", Repo: "repo", Kind: "issue", Number: 1, Query: "searchable", Limit: 10,
	})
	if !errors.Is(err, mcpserver.ErrNotFound) {
		t.Fatalf("explain different kind error = %v, want ErrNotFound", err)
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
		OpportunityID: opp.ID,
		Type:          evidence.EvidenceTypeMinimalReproduction,
		Relation:      evidence.RelationSupporting,
		Description:   "base branch crashes with attached input",
	}); err != nil {
		t.Fatalf("create evidence: %v", err)
	}
	if err := evSvc.CreateEvidence(ctx, &evidence.Evidence{
		OpportunityID: opp.ID,
		Type:          evidence.EvidenceTypeManualObservation, Relation: evidence.RelationSupporting,
		Description: "maintainer confirmed the expected behavior",
	}); err != nil {
		t.Fatalf("create second evidence: %v", err)
	}
	for _, title := range []string{"second hypothesis", "third hypothesis"} {
		if _, err := invSvc.RecordHypothesis(ctx, inv.ID, title, "additional context", investigation.CategoryOther, nil); err != nil {
			t.Fatalf("record %s: %v", title, err)
		}
	}

	reader := svc.MCPReader()

	invOut, err := reader.Investigation(ctx, mcpserver.InvestigationInput{ID: inv.ID, HypothesisLimit: 1})
	if err != nil {
		t.Fatalf("get investigation: %v", err)
	}
	if invOut.ID != inv.ID || invOut.Owner != "owner" || invOut.Repo != "repo" || invOut.Status != "open" {
		t.Fatalf("unexpected investigation output: %+v", invOut)
	}
	if invOut.HypothesisTotal != 3 || len(invOut.Hypotheses) != 1 || invOut.Hypotheses[0].ID != hyp.ID {
		t.Fatalf("unexpected hypotheses: %+v", invOut.Hypotheses)
	}

	listOut, err := reader.ListOpportunities(ctx, mcpserver.ListOpportunitiesInput{InvestigationID: inv.ID, Limit: 10})
	if err != nil {
		t.Fatalf("list opportunities: %v", err)
	}
	if listOut.Total != 1 || len(listOut.Opportunities) != 1 || listOut.Opportunities[0].ID != opp.ID {
		t.Fatalf("unexpected opportunities: %+v", listOut)
	}

	oppOut, err := reader.Opportunity(ctx, mcpserver.OpportunityInput{ID: opp.ID, EvidenceLimit: 1})
	if err != nil {
		t.Fatalf("get opportunity: %v", err)
	}
	if oppOut.ID != opp.ID || oppOut.InvestigationID != inv.ID || oppOut.Confidence != 0.75 {
		t.Fatalf("unexpected opportunity output: %+v", oppOut)
	}
	if len(oppOut.SourceRefs) != 1 || oppOut.SourceRefs[0].URL != "https://github.com/owner/repo/issues/1" {
		t.Fatalf("unexpected opportunity source refs: %+v", oppOut.SourceRefs)
	}
	if oppOut.EvidenceTotal != 2 || len(oppOut.EvidenceIDs) != 1 {
		t.Fatalf("unexpected evidence ids: %+v", oppOut.EvidenceIDs)
	}

	evOut, err := reader.Evidence(ctx, mcpserver.EvidenceInput{OpportunityID: opp.ID, Limit: 10})
	if err != nil {
		t.Fatalf("get evidence: %v", err)
	}
	if evOut.Total != 2 || len(evOut.Evidence) != 2 || evOut.Evidence[0].Relation != "supporting" {
		t.Fatalf("unexpected evidence: %+v", evOut)
	}
	for _, item := range evOut.Evidence {
		if item.Freshness != string(evidence.FreshnessNotApplicable) || item.FreshnessReason == "" {
			t.Fatalf("evidence freshness missing from MCP output: %+v", item)
		}
	}

	evByInv, err := reader.Evidence(ctx, mcpserver.EvidenceInput{InvestigationID: inv.ID, Relation: "supporting", Limit: 10})
	if err != nil {
		t.Fatalf("get evidence by investigation: %v", err)
	}
	if evByInv.Total != 2 {
		t.Fatalf("unexpected evidence by investigation: %+v", evByInv)
	}
	if _, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{
		Owner: "owner", Name: "repo", DefaultBranch: "main",
	}, `{}`); err != nil {
		t.Fatalf("seed repository projection: %v", err)
	}

	readinessOut, err := reader.Readiness(ctx, mcpserver.ReadinessInput{OpportunityID: opp.ID})
	if err != nil {
		t.Fatalf("get readiness: %v", err)
	}
	if readinessOut.OpportunityID != opp.ID || readinessOut.RuleSetVersion == "" || len(readinessOut.Checks) == 0 {
		t.Fatalf("unexpected readiness output: %+v", readinessOut)
	}
}
