package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// These evaluations deliberately use scripted calls. They measure the MCP
// contract an agent sees, not a model's ability to choose the right tool.
func TestAgentEvalBaselineArtifact(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(filepath.Join("testdata", "agent-eval", "baseline.json"))
	if err != nil {
		t.Fatal(err)
	}
	var baseline struct {
		Version   string `json:"version"`
		Scenarios []struct {
			Name    string `json:"name"`
			Metrics struct {
				Completed             bool `json:"completed"`
				ToolCalls             int  `json:"tool_calls"`
				ToolErrors            int  `json:"tool_errors"`
				InvalidArgumentErrors int  `json:"invalid_argument_errors"`
				ResponseBytes         int  `json:"response_bytes"`
				PollCalls             int  `json:"poll_calls"`
			} `json:"metrics"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(data, &baseline); err != nil {
		t.Fatalf("decode baseline: %v", err)
	}
	if baseline.Version != "agent-tool-eval.v1" {
		t.Fatalf("baseline version = %q", baseline.Version)
	}
	if len(baseline.Scenarios) < 3 {
		t.Fatalf("baseline has %d scenarios, want at least 3", len(baseline.Scenarios))
	}
	seen := map[string]bool{}
	for _, scenario := range baseline.Scenarios {
		if scenario.Name == "" || seen[scenario.Name] {
			t.Fatalf("missing or duplicate scenario name %q", scenario.Name)
		}
		seen[scenario.Name] = true
		if scenario.Metrics.ToolCalls < 1 || scenario.Metrics.ResponseBytes < 1 {
			t.Fatalf("scenario %q has incomplete metrics: %+v", scenario.Name, scenario.Metrics)
		}
	}
}

func TestAgentEvalScriptedCurrentContracts(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	t.Run("known repository search is one bounded structured call", func(t *testing.T) {
		result := callAgentEvalTool(t, client, ToolSearchRepositories, map[string]any{
			"owner": "acme", "repo": "rocket", "limit": 5,
		})
		if result.IsError || result.StructuredContent == nil {
			t.Fatalf("search result = %+v", result)
		}
	})

	t.Run("ambiguous repository identity is rejected visibly", func(t *testing.T) {
		result, err := client.CallTool(context.Background(), &mcp.CallToolParams{
			Name: ToolSearchRepositories, Arguments: map[string]any{"owner": "acme"},
		})
		if err == nil && (result == nil || !result.IsError) {
			t.Fatalf("partial repository identity was accepted: result=%+v err=%v", result, err)
		}
		if result != nil && agentEvalToolError(t, result).Code != "invalid_argument" {
			t.Fatalf("error is not actionable: %+v", result.Content)
		}
	})

	t.Run("raw and structured search modes are rejected", func(t *testing.T) {
		result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: ToolSearchGitHubRepositories, Arguments: map[string]any{"raw_query": "topic:cuda", "language": "Go"}})
		if err == nil && (result == nil || !result.IsError) {
			t.Fatalf("ambiguous search was accepted: result=%+v err=%v", result, err)
		}
		if result != nil && agentEvalToolError(t, result).Code != "invalid_argument" {
			t.Fatalf("error is not actionable: %+v", result.Content)
		}
	})

	t.Run("durable operation exposes a pollable job", func(t *testing.T) {
		started := callAgentEvalTool(t, client, ToolBuildRepositoryDossier, map[string]any{"owner": "acme", "repo": "rocket"})
		payload, err := json.Marshal(started.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var job JobReference
		if err := json.Unmarshal(payload, &job); err != nil {
			t.Fatal(err)
		}
		if job.ID == "" || job.Ref != "job:"+job.ID || job.Status == "" || job.PollAfterMS < 1 || len(job.SuggestedActions) != 1 || job.SuggestedActions[0].Tool != ToolGetJob {
			t.Fatalf("job reference is not pollable: %+v", job)
		}
		polled := callAgentEvalTool(t, client, ToolGetJob, map[string]any{"ids": []string{job.ID}})
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

func agentEvalToolError(t *testing.T, result *mcp.CallToolResult) ToolError {
	t.Helper()
	for _, content := range result.Content {
		text, ok := content.(*mcp.TextContent)
		if !ok {
			continue
		}
		var toolError ToolError
		if json.Unmarshal([]byte(text.Text), &toolError) == nil && toolError.Code != "" {
			return toolError
		}
	}
	t.Fatalf("tool error content is not a ToolError JSON object: %+v", result.Content)
	return ToolError{}
}
