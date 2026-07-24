package mcpserver

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func TestSemanticCommitToolsAreReadOnlyAndSequenced(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()
	tools := map[string]*mcp.Tool{}
	for tool, err := range client.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		tools[tool.Name] = tool
	}
	for _, name := range []string{mcpcontract.ToolInspectCommitChanges, mcpcontract.ToolPlanSemanticCommits} {
		tool := tools[name]
		if tool == nil || tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Fatalf("tool %q is missing or not read-only: %+v", name, tool)
		}
	}
	assertSchemaValue(t, tools[mcpcontract.ToolPlanSemanticCommits].InputSchema, []string{"properties", "groups", "maxItems"}, float64(100))
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: mcpcontract.ToolInspectCommitChanges, Arguments: map[string]any{"workspace_id": "ws-1"}})
	if err != nil || result.IsError || result.StructuredContent == nil {
		t.Fatalf("inspect commit changes: err=%v result=%+v", err, result)
	}
}
