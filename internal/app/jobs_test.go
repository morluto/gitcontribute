package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/corpus"
)

func newJobTestService(t *testing.T) *Service {
	t.Helper()
	ctx := context.Background()
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	if _, err := svc.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}
	return svc
}

func waitForJobStatus(t *testing.T, jobs *JobExecutor, id, want string, timeout time.Duration) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		job, err := jobs.Get(ctx, id)
		if err != nil {
			t.Fatalf("get job: %v", err)
		}
		if job == nil {
			t.Fatal("job not found")
		}
		if job.Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job did not reach status %q within %s", want, timeout)
}

func TestSubmitAndCompleteJob(t *testing.T) {
	ctx := context.Background()
	svc := newJobTestService(t)
	jobs, err := svc.Jobs(ctx)
	if err != nil {
		t.Fatalf("jobs: %v", err)
	}

	id, err := jobs.Submit(ctx, "echo", map[string]any{"value": 42}, func(ctx context.Context, report func(progress, statistics string) error) (any, error) {
		if err := report("working", `{"step":1}`); err != nil {
			return nil, err
		}
		return map[string]any{"value": 42}, nil
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	waitForJobStatus(t, jobs, id, corpus.JobStatusSucceeded, 2*time.Second)

	job, err := jobs.Get(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if job.Result != `{"value":42}` {
		t.Fatalf("result = %q, want %q", job.Result, `{"value":42}`)
	}
	if job.Progress != "working" {
		t.Fatalf("progress = %q", job.Progress)
	}
	if job.Statistics != `{"step":1}` {
		t.Fatalf("statistics = %q", job.Statistics)
	}
}

func TestJobCancellation(t *testing.T) {
	ctx := context.Background()
	svc := newJobTestService(t)
	jobs, err := svc.Jobs(ctx)
	if err != nil {
		t.Fatalf("jobs: %v", err)
	}

	blocked := make(chan struct{})
	id, err := jobs.Submit(ctx, "block", nil, func(ctx context.Context, report func(progress, statistics string) error) (any, error) {
		close(blocked)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	<-blocked
	if err := jobs.Cancel(ctx, id); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	waitForJobStatus(t, jobs, id, corpus.JobStatusCancelled, 2*time.Second)

	job, err := jobs.Get(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if job.Status != corpus.JobStatusCancelled {
		t.Fatalf("status = %q, want %q", job.Status, corpus.JobStatusCancelled)
	}
	if job.Error == "" {
		t.Fatal("cancelled job has no error message")
	}
}

func TestCancelQueuedJob(t *testing.T) {
	ctx := context.Background()
	svc := newJobTestService(t)
	jobs, err := svc.Jobs(ctx)
	if err != nil {
		t.Fatalf("jobs: %v", err)
	}

	// Delayed function that will never be started before cancel.
	id, err := jobs.Submit(ctx, "never", nil, func(ctx context.Context, report func(progress, statistics string) error) (any, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			return nil, errors.New("should not run")
		}
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	if err := jobs.Cancel(ctx, id); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	waitForJobStatus(t, jobs, id, corpus.JobStatusCancelled, 2*time.Second)
}

func TestJobExecutorCloseCancelsAndWaits(t *testing.T) {
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

func TestStartupReconciliation(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	paths := config.NewPaths(&config.Env{Home: dir})
	dbPath, err := paths.DatabasePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureDatabaseDir(dbPath); err != nil {
		t.Fatalf("ensure db dir: %v", err)
	}

	// Simulate an interrupted run by creating a corpus directly, inserting
	// a running job, and closing it without completing.
	c, err := corpus.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	job, err := c.CreateJob(ctx, "sync", `{}`)
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if err := c.StartJob(ctx, job.ID); err != nil {
		t.Fatalf("start job: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("close corpus: %v", err)
	}

	// Opening the same database through the service must reconcile the
	// interrupted job.
	svc, err := New(paths, "test")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer func() { _ = svc.Close() }()
	jobs, err := svc.Jobs(ctx)
	if err != nil {
		t.Fatalf("jobs: %v", err)
	}

	reconciled, err := jobs.Get(ctx, job.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if reconciled == nil {
		t.Fatal("reconciled job not found")
	}
	if reconciled.Status != corpus.JobStatusFailed {
		t.Fatalf("status = %q, want %q", reconciled.Status, corpus.JobStatusFailed)
	}
	if reconciled.Error != "interrupted by restart" {
		t.Fatalf("error = %q, want %q", reconciled.Error, "interrupted by restart")
	}
}

func TestConcurrentReadWhileJobRunning(t *testing.T) {
	ctx := context.Background()
	svc := newJobTestService(t)
	jobs, err := svc.Jobs(ctx)
	if err != nil {
		t.Fatalf("jobs: %v", err)
	}

	blocked := make(chan struct{})
	id, err := jobs.Submit(ctx, "block", nil, func(ctx context.Context, report func(progress, statistics string) error) (any, error) {
		close(blocked)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	<-blocked

	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			j, err := jobs.Get(ctx, id)
			if err != nil {
				errs <- err
				return
			}
			if j == nil {
				errs <- fmt.Errorf("job not found")
				return
			}
			if _, err := jobs.List(ctx, corpus.JobStatusRunning, 100); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent read error: %v", err)
	}

	if err := jobs.Cancel(ctx, id); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	waitForJobStatus(t, jobs, id, corpus.JobStatusCancelled, 2*time.Second)
}
