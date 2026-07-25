package app

import (
	"context"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

type exactHydrationReader struct {
	*fakeHydrationReader
	header   github.Issue
	getCalls int
}

func (r *exactHydrationReader) GetIssue(context.Context, string, string, int) (github.Issue, github.RateInfo, error) {
	r.getCalls++
	return r.header, github.RateInfo{}, nil
}

func TestHydrateRefreshesStaleThreadHeaderBeforeEmptyFacet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()

	repo, thread := seedRepoAndThread(t, svc, corpus.ThreadKindIssue, 223)
	current := thread.SourceUpdatedAt.Add(24 * time.Hour)
	reader := &exactHydrationReader{
		fakeHydrationReader: &fakeHydrationReader{issueCommentsPages: [][]github.IssueComment{{}}},
		header: github.Issue{
			RepositoryOwner: "owner",
			RepositoryName:  "repo",
			Number:          223,
			Kind:            corpus.ThreadKindIssue,
			State:           "open",
			Title:           "current title",
			UpdatedAt:       current,
		},
	}
	svc.SetGitHubReader(reader)

	result, err := svc.Hydrate(ctx, contracts.RepoRef{Owner: "owner", Repo: "repo"}, 223, contracts.HydrateOptions{Facets: []string{FacetIssueComments}})
	if err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	if reader.getCalls != 1 || result.Requests != 2 {
		t.Fatalf("header calls = %d, requests = %d", reader.getCalls, result.Requests)
	}
	stored, err := svc.corpus.GetThreadByNumber(ctx, repo.ID, 223)
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil || !stored.SourceUpdatedAt.Equal(current) || stored.Title != "current title" {
		t.Fatalf("stored thread = %+v", stored)
	}
	coverage, err := svc.corpus.GetCoverage(ctx, repo.ID, &thread.ID, FacetIssueComments)
	if err != nil {
		t.Fatal(err)
	}
	if coverage == nil || !coverage.Complete || !coverage.SourceUpdatedAt.Equal(current) {
		t.Fatalf("coverage = %+v", coverage)
	}
}

func TestHydrateFetchesMissingExactThreadHeader(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()

	repo, _ := seedRepoAndThread(t, svc, corpus.ThreadKindIssue, 1)
	current := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	reader := &exactHydrationReader{
		fakeHydrationReader: &fakeHydrationReader{issueCommentsPages: [][]github.IssueComment{{}}},
		header: github.Issue{
			RepositoryOwner: "owner",
			RepositoryName:  "repo",
			Number:          224,
			Kind:            corpus.ThreadKindIssue,
			State:           "open",
			Title:           "new issue",
			UpdatedAt:       current,
		},
	}
	svc.SetGitHubReader(reader)

	result, err := svc.Hydrate(ctx, contracts.RepoRef{Owner: "owner", Repo: "repo"}, 224, contracts.HydrateOptions{Facets: []string{FacetIssueComments}})
	if err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	if result.Number != 224 || result.Requests != 2 {
		t.Fatalf("result = %+v", result)
	}
	stored, err := svc.corpus.GetThreadByNumber(ctx, repo.ID, 224)
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil || stored.Title != "new issue" || !stored.SourceUpdatedAt.Equal(current) {
		t.Fatalf("stored thread = %+v", stored)
	}
}

func TestHydrateThreadsReportsHeaderRefresh(t *testing.T) {
	t.Parallel()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()
	seedRepoAndThread(t, svc, corpus.ThreadKindIssue, 1)
	svc.SetGitHubReader(&exactHydrationReader{
		fakeHydrationReader: &fakeHydrationReader{issueCommentsPages: [][]github.IssueComment{{}}},
		header: github.Issue{
			RepositoryOwner: "owner", RepositoryName: "repo", Number: 224,
			Kind: corpus.ThreadKindIssue, State: "open", Title: "new issue",
			UpdatedAt: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
		},
	})

	out, err := svc.hydrateThreadsBatch(context.Background(), mcpcontract.HydrateThreadsInput{
		Threads: []mcpcontract.ThreadRef{{Owner: "owner", Repo: "repo", Number: 224}},
		Facets:  []string{FacetIssueComments},
	}, func(string, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	items, ok := out["items"].([]map[string]any)
	if !ok || len(items) != 1 || items[0]["header_refreshed"] != true || items[0]["requests"] != 2 {
		t.Fatalf("hydration result = %+v", out)
	}
}
