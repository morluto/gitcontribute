package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Adopt inspects an existing worktree without fetching, changing refs, or
// taking ownership of the path. The returned administrative identities are
// revalidated by later workspace operations.
func (m *Manager) Adopt(ctx context.Context, opts AdoptOptions) (*Workspace, error) {
	if err := validateName(opts.Name); err != nil {
		return nil, err
	}
	if strings.TrimSpace(opts.BaseRef) == "" {
		return nil, errors.New("base ref is required")
	}
	canonical, err := canonicalDirectory(opts.Path)
	if err != nil {
		return nil, err
	}
	record, err := m.findWorktreeRecord(ctx, canonical)
	if err != nil {
		return nil, err
	}
	inside, err := m.git(ctx, canonical, "rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(inside) != "true" {
		return nil, errors.New("path is not a Git worktree")
	}
	bare, err := m.git(ctx, canonical, "rev-parse", "--is-bare-repository")
	if err != nil || strings.TrimSpace(bare) != "false" {
		return nil, errors.New("bare repositories cannot be adopted")
	}
	topLevel, err := m.git(ctx, canonical, "rev-parse", "--path-format=absolute", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("resolve worktree root: %w", err)
	}
	if cleanGitPath(topLevel) != canonical {
		return nil, errors.New("path must be the worktree root")
	}
	gitDir, err := m.gitPath(ctx, canonical, "--absolute-git-dir")
	if err != nil {
		return nil, err
	}
	commonDir, err := m.gitPath(ctx, canonical, "--git-common-dir")
	if err != nil {
		return nil, err
	}
	candidateSHA, err := m.resolvePathRef(ctx, canonical, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("resolve candidate: %w", err)
	}
	if record.head != candidateSHA {
		return nil, fmt.Errorf("%w: worktree HEAD changed during adoption", ErrPathChanged)
	}
	baseSHA, err := m.resolvePathRef(ctx, canonical, opts.BaseRef)
	if err != nil {
		return nil, fmt.Errorf("resolve base: %w", err)
	}
	mergeBase, err := m.git(ctx, canonical, "merge-base", baseSHA, candidateSHA)
	if err != nil {
		return nil, fmt.Errorf("merge-base: %w", err)
	}
	remote, err := m.validateRemotes(ctx, canonical)
	if err != nil {
		return nil, err
	}
	status, err := m.status(ctx, canonical)
	if err != nil {
		return nil, err
	}
	hasUntracked, err := m.hasUntracked(ctx, canonical)
	if err != nil {
		return nil, err
	}
	return &Workspace{
		Name: opts.Name, Path: canonical, Remote: remote,
		BaseSHA: baseSHA, CandidateSHA: candidateSHA, MergeBase: strings.TrimSpace(mergeBase),
		Dirty: status.Dirty, HasUntracked: hasUntracked, Ownership: OwnershipExternal,
		GitDir: gitDir, GitCommonDir: commonDir, CreatedAt: time.Now().UTC(),
	}, nil
}

type worktreeRecord struct {
	path string
	head string
}

func (m *Manager) findWorktreeRecord(ctx context.Context, canonical string) (worktreeRecord, error) {
	out, err := m.git(ctx, canonical, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return worktreeRecord{}, fmt.Errorf("list Git worktrees: %w", err)
	}
	var current worktreeRecord
	for _, field := range strings.Split(out, "\x00") {
		switch {
		case field == "":
			if current.path == canonical && current.head != "" {
				return current, nil
			}
			current = worktreeRecord{}
		case strings.HasPrefix(field, "worktree "):
			path, err := canonicalDirectory(strings.TrimPrefix(field, "worktree "))
			if err == nil {
				current.path = path
			}
		case strings.HasPrefix(field, "HEAD "):
			current.head = strings.TrimPrefix(field, "HEAD ")
		case field == "bare" || strings.HasPrefix(field, "prunable"):
			current = worktreeRecord{}
		}
	}
	return worktreeRecord{}, errors.New("path is not a registered Git worktree")
}

func canonicalDirectory(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("workspace path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve workspace path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(abs))
	if err != nil {
		return "", fmt.Errorf("resolve workspace path symlinks: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat workspace path: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("workspace path is not a directory")
	}
	return resolved, nil
}

func cleanGitPath(output string) string {
	return filepath.Clean(strings.TrimSuffix(strings.TrimSuffix(output, "\n"), "\r"))
}

func (m *Manager) gitPath(ctx context.Context, worktree, arg string) (string, error) {
	out, err := m.git(ctx, worktree, "rev-parse", "--path-format=absolute", arg)
	if err != nil {
		return "", fmt.Errorf("resolve Git administrative path: %w", err)
	}
	path := cleanGitPath(out)
	if !filepath.IsAbs(path) {
		path = filepath.Join(worktree, path)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve Git administrative path symlinks: %w", err)
	}
	return filepath.Clean(resolved), nil
}

func (m *Manager) resolvePathRef(ctx context.Context, path, ref string) (string, error) {
	out, err := m.git(ctx, path, "rev-parse", "--verify", "--end-of-options", strings.TrimSpace(ref)+"^{commit}")
	if err != nil {
		return "", ErrNotFound
	}
	return strings.TrimSpace(out), nil
}

func (m *Manager) validateRemotes(ctx context.Context, path string) (string, error) {
	out, err := m.git(ctx, path, "remote")
	if err != nil {
		return "", fmt.Errorf("list remotes: %w", err)
	}
	var origin string
	for _, name := range strings.Fields(out) {
		urls, err := m.git(ctx, path, "remote", "get-url", "--all", name)
		if err != nil {
			return "", fmt.Errorf("read remote %q: %w", name, err)
		}
		for _, remote := range strings.Split(strings.TrimSpace(urls), "\n") {
			if err := validateRemote(remote); err != nil {
				return "", fmt.Errorf("%w: remote %q contains an unsafe URL", ErrInvalidRemote, name)
			}
			if name == "origin" && origin == "" {
				origin = remote
			}
		}
	}
	if origin == "" {
		return "", errors.New("origin remote is required for adoption")
	}
	return origin, nil
}

// ValidateWorkspace revalidates the authority recorded for a persisted
// workspace before it is used.
func (m *Manager) ValidateWorkspace(ctx context.Context, ws *Workspace) error {
	_, err := m.authorizedPath(ctx, ws)
	return err
}

func (m *Manager) authorizedPath(ctx context.Context, ws *Workspace) (string, error) {
	if ws == nil {
		return "", ErrNotFound
	}
	if ws.Ownership == "" || ws.Ownership == OwnershipManaged {
		return m.managedPath(ws.Path)
	}
	if ws.Ownership != OwnershipExternal {
		return "", fmt.Errorf("%w: unknown ownership %q", ErrNotManaged, ws.Ownership)
	}
	canonical, err := canonicalDirectory(ws.Path)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrPathChanged, err)
	}
	if canonical != ws.Path {
		return "", fmt.Errorf("%w: workspace root", ErrPathChanged)
	}
	record, err := m.findWorktreeRecord(ctx, canonical)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrPathChanged, err)
	}
	gitDir, err := m.gitPath(ctx, canonical, "--absolute-git-dir")
	if err != nil || gitDir != ws.GitDir {
		return "", fmt.Errorf("%w: Git directory", ErrPathChanged)
	}
	commonDir, err := m.gitPath(ctx, canonical, "--git-common-dir")
	if err != nil || commonDir != ws.GitCommonDir {
		return "", fmt.Errorf("%w: Git common directory", ErrPathChanged)
	}
	if record.path != canonical {
		return "", fmt.Errorf("%w: worktree registration", ErrPathChanged)
	}
	remote, err := m.validateRemotes(ctx, canonical)
	if err != nil {
		return "", err
	}
	if remote != ws.Remote {
		return "", fmt.Errorf("%w: origin remote", ErrPathChanged)
	}
	return canonical, nil
}
