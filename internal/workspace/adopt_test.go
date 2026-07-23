package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagerAdoptExternalWorktreeWithoutMutation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	remote, baseSHA, candidateSHA := setupRemote(t)
	source := filepath.Join(filepath.Dir(remote), "src")
	external := filepath.Join(t.TempDir(), "existing worktree")
	runGit(t, source, "worktree", "add", "--detach", external, candidateSHA)
	runGit(t, source, "remote", "set-url", "origin", "https://github.com/owner/repo.git")
	writeFile(t, filepath.Join(external, "feature.txt"), "modified")
	writeFile(t, filepath.Join(external, "untracked.txt"), "new")

	beforeHead := strings.TrimSpace(runGit(t, external, "rev-parse", "HEAD"))
	beforeRefs := runGit(t, external, "show-ref")
	beforeStatus := runGit(t, external, "status", "--porcelain=v1", "-z")

	mgr := newManager(t)
	ws, err := mgr.Adopt(ctx, AdoptOptions{Path: external, BaseRef: "master", Name: "external"})
	if err != nil {
		t.Fatal(err)
	}
	if ws.Ownership != OwnershipExternal || ws.BaseSHA != baseSHA || ws.CandidateSHA != candidateSHA || !ws.Dirty || !ws.HasUntracked {
		t.Fatalf("unexpected adopted workspace: %+v", ws)
	}
	if err := mgr.ValidateWorkspace(ctx, ws); err != nil {
		t.Fatalf("ValidateWorkspace: %v", err)
	}
	if _, err := mgr.DiffWorkspace(ctx, ws); err != nil {
		t.Fatalf("DiffWorkspace: %v", err)
	}
	if err := mgr.Remove(ctx, ws.Path, true); !errors.Is(err, ErrNotManaged) {
		t.Fatalf("Remove external workspace = %v, want ErrNotManaged", err)
	}
	if got := strings.TrimSpace(runGit(t, external, "rev-parse", "HEAD")); got != beforeHead {
		t.Fatalf("HEAD changed: got %s want %s", got, beforeHead)
	}
	if got := runGit(t, external, "show-ref"); got != beforeRefs {
		t.Fatal("adoption changed refs")
	}
	if got := runGit(t, external, "status", "--porcelain=v1", "-z"); got != beforeStatus {
		t.Fatal("adoption changed worktree state")
	}
}

func TestManagerExternalWorkspaceRejectsChangedIdentity(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	remote, _, candidateSHA := setupRemote(t)
	source := filepath.Join(filepath.Dir(remote), "src")
	external := filepath.Join(t.TempDir(), "worktree")
	runGit(t, source, "worktree", "add", "--detach", external, candidateSHA)
	runGit(t, source, "remote", "set-url", "origin", "https://github.com/owner/repo.git")
	mgr := newManager(t)
	ws, err := mgr.Adopt(ctx, AdoptOptions{Path: external, BaseRef: "master", Name: "external"})
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, source, "remote", "set-url", "origin", "https://github.com/other/repo.git")
	if _, err := mgr.StatusWorkspace(ctx, ws); !errors.Is(err, ErrPathChanged) {
		t.Fatalf("StatusWorkspace changed origin error = %v, want ErrPathChanged", err)
	}
	runGit(t, source, "remote", "set-url", "origin", ws.Remote)
	moved := external + "-moved"
	if err := os.Rename(external, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(moved, external); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.StatusWorkspace(ctx, ws); !errors.Is(err, ErrPathChanged) {
		t.Fatalf("StatusWorkspace replaced path error = %v, want ErrPathChanged", err)
	}
}
