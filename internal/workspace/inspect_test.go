package workspace

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectPathReadsBranchHeadAndRemotesWithoutMutation(t *testing.T) {
	t.Parallel()
	path := t.TempDir()
	runGit(t, path, "init", "-b", "main")
	runGit(t, path, "config", "user.email", "test@example.com")
	runGit(t, path, "config", "user.name", "Test")
	writeFile(t, filepath.Join(path, "file.txt"), "base\n")
	runGit(t, path, "add", ".")
	runGit(t, path, "commit", "-m", "base")
	runGit(t, path, "checkout", "-b", "feature")
	runGit(t, path, "remote", "add", "origin", "https://github.com/fork/project.git")
	before := strings.TrimSpace(runGit(t, path, "status", "--porcelain"))

	got, err := InspectPath(context.Background(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	expectedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != expectedPath || got.Branch != "feature" || got.HeadSHA == "" {
		t.Fatalf("inspection = %+v", got)
	}
	if len(got.Remotes["origin"]) != 1 || got.Remotes["origin"][0] != "https://github.com/fork/project.git" {
		t.Fatalf("remotes = %+v", got.Remotes)
	}
	after := strings.TrimSpace(runGit(t, path, "status", "--porcelain"))
	if before != after {
		t.Fatalf("inspection changed worktree status: before=%q after=%q", before, after)
	}
}
