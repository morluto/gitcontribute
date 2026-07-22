package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/workspace"
)

func TestWorkspaceCreateAndShow(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	ctx := context.Background()
	remote, baseSHA, candidateSHA := setupAppGitRemote(t)

	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer func() { _ = svc.Close() }()
	if _, err := svc.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}

	inv, err := svc.StartInvestigation(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, candidateSHA, "")
	if err != nil {
		t.Fatalf("start investigation: %v", err)
	}

	t.Run("rejects credential remote before persistence", func(t *testing.T) {
		fixtureUser := strings.Join([]string{"fixture", "user"}, "-")
		fixturePassword := strings.Join([]string{"fixture", "password"}, "-")
		credentialRemote := "https://" + fixtureUser + ":" + fixturePassword + "@github.com/owner/repo.git"
		_, err := svc.CreateWorkspace(ctx, inv.ID, cli.WorkspaceCreateOptions{
			Remote: credentialRemote,
			Name:   "credential-test",
		})
		if !errors.Is(err, workspace.ErrInvalidRemote) {
			t.Fatalf("CreateWorkspace credential remote error = %v, want ErrInvalidRemote", err)
		}
		if strings.Contains(err.Error(), fixturePassword) {
			t.Fatalf("CreateWorkspace error exposed credential: %v", err)
		}

		dataDir, err := paths.DataDir()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dataDir, "workspaces", "mirrors")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("mirror directory was written before remote validation: %v", err)
		}
		c, err := svc.openCorpus(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.GetWorkspace(ctx, "credential-test"); !errors.Is(err, workspace.ErrNotFound) {
			t.Fatalf("credential remote workspace was persisted: %v", err)
		}
	})

	ws, err := svc.CreateWorkspace(ctx, inv.ID, cli.WorkspaceCreateOptions{
		Remote:       remote,
		BaseRef:      "master",
		CandidateRef: "feature",
		Name:         "ws-test",
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if ws.ID != "ws-test" || ws.InvestigationID != inv.ID || ws.BaseSHA != baseSHA || ws.CandidateSHA != candidateSHA {
		t.Fatalf("unexpected workspace: %+v", ws)
	}
	if _, err := os.Stat(ws.Path); err != nil {
		t.Fatalf("workspace path missing: %v", err)
	}

	shown, err := svc.ShowWorkspace(ctx, ws.ID)
	if err != nil {
		t.Fatalf("show workspace: %v", err)
	}
	if shown.ID != ws.ID || shown.BaseSHA != baseSHA {
		t.Fatalf("workspace roundtrip failed: %+v", shown)
	}
}
