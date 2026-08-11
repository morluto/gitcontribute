package corpus

import (
	"context"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

func TestRunCompletionAndStats(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	run, err := c.StartRun(ctx, "sync")
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if err := c.FinishRun(ctx, run.ID, `{"pages":3,"items":42}`); err != nil {
		t.Fatalf("finish run: %v", err)
	}

	run, err = c.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.State.Status() != RunStatusCompleted {
		t.Fatalf("run status = %q, want completed", run.State.Status())
	}
	if run.Stats != `{"pages":3,"items":42}` {
		t.Fatalf("run stats = %q", run.Stats)
	}
	if _, ok := run.State.CompletedAt(); !ok {
		t.Fatal("run completed_at is nil")
	}
}

func TestProjectionIgnoresStaleThreadObservationsBySourceUpdatedAt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "1", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("apply repository: %v", err)
	}

	newer := time.Unix(2000, 0).UTC()
	older := time.Unix(1000, 0).UTC()

	// Apply observations out of chronological order.
	if _, err := c.ApplyThreadObservation(ctx, repo.ID, domain.IssueKind, 1, "open", "new", "b", "a", newer, `{}`); err != nil {
		t.Fatalf("apply newer: %v", err)
	}
	if _, err := c.ApplyThreadObservation(ctx, repo.ID, domain.IssueKind, 1, "open", "old", "b", "a", older, `{}`); err != nil {
		t.Fatalf("apply older: %v", err)
	}

	thread, err := c.GetThread(ctx, repo.ID, domain.IssueKind, 1)
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if thread.Title != "new" {
		t.Fatalf("title = %q, want new", thread.Title)
	}
	if !thread.SourceUpdatedAt.Equal(newer) {
		t.Fatalf("source_updated_at = %v, want %v", thread.SourceUpdatedAt, newer)
	}
}
