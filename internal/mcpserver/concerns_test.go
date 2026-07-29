package mcpserver

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestConcernToolsExposeBoundedLocalContracts(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()
	tools := map[string]*mcp.Tool{}
	for tool, err := range client.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		tools[tool.Name] = tool
	}
	for _, name := range []string{ToolListConcerns, ToolCreateConcern, ToolUpdateConcern, ToolSetConcernState, ToolLinkConcern, ToolPromoteConcern} {
		if tools[name] == nil {
			t.Fatalf("missing concern tool %q", name)
		}
	}
	assertSchemaValue(t, tools[ToolListConcerns].InputSchema, []string{"properties", "limit", "maximum"}, float64(100))
	assertSchemaValue(t, tools[ToolCreateConcern].InputSchema, []string{"properties", "source_provenance", "maxItems"}, float64(100))
	listed, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: ToolListConcerns})
	if err != nil || listed.IsError {
		t.Fatalf("list concerns: err=%v result=%+v", err, listed)
	}
	content, ok := listed.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("list concerns structured content = %T", listed.StructuredContent)
	}
	concerns, ok := content["concerns"].([]any)
	if !ok || len(concerns) != 1 {
		t.Fatalf("list concerns summaries = %#v", content["concerns"])
	}
	summary, ok := concerns[0].(map[string]any)
	if !ok || summary["uri"] != "gitcontribute://concern/concern-1" {
		t.Fatalf("list concern summary = %#v", concerns[0])
	}
	for _, field := range []string{"problem_statement", "notes", "links", "promotion"} {
		if _, exists := summary[field]; exists {
			t.Fatalf("compact concern summary contains %q: %#v", field, summary)
		}
	}
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: ToolCreateConcern, Arguments: map[string]any{
		"owner": "owner", "repo": "repo", "commit_sha": "abc", "title": "flaky", "problem_statement": "intermittent", "confidence": 0.5,
	}})
	if err != nil || result.IsError || result.StructuredContent == nil {
		t.Fatalf("create concern: err=%v result=%+v", err, result)
	}
}
