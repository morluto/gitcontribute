package corpus

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

func TestThreadSearchCursorRejectsDifferentLabelSets(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	for number := 1; number <= 3; number++ {
		_, err := c.UpsertThread(ctx, Thread{RepositoryID: repo.ID, Kind: domain.IssueKind, Number: number, State: "open", Title: "search term", Labels: []string{"bug,urgent", "bug", "urgent", " bug", "Ä", "ä"}, SourceUpdatedAt: time.Unix(int64(number), 0)}, `{}`)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct{ labels, replacement []string }{
		{[]string{"bug,urgent"}, []string{"bug", "urgent"}},
		{[]string{" bug"}, []string{"bug"}},
		{[]string{"Ä"}, []string{"ä"}},
	} {
		labels, replacement := test.labels, test.replacement
		first, err := c.SearchThreadsPage(ctx, "term", SearchFilter{Labels: labels, Page: mustSearchPage(t, 1)})
		if err != nil || first.NextCursor == "" {
			t.Fatalf("first page = %+v, %v", first, err)
		}
		if _, err := c.SearchThreadsPage(ctx, "term", SearchFilter{Labels: replacement, Page: mustSearchPage(t, 1, first.NextCursor)}); err == nil {
			t.Errorf("cursor for labels %q accepted different labels %q", labels, replacement)
		}
	}
}

func TestThreadSearchUnknownMergeFilter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	for i, merge := range []domain.MergeStatus{domain.UnknownMergeStatus(), domain.UnknownMergeStatus(), domain.UnmergedStatus(), domain.MergedStatus(time.Time{})} {
		_, err := c.UpsertThread(ctx, Thread{RepositoryID: repo.ID, Kind: domain.PullRequestKind, Number: i + 1, State: "closed", Title: "search term", Merge: merge, SourceUpdatedAt: time.Unix(int64(i+2), 0)}, `{}`)
		if err != nil {
			t.Fatal(err)
		}
	}
	merge, err := ParseMergeFilter("unknown")
	if err != nil {
		t.Fatal(err)
	}
	first, err := c.SearchThreadsPage(ctx, "term", SearchFilter{Merge: merge, Page: mustSearchPage(t, 1)})
	if err != nil {
		t.Fatal(err)
	}
	if first.Total != 2 || len(first.Threads) != 1 || first.Threads[0].Merge.Known() || first.NextCursor == "" || first.UnknownMergeCount != 0 {
		t.Fatalf("unknown merge first page = %+v", first)
	}
	second, err := c.SearchThreadsPage(ctx, "term", SearchFilter{Merge: merge, Page: mustSearchPage(t, 1, first.NextCursor)})
	if err != nil {
		t.Fatal(err)
	}
	if second.Total != 2 || len(second.Threads) != 1 || second.Threads[0].Merge.Known() || second.Threads[0].ID == first.Threads[0].ID || second.NextCursor != "" {
		t.Fatalf("unknown merge second page = %+v", second)
	}
}

func TestThreadSearchKeepsTitleMatchesAheadOfIncidentalMentions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	seedSearchRelevance(t, c)
	for _, mode := range []TermMatch{MatchAllTerms(), MatchAnyTerm()} {
		first, err := c.SearchThreadsPage(ctx, "connection timeout", SearchFilter{TermMatch: mode, Page: mustSearchPage(t, 1)})
		if err != nil {
			t.Fatal(err)
		}
		if first.Total != 2 || len(first.Threads) != 1 || first.Threads[0].Number != 1 || first.NextCursor == "" {
			t.Fatalf("%s first page numbers = %v, total=%d", mode, searchThreadNumbers(first), first.Total)
		}
		second, err := c.SearchThreadsPage(ctx, "connection timeout", SearchFilter{TermMatch: mode, Page: mustSearchPage(t, 1, first.NextCursor)})
		if err != nil {
			t.Fatal(err)
		}
		if second.Total != 2 || len(second.Threads) != 1 || second.Threads[0].Number != 2 || second.NextCursor != "" {
			t.Fatalf("%s second page numbers = %v, total=%d", mode, searchThreadNumbers(second), second.Total)
		}
	}
	updated, err := c.SearchThreadsPage(ctx, "connection timeout", SearchFilter{Order: UpdatedSearchOrder(), Page: mustSearchPage(t, 10)})
	if err != nil {
		t.Fatal(err)
	}
	if got := searchThreadNumbers(updated); !slices.Equal(got, []int{2, 1}) {
		t.Fatalf("updated order = %v", got)
	}
}

func searchThreadNumbers(page ThreadSearchPage) []int {
	out := make([]int, len(page.Threads))
	for i, thread := range page.Threads {
		out[i] = thread.Number
	}
	return out
}

func seedSearchRelevance(t testing.TB, c *Corpus) {
	t.Helper()
	ctx := context.Background()
	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 102; i++ {
		title, body := "unrelated report", "ordinary discussion"
		if i == 1 {
			title, body = "connection timeout during startup", "network request stalls"
		}
		if i == 2 {
			title, body = "configuration example", "connection timeout"
		}
		thread, err := c.ApplyThreadObservation(ctx, repo.ID, domain.IssueKind, i, "open", title, body, "author", time.Unix(int64(i+1), 0), `{}`)
		if err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			if err := c.ApplyFacetObservationSet(ctx, repo.ID, &thread.ID, "issue_comments", time.Unix(200, 0), []FacetObservationInput{{SourceUpdatedAt: time.Unix(200, 0), Payload: `[]`, SearchText: strings.Repeat("unrelated discussion ", 1000)}}, true, 0); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestThreadSearchAnyTermPromotesOnlyCompleteTitleMatches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	for i, title := range []string{"alpha", "beta", "alpha beta"} {
		body := ""
		if i == 2 {
			body = strings.Repeat("discussion ", 1000)
		}
		if _, err := c.ApplyThreadObservation(ctx, repo.ID, domain.IssueKind, i+1, "open", title, body, "author", time.Unix(int64(i+2), 0), `{}`); err != nil {
			t.Fatal(err)
		}
	}
	page, err := c.SearchThreadsPage(ctx, "alpha beta", SearchFilter{TermMatch: MatchAnyTerm(), Page: mustSearchPage(t, 10)})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || len(page.Threads) != 3 || page.Threads[0].Number != 3 {
		t.Fatalf("any-term order = %v, total=%d", searchThreadNumbers(page), page.Total)
	}
}
