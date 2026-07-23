package app

import (
	"context"
	"errors"

	"github.com/morluto/gitcontribute/internal/commitplan"
)

// InspectCommitChanges returns stable assignable units for a managed workspace.
// It reads only the local worktree and Git object database.
func (s *Service) InspectCommitChanges(ctx context.Context, workspaceID string) (commitplan.Inventory, error) {
	return s.commitPlanInventory(ctx, workspaceID)
}

// PlanSemanticCommits validates agent-authored semantic groups against the
// current workspace snapshot. It never stages changes or rewrites history.
func (s *Service) PlanSemanticCommits(ctx context.Context, workspaceID, expectedInventorySHA256 string, input commitplan.PlanInput) (commitplan.Plan, error) {
	inventory, err := s.commitPlanInventory(ctx, workspaceID)
	if err != nil {
		return commitplan.Plan{}, err
	}
	if expectedInventorySHA256 != "" && expectedInventorySHA256 != inventory.InventorySHA256 {
		return commitplan.Plan{}, errors.New("workspace diff changed after inspection; inspect again before planning")
	}
	return commitplan.Build(ctx, inventory, input)
}

func (s *Service) commitPlanInventory(ctx context.Context, workspaceID string) (commitplan.Inventory, error) {
	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return commitplan.Inventory{}, err
	}
	ws, err := c.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return commitplan.Inventory{}, mapWorkspaceError(err)
	}
	mgr, err := s.workspaceReader()
	if err != nil {
		return commitplan.Inventory{}, err
	}
	patch, err := mgr.DiffByPath(ctx, ws.Path, ws.BaseSHA)
	if err != nil {
		return commitplan.Inventory{}, err
	}
	untracked, err := mgr.UntrackedFilesByPath(ctx, ws.Path)
	if err != nil {
		return commitplan.Inventory{}, err
	}
	files := make([]commitplan.UntrackedFile, len(untracked))
	for index, file := range untracked {
		files[index] = commitplan.UntrackedFile{Path: file.Path, ObjectID: file.ObjectID}
	}
	return commitplan.Inspect(ctx, commitplan.Snapshot{Patch: []byte(patch), Untracked: files})
}
