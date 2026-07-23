package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/mcpserver"
	"github.com/morluto/gitcontribute/internal/workspace"
)

func TestMCPValidationResolvesManagedWorkspaceAndRejectsCrossInvestigation(t *testing.T) {
	ctx := context.Background()
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	if _, err := svc.Init(ctx); err != nil {
		t.Fatal(err)
	}
	inv, err := svc.StartInvestigation(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, "abc123", "")
	if err != nil {
		t.Fatal(err)
	}
	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dataDir, err := paths.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dataDir, "workspaces", "workspaces", "managed")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := c.SaveWorkspace(ctx, &workspace.Workspace{Name: "managed", InvestigationID: inv.ID, RepoOwner: "owner", RepoName: "repo", Path: path, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	reader := &MCPReader{Service: svc}
	defined, err := reader.DefineValidation(ctx, mcpserver.DefineValidationInput{InvestigationID: inv.ID, Kind: "test", Command: "go test ./...", WorkspaceID: "managed"})
	if err != nil {
		t.Fatal(err)
	}
	if defined.WorkingDir != path {
		t.Fatalf("working directory = %q, want managed path %q", defined.WorkingDir, path)
	}
	other, err := svc.StartInvestigation(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, "def456", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.DefineValidation(ctx, mcpserver.DefineValidationInput{InvestigationID: other.ID, Kind: "test", Command: "go test ./...", WorkspaceID: "managed"}); err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("cross-investigation validation error = %v", err)
	}
}

func TestDefineValidationParsesQuotedArguments(t *testing.T) {
	ctx := context.Background()
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	if _, err := svc.Init(ctx); err != nil {
		t.Fatal(err)
	}
	inv, err := svc.StartInvestigation(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, "abc123", "")
	if err != nil {
		t.Fatal(err)
	}

	def, err := svc.DefineValidation(ctx, inv.ID, cli.DefineValidationOptions{
		Kind: "test", Command: `printf '%s value' ok`, WorkingDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"printf", "%s value", "ok"}, def.Command); diff != "" {
		t.Fatalf("command argv mismatch (-want +got):\n%s", diff)
	}
}
