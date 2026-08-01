package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
	"github.com/morluto/gitcontribute/internal/radar"
)

func TestRadarCandidateToMCPPreservesRelatedWorkSemantics(t *testing.T) {
	t.Parallel()
	out := radarCandidateToMCP(radar.Candidate{RelatedWork: []radar.RelatedWork{{
		Ref: "pull_request:owner/repo#9", Relation: "depends_on", Direction: "outbound", State: "open",
	}}})
	if len(out.RelatedWork) != 1 || out.RelatedWork[0].Ref != "pull_request:owner/repo#9" || out.RelatedWork[0].Relation != "depends_on" || out.RelatedWork[0].Direction != "outbound" || out.RelatedWork[0].State != "open" {
		t.Fatalf("related work = %+v", out.RelatedWork)
	}
}

func TestRankOpportunitiesReportsBoundedNonPaginatedTruncation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	svc.SetGitHubReader(panicRadarReader{})
	seedRadarRepository(ctx, t, svc, "rocket", 5, now)

	reader := &MCPReader{svc}
	bounded, err := reader.RankOpportunities(ctx, mcpcontract.RankOpportunitiesInput{
		Repositories: []mcpcontract.RepositoryRef{{Owner: "acme", Repo: "rocket"}},
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
	perRepositoryBound, err := reader.RankOpportunities(ctx, mcpcontract.RankOpportunitiesInput{
		Repositories: []mcpcontract.RepositoryRef{{Owner: "acme", Repo: "rocket"}}, Limit: 100, MaxResultsPerRepository: 3,
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
	full, err := reader.RankOpportunities(ctx, mcpcontract.RankOpportunitiesInput{
		Repositories: []mcpcontract.RepositoryRef{{Owner: "acme", Repo: "rocket"}}, Limit: 100, MaxResultsPerRepository: 5,
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

func assertRadarCandidateRanks(t *testing.T, candidates []mcpcontract.OpportunityCandidateOutput) {
	t.Helper()
	for index, candidate := range candidates {
		if candidate.Rank != index+1 {
			t.Fatalf("candidate %s rank = %d, want %d", candidate.Ref, candidate.Rank, index+1)
		}
	}
}

func TestRankOpportunitiesUsesOneEvaluationTimeAcrossRepositories(t *testing.T) {
	t.Parallel()
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
	out, err := (&MCPReader{svc}).RankOpportunities(ctx, mcpcontract.RankOpportunitiesInput{
		Repositories: []mcpcontract.RepositoryRef{{Owner: "acme", Repo: "one"}, {Owner: "acme", Repo: "two"}},
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
	t.Parallel()
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
	dossierAsOf := time.Unix(20, 0).UTC()
	if _, err := svc.corpus.SaveDossier(ctx, observed.ID, observed.Owner, observed.Name, "sha", dossierAsOf, `{}`, `{}`, dossierAsOf, nil); err != nil {
		t.Fatal(err)
	}
	out, err := (&MCPReader{svc}).GetRepositories(ctx, mcpcontract.GetRepositoriesInput{Repositories: []mcpcontract.RepositoryRef{{Owner: "acme", Repo: placeholder.Name}, {Owner: "acme", Repo: observed.Name}, {Owner: "acme", Repo: "missing"}}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "partial" || len(out.Items) != 3 {
		t.Fatalf("unexpected batch: %+v", out)
	}
	if got := out.Items[0].Value; got == nil || got.Metadata.Status != "missing" || got.Stars != nil || got.DossierStatus != "missing" || got.DossierAsOf != "" {
		t.Fatalf("placeholder exposed false facts: %+v", got)
	}
	if got := out.Items[1].Value; got == nil || got.Metadata.Status != "complete" || got.Stars == nil || *got.Stars != 42 || got.DossierStatus != "available" || got.DossierAsOf != dossierAsOf.Format(time.RFC3339) {
		t.Fatalf("observed metadata missing: %+v", got)
	}
	if out.Items[2].Key != "acme/missing" || out.Items[2].Status != "unavailable" {
		t.Fatalf("missing item = %+v", out.Items[2])
	}
}

func TestGetCoveragePreservesTargetOrderAndMissingItems(t *testing.T) {
	t.Parallel()
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

	out, err := (&MCPReader{svc}).GetCoverage(ctx, mcpcontract.GetCoverageInput{Targets: []mcpcontract.CoverageTarget{
		{Type: mcpcontract.CoverageTargetRepository, Repository: mcpcontract.RepositoryRef{Owner: "acme", Repo: "rocket"}},
		{Type: mcpcontract.CoverageTargetRepository, Repository: mcpcontract.RepositoryRef{Owner: "acme", Repo: "missing"}},
		{Type: mcpcontract.CoverageTargetExactThread, Repository: mcpcontract.RepositoryRef{Owner: "acme", Repo: "rocket"}, Thread: &mcpcontract.ExactCoverageThread{Kind: "issue", Number: 7}},
		{Type: mcpcontract.CoverageTargetExactThread, Repository: mcpcontract.RepositoryRef{Owner: "acme", Repo: "rocket"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "partial" || len(out.Items) != 4 {
		t.Fatalf("coverage batch = %+v", out)
	}
	if out.Items[0].Key != "acme/rocket" || out.Items[0].Status != "retryable" || out.Items[0].Reason != "coverage_incomplete" || out.Items[0].Value == nil || out.Items[0].Value.Facets[0].Facet != "metadata" {
		t.Fatalf("repository coverage = %+v", out.Items[0])
	}
	if out.Items[0].Recovery == nil || len(out.Items[0].Recovery.Then) != 1 || out.Items[0].Recovery.Then[0].Type != "ensure_coverage" {
		t.Fatalf("repository coverage recovery = %+v", out.Items[0].Recovery)
	}
	if out.Items[1].Key != "acme/missing" || out.Items[1].Status != "unavailable" || out.Items[1].Reason != "repository_not_indexed" {
		t.Fatalf("missing coverage = %+v", out.Items[1])
	}
	if out.Items[2].Status != "retryable" || out.Items[2].Value == nil || out.Items[2].Value.Kind != "issue" || out.Items[2].Value.Number != 7 || out.Items[2].Value.Facets[0].Status != "incomplete" {
		t.Fatalf("thread coverage = %+v", out.Items[2])
	}
	if out.Items[2].Recovery == nil || len(out.Items[2].Recovery.Then) == 0 || out.Items[2].Recovery.Then[0].Type != "hydrate_threads" {
		t.Fatalf("thread coverage recovery = %+v", out.Items[2].Recovery)
	}
	if out.Items[3].Status != "unavailable" || out.Items[3].Reason != "invalid_reference" {
		t.Fatalf("invalid coverage target = %+v", out.Items[3])
	}
	if !out.Provenance.UnknownCoverage || out.Provenance.Complete || out.Provenance.QueryDigestSHA256 == "" {
		t.Fatalf("coverage provenance = %+v", out.Provenance)
	}
}

func TestGetThreadsPreservesUnknownAndObservedFalseMergeState(t *testing.T) {
	t.Parallel()
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
	out, err := (&MCPReader{svc}).GetThreads(ctx, mcpcontract.GetThreadsInput{View: "compact", Threads: []mcpcontract.ThreadRef{
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
	t.Parallel()
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
	out, err := reader.CancelJobs(ctx, mcpcontract.CancelJobInput{IDs: []string{queued.ID, "missing-job", running.ID, terminal.ID, queued.ID, " "}})
	if err != nil {
		t.Fatal(err)
	}
	assertCancelJobsOutput(t, out, queued.ID)
}

func TestCancelJobsDoesNotExposeOrDependOnMalformedStoredPayload(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)
	if _, err := svc.Jobs(ctx); err != nil {
		t.Fatal(err)
	}
	queued, err := svc.corpus.CreateJob(ctx, "sync", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	malformed, err := svc.corpus.CreateJob(ctx, "sync", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.StartJob(ctx, malformed.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.TransitionJob(ctx, malformed.ID, corpus.JobStatusRunning, corpus.JobStatusCancelled, "not-json", ""); err != nil {
		t.Fatal(err)
	}

	out, err := (&MCPReader{svc}).CancelJobs(ctx, mcpcontract.CancelJobInput{IDs: []string{malformed.ID, queued.ID}})
	if err != nil {
		t.Fatalf("cancel jobs: %v", err)
	}
	if out.Status != "complete" || len(out.Items) != 2 {
		t.Fatalf("cancellation batch = %+v", out)
	}
	if got := out.Items[0]; got.Status != "complete" || got.Value == nil || got.Value.Status != "cancelled" || len(got.Value.Artifacts) != 0 {
		t.Fatalf("malformed item = %+v", got)
	}
	if got := out.Items[1]; got.Status != "complete" || got.Value == nil || got.Value.Status != "cancelled" {
		t.Fatalf("queued item = %+v", got)
	}
}

func TestMCPSourceRefsToDomainRejectsInvalidTimestamps(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		ref  mcpcontract.SourceRef
		want string
	}{
		{name: "observed at", ref: mcpcontract.SourceRef{ObservedAt: "not-a-date"}, want: "source_refs[0].observed_at"},
		{name: "as of", ref: mcpcontract.SourceRef{AsOf: "not-a-date"}, want: "source_refs[0].as_of"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := mcpSourceRefsToDomain([]mcpcontract.SourceRef{tc.ref})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want field path %q", err, tc.want)
			}
		})
	}

	refs, err := mcpSourceRefsToDomain([]mcpcontract.SourceRef{{ObservedAt: "2026-07-21T00:00:00Z"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].ObservedAt.IsZero() || !refs[0].AsOf.IsZero() {
		t.Fatalf("source refs = %+v", refs)
	}
}

func assertCancelJobsOutput(t *testing.T, out mcpcontract.GetJobsOutput, queuedID string) {
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
	if out.Items[2].Value == nil || out.Items[2].Value.Status != "running" || !out.Items[2].Value.CancellationRequested || out.Items[2].Value.RetryAfterMS != 1000 || out.Items[2].Recovery == nil || len(out.Items[2].Recovery.Then) != 1 {
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
	t.Parallel()
	out := jobResultToMCP(&contracts.JobResult{ID: "job-1", Kind: "sync_threads", Status: "running", Request: `{}`, Progress: "thread_headers", Statistics: `{"completed_items":2,"total_items":5}`, CreatedAt: "2026-07-19T00:00:00Z"}, true)
	if out.Phase != "thread_headers" || out.CompletedItems != 2 || out.TotalItems != 5 || out.ProgressPercent != 40 || out.RetryAfterMS != 1000 {
		t.Fatalf("structured progress = %+v", out)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"progress":`) || strings.Contains(string(encoded), `"statistics":`) ||
		strings.Contains(string(encoded), `"request":`) || strings.Contains(string(encoded), `"result":`) {
		t.Fatalf("executor storage representation leaked into MCP output: %s", encoded)
	}
}

func TestJobResultToMCPPreservesEmptyAndPartialTypedOutcomes(t *testing.T) {
	t.Parallel()
	empty := jobResultToMCP(&contracts.JobResult{
		ID: "job-empty", Kind: "sync_threads", Status: "succeeded",
		Result: `{"status":"complete","items":[]}`, CreatedAt: "2026-07-19T00:00:00Z",
	}, true)
	if len(empty.Artifacts) != 1 || empty.Artifacts[0].Count == nil || *empty.Artifacts[0].Count != 0 {
		t.Fatalf("known empty artifact count = %+v", empty.Artifacts)
	}

	partial := jobResultToMCP(&contracts.JobResult{
		ID: "job-partial", Kind: "sync_pull_request_portfolio", Status: "succeeded",
		Result:    `{"status":"partial","pull_requests":["acme/rocket#7"],"refreshed":0,"failures":[{"reference":"acme/rocket#7","status":"retryable","reason":"facet_incomplete"}]}`,
		CreatedAt: "2026-07-19T00:00:00Z",
	}, true)
	if !strings.Contains(partial.Summary, "partial") || len(partial.Artifacts) != 1 ||
		!reflect.DeepEqual(partial.Artifacts[0].References, []string{"acme/rocket#7"}) ||
		len(partial.Artifacts[0].Failures) != 1 || partial.Artifacts[0].Failures[0].Reason != "facet_incomplete" {
		t.Fatalf("partial portfolio outcome = %+v", partial)
	}

	fixPatterns := jobResultToMCP(&contracts.JobResult{
		ID: "job-patterns", Kind: "mine_repository_fix_patterns", Status: "succeeded",
		Result: `{"status":"complete","coverage":{"unique_candidates":21}}`, CreatedAt: "2026-07-19T00:00:00Z",
	}, true)
	if fixPatterns.ExecutionState != "terminal" || fixPatterns.Outcome != "succeeded" ||
		len(fixPatterns.Artifacts) != 1 || fixPatterns.Artifacts[0].Kind != "fix_pattern_report" ||
		fixPatterns.Artifacts[0].URI != "gitcontribute://fix-pattern-report/job-patterns" {
		t.Fatalf("fix-pattern job outcome = %+v", fixPatterns)
	}

	runningPatterns := jobResultToMCP(&contracts.JobResult{
		ID: "job-running-patterns", Kind: "mine_repository_fix_patterns", Status: "running",
	}, true)
	if len(runningPatterns.Artifacts) != 0 || runningPatterns.FollowUp == nil ||
		runningPatterns.FollowUp.Action.Type != "poll_job" {
		t.Fatalf("running fix-pattern job advertised unavailable artifacts: %+v", runningPatterns)
	}
}

func TestGetJobsDetailedReturnsTypedArtifactsWithoutStoredPayloads(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)
	job, err := svc.corpus.CreateJob(ctx, "build_repository_dossier", `{"owner":"acme","repo":"rocket"}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.StartJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.TransitionJob(ctx, job.ID, corpus.JobStatusRunning, corpus.JobStatusSucceeded, `{"status":"complete"}`, ""); err != nil {
		t.Fatal(err)
	}
	reader := &MCPReader{svc}
	concise, err := reader.GetJobs(ctx, mcpcontract.GetJobsInput{IDs: []string{job.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if len(concise.Items) != 1 || concise.Items[0].Value == nil || len(concise.Items[0].Value.Artifacts) != 0 {
		t.Fatalf("concise jobs output should remain a bounded state summary: %+v", concise)
	}
	if concise.Items[0].Recovery == nil || len(concise.Items[0].Recovery.Then) != 1 {
		t.Fatalf("default concise terminal result lacks detailed recovery hint: %+v", concise)
	}
	detailed, err := reader.GetJobs(ctx, mcpcontract.GetJobsInput{IDs: []string{job.ID}, ResponseFormat: "detailed"})
	if err != nil {
		t.Fatal(err)
	}
	value := detailed.Items[0].Value
	if value == nil || len(value.Artifacts) != 1 || value.Artifacts[0].Kind != "dossier" ||
		value.Artifacts[0].URI != "gitcontribute://dossier/acme/rocket" ||
		value.FollowUp == nil || value.FollowUp.Action.Type != "read_resource" || value.FollowUp.Action.ReadResource == nil ||
		value.FollowUp.Action.ReadResource.URI != "gitcontribute://dossier/acme/rocket" {
		t.Fatalf("detailed jobs output lost typed artifact reference: %+v", detailed)
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
	if out.Items[0].Value.Watchers != nil || len(out.RecoveryPlans) != 1 || len(out.RecoveryPlans[0].Then) != 1 || out.RecoveryPlans[0].Then[0].Type != "sync_threads" {
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
	match := liveRepositorySearchMatch(remote, mcpcontract.RepositoryMetadataOutput{Status: "complete"}, "detailed")
	if match.Ref != "repository:acme/rocket" || match.Watchers == nil || *match.Watchers != 9 || match.Archived == nil || !*match.Archived || len(match.Topics) != 1 {
		t.Fatalf("detailed match = %+v", match)
	}
}

func TestFindPrecedentsUsesClosedAndMergedHistory(t *testing.T) {
	t.Parallel()
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
	out, err := (&MCPReader{svc}).FindPrecedents(ctx, mcpcontract.FindPrecedentsInput{Threads: []mcpcontract.ThreadRef{{Owner: "acme", Repo: "rocket", Number: 1}}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if out.Total != 1 || out.Items[0].Value == nil || out.Items[0].Value.Matches[0].Ref != "acme/rocket#2" {
		t.Fatalf("unexpected precedents: %+v", out)
	}
	if reasons := out.Items[0].Value.Matches[0].Reasons; len(reasons) < 2 || reasons[1] != "pull request merged" {
		t.Fatalf("missing merged evidence: %v", reasons)
	}
	if got := out.Items[0].Value.Matches[0].RuleVersion; got != "precedent-v1" {
		t.Fatalf("rule version = %q, want precedent-v1", got)
	}
	if out.Provenance.SnapshotToken == "" || out.Provenance.QueryDigestSHA256 == "" {
		t.Fatalf("precedent provenance = %+v", out.Provenance)
	}
}

func TestFindPrecedentsReturnsRecoveryForMissingHistory(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)
	if _, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: "acme", Name: "rocket"}, `{}`); err != nil {
		t.Fatal(err)
	}
	out, err := (&MCPReader{svc}).FindPrecedents(ctx, mcpcontract.FindPrecedentsInput{Threads: []mcpcontract.ThreadRef{
		{Owner: "acme", Repo: "missing", Number: 1},
		{Owner: "acme", Repo: "rocket", Number: 2},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "partial" || len(out.Items) != 2 {
		t.Fatalf("precedent recovery output = %+v", out)
	}
	for _, item := range out.Items {
		if item.Status != "unavailable" || item.Recovery == nil || len(item.Recovery.Then) != 1 || item.Recovery.Then[0].Type != "ensure_coverage" {
			t.Fatalf("missing precedent recovery = %+v", item)
		}
		ensure := item.Recovery.Then[0].EnsureCoverage
		if ensure == nil || ensure.Target.Type != mcpcontract.CoverageTargetRepository || ensure.Target.Repository.Owner != "acme" || ensure.LimitPerRepository != 1000 {
			t.Fatalf("precedent ensure-coverage target = %+v", ensure)
		}
	}
}

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
