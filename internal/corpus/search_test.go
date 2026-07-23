package corpus

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/domain"
)

func TestSearchThreadsPageReturnsNextCursorAndTotal(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	_, err := c.SearchThreadsPage(ctx, "term", SearchFilter{Limit: 101})
	if err == nil || err.Error() != "search limit cannot exceed 100" {
		t.Fatalf("unexpected error = %v", err)
	}
}

func TestSearchThreadsWeightsTitleLabelsAndSupportsNewestSort(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	threads := []Thread{
		{RepositoryID: repo.ID, Kind: ThreadKindIssue, Number: 1, State: "open", Title: "music playback fails", Body: "short", SourceUpdatedAt: time.Unix(100, 0).UTC()},
		{RepositoryID: repo.ID, Kind: ThreadKindIssue, Number: 2, State: "open", Title: "unrelated request", Body: "music music music music", SourceUpdatedAt: time.Unix(300, 0).UTC()},
		{RepositoryID: repo.ID, Kind: ThreadKindIssue, Number: 3, State: "open", Title: "label-only request", Labels: []string{"music"}, SourceUpdatedAt: time.Unix(200, 0).UTC()},
	}
	for _, thread := range threads {
		thread.SourceCreatedAt = thread.SourceUpdatedAt
		if _, err := c.UpsertThread(ctx, thread, `{}`); err != nil {
			t.Fatal(err)
		}
	}

	relevance, err := c.SearchThreadsPage(ctx, "music", SearchFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(relevance.Threads) != 3 || relevance.Threads[0].Number != 1 {
		t.Fatalf("weighted relevance order = %+v", relevance.Threads)
	}

	newest, err := c.SearchThreadsPage(ctx, "music", SearchFilter{Limit: 10, Sort: "updated"})
	if err != nil {
		t.Fatal(err)
	}
	if len(newest.Threads) != 3 || newest.Threads[0].Number != 2 || newest.Threads[1].Number != 3 {
		t.Fatalf("updated order = %+v", newest.Threads)
	}
}

func TestSearchThreadsPageAppliesMetadataFiltersAndBindsCursor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	for i, thread := range []Thread{
		{Kind: ThreadKindIssue, Number: 1, State: "open", Title: "shared term", Author: "Alice", Labels: []string{"bug", "help wanted"}, SourceUpdatedAt: time.Unix(100, 0).UTC()},
		{Kind: ThreadKindIssue, Number: 2, State: "closed", Title: "shared term", Author: "alice", Labels: []string{"bug"}, SourceUpdatedAt: time.Unix(200, 0).UTC()},
		{Kind: ThreadKindIssue, Number: 3, State: "open", Title: "shared term", Author: "bob", Labels: []string{"bug"}, SourceUpdatedAt: time.Unix(300, 0).UTC()},
	} {
		thread.RepositoryID = repo.ID
		thread.SourceCreatedAt = thread.SourceUpdatedAt
		if _, err := c.UpsertThread(ctx, thread, `{}`); err != nil {
			t.Fatalf("seed thread %d: %v", i, err)
		}
	}
	filter := SearchFilter{State: "open", Labels: []string{"bug"}, UpdatedAfter: time.Unix(50, 0).UTC(), Limit: 1}
	page, err := c.SearchThreadsPage(ctx, "term", filter)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(page.Threads) != 1 || page.Threads[0].Number != 3 || page.Total != 2 || page.NextCursor == "" {
		t.Fatalf("page = %+v", page)
	}
	filter.Cursor = page.NextCursor
	filter.Author = "bob"
	if _, err := c.SearchThreadsPage(ctx, "term", filter); err == nil {
		t.Fatal("cursor should be rejected when metadata filters change")
	}
	authorPage, err := c.SearchThreadsPage(ctx, "term", SearchFilter{Author: "ALICE", Limit: 10})
	if err != nil || len(authorPage.Threads) != 2 {
		t.Fatalf("case-insensitive author filter = %+v, err=%v", authorPage, err)
	}
}

func TestSearchThreadsPageDoesNotTreatUnknownMergeStateAsFalse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, thread := range []Thread{
		{Kind: ThreadKindPullRequest, Number: 1, State: "closed", Title: "shared term", Merged: true, SourceUpdatedAt: time.Unix(10, 0).UTC()},
		{Kind: ThreadKindPullRequest, Number: 2, State: "closed", Title: "shared term", MergedKnown: true, SourceUpdatedAt: time.Unix(20, 0).UTC()},
		{Kind: ThreadKindPullRequest, Number: 3, State: "closed", Title: "shared term", SourceUpdatedAt: time.Unix(30, 0).UTC()},
	} {
		thread.RepositoryID = repo.ID
		if _, err := c.UpsertThread(ctx, thread, `{}`); err != nil {
			t.Fatal(err)
		}
	}
	merged, unmerged := true, false
	mergedPage, err := c.SearchThreadsPage(ctx, "term", SearchFilter{Merged: &merged, Limit: 10})
	if err != nil || len(mergedPage.Threads) != 1 || mergedPage.Threads[0].Number != 1 {
		t.Fatalf("merged page = %+v, %v", mergedPage, err)
	}
	unmergedPage, err := c.SearchThreadsPage(ctx, "term", SearchFilter{Merged: &unmerged, Limit: 10})
	if err != nil || len(unmergedPage.Threads) != 1 || unmergedPage.Threads[0].Number != 2 {
		t.Fatalf("unmerged page = %+v, %v", unmergedPage, err)
	}
}

func TestSearchThreadsPageIncludesAtomicFacetEvidence(t *testing.T) {
	t.Parallel()
	ctx, c, thread, newer := seedFacetSearch(t)

	page, err := c.SearchThreadsPage(ctx, "transport invariant", SearchFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Threads) != 1 || page.Threads[0].ID != thread.ID {
		t.Fatalf("facet search page = %+v", page)
	}
	if page.Threads[0].MatchSource != "issue_comments" || !strings.Contains(page.Threads[0].MatchExcerpt, "transport") {
		t.Fatalf("facet match evidence = %+v", page.Threads[0])
	}
	duplicatePage, err := c.SearchThreadsPage(ctx, "plain", SearchFilter{Limit: 10})
	if err != nil || duplicatePage.Total != 1 || len(duplicatePage.Threads) != 1 {
		t.Fatalf("thread/facet duplicate search = %+v, err=%v", duplicatePage, err)
	}
	evidence, found, err := c.FindThreadSearchEvidence(ctx, thread.ID, "transport invariant")
	if err != nil {
		t.Fatal(err)
	}
	if !found || evidence.Source != "issue_comments" || !strings.Contains(evidence.Text, "boundary invariant") || !evidence.SourceUpdatedAt.Equal(newer) {
		t.Fatalf("exact facet evidence = %+v", evidence)
	}
}

func TestSearchableFacetReplacementHonorsSourceOrdering(t *testing.T) {
	t.Parallel()
	ctx, c, thread, _ := seedFacetSearch(t)
	older := time.Unix(10, 0).UTC()
	if err := c.ApplyFacetObservationSet(ctx, thread.RepositoryID, &thread.ID, "issue_comments", older, []FacetObservationInput{{SourceUpdatedAt: older, Payload: `[]`, SearchText: "stale replacement"}}, true, 0); err != nil {
		t.Fatal(err)
	}
	page, err := c.SearchThreadsPage(ctx, "transport invariant", SearchFilter{Limit: 10})
	if err != nil || len(page.Threads) != 1 {
		t.Fatalf("stale replacement changed search projection: page=%+v err=%v", page, err)
	}

	latest := time.Unix(30, 0).UTC()
	if err := c.ApplyFacetObservationSet(ctx, thread.RepositoryID, &thread.ID, "issue_comments", latest, []FacetObservationInput{{SourceUpdatedAt: latest, Payload: `[]`, SearchText: "replacement evidence"}}, true, 0); err != nil {
		t.Fatal(err)
	}
	oldPage, err := c.SearchThreadsPage(ctx, "transport", SearchFilter{Limit: 10})
	if err != nil || len(oldPage.Threads) != 0 {
		t.Fatalf("old facet term remains searchable: page=%+v err=%v", oldPage, err)
	}
	newPage, err := c.SearchThreadsPage(ctx, "replacement", SearchFilter{Limit: 10})
	if err != nil || len(newPage.Threads) != 1 {
		t.Fatalf("replacement facet term missing: page=%+v err=%v", newPage, err)
	}

	if err := c.ApplyFacetObservationSet(ctx, thread.RepositoryID, &thread.ID, "issue_comments", time.Unix(40, 0).UTC(), nil, true, 0); err != nil {
		t.Fatal(err)
	}
	emptyPage, err := c.SearchThreadsPage(ctx, "replacement", SearchFilter{Limit: 10})
	if err != nil || len(emptyPage.Threads) != 0 {
		t.Fatalf("empty facet replacement remains searchable: page=%+v err=%v", emptyPage, err)
	}
}

func TestThreadSearchReportsBoundedHydratedDocument(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 1, "open", "titlematch", "plain", "a", time.Unix(2, 0).UTC(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	searchText := "insideboundary " + strings.Repeat("x", maxThreadFacetSearchCharacters) + " titlematch outsideboundary"
	if err := c.ApplyFacetObservationSet(ctx, repo.ID, &thread.ID, "issue_comments", time.Unix(3, 0).UTC(), []FacetObservationInput{{SourceUpdatedAt: time.Unix(3, 0).UTC(), SearchText: searchText}}, true, 0); err != nil {
		t.Fatal(err)
	}
	page, err := c.SearchThreadsPage(ctx, "insideboundary", SearchFilter{Limit: 10})
	if err != nil || len(page.Threads) != 1 || !page.Threads[0].MatchTruncated || page.Threads[0].MatchSource != "hydrated_facets" {
		t.Fatalf("bounded search page = %+v, err=%v", page, err)
	}
	titlePage, err := c.SearchThreadsPage(ctx, "titlematch", SearchFilter{Limit: 10})
	if err != nil || len(titlePage.Threads) != 1 || titlePage.Threads[0].MatchSource != "thread" {
		t.Fatalf("truncated facet must not replace title attribution: page=%+v err=%v", titlePage, err)
	}
	omitted, err := c.SearchThreadsPage(ctx, "outsideboundary", SearchFilter{Limit: 10})
	if err != nil || omitted.Total != 0 {
		t.Fatalf("omitted suffix search = %+v, err=%v", omitted, err)
	}
}

func seedFacetSearch(t *testing.T) (context.Context, *Corpus, *Thread, time.Time) {
	t.Helper()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 1, "open", "plain title", "plain body", "a", time.Unix(2, 0).UTC(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	newer := time.Unix(20, 0).UTC()
	pages := []FacetObservationInput{
		{SourceUpdatedAt: newer.Add(-time.Second), Payload: `[{"Body":"transport"}]`, SearchText: "alice transport"},
		{SourceUpdatedAt: newer, Payload: `[{"Body":"boundary"}]`, SearchText: "boundary invariant"},
	}
	// One facet snapshot is one search document even though its source payload
	// remains paginated.
	pages[0].SearchText += "\n" + pages[1].SearchText + "\nplain"
	pages[1].SearchText = ""
	if err := c.ApplyFacetObservationSet(ctx, repo.ID, &thread.ID, "issue_comments", newer, pages, true, 0); err != nil {
		t.Fatal(err)
	}
	return ctx, c, thread, newer
}

func TestSearchThreadsPageFiltersByAssociationAndAssignee(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	for i, thread := range []Thread{
		{Kind: ThreadKindIssue, Number: 1, State: "open", Title: "shared term", Author: "Alice", AuthorAssociation: "OWNER", Assignees: []string{"alice"}, SourceUpdatedAt: time.Unix(100, 0).UTC()},
		{Kind: ThreadKindIssue, Number: 2, State: "open", Title: "shared term", Author: "bob", AuthorAssociation: "CONTRIBUTOR", Assignees: []string{"alice", "bob"}, SourceUpdatedAt: time.Unix(200, 0).UTC()},
		{Kind: ThreadKindIssue, Number: 3, State: "open", Title: "shared term", Author: "charlie", AuthorAssociation: "NONE", Assignees: []string{"bob"}, SourceUpdatedAt: time.Unix(300, 0).UTC()},
	} {
		thread.RepositoryID = repo.ID
		thread.SourceCreatedAt = thread.SourceUpdatedAt
		if _, err := c.UpsertThread(ctx, thread, `{}`); err != nil {
			t.Fatalf("seed thread %d: %v", i, err)
		}
	}

	assocPage, err := c.SearchThreadsPage(ctx, "term", SearchFilter{Association: "owner", Limit: 10})
	if err != nil || len(assocPage.Threads) != 1 || assocPage.Threads[0].Number != 1 {
		t.Fatalf("association filter = %+v, err=%v", assocPage, err)
	}

	assigneePage, err := c.SearchThreadsPage(ctx, "term", SearchFilter{Assignee: "ALICE", Limit: 10})
	if err != nil || len(assigneePage.Threads) != 2 {
		t.Fatalf("assignee filter = %+v, err=%v", assigneePage, err)
	}
	for _, th := range assigneePage.Threads {
		if th.Number != 1 && th.Number != 2 {
			t.Fatalf("unexpected assignee match: %d", th.Number)
		}
	}

	combined, err := c.SearchThreadsPage(ctx, "term", SearchFilter{Association: "contributor", Assignee: "bob", Limit: 10})
	if err != nil || len(combined.Threads) != 1 || combined.Threads[0].Number != 2 {
		t.Fatalf("combined filter = %+v, err=%v", combined, err)
	}
}

func TestListRepositoriesPageReturnsNextCursorAndTotal(t *testing.T) {
	t.Parallel()
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
	blank, err := c.ListRepositoriesWithOptions(ctx, " \t ", RepositorySearchOptions{Limit: 10})
	if err != nil || len(blank.Repositories) != 5 {
		t.Fatalf("whitespace-only query = %+v, err=%v", blank, err)
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

func TestListRepositoriesQueryWithCursorParenthesizesOR(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	if _, err := c.UpsertRepository(ctx, Repository{
		Owner: "alpha", Name: "owner-match", Description: "x",
		SourceUpdatedAt: time.Unix(300, 0).UTC(),
	}, `{}`); err != nil {
		t.Fatalf("seed owner match: %v", err)
	}
	if _, err := c.UpsertRepository(ctx, Repository{
		Owner: "beta", Name: "repo2", Description: "query-match",
		SourceUpdatedAt: time.Unix(200, 0).UTC(),
	}, `{}`); err != nil {
		t.Fatalf("seed description match 1: %v", err)
	}
	if _, err := c.UpsertRepository(ctx, Repository{
		Owner: "gamma", Name: "repo3", Description: "query-match",
		SourceUpdatedAt: time.Unix(100, 0).UTC(),
	}, `{}`); err != nil {
		t.Fatalf("seed description match 2: %v", err)
	}

	first, err := c.ListRepositoriesWithOptions(ctx, "match", RepositorySearchOptions{Limit: 1})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first.Repositories) != 1 || first.Repositories[0].Name != "owner-match" {
		t.Fatalf("first page = %+v", first.Repositories)
	}

	second, err := c.ListRepositoriesWithOptions(ctx, "match", RepositorySearchOptions{Limit: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second.Repositories) != 2 {
		t.Fatalf("second page repositories = %d, want 2", len(second.Repositories))
	}
	names := []string{second.Repositories[0].Name, second.Repositories[1].Name}
	want := []string{"repo2", "repo3"}
	if diff := cmp.Diff(want, names); diff != "" {
		t.Fatalf("second page mismatch (-want +got):\n%s", diff)
	}
}

func TestListRepositoriesPageMalformedCursorRejected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	_, err := c.ListRepositoriesWithOptions(ctx, "", RepositorySearchOptions{Limit: 10, Cursor: "bad-cursor"})
	if err == nil {
		t.Fatal("expected malformed cursor error")
	}
}

func TestListRepositoriesPageHonorsHardMax(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	_, err := c.ListRepositoriesWithOptions(ctx, "", RepositorySearchOptions{Limit: 101})
	if err == nil || err.Error() != "repository list limit cannot exceed 100" {
		t.Fatalf("unexpected error = %v", err)
	}
}

func TestRepositorySearchWeightsNameTopicsDescriptionAndSupportsNewestSort(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	repositories := []Repository{
		{Owner: "acme", Name: "music", Description: "tool", SourceUpdatedAt: time.Unix(100, 0).UTC()},
		{Owner: "acme", Name: "topic-match", Topics: []string{"music"}, SourceUpdatedAt: time.Unix(200, 0).UTC()},
		{Owner: "acme", Name: "description-match", Description: "music", SourceUpdatedAt: time.Unix(300, 0).UTC()},
	}
	for _, repository := range repositories {
		if _, err := c.UpsertRepository(ctx, repository, `{}`); err != nil {
			t.Fatal(err)
		}
	}

	relevance, err := c.ListRepositoriesWithOptions(ctx, "music", RepositorySearchOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{relevance.Repositories[0].Name, relevance.Repositories[1].Name, relevance.Repositories[2].Name}; !slices.Equal(got, []string{"music", "topic-match", "description-match"}) {
		t.Fatalf("weighted repository order = %v", got)
	}

	newest, err := c.ListRepositoriesWithOptions(ctx, "music", RepositorySearchOptions{Limit: 10, Sort: "updated"})
	if err != nil {
		t.Fatal(err)
	}
	if newest.Repositories[0].Name != "description-match" {
		t.Fatalf("updated repository order = %+v", newest.Repositories)
	}
}

func TestRepositorySearchMatchesCanonicalSlug(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	if _, err := c.UpsertRepository(ctx, Repository{Owner: "acme", Name: "rocket", SourceUpdatedAt: time.Unix(100, 0).UTC()}, `{}`); err != nil {
		t.Fatal(err)
	}
	if _, err := c.UpsertRepository(ctx, Repository{Owner: "acme", Name: "other", Description: "rocket", SourceUpdatedAt: time.Unix(200, 0).UTC()}, `{}`); err != nil {
		t.Fatal(err)
	}

	page, err := c.ListRepositoriesWithOptions(ctx, "acme/rocket", RepositorySearchOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Repositories) != 1 || page.Repositories[0].Owner != "acme" || page.Repositories[0].Name != "rocket" {
		t.Fatalf("canonical slug search = %+v", page.Repositories)
	}
}

func TestSearchCodePageReturnsNextCursorAndTotal(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	ref := domain.RepoRef{Owner: "owner", Repo: "repo"}
	_, err := c.SearchCodeWithOptions(ctx, "term", CodeSearchOptions{Ref: ref, Limit: 10, Cursor: "invalid"})
	if err == nil {
		t.Fatal("expected malformed cursor error")
	}
}

func TestSearchCodePageHonorsHardMax(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	ref := domain.RepoRef{Owner: "owner", Repo: "repo"}
	_, err := c.SearchCodeWithOptions(ctx, "term", CodeSearchOptions{Ref: ref, Limit: 101})
	if err == nil || err.Error() != "code search limit cannot exceed 100" {
		t.Fatalf("unexpected error = %v", err)
	}
}
