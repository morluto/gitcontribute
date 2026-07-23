package mcpserver

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRepeatedValidationAppliesDocumentedDefaults(t *testing.T) {
	fake := &fakeReader{searchStarted: make(chan struct{})}
	client, closeSessions := connect(t, fake)
	defer closeSessions()
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      ToolRunRepeatedValidation,
		Arguments: map[string]any{"id": "val-1", "target": "candidate", "execute": true},
	})
	if err != nil || result.IsError {
		t.Fatalf("call repeated validation: err=%v result=%+v", err, result)
	}
	if fake.repeatInput.RunCount != 3 || fake.repeatInput.Concurrency != 1 || fake.repeatInput.SampleInterval != "100ms" {
		t.Fatalf("repeat defaults = %+v", fake.repeatInput)
	}
}
