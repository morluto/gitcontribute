package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/domain"
)

func newTestRemote(t *testing.T) (remoteURL, baseSHA string) {
	t.Helper()
	dir := t.TempDir()
	remote := filepath.Join(dir, "remote.git")
	runGitApp(t, "", "init", "--bare", remote)

	src := filepath.Join(dir, "src")
	runGitApp(t, "", "clone", remote, src)
	runGitApp(t, src, "config", "user.email", "test@example.com")
	runGitApp(t, src, "config", "user.name", "Test")

	writeAppFile(t, filepath.Join(src, "main.go"), "package main\n")
	runGitApp(t, src, "add", ".")
	runGitApp(t, src, "commit", "-m", "initial")
	runGitApp(t, src, "push", "origin", "master")

	baseSHA = strings.TrimSpace(runGitApp(t, src, "rev-parse", "master"))
	runGitApp(t, remote, "symbolic-ref", "HEAD", "refs/heads/master")
	return remote, baseSHA
}

func pushCommit(t *testing.T, remote, file, content, message string) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	runGitApp(t, "", "clone", remote, src)
	runGitApp(t, src, "config", "user.email", "test@example.com")
	runGitApp(t, src, "config", "user.name", "Test")

	writeAppFile(t, filepath.Join(src, file), content)
	runGitApp(t, src, "add", ".")
	runGitApp(t, src, "commit", "-m", message)
	runGitApp(t, src, "push", "origin", "master")

	return strings.TrimSpace(runGitApp(t, src, "rev-parse", "master"))
}

func TestAcquireSuccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	remote, baseSHA := newTestRemote(t)

	svc := newLocalService(t)
	defer func() { _ = svc.Close() }()

	res, err := svc.Acquire(ctx, cli.RepoRef{Owner: "testowner", Repo: "testrepo"}, remote)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if res.CommitSHA != baseSHA {
		t.Fatalf("commit sha mismatch: got %q want %q", res.CommitSHA, baseSHA)
	}
	if res.DefaultBranch != "master" {
		t.Fatalf("default branch: got %q want master", res.DefaultBranch)
	}
	if !res.Indexed {
		t.Fatal("expected indexed")
	}
	if !res.Inserted {
		t.Fatal("expected inserted")
	}
	if res.Files != 1 {
		t.Fatalf("files: got %d want 1", res.Files)
	}

	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	snap, err := c.LatestCodeSnapshot(ctx, domain.RepoRef{Owner: "testowner", Repo: "testrepo"})
	if err != nil {
		t.Fatalf("latest snapshot: %v", err)
	}
	if snap == nil || snap.CommitSHA != baseSHA {
		t.Fatalf("snapshot not stored: %+v", snap)
	}
}

func TestAcquireRepeatFetch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	remote, baseSHA := newTestRemote(t)

	svc := newLocalService(t)
	defer func() { _ = svc.Close() }()

	res1, err := svc.Acquire(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, remote)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if res1.CommitSHA != baseSHA {
		t.Fatalf("first commit sha: got %q want %q", res1.CommitSHA, baseSHA)
	}

	newSHA := pushCommit(t, remote, "feature.go", "package feature\n", "add feature")

	res2, err := svc.Acquire(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, remote)
	if err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	if res2.CommitSHA != newSHA {
		t.Fatalf("second commit sha: got %q want %q", res2.CommitSHA, newSHA)
	}
	if !res2.Inserted {
		t.Fatal("expected new snapshot to be inserted")
	}

	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	snap, err := c.LatestCodeSnapshot(ctx, domain.RepoRef{Owner: "owner", Repo: "repo"})
	if err != nil {
		t.Fatalf("latest snapshot: %v", err)
	}
	if snap == nil || snap.CommitSHA != newSHA {
		t.Fatalf("latest snapshot not updated: %+v", snap)
	}
}

func TestAcquireFailureAtomicity(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, _ = newTestRemote(t)

	svc := newLocalService(t)
	defer func() { _ = svc.Close() }()

	_, err := svc.Acquire(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, "/nonexistent/remote/path")
	if err == nil {
		t.Fatal("expected error for invalid remote")
	}

	cacheRoot, err := svc.paths.AcquisitionCacheDir()
	if err != nil {
		t.Fatalf("cache dir: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(cacheRoot, "mirrors"))
	if err == nil && len(entries) > 0 {
		t.Fatalf("expected no mirror cache after failed clone, got %v", entries)
	}
}

func TestAcquireConcurrent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	remote, baseSHA := newTestRemote(t)

	svc := newLocalService(t)
	defer func() { _ = svc.Close() }()

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	commits := make(chan string, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := svc.Acquire(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, remote)
			if err != nil {
				errs <- err
				return
			}
			commits <- res.CommitSHA
		}()
	}
	wg.Wait()
	close(errs)
	close(commits)

	for err := range errs {
		t.Fatalf("concurrent acquire error: %v", err)
	}
	for commit := range commits {
		if commit != baseSHA {
			t.Fatalf("unexpected commit: got %q want %q", commit, baseSHA)
		}
	}
}

func TestAcquireInvalidRepo(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newLocalService(t)
	defer func() { _ = svc.Close() }()

	_, err := svc.Acquire(ctx, cli.RepoRef{Owner: "", Repo: "repo"}, "/some/remote")
	if err == nil {
		t.Fatal("expected error for invalid repo ref")
	}
}

func TestAcquireInvalidRemote(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newLocalService(t)
	defer func() { _ = svc.Close() }()

	_, err := svc.Acquire(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, "--unsafe")
	if err == nil {
		t.Fatal("expected error for invalid remote")
	}
}
