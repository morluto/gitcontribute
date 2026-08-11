package workspace

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// UntrackedFileSnapshot identifies exact untracked content without returning
// file bytes. ObjectID is Git's content hash from hash-object --no-filters.
type UntrackedFileSnapshot struct {
	Path     string
	ObjectID string
}

// UntrackedFilesByPath returns a bounded, deterministic snapshot of untracked,
// non-ignored files. Git performs path discovery and content hashing.
func (m *Manager) UntrackedFilesByPath(ctx context.Context, path string) ([]UntrackedFileSnapshot, error) {
	managed, err := m.managedPath(path)
	if err != nil {
		return nil, err
	}
	out, err := m.git(ctx, managed, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, fmt.Errorf("list untracked files: %w", err)
	}
	parts := strings.Split(out, "\x00")
	files := make([]UntrackedFileSnapshot, 0, len(parts))
	for _, file := range parts {
		if file == "" {
			continue
		}
		if len(files) == maxUntrackedFiles {
			return nil, fmt.Errorf("workspace exceeds %d untracked files", maxUntrackedFiles)
		}
		objectID, err := m.git(ctx, managed, "hash-object", "--no-filters", "--", file)
		if err != nil {
			return nil, fmt.Errorf("hash untracked file %q: %w", file, err)
		}
		files = append(files, UntrackedFileSnapshot{Path: file, ObjectID: strings.TrimSpace(objectID)})
	}
	return files, nil
}

// ChangedFilesByPath returns raw Git paths changed from the supplied base.
// Git owns rename, deletion, and quoted-path handling; NUL delimiters preserve
// paths containing whitespace or other special characters.
func (m *Manager) ChangedFilesByPath(ctx context.Context, path, baseSHA string) ([]string, error) {
	managed, err := m.managedPath(path)
	if err != nil {
		return nil, err
	}
	return m.changedFilesByPath(ctx, managed, baseSHA)
}

// ChangedFilesWorkspace revalidates workspace authority before listing files.
func (m *Manager) ChangedFilesWorkspace(ctx context.Context, ws *Workspace) ([]string, error) {
	path, err := m.authorizedPath(ctx, ws)
	if err != nil {
		return nil, err
	}
	return m.changedFilesByPath(ctx, path, ws.BaseSHA)
}

func (m *Manager) changedFilesByPath(ctx context.Context, path, baseSHA string) ([]string, error) {
	args := []string{"diff", "--name-only", "--find-renames", "-z"}
	if baseSHA != "" {
		args = append(args, baseSHA)
	}
	args = append(args, "--")
	out, err := m.git(ctx, path, args...)
	if err != nil {
		return nil, fmt.Errorf("list changed files: %w", err)
	}
	parts := strings.Split(out, "\x00")
	files := make([]string, 0, len(parts))
	for _, path := range parts {
		if path != "" {
			files = append(files, path)
		}
	}
	return files, nil
}

// HasUntrackedByPath reports whether a managed workspace contains untracked,
// non-ignored files. Callers preparing a complete diff must handle these
// explicitly because git diff does not include them.
func (m *Manager) HasUntrackedByPath(ctx context.Context, path string) (bool, error) {
	managed, err := m.managedPath(path)
	if err != nil {
		return false, err
	}
	return m.hasUntracked(ctx, managed)
}

// HasUntrackedWorkspace revalidates workspace authority before reading files.
func (m *Manager) HasUntrackedWorkspace(ctx context.Context, ws *Workspace) (bool, error) {
	path, err := m.authorizedPath(ctx, ws)
	if err != nil {
		return false, err
	}
	return m.hasUntracked(ctx, path)
}

func (m *Manager) hasUntracked(ctx context.Context, path string) (bool, error) {
	out, err := m.git(ctx, path, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return false, fmt.Errorf("list untracked files: %w", err)
	}
	return len(out) > 0, nil
}

// ValidateWorkspacePath verifies that path exists within the managed worktree
// subtree without invoking Git or changing filesystem state. Mirrors and other
// manager state are deliberately excluded from executable capabilities.
func (m *Manager) ValidateWorkspacePath(path string) error {
	resolved, err := m.managedPath(path)
	if err != nil {
		return err
	}
	if !containsPath(filepath.Join(m.root, "workspaces"), resolved) {
		return ErrNotManaged
	}
	return nil
}

func (m *Manager) managedPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(abs))
	if err != nil {
		return "", fmt.Errorf("resolve managed path symlinks: %w", err)
	}
	if !m.contains(resolved) {
		return "", ErrNotManaged
	}
	return resolved, nil
}

func (m *Manager) contains(path string) bool {
	return containsPath(m.root, path)
}

func containsPath(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
