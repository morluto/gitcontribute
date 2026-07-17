package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/mcpserver"
)

func newSearchTestService(t *testing.T) *Service {
	t.Helper()
	ctx := context.Background()
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	if _, err := svc.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}
	return svc
}

func TestSearchReturnsNextCursorAndCoverage(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	c := svc.corpus

	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("apply repository: %v", err)
	}
	run, err := c.StartRun(ctx, "sync")
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if err := c.AdvanceFacet(ctx, repo.ID, nil, "metadata", time.Unix(2, 0).UTC(), true, run.ID); err != nil {
		t.Fatalf("advance metadata facet: %v", err)
	}
	if err := c.AdvanceFacet(ctx, repo.ID, nil, "threads", time.Unix(2, 0).UTC(), true, run.ID); err != nil {
		t.Fatalf("advance threads facet: %v", err)
	}

	for i := 1; i <= 5; i++ {
		if _, err := c.ApplyThreadObservation(ctx, repo.ID, corpus.ThreadKindIssue, i, "open", "term title", "body", "a", time.Unix(int64(i), 0).UTC(), `{}`); err != nil {
			t.Fatalf("apply thread %d: %v", i, err)
		}
	}

	svc.SetClock(func() time.Time { return time.Unix(100, 0).UTC() })

	first, err := svc.Search(ctx, "term", cli.SearchOptions{Kind: "issues", Limit: 2})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first.Matches) != 2 {
		t.Fatalf("first page matches = %d, want 2", len(first.Matches))
	}
	if first.Total != 5 {
		t.Fatalf("first page total = %d, want 5", first.Total)
	}
	if first.NextCursor == "" {
		t.Fatal("first page next_cursor is empty")
	}

	second, err := svc.Search(ctx, "term", cli.SearchOptions{Kind: "issues", Limit: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second.Matches) != 2 {
		t.Fatalf("second page matches = %d, want 2", len(second.Matches))
	}
	if second.Total != 5 {
		t.Fatalf("second page total = %d, want 5", second.Total)
	}

	third, err := svc.Search(ctx, "term", cli.SearchOptions{Kind: "issues", Limit: 2, Cursor: second.NextCursor})
	if err != nil {
		t.Fatalf("third page: %v", err)
	}
	if len(third.Matches) != 1 {
		t.Fatalf("third page matches = %d, want 1", len(third.Matches))
	}
	if third.NextCursor != "" {
		t.Fatalf("third page next_cursor = %q, want empty", third.NextCursor)
	}

	for _, match := range first.Matches {
		if match.Freshness == "" {
			t.Fatalf("match freshness empty: %+v", match)
		}
		if diff := cmp.Diff([]string{"metadata", "threads"}, match.Coverage); diff != "" {
			t.Fatalf("match coverage mismatch (-want +got):\n%s", diff)
		}
	}

	mcpFirst, err := (&MCPReader{Service: svc}).Search(ctx, mcpserver.SearchInput{Query: "term", Kind: "issues", Limit: 2})
	if err != nil || mcpFirst.NextCursor == "" || len(mcpFirst.Matches) != 2 {
		t.Fatalf("MCP first page = %+v, err=%v", mcpFirst, err)
	}
	mcpSecond, err := (&MCPReader{Service: svc}).Search(ctx, mcpserver.SearchInput{Query: "term", Kind: "issues", Limit: 2, Cursor: mcpFirst.NextCursor})
	if err != nil || len(mcpSecond.Matches) != 2 {
		t.Fatalf("MCP second page = %+v, err=%v", mcpSecond, err)
	}
}

func TestSearchRejectsMalformedCursor(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	_, err := svc.Search(ctx, "term", cli.SearchOptions{Kind: "issues", Limit: 10, Cursor: "not-a-cursor"})
	if err == nil || !strings.Contains(err.Error(), "invalid cursor") {
		t.Fatalf("unexpected error = %v", err)
	}
}

func TestSearchRejectsCursorForAll(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	_, err := svc.Search(ctx, "term", cli.SearchOptions{Kind: "all", Limit: 10, Cursor: "cursor"})
	if err == nil || err.Error() != "cursor pagination is not supported for combined search" {
		t.Fatalf("unexpected error = %v", err)
	}
}

func TestSearchAllRanksAcrossKindsAndPreservesTotal(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{
		Owner: "owner", Name: "repo", Description: "term in description", SourceUpdatedAt: time.Unix(90, 0).UTC(),
	}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.corpus.ApplyThreadObservation(ctx, repo.ID, corpus.ThreadKindIssue, 1, "open", "term", "body", "alice", time.Unix(95, 0).UTC(), `{}`); err != nil {
		t.Fatal(err)
	}
	svc.SetClock(func() time.Time { return time.Unix(100, 0).UTC() })
	result, err := svc.Search(ctx, "term", cli.SearchOptions{Kind: "all", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 || len(result.Matches) != 1 || result.Matches[0].Kind != corpus.ThreadKindIssue {
		t.Fatalf("combined result = %+v", result)
	}
}

func TestSearchHardMaxLimit(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	_, err := svc.Search(ctx, "term", cli.SearchOptions{Kind: "issues", Limit: 101})
	if err == nil || err.Error() != "search limit cannot exceed 100" {
		t.Fatalf("unexpected error = %v", err)
	}
}

func TestExplainMatchReturnsFactualReasons(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	c := svc.corpus

	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("apply repository: %v", err)
	}
	run, err := c.StartRun(ctx, "sync")
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if err := c.AdvanceFacet(ctx, repo.ID, nil, "metadata", time.Unix(2, 0).UTC(), true, run.ID); err != nil {
		t.Fatalf("advance metadata facet: %v", err)
	}

	updated := time.Unix(50, 0).UTC()
	if _, err := c.ApplyThreadObservation(ctx, repo.ID, corpus.ThreadKindIssue, 1, "open", "term title", "body", "a", updated, `{}`); err != nil {
		t.Fatalf("apply thread: %v", err)
	}

	svc.SetClock(func() time.Time { return time.Unix(100, 0).UTC() })

	result, err := svc.Search(ctx, "term", cli.SearchOptions{Kind: "issues", Limit: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(result.Matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(result.Matches))
	}

	explanation, err := svc.ExplainMatch(ctx, "term", result.Matches[0])
	if err != nil {
		t.Fatalf("explain match: %v", err)
	}
	if explanation.Score <= 0 {
		t.Fatalf("score = %f, want positive", explanation.Score)
	}

	want := []string{
		`query term "term" matched in title`,
		"all query terms matched",
		"source updated",
		"coverage includes metadata",
	}
	for _, w := range want {
		found := false
		for _, reason := range explanation.Reasons {
			if strings.Contains(reason, w) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing reason %q in %v", w, explanation.Reasons)
		}
	}
}

func TestSearchDefaultLimitAndMax(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)

	result, err := svc.Search(ctx, "x", cli.SearchOptions{})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Limit != 20 {
		t.Fatalf("limit = %d, want 20", result.Limit)
	}
}
