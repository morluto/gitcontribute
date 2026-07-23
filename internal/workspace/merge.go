package workspace

import (
	"context"
	"errors"
	"strings"
)

// MergeCheck is a non-mutating comparison of two already-fetched revisions.
type MergeCheck struct {
	MergeBase  string
	Conflicted bool
	Summary    string
}

// CheckMerge compares already-fetched revisions without fetching or changing
// refs, the index, or a worktree.
func (m *Manager) CheckMerge(ctx context.Context, path, baseOID, headOID string) (MergeCheck, error) {
	return m.checkMerge(ctx, path, baseOID, headOID)
}

// CheckMergeWorkspace revalidates workspace authority before comparing refs.
func (m *Manager) CheckMergeWorkspace(ctx context.Context, ws *Workspace, baseOID, headOID string) (MergeCheck, error) {
	path, err := m.authorizedPath(ctx, ws)
	if err != nil {
		return MergeCheck{}, err
	}
	return m.checkMerge(ctx, path, baseOID, headOID)
}

func (m *Manager) checkMerge(ctx context.Context, path, baseOID, headOID string) (MergeCheck, error) {
	baseOID, headOID = strings.TrimSpace(baseOID), strings.TrimSpace(headOID)
	if baseOID == "" || headOID == "" {
		return MergeCheck{}, errors.New("base and head OIDs are required")
	}
	mergeBase, err := m.git(ctx, path, "merge-base", baseOID, headOID)
	if err != nil {
		return MergeCheck{}, err
	}
	mergeBase = strings.TrimSpace(mergeBase)
	out, err := m.git(ctx, path, "merge-tree", mergeBase, baseOID, headOID)
	if err != nil {
		return MergeCheck{}, err
	}
	conflicted := strings.Contains(out, "changed in both") || strings.Contains(out, "<<<<<<<") || strings.Contains(out, "CONFLICT")
	summary := "revisions merge cleanly"
	if conflicted {
		summary = "revisions have merge conflicts"
	}
	return MergeCheck{MergeBase: mergeBase, Conflicted: conflicted, Summary: summary}, nil
}
