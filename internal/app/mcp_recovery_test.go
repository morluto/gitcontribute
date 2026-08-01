package app

import (
	"context"
	"testing"

	"github.com/morluto/gitcontribute/internal/contracts"
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
	if !threads.Provenance.UnknownCoverage || threads.Recovery == nil || len(threads.Recovery.Then) != 1 || threads.Recovery.Then[0].Type != "ensure_coverage" {
		t.Fatalf("thread search recovery = %+v", threads)
	}
	if got := threads.Recovery.Then[0].EnsureCoverage; got == nil || got.Target.Repository.Owner != "owner" || got.Target.Repository.Repo != "repo" {
		t.Fatalf("thread recovery target = %+v", got)
	}

	code, err := reader.SearchCode(ctx, mcpcontract.SearchCodeInput{Query: "missing", Limit: 10})
	if err != nil {
		t.Fatalf("unscoped code search: %v", err)
	}
	if !code.Provenance.UnknownCoverage || code.Recovery == nil || len(code.Recovery.Then) != 1 || code.Recovery.Then[0].Type != "search_github_repositories" {
		t.Fatalf("unscoped code recovery = %+v", code)
	}

	repositories, err := reader.SearchRepositories(ctx, mcpcontract.SearchRepositoriesInput{Owner: "owner", Repo: "repo", Query: "missing", Limit: 10})
	if err != nil {
		t.Fatalf("repository search: %v", err)
	}
	if !repositories.Incomplete || repositories.Recovery == nil || len(repositories.Recovery.Then) != 1 || repositories.Recovery.Then[0].Type != "sync_repository_context" {
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
	duplicates, err := reader.CheckDuplicates(ctx, mcpcontract.CheckDuplicatesInput{Target: "hypothesis", ID: hypothesis.ID, Limit: 10})
	if err != nil {
		t.Fatalf("check duplicates: %v", err)
	}
	assertRelatedWorkRecovery(t, duplicates, "duplicate")

	collisions, err := reader.CheckCollisions(ctx, mcpcontract.CheckCollisionsInput{Target: "hypothesis", ID: hypothesis.ID, Limit: 10})
	if err != nil {
		t.Fatalf("check collisions: %v", err)
	}
	assertRelatedWorkRecovery(t, collisions, "collision")
}

func assertRelatedWorkRecovery(t *testing.T, output mcpcontract.CheckOutput, kind string) {
	t.Helper()
	if output.Status != "unavailable" || output.Coverage != "unknown" || output.Total != 0 || output.Recovery == nil || len(output.Recovery.Then) != 1 || output.Recovery.Then[0].Type != "sync_repository_context" {
		t.Fatalf("%s output = %+v", kind, output)
	}
	action := output.Recovery.Then[0].SyncRepositoryContext
	if action == nil || len(action.Repositories) != 1 || action.Repositories[0].Owner != "owner" || action.Repositories[0].Repo != "absent" {
		t.Fatalf("%s recovery = %+v", kind, output.Recovery)
	}
}
