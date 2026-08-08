package workspace

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagerCheckMergeUsesAlreadyFetchedRevisionsWithoutMutation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	remote, baseSHA, candidateSHA := setupRemote(t)
	mgr := newManager(t)
	if err := mgr.Clone(ctx, remote, "origin"); err != nil {
		t.Fatal(err)
	}
	path := mgr.mirrors["origin"].path
	before := strings.TrimSpace(runGit(t, path, "show-ref"))
	beforeObjects := strings.TrimSpace(runGit(t, path, "count-objects", "-v"))
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
	afterObjects := strings.TrimSpace(runGit(t, path, "count-objects", "-v"))
	if beforeObjects != afterObjects {
		t.Fatalf("merge check changed object store\nbefore: %s\nafter: %s", beforeObjects, afterObjects)
	}
}

// TestManagerCheckMergeNoFalsePositiveOnConflictMarkerInContent proves that
// a file containing conflict markers in its content does not trigger a
// false-positive conflict detection when the merge is actually clean.
func TestManagerCheckMergeNoFalsePositiveOnConflictMarkerInContent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	dir := t.TempDir()
	remote := filepath.Join(dir, "remote.git")
	runGit(t, "", "init", "--bare", remote)

	src := filepath.Join(dir, "src")
	runGit(t, "", "clone", remote, src)

	writeFile(t, filepath.Join(src, "file.txt"), "hello")
	runGit(t, src, "add", ".")
	runGit(t, src, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-m", "base")
	runGit(t, src, "push", "origin", "master")

	marker := strings.Repeat("<", 7)
	runGit(t, src, "checkout", "-b", "feature")
	writeFile(t, filepath.Join(src, "file.txt"), marker+" HEAD\nworld\n=======\nuniverse\n"+strings.Repeat(">", 7)+" branch\n")
	runGit(t, src, "add", ".")
	runGit(t, src, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-m", "feature adds conflict markers in content")
	runGit(t, src, "push", "origin", "feature")

	runGit(t, src, "checkout", "master")
	writeFile(t, filepath.Join(src, "other.txt"), "different change")
	runGit(t, src, "add", ".")
	runGit(t, src, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-m", "master adds other file")
	runGit(t, src, "push", "origin", "master")

	baseSHA := strings.TrimSpace(runGit(t, src, "rev-parse", "master"))
	candidateSHA := strings.TrimSpace(runGit(t, src, "rev-parse", "feature"))
	mgr := newManager(t)
	if err := mgr.Clone(ctx, remote, "origin"); err != nil {
		t.Fatal(err)
	}

	result, err := mgr.CheckMerge(ctx, mgr.mirrors["origin"].path, baseSHA, candidateSHA)
	if err != nil {
		t.Fatal(err)
	}
	if result.Conflicted {
		t.Fatalf("false positive: merge reported conflict but branches merge cleanly. Result: %+v", result)
	}
}

func TestManagerCheckMergeDetectsAddAddConflict(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

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
	writeFile(t, filepath.Join(src, "same.txt"), "feature")
	runGit(t, src, "add", ".")
	runGit(t, src, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-m", "feature adds same path")
	runGit(t, src, "push", "origin", "feature")

	runGit(t, src, "checkout", "master")
	writeFile(t, filepath.Join(src, "same.txt"), "master")
	runGit(t, src, "add", ".")
	runGit(t, src, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-m", "master adds same path")
	runGit(t, src, "push", "origin", "master")

	baseSHA := strings.TrimSpace(runGit(t, src, "rev-parse", "master"))
	candidateSHA := strings.TrimSpace(runGit(t, src, "rev-parse", "feature"))
	mgr := newManager(t)
	if err := mgr.Clone(ctx, remote, "origin"); err != nil {
		t.Fatal(err)
	}
	result, err := mgr.CheckMerge(ctx, mgr.mirrors["origin"].path, baseSHA, candidateSHA)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Conflicted {
		t.Fatalf("add/add conflict reported clean: %+v", result)
	}
}
