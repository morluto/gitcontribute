package workspace

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// LocalWorktree is a read-only inspection of a Git worktree. It deliberately
// contains no ownership or workflow identity; callers may use it for routing
// without adopting the path or changing refs.
type LocalWorktree struct {
	Path    string
	Branch  string
	HeadSHA string
	Remotes map[string][]string
}

// InspectPath reads the current branch, HEAD, and configured remotes from a
// local worktree. It does not fetch, write refs, invoke hooks, or modify the
// worktree. A nil runner uses the safe default Git runner.
func InspectPath(ctx context.Context, path string, runner Runner) (LocalWorktree, error) {
	if runner == nil {
		runner = DefaultRunner()
	}
	m := &Manager{runner: runner}
	canonical, err := canonicalDirectory(path)
	if err != nil {
		return LocalWorktree{}, err
	}
	inside, err := m.git(ctx, canonical, "rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(inside) != "true" {
		if err != nil {
			return LocalWorktree{}, fmt.Errorf("inspect worktree: %w", err)
		}
		return LocalWorktree{}, fmt.Errorf("inspect worktree: path is not a Git worktree")
	}
	topLevel, err := m.git(ctx, canonical, "rev-parse", "--path-format=absolute", "--show-toplevel")
	if err != nil {
		return LocalWorktree{}, fmt.Errorf("resolve worktree root: %w", err)
	}
	if cleanGitPath(topLevel) != canonical {
		return LocalWorktree{}, fmt.Errorf("path must be the worktree root")
	}
	head, err := m.git(ctx, canonical, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return LocalWorktree{}, fmt.Errorf("resolve worktree HEAD: %w", err)
	}
	branch, err := m.git(ctx, canonical, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		// Detached HEAD is still a useful identity. The caller can match it by
		// commit SHA, so only a non-detached branch failure is fatal here.
		branch = ""
	}
	remoteNames, err := m.git(ctx, canonical, "remote")
	if err != nil {
		return LocalWorktree{}, fmt.Errorf("list worktree remotes: %w", err)
	}
	remotes := make(map[string][]string)
	for _, name := range strings.Fields(remoteNames) {
		urls, err := m.git(ctx, canonical, "remote", "get-url", "--all", name)
		if err != nil {
			return LocalWorktree{}, fmt.Errorf("read remote %q: %w", name, err)
		}
		for _, raw := range strings.Split(strings.TrimSpace(urls), "\n") {
			if strings.TrimSpace(raw) == "" {
				continue
			}
			if err := validateRemote(raw); err != nil {
				return LocalWorktree{}, fmt.Errorf("remote %q is unsafe: %w", name, err)
			}
			remotes[name] = append(remotes[name], strings.TrimSpace(raw))
		}
	}
	return LocalWorktree{Path: filepath.Clean(canonical), Branch: strings.TrimSpace(branch), HeadSHA: strings.TrimSpace(head), Remotes: remotes}, nil
}
