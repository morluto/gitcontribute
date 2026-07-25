package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/domain"
)

func TestIndexUnchangedCommitReusesCurrentSnapshot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repoPath := t.TempDir()
	runGitApp(t, repoPath, "init")
	runGitApp(t, repoPath, "config", "user.email", "test@example.com")
	runGitApp(t, repoPath, "config", "user.name", "Test")
	writeAppFile(t, filepath.Join(repoPath, "main.go"), "package main\n")
	runGitApp(t, repoPath, "add", ".")
	runGitApp(t, repoPath, "commit", "-m", "initial")

	ref := contracts.RepoRef{Owner: "owner", Repo: "repo"}
	svc := newLocalService(t)
	defer func() { _ = svc.Close() }()
	first, err := svc.Index(ctx, ref, repoPath)
	if err != nil {
		t.Fatalf("first index: %v", err)
	}

	replacement := codeindex.Snapshot{
		RepoPath: repoPath,
		Commit:   first.Commit,
		Documents: []codeindex.Document{{
			Path: "sentinel.txt", Content: "preserved sentinel", Bytes: len("preserved sentinel"),
		}},
		TotalBytes: len("preserved sentinel"),
		CreatedAt:  time.Now().UTC(),
		Manifest: codeindex.Manifest{
			FormatVersion:  codeindex.FormatVersion,
			CoverageKnown:  true,
			TrackedEntries: 1,
			IndexedFiles:   1,
		},
	}
	if _, _, err := svc.corpus.StoreCodeSnapshot(
		ctx, domain.RepoRef{Owner: ref.Owner, Repo: ref.Repo}, replacement,
	); err != nil {
		t.Fatalf("replace snapshot: %v", err)
	}

	second, err := svc.Index(ctx, ref, repoPath)
	if err != nil {
		t.Fatalf("second index: %v", err)
	}
	if second.Inserted || second.Message != "snapshot already indexed" {
		t.Fatalf("second index = %+v", second)
	}
	matches, err := svc.corpus.SearchCode(
		ctx, "sentinel", domain.RepoRef{Owner: ref.Owner, Repo: ref.Repo}, 10,
	)
	if err != nil {
		t.Fatalf("search preserved snapshot: %v", err)
	}
	if len(matches) != 1 || matches[0].Path != "sentinel.txt" {
		t.Fatalf("snapshot was reindexed: %+v", matches)
	}
}
