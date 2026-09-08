package app

import (
	"context"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/investigation"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func TestMCPThreadAndRepositorySearchExposeCoverageRecovery(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newLocalService(t)
	defer func() { _ = svc.Close() }()
	reader := &MCPReader{svc}

	threads, err := reader.Search(ctx, mcpcontract.SearchInput{Owner: "owner", Repo: "repo", Kind: "issue", Query: "missing", Limit: 10})
	if err != nil {
		t.Fatalf("thread search: %v", err)
	}
	if !threads.Provenance.UnknownCoverage() || threads.Provenance.Complete() || threads.Recovery == nil || len(threads.Recovery.Then) != 1 || threads.Recovery.Then[0].Type() != "ensure_coverage" {
		t.Fatalf("thread search recovery = %+v", threads)
	}
	if got, ok := mcpcontract.RecoveryInput[mcpcontract.EnsureCoverageInput](threads.Recovery.Then[0]); !ok || got.Target.Repository.Owner != "owner" || got.Target.Repository.Repo != "repo" {
		t.Fatalf("thread recovery target = %+v", got)
	}

	repositories, err := reader.SearchRepositories(ctx, mcpcontract.SearchRepositoriesInput{Owner: "owner", Repo: "repo", Query: "missing", Limit: 10})
	if err != nil {
		t.Fatalf("repository search: %v", err)
	}
	if !repositories.Incomplete || !repositories.Provenance.UnknownCoverage() || repositories.Provenance.Complete() || repositories.Recovery == nil || len(repositories.Recovery.Then) != 1 || repositories.Recovery.Then[0].Type() != "sync_repository_context" {
		t.Fatalf("repository search recovery = %+v", repositories)
	}
}
func TestMCPRelatedWorkDoesNotTreatAbsentRepositoryAsNoFindings(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newLocalService(t)
	defer func() { _ = svc.Close() }()
	inv, err := svc.StartInvestigation(ctx, contracts.RepoRef{Owner: "owner", Repo: "absent"}, "abc", "")
	if err != nil {
		t.Fatalf("start investigation: %v", err)
	}
	hypothesis, err := svc.CreateHypothesis(ctx, inv.ID, investigation.CreateHypothesisInput{Title: "duplicate", Description: "check", Category: investigation.CategoryBug})
	if err != nil {
		t.Fatalf("create hypothesis: %v", err)
	}

	reader := &MCPReader{svc}
	duplicates, err := reader.CheckDuplicates(ctx, mcpcontract.CheckDuplicatesInput{Target: " Hypothesis ", ID: "  " + hypothesis.ID + "  ", Limit: 10})
	if err != nil {
		t.Fatalf("check duplicates: %v", err)
	}
	assertRelatedWorkRecovery(t, duplicates, "duplicate")
	if duplicates.Target != "hypothesis" || duplicates.ID != hypothesis.ID {
		t.Fatalf("canonical related-work identity = %+v", duplicates)
	}

	collisions, err := reader.CheckCollisions(ctx, mcpcontract.CheckCollisionsInput{Target: "hypothesis", ID: hypothesis.ID, Limit: 10})
	if err != nil {
		t.Fatalf("check collisions: %v", err)
	}
	assertRelatedWorkRecovery(t, collisions, "collision")
}

func assertRelatedWorkRecovery(t *testing.T, output mcpcontract.CheckOutput, kind string) {
	t.Helper()
	if output.Status != "unavailable" || output.Coverage != "unknown" || output.Total != 0 || output.Recovery == nil || len(output.Recovery.Then) != 1 || output.Recovery.Then[0].Type() != "sync_repository_context" {
		t.Fatalf("%s output = %+v", kind, output)
	}
	action, ok := mcpcontract.RecoveryInput[mcpcontract.SyncRepositoryContextInput](output.Recovery.Then[0])
	if !ok || len(action.Repositories) != 1 || action.Repositories[0].Owner != "owner" || action.Repositories[0].Repo != "absent" {
		t.Fatalf("%s recovery = %+v", kind, output.Recovery)
	}
}

func TestMCPRelatedWorkPreservesUnknownThreadCoverage(t *testing.T) {
	for _, coverage := range []string{"missing", "incomplete", "complete"} {
		t.Run(coverage, func(t *testing.T) {
			ctx := context.Background()
			svc := newSearchTestService(t)
			repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: "owner", Name: "repo"}, `{}`)
			if err != nil {
				t.Fatal(err)
			}
			if coverage != "missing" {
				if err := svc.corpus.AdvanceFacet(ctx, repo.ID, nil, "threads", time.Now(), coverage == "complete", 0); err != nil {
					t.Fatal(err)
				}
			}
			inv, err := svc.StartInvestigation(ctx, contracts.RepoRef{Owner: "owner", Repo: "repo"}, "abc", "")
			if err != nil {
				t.Fatal(err)
			}
			hypothesis, err := svc.CreateHypothesis(ctx, inv.ID, investigation.CreateHypothesisInput{Title: "parser", Description: "cancellation", Category: investigation.CategoryBug})
			if err != nil {
				t.Fatal(err)
			}
			for _, tool := range []string{"corpus.find_duplicates", "corpus.find_competing_pull_requests"} {
				session := connectAuditMCP(t, svc)
				out := callMCPTool[mcpcontract.CheckOutput](ctx, t, session, tool, map[string]any{"target": "hypothesis", "id": hypothesis.ID, "limit": 10})
				if coverage == "complete" {
					if out.Coverage != "complete" {
						t.Fatalf("complete corpus: %+v", out)
					}
					continue
				}
				if out.Coverage == "complete" || out.Status == "complete" || out.Recovery == nil {
					t.Fatalf("incomplete corpus became negative evidence: %+v", out)
				}
			}
		})
	}
}
