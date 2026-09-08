package corpus

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

func BenchmarkThreadSearchRelevance(b *testing.B) {
	ctx := context.Background()
	c, err := Open(ctx, filepath.Join(b.TempDir(), "corpus.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = c.Close() })
	seedSearchRelevance(b, c)
	page, err := ParseSearchPage(10, "")
	if err != nil {
		b.Fatal(err)
	}
	for _, query := range []string{"connection timeout", "unrelated"} {
		b.Run(query, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := c.SearchThreadsPage(ctx, query, SearchFilter{Page: page}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkThreadSearchHydratedScope(b *testing.B) {
	for _, background := range []int{0, 100, 1000} {
		b.Run(fmt.Sprintf("background_%d", background), func(b *testing.B) {
			ctx := context.Background()
			c, err := Open(ctx, filepath.Join(b.TempDir(), "corpus.db"))
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { _ = c.Close() })
			thread, scope := seedHydratedSearchScope(b, c, background)
			page, err := ParseSearchPage(10, "")
			if err != nil {
				b.Fatal(err)
			}
			b.Run("repository_page", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					result, err := c.SearchThreadsPage(ctx, "shared", SearchFilter{Repository: scope, Page: page})
					if err != nil || result.Total != 1 {
						b.Fatalf("search total=%d, error=%v", result.Total, err)
					}
				}
			})
			b.Run("exact_evidence", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					_, found, err := c.FindThreadSearchEvidence(ctx, thread.ID, "shared")
					if err != nil || !found {
						b.Fatalf("found=%v, error=%v", found, err)
					}
				}
			})
		})
	}
}

func seedHydratedSearchScope(t testing.TB, c *Corpus, background int) (*Thread, ThreadRepositoryScope) {
	t.Helper()
	ctx := context.Background()
	ref := domain.MustRepoRef("owner", "selected")
	repo, err := c.ApplyRepositoryObservation(ctx, ref.Owner(), ref.Repo(), "selected", time.Unix(1, 0), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := NewThreadRepositoryScope(ref, repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	other, err := c.ApplyRepositoryObservation(ctx, "owner", "background", "background", time.Unix(1, 0), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	var selected *Thread
	for i := 0; i <= background; i++ {
		repoID := other.ID
		if i == 0 {
			repoID = repo.ID
		}
		thread, err := c.ApplyThreadObservation(ctx, repoID, domain.IssueKind, i+1, "open", "shared report", "body", "author", time.Unix(2, 0), `{}`)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			selected = thread
		}
		text := "shared " + strings.Repeat("discussion context ", 200)
		if err := c.ApplyFacetObservationSet(ctx, repoID, &thread.ID, "issue_comments", time.Unix(3, 0), []FacetObservationInput{{Payload: `[]`, SearchText: text, SourceUpdatedAt: time.Unix(3, 0)}}, true, 0); err != nil {
			t.Fatal(err)
		}
	}
	return selected, scope
}
