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
