package corpus

import (
	"context"
	"testing"
	"time"
)

func TestFrontierDeduplicatesReplay(t *testing.T) {
	c, _ := openTestCorpus(t)
	ctx := context.Background()
	item := FrontierItem{
		WorkKey: "repository:owner/repo:metadata", SubjectKind: "repository",
		Owner: "owner", Repo: "repo", Facet: "metadata", Priority: 10,
		Reason: "search", Source: "github-search", MaxAttempts: 4,
	}
	first, inserted, err := c.EnqueueFrontierItem(ctx, item)
	if err != nil || !inserted {
		t.Fatalf("first enqueue = (%+v, %v, %v), want insertion", first, inserted, err)
	}
	item.Priority = 999
	second, inserted, err := c.EnqueueFrontierItem(ctx, item)
	if err != nil || inserted {
		t.Fatalf("replay enqueue = (%+v, %v, %v), want existing", second, inserted, err)
	}
	if second.ID != first.ID || second.Priority != 10 {
		t.Fatalf("replay changed item: first=%+v second=%+v", first, second)
	}
}

func TestFrontierLeaseHonorsPriorityReadinessAndBudget(t *testing.T) {
	c, _ := openTestCorpus(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 123).UTC()
	for _, item := range []FrontierItem{
		{WorkKey: "low", SubjectKind: "repository", Priority: 1, BudgetEstimate: 1},
		{WorkKey: "high", SubjectKind: "repository", Priority: 10, BudgetEstimate: 2},
		{WorkKey: "future", SubjectKind: "repository", Priority: 100, EarliestRunAt: now.Add(time.Hour)},
	} {
		if _, _, err := c.EnqueueFrontierItem(ctx, item); err != nil {
			t.Fatalf("enqueue %s: %v", item.WorkKey, err)
		}
	}
	leased, err := c.LeaseFrontierItems(ctx, "worker-a", now, time.Minute, 10, 2)
	if err != nil {
		t.Fatalf("lease: %v", err)
	}
	if len(leased) != 1 || leased[0].WorkKey != "high" || leased[0].Attempts != 1 {
		t.Fatalf("leased = %+v, want only high", leased)
	}
}

func TestFrontierExpiredLeaseCanBeReclaimed(t *testing.T) {
	c, _ := openTestCorpus(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()
	if _, _, err := c.EnqueueFrontierItem(ctx, FrontierItem{WorkKey: "work", SubjectKind: "facet"}); err != nil {
		t.Fatal(err)
	}
	first, err := c.LeaseFrontierItems(ctx, "worker-a", now, time.Minute, 1, 10)
	if err != nil || len(first) != 1 {
		t.Fatalf("first lease = (%+v, %v)", first, err)
	}
	beforeExpiry, err := c.LeaseFrontierItems(ctx, "worker-b", now.Add(30*time.Second), time.Minute, 1, 10)
	if err != nil || len(beforeExpiry) != 0 {
		t.Fatalf("before expiry = (%+v, %v), want none", beforeExpiry, err)
	}
	afterExpiry, err := c.LeaseFrontierItems(ctx, "worker-b", now.Add(2*time.Minute), time.Minute, 1, 10)
	if err != nil || len(afterExpiry) != 1 || afterExpiry[0].Attempts != 2 {
		t.Fatalf("after expiry = (%+v, %v), want reclaimed attempt 2", afterExpiry, err)
	}
	if err := c.CompleteFrontierItem(ctx, afterExpiry[0].ID, "worker-a", now.Add(3*time.Minute)); err == nil {
		t.Fatal("stale worker completed reclaimed lease")
	}
	if err := c.CompleteFrontierItem(ctx, afterExpiry[0].ID, "worker-b", now.Add(3*time.Minute)); err != nil {
		t.Fatalf("complete current lease: %v", err)
	}
}

func TestFrontierReleaseRefundsUnstartedAttempt(t *testing.T) {
	c, _ := openTestCorpus(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()
	if _, _, err := c.EnqueueFrontierItem(ctx, FrontierItem{
		WorkKey: "unstarted", SubjectKind: "repository", MaxAttempts: 1,
	}); err != nil {
		t.Fatal(err)
	}
	leased, err := c.LeaseFrontierItems(ctx, "worker-a", now, time.Minute, 1, 1)
	if err != nil || len(leased) != 1 || leased[0].Attempts != 1 {
		t.Fatalf("lease = (%+v, %v)", leased, err)
	}
	if err := c.ReleaseFrontierItem(ctx, leased[0].ID, "worker-b", now); err == nil {
		t.Fatal("non-owner released frontier item")
	}
	if err := c.ReleaseFrontierItem(ctx, leased[0].ID, "worker-a", now); err != nil {
		t.Fatal(err)
	}
	item, err := c.GetFrontierItem(ctx, "unstarted")
	if err != nil {
		t.Fatal(err)
	}
	if item.State != FrontierQueued || item.Attempts != 0 {
		t.Fatalf("released item = %+v", item)
	}
	again, err := c.LeaseFrontierItems(ctx, "worker-b", now, time.Minute, 1, 1)
	if err != nil || len(again) != 1 || again[0].Attempts != 1 {
		t.Fatalf("second lease = (%+v, %v)", again, err)
	}
}

func TestFrontierRetryIsBounded(t *testing.T) {
	c, _ := openTestCorpus(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()
	if _, _, err := c.EnqueueFrontierItem(ctx, FrontierItem{
		WorkKey: "bounded", SubjectKind: "thread", MaxAttempts: 2,
	}); err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		leased, err := c.LeaseFrontierItems(ctx, "worker", now, time.Minute, 1, 10)
		if err != nil || len(leased) != 1 {
			t.Fatalf("lease attempt %d = (%+v, %v)", attempt, leased, err)
		}
		if err := c.RetryFrontierItem(ctx, leased[0].ID, "worker", "temporary", now, now); err != nil {
			t.Fatalf("retry attempt %d: %v", attempt, err)
		}
	}
	item, err := c.GetFrontierItem(ctx, "bounded")
	if err != nil {
		t.Fatal(err)
	}
	if item.State != FrontierFailed || item.Attempts != 2 || item.FailureKind != FrontierFailureTransientExhausted || item.LastError != "temporary" {
		t.Fatalf("bounded item = %+v", item)
	}
	leasing, err := c.LeaseFrontierItems(ctx, "worker", now, time.Minute, 1, 10)
	if err != nil || len(leasing) != 0 {
		t.Fatalf("terminal item leased: (%+v, %v)", leasing, err)
	}
}

func TestFrontierExpiredFinalAttemptBecomesTerminal(t *testing.T) {
	c, _ := openTestCorpus(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()
	if _, _, err := c.EnqueueFrontierItem(ctx, FrontierItem{
		WorkKey: "crashed", SubjectKind: "repository", MaxAttempts: 1,
	}); err != nil {
		t.Fatal(err)
	}
	leasing, err := c.LeaseFrontierItems(ctx, "crashing-worker", now, time.Minute, 1, 10)
	if err != nil || len(leasing) != 1 {
		t.Fatalf("lease = (%+v, %v)", leasing, err)
	}
	leasing, err = c.LeaseFrontierItems(ctx, "replacement", now.Add(2*time.Minute), time.Minute, 1, 10)
	if err != nil || len(leasing) != 0 {
		t.Fatalf("re-lease exhausted item = (%+v, %v)", leasing, err)
	}
	item, err := c.GetFrontierItem(ctx, "crashed")
	if err != nil {
		t.Fatal(err)
	}
	if item.State != FrontierFailed || item.FailureKind != FrontierFailureTransientExhausted {
		t.Fatalf("expired item = %+v", item)
	}
}
