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
			Then: []mcpcontract.ToolCall{{Tool: mcpcontract.ToolSyncThreads, Arguments: &mcpcontract.ToolCallArguments{
				Selection: "threads", Threads: []mcpcontract.ThreadRef{{Owner: "acme", Repo: "rocket", Kind: "pull_request", Number: 7}},
			}}},
		},
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(data)
	if strings.Contains(encoded, "next_action") || !strings.Contains(encoded, `"version":"`+mcpcontract.RecoveryPlanVersion+`"`) || !strings.Contains(encoded, `"tool":"github.sync_threads"`) || !strings.Contains(encoded, `"kind":"pull_request"`) {
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
