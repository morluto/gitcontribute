package app

import (
	"context"
	"testing"

	"github.com/morluto/gitcontribute/internal/failure"
)

func TestWorkspaceResourceMapsMissingWorkspaceToNotFound(t *testing.T) {
	svc := newLocalService(t)
	t.Cleanup(func() { _ = svc.Close() })
	_, err := (&MCPReader{Service: svc}).Workspace(context.Background(), "missing")
	if !failure.Is(err, failure.KindNotFound) {
		t.Fatalf("Workspace error = %v, want not_found", err)
	}
}
