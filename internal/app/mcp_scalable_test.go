package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/deepwiki"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpserver"
	"github.com/morluto/gitcontribute/internal/radar"
)

func TestRadarCandidateToMCPPreservesRelatedWorkSemantics(t *testing.T) {
	out := radarCandidateToMCP(radar.Candidate{RelatedWork: []radar.RelatedWork{{
		Ref: "pull_request:owner/repo#9", Relation: "depends_on", Direction: "outbound", State: "open",
	}}})
	if len(out.RelatedWork) != 1 || out.RelatedWork[0].Ref != "pull_request:owner/repo#9" || out.RelatedWork[0].Relation != "depends_on" || out.RelatedWork[0].Direction != "outbound" || out.RelatedWork[0].State != "open" {
		t.Fatalf("related work = %+v", out.RelatedWork)
	}
}

func TestRankOpportunitiesReportsBoundedNonPaginatedTruncation(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	svc.SetGitHubReader(panicRadarReader{})
	seedRadarRepository(ctx, t, svc, "rocket", 5, now)

	reader := &MCPReader{svc}
	bounded, err := reader.RankOpportunities(ctx, mcpserver.RankOpportunitiesInput{
		Repositories: []mcpserver.RepositoryRef{{Owner: "acme", Repo: "rocket"}},
		Limit:        2, MaxResultsPerRepository: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if bounded.Total != 5 || len(bounded.Candidates) != 2 || !bounded.Truncated {
		t.Fatalf("bounded radar result = %+v", bounded)
	}
	if summary := bounded.Repositories[0].Value; summary == nil || summary.Considered != 5 || summary.Returned != 5 || summary.Truncated || summary.PopulationCapped {
		t.Fatalf("bounded repository summary = %+v", summary)
	}
	perRepositoryBound, err := reader.RankOpportunities(ctx, mcpserver.RankOpportunitiesInput{
		Repositories: []mcpserver.RepositoryRef{{Owner: "acme", Repo: "rocket"}}, Limit: 100, MaxResultsPerRepository: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if perRepositoryBound.Total != 5 || len(perRepositoryBound.Candidates) != 3 || !perRepositoryBound.Truncated {
		t.Fatalf("per-repository bounded result = %+v", perRepositoryBound)
	}
	if summary := perRepositoryBound.Repositories[0].Value; summary == nil || summary.Considered != 5 || summary.Returned != 3 || !summary.Truncated {
		t.Fatalf("per-repository bounded summary = %+v", summary)
	}
	full, err := reader.RankOpportunities(ctx, mcpserver.RankOpportunitiesInput{
		Repositories: []mcpserver.RepositoryRef{{Owner: "acme", Repo: "rocket"}}, Limit: 100, MaxResultsPerRepository: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if full.Total != 5 || len(full.Candidates) != 5 || full.Truncated {
		t.Fatalf("full radar result = %+v", full)
	}
	assertRadarCandidateRanks(t, full.Candidates)
	for i := range bounded.Candidates {
		if bounded.Candidates[i].Ref != full.Candidates[i].Ref {
			t.Fatalf("bounded order = %+v, full order = %+v", bounded.Candidates, full.Candidates)
		}
	}
}

func seedRadarRepository(ctx context.Context, t *testing.T, svc *Service, name string, candidates int, now time.Time) {
	t.Helper()
	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: "acme", Name: name, SourceUpdatedAt: now}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	for number := 1; number <= candidates; number++ {
		if _, err := svc.corpus.UpsertThread(ctx, corpus.Thread{
			RepositoryID: repo.ID, Kind: corpus.ThreadKindIssue, Number: number, State: "open",
			Title: "same-score candidate", SourceUpdatedAt: now.Add(-time.Hour),
		}, `{}`); err != nil {
			t.Fatal(err)
		}
	}
}

func assertRadarCandidateRanks(t *testing.T, candidates []mcpserver.OpportunityCandidateOutput) {
	t.Helper()
	for index, candidate := range candidates {
		if candidate.Rank != index+1 {
			t.Fatalf("candidate %s rank = %d, want %d", candidate.Ref, candidate.Rank, index+1)
		}
	}
}

func TestRankOpportunitiesUsesOneEvaluationTimeAcrossRepositories(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	clockCalls := 0
	svc.SetClock(func() time.Time {
		clockCalls++
		return now.Add(time.Duration(clockCalls) * time.Hour)
	})
	for _, name := range []string{"one", "two"} {
		seedRadarRepository(ctx, t, svc, name, 1, now)
	}
	out, err := (&MCPReader{svc}).RankOpportunities(ctx, mcpserver.RankOpportunitiesInput{
		Repositories: []mcpserver.RepositoryRef{{Owner: "acme", Repo: "one"}, {Owner: "acme", Repo: "two"}},
		Limit:        2, MaxResultsPerRepository: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if clockCalls != 1 || out.GeneratedAt != now.Add(time.Hour).Format(time.RFC3339) || len(out.Candidates) != 2 {
		t.Fatalf("cross-repository evaluation = calls:%d output:%+v", clockCalls, out)
	}
}

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

func TestGetCoveragePreservesTargetOrderAndMissingItems(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: "acme", Name: "rocket", SourceUpdatedAt: time.Unix(10, 0).UTC()}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := svc.corpus.UpsertThread(ctx, corpus.Thread{RepositoryID: repo.ID, Kind: corpus.ThreadKindIssue, Number: 7, State: "open", Title: "bounded coverage", SourceUpdatedAt: time.Unix(20, 0).UTC()}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.AdvanceFacet(ctx, repo.ID, nil, "metadata", time.Unix(11, 0).UTC(), true, 0); err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.AdvanceFacet(ctx, repo.ID, &thread.ID, "comments", time.Unix(21, 0).UTC(), false, 0); err != nil {
		t.Fatal(err)
	}

	out, err := (&MCPReader{svc}).GetCoverage(ctx, mcpserver.GetCoverageInput{Targets: []mcpserver.CoverageTarget{
		{Owner: "acme", Repo: "rocket"},
		{Owner: "acme", Repo: "missing"},
		{Owner: "acme", Repo: "rocket", Kind: "issue", Number: 7},
		{Owner: "acme", Repo: "rocket", Kind: "issue"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "partial" || len(out.Items) != 4 {
		t.Fatalf("coverage batch = %+v", out)
	}
	if out.Items[0].Key != "acme/rocket" || out.Items[0].Value == nil || out.Items[0].Value.Facets[0].Facet != "metadata" {
		t.Fatalf("repository coverage = %+v", out.Items[0])
	}
	if out.Items[1].Key != "acme/missing" || out.Items[1].Status != "unavailable" || out.Items[1].Reason != "not_indexed" {
		t.Fatalf("missing coverage = %+v", out.Items[1])
	}
	if out.Items[2].Value == nil || out.Items[2].Value.Kind != "issue" || out.Items[2].Value.Number != 7 || out.Items[2].Value.Facets[0].Status != "incomplete" {
		t.Fatalf("thread coverage = %+v", out.Items[2])
	}
	if out.Items[3].Status != "unavailable" || out.Items[3].Reason != "invalid_reference" {
		t.Fatalf("invalid coverage target = %+v", out.Items[3])
	}
}

func TestGetThreadsPreservesUnknownAndObservedFalseMergeState(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: "acme", Name: "rocket"}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, thread := range []corpus.Thread{
		{RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 1, State: "closed", Title: "unknown", SourceUpdatedAt: time.Unix(1, 0).UTC()},
		{RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 2, State: "closed", Title: "observed false", MergedKnown: true, SourceUpdatedAt: time.Unix(2, 0).UTC()},
	} {
		if _, err := svc.corpus.UpsertThread(ctx, thread, `{}`); err != nil {
			t.Fatal(err)
		}
	}
	out, err := (&MCPReader{svc}).GetThreads(ctx, mcpserver.GetThreadsInput{View: "compact", Threads: []mcpserver.ThreadRef{
		{Owner: "acme", Repo: "rocket", Kind: "pull_request", Number: 1},
		{Owner: "acme", Repo: "rocket", Kind: "pull_request", Number: 2},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Items[0].Value == nil || out.Items[0].Value.Merged != nil {
		t.Fatalf("unknown merge output = %+v", out.Items[0])
	}
	if out.Items[1].Value == nil || out.Items[1].Value.Merged == nil || *out.Items[1].Value.Merged {
		t.Fatalf("observed false merge output = %+v", out.Items[1])
	}
}

func TestCancelJobsPreservesOrderAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	if _, err := svc.Jobs(ctx); err != nil {
		t.Fatal(err)
	}
	queued, err := svc.corpus.CreateJob(ctx, "sync", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := svc.corpus.CreateJob(ctx, "sync", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.StartJob(ctx, terminal.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.TransitionJob(ctx, terminal.ID, corpus.JobStatusRunning, corpus.JobStatusSucceeded, `{}`, ""); err != nil {
		t.Fatal(err)
	}
	running, err := svc.corpus.CreateJob(ctx, "sync", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.StartJob(ctx, running.ID); err != nil {
		t.Fatal(err)
	}

	reader := &MCPReader{svc}
	out, err := reader.CancelJobs(ctx, mcpserver.CancelJobInput{IDs: []string{queued.ID, "missing-job", running.ID, terminal.ID, queued.ID, " "}})
	if err != nil {
		t.Fatal(err)
	}
	assertCancelJobsOutput(t, out, queued.ID)
}

func TestMCPSourceRefsToDomainRejectsInvalidTimestamps(t *testing.T) {
	for _, tc := range []struct {
		name string
		ref  mcpserver.SourceRef
		want string
	}{
		{name: "observed at", ref: mcpserver.SourceRef{ObservedAt: "not-a-date"}, want: "source_refs[0].observed_at"},
		{name: "as of", ref: mcpserver.SourceRef{AsOf: "not-a-date"}, want: "source_refs[0].as_of"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := mcpSourceRefsToDomain([]mcpserver.SourceRef{tc.ref})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want field path %q", err, tc.want)
			}
		})
	}

	refs, err := mcpSourceRefsToDomain([]mcpserver.SourceRef{{ObservedAt: "2026-07-21T00:00:00Z"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].ObservedAt.IsZero() || !refs[0].AsOf.IsZero() {
		t.Fatalf("source refs = %+v", refs)
	}
}

func assertCancelJobsOutput(t *testing.T, out mcpserver.GetJobsOutput, queuedID string) {
	t.Helper()
	if out.Status != "partial" || len(out.Items) != 6 {
		t.Fatalf("cancellation batch = %+v", out)
	}
	if out.Items[0].Key != queuedID || out.Items[0].Value == nil || out.Items[0].Value.Status != "cancelled" {
		t.Fatalf("queued cancellation = %+v", out.Items[0])
	}
	if out.Items[1].Status != "unavailable" || out.Items[1].Reason != "not_found" {
		t.Fatalf("missing cancellation = %+v", out.Items[1])
	}
	if out.Items[2].Value == nil || out.Items[2].Value.Status != "running" || !out.Items[2].Value.CancellationRequested || out.Items[2].Value.RetryAfterMS != 1000 || out.Items[2].NextAction == "" {
		t.Fatalf("running cancellation = %+v", out.Items[2])
	}
	if out.Items[3].Status != "unavailable" || out.Items[3].Reason != "terminal" {
		t.Fatalf("terminal cancellation = %+v", out.Items[3])
	}
	if out.Items[4].Value == nil || out.Items[4].Value.Status != "cancelled" {
		t.Fatalf("repeated cancellation = %+v", out.Items[4])
	}
	if out.Items[5].Status != "failed" || out.Items[5].Reason != "invalid_id" {
		t.Fatalf("invalid cancellation = %+v", out.Items[5])
	}
}

func TestJobResultToMCPExposesStructuredDurableProgress(t *testing.T) {
	out, err := jobResultToMCP(&cli.JobResult{ID: "job-1", Kind: "sync_threads", Status: "running", Request: `{}`, Progress: "thread_headers", Statistics: `{"completed_items":2,"total_items":5}`, CreatedAt: "2026-07-19T00:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Phase != "thread_headers" || out.CompletedItems != 2 || out.TotalItems != 5 || out.ProgressPercent != 40 || out.RetryAfterMS != 1000 {
		t.Fatalf("structured progress = %+v", out)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"progress":`) || strings.Contains(string(encoded), `"statistics":`) {
		t.Fatalf("legacy free-form progress leaked into MCP output: %s", encoded)
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
	reader := &fakeRepositorySearchReader{result: github.RepositorySearchResult{Total: 321, Items: []github.Repository{remote}, Page: github.PageInfo{Page: 2, NextPage: 3, HasNext: true}}}
	svc.SetGitHubReader(reader)

	out, err := (&MCPReader{svc}).SearchGitHubRepositories(ctx, mcpserver.SearchGitHubRepositoriesInput{Text: "fast inference", MatchFields: []string{"name", "description"}, Topics: []string{"llm-inference"}, Language: "Go", StarsMin: 200, PushedAfter: "2026-06-15", Archived: ptr(false), Fork: ptr(false), Sort: "stars", Order: "desc", Limit: 12, Page: 2, ResponseFormat: "concise"})
	if err != nil {
		t.Fatal(err)
	}
	if reader.options.PerPage != 12 || reader.options.Page != 2 || reader.options.Sort != "stars" || reader.options.Query != `"fast inference" in:name,description topic:llm-inference language:Go stars:>=200 pushed:>=2026-06-15 archived:false fork:false` {
		t.Fatalf("compiled options = %+v", reader.options)
	}
	if out.NextPage != 3 || out.ResponseFormat != "concise" || len(out.Items) != 1 || out.Items[0].Value == nil || out.Items[0].Value.Ref != "repository:acme/rocket" || *out.Items[0].Value.Stars != 9001 {
		t.Fatalf("live search result = %+v, options = %+v", out, reader.options)
	}
	if out.Items[0].Value.Watchers != nil || len(out.SuggestedActions) != 1 || out.SuggestedActions[0].Tool != mcpserver.ToolSyncThreads {
		t.Fatalf("concise search context = %+v", out)
	}
	stored, err := (&MCPReader{svc}).GetRepositories(ctx, mcpserver.GetRepositoriesInput{Repositories: []mcpserver.RepositoryRef{{Owner: "acme", Repo: "rocket"}}})
	if err != nil {
		t.Fatal(err)
	}
	if stored.Items[0].Value == nil || stored.Items[0].Value.Metadata.Status != "complete" || *stored.Items[0].Value.Stars != 9001 {
		t.Fatalf("search metadata was not persisted: %+v", stored)
	}
}

func TestCompileRepositorySearchRejectsAmbiguousAndInvalidInputs(t *testing.T) {
	cases := []struct {
		name string
		in   mcpserver.SearchGitHubRepositoriesInput
	}{
		{name: "empty", in: mcpserver.SearchGitHubRepositoriesInput{}},
		{name: "two raw fields", in: mcpserver.SearchGitHubRepositoriesInput{Query: "cuda", RawQuery: "triton"}},
		{name: "raw and structured", in: mcpserver.SearchGitHubRepositoriesInput{RawQuery: "cuda", Language: "Go"}},
		{name: "unknown match field", in: mcpserver.SearchGitHubRepositoriesInput{Text: "cuda", MatchFields: []string{"topics"}}},
		{name: "reversed stars", in: mcpserver.SearchGitHubRepositoriesInput{Text: "cuda", StarsMin: 20, StarsMax: 10}},
		{name: "invalid date", in: mcpserver.SearchGitHubRepositoriesInput{PushedAfter: "yesterday"}},
		{name: "reversed dates", in: mcpserver.SearchGitHubRepositoriesInput{CreatedAfter: "2026-07-01", CreatedBefore: "2026-06-01"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, _, err := compileRepositorySearch(tc.in); err == nil {
				t.Fatal("invalid search was accepted")
			}
		})
	}
}

func TestCompileRepositorySearchWarnsAboutLegacyAndReadmeQueries(t *testing.T) {
	query, interpretation, warnings, err := compileRepositorySearch(mcpserver.SearchGitHubRepositoriesInput{Query: "attention in:readme"})
	if err != nil {
		t.Fatal(err)
	}
	if query != "attention in:readme" || !strings.Contains(interpretation, "legacy") || len(warnings) != 2 || warnings[0].Code != "deprecated_query" || warnings[1].Code != "broad_readme_match" {
		t.Fatalf("legacy query context = %q %q %+v", query, interpretation, warnings)
	}
}

func TestCompileRepositorySearchWarnsAboutStructuredReadmeMatching(t *testing.T) {
	query, _, warnings, err := compileRepositorySearch(mcpserver.SearchGitHubRepositoriesInput{Text: "attention", MatchFields: []string{"name", "readme"}})
	if err != nil {
		t.Fatal(err)
	}
	if query != "attention in:name,readme" || len(warnings) != 1 || warnings[0].Code != "broad_readme_match" {
		t.Fatalf("structured README warning = %q %+v", query, warnings)
	}
}

func TestRepositorySearchDetailedFormatPreservesSecondaryFacts(t *testing.T) {
	archived := true
	remote := github.Repository{Owner: "acme", Name: "rocket", Description: "fast", Stars: 42, Watchers: 9, Forks: 3, OpenIssues: 7, Archived: archived, Topics: []string{"cuda"}}
	match := liveRepositorySearchMatch(remote, mcpserver.RepositoryMetadataOutput{Status: "complete"}, "detailed")
	if match.Ref != "repository:acme/rocket" || match.Watchers == nil || *match.Watchers != 9 || match.Archived == nil || !*match.Archived || len(match.Topics) != 1 {
		t.Fatalf("detailed match = %+v", match)
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
	if got := (*out.Items[0].Value)[0].RuleVersion; got != "precedent-v1" {
		t.Fatalf("rule version = %q, want precedent-v1", got)
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
	thread, err := svc.corpus.UpsertThread(ctx, corpus.Thread{RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 9, State: "closed", Title: "abandoned change", Author: "alice", MergedKnown: true, SourceUpdatedAt: now}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := svc.corpus.UpsertThread(ctx, corpus.Thread{RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 10, State: "closed", Title: "header only", Author: "alice", SourceUpdatedAt: now}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	out, err := (&MCPReader{svc}).ListPullRequestPortfolio(ctx, mcpserver.ListPullRequestPortfolioInput{Author: "alice", State: "closed", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.PullRequests) != 2 {
		t.Fatalf("closed pull request classification = %+v", out.PullRequests)
	}
	byNumber := map[int]mcpserver.PullRequestPortfolioItem{}
	for _, item := range out.PullRequests {
		byNumber[item.Number] = item
	}
	if byNumber[thread.Number].Attention != "closed_unmerged" || byNumber[unknown.Number].Attention != "unknown" || !strings.Contains(byNumber[unknown.Number].Reasons[0], "merge state has not been observed") {
		t.Fatalf("closed pull request classification = %+v", out.PullRequests)
	}
}

func TestPullRequestPortfolioKeepsComputingMergeabilityUnknown(t *testing.T) {
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
	values := map[string]any{
		FacetPRDetails:       github.PullRequestDetails{Number: 10, UpdatedAt: now},
		FacetPRReviews:       []github.Review{},
		FacetPRMergeState:    github.PullRequestMergeState{MergeStateStatus: "UNKNOWN", Mergeable: "UNKNOWN", MergeableKnown: false},
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
	out, err := (&MCPReader{svc}).ListPullRequestPortfolio(ctx, mcpserver.ListPullRequestPortfolioInput{Author: "alice", State: "open", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.PullRequests) != 1 || out.PullRequests[0].Attention != "unknown" || !strings.Contains(strings.Join(out.PullRequests[0].Reasons, " "), "mergeability is still computing") {
		t.Fatalf("portfolio = %+v", out)
	}
}
