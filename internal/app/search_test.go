package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/lens"
	"github.com/morluto/gitcontribute/internal/mcpserver"
)

func newSearchTestService(t *testing.T) *Service {
	t.Helper()
	ctx := context.Background()
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test", nil)
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

func TestThreadSearchMergesRepositoryAndThreadCoverage(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	repo, err := svc.corpus.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := svc.corpus.ApplyThreadObservation(ctx, repo.ID, corpus.ThreadKindIssue, 1, "open", "search term", "body", "author", time.Unix(2, 0).UTC(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.AdvanceFacet(ctx, repo.ID, nil, "threads", time.Unix(2, 0).UTC(), true, 0); err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.AdvanceFacet(ctx, repo.ID, &thread.ID, FacetIssueComments, time.Unix(2, 0).UTC(), true, 0); err != nil {
		t.Fatal(err)
	}

	result, err := svc.Search(ctx, "search", cli.SearchOptions{Kind: "issues", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(result.Matches))
	}
	if diff := cmp.Diff([]string{FacetIssueComments, "threads"}, result.Matches[0].Coverage); diff != "" {
		t.Fatalf("coverage mismatch (-want +got):\n%s", diff)
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

func seedLensCorpus(t *testing.T, svc *Service) {
	t.Helper()
	ctx := context.Background()
	c := svc.corpus

	repo, err := c.UpsertRepository(ctx, corpus.Repository{
		Owner:           "owner",
		Name:            "repo",
		ExternalID:      "id",
		Language:        "Go",
		Stars:           50,
		Watchers:        2,
		Forks:           1,
		SourceUpdatedAt: time.Unix(1000, 0).UTC(),
	}, `{}`)
	if err != nil {
		t.Fatalf("seed repository: %v", err)
	}

	base := time.Unix(1000, 0).UTC()
	threads := []struct {
		number int
		title  string
		body   string
		labels []string
		state  string
	}{
		{1, "fix login crash", "login crashes on startup", []string{"bug"}, "open"},
		{2, "login crash on startup", "the login page crashes", nil, "open"},
		{3, "unrelated feature", "add dark mode", nil, "closed"},
		{4, "fix login crash", "duplicate of #1", nil, "closed"},
		{5, "unrelated open issue", "add keyboard shortcuts", nil, "open"},
	}
	for _, th := range threads {
		updated := base.Add(time.Duration(5-th.number) * time.Hour)
		if _, err := c.UpsertThread(ctx, corpus.Thread{
			RepositoryID:    repo.ID,
			Kind:            corpus.ThreadKindIssue,
			Number:          th.number,
			State:           th.state,
			Title:           th.title,
			Body:            th.body,
			Author:          "alice",
			Labels:          th.labels,
			SourceCreatedAt: updated,
			SourceUpdatedAt: updated,
		}, `{}`); err != nil {
			t.Fatalf("seed thread %d: %v", th.number, err)
		}
	}
}

func TestSearchWithLensRanksAndFiltersThreads(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	seedLensCorpus(t, svc)

	def := lens.Definition{
		Name: "active-go",
		Filter: lens.Filter{
			Kinds:           []string{"issue"},
			States:          []string{"open"},
			Languages:       []string{"Go"},
			ExcludeArchived: true,
			MinStars:        10,
		},
		Weights: map[string]float64{"text_relevance": 2, "freshness": 1},
	}
	if _, err := svc.corpus.SaveLens(ctx, def); err != nil {
		t.Fatalf("save lens: %v", err)
	}

	svc.SetClock(func() time.Time { return time.Unix(100000, 0).UTC() })

	result, err := svc.Search(ctx, "login", cli.SearchOptions{Kind: "issues", Lens: "active-go", Limit: 2})
	if err != nil {
		t.Fatalf("search with lens: %v", err)
	}
	if result.Total != 2 {
		t.Fatalf("total = %d, want 2", result.Total)
	}
	if len(result.Matches) != 2 {
		t.Fatalf("matches = %d, want 2", len(result.Matches))
	}
	if result.Matches[0].Number != 1 || result.Matches[1].Number != 2 {
		t.Fatalf("expected issue 1 then issue 2, got %d then %d", result.Matches[0].Number, result.Matches[1].Number)
	}
	if result.Matches[0].Score <= result.Matches[1].Score {
		t.Fatalf("expected first score > second score, got %v and %v", result.Matches[0].Score, result.Matches[1].Score)
	}
}

func TestSearchWithLensDoesNotLeakMatchesForMissingRepository(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	seedLensCorpus(t, svc)

	if _, err := svc.corpus.SaveLens(ctx, lens.Definition{
		Name:    "active-go",
		Weights: map[string]float64{"text_relevance": 1},
	}); err != nil {
		t.Fatalf("save lens: %v", err)
	}

	result, err := svc.Search(ctx, "login", cli.SearchOptions{
		Kind: "issues", Repo: "missing/repo", Lens: "active-go", Limit: 10,
	})
	if err != nil {
		t.Fatalf("search with lens: %v", err)
	}
	if result.Total != 0 || len(result.Matches) != 0 {
		t.Fatalf("missing repository returned matches: %+v", result)
	}
}

func TestExplainLens(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	seedLensCorpus(t, svc)

	def := lens.Definition{
		Name: "active-go",
		Filter: lens.Filter{
			Kinds:           []string{"issue"},
			States:          []string{"open"},
			Languages:       []string{"Go"},
			ExcludeArchived: true,
			MinStars:        10,
		},
		Weights: map[string]float64{"text_relevance": 2, "freshness": 1, "novel_signal": 0.5},
	}
	if _, err := svc.corpus.SaveLens(ctx, def); err != nil {
		t.Fatalf("save lens: %v", err)
	}

	svc.SetClock(func() time.Time { return time.Unix(100000, 0).UTC() })

	ex, err := svc.ExplainLens(ctx, "active-go", "owner/repo#1", cli.LensExplainOptions{Query: "login"})
	if err != nil {
		t.Fatalf("explain lens: %v", err)
	}
	if ex.Lens.Name != "active-go" {
		t.Fatalf("lens name = %q", ex.Lens.Name)
	}
	if ex.Candidate.Number != 1 {
		t.Fatalf("candidate number = %d, want 1", ex.Candidate.Number)
	}
	if ex.Candidate.Title != "fix login crash" || ex.Candidate.URL != "https://github.com/owner/repo/issues/1" || ex.Query != "login" {
		t.Fatalf("unexpected candidate facts or query: candidate=%+v query=%q", ex.Candidate, ex.Query)
	}
	if ex.Score <= 0 {
		t.Fatalf("expected positive score, got %v", ex.Score)
	}
	if ex.PopulationSize != 2 {
		t.Fatalf("population size = %d, want 2", ex.PopulationSize)
	}
	if len(ex.Signals) != 3 {
		t.Fatalf("signals = %d, want 3", len(ex.Signals))
	}
	var foundMissing bool
	for _, sig := range ex.Signals {
		if sig.Missing && sig.Name == "novel_signal" {
			foundMissing = true
		}
	}
	if !foundMissing {
		t.Fatalf("expected missing signal to be reported: %+v", ex.Signals)
	}
	search, err := svc.Search(ctx, "login", cli.SearchOptions{Kind: "issues", Lens: "active-go", Limit: 10})
	if err != nil {
		t.Fatalf("search with lens: %v", err)
	}
	if len(search.Matches) == 0 || ex.Score != search.Matches[0].Score {
		t.Fatalf("explanation score %v does not match search result %+v", ex.Score, search.Matches)
	}
}

func TestSearchAllHonorsRepositoryScopeForEveryKind(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	for _, name := range []string{"one", "two"} {
		repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{
			Owner: "owner", Name: name, Description: "shared term",
		}, `{}`)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.corpus.UpsertThread(ctx, corpus.Thread{
			RepositoryID: repo.ID, Kind: corpus.ThreadKindIssue, Number: 1,
			State: "open", Title: "shared term", SourceUpdatedAt: time.Now().UTC(),
		}, `{}`); err != nil {
			t.Fatal(err)
		}
	}

	result, err := svc.Search(ctx, "shared term", cli.SearchOptions{Kind: "all", Repo: "owner/one", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 || len(result.Matches) != 2 {
		t.Fatalf("scoped combined result = %+v", result)
	}
	for _, match := range result.Matches {
		if match.Repo.String() != "owner/one" {
			t.Fatalf("repository scope leaked match: %+v", match)
		}
	}
}

func TestCodeLensUsesSnapshotTimeForFreshnessFilter(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	ref := domain.RepoRef{Owner: "owner", Repo: "repo"}
	if _, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: ref.Owner, Name: ref.Repo}, `{}`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, _, err := svc.corpus.StoreCodeSnapshot(ctx, ref, codeindex.Snapshot{
		RepoPath: "/repo", Commit: "abc", CreatedAt: now,
		Documents: []codeindex.Document{{Path: "fresh.go", Content: "freshSearchTerm", Bytes: 15, LanguageHint: "go"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.corpus.SaveLens(ctx, lens.Definition{
		Name: "fresh", Filter: lens.Filter{UpdatedWithin: time.Hour}, Weights: map[string]float64{"freshness": 1},
	}); err != nil {
		t.Fatal(err)
	}
	svc.SetClock(func() time.Time { return now.Add(time.Minute) })

	result, err := svc.Search(ctx, "freshSearchTerm", cli.SearchOptions{Kind: "code", Repo: ref.String(), Lens: "fresh", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || len(result.Matches) != 1 {
		t.Fatalf("fresh code was filtered out: %+v", result)
	}
}
