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
	// The three-tree form reads only existing objects. Its output includes
	// arbitrary file content, so recognize only a conflict heading followed by
	// Git's object metadata rather than matching a marker or message anywhere
	// in the diff.
	out, err := m.git(ctx, path, "merge-tree", mergeBase, baseOID, headOID)
	if err != nil {
		return MergeCheck{}, err
	}
	conflicted := legacyMergeTreeConflicted(out)
	summary := "revisions merge cleanly"
	if conflicted {
		summary = "revisions have merge conflicts"
	}
	return MergeCheck{MergeBase: mergeBase, Conflicted: conflicted, Summary: summary}, nil
}

func legacyMergeTreeConflicted(out string) bool {
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		switch line {
		case "changed in both", "added in both", "removed in local", "removed in remote":
			if hasMergeTreeObjectMetadata(lines[i+1:]) {
				return true
			}
		}
	}
	return false
}

func hasMergeTreeObjectMetadata(lines []string) bool {
	metadataLines := 0
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 4 || (fields[0] != "base" && fields[0] != "our" && fields[0] != "their") || !isFileMode(fields[1]) || !isObjectID(fields[2]) {
			break
		}
		metadataLines++
	}
	return metadataLines >= 2
}

func isFileMode(value string) bool {
	if len(value) != 6 {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '7' {
			return false
		}
	}
	return true
}

func isObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
