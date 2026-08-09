package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func TestPullRequestPortfolioDerivesConflictAndPreservesUnknownCoverage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)
	now := time.Unix(1000, 0).UTC()
	svc.SetClock(func() time.Time { return now })
	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: "acme", Name: "rocket"}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	conflicted, err := svc.corpus.UpsertThread(ctx, corpus.Thread{RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 1, State: "open", Title: "fix cache", Author: "alice", SourceUpdatedAt: now}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := svc.corpus.UpsertThread(ctx, corpus.Thread{RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 2, State: "open", Title: "fix parser", Author: "alice", SourceUpdatedAt: now}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	mergeable := false
	details, _ := json.Marshal(github.PullRequestDetails{Number: 1, Mergeable: &mergeable, HeadRef: "feature", HeadSHA: "head", BaseRef: "main", BaseSHA: "base", UpdatedAt: now})
	if err := svc.corpus.ApplyFacetObservationSet(ctx, repo.ID, &conflicted.ID, FacetPRDetails, now, []corpus.FacetObservationInput{{SourceUpdatedAt: now, Payload: string(details)}}, true, 0); err != nil {
		t.Fatal(err)
	}
	out, err := (&MCPReader{svc}).ListPullRequestPortfolio(ctx, mcpcontract.ListPullRequestPortfolioInput{Authors: []string{"alice"}, State: "open", Limit: 10, View: "full"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "partial" || len(out.PullRequests) != 2 {
		t.Fatalf("unexpected portfolio: %+v", out)
	}
	byNumber := map[int]mcpcontract.PullRequestPortfolioItem{}
	for _, item := range out.PullRequests {
		byNumber[item.Number] = item
	}
	if byNumber[conflicted.Number].Attention != "conflicted" || byNumber[conflicted.Number].HeadSHA != "head" {
		t.Fatalf("conflict not derived: %+v", byNumber[conflicted.Number])
	}
	if byNumber[unknown.Number].Attention != "unknown" || byNumber[unknown.Number].StatusCoverage != "missing" {
		t.Fatalf("unknown coverage collapsed: %+v", byNumber[unknown.Number])
	}
	if byNumber[unknown.Number].Recovery == nil || len(byNumber[unknown.Number].Recovery.Then) == 0 || byNumber[unknown.Number].Recovery.Then[0].Type() != "sync_portfolio" {
		t.Fatalf("unknown portfolio recovery = %+v", byNumber[unknown.Number].Recovery)
	}
	concise, err := (&MCPReader{svc}).ListPullRequestPortfolio(ctx, mcpcontract.ListPullRequestPortfolioInput{Authors: []string{"alice"}, State: "open", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if concise.View != "compact" || concise.PullRequests[0].HeadSHA != "" || len(concise.PullRequests[0].Facets) != 0 {
		t.Fatalf("concise portfolio leaked detailed fields: %+v", concise)
	}
	detailedJSON, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	conciseJSON, err := json.Marshal(concise)
	if err != nil {
		t.Fatal(err)
	}
	if len(conciseJSON) >= len(detailedJSON) {
		t.Fatalf("concise portfolio is not smaller: concise=%d detailed=%d", len(conciseJSON), len(detailedJSON))
	}
}

func TestPullRequestPortfolioClassifiesClosedUnmerged(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)
	now := time.Unix(1000, 0).UTC()
	svc.SetClock(func() time.Time { return now })
	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: "acme", Name: "rocket"}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := svc.corpus.UpsertThread(ctx, corpus.Thread{RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 9, State: "closed", Title: "abandoned change", Author: "alice", Merge: domain.UnmergedStatus(), SourceUpdatedAt: now}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := svc.corpus.UpsertThread(ctx, corpus.Thread{RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 10, State: "closed", Title: "header only", Author: "alice", SourceUpdatedAt: now}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	out, err := (&MCPReader{svc}).ListPullRequestPortfolio(ctx, mcpcontract.ListPullRequestPortfolioInput{Authors: []string{"alice"}, State: "closed", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.PullRequests) != 2 {
		t.Fatalf("closed pull request classification = %+v", out.PullRequests)
	}
	byNumber := map[int]mcpcontract.PullRequestPortfolioItem{}
	for _, item := range out.PullRequests {
		byNumber[item.Number] = item
	}
	if byNumber[thread.Number].Attention != "closed_unmerged" || byNumber[unknown.Number].Attention != "unknown" || !strings.Contains(byNumber[unknown.Number].Reasons[0], "merge state has not been observed") {
		t.Fatalf("closed pull request classification = %+v", out.PullRequests)
	}
}

func TestPullRequestPortfolioKeepsComputingMergeabilityUnknown(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)
	now := time.Unix(1000, 0).UTC()
	svc.SetClock(func() time.Time { return now })
	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: "acme", Name: "rocket"}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := svc.corpus.UpsertThread(ctx, corpus.Thread{RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 10, State: "open", Title: "computing", Author: "alice", SourceUpdatedAt: now}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	unknownMergeability := "UNKNOWN"
	values := map[string]any{
		FacetPRDetails:       github.PullRequestDetails{Number: 10, UpdatedAt: now},
		FacetPRReviews:       []github.Review{},
		FacetPRMergeState:    github.NewPullRequestMergeState("UNKNOWN", &unknownMergeability),
		FacetPRMergeQueue:    (*github.PullRequestMergeQueueEntry)(nil),
		FacetPRChecks:        []github.PullRequestCheck{},
		FacetPRReviewThreads: []github.PullRequestReviewThread{},
		FacetPRClosingIssues: []github.PullRequestClosingIssue{},
		FacetPRFiles:         []github.PullRequestFile{},
	}
	for facet, value := range values {
		payload, _ := json.Marshal(value)
		if err := svc.corpus.ApplyFacetObservationSet(ctx, repo.ID, &thread.ID, facet, now, []corpus.FacetObservationInput{{SourceUpdatedAt: now, Payload: string(payload)}}, true, 0); err != nil {
			t.Fatal(err)
		}
	}
	out, err := (&MCPReader{svc}).ListPullRequestPortfolio(ctx, mcpcontract.ListPullRequestPortfolioInput{Authors: []string{"alice"}, State: "open", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.PullRequests) != 1 || out.PullRequests[0].Attention != "unknown" || !strings.Contains(strings.Join(out.PullRequests[0].Reasons, " "), "mergeability is still computing") {
		t.Fatalf("portfolio = %+v", out)
	}
}

func TestPullRequestPortfolioExactSelectionDoesNotSubstituteNewerPullRequests(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)
	now := time.Unix(1000, 0).UTC()
	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: "acme", Name: "rocket"}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.corpus.UpsertThread(ctx, corpus.Thread{RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 1, State: "open", Title: "selected", Author: "alice", SourceUpdatedAt: now}, `{}`); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.corpus.UpsertThread(ctx, corpus.Thread{RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 2, State: "open", Title: "newer unrelated", Author: "alice", SourceUpdatedAt: now.Add(time.Second)}, `{}`); err != nil {
		t.Fatal(err)
	}
	out, err := (&MCPReader{svc}).ListPullRequestPortfolio(ctx, mcpcontract.ListPullRequestPortfolioInput{
		PullRequests: []mcpcontract.ThreadRef{{Owner: "acme", Repo: "rocket", Kind: "pull_request", Number: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "partial" || len(out.PullRequests) != 1 || out.PullRequests[0].Number != 1 || len(out.UnavailablePullRequests) != 0 {
		t.Fatalf("exact portfolio = %+v", out)
	}
}

func TestPullRequestPortfolioRepositoryScopePreservesTotalAndTruncationRecovery(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)
	now := time.Unix(1000, 0).UTC()
	selected, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: "acme", Name: "rocket"}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	other, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: "acme", Name: "other"}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, thread := range []corpus.Thread{
		{RepositoryID: selected.ID, Kind: corpus.ThreadKindPullRequest, Number: 1, State: "open", Author: "alice", Title: "older selected", SourceUpdatedAt: now},
		{RepositoryID: selected.ID, Kind: corpus.ThreadKindPullRequest, Number: 2, State: "open", Author: "alice", Title: "newer selected", SourceUpdatedAt: now.Add(time.Second)},
		{RepositoryID: other.ID, Kind: corpus.ThreadKindPullRequest, Number: 3, State: "open", Author: "alice", Title: "newest other repository", SourceUpdatedAt: now.Add(2 * time.Second)},
	} {
		if _, err := svc.corpus.UpsertThread(ctx, thread, `{}`); err != nil {
			t.Fatal(err)
		}
	}
	scope := &mcpcontract.RepositoryRef{Owner: "acme", Repo: "rocket"}
	out, err := (&MCPReader{svc}).ListPullRequestPortfolio(ctx, mcpcontract.ListPullRequestPortfolioInput{Repository: scope, Authors: []string{"alice"}, State: "open", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if out.Total != 2 || !out.Truncated || len(out.PullRequests) != 1 || out.PullRequests[0].Repo != "rocket" || out.PullRequests[0].Number != 2 {
		t.Fatalf("scoped portfolio = %+v", out)
	}
	if out.Recovery == nil || len(out.Recovery.Then) != 1 {
		t.Fatalf("scoped truncation recovery = %+v", out.Recovery)
	}
	next, ok := mcpcontract.RecoveryInput[mcpcontract.ListPullRequestPortfolioInput](out.Recovery.Then[0])
	if !ok || next.Repository == nil || *next.Repository != *scope {
		t.Fatalf("scoped truncation recovery = %+v", out.Recovery)
	}
	if _, err := (&MCPReader{svc}).ListPullRequestPortfolio(ctx, mcpcontract.ListPullRequestPortfolioInput{Repository: scope, PullRequests: []mcpcontract.ThreadRef{{Owner: "acme", Repo: "rocket", Kind: "pull_request", Number: 1}}}); err == nil {
		t.Fatal("explicit portfolio selection accepted repository scope")
	}
}

func TestPullRequestPortfolioRejectsDuplicateDefaultKindReferences(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)
	_, err := (&MCPReader{svc}).ListPullRequestPortfolio(ctx, mcpcontract.ListPullRequestPortfolioInput{
		PullRequests: []mcpcontract.ThreadRef{
			{Owner: "acme", Repo: "rocket", Number: 7},
			{Owner: "acme", Repo: "rocket", Kind: "pull_request", Number: 7},
		},
	})
	if err == nil {
		t.Fatal("expected duplicate pull-request references to be rejected")
	}
}
