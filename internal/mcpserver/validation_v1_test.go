package mcpserver

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func TestRunValidationAppliesDocumentedDefaults(t *testing.T) {
	fake := &fakeReader{searchStarted: make(chan struct{})}
	client, closeSessions := connect(t, fake)
	defer closeSessions()
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      mcpcontract.ToolRunValidation,
		Arguments: map[string]any{"id": "val-1", "target": "candidate", "execute": true},
	})
	if err != nil || result.IsError {
		t.Fatalf("call validation: err=%v result=%+v", err, result)
	}
	if fake.validationInput.RunCount != 1 || fake.validationInput.Concurrency != 1 || fake.validationInput.SampleInterval != "100ms" {
		t.Fatalf("validation defaults = %+v", fake.validationInput)
	}
}
