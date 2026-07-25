package app

import (
	"context"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/corpus"
)

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
