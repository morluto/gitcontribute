package workspace

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestManager_CreateAndInspect(t *testing.T) {
	t.Parallel()
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
	if runtime.GOOS != "windows" {
		for _, path := range []string{
			filepath.Join(mgr.root, "mirrors"),
			filepath.Join(mgr.root, "workspaces"),
			ws.Path,
		} {
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("stat managed path %q: %v", path, err)
			}
			if info.Mode().Perm()&0o027 != 0 {
				t.Errorf("managed path %q permissions = %04o, want no group write or world access", path, info.Mode().Perm())
			}
		}
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
