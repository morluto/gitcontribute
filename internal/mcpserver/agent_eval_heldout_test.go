package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

type heldOutScenario struct {
	ID     string `json:"id"`
	Prompt string `json:"prompt"`
}

type heldOutFixture struct {
	Version         string            `json:"version"`
	FixtureRevision string            `json:"fixture_revision"`
	Scenarios       []heldOutScenario `json:"scenarios"`
	Metrics         []string          `json:"measured_metrics"`
}

type heldOutMetric struct {
	Scenario             string  `json:"scenario"`
	Success              bool    `json:"task_success"`
	ToolCalls            int     `json:"tool_calls"`
	ResourceReads        int     `json:"resource_reads"`
	ToolErrors           int     `json:"tool_errors"`
	Retries              int     `json:"retries"`
	LatencyMS            float64 `json:"latency_ms"`
	ResponseBytes        int     `json:"response_bytes"`
	ContextTokenEstimate int     `json:"context_token_estimate"`
}

type heldOutRun struct {
	client        *mcp.ClientSession
	calls         []string
	seen          map[string]int
	toolErrors    int
	responseBytes int
	resourceReads int
}

type heldOutReader struct {
	*fakeReader
}

func (r *heldOutReader) Search(ctx context.Context, in mcpcontract.SearchInput) (mcpcontract.SearchOutput, error) {
	if in.SnapshotToken == "stale-token" {
		return mcpcontract.SearchOutput{}, mcpcontract.Unavailable("snapshot_expired", "the supplied snapshot is no longer current")
	}
	return r.fakeReader.Search(ctx, in)
}

func (r *heldOutReader) GetCoverage(ctx context.Context, in mcpcontract.GetCoverageInput) (mcpcontract.GetCoverageOutput, error) {
	out, err := r.fakeReader.GetCoverage(ctx, in)
	if err != nil {
		return out, err
	}
	for i := range out.Items {
		if out.Items[i].Value == nil || out.Items[i].Value.Repo != "heldout" {
			continue
		}
		out.Items[i].Value.Facets = []mcpcontract.FacetCoverageOutput{{Facet: "metadata", Complete: false, Status: "unknown", UpdatedAt: "2026-07-17T00:00:00Z"}}
	}
	return out, nil
}

func (*heldOutReader) PreviewRepositoryFixPatterns(_ context.Context, in mcpcontract.PreviewRepositoryFixPatternsInput) (mcpcontract.FixPatternReport, error) {
	return mcpcontract.FixPatternReport{
		Status: "complete", Repository: in.Repository, Persisted: false,
		SnapshotToken: "ephemeral:heldout", Complete: true,
		Limitations: []string{"preview is read-only"},
	}, nil
}

func connectHeldOut(t *testing.T, base *heldOutReader) (*mcp.ClientSession, func()) {
	t.Helper()
	optional := &fakeOptionalCapabilities{base: base.fakeReader}
	reader := completeTestReader{
		Reader: base, NeighborReader: optional, ScalableReader: optional, ThreadFacetReader: optional,
		threadFacetResourceReader: base.fakeReader, IssueSetReader: optional,
		PortfolioReader: optional, GitHubOperator: optional, PullRequestFeedbackOperator: optional,
		CIFailureOperator: optional, FixPatternOperator: optional, FixPatternReader: base.fakeReader,
		FixPatternPreviewReader: base, CodeIndexer: optional, MergeConflictReader: optional,
		ResearchReader: optional, CommitPlannerReader: base.fakeReader, PortfolioOperator: optional,
		Operator: base.fakeReader, ConcernReader: base.fakeReader, ConcernOperator: base.fakeReader,
		WorkspaceCreator: base.fakeReader, WorkspaceAdopter: base.fakeReader,
		ValidationReceiptOperator: base.fakeReader, PublishedDraftVerifier: base.fakeReader,
	}
	server, err := New(reader, "test")
	if err != nil {
		t.Fatalf("create held-out server: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "held-out-test-client", Version: "test"}, nil)
	t1, t2 := mcp.NewInMemoryTransports()
	serverSession, err := server.MCP().Connect(context.Background(), t1, nil)
	if err != nil {
		t.Fatalf("connect held-out server: %v", err)
	}
	clientSession, err := client.Connect(context.Background(), t2, nil)
	if err != nil {
		_ = serverSession.Close()
		t.Fatalf("connect held-out client: %v", err)
	}
	return clientSession, func() {
		_ = clientSession.Close()
		_ = serverSession.Close()
	}
}

func TestAgentEvalHeldOutWorkflowMetrics(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join(agentEvalV5Root, "heldout.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture heldOutFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Version != "agent-tool-eval.heldout.v1" || fixture.FixtureRevision == "" || len(fixture.Scenarios) < 4 {
		t.Fatalf("incomplete held-out fixture: %+v", fixture)
	}
	for _, metric := range []string{"task_success", "tool_calls", "resource_reads", "tool_errors", "retries", "latency_ms", "context_token_estimate"} {
		if !containsString(fixture.Metrics, metric) {
			t.Errorf("held-out metrics omit %q", metric)
		}
	}

	for _, scenario := range fixture.Scenarios {
		scenario := scenario
		t.Run(scenario.ID, func(t *testing.T) {
			reader := &heldOutReader{fakeReader: &fakeReader{searchStarted: make(chan struct{})}}
			client, closeSessions := connectHeldOut(t, reader)
			defer closeSessions()
			run := &heldOutRun{client: client, seen: make(map[string]int)}
			started := time.Now()
			success := runHeldOutOracle(t, scenario.ID, run)
			elapsed := float64(time.Since(started).Microseconds()) / 1000
			if elapsed <= 0 {
				elapsed = 0.001
			}
			metric := heldOutMetric{
				Scenario: scenario.ID, Success: success, ToolCalls: len(run.calls),
				ResourceReads: run.resourceReads,
				ToolErrors:    run.toolErrors, Retries: heldOutRetries(run.seen),
				LatencyMS: elapsed, ResponseBytes: run.responseBytes,
				ContextTokenEstimate: (run.responseBytes + 3) / 4,
			}
			if !metric.Success || metric.ToolCalls == 0 || metric.ResourceReads < 0 || metric.LatencyMS <= 0 || metric.ContextTokenEstimate == 0 {
				t.Fatalf("held-out metric failed: %+v", metric)
			}
			t.Logf("held-out metric: %s", marshalHeldOutMetric(metric))
		})
	}
}

func runHeldOutOracle(t *testing.T, scenario string, run *heldOutRun) bool {
	t.Helper()
	switch scenario {
	case "missing_coverage_recovery":
		coverage := run.tool(t, mcpcontract.ToolGetCoverage, map[string]any{
			"targets": []any{map[string]any{
				"type": "exact_thread", "repository": map[string]any{"owner": "acme", "repo": "heldout"},
				"thread": map[string]any{"kind": "pull_request", "number": 19},
			}},
		})
		if coverage == nil || coverage.IsError {
			return false
		}
		var before mcpcontract.GetCoverageOutput
		if !decodeHeldOut(coverage.StructuredContent, &before) || len(before.Items) != 1 || before.Items[0].Value == nil || before.Items[0].Value.Facets[0].Complete {
			return false
		}
		job := run.tool(t, mcpcontract.ToolSyncThreads, map[string]any{
			"selection": "repositories", "repositories": []any{map[string]any{"owner": "acme", "repo": "heldout"}},
			"kind": "pull_request", "state": "all", "max_requests": 2,
		})
		if job == nil || job.IsError {
			return false
		}
		var reference mcpcontract.JobReference
		if !decodeHeldOut(job.StructuredContent, &reference) || reference.ID == "" {
			return false
		}
		firstPoll := run.tool(t, mcpcontract.ToolGetJob, map[string]any{"ids": []any{reference.ID}, "response_format": "concise"})
		secondPoll := run.tool(t, mcpcontract.ToolGetJob, map[string]any{"ids": []any{reference.ID}, "response_format": "detailed"})
		return firstPoll != nil && secondPoll != nil && !firstPoll.IsError && !secondPoll.IsError

	case "stale_snapshot_recovery":
		stale := run.tool(t, mcpcontract.ToolSearchThreads, map[string]any{"query": "stall", "snapshot_token": "stale-token"})
		if stale == nil || !stale.IsError || agentEvalToolError(t, stale).Code != "snapshot_expired" {
			return false
		}
		fresh := run.tool(t, mcpcontract.ToolGetCoverage, map[string]any{
			"snapshot_token": "fresh-token",
			"targets":        []any{map[string]any{"type": "repository", "repository": map[string]any{"owner": "acme", "repo": "rocket"}}},
		})
		return fresh != nil && !fresh.IsError

	case "exact_resource_handoff":
		created := run.tool(t, ToolCreateConcern, map[string]any{
			"owner": "acme", "repo": "rocket", "commit_sha": "heldout-sha",
			"title": "bounded audit", "problem_statement": "stored evidence needs review", "confidence": 0.5,
		})
		if created == nil || created.IsError {
			return false
		}
		var ref mcpcontract.DurableArtifactReference
		if !decodeHeldOut(created.StructuredContent, &ref) || ref.URI == "" || !strings.HasPrefix(ref.URI, "gitcontribute://") {
			return false
		}
		resource, err := run.resource(ref.URI)
		return err == nil && resource != nil && len(resource.Contents) > 0

	case "audit_only_fix_pattern_preview":
		result := run.tool(t, mcpcontract.ToolPreviewRepositoryFixPatterns, map[string]any{
			"repository":       map[string]any{"owner": "acme", "repo": "heldout"},
			"time_window":      map[string]any{"updated_after": "2026-01-01T00:00:00Z"},
			"symptom_taxonomy": []any{map[string]any{"name": "stall", "terms": []string{"stall"}}},
			"candidate_limit":  10, "representative_limit": 2,
		})
		if result == nil || result.IsError {
			return false
		}
		var report mcpcontract.FixPatternReport
		return decodeHeldOut(result.StructuredContent, &report) && !report.Persisted && report.SnapshotToken != ""
	}
	t.Fatalf("unknown held-out scenario %q", scenario)
	return false
}

func (r *heldOutRun) tool(t *testing.T, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	r.calls = append(r.calls, name)
	r.seen[name]++
	result, err := r.client.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	if result.IsError {
		r.toolErrors++
	}
	r.responseBytes += heldOutResultBytes(result)
	return result
}

func (r *heldOutRun) resource(uri string) (*mcp.ReadResourceResult, error) {
	result, err := r.client.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: uri})
	if result != nil {
		r.resourceReads++
		r.responseBytes += len(marshalHeldOutValue(result))
	}
	return result, err
}

func heldOutRetries(seen map[string]int) int {
	total := 0
	for _, calls := range seen {
		if calls > 1 {
			total += calls - 1
		}
	}
	return total
}

func heldOutResultBytes(result *mcp.CallToolResult) int {
	if result == nil {
		return 0
	}
	return len(marshalHeldOutValue(result.StructuredContent)) + len(agentEvalResultText(result))
}

func decodeHeldOut(value any, target any) bool {
	data, err := json.Marshal(value)
	return err == nil && json.Unmarshal(data, target) == nil
}

func marshalHeldOutValue(value any) []byte {
	data, _ := json.Marshal(value)
	return data
}

func marshalHeldOutMetric(metric heldOutMetric) string {
	data, _ := json.Marshal(metric)
	return string(data)
}
