package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

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
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)
	now := time.Unix(1000, 0).UTC()
	remote := github.Repository{Owner: "acme", Name: "rocket", Description: "fast inference", Stars: 9001, Language: "Go", UpdatedAt: now}
	reader := &fakeRepositorySearchReader{result: github.RepositorySearchResult{Total: 321, Items: []github.Repository{remote}, Page: github.PageInfo{Page: 2, NextPage: 3, HasNext: true}}}
	svc.SetGitHubReader(reader)

	out, err := (&MCPReader{svc}).SearchGitHubRepositories(ctx, mcpcontract.SearchGitHubRepositoriesInput{Text: "fast inference", MatchFields: []string{"name", "description"}, Topics: []string{"llm-inference"}, Language: "Go", StarsMin: ptr(200), PushedAfter: "2026-06-15", Archived: ptr(false), Fork: ptr(false), Sort: "stars", Order: "desc", Limit: 12, Page: 2, ResponseFormat: "concise"})
	if err != nil {
		t.Fatal(err)
	}
	if reader.options.PerPage != 12 || reader.options.Page != 2 || reader.options.Sort != "stars" || reader.options.Query != `"fast inference" in:name,description topic:llm-inference language:Go stars:>=200 pushed:>=2026-06-15 archived:false fork:false` {
		t.Fatalf("compiled options = %+v", reader.options)
	}
	if out.NextPage != 3 || out.ResponseFormat != "concise" || len(out.Items) != 1 || out.Items[0].Value == nil || out.Items[0].Value.Ref != "repository:acme/rocket" || *out.Items[0].Value.Stars != 9001 {
		t.Fatalf("live search result = %+v, options = %+v", out, reader.options)
	}
	if out.Items[0].Value.Watchers != nil || len(out.RecoveryPlans) != 1 || len(out.RecoveryPlans[0].Then) != 1 || out.RecoveryPlans[0].Then[0].Type() != "sync_threads" {
		t.Fatalf("concise search context = %+v", out)
	}
	if out.Items[0].Value.DossierStatus != "missing" {
		t.Fatalf("new search result dossier availability = %+v", out.Items[0].Value)
	}
	stored, err := (&MCPReader{svc}).GetRepositories(ctx, mcpcontract.GetRepositoriesInput{Repositories: []mcpcontract.RepositoryRef{{Owner: "acme", Repo: "rocket"}}})
	if err != nil {
		t.Fatal(err)
	}
	if stored.Items[0].Value == nil || stored.Items[0].Value.Metadata.Status != "complete" || *stored.Items[0].Value.Stars != 9001 {
		t.Fatalf("search metadata was not persisted: %+v", stored)
	}
	if _, err := svc.BuildRepositoryDossier(ctx, contracts.RepoRef{Owner: "acme", Repo: "rocket"}); err != nil {
		t.Fatal(err)
	}
	out, err = (&MCPReader{svc}).SearchGitHubRepositories(ctx, mcpcontract.SearchGitHubRepositoriesInput{Text: "fast inference", Limit: 12, Page: 2})
	if err != nil {
		t.Fatal(err)
	}
	if out.Items[0].Value == nil || out.Items[0].Value.DossierStatus != "available" || out.Items[0].Value.DossierAsOf == "" {
		t.Fatalf("live search did not report local dossier availability: %+v", out)
	}
}

func TestCompileRepositorySearchRejectsAmbiguousAndInvalidInputs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   mcpcontract.SearchGitHubRepositoriesInput
	}{
		{name: "empty", in: mcpcontract.SearchGitHubRepositoriesInput{}},
		{name: "raw and structured", in: mcpcontract.SearchGitHubRepositoriesInput{RawQuery: "cuda", Language: "Go"}},
		{name: "unknown match field", in: mcpcontract.SearchGitHubRepositoriesInput{Text: "cuda", MatchFields: []string{"topics"}}},
		{name: "reversed stars", in: mcpcontract.SearchGitHubRepositoriesInput{Text: "cuda", StarsMin: ptr(20), StarsMax: ptr(10)}},
		{name: "invalid date", in: mcpcontract.SearchGitHubRepositoriesInput{PushedAfter: "yesterday"}},
		{name: "reversed dates", in: mcpcontract.SearchGitHubRepositoriesInput{CreatedAfter: "2026-07-01", CreatedBefore: "2026-06-01"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, _, err := compileRepositorySearch(tc.in); err == nil {
				t.Fatal("invalid search was accepted")
			}
		})
	}
}

func TestCompileRepositorySearchPreservesExplicitZeroStarBound(t *testing.T) {
	t.Parallel()
	zero := 0
	query, _, _, err := compileRepositorySearch(mcpcontract.SearchGitHubRepositoriesInput{StarsMax: &zero})
	if err != nil {
		t.Fatal(err)
	}
	if query != "stars:<=0" {
		t.Fatalf("query = %q, want stars:<=0", query)
	}
}

func TestRepositorySearchValidationExamplesAreUsable(t *testing.T) {
	t.Parallel()
	_, _, _, err := compileRepositorySearch(mcpcontract.SearchGitHubRepositoriesInput{})
	var toolErr *mcpcontract.ToolError
	if !errors.As(err, &toolErr) {
		t.Fatalf("error = %v, want ToolError", err)
	}
	if toolErr.Example["text"] != "GitHub contribution research" || !reflect.DeepEqual(toolErr.Example["match_fields"], []string{"name", "description"}) {
		t.Fatalf("empty-search example = %#v", toolErr.Example)
	}

	_, _, _, err = compileRepositorySearch(mcpcontract.SearchGitHubRepositoriesInput{RawQuery: "language:go", Language: "Go"})
	if !errors.As(err, &toolErr) || toolErr.Example["raw_query"] != "is:public language:go stars:>=100" {
		t.Fatalf("ambiguous-search example = %#v, error=%v", toolErr.Example, err)
	}
}

func TestCompileRepositorySearchWarnsAboutRawReadmeQueries(t *testing.T) {
	t.Parallel()
	query, interpretation, warnings, err := compileRepositorySearch(mcpcontract.SearchGitHubRepositoriesInput{RawQuery: "attention in:readme"})
	if err != nil {
		t.Fatal(err)
	}
	if query != "attention in:readme" || !strings.Contains(interpretation, "advanced raw query") || len(warnings) != 1 || warnings[0].Code != "broad_readme_match" {
		t.Fatalf("raw query context = %q %q %+v", query, interpretation, warnings)
	}
}

func TestCompileRepositorySearchWarnsAboutStructuredReadmeMatching(t *testing.T) {
	t.Parallel()
	query, _, warnings, err := compileRepositorySearch(mcpcontract.SearchGitHubRepositoriesInput{Text: "attention", MatchFields: []string{"name", "readme"}})
	if err != nil {
		t.Fatal(err)
	}
	if query != "attention in:name,readme" || len(warnings) != 1 || warnings[0].Code != "broad_readme_match" {
		t.Fatalf("structured README warning = %q %+v", query, warnings)
	}
}

func TestRepositorySearchDetailedFormatPreservesSecondaryFacts(t *testing.T) {
	t.Parallel()
	archived := true
	remote := github.Repository{Owner: "acme", Name: "rocket", Description: "fast", Stars: 42, Watchers: 9, Forks: 3, OpenIssues: 7, Archived: archived, Topics: []string{"cuda"}}
	match := liveRepositorySearchMatch(remote, mcpcontract.RepositoryMetadataOutput{Status: "complete"}, detailedResponse)
	if match.Ref != "repository:acme/rocket" || match.Watchers == nil || *match.Watchers != 9 || match.Archived == nil || !*match.Archived || len(match.Topics) != 1 {
		t.Fatalf("detailed match = %+v", match)
	}
}
