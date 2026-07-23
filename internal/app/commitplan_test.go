package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/commitplan"
)

func TestSemanticCommitPlanUsesFrozenWorkspaceSnapshot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	ctx := context.Background()
	svc := newLocalService(t)
	defer func() { _ = svc.Close() }()
	remote, _, candidateSHA := setupAppGitRemote(t)
	inv, err := svc.StartInvestigation(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, candidateSHA, "")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := svc.CreateWorkspace(ctx, inv.ID, cli.WorkspaceCreateOptions{Remote: remote, BaseRef: "master", CandidateRef: "feature", Name: "commit-plan"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws.Path, "untracked.txt"), []byte("proof\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inventory, err := svc.InspectCommitChanges(ctx, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Units) < 2 {
		t.Fatalf("inventory = %+v", inventory)
	}
	ids := make([]string, len(inventory.Units))
	for index := range inventory.Units {
		ids[index] = inventory.Units[index].ID
	}
	plan, err := svc.PlanSemanticCommits(ctx, ws.ID, inventory.InventorySHA256, commitplan.PlanInput{Groups: []commitplan.GroupInput{{
		Name: "feature", Intent: "add feature", Type: "feat", UnitIDs: ids,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Reconstruction.Verified {
		t.Fatalf("plan = %+v", plan)
	}
	if err := os.WriteFile(filepath.Join(ws.Path, "changed-after-inspection.txt"), []byte("later\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PlanSemanticCommits(ctx, ws.ID, inventory.InventorySHA256, commitplan.PlanInput{}); err == nil {
		t.Fatal("expected stale snapshot rejection")
	}
}
