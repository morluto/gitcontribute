package corpus

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

func TestThreadSearchPagesPreserveExactEvidenceInReadOnlyCorpus(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, path := openTestCorpus(t)
	selected, scope := seedHydratedSearchScope(t, c, 5)
	for i := 1; i <= 12; i++ {
		title, body, facet := "shared title", "body", "discussion"
		switch i % 3 {
		case 1:
			title, body = "report", "shared body"
		case 2:
			title, facet = "report", "shared comment"
		}
		thread, err := c.ApplyThreadObservation(ctx, selected.RepositoryID, domain.IssueKind, i+1, "open", title, body, "author", time.Unix(int64(i+10), 0), `{}`)
		if err != nil {
			t.Fatal(err)
		}
		if err := c.ApplyFacetObservationSet(ctx, thread.RepositoryID, &thread.ID, "issue_comments", time.Unix(40, 0), []FacetObservationInput{{Payload: `[]`, SearchText: facet, SourceUpdatedAt: time.Unix(40, 0)}}, true, 0); err != nil {
			t.Fatal(err)
		}
	}
	reader, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	revision, err := reader.CorpusRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, order := range []SearchOrder{RelevanceSearchOrder(), UpdatedSearchOrder()} {
		filter := SearchFilter{Repository: scope, Order: order, Page: mustSearchPage(t, 100)}
		whole, err := reader.SearchThreadsPage(ctx, "shared", filter)
		if err != nil || whole.Total != 13 || len(whole.Threads) != 13 {
			t.Fatalf("whole total=%d, count=%d, err=%v", whole.Total, len(whole.Threads), err)
		}
		cursor, seen := "", 0
		for {
			filter.Page = mustSearchPage(t, 3, cursor)
			page, err := reader.SearchThreadsPage(ctx, "shared", filter)
			if err != nil {
				t.Fatal(err)
			}
			if page.Total != whole.Total || len(page.Threads) == 0 {
				t.Fatalf("page total=%d, count=%d", page.Total, len(page.Threads))
			}
			for _, thread := range page.Threads {
				if seen >= len(whole.Threads) || thread.ID != whole.Threads[seen].ID || thread.Rank != whole.Threads[seen].Rank {
					t.Fatalf("page differs at position %d: thread=%d rank=%g", seen, thread.ID, thread.Rank)
				}
				evidence, found, err := reader.FindThreadSearchEvidence(ctx, thread.ID, "shared")
				if err != nil || !found {
					t.Fatalf("exact evidence found=%v, error=%v", found, err)
				}
				if evidence.Rank != thread.Rank || evidence.Source != thread.MatchSource || evidence.Excerpt != thread.MatchExcerpt || !evidence.SourceUpdatedAt.Equal(thread.MatchUpdatedAt) || evidence.Truncated != thread.MatchTruncated {
					t.Fatalf("page and exact evidence differ: thread=%+v evidence=%+v", thread, evidence)
				}
				seen++
			}
			cursor = page.NextCursor
			if cursor == "" {
				break
			}
		}
		if seen != 13 {
			t.Fatalf("visited %d threads", seen)
		}
	}
	if err := reader.RequireCorpusRevision(ctx, revision); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := reader.SearchThreadsPage(cancelled, "shared", SearchFilter{Repository: scope}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled search: %v", err)
	}
	if _, _, err := reader.FindThreadSearchEvidence(cancelled, selected.ID, "shared"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled evidence lookup: %v", err)
	}
}
