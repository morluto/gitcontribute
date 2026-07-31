package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

const agentEvalV5Root = "testdata/agent-eval/v5"

func TestAgentEvalV5UnifiedCatalogDecisionFixtures(t *testing.T) {
	t.Parallel()
	type condition struct {
		ID          string `json:"id"`
		CatalogMode string `json:"catalog_mode"`
		LoadingMode string `json:"loading_mode"`
	}
	type scenario struct {
		ID     string `json:"id"`
		Prompt string `json:"prompt"`
	}
	var public struct {
		Version                   string `json:"version"`
		FixtureRevision           string `json:"fixture_revision"`
		MinimumTrialsPerCondition int    `json:"minimum_trials_per_condition"`
		ExternalOracle            struct {
			Version string `json:"version"`
			SHA256  string `json:"sha256"`
		} `json:"external_oracle"`
		ControlledVariables []string    `json:"controlled_variables"`
		Conditions          []condition `json:"conditions"`
		Scenarios           []scenario  `json:"scenarios"`
		SemanticMetrics     []string    `json:"semantic_metrics"`
		EfficiencyMetrics   []string    `json:"efficiency_metrics"`
	}
	data, err := os.ReadFile(filepath.Join(agentEvalV5Root, "public.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &public); err != nil {
		t.Fatal(err)
	}
	if public.Version != "agent-tool-eval.v5" ||
		public.FixtureRevision != "unified-catalog-decision-v1" ||
		public.MinimumTrialsPerCondition < 3 ||
		public.ExternalOracle.Version != "agent-tool-eval-oracle.v5" ||
		len(public.ExternalOracle.SHA256) != 64 {
		t.Fatalf("incomplete public v5 fixture: %+v", public)
	}
	for _, leaked := range []string{"acceptable_tools", "hard_failures"} {
		if strings.Contains(string(data), leaked) {
			t.Fatalf("public v5 fixture leaks oracle field %q", leaked)
		}
	}
	for _, required := range []string{
		"model", "sampling_settings", "catalog_fingerprint", "snapshot_token",
		"permissions", "task_prompt", "token_budget",
	} {
		if !containsString(public.ControlledVariables, required) {
			t.Errorf("controlled variables omit %q", required)
		}
	}
	wantConditions := map[string]condition{
		"unified_eager":       {ID: "unified_eager", CatalogMode: "all", LoadingMode: "eager"},
		"unified_host_search": {ID: "unified_host_search", CatalogMode: "all", LoadingMode: "host_tool_search"},
	}
	if len(public.Conditions) != len(wantConditions) {
		t.Fatalf("conditions = %+v", public.Conditions)
	}
	for _, got := range public.Conditions {
		if want, ok := wantConditions[got.ID]; !ok || got != want {
			t.Errorf("unexpected condition %+v", got)
		}
	}
	scenarioIDs := make(map[string]bool, len(public.Scenarios))
	for _, scenario := range public.Scenarios {
		if scenario.ID == "" || scenarioIDs[scenario.ID] || strings.TrimSpace(scenario.Prompt) == "" {
			t.Fatalf("incomplete or duplicate scenario %+v", scenario)
		}
		scenarioIDs[scenario.ID] = true
	}
	for _, required := range []string{"task_success", "side_effect_correctness"} {
		if !containsString(public.SemanticMetrics, required) {
			t.Errorf("semantic metrics omit %q", required)
		}
	}
	for _, required := range []string{
		"initial_tool_context_tokens", "loaded_tool_context_tokens",
		"tool_search_calls", "operational_tool_calls",
	} {
		if !containsString(public.EfficiencyMetrics, required) {
			t.Errorf("efficiency metrics omit %q", required)
		}
	}
	if _, err := os.Stat(filepath.Join(agentEvalV5Root, "private", "oracle.json")); !os.IsNotExist(err) {
		t.Fatalf("v5 oracle must remain out of band, stat error = %v", err)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestAgentEvalScriptedCurrentContracts(t *testing.T) {
	client, closeSessions := connect(t, completeFakeReader(&fakeReader{searchStarted: make(chan struct{})}))
	defer closeSessions()

	t.Run("known repository search is one bounded structured call", func(t *testing.T) {
		result := callAgentEvalTool(t, client, mcpcontract.ToolSearchRepositories, map[string]any{
			"owner": "acme", "repo": "rocket", "limit": 5,
		})
		if result.IsError || result.StructuredContent == nil {
			t.Fatalf("search result = %+v", result)
		}
	})

	t.Run("exact issue-set preparation is one offline aggregate call", func(t *testing.T) {
		result := callAgentEvalTool(t, client, mcpcontract.ToolPrepareIssueSet, map[string]any{
			"owner": "acme", "repo": "rocket", "issue_numbers": []int{7, 11, 14},
		})
		if result.IsError || result.StructuredContent == nil {
			t.Fatalf("issue-set result = %+v; content = %q", result, agentEvalResultText(result))
		}
	})

	t.Run("repository and dossier availability is one offline batch", func(t *testing.T) {
		result := callAgentEvalTool(t, client, mcpcontract.ToolGetRepositories, map[string]any{
			"repositories": []map[string]any{
				{"owner": "acme", "repo": "rocket"},
				{"owner": "acme", "repo": "player"},
				{"owner": "acme", "repo": "synth"},
			},
		})
		payload, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var output mcpcontract.GetRepositoriesOutput
		if err := json.Unmarshal(payload, &output); err != nil {
			t.Fatal(err)
		}
		if result.IsError || output.Status != "complete" || len(output.Items) != 3 {
			t.Fatalf("repository batch result = %+v", result)
		}
		if output.Items[0].Value == nil || output.Items[0].Value.DossierStatus != "available" ||
			output.Items[1].Value == nil || output.Items[1].Value.DossierStatus != "missing" {
			t.Fatalf("dossier availability is not explicit: %+v", output.Items)
		}
	})

	t.Run("ambiguous repository identity is rejected visibly", func(t *testing.T) {
		result, err := client.CallTool(context.Background(), &mcp.CallToolParams{
			Name: mcpcontract.ToolSearchRepositories, Arguments: map[string]any{"owner": "acme"},
		})
		if err == nil && (result == nil || !result.IsError) {
			t.Fatalf("partial repository identity was accepted: result=%+v err=%v", result, err)
		}
		if result != nil && agentEvalToolError(t, result).Code != "invalid_argument" {
			t.Fatalf("error is not actionable: %+v", result.Content)
		}
	})

	t.Run("raw and structured search modes are rejected", func(t *testing.T) {
		result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: mcpcontract.ToolSearchGitHubRepositories, Arguments: map[string]any{"raw_query": "topic:cuda", "language": "Go"}})
		if err == nil && (result == nil || !result.IsError) {
			t.Fatalf("ambiguous search was accepted: result=%+v err=%v", result, err)
		}
		if result != nil {
			message := agentEvalResultText(result)
			if !strings.Contains(message, "github-search-raw-query") || !strings.Contains(message, "github-search-structured") {
				t.Fatalf("schema error does not identify the rejected modes: %q", message)
			}
		}
	})

	t.Run("durable operation exposes a pollable job", func(t *testing.T) {
		started := callAgentEvalTool(t, client, mcpcontract.ToolBuildRepositoryDossier, map[string]any{"owner": "acme", "repo": "rocket"})
		payload, err := json.Marshal(started.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var job mcpcontract.JobReference
		if err := json.Unmarshal(payload, &job); err != nil {
			t.Fatal(err)
		}
		if job.ID == "" || job.Ref != "job:"+job.ID || job.Status == "" || job.PollAfterMS < 1 || job.FollowUp == nil || job.FollowUp.Action.Type != "poll_job" {
			t.Fatalf("job reference is not pollable: %+v", job)
		}
		polled := callAgentEvalTool(t, client, mcpcontract.ToolGetJob, map[string]any{"ids": []string{job.ID}})
		if polled.IsError || polled.StructuredContent == nil {
			t.Fatalf("poll result = %+v", polled)
		}
	})
}

func TestAgentEvalToolSchemasAreLegible(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	for tool, err := range client.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("list tools: %v", err)
		}
		t.Run(tool.Name, func(t *testing.T) {
			if strings.TrimSpace(tool.Description) == "" {
				t.Fatal("description is empty")
			}
			schema, ok := tool.InputSchema.(map[string]any)
			if !ok {
				payload, err := json.Marshal(tool.InputSchema)
				if err != nil {
					t.Fatalf("marshal input schema: %v", err)
				}
				if err := json.Unmarshal(payload, &schema); err != nil {
					t.Fatalf("decode input schema: %v", err)
				}
			}
			if schema["type"] != "object" {
				t.Errorf("root schema type = %v, want object", schema["type"])
			}
			if _, exists := schema["allOf"]; exists {
				t.Error("root schema uses allOf; clients may render this as an unreadable intersection")
			}
			properties, ok := schema["properties"].(map[string]any)
			if !ok && schema["properties"] != nil {
				t.Fatal("schema properties are opaque")
			}
			for name, value := range properties {
				property, ok := value.(map[string]any)
				if !ok || len(property) == 0 {
					t.Errorf("property %q has an empty or opaque schema", name)
					continue
				}
				if strings.TrimSpace(agentEvalString(property["description"])) == "" {
					t.Errorf("property %q has no description", name)
				}
			}
		})
	}
}

func callAgentEvalTool(t *testing.T, client *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	return result
}

func agentEvalString(value any) string {
	text, _ := value.(string)
	return text
}

func agentEvalResultText(result *mcp.CallToolResult) string {
	var parts []string
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func agentEvalToolError(t *testing.T, result *mcp.CallToolResult) mcpcontract.ToolError {
	t.Helper()
	for _, content := range result.Content {
		text, ok := content.(*mcp.TextContent)
		if !ok {
			continue
		}
		var toolError mcpcontract.ToolError
		if json.Unmarshal([]byte(text.Text), &toolError) == nil && toolError.Code != "" {
			return toolError
		}
	}
	t.Fatalf("tool error content is not a ToolError JSON object: %+v", result.Content)
	return mcpcontract.ToolError{}
}
