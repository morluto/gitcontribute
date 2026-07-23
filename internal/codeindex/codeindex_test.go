package codeindex

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/morluto/gitcontribute/internal/buflimit"
)

func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	testGit(t, dir, "init")
	testGit(t, dir, "config", "user.email", "test@example.com")
	testGit(t, dir, "config", "user.name", "Test")
	return dir
}

func testGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}

func writeFile(t *testing.T, repo, p, content string) {
	t.Helper()
	full := filepath.Join(repo, filepath.FromSlash(p))
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func commitAll(t *testing.T, repo, message string) {
	t.Helper()
	testGit(t, repo, "add", "-A")
	testGit(t, repo, "commit", "-m", message)
}

func TestIndexBasic(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "main.go", "package main\n")
	writeFile(t, repo, "README.md", "# test\n")
	writeFile(t, repo, "sub/nested.py", "print(1)\n")
	commitAll(t, repo, "initial")

	start := time.Now().UTC()
	snap, err := Index(context.Background(), repo, Options{})
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}
	if snap.CreatedAt.Before(start) || snap.CreatedAt.After(time.Now().UTC()) {
		t.Fatalf("unexpected CreatedAt: %v", snap.CreatedAt)
	}

	wantCommit := strings.TrimSpace(testGit(t, repo, "rev-parse", "HEAD"))
	if snap.Commit != wantCommit {
		t.Fatalf("commit mismatch: got %q, want %q", snap.Commit, wantCommit)
	}

	evalRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatalf("eval symlinks: %v", err)
	}
	if snap.RepoPath != evalRepo {
		t.Fatalf("repo path mismatch: got %q, want %q", snap.RepoPath, evalRepo)
	}

	if len(snap.Documents) != 3 {
		t.Fatalf("expected 3 documents, got %d: %+v", len(snap.Documents), snap.Documents)
	}
	wantPaths := []string{"README.md", "main.go", "sub/nested.py"}
	for i, d := range snap.Documents {
		if d.Path != wantPaths[i] {
			t.Fatalf("document path[%d]: got %q, want %q", i, d.Path, wantPaths[i])
		}
	}

	hints := map[string]string{
		"README.md":     "markdown",
		"main.go":       "go",
		"sub/nested.py": "python",
	}
	total := 0
	for _, d := range snap.Documents {
		if d.LanguageHint != hints[d.Path] {
			t.Fatalf("language hint for %q: got %q, want %q", d.Path, d.LanguageHint, hints[d.Path])
		}
		if d.Bytes != len(d.Content) {
			t.Fatalf("Bytes field for %q: got %d, want %d", d.Path, d.Bytes, len(d.Content))
		}
		total += d.Bytes
	}
	if snap.TotalBytes != total {
		t.Fatalf("TotalBytes: got %d, want %d", snap.TotalBytes, total)
	}
}

func TestIndexStableResults(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "b.txt", "b\n")
	writeFile(t, repo, "a.txt", "a\n")
	commitAll(t, repo, "initial")

	snap1, err := Index(context.Background(), repo, Options{})
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}
	snap2, err := Index(context.Background(), repo, Options{})
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}

	want := Snapshot{
		RepoPath:   snap1.RepoPath,
		Commit:     snap1.Commit,
		Documents:  snap1.Documents,
		TotalBytes: snap1.TotalBytes,
	}
	got := Snapshot{
		RepoPath:   snap2.RepoPath,
		Commit:     snap2.Commit,
		Documents:  snap2.Documents,
		TotalBytes: snap2.TotalBytes,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshots differ:\n%s", cmp.Diff(want, got))
	}
	if snap1.CreatedAt.Equal(snap2.CreatedAt) {
		t.Fatalf("expected different CreatedAt values")
	}
}

func TestIndexDirtyWorktree(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "a.txt", "a\n")
	commitAll(t, repo, "initial")

	writeFile(t, repo, "a.txt", "changed\n")
	snap, err := Index(context.Background(), repo, Options{})
	if !errors.Is(err, ErrDirtyWorktree) {
		t.Fatalf("expected ErrDirtyWorktree, got snap=%+v err=%v", snap, err)
	}
}

func TestIndexUntrackedDirty(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "a.txt", "a\n")
	commitAll(t, repo, "initial")

	writeFile(t, repo, "untracked.txt", "x\n")
	snap, err := Index(context.Background(), repo, Options{})
	if !errors.Is(err, ErrDirtyWorktree) {
		t.Fatalf("expected ErrDirtyWorktree, got snap=%+v err=%v", snap, err)
	}
}

func TestIndexNoCommit(t *testing.T) {
	repo := newRepo(t)
	_, err := Index(context.Background(), repo, Options{})
	if !errors.Is(err, ErrNoCommit) {
		t.Fatalf("expected ErrNoCommit, got %v", err)
	}
}

func TestIndexNotARepository(t *testing.T) {
	dir := t.TempDir()
	_, err := Index(context.Background(), dir, Options{})
	if !errors.Is(err, ErrNotARepository) {
		t.Fatalf("expected ErrNotARepository, got %v", err)
	}
}

func TestIndexNonRootPath(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "a.txt", "a\n")
	writeFile(t, repo, "sub/b.txt", "b\n")
	commitAll(t, repo, "initial")

	_, err := Index(context.Background(), filepath.Join(repo, "sub"), Options{})
	if err == nil {
		t.Fatal("expected error for non-root repo path")
	}
}

func TestIndexSymlinkContainment(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "real.txt", "safe content\n")
	if err := os.Symlink("/etc/passwd", filepath.Join(repo, "escape")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if err := os.Symlink("real.txt", filepath.Join(repo, "inlink")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	commitAll(t, repo, "initial")

	snap, err := Index(context.Background(), repo, Options{})
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}
	if len(snap.Documents) != 1 || snap.Documents[0].Path != "real.txt" {
		t.Fatalf("expected only real.txt, got %+v", snap.Documents)
	}
	if strings.Contains(snap.Documents[0].Content, "root") {
		t.Fatal("symlink target content leaked into snapshot")
	}
}

func TestIndexTraversalPath(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "sub/nested.txt", "ok\n")
	commitAll(t, repo, "initial")

	snap, err := Index(context.Background(), repo, Options{})
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}
	if len(snap.Documents) != 1 || snap.Documents[0].Path != "sub/nested.txt" {
		t.Fatalf("expected sub/nested.txt, got %+v", snap.Documents)
	}
}

func TestIndexBinarySkip(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "text.txt", "hello\n")
	writeFile(t, repo, "null.bin", "hello\x00world")
	writeFile(t, repo, "badutf.bin", "\xff\xfe")
	commitAll(t, repo, "initial")

	snap, err := Index(context.Background(), repo, Options{})
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}
	if len(snap.Documents) != 1 || snap.Documents[0].Path != "text.txt" {
		t.Fatalf("expected only text.txt, got %+v", snap.Documents)
	}
}

func TestIndexMaxFiles(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "a.txt", "a\n")
	writeFile(t, repo, "b.txt", "b\n")
	writeFile(t, repo, "c.txt", "c\n")
	commitAll(t, repo, "initial")

	snap, err := Index(context.Background(), repo, Options{MaxFiles: 1})
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}
	if len(snap.Documents) != 1 || snap.Documents[0].Path != "a.txt" {
		t.Fatalf("expected one document (a.txt), got %+v", snap.Documents)
	}
	if !snap.Manifest.Truncated || snap.Manifest.IndexedFiles != 1 || snap.Manifest.SkippedFileLimit != 2 || snap.Manifest.TrackedEntries != 3 {
		t.Fatalf("index manifest = %+v", snap.Manifest)
	}

	snap, err = Index(context.Background(), repo, Options{MaxFiles: 2})
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}
	if len(snap.Documents) != 2 {
		t.Fatalf("expected 2 documents, got %d", len(snap.Documents))
	}
}

func TestIndexMaxBytesPerFile(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "small.txt", "ab")
	writeFile(t, repo, "big.txt", strings.Repeat("x", 100))
	commitAll(t, repo, "initial")

	snap, err := Index(context.Background(), repo, Options{MaxBytesPerFile: 10})
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}
	if len(snap.Documents) != 1 || snap.Documents[0].Path != "small.txt" {
		t.Fatalf("expected only small.txt, got %+v", snap.Documents)
	}
}

func TestIndexMaxTotalBytes(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "a.txt", "aa")  // 2 bytes
	writeFile(t, repo, "b.txt", "bbb") // 3 bytes
	writeFile(t, repo, "c.txt", "c")   // 1 byte
	commitAll(t, repo, "initial")

	// Max total is 4 bytes. a.txt fits (2), b.txt would exceed (5), so it is
	// skipped; c.txt still fits (3) and is included.
	snap, err := Index(context.Background(), repo, Options{MaxTotalBytes: 4})
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}
	if snap.TotalBytes > 4 {
		t.Fatalf("TotalBytes %d exceeds limit 4", snap.TotalBytes)
	}
	want := []string{"a.txt", "c.txt"}
	if len(snap.Documents) != len(want) {
		t.Fatalf("expected %d documents, got %d: %+v", len(want), len(snap.Documents), snap.Documents)
	}
	for i, d := range snap.Documents {
		if d.Path != want[i] {
			t.Fatalf("document[%d]: got %q, want %q", i, d.Path, want[i])
		}
	}
	if snap.TotalBytes != 3 {
		t.Fatalf("TotalBytes: got %d, want 3", snap.TotalBytes)
	}
}

func TestIndexExclusions(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "keep.go", "package main\n")
	writeFile(t, repo, "skip.md", "# doc\n")
	writeFile(t, repo, "dir/ignored.txt", "x\n")
	writeFile(t, repo, "dir/kept.go", "package x\n")
	commitAll(t, repo, "initial")

	snap, err := Index(context.Background(), repo, Options{Exclusions: []string{"*.md", "dir/*.txt"}})
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}
	want := []string{"dir/kept.go", "keep.go"}
	if len(snap.Documents) != len(want) {
		t.Fatalf("expected %d documents, got %d: %+v", len(want), len(snap.Documents), snap.Documents)
	}
	for i, d := range snap.Documents {
		if d.Path != want[i] {
			t.Fatalf("document[%d]: got %q, want %q", i, d.Path, want[i])
		}
	}
}

func TestIndexRejectsMalformedExclusionPattern(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "keep.go", "package main\n")
	commitAll(t, repo, "initial")

	if _, err := Index(context.Background(), repo, Options{Exclusions: []string{"["}}); err == nil {
		t.Fatal("Index accepted malformed exclusion pattern")
	}
}

func TestIndexCancellation(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "a.txt", "a\n")
	commitAll(t, repo, "initial")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Index(ctx, repo, Options{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestIndexRejectsNegativeLimits(t *testing.T) {
	_, err := Index(context.Background(), t.TempDir(), Options{MaxFiles: -1})
	if err == nil || !strings.Contains(err.Error(), "cannot be negative") {
		t.Fatalf("expected negative-limit error, got %v", err)
	}
}

func TestIndexRejectsLimitsAboveHardCeilings(t *testing.T) {
	tests := []Options{
		{MaxFiles: hardMaxFiles + 1},
		{MaxBytesPerFile: hardMaxBytesPerFile + 1},
		{MaxTotalBytes: hardMaxTotalBytes + 1},
	}
	for _, opts := range tests {
		if _, err := Index(context.Background(), t.TempDir(), opts); err == nil || !strings.Contains(err.Error(), "hard limit") {
			t.Fatalf("Index(%+v) error = %v, want hard-limit rejection", opts, err)
		}
	}
}

func TestRunGitDisablesRepositoryFSMonitor(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "a.txt", "a\n")
	commitAll(t, repo, "initial")
	marker := filepath.Join(t.TempDir(), "executed")
	hook := filepath.Join(repo, "fsmonitor.sh")
	writeFile(t, repo, "fsmonitor.sh", "#!/bin/sh\ntouch \""+marker+"\"\n")
	if err := os.Chmod(hook, 0755); err != nil {
		t.Fatal(err)
	}
	testGit(t, repo, "config", "core.fsmonitor", hook)

	// The untracked hook file makes the tree dirty, but inspecting it must not
	// execute the repository-controlled fsmonitor command.
	_, err := Index(context.Background(), repo, Options{})
	if !errors.Is(err, ErrDirtyWorktree) {
		t.Fatalf("Index error = %v, want ErrDirtyWorktree", err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("repository-controlled fsmonitor executed; stat error = %v", err)
	}
}

func TestRunGitOutputIsBounded(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "a.txt", "a\n")
	commitAll(t, repo, "initial")
	_, err := runGitLimited(context.Background(), repo, 1, "rev-parse", "HEAD")
	if !errors.Is(err, buflimit.ErrOutputLimit) {
		t.Fatalf("runGitLimited error = %v, want ErrOutputLimit", err)
	}
}

func TestGitBlobReadsCapturedCommitNotWorktree(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "a.txt", "committed\n")
	commitAll(t, repo, "initial")
	commit := strings.TrimSpace(testGit(t, repo, "rev-parse", "HEAD"))
	entries, err := gitTree(context.Background(), repo, commit, 10)
	if err != nil || len(entries) != 1 {
		t.Fatalf("gitTree = (%+v, %v)", entries, err)
	}

	writeFile(t, repo, "a.txt", "worktree replacement\n")
	content, err := gitBlob(context.Background(), repo, entries[0].object, entries[0].size)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "committed\n" {
		t.Fatalf("blob content = %q, want captured commit bytes", content)
	}
}
