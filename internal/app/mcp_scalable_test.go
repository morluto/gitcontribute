package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/deepwiki"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpserver"
)

func TestGetRepositoriesPreservesUnknownMetadataAndInputOrder(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	placeholder, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: "acme", Name: "placeholder"}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: "acme", Name: "observed", Stars: 42, SourceUpdatedAt: time.Unix(10, 0).UTC()}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.AdvanceFacet(ctx, observed.ID, nil, "metadata", observed.SourceUpdatedAt, true, 0); err != nil {
		t.Fatal(err)
	}
	out, err := (&MCPReader{svc}).GetRepositories(ctx, mcpserver.GetRepositoriesInput{Repositories: []mcpserver.RepositoryRef{{Owner: "acme", Repo: placeholder.Name}, {Owner: "acme", Repo: observed.Name}, {Owner: "acme", Repo: "missing"}}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "partial" || len(out.Items) != 3 {
		t.Fatalf("unexpected batch: %+v", out)
	}
	if got := out.Items[0].Value; got == nil || got.Metadata.Status != "missing" || got.Stars != nil {
		t.Fatalf("placeholder exposed false facts: %+v", got)
	}
	if got := out.Items[1].Value; got == nil || got.Metadata.Status != "complete" || got.Stars == nil || *got.Stars != 42 {
		t.Fatalf("observed metadata missing: %+v", got)
	}
	if out.Items[2].Key != "acme/missing" || out.Items[2].Status != "unavailable" {
		t.Fatalf("missing item = %+v", out.Items[2])
	}
}

type fakeRepositorySearchReader struct {
	github.Reader
	result  github.RepositorySearchResult
	options github.RepositorySearchOptions
}

func (f *fakeRepositorySearchReader) SearchRepositories(_ context.Context, options github.RepositorySearchOptions) (github.RepositorySearchResult, error) {
	f.options = options
	return f.result, nil
}

func TestSearchGitHubRepositoriesPersistsObservedMetadata(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	now := time.Unix(1000, 0).UTC()
	remote := github.Repository{Owner: "acme", Name: "rocket", Description: "fast inference", Stars: 9001, Language: "Go", UpdatedAt: now}
	reader := &fakeRepositorySearchReader{result: github.RepositorySearchResult{Total: 321, Items: []github.Repository{remote}}}
	svc.SetGitHubReader(reader)

	out, err := (&MCPReader{svc}).SearchGitHubRepositories(ctx, mcpserver.SearchGitHubRepositoriesInput{Query: "inference language:go", Sort: "stars", Order: "desc", Limit: 12})
	if err != nil {
		t.Fatal(err)
	}
	if reader.options.PerPage != 12 || reader.options.Sort != "stars" || len(out.Items) != 1 || out.Items[0].Value == nil || *out.Items[0].Value.Stars != 9001 {
		t.Fatalf("live search result = %+v, options = %+v", out, reader.options)
	}
	stored, err := (&MCPReader{svc}).GetRepositories(ctx, mcpserver.GetRepositoriesInput{Repositories: []mcpserver.RepositoryRef{{Owner: "acme", Repo: "rocket"}}})
	if err != nil {
		t.Fatal(err)
	}
	if stored.Items[0].Value == nil || stored.Items[0].Value.Metadata.Status != "complete" || *stored.Items[0].Value.Stars != 9001 {
		t.Fatalf("search metadata was not persisted: %+v", stored)
	}
}

func TestFindPrecedentsUsesClosedAndMergedHistory(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: "acme", Name: "rocket"}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	threads := []corpus.Thread{
		{RepositoryID: repo.ID, Kind: corpus.ThreadKindIssue, Number: 1, State: "open", Title: "cache path ignores configured root", Body: "compiled cache artifacts use tmp", SourceUpdatedAt: time.Unix(30, 0).UTC()},
		{RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 2, State: "closed", Title: "honor configured cache root", Body: "move compiled cache artifacts out of tmp", Merged: true, MergedAt: time.Unix(20, 0).UTC(), ClosedAt: time.Unix(20, 0).UTC(), SourceUpdatedAt: time.Unix(20, 0).UTC()},
		{RepositoryID: repo.ID, Kind: corpus.ThreadKindIssue, Number: 3, State: "open", Title: "unrelated typo", Body: "docs", SourceUpdatedAt: time.Unix(10, 0).UTC()},
	}
	for _, thread := range threads {
		if _, err := svc.corpus.UpsertThread(ctx, thread, `{}`); err != nil {
			t.Fatal(err)
		}
	}
	out, err := (&MCPReader{svc}).FindPrecedents(ctx, mcpserver.FindPrecedentsInput{Threads: []mcpserver.ThreadRef{{Owner: "acme", Repo: "rocket", Number: 1}}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if out.Total != 1 || out.Items[0].Value == nil || (*out.Items[0].Value)[0].Ref != "acme/rocket#2" {
		t.Fatalf("unexpected precedents: %+v", out)
	}
	if reasons := (*out.Items[0].Value)[0].Reasons; len(reasons) < 2 || reasons[1] != "pull request merged" {
		t.Fatalf("missing merged evidence: %v", reasons)
	}
}

type fakeDeepWikiReader struct {
	response deepwiki.Response
	request  deepwiki.Request
}

func (f *fakeDeepWikiReader) Read(_ context.Context, request deepwiki.Request) (deepwiki.Response, error) {
	f.request = request
	return f.response, nil
}

func TestDeepWikiReturnsDerivedProvenanceAndBoundsOutput(t *testing.T) {
	svc := newSearchTestService(t)
	fake := &fakeDeepWikiReader{response: deepwiki.Response{Available: true, Text: strings.Repeat("x", 2048), SourceURL: "https://deepwiki.com/acme/rocket"}}
	svc.SetDeepWikiReader(fake)
	out, err := (&MCPReader{svc}).DeepWiki(context.Background(), mcpserver.DeepWikiInput{Action: "question", Repositories: []string{"acme/rocket"}, Question: "architecture?", MaxOutputBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if out.Provenance != "derived_external" || !out.Truncated || len(out.Result) != 1024 {
		t.Fatalf("unexpected DeepWiki result: %+v", out)
	}
}

func TestDeepWikiTruncationPreservesUTF8(t *testing.T) {
	svc := newSearchTestService(t)
	fake := &fakeDeepWikiReader{response: deepwiki.Response{Available: true, Text: strings.Repeat("x", 1023) + "€", SourceURL: "https://deepwiki.com/acme/rocket"}}
	svc.SetDeepWikiReader(fake)
	out, err := (&MCPReader{svc}).DeepWiki(context.Background(), mcpserver.DeepWikiInput{Action: "contents", Repository: "acme/rocket", MaxOutputBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Truncated || len(out.Result) > 1024 || !utf8.ValidString(out.Result) {
		t.Fatalf("invalid bounded UTF-8 result: bytes=%d valid=%v truncated=%v", len(out.Result), utf8.ValidString(out.Result), out.Truncated)
	}
}

func TestScalableRuntimeBoundsMatchSchemas(t *testing.T) {
	reader := &MCPReader{newSearchTestService(t)}
	if _, err := reader.RankOpportunities(context.Background(), mcpserver.RankOpportunitiesInput{Repositories: []mcpserver.RepositoryRef{{Owner: "acme", Repo: "rocket"}}, Limit: 101}); err == nil {
		t.Fatal("rank opportunities accepted limit above schema maximum")
	}
	if _, err := reader.FindPrecedents(context.Background(), mcpserver.FindPrecedentsInput{Threads: []mcpserver.ThreadRef{{Owner: "acme", Repo: "rocket", Number: 1}}, Limit: 101}); err == nil {
		t.Fatal("find precedents accepted limit above schema maximum")
	}
	if _, err := reader.DeepWiki(context.Background(), mcpserver.DeepWikiInput{Action: "question", Repositories: []string{"acme/rocket"}, Question: "architecture?", MaxOutputBytes: 100}); err == nil {
		t.Fatal("DeepWiki accepted max_output_bytes below schema minimum")
	}
}

func TestPullRequestPortfolioDerivesConflictAndPreservesUnknownCoverage(t *testing.T) {
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
	out, err := (&MCPReader{svc}).ListPullRequestPortfolio(ctx, mcpserver.ListPullRequestPortfolioInput{Author: "alice", State: "open", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "partial" || len(out.PullRequests) != 2 {
		t.Fatalf("unexpected portfolio: %+v", out)
	}
	byNumber := map[int]mcpserver.PullRequestPortfolioItem{}
	for _, item := range out.PullRequests {
		byNumber[item.Number] = item
	}
	if byNumber[conflicted.Number].Attention != "conflicted" || byNumber[conflicted.Number].HeadSHA != "head" {
		t.Fatalf("conflict not derived: %+v", byNumber[conflicted.Number])
	}
	if byNumber[unknown.Number].Attention != "unknown" || byNumber[unknown.Number].StatusCoverage != "missing" {
		t.Fatalf("unknown coverage collapsed: %+v", byNumber[unknown.Number])
	}
}

func TestPullRequestPortfolioClassifiesClosedUnmerged(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	now := time.Unix(1000, 0).UTC()
	svc.SetClock(func() time.Time { return now })
	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: "acme", Name: "rocket"}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := svc.corpus.UpsertThread(ctx, corpus.Thread{RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 9, State: "closed", Title: "abandoned change", Author: "alice", SourceUpdatedAt: now}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	out, err := (&MCPReader{svc}).ListPullRequestPortfolio(ctx, mcpserver.ListPullRequestPortfolioInput{Author: "alice", State: "closed", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.PullRequests) != 1 || out.PullRequests[0].Number != thread.Number || out.PullRequests[0].Attention != "closed_unmerged" {
		t.Fatalf("closed pull request classification = %+v", out.PullRequests)
	}
}
