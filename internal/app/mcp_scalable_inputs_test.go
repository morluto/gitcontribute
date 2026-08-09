package app

import (
	"context"
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func TestScalableBatchInputsRejectDuplicatesInsteadOfDroppingOutcomes(t *testing.T) {
	t.Parallel()
	if err := rejectDuplicateRepositoryRefs([]mcpcontract.RepositoryRef{{Owner: "one", Repo: "repo"}, {Owner: "ONE", Repo: "repo"}}); err == nil {
		t.Fatal("duplicate repositories were silently accepted")
	}
	if err := rejectDuplicateThreadRefs([]mcpcontract.ThreadRef{{Owner: "one", Repo: "repo", Number: 1}, {Owner: "one", Repo: "repo", Number: 1}}); err == nil {
		t.Fatal("duplicate threads were silently accepted")
	}
	if err := rejectDuplicateThreadRefs([]mcpcontract.ThreadRef{{Owner: "one", Repo: "repo", Kind: "issue", Number: 1}, {Owner: "one", Repo: "repo", Kind: "pull_request", Number: 1}}); err != nil {
		t.Fatalf("issue and pull request with the same number were conflated: %v", err)
	}
	if err := rejectDuplicateIndexRepositoryInputs([]mcpcontract.IndexRepositoryInput{{Owner: "one", Repo: "repo", Remote: "first"}, {Owner: "one", Repo: "repo", Remote: "second"}}); err == nil {
		t.Fatal("conflicting repository remotes were silently accepted")
	}
}

func TestPullRequestWorkflowsRejectMalformedReferencesBeforeSubmission(t *testing.T) {
	t.Parallel()
	reader := &MCPReader{newSearchTestService(t)}
	if _, err := reader.SyncPortfolio(context.Background(), mcpcontract.SyncPortfolioInput{}); err == nil || !strings.Contains(err.Error(), "selection is required") {
		t.Fatalf("missing portfolio selection error = %v", err)
	}
	for _, ref := range []mcpcontract.ThreadRef{
		{Owner: " ", Repo: "rocket", Number: 1},
		{Owner: "acme", Repo: " ", Number: 1},
		{Owner: "acme", Repo: "rocket", Number: 0},
		{Owner: "acme", Repo: "rocket", Kind: "issue", Number: 1},
	} {
		if _, err := reader.SyncPortfolio(context.Background(), mcpcontract.SyncPortfolioInput{
			Selection: "explicit", PullRequests: []mcpcontract.ThreadRef{ref},
		}); err == nil {
			t.Fatalf("SyncPortfolio accepted malformed pull request %+v", ref)
		}
	}
}

func TestSyncPortfolioRejectsDuplicateDefaultKindReferences(t *testing.T) {
	t.Parallel()
	reader := &MCPReader{newSearchTestService(t)}
	_, err := reader.SyncPortfolio(context.Background(), mcpcontract.SyncPortfolioInput{
		Selection: "explicit",
		PullRequests: []mcpcontract.ThreadRef{
			{Owner: "acme", Repo: "rocket", Number: 7},
			{Owner: "acme", Repo: "rocket", Kind: "pull_request", Number: 7},
		},
	})
	if err == nil {
		t.Fatal("expected duplicate pull-request references to be rejected")
	}
}

func TestParsedSyncInputsRejectDuplicatesAfterCanonicalization(t *testing.T) {
	t.Parallel()
	if _, _, err := parseSyncThreadsInput(mcpcontract.SyncThreadsInput{
		Selection: "repositories",
		Repositories: []mcpcontract.RepositoryRef{
			{Owner: "acme", Repo: "rocket"},
			{Owner: " ACME ", Repo: " rocket "},
		},
	}); err == nil {
		t.Fatal("repository duplicates created by canonicalization were accepted")
	}
	if _, _, err := parseSyncPortfolioInput(mcpcontract.SyncPortfolioInput{
		Selection: "explicit",
		PullRequests: []mcpcontract.ThreadRef{
			{Owner: "acme", Repo: "rocket", Number: 7},
			{Owner: " ACME ", Repo: " rocket ", Kind: " pull_request ", Number: 7},
		},
	}); err == nil {
		t.Fatal("pull-request duplicates created by canonicalization were accepted")
	}
}

func TestSyncInputsRejectFieldsFromTheOtherSelectionVariant(t *testing.T) {
	t.Parallel()
	repository := mcpcontract.RepositoryRef{Owner: "acme", Repo: "rocket"}
	thread := mcpcontract.ThreadRef{Owner: "acme", Repo: "rocket", Kind: "pull_request", Number: 7}
	threadCases := []mcpcontract.SyncThreadsInput{
		{Selection: "repositories", Repositories: []mcpcontract.RepositoryRef{repository}, Threads: []mcpcontract.ThreadRef{thread}},
		{Selection: "threads", Threads: []mcpcontract.ThreadRef{thread}, Repositories: []mcpcontract.RepositoryRef{repository}},
		{Selection: "threads", Threads: []mcpcontract.ThreadRef{thread}, State: "open"},
		{Selection: "threads", Threads: []mcpcontract.ThreadRef{thread}, LimitPerRepository: 1},
	}
	for _, input := range threadCases {
		if _, _, err := parseSyncThreadsInput(input); err == nil {
			t.Fatalf("parseSyncThreadsInput accepted mixed variants: %+v", input)
		}
	}
	portfolioCases := []mcpcontract.SyncPortfolioInput{
		{Selection: "authored", PullRequests: []mcpcontract.ThreadRef{thread}},
		{Selection: "explicit", PullRequests: []mcpcontract.ThreadRef{thread}, Repository: &repository},
		{Selection: "explicit", PullRequests: []mcpcontract.ThreadRef{thread}, State: "open"},
	}
	for _, input := range portfolioCases {
		if _, _, err := parseSyncPortfolioInput(input); err == nil {
			t.Fatalf("parseSyncPortfolioInput accepted mixed variants: %+v", input)
		}
	}
}

func TestParsedSyncInputsOwnCanonicalCopies(t *testing.T) {
	t.Parallel()
	repositories := []mcpcontract.RepositoryRef{{Owner: " acme ", Repo: " rocket "}}
	request, normalized, err := parseSyncThreadsInput(mcpcontract.SyncThreadsInput{Selection: "repositories", Repositories: repositories})
	if err != nil {
		t.Fatal(err)
	}
	repositories[0] = mcpcontract.RepositoryRef{Owner: "changed", Repo: "changed"}
	selection, ok := request.selection.(repositoryThreadSelection)
	if !ok || selection.repositories[0].Owner != "acme" || normalized.Repositories[0].Repo != "rocket" {
		t.Fatalf("parsed repository selection = %+v, normalized = %+v", request.selection, normalized)
	}

	inputRepository := &mcpcontract.RepositoryRef{Owner: " acme ", Repo: " rocket "}
	portfolio, normalizedPortfolio, err := parseSyncPortfolioInput(mcpcontract.SyncPortfolioInput{Selection: "authored", Repository: inputRepository})
	if err != nil {
		t.Fatal(err)
	}
	inputRepository.Owner = "changed"
	authored, ok := portfolio.selection.(authoredPortfolioSelection)
	if !ok || authored.repository.Owner != "acme" || normalizedPortfolio.Repository.Owner != "acme" {
		t.Fatalf("parsed authored selection = %+v, normalized = %+v", portfolio.selection, normalizedPortfolio)
	}
}

func TestScalableRuntimeRejectsPageBoundsBeforeSubmittingJob(t *testing.T) {
	t.Parallel()
	reader := &MCPReader{newSearchTestService(t)}
	ctx := context.Background()
	thread := mcpcontract.ThreadRef{Owner: "acme", Repo: "rocket", Number: 1}
	for _, maxPages := range []int{-1, 101} {
		if _, err := reader.HydrateThreads(ctx, mcpcontract.HydrateThreadsInput{Threads: []mcpcontract.ThreadRef{thread}, Facets: []string{"issue_comments"}, MaxPages: maxPages}); err == nil {
			t.Fatalf("HydrateThreads accepted max_pages=%d", maxPages)
		}
	}
	for _, maxPages := range []int{-1, 21} {
		if _, err := reader.SyncPortfolio(ctx, mcpcontract.SyncPortfolioInput{Selection: "explicit", PullRequests: []mcpcontract.ThreadRef{thread}, StatusMaxPages: maxPages}); err == nil {
			t.Fatalf("SyncPortfolio accepted status_max_pages=%d", maxPages)
		}
	}
	for _, limit := range []int{-1, 1001} {
		if _, err := reader.SyncThreads(ctx, mcpcontract.SyncThreadsInput{Selection: "repositories", Repositories: []mcpcontract.RepositoryRef{{Owner: "acme", Repo: "rocket"}}, LimitPerRepository: limit}); err == nil {
			t.Fatalf("SyncThreads accepted limit_per_repository=%d", limit)
		}
	}
	if _, err := reader.SyncPortfolio(ctx, mcpcontract.SyncPortfolioInput{Selection: "authored", Limit: 1, DiscoveryMaxRequests: 1}); err == nil {
		t.Fatal("SyncPortfolio accepted a budget that cannot fund identity and discovery")
	}
}

func TestScalableRuntimeBoundsMatchSchemas(t *testing.T) {
	t.Parallel()
	reader := &MCPReader{newSearchTestService(t)}
	if _, err := reader.RankOpportunities(context.Background(), mcpcontract.RankOpportunitiesInput{Repositories: []mcpcontract.RepositoryRef{{Owner: "acme", Repo: "rocket"}}, Limit: 101}); err == nil {
		t.Fatal("rank opportunities accepted limit above schema maximum")
	}
	if _, err := reader.FindPrecedents(context.Background(), mcpcontract.FindPrecedentsInput{Threads: []mcpcontract.ThreadRef{{Owner: "acme", Repo: "rocket", Number: 1}}, Limit: 101}); err == nil {
		t.Fatal("find precedents accepted limit above schema maximum")
	}
}
