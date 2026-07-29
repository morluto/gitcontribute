package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/corpus"
)

type delayedHeartbeatStore struct {
	jobStore
	calls        chan time.Time
	releaseFirst chan struct{}
	once         sync.Once
}

func (s *delayedHeartbeatStore) HeartbeatJobOwner(_ context.Context, _ string, _ time.Time) error {
	s.calls <- time.Now()
	s.once.Do(func() {
		<-s.releaseFirst
	})
	return nil
}

func TestJobExecutorCloseCancelsAndWaits(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newJobTestService(t)
	jobs, err := svc.Jobs(ctx)
	if err != nil {
		t.Fatalf("jobs: %v", err)
	}

	started := make(chan struct{})
	id, err := jobs.Submit(ctx, "block", nil, func(ctx context.Context, report func(progress, statistics string) error) (any, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	<-started

	done := make(chan struct{})
	go func() {
		_ = svc.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not finish in time")
	}

	c, err := corpus.Open(ctx, svc.databasePath())
	if err != nil {
		t.Fatalf("reopen corpus after close: %v", err)
	}
	defer func() { _ = c.Close() }()
	job, err := c.GetJob(ctx, id)
	if err != nil {
		t.Fatalf("get after close: %v", err)
	}
	if job.Status != corpus.JobStatusCancelled {
		t.Fatalf("status = %q, want %q", job.Status, corpus.JobStatusCancelled)
	}
}

func TestJobExecutorUsesLifecycleContext(t *testing.T) {
	t.Parallel()
	lifecycle, cancelLifecycle := context.WithCancel(context.Background())
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := NewWithContext(lifecycle, paths, "test", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	if _, err := svc.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	jobs, err := svc.Jobs(context.Background())
	if err != nil {
		t.Fatalf("jobs: %v", err)
	}

	started := make(chan struct{})
	id, err := jobs.Submit(context.Background(), "lifecycle", nil, func(ctx context.Context, _ func(string, string) error) (any, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	<-started
	cancelLifecycle()

	waitForJobStatus(t, jobs, id, corpus.JobStatusCancelled, 2*time.Second)
}

func TestJobExecutorHeartbeatWaitsAfterSlowAttempt(t *testing.T) {
	t.Parallel()
	const interval = 100 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	store := &delayedHeartbeatStore{
		calls:        make(chan time.Time, 2),
		releaseFirst: make(chan struct{}),
	}
	executor := &JobExecutor{
		corpus:  store,
		ownerID: "owner",
		cfg: jobExecutorConfig{
			heartbeatInterval: interval,
		},
		rootCtx: ctx,
	}
	executor.backgroundWG.Add(1)
	go executor.heartbeat()
	t.Cleanup(func() {
		cancel()
		executor.backgroundWG.Wait()
	})

	select {
	case <-store.calls:
	case <-time.After(5 * interval):
		t.Fatal("first heartbeat did not start")
	}

	// Keep the first attempt blocked past several nominal ticks. A fixed-rate
	// ticker would leave a catch-up tick waiting and issue the next write as
	// soon as this attempt returns.
	time.Sleep(3 * interval)
	close(store.releaseFirst)

	select {
	case <-store.calls:
		t.Fatal("second heartbeat started without waiting after the slow attempt")
	case <-time.After(interval / 2):
	}

	select {
	case <-store.calls:
	case <-time.After(5 * interval):
		t.Fatal("second heartbeat did not start after the interval")
	}
}
