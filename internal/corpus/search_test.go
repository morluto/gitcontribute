package corpus

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/domain"
)

func TestSearchThreadsPageReturnsNextCursorAndTotal(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("apply repository: %v", err)
	}

	for i := 1; i <= 5; i++ {
		title := "shared term"
		if _, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, i, "open", title, "body", "a", time.Unix(int64(i), 0).UTC(), `{}`); err != nil {
			t.Fatalf("apply thread %d: %v", i, err)
		}
	}

	first, err := c.SearchThreadsPage(ctx, "term", SearchFilter{Limit: 2})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first.Threads) != 2 {
		t.Fatalf("first page threads = %d, want 2", len(first.Threads))
	}
	if first.Total != 5 {
		t.Fatalf("first page total = %d, want 5", first.Total)
	}
	if first.NextCursor == "" {
		t.Fatal("first page next_cursor is empty")
	}

	second, err := c.SearchThreadsPage(ctx, "term", SearchFilter{Limit: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second.Threads) != 2 {
		t.Fatalf("second page threads = %d, want 2", len(second.Threads))
	}
	if second.Total != 5 {
		t.Fatalf("second page total = %d, want 5", second.Total)
	}

	third, err := c.SearchThreadsPage(ctx, "term", SearchFilter{Limit: 2, Cursor: second.NextCursor})
	if err != nil {
		t.Fatalf("third page: %v", err)
	}
	if len(third.Threads) != 1 {
		t.Fatalf("third page threads = %d, want 1", len(third.Threads))
	}
	if third.NextCursor != "" {
		t.Fatalf("third page next_cursor = %q, want empty", third.NextCursor)
	}
	if third.Total != 5 {
		t.Fatalf("third page total = %d, want 5", third.Total)
	}

	seen := map[int]bool{}
	for _, page := range []ThreadSearchPage{first, second, third} {
		for _, thread := range page.Threads {
			if seen[thread.Number] {
				t.Fatalf("duplicate thread number %d", thread.Number)
			}
			seen[thread.Number] = true
		}
	}
	if len(seen) != 5 {
		t.Fatalf("saw %d distinct threads, want 5", len(seen))
	}
}

func TestSearchThreadsPageMalformedCursorRejected(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	for _, cursor := range []string{"not-base64", "e30=", encodeCursor(searchCursor{Scope: "code"})} {
		_, err := c.SearchThreadsPage(ctx, "term", SearchFilter{Limit: 10, Cursor: cursor})
		if err == nil {
			t.Fatalf("cursor %q should be rejected", cursor)
		}
	}
}

func TestSearchThreadsPageHonorsHardMax(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	_, err := c.SearchThreadsPage(ctx, "term", SearchFilter{Limit: 101})
	if err == nil || err.Error() != "search limit cannot exceed 100" {
		t.Fatalf("unexpected error = %v", err)
	}
}

func TestListRepositoriesPageReturnsNextCursorAndTotal(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	for i := 1; i <= 5; i++ {
		name := fmt.Sprintf("repo%d", i)
		if _, err := c.ApplyRepositoryObservation(ctx, "owner", name, "id", time.Unix(int64(10-i), 0).UTC(), `{}`); err != nil {
			t.Fatalf("apply repository %d: %v", i, err)
		}
	}

	first, err := c.ListRepositoriesWithOptions(ctx, "", RepositorySearchOptions{Limit: 2})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first.Repositories) != 2 {
		t.Fatalf("first page repositories = %d, want 2", len(first.Repositories))
	}
	if first.Total != 5 {
		t.Fatalf("first page total = %d, want 5", first.Total)
	}
	if first.NextCursor == "" {
		t.Fatal("first page next_cursor is empty")
	}

	second, err := c.ListRepositoriesWithOptions(ctx, "", RepositorySearchOptions{Limit: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second.Repositories) != 2 {
		t.Fatalf("second page repositories = %d, want 2", len(second.Repositories))
	}

	third, err := c.ListRepositoriesWithOptions(ctx, "", RepositorySearchOptions{Limit: 2, Cursor: second.NextCursor})
	if err != nil {
		t.Fatalf("third page: %v", err)
	}
	if len(third.Repositories) != 1 {
		t.Fatalf("third page repositories = %d, want 1", len(third.Repositories))
	}
	if third.NextCursor != "" {
		t.Fatalf("third page next_cursor = %q, want empty", third.NextCursor)
	}
}

func TestListRepositoriesPageMalformedCursorRejected(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	_, err := c.ListRepositoriesWithOptions(ctx, "", RepositorySearchOptions{Limit: 10, Cursor: "bad-cursor"})
	if err == nil {
		t.Fatal("expected malformed cursor error")
	}
}

func TestListRepositoriesPageHonorsHardMax(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	_, err := c.ListRepositoriesWithOptions(ctx, "", RepositorySearchOptions{Limit: 101})
	if err == nil || err.Error() != "repository list limit cannot exceed 100" {
		t.Fatalf("unexpected error = %v", err)
	}
}

func TestSearchCodePageReturnsNextCursorAndTotal(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	ref := domain.RepoRef{Owner: "owner", Repo: "repo"}
	snapshot := codeindex.Snapshot{
		RepoPath:   "/repo",
		Commit:     "abc",
		CreatedAt:  time.Unix(100, 0).UTC(),
		TotalBytes: 100,
		Documents: []codeindex.Document{
			{Path: "a.go", Content: "term one", Bytes: 8, LanguageHint: "go"},
			{Path: "b.go", Content: "term two", Bytes: 8, LanguageHint: "go"},
			{Path: "c.go", Content: "term three", Bytes: 10, LanguageHint: "go"},
		},
	}
	if _, _, err := c.StoreCodeSnapshot(ctx, ref, snapshot); err != nil {
		t.Fatalf("store snapshot: %v", err)
	}

	first, err := c.SearchCodeWithOptions(ctx, "term", CodeSearchOptions{Ref: ref, Limit: 2})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first.Matches) != 2 {
		t.Fatalf("first page matches = %d, want 2", len(first.Matches))
	}
	if first.Total != 3 {
		t.Fatalf("first page total = %d, want 3", first.Total)
	}
	if first.NextCursor == "" {
		t.Fatal("first page next_cursor is empty")
	}

	second, err := c.SearchCodeWithOptions(ctx, "term", CodeSearchOptions{Ref: ref, Limit: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second.Matches) != 1 {
		t.Fatalf("second page matches = %d, want 1", len(second.Matches))
	}
	if second.NextCursor != "" {
		t.Fatalf("second page next_cursor = %q, want empty", second.NextCursor)
	}

	seen := map[string]bool{}
	for _, page := range []CodeSearchPage{first, second} {
		for _, match := range page.Matches {
			if seen[match.Path] {
				t.Fatalf("duplicate path %q", match.Path)
			}
			seen[match.Path] = true
		}
	}
	if len(seen) != 3 {
		t.Fatalf("saw %d distinct files, want 3", len(seen))
	}

	if diff := cmp.Diff(ref, first.Matches[0].Repo); diff != "" {
		t.Fatalf("match repo mismatch (-want +got):\n%s", diff)
	}
	if first.Matches[0].SnapshotCreatedAt.IsZero() {
		t.Fatal("match snapshot created_at is zero")
	}
}

func TestSearchCodePageMalformedCursorRejected(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	ref := domain.RepoRef{Owner: "owner", Repo: "repo"}
	_, err := c.SearchCodeWithOptions(ctx, "term", CodeSearchOptions{Ref: ref, Limit: 10, Cursor: "invalid"})
	if err == nil {
		t.Fatal("expected malformed cursor error")
	}
}

func TestSearchCodePageHonorsHardMax(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	ref := domain.RepoRef{Owner: "owner", Repo: "repo"}
	_, err := c.SearchCodeWithOptions(ctx, "term", CodeSearchOptions{Ref: ref, Limit: 101})
	if err == nil || err.Error() != "code search limit cannot exceed 100" {
		t.Fatalf("unexpected error = %v", err)
	}
}
