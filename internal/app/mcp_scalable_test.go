package app

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func TestTypedBatchResultsPreserveDurableJSONShapes(t *testing.T) {
	t.Parallel()
	threadSync := threadSyncBatchResult{
		batchOperationSummary: batchOperationSummary[threadSyncItem]{
			Status: batchOperationPartial,
			Items: []threadSyncItem{
				successfulThreadSyncItem("acme/rocket", 3, 2, true, "request budget reached", []mcpcontract.ThreadRef{{Owner: "acme", Repo: "rocket", Kind: "issue", Number: 1}}),
				failedThreadSyncItem("acme/missing", mcpcontract.BatchItemRetryable, "rate_limited", "wait", 0),
			},
			Completed: 0,
			Total:     2,
		},
		Requests: 2, RequestBudget: 3, PlannedRequests: 3,
	}
	assertJSONDocumentEqual(t, threadSync, `{
		"status":"partial","items":[
			{"key":"acme/rocket","status":"partial","updated":3,"requests":2,"request_capped":true,"message":"request budget reached","threads":[{"owner":"acme","repo":"rocket","kind":"issue","number":1}]},
			{"key":"acme/missing","status":"retryable","reason":"rate_limited","message":"wait","retry_after_ms":0}
		],"completed":0,"total":2,"requests":2,"request_budget":3,"planned_requests":3
	}`)

	exactFailure, err := unavailableThreadSyncItem("acme/rocket/issue", "repository_not_indexed", "missing").forExactThread(
		"acme/rocket/issue#1", mcpcontract.ThreadRef{Owner: "acme", Repo: "rocket", Kind: "issue", Number: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertJSONDocumentEqual(t, exactFailure, `{
		"key":"acme/rocket/issue#1","status":"unavailable","reason":"repository_not_indexed","message":"missing",
		"threads":[{"owner":"acme","repo":"rocket","kind":"issue","number":1}]
	}`)

	index := repositoryIndexBatchResult{
		batchOperationSummary: batchOperationSummary[repositoryIndexItem]{
			Status: batchOperationPartial,
			Items: []repositoryIndexItem{
				successfulRepositoryIndexItem("acme/rocket", contracts.AcquisitionResult{
					CommitSHA: "abc", Files: 2, Bytes: 3, Inserted: true, SnapshotToken: "snapshot",
					IndexManifest:  codeindex.Manifest{FormatVersion: "v1", CoverageKnown: true, TrackedEntries: 2, IndexedFiles: 2},
					ArtifactDigest: "artifact", ManifestDigest: "manifest",
				}),
				failedRepositoryIndexItem("acme/missing", "acquisition_or_index_failed", "checkout failed", 0),
			},
			Completed: 1,
			Total:     2,
		},
		SnapshotToken: "snapshot",
	}
	assertJSONDocumentEqual(t, index, `{
		"status":"partial","items":[
			{"key":"acme/rocket","status":"complete","commit_sha":"abc","files":2,"bytes":3,"inserted":true,"snapshot_token":"snapshot",
			 "index_manifest":{"format_version":"v1","coverage_known":true,"tracked_entries":2,"indexed_files":2,"skipped_invalid_path":0,"skipped_excluded":0,"skipped_non_regular":0,"skipped_oversize":0,"skipped_total_budget":0,"skipped_non_text":0,"skipped_file_limit":0,"truncated":false},
			 "artifact_digest":"artifact","manifest_digest":"manifest"},
			{"key":"acme/missing","status":"failed","reason":"acquisition_or_index_failed","message":"checkout failed","retry_after_ms":0}
		],"completed":1,"total":2,"snapshot_token":"snapshot"
	}`)

	contextResult := repositoryContextBatchResult{
		batchOperationSummary: batchOperationSummary[repositoryContextItem]{
			Status: batchOperationPartial,
			Items: []repositoryContextItem{
				successfulRepositoryContextItem("acme/rocket", 2, mcpcontract.RepositoryOutput{Ref: "repository:acme/rocket", Owner: "acme", Repo: "rocket"}),
				unavailableRepositoryContextItem("acme/missing", "request_budget_exceeded", "budget exhausted"),
			},
			Completed: 1,
			Total:     2,
		},
		Requests: 2, RequestBudget: 2, PlannedRequests: 2,
	}
	assertJSONDocumentEqual(t, contextResult, `{
		"status":"partial","items":[
			{"key":"acme/rocket","status":"complete","requests":2,"repository":{"ref":"repository:acme/rocket","owner":"acme","repo":"rocket","metadata":{"status":""},"dossier_status":"","description":null,"default_branch":null,"language":null,"license":null,"stars":null,"watchers":null,"forks":null,"open_issues":null,"archived":null,"fork":null},"facets":{"metadata":{"status":"complete"},"contribution_guidance":{"status":"complete"}}},
			{"key":"acme/missing","status":"unavailable","reason":"request_budget_exceeded","message":"budget exhausted"}
		],"completed":1,"total":2,"requests":2,"request_budget":2,"planned_requests":2
	}`)

	hydration := threadHydrationBatchResult{
		Status: batchOperationPartial,
		Items: []threadHydrationItem{
			completeThreadHydrationItem("acme/rocket/issue#1", "issue", 2, []contracts.HydratedFacet{{Facet: "comments", Count: 3, Pages: 1, Complete: true}}),
			failedThreadHydrationItem("acme/rocket/issue#2", mcpcontract.BatchItemRetryable, "rate_limited", "wait", 0),
		},
		Completed: 1,
		Total:     2,
	}
	assertJSONDocumentEqual(t, hydration, `{
		"status":"partial","items":[
			{"key":"acme/rocket/issue#1","status":"complete","kind":"issue","header_refreshed":true,"requests":2,"facets":[{"facet":"comments","count":3,"pages":1,"complete":true}]},
			{"key":"acme/rocket/issue#2","status":"retryable","reason":"rate_limited","message":"wait","retry_after_ms":0}
		],"completed":1,"total":2
	}`)

	zero := 0
	pullRequests := pullRequestStatusBatchResult{
		Status: batchOperationPartial,
		Items: []pullRequestStatusItem{
			pullRequestStatusSnapshotItem("acme/rocket/pull_request#1", mcpcontract.BatchItemComplete, "", nil, []pullRequestHealthFacet{}, ""),
			pullRequestStatusFailureItem("acme/rocket/pull_request#2", mcpcontract.BatchItemRetryable, "rate_limited", "wait", &zero, nil),
		},
		Failures:  []pullRequestStatusFailure{{Reference: "acme/rocket/pull_request#2", Status: mcpcontract.BatchItemRetryable, Reason: "rate_limited", Message: "wait"}},
		Completed: 1,
		Total:     2,
	}
	assertJSONDocumentEqual(t, pullRequests, `{
		"status":"partial","items":[
			{"key":"acme/rocket/pull_request#1","status":"complete","facets":[],"head_sha":""},
			{"key":"acme/rocket/pull_request#2","status":"retryable","reason":"rate_limited","message":"wait","retry_after_ms":0}
		],"failures":[{"reference":"acme/rocket/pull_request#2","status":"retryable","reason":"rate_limited","message":"wait"}],"completed":1,"total":2
	}`)
}

func assertJSONDocumentEqual(t *testing.T, value any, expected string) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var gotDocument, expectedDocument any
	if err := json.Unmarshal(encoded, &gotDocument); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(expected), &expectedDocument); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotDocument, expectedDocument) {
		t.Fatalf("JSON = %s, want %s", encoded, expected)
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
	thread, err := svc.corpus.UpsertThread(ctx, corpus.Thread{RepositoryID: repo.ID, Kind: domain.IssueKind, Number: 7, State: "open", Title: "bounded coverage", SourceUpdatedAt: time.Unix(20, 0).UTC()}, `{}`)
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
	if out.Items[0].Recovery == nil || len(out.Items[0].Recovery.Then) != 1 || out.Items[0].Recovery.Then[0].Type() != "ensure_coverage" {
		t.Fatalf("repository coverage recovery = %+v", out.Items[0].Recovery)
	}
	if out.Items[1].Key != "acme/missing" || out.Items[1].Status != "unavailable" || out.Items[1].Reason != "repository_not_indexed" {
		t.Fatalf("missing coverage = %+v", out.Items[1])
	}
	if out.Items[2].Status != "retryable" || out.Items[2].Value == nil || out.Items[2].Value.Kind != "issue" || out.Items[2].Value.Number != 7 || out.Items[2].Value.Facets[0].Status != "incomplete" {
		t.Fatalf("thread coverage = %+v", out.Items[2])
	}
	if out.Items[2].Recovery == nil || len(out.Items[2].Recovery.Then) == 0 || out.Items[2].Recovery.Then[0].Type() != "hydrate_threads" {
		t.Fatalf("thread coverage recovery = %+v", out.Items[2].Recovery)
	}
	if out.Items[3].Status != "unavailable" || out.Items[3].Reason != "invalid_reference" {
		t.Fatalf("invalid coverage target = %+v", out.Items[3])
	}
	if !out.Provenance.UnknownCoverage() || out.Provenance.Complete() || out.Provenance.QueryDigestSHA256 == "" {
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
		{RepositoryID: repo.ID, Kind: domain.PullRequestKind, Number: 1, State: "closed", Title: "unknown", SourceUpdatedAt: time.Unix(1, 0).UTC()},
		{RepositoryID: repo.ID, Kind: domain.PullRequestKind, Number: 2, State: "closed", Title: "observed false", Merge: domain.UnmergedStatus(), SourceUpdatedAt: time.Unix(2, 0).UTC()},
	} {
		if _, err := svc.corpus.UpsertThread(ctx, thread, `{}`); err != nil {
			t.Fatal(err)
		}
	}
	out, err := (&MCPReader{svc}).GetThreads(ctx, mcpcontract.GetThreadsInput{View: "compact", Threads: []mcpcontract.ThreadRef{
		{Owner: " acme ", Repo: " rocket ", Kind: " pull_request ", Number: 1},
		{Owner: "acme", Repo: "rocket", Kind: "pull_request", Number: 2},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Items[0].Key != "acme/rocket/pull_request#1" || out.Items[0].Value == nil || out.Items[0].Value.Owner != "acme" || out.Items[0].Value.Repo != "rocket" || out.Items[0].Value.Kind != "pull_request" || out.Items[0].Value.Merged != nil {
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
	if err := svc.corpus.TransitionJob(ctx, terminal.ID, corpus.JobRunningToSucceeded, `{}`, ""); err != nil {
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
	if err := svc.corpus.TransitionJob(ctx, malformed.ID, corpus.JobRunningToCancelled, "not-json", ""); err != nil {
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
	out := jobResultToMCP(&contracts.JobResult{ID: "job-1", Kind: "sync_threads", Status: "running", Request: `{}`, Progress: "thread_headers", Statistics: `{"completed_items":2,"total_items":5}`, CreatedAt: "2026-07-19T00:00:00Z"}, detailedResponse)
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
	}, detailedResponse)
	if len(empty.Artifacts) != 1 || empty.Artifacts[0].Count == nil || *empty.Artifacts[0].Count != 0 {
		t.Fatalf("known empty artifact count = %+v", empty.Artifacts)
	}

	partial := jobResultToMCP(&contracts.JobResult{
		ID: "job-partial", Kind: "sync_pull_request_portfolio", Status: "succeeded",
		Result:    `{"status":"partial","pull_requests":["acme/rocket#7"],"refreshed":0,"failures":[{"reference":"acme/rocket#7","status":"retryable","reason":"facet_incomplete"}]}`,
		CreatedAt: "2026-07-19T00:00:00Z",
	}, detailedResponse)
	if !strings.Contains(partial.Summary, "partial") || len(partial.Artifacts) != 1 ||
		!reflect.DeepEqual(partial.Artifacts[0].References, []string{"acme/rocket#7"}) ||
		len(partial.Artifacts[0].Failures) != 1 || partial.Artifacts[0].Failures[0].Reason != "facet_incomplete" ||
		partial.Artifacts[0].Recovery == nil || len(partial.Artifacts[0].Recovery.Then) != 1 || partial.Artifacts[0].Recovery.Then[0].Type() != "sync_portfolio" {
		t.Fatalf("partial portfolio outcome = %+v", partial)
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
		{RepositoryID: repo.ID, Kind: domain.IssueKind, Number: 1, State: "open", Title: "cache path ignores configured root", Body: "compiled cache artifacts use tmp", SourceUpdatedAt: time.Unix(30, 0).UTC()},
		{RepositoryID: repo.ID, Kind: domain.PullRequestKind, Number: 2, State: "closed", Title: "honor configured cache root", Body: "move compiled cache artifacts out of tmp", Merge: domain.MergedStatus(time.Unix(20, 0).UTC()), ClosedAt: time.Unix(20, 0).UTC(), SourceUpdatedAt: time.Unix(20, 0).UTC()},
		{RepositoryID: repo.ID, Kind: domain.IssueKind, Number: 3, State: "open", Title: "unrelated typo", Body: "docs", SourceUpdatedAt: time.Unix(10, 0).UTC()},
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
		if item.Status != "unavailable" || item.Recovery == nil || len(item.Recovery.Then) != 1 || item.Recovery.Then[0].Type() != "ensure_coverage" {
			t.Fatalf("missing precedent recovery = %+v", item)
		}
		ensure, ok := mcpcontract.RecoveryInput[mcpcontract.EnsureCoverageInput](item.Recovery.Then[0])
		if !ok || ensure.Target.Type != mcpcontract.CoverageTargetRepository || ensure.Target.Repository.Owner != "acme" || ensure.LimitPerRepository != 1000 {
			t.Fatalf("precedent ensure-coverage target = %+v", ensure)
		}
	}
}
