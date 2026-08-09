package app

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func TestFeedbackIndexArtifactBoundsReferencesAndKeepsCount(t *testing.T) {
	t.Parallel()
	items := make([]pullRequestFeedbackIndexItem, 0, maxJobArtifactItems+1)
	for number := 1; number <= maxJobArtifactItems+1; number++ {
		items = append(items, pullRequestFeedbackIndexItem{Key: fmt.Sprintf("acme/rocket/pull_request#%d", number), Status: "complete"})
	}
	result, err := json.Marshal(pullRequestFeedbackIndexResult{Status: "complete", DiscoveryStatus: "complete", Items: items})
	if err != nil {
		t.Fatal(err)
	}
	request, err := json.Marshal(mcpcontract.IndexPullRequestFeedbackInput{Repository: mcpcontract.RepositoryRef{Owner: "acme", Repo: "rocket"}})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, _ := pullRequestFeedbackIndexJobArtifact(&contracts.JobResult{Kind: jobKindIndexPullRequestFeedback, Request: string(request), Result: string(result)})
	if len(artifacts) != 1 {
		t.Fatalf("artifacts = %+v", artifacts)
	}
	artifact := artifacts[0]
	if artifact.Count == nil || int(*artifact.Count) != maxJobArtifactItems+1 || len(artifact.References) != maxJobArtifactItems || !artifact.ReferencesTruncated {
		t.Fatalf("bounded feedback-index artifact = %+v", artifact)
	}
}

func TestFeedbackIndexArtifactSignalsBoundedFailures(t *testing.T) {
	t.Parallel()
	items := make([]pullRequestFeedbackIndexItem, 0, maxJobArtifactItems+1)
	for number := 1; number <= maxJobArtifactItems+1; number++ {
		items = append(items, pullRequestFeedbackIndexItem{Key: fmt.Sprintf("acme/rocket/pull_request#%d", number), Status: "failed", Code: "transient", Message: "retry later"})
	}
	result, err := json.Marshal(pullRequestFeedbackIndexResult{Status: "partial", DiscoveryStatus: "partial", Items: items})
	if err != nil {
		t.Fatal(err)
	}
	request, err := json.Marshal(mcpcontract.IndexPullRequestFeedbackInput{Repository: mcpcontract.RepositoryRef{Owner: "acme", Repo: "rocket"}})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, _ := pullRequestFeedbackIndexJobArtifact(&contracts.JobResult{Kind: jobKindIndexPullRequestFeedback, Request: string(request), Result: string(result)})
	if len(artifacts) != 1 || len(artifacts[0].Failures) != maxJobArtifactItems || !artifacts[0].FailuresTruncated {
		t.Fatalf("bounded feedback-index failures = %+v", artifacts)
	}
}

func TestRepositoryBatchArtifactDoesNotCallFailuresReferenceTruncation(t *testing.T) {
	t.Parallel()
	job := &contracts.JobResult{Kind: "sync_repository_context", Result: `{"items":[{"key":"acme/rocket","status":"complete"},{"key":"acme/missing","status":"failed","reason":"not_found"}]}`}
	artifacts, _ := repositoryBatchJobArtifact(job, 2)
	if len(artifacts) != 1 || artifacts[0].ReferencesTruncated || len(artifacts[0].References) != 1 || len(artifacts[0].Failures) != 1 {
		t.Fatalf("repository batch artifact = %+v", artifacts)
	}
}

func TestRepositoryBatchArtifactSignalsBoundedFailures(t *testing.T) {
	t.Parallel()
	items := make([]syncBatchItem, maxJobArtifactItems+1)
	for i := range items {
		items[i] = syncBatchItem{Key: fmt.Sprintf("acme/repo-%d", i), Status: "failed", Reason: "transient"}
	}
	result, err := json.Marshal(syncBatchResult{Items: items})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, _ := repositoryBatchJobArtifact(&contracts.JobResult{Kind: "sync_repository_context", Result: string(result)}, len(items))
	if len(artifacts) != 1 || len(artifacts[0].Failures) != maxJobArtifactItems || !artifacts[0].FailuresTruncated {
		t.Fatalf("bounded repository failures = %+v", artifacts)
	}
}

func TestWorkflowArtifactBoundsPersistedTerminalLists(t *testing.T) {
	t.Parallel()
	items := make([]pullRequestWorkflowItem, 0, 2*maxJobArtifactItems+2)
	for i := 0; i <= maxJobArtifactItems; i++ {
		items = append(items, pullRequestWorkflowItem{Key: fmt.Sprintf("acme/rocket/pull_request#%d", i+1), Status: "complete", ResourceURI: fmt.Sprintf("gitcontribute://pull-request-feedback/acme/rocket/%d", i+1)})
	}
	for i := 0; i <= maxJobArtifactItems; i++ {
		items = append(items, pullRequestWorkflowItem{Key: fmt.Sprintf("acme/failed/pull_request#%d", i+1), Status: "failed", Code: "transient"})
	}
	result, err := json.Marshal(pullRequestWorkflowResult{BatchStatus: "partial", Items: items})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, _ := pullRequestWorkflowJobArtifact(&contracts.JobResult{Kind: "sync_pull_request_feedback", Result: string(result)})
	if len(artifacts) != 1 {
		t.Fatalf("artifacts = %+v", artifacts)
	}
	artifact := artifacts[0]
	if artifact.Count == nil || int(*artifact.Count) != maxJobArtifactItems+1 || len(artifact.References) != maxJobArtifactItems || !artifact.ReferencesTruncated || len(artifact.Failures) != maxJobArtifactItems || !artifact.FailuresTruncated {
		t.Fatalf("bounded workflow artifact = %+v", artifact)
	}
}

func TestPortfolioArtifactBoundsPersistedTerminalLists(t *testing.T) {
	t.Parallel()
	refs := make([]string, maxJobArtifactItems+1)
	failures := make([]pullRequestStatusFailure, maxJobArtifactItems+1)
	for i := range refs {
		refs[i] = fmt.Sprintf("acme/rocket/pull_request#%d", i+1)
		failures[i] = pullRequestStatusFailure{Reference: refs[i], Status: "failed", Reason: "transient"}
	}
	result, err := json.Marshal(syncPortfolioResult{Status: "partial", PullRequests: refs, Failures: failures})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, _ := portfolioJobArtifact(&contracts.JobResult{Kind: jobKindSyncPullRequestPortfolio, Request: `{"selection":"explicit"}`, Result: string(result)})
	if len(artifacts) != 1 {
		t.Fatalf("artifacts = %+v", artifacts)
	}
	artifact := artifacts[0]
	if len(artifact.References) != maxJobArtifactItems || !artifact.ReferencesTruncated || len(artifact.Failures) != maxJobArtifactItems || !artifact.FailuresTruncated {
		t.Fatalf("bounded portfolio artifact = %+v", artifact)
	}
	if artifact.Recovery == nil || len(artifact.Recovery.Then) != 1 || artifact.Recovery.Then[0].SyncPortfolio == nil || len(artifact.Recovery.Then[0].SyncPortfolio.PullRequests) != maxJobArtifactItems {
		t.Fatalf("portfolio recovery exceeds bounded artifact scope: %+v", artifact.Recovery)
	}
}

func TestPortfolioArtifactOmitsExplicitRecoveryWithoutUsableReferences(t *testing.T) {
	t.Parallel()
	result, err := json.Marshal(syncPortfolioResult{
		Status:       "partial",
		PullRequests: []string{"malformed"},
	})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, _ := portfolioJobArtifact(&contracts.JobResult{
		Kind:    jobKindSyncPullRequestPortfolio,
		Request: `{"selection":"explicit","pull_requests":[{"owner":"acme","repo":"rocket","number":7}]}`,
		Result:  string(result),
	})
	if len(artifacts) != 1 {
		t.Fatalf("artifacts = %+v", artifacts)
	}
	if artifacts[0].Recovery != nil {
		t.Fatalf("recovery without usable references = %+v", artifacts[0].Recovery)
	}
}

func TestCodeIndexBatchArtifactDoesNotCallFailuresReferenceTruncation(t *testing.T) {
	t.Parallel()
	result, err := json.Marshal(indexJobResult{Items: []indexJobItem{
		{Key: "acme/rocket", Status: "complete", CommitSHA: "abc123", ArtifactDigest: "artifact"},
		{Key: "acme/missing", Status: "failed", Reason: "not_found"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, _ := indexRepositoriesJobArtifact(&contracts.JobResult{Kind: "index_repositories", Result: string(result)})
	for _, artifact := range artifacts {
		if artifact.Kind != "repository_batch" {
			continue
		}
		if artifact.ReferencesTruncated || len(artifact.References) != 1 || len(artifact.Failures) != 1 {
			t.Fatalf("code-index batch artifact = %+v", artifact)
		}
		return
	}
	t.Fatalf("missing code-index batch artifact: %+v", artifacts)
}

func TestJobExecutionSeparatesRunningStateFromTerminalOutcome(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		job       contracts.JobResult
		execution mcpcontract.JobExecutionState
		outcome   mcpcontract.JobOutcome
	}{
		{name: "queued", job: contracts.JobResult{Status: "queued"}, execution: "queued"},
		{name: "running", job: contracts.JobResult{Status: "running"}, execution: "running"},
		{name: "succeeded", job: contracts.JobResult{Status: "succeeded", Result: `{"status":"complete"}`}, execution: "terminal", outcome: "succeeded"},
		{name: "partial", job: contracts.JobResult{Status: "succeeded", Result: `{"status":"partial"}`}, execution: "terminal", outcome: "partial"},
		{name: "partial batch", job: contracts.JobResult{Status: "succeeded", Result: `{"batch_status":"partial"}`}, execution: "terminal", outcome: "partial"},
		{name: "failed batch", job: contracts.JobResult{Status: "succeeded", Result: `{"batch_status":"failed"}`}, execution: "terminal", outcome: "failed"},
		{name: "failed", job: contracts.JobResult{Status: "failed"}, execution: "terminal", outcome: "failed"},
		{name: "cancelled", job: contracts.JobResult{Status: "cancelled"}, execution: "terminal", outcome: "cancelled"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			execution, outcome := jobExecution(&test.job)
			if execution != test.execution || outcome != test.outcome {
				t.Fatalf("jobExecution() = (%q, %q), want (%q, %q)", execution, outcome, test.execution, test.outcome)
			}
		})
	}
}

func TestGetJobOutputHidesLegacyStatusFromModelVisibleJSON(t *testing.T) {
	t.Parallel()
	data, err := json.Marshal(mcpcontract.GetJobOutput{
		Status:         "succeeded",
		ExecutionState: "terminal",
		Outcome:        "succeeded",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"status"`) {
		t.Fatalf("legacy status leaked into output: %s", data)
	}
	if !strings.Contains(string(data), `"execution_state":"terminal"`) || !strings.Contains(string(data), `"outcome":"succeeded"`) {
		t.Fatalf("new job state contract missing: %s", data)
	}
}

func TestRecoveryPlanUsesVersionedTypedCallsOnTheWire(t *testing.T) {
	t.Parallel()
	value := mcpcontract.BatchItem[struct{}]{
		Key: "acme/rocket/pull_request#7", Status: "unavailable",
		Recovery: &mcpcontract.RecoveryPlan{
			Version: mcpcontract.RecoveryPlanVersion, Reason: "thread_not_indexed", Message: "sync the exact thread",
			Then: []mcpcontract.ToolCall{mcpcontract.RecoveryAction(mcpcontract.SyncThreadsInput{
				Selection: "threads", Threads: []mcpcontract.ThreadRef{{Owner: "acme", Repo: "rocket", Kind: "pull_request", Number: 7}},
			})},
		},
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(data)
	if strings.Contains(encoded, "next_action") || !strings.Contains(encoded, `"version":"`+mcpcontract.RecoveryPlanVersion+`"`) || !strings.Contains(encoded, `"type":"sync_threads"`) || !strings.Contains(encoded, `"kind":"pull_request"`) {
		t.Fatalf("recovery wire contract = %s", encoded)
	}
}

func TestRemovedJobKindsDoNotExposeCompatibilityArtifacts(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"sync_portfolio", "sync_authored_pull_requests", "sync_pull_request_status", "hydrate_threads"} {
		t.Run(kind, func(t *testing.T) {
			out := jobResultToMCP(&contracts.JobResult{
				Kind: kind, Status: "succeeded", Result: `{"status":"complete","items":[]}`,
			}, true)
			if len(out.Artifacts) != 0 || out.FollowUp != nil {
				t.Fatalf("removed job kind exposed compatibility output: artifacts=%+v follow_up=%+v", out.Artifacts, out.FollowUp)
			}
		})
	}
}

func TestThreadSyncFollowUpUsesResolvedExactThreads(t *testing.T) {
	t.Parallel()
	job := &contracts.JobResult{
		Kind: "sync_threads", Status: "succeeded",
		Request: `{"selection":"repositories","repositories":[{"owner":"acme","repo":"rocket"}]}`,
		Result:  `{"status":"complete","items":[{"key":"acme/rocket","status":"complete","threads":[{"owner":"acme","repo":"rocket","kind":"pull_request","number":7}]}]}`,
	}
	artifacts, follow := jobArtifactsAndFollowUp(job, 1)
	if len(artifacts) != 1 || follow == nil || follow.Action.Type != "get_threads" || follow.Action.GetThreads == nil {
		t.Fatalf("thread sync handoff = artifacts:%+v follow:%+v", artifacts, follow)
	}
	if len(follow.Action.GetThreads.Threads) != 1 || follow.Action.GetThreads.Threads[0].Kind != "pull_request" || follow.Action.GetThreads.Threads[0].Number != 7 {
		t.Fatalf("thread sync follow-up arguments = %+v", follow.Action)
	}
	if len(artifacts[0].References) != 1 || artifacts[0].References[0] != "acme/rocket/pull_request#7" {
		t.Fatalf("thread sync references = %+v", artifacts[0].References)
	}
}

func TestPersistedWorkflowFollowUpReadsResourceWithoutResubmittingMutation(t *testing.T) {
	t.Parallel()
	job := &contracts.JobResult{
		Kind: "sync_pull_request_feedback", Status: "succeeded",
		Result: `{"status":"complete","items":[{"key":"acme/rocket/pull_request#7","item_status":"complete","resource_uri":"gitcontribute://pull-request-feedback/acme/rocket/7"}]}`,
	}
	_, follow := jobArtifactsAndFollowUp(job, 1)
	if follow == nil || follow.Action.Type != "read_resource" || follow.Action.ReadResource == nil || follow.Action.ReadResource.URI != "gitcontribute://pull-request-feedback/acme/rocket/7" {
		t.Fatalf("resource handoff = %+v", follow)
	}
}

func TestPortfolioFollowUpUsesPortfolioReadArguments(t *testing.T) {
	t.Parallel()
	job := &contracts.JobResult{
		Kind: jobKindSyncPullRequestPortfolio, Status: "succeeded",
		Request: `{"selection":"authored","repository":{"owner":"acme","repo":"rocket"},"state":"closed","limit":10}`,
		Result:  `{"status":"complete","login":"alice","pull_requests":["acme/rocket/pull_request#7"],"refreshed":1}`,
	}
	_, follow := jobArtifactsAndFollowUp(job, 1)
	if follow == nil || follow.Action.Type != "list_pull_request_portfolio" || follow.Action.ListPortfolio == nil {
		t.Fatalf("portfolio handoff = %+v", follow)
	}
	if follow.Action.ListPortfolio.Repository == nil || follow.Action.ListPortfolio.Repository.Owner != "acme" || follow.Action.ListPortfolio.Repository.Repo != "rocket" || len(follow.Action.ListPortfolio.Authors) != 1 || follow.Action.ListPortfolio.Authors[0] != "alice" || follow.Action.ListPortfolio.State != "closed" || follow.Action.ListPortfolio.Limit != 10 || follow.Action.ListPortfolio.View != "compact" {
		t.Fatalf("portfolio follow-up arguments = %+v", follow.Action)
	}
}

func TestExplicitPortfolioFollowUpPreservesExactReferences(t *testing.T) {
	t.Parallel()
	job := &contracts.JobResult{
		Kind: jobKindSyncPullRequestPortfolio, Status: "succeeded",
		Request: `{"selection":"explicit","pull_requests":[{"owner":"acme","repo":"rocket","kind":"pull_request","number":7}]}`,
		Result:  `{"status":"complete","pull_requests":["acme/rocket/pull_request#7"],"refreshed":1,"discovery_status":"complete"}`,
	}
	_, follow := jobArtifactsAndFollowUp(job, 1)
	if follow == nil || follow.Action.ListPortfolio == nil || len(follow.Action.ListPortfolio.PullRequests) != 1 {
		t.Fatalf("portfolio handoff = %+v", follow)
	}
	ref := follow.Action.ListPortfolio.PullRequests[0]
	if ref.Owner != "acme" || ref.Repo != "rocket" || ref.Kind != "pull_request" || ref.Number != 7 {
		t.Fatalf("exact portfolio handoff = %+v", ref)
	}
}

func TestLegacyAuthoredPortfolioFollowUpUsesObservedLogin(t *testing.T) {
	t.Parallel()
	job := &contracts.JobResult{
		Kind: jobKindSyncPullRequestPortfolio, Status: "succeeded",
		Request: `{}`,
		Result:  `{"status":"complete","login":"alice","pull_requests":["acme/rocket/pull_request#7"],"refreshed":1,"discovery_status":"complete"}`,
	}
	_, follow := jobArtifactsAndFollowUp(job, 1)
	if follow == nil || follow.Action.ListPortfolio == nil || len(follow.Action.ListPortfolio.Authors) != 1 || follow.Action.ListPortfolio.Authors[0] != "alice" {
		t.Fatalf("legacy authored portfolio handoff = %+v", follow)
	}
}

func TestLegacyExplicitPortfolioFollowUpPreservesResultReferences(t *testing.T) {
	t.Parallel()
	job := &contracts.JobResult{
		Kind: jobKindSyncPullRequestPortfolio, Status: "succeeded",
		Request: `{}`,
		Result:  `{"status":"complete","pull_requests":["acme/rocket/pull_request#7"],"refreshed":1,"discovery_status":"complete"}`,
	}
	_, follow := jobArtifactsAndFollowUp(job, 1)
	if follow == nil || follow.Action.ListPortfolio == nil || len(follow.Action.ListPortfolio.PullRequests) != 1 {
		t.Fatalf("legacy explicit portfolio handoff = %+v", follow)
	}
}

func TestPortfolioFollowUpOmitsUnprovenScope(t *testing.T) {
	t.Parallel()
	for _, job := range []*contracts.JobResult{
		{
			Kind: jobKindSyncPullRequestPortfolio, Status: "succeeded",
			Request: `{"selection":"authored","state":"closed","limit":10}`,
			Result:  `{"status":"complete","pull_requests":["acme/rocket/pull_request#7"],"refreshed":1}`,
		},
		{
			Kind: jobKindSyncPullRequestPortfolio, Status: "succeeded",
			Request: `{"selection":"explicit","pull_requests":[{"owner":"acme","repo":"rocket","kind":"pull_request","number":7}]}`,
			Result:  `{"status":"complete","pull_requests":["not-a-pull-request"],"refreshed":1}`,
		},
		{
			Kind: jobKindSyncPullRequestPortfolio, Status: "succeeded",
			Request: `{}`,
			Result:  `{"status":"complete","pull_requests":["not-a-pull-request"],"refreshed":1}`,
		},
	} {
		_, follow := jobArtifactsAndFollowUp(job, 1)
		if follow != nil {
			t.Fatalf("unproven portfolio scope yielded follow-up = %+v", follow)
		}
	}
}

func TestJobArtifactsOmitFollowUpWhenRequestCannotProveScope(t *testing.T) {
	t.Parallel()
	for _, job := range []*contracts.JobResult{
		{
			Kind:    "sync_repository_context",
			Status:  "succeeded",
			Request: `not-json`,
			Result:  `{"items":[{"key":"acme/rocket","status":"complete"}]}`,
		},
		{
			Kind:    jobKindSyncThreadFacets,
			Status:  "succeeded",
			Request: `not-json`,
			Result:  `{"status":"complete"}`,
		},
	} {
		_, follow := jobArtifactsAndFollowUp(job, 1)
		if follow != nil {
			t.Fatalf("unproven job scope yielded follow-up = %+v", follow)
		}
	}
}

func TestIndexJobPreservesFailuresAndBindsCompletedArtifactsToSnapshot(t *testing.T) {
	t.Parallel()
	job := &contracts.JobResult{
		Kind: "index_repositories", Status: "succeeded",
		Result: `{"status":"partial","snapshot_token":"ephemeral:2","items":[{"key":"acme/rocket","status":"complete","commit_sha":"sha","snapshot_token":"ephemeral:1","artifact_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","manifest_digest":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","index_manifest":{"format_version":"v1","indexed_files":2}},{"key":"acme/missing","status":"failed","reason":"acquisition_or_index_failed","message":"checkout failed","retry_after_ms":1000}]}`,
	}
	artifacts, _ := jobArtifactsAndFollowUp(job, 2)
	if len(artifacts) != 2 || artifacts[0].CodeIndex == nil || artifacts[0].CodeIndex.SnapshotToken != "ephemeral:2" {
		t.Fatalf("index artifacts = %+v", artifacts)
	}
	failures := artifacts[1].Failures
	if len(failures) != 1 || failures[0].Reference != "acme/missing" || failures[0].Reason != "acquisition_or_index_failed" || failures[0].RetryAfterMS != 1000 {
		t.Fatalf("index failures = %+v", failures)
	}
}

func TestPullRequestFeedbackIndexJobOffersOfflineSearchFollowUp(t *testing.T) {
	t.Parallel()
	job := &contracts.JobResult{
		Kind: jobKindIndexPullRequestFeedback, Status: "succeeded",
		Request: `{"repository":{"owner":"acme","repo":"rocket"}}`,
		Result:  `{"status":"partial","discovery_status":"partial","items":[{"key":"acme/rocket/pull_request#7","item_status":"complete"}],"recovery":{"version":"recovery.v1","reason":"feedback_discovery_incomplete","message":"continue"}}`,
	}
	artifacts, follow := jobArtifactsAndFollowUp(job, 1)
	if len(artifacts) != 1 || artifacts[0].Kind != "pull_request_feedback_index" || artifacts[0].DiscoveryStatus != "partial" || follow == nil {
		t.Fatalf("feedback index artifact = %+v follow=%+v", artifacts, follow)
	}
	if follow.Action.Type != "search_pull_request_feedback" || follow.Action.SearchFeedback == nil || follow.Action.SearchFeedback.Repository.Owner != "acme" || follow.Action.SearchFeedback.Repository.Repo != "rocket" {
		t.Fatalf("feedback index follow-up = %+v", follow)
	}
}
