package crawl

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/github"
)

func openFrontier(t *testing.T) *corpus.Corpus {
	t.Helper()
	c, err := corpus.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func enqueue(t *testing.T, c *corpus.Corpus, key string, maxAttempts int) {
	t.Helper()
	_, _, err := c.EnqueueFrontierItem(context.Background(), corpus.FrontierItem{
		WorkKey: key, SubjectKind: "repository", MaxAttempts: maxAttempts, BudgetEstimate: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWorkerCompletesRetriesAndClassifiesTerminalFailures(t *testing.T) {
	ctx := context.Background()
	c := openFrontier(t)
	for _, key := range []string{"complete", "retry", "missing", "denied", "deleted", "archived"} {
		enqueue(t, c, key, 3)
	}
	now := time.Unix(1_700_000_000, 0).UTC()
	w := &Worker{
		Frontier: c, ID: "worker", BatchSize: 6, Budget: 6,
		LeaseDuration: time.Minute, HandlerTimeout: 30 * time.Second, Backoff: time.Minute,
		Now: func() time.Time { return now },
		Handler: HandlerFunc(func(_ context.Context, item corpus.FrontierItem) error {
			switch item.WorkKey {
			case "complete":
				return nil
			case "retry":
				return &github.TransientError{Cause: errors.New("upstream unavailable")}
			case "missing":
				return &github.NotFoundError{Resource: "repository"}
			case "denied":
				return &github.AccessDeniedError{StatusCode: 403, Message: "forbidden"}
			case "deleted":
				return &github.GoneError{Resource: "repository"}
			case "archived":
				return &TerminalError{Kind: corpus.FrontierFailureArchived, Err: errors.New("repository archived")}
			default:
				return errors.New("unexpected item")
			}
		}),
	}
	stats, err := w.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats != (Stats{Leased: 6, Completed: 1, Retried: 1, Failed: 4, Budget: 6}) {
		t.Fatalf("stats = %+v", stats)
	}
	assertFrontierState(t, c, "complete", corpus.FrontierCompleted, "")
	assertFrontierState(t, c, "retry", corpus.FrontierQueued, "")
	assertFrontierState(t, c, "missing", corpus.FrontierFailed, corpus.FrontierFailureAbsent)
	assertFrontierState(t, c, "denied", corpus.FrontierFailed, corpus.FrontierFailureUnauthorized)
	assertFrontierState(t, c, "deleted", corpus.FrontierFailed, corpus.FrontierFailureDeleted)
	assertFrontierState(t, c, "archived", corpus.FrontierFailed, corpus.FrontierFailureArchived)
	retry, err := c.GetFrontierItem(ctx, "retry")
	if err != nil {
		t.Fatal(err)
	}
	if !retry.EarliestRunAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("retry at = %v", retry.EarliestRunAt)
	}
}

func TestBoundedErrorPreservesValidUTF8(t *testing.T) {
	message := strings.Repeat("x", maxErrorLength-1) + "🙂"
	got := boundedError(errors.New(message))
	if !strings.HasSuffix(got, "x") || len(got) != maxErrorLength-1 {
		t.Fatalf("bounded error has %d bytes and suffix %q", len(got), got[len(got)-1:])
	}
}

func TestWorkerExhaustsRetriesAndBoundsStoredErrors(t *testing.T) {
	c := openFrontier(t)
	enqueue(t, c, "bounded", 1)
	now := time.Unix(1_700_000_000, 0).UTC()
	w := &Worker{
		Frontier: c, Handler: HandlerFunc(func(context.Context, corpus.FrontierItem) error {
			return errors.New(strings.Repeat("x", maxErrorLength+100))
		}),
		ID: "worker", BatchSize: 1, Budget: 1, LeaseDuration: time.Minute,
		HandlerTimeout: 30 * time.Second, Now: func() time.Time { return now },
	}
	stats, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Failed != 1 || stats.Retried != 0 {
		t.Fatalf("stats = %+v", stats)
	}
	item, err := c.GetFrontierItem(context.Background(), "bounded")
	if err != nil {
		t.Fatal(err)
	}
	if item.State != corpus.FrontierFailed || item.FailureKind != corpus.FrontierFailureTransientExhausted || len(item.LastError) != maxErrorLength {
		t.Fatalf("item = %+v, error length = %d", item, len(item.LastError))
	}
}

func TestWorkerUsesRateLimitRetryAfter(t *testing.T) {
	c := openFrontier(t)
	enqueue(t, c, "limited", 2)
	now := time.Unix(1_700_000_000, 0).UTC()
	w := &Worker{
		Frontier: c, Handler: HandlerFunc(func(context.Context, corpus.FrontierItem) error {
			return &github.SecondaryRateLimitError{RetryAfter: 5 * time.Minute}
		}),
		ID: "worker", BatchSize: 1, Budget: 1, LeaseDuration: time.Minute,
		HandlerTimeout: 30 * time.Second, Now: func() time.Time { return now },
	}
	if _, err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	item, err := c.GetFrontierItem(context.Background(), "limited")
	if err != nil {
		t.Fatal(err)
	}
	if !item.EarliestRunAt.Equal(now.Add(5 * time.Minute)) {
		t.Fatalf("retry at = %v", item.EarliestRunAt)
	}
}

func TestWorkerReleasesLeasedRemainderOnCancellation(t *testing.T) {
	c := openFrontier(t)
	enqueue(t, c, "first", 3)
	enqueue(t, c, "second", 1)
	ctx, cancel := context.WithCancel(context.Background())
	now := time.Unix(1_700_000_000, 0).UTC()
	w := &Worker{
		Frontier: c, Handler: HandlerFunc(func(context.Context, corpus.FrontierItem) error {
			cancel()
			return context.Canceled
		}),
		ID: "worker", BatchSize: 2, Budget: 2, LeaseDuration: time.Minute,
		HandlerTimeout: 30 * time.Second, Now: func() time.Time { return now }, Backoff: time.Second,
	}
	stats, err := w.RunOnce(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	if stats.Leased != 2 || stats.Retried != 1 {
		t.Fatalf("stats = %+v", stats)
	}
	for _, key := range []string{"first", "second"} {
		assertFrontierState(t, c, key, corpus.FrontierQueued, "")
	}
	second, err := c.GetFrontierItem(context.Background(), "second")
	if err != nil {
		t.Fatal(err)
	}
	if second.Attempts != 0 {
		t.Fatalf("unprocessed attempt was not refunded: %+v", second)
	}
}

func TestWorkerRejectsUnsafeConfiguration(t *testing.T) {
	c := openFrontier(t)
	tests := []*Worker{
		{Frontier: c, Handler: HandlerFunc(func(context.Context, corpus.FrontierItem) error { return nil }), Budget: 1},
		{Frontier: c, Handler: HandlerFunc(func(context.Context, corpus.FrontierItem) error { return nil }), ID: "w", Budget: 1, BatchSize: maxBatchSize + 1},
		{Frontier: c, Handler: HandlerFunc(func(context.Context, corpus.FrontierItem) error { return nil }), ID: "w", Budget: 1, LeaseDuration: time.Second, HandlerTimeout: time.Second},
	}
	for _, worker := range tests {
		if _, err := worker.RunOnce(context.Background()); err == nil {
			t.Fatalf("RunOnce accepted %+v", worker)
		}
	}
}

func assertFrontierState(t *testing.T, c *corpus.Corpus, key, state, failureKind string) {
	t.Helper()
	item, err := c.GetFrontierItem(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if item == nil || item.State != state || item.FailureKind != failureKind {
		t.Fatalf("frontier %q = %+v", key, item)
	}
}
