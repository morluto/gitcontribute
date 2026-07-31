package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

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
		Request: `{"selection":"authored","state":"closed","limit":10}`,
		Result:  `{"status":"complete","login":"alice","pull_requests":["acme/rocket/pull_request#7"],"refreshed":1}`,
	}
	_, follow := jobArtifactsAndFollowUp(job, 1)
	if follow == nil || follow.Action.Type != "list_pull_request_portfolio" || follow.Action.ListPortfolio == nil {
		t.Fatalf("portfolio handoff = %+v", follow)
	}
	if len(follow.Action.ListPortfolio.Authors) != 1 || follow.Action.ListPortfolio.Authors[0] != "alice" || follow.Action.ListPortfolio.State != "closed" || follow.Action.ListPortfolio.Limit != 10 || follow.Action.ListPortfolio.View != "compact" {
		t.Fatalf("portfolio follow-up arguments = %+v", follow.Action)
	}
}

func TestIndexJobPreservesFailuresAndBindsCompletedArtifactsToFinalRevision(t *testing.T) {
	t.Parallel()
	job := &contracts.JobResult{
		Kind: "index_repositories", Status: "succeeded",
		Result: `{"status":"partial","corpus_revision":42,"items":[{"key":"acme/rocket","status":"complete","commit_sha":"sha","corpus_revision":40,"artifact_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","manifest_digest":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","snapshot_token":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","index_manifest":{"format_version":"v1","indexed_files":2}},{"key":"acme/missing","status":"failed","reason":"acquisition_or_index_failed","message":"checkout failed","retry_after_ms":1000}]}`,
	}
	artifacts, _ := jobArtifactsAndFollowUp(job, 2)
	if len(artifacts) != 2 || artifacts[0].CodeIndex == nil || artifacts[0].CodeIndex.CorpusRevision != 42 {
		t.Fatalf("index artifacts = %+v", artifacts)
	}
	failures := artifacts[1].Failures
	if len(failures) != 1 || failures[0].Reference != "acme/missing" || failures[0].Reason != "acquisition_or_index_failed" || failures[0].RetryAfterMS != 1000 {
		t.Fatalf("index failures = %+v", failures)
	}
}
