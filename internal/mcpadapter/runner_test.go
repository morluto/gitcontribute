package mcpadapter

import (
	"context"
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/contracts"
)

func TestRunnerRejectsUnsupportedTransportBeforeUsingService(t *testing.T) {
	err := New(nil, "test").Run(context.Background(), contracts.MCPOptions{Transport: "http"})
	if err == nil || !strings.Contains(err.Error(), `unsupported mcp transport "http"`) {
		t.Fatalf("run error = %v", err)
	}
}

func TestRunnerRejectsMissingService(t *testing.T) {
	err := New(nil, "test").Run(context.Background(), contracts.MCPOptions{Transport: "stdio"})
	if err == nil || !strings.Contains(err.Error(), "mcp service is required") {
		t.Fatalf("run error = %v", err)
	}
}
