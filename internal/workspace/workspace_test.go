package workspace

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"--no-pager"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_NO_PAGER=1",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_SSH_COMMAND=/bin/false",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %q: %v\n%s", args, dir, err, out)
	}
	return string(out)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func setupRemote(t *testing.T) (remoteURL, baseSHA, candidateSHA string) {
	t.Helper()
	dir := t.TempDir()
	remote := filepath.Join(dir, "remote.git")
	runGit(t, "", "init", "--bare", remote)

	src := filepath.Join(dir, "src")
	runGit(t, "", "clone", remote, src)

	writeFile(t, filepath.Join(src, "base.txt"), "base")
	runGit(t, src, "add", ".")
	runGit(t, src, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-m", "base")
	runGit(t, src, "push", "origin", "master")

	runGit(t, src, "checkout", "-b", "feature")
	writeFile(t, filepath.Join(src, "feature.txt"), "feature")
	runGit(t, src, "add", ".")
	runGit(t, src, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-m", "feature")
	runGit(t, src, "push", "origin", "feature")

	baseSHA = strings.TrimSpace(runGit(t, src, "rev-parse", "master"))
	candidateSHA = strings.TrimSpace(runGit(t, src, "rev-parse", "feature"))
	return remote, baseSHA, candidateSHA
}

func newManager(t *testing.T) *Manager {
	t.Helper()
	m, err := NewManager(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestManager_CloneAndResolve(t *testing.T) {
	ctx := context.Background()
	remote, baseSHA, candidateSHA := setupRemote(t)
	mgr := newManager(t)

	if err := mgr.Clone(ctx, remote, "origin"); err != nil {
		t.Fatal(err)
	}

	t.Run("resolve by branch", func(t *testing.T) {
		got, err := mgr.Resolve(ctx, "origin", "master")
		if err != nil {
			t.Fatal(err)
		}
		if got != baseSHA {
			t.Fatalf("Resolve(master) = %q, want %q", got, baseSHA)
		}
	})

	t.Run("resolve by sha", func(t *testing.T) {
		got, err := mgr.Resolve(ctx, "origin", candidateSHA)
		if err != nil {
			t.Fatal(err)
		}
		if got != candidateSHA {
			t.Fatalf("Resolve(sha) = %q, want %q", got, candidateSHA)
		}
	})
}

func TestManagerCheckMergeUsesAlreadyFetchedRevisionsWithoutMutation(t *testing.T) {
	ctx := context.Background()
	remote, baseSHA, candidateSHA := setupRemote(t)
	mgr := newManager(t)
	if err := mgr.Clone(ctx, remote, "origin"); err != nil {
		t.Fatal(err)
	}
	path := mgr.mirrors["origin"].path
	before := strings.TrimSpace(runGit(t, path, "show-ref"))
	result, err := mgr.CheckMerge(ctx, path, baseSHA, candidateSHA)
	if err != nil {
		t.Fatal(err)
	}
	if result.Conflicted || result.MergeBase != baseSHA {
		t.Fatalf("unexpected merge result: %+v", result)
	}
	after := strings.TrimSpace(runGit(t, path, "show-ref"))
	if before != after {
		t.Fatalf("merge check changed refs\nbefore: %s\nafter: %s", before, after)
	}
}

func TestManager_CreateAndInspect(t *testing.T) {
	ctx := context.Background()
	remote, baseSHA, candidateSHA := setupRemote(t)
	mgr := newManager(t)

	if err := mgr.Clone(ctx, remote, "origin"); err != nil {
		t.Fatal(err)
	}

	ws, err := mgr.Create(ctx, "origin", "master", "feature", "ws1")
	if err != nil {
		t.Fatal(err)
	}

	if ws.Remote != remote {
		t.Errorf("Remote = %q, want %q", ws.Remote, remote)
	}
	if ws.BaseSHA != baseSHA {
		t.Errorf("BaseSHA = %q, want %q", ws.BaseSHA, baseSHA)
	}
	if ws.CandidateSHA != candidateSHA {
		t.Errorf("CandidateSHA = %q, want %q", ws.CandidateSHA, candidateSHA)
	}
	if ws.MergeBase != baseSHA {
		t.Errorf("MergeBase = %q, want %q", ws.MergeBase, baseSHA)
	}

	if _, err := os.Stat(ws.Path); err != nil {
		t.Errorf("workspace path does not exist: %v", err)
	}

	mergeBase, err := mgr.MergeBase(ctx, "ws1")
	if err != nil {
		t.Fatal(err)
	}
	if mergeBase != baseSHA {
		t.Fatalf("MergeBase() = %q, want %q", mergeBase, baseSHA)
	}

	diff, err := mgr.Diff(ctx, "ws1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "feature.txt") {
		t.Fatalf("diff does not contain feature.txt:\n%s", diff)
	}

	got, ok := mgr.Get("ws1")
	if !ok || got.Name != "ws1" {
		t.Fatalf("Get(ws1) = (%v, %v)", got, ok)
	}
	if len(mgr.List()) != 1 {
		t.Fatalf("List() = %d items, want 1", len(mgr.List()))
	}
}

func TestManager_Fetch(t *testing.T) {
	ctx := context.Background()
	remote, _, _ := setupRemote(t)
	mgr := newManager(t)

	if err := mgr.Clone(ctx, remote, "origin"); err != nil {
		t.Fatal(err)
	}

	pushdir := filepath.Join(t.TempDir(), "push")
	runGit(t, "", "clone", remote, pushdir)
	runGit(t, pushdir, "checkout", "-b", "newbranch")
	writeFile(t, filepath.Join(pushdir, "new.txt"), "new")
	runGit(t, pushdir, "add", ".")
	runGit(t, pushdir, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-m", "new")
	runGit(t, pushdir, "push", "origin", "newbranch")

	if err := mgr.Fetch(ctx, "origin"); err != nil {
		t.Fatal(err)
	}

	if _, err := mgr.Resolve(ctx, "origin", "newbranch"); err != nil {
		t.Fatalf("Resolve(newbranch) after fetch: %v", err)
	}
}

func TestManager_ReopensAndRefreshesExistingMirror(t *testing.T) {
	ctx := context.Background()
	remote, _, _ := setupRemote(t)
	root := t.TempDir()
	first, err := NewManager(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Clone(ctx, remote, "origin"); err != nil {
		t.Fatal(err)
	}

	pushdir := filepath.Join(t.TempDir(), "push")
	runGit(t, "", "clone", remote, pushdir)
	runGit(t, pushdir, "checkout", "-b", "after-restart")
	writeFile(t, filepath.Join(pushdir, "new.txt"), "new")
	runGit(t, pushdir, "add", ".")
	runGit(t, pushdir, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-m", "new")
	runGit(t, pushdir, "push", "origin", "after-restart")

	reopened, err := NewManager(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.Clone(ctx, remote, "origin"); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.Resolve(ctx, "origin", "after-restart"); err != nil {
		t.Fatalf("existing mirror was not refreshed: %v", err)
	}
}

func TestManager_RejectsExistingMirrorForDifferentRemote(t *testing.T) {
	ctx := context.Background()
	firstRemote, _, _ := setupRemote(t)
	secondRemote, _, _ := setupRemote(t)
	root := t.TempDir()
	first, err := NewManager(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Clone(ctx, firstRemote, "origin"); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewManager(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.Clone(ctx, secondRemote, "origin"); !errors.Is(err, ErrRemoteMismatch) {
		t.Fatalf("Clone with different existing remote = %v, want ErrRemoteMismatch", err)
	}
}

func TestManager_DirtyState(t *testing.T) {
	ctx := context.Background()
	remote, _, _ := setupRemote(t)
	mgr := newManager(t)

	if err := mgr.Clone(ctx, remote, "origin"); err != nil {
		t.Fatal(err)
	}
	ws, err := mgr.Create(ctx, "origin", "master", "feature", "ws1")
	if err != nil {
		t.Fatal(err)
	}

	st, err := mgr.Status(ctx, "ws1")
	if err != nil {
		t.Fatal(err)
	}
	if st.Dirty {
		t.Fatal("new workspace should be clean")
	}

	writeFile(t, filepath.Join(ws.Path, "dirty.txt"), "dirty")
	st, err = mgr.Status(ctx, "ws1")
	if err != nil {
		t.Fatal(err)
	}
	if !st.Dirty {
		t.Fatal("workspace should be dirty after adding a file")
	}

	if err := mgr.Remove(ctx, ws.Path, false); !errors.Is(err, ErrDirty) {
		t.Fatalf("Remove(clean=false) on dirty workspace = %v, want ErrDirty", err)
	}

	if err := mgr.Remove(ctx, ws.Path, true); err != nil {
		t.Fatalf("Remove(force=true) on dirty workspace = %v", err)
	}
}

func TestManager_DuplicateCreate(t *testing.T) {
	ctx := context.Background()
	remote, _, _ := setupRemote(t)
	mgr := newManager(t)

	if err := mgr.Clone(ctx, remote, "origin"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Create(ctx, "origin", "master", "feature", "ws1"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Create(ctx, "origin", "master", "feature", "ws1"); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate Create = %v, want ErrExists", err)
	}
}

func TestManager_PathContainment(t *testing.T) {
	ctx := context.Background()
	remote, _, _ := setupRemote(t)
	mgr := newManager(t)

	if err := mgr.Clone(ctx, remote, "origin"); err != nil {
		t.Fatal(err)
	}
	ws, err := mgr.Create(ctx, "origin", "master", "feature", "ws1")
	if err != nil {
		t.Fatal(err)
	}

	if err := mgr.Remove(ctx, "/tmp/outside-path", false); !errors.Is(err, ErrNotManaged) {
		t.Fatalf("Remove outside root = %v, want ErrNotManaged", err)
	}

	unmanaged := filepath.Join(mgr.root, "workspaces", "unmanaged")
	if err := mgr.Remove(ctx, unmanaged, false); !errors.Is(err, ErrNotManaged) {
		t.Fatalf("Remove unmanaged path = %v, want ErrNotManaged", err)
	}

	if err := mgr.Remove(ctx, ws.Path, false); err != nil {
		t.Fatalf("Remove recorded path = %v", err)
	}
}

func TestManager_PathMethodsRejectSymlinkEscape(t *testing.T) {
	mgr := newManager(t)
	outside := t.TempDir()
	link := filepath.Join(mgr.root, "workspaces", "escape")
	if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.StatusByPath(context.Background(), link); !errors.Is(err, ErrNotManaged) {
		t.Fatalf("StatusByPath symlink escape = %v, want ErrNotManaged", err)
	}
	if _, err := mgr.DiffByPath(context.Background(), link, "HEAD"); !errors.Is(err, ErrNotManaged) {
		t.Fatalf("DiffByPath symlink escape = %v, want ErrNotManaged", err)
	}
	if _, err := mgr.ChangedFilesByPath(context.Background(), link, "HEAD"); !errors.Is(err, ErrNotManaged) {
		t.Fatalf("ChangedFilesByPath symlink escape = %v, want ErrNotManaged", err)
	}
	if _, err := mgr.HasUntrackedByPath(context.Background(), link); !errors.Is(err, ErrNotManaged) {
		t.Fatalf("HasUntrackedByPath symlink escape = %v, want ErrNotManaged", err)
	}
}

func TestManager_HasUntrackedByPath(t *testing.T) {
	ctx := context.Background()
	remote, _, _ := setupRemote(t)
	mgr := newManager(t)
	if err := mgr.Clone(ctx, remote, "origin"); err != nil {
		t.Fatal(err)
	}
	ws, err := mgr.Create(ctx, "origin", "master", "feature", "ws1")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := mgr.HasUntrackedByPath(ctx, ws.Path); err != nil || got {
		t.Fatalf("clean HasUntrackedByPath = %v, %v", got, err)
	}
	writeFile(t, filepath.Join(ws.Path, "new-untracked.txt"), "new")
	if got, err := mgr.HasUntrackedByPath(ctx, ws.Path); err != nil || !got {
		t.Fatalf("dirty HasUntrackedByPath = %v, %v", got, err)
	}
}

func TestManager_Spaces(t *testing.T) {
	ctx := context.Background()
	remote, _, _ := setupRemote(t)
	mgr := newManager(t)

	if err := mgr.Clone(ctx, remote, "origin"); err != nil {
		t.Fatal(err)
	}
	ws, err := mgr.Create(ctx, "origin", "master", "feature", "my workspace")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ws.Path, "my workspace") {
		t.Fatalf("workspace path does not contain spaces: %q", ws.Path)
	}

	st, err := mgr.Status(ctx, "my workspace")
	if err != nil {
		t.Fatal(err)
	}
	if st.Dirty {
		t.Fatal("spaced workspace should be clean")
	}

	if err := mgr.Remove(ctx, ws.Path, false); err != nil {
		t.Fatalf("Remove spaced workspace = %v", err)
	}
}

func TestManager_Cancellation(t *testing.T) {
	remote, _, _ := setupRemote(t)
	mgr := newManager(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := mgr.Clone(ctx, remote, "origin"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Clone with canceled context = %v, want context.Canceled", err)
	}
}

func TestManager_InvalidName(t *testing.T) {
	ctx := context.Background()
	remote, _, _ := setupRemote(t)
	mgr := newManager(t)

	if err := mgr.Clone(ctx, remote, "origin"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Create(ctx, "origin", "master", "feature", "../escape"); err == nil {
		t.Fatal("expected error for invalid workspace name")
	}
}

func TestManager_RejectsExecutableRemoteTransport(t *testing.T) {
	mgr := newManager(t)
	err := mgr.Clone(context.Background(), "ext::sh -c touch /tmp/pwned", "origin")
	if !errors.Is(err, ErrInvalidRemote) {
		t.Fatalf("Clone executable transport error = %v, want ErrInvalidRemote", err)
	}
}

type countingRunner struct {
	calls int
}

func (r *countingRunner) Run(context.Context, string, ...string) (string, error) {
	r.calls++
	return "", errors.New("unexpected Git invocation")
}

func TestManager_RejectsCredentialRemoteBeforeGitOrMirrorWrite(t *testing.T) {
	fixtureUser := strings.Join([]string{"fixture", "user"}, "-")
	fixturePassword := strings.Join([]string{"fixture", "password"}, "-")
	remote := "https://" + fixtureUser + ":" + fixturePassword + "@github.com/owner/repo.git"
	root := t.TempDir()
	runner := &countingRunner{}
	mgr, err := NewManager(root, runner)
	if err != nil {
		t.Fatal(err)
	}

	err = mgr.Clone(context.Background(), remote, "origin")
	if !errors.Is(err, ErrInvalidRemote) {
		t.Fatalf("Clone credential remote error = %v, want ErrInvalidRemote", err)
	}
	if strings.Contains(err.Error(), fixturePassword) {
		t.Fatalf("Clone error exposed credential: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("Git runner calls = %d, want 0", runner.calls)
	}
	if _, err := os.Stat(filepath.Join(root, "mirrors")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("mirror directory was written before remote validation: %v", err)
	}
}

func TestManager_StatusDisablesRepositoryFSMonitor(t *testing.T) {
	ctx := context.Background()
	remote, _, _ := setupRemote(t)
	mgr := newManager(t)
	if err := mgr.Clone(ctx, remote, "origin"); err != nil {
		t.Fatal(err)
	}
	ws, err := mgr.Create(ctx, "origin", "master", "feature", "ws1")
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "executed")
	hook := filepath.Join(t.TempDir(), "fsmonitor.sh")
	writeFile(t, hook, "#!/bin/sh\ntouch \""+marker+"\"\n")
	if err := os.Chmod(hook, 0755); err != nil {
		t.Fatal(err)
	}
	runGit(t, ws.Path, "config", "core.fsmonitor", hook)
	if _, err := mgr.Status(ctx, "ws1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("repository-controlled fsmonitor executed; stat error = %v", err)
	}
}

func TestManager_DiffIncludesWorkingChanges(t *testing.T) {
	ctx := context.Background()
	remote, _, _ := setupRemote(t)
	mgr := newManager(t)
	if err := mgr.Clone(ctx, remote, "origin"); err != nil {
		t.Fatal(err)
	}
	ws, err := mgr.Create(ctx, "origin", "master", "feature", "ws1")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(ws.Path, "feature.txt"), "working")
	diff, err := mgr.Diff(ctx, "ws1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "+working") {
		t.Fatalf("working-tree diff missing untracked file:\n%s", diff)
	}
}
