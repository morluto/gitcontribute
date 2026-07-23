package corpus

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/domain"
)

func TestCodeSnapshotsAreAtomicDeduplicatedAndSearchLatest(t *testing.T) {
	t.Parallel()
	c, _ := openTestCorpus(t)
	ctx := context.Background()
	ref := domain.RepoRef{Owner: "owner", Repo: "repo"}
	first := codeindex.Snapshot{RepoPath: "/repo", Commit: "first", CreatedAt: time.Unix(100, 0), Documents: []codeindex.Document{{Path: "old.go", Content: "legacy needle", Bytes: 13, LanguageHint: "go"}}, TotalBytes: 13}
	firstID, inserted, err := c.StoreCodeSnapshot(ctx, ref, first)
	if err != nil || !inserted {
		t.Fatalf("first snapshot = (%d, %v, %v)", firstID, inserted, err)
	}
	replayedID, inserted, err := c.StoreCodeSnapshot(ctx, ref, first)
	if err != nil || inserted || replayedID != firstID {
		t.Fatalf("replayed snapshot = (%d, %v, %v)", replayedID, inserted, err)
	}
	second := codeindex.Snapshot{RepoPath: "/repo", Commit: "second", CreatedAt: time.Unix(200, 0), Documents: []codeindex.Document{{Path: "new.go", Content: "current needle", Bytes: 14, LanguageHint: "go"}}, TotalBytes: 14}
	if _, _, err := c.StoreCodeSnapshot(ctx, ref, second); err != nil {
		t.Fatal(err)
	}
	matches, err := c.SearchCode(ctx, "needle", ref, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].Commit != "second" || matches[0].Path != "new.go" {
		t.Fatalf("matches = %+v", matches)
	}
	page, err := c.SearchCodeWithOptions(ctx, "needle", CodeSearchOptions{Ref: ref, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Snapshots) != 1 || page.Snapshots[0].CommitSHA != page.Matches[0].Commit {
		t.Fatalf("search snapshot does not describe matches: %+v", page)
	}
}

func TestCodeSearchWeightsPathAndReturnsBoundedSnippet(t *testing.T) {
	t.Parallel()
	c, _ := openTestCorpus(t)
	ctx := context.Background()
	ref := domain.RepoRef{Owner: "owner", Repo: "repo"}
	longContent := "music " + strings.Repeat("padding ", 500)
	snapshot := codeindex.Snapshot{
		RepoPath: "/repo", Commit: "abc", CreatedAt: time.Unix(100, 0),
		Documents: []codeindex.Document{
			{Path: "music.go", Content: "short", Bytes: 5, LanguageHint: "go"},
			{Path: "other.go", Content: longContent, Bytes: len(longContent), LanguageHint: "go"},
		},
		TotalBytes: len(longContent) + 5,
	}
	if _, _, err := c.StoreCodeSnapshot(ctx, ref, snapshot); err != nil {
		t.Fatal(err)
	}
	matches, err := c.SearchCode(ctx, "music", ref, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 || matches[0].Path != "music.go" {
		t.Fatalf("weighted code matches = %+v", matches)
	}
	if len(matches[1].Content) >= len(longContent) {
		t.Fatalf("code search returned full file (%d bytes)", len(matches[1].Content))
	}
}
