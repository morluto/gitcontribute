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

func newJobExecutorOnService(t *testing.T, svc *Service, cfg jobExecutorConfig) *JobExecutor {
	t.Helper()
	jobs, err := newJobExecutorWithConfig(context.Background(), svc.corpus, cfg)
	if err != nil {
		t.Fatalf("new job executor: %v", err)
	}
	svc.jobs = jobs
	return jobs
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

func TestRemoteCancellationAcrossExecutors(t *testing.T) {
	ctx := context.Background()
	svc := newJobTestService(t)
	jobs := newJobExecutorOnService(t, svc, jobExecutorConfig{
		pollInterval: 50 * time.Millisecond,
	})

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

	// Simulate a second process by opening a separate connection to the same
	// database and requesting cancellation there.
	c2, err := corpus.Open(ctx, svc.databasePath())
	if err != nil {
		t.Fatalf("open second corpus: %v", err)
	}
	defer func() { _ = c2.Close() }()

	if err := c2.RequestJobCancellation(ctx, id); err != nil {
		t.Fatalf("remote cancel: %v", err)
	}

	waitForJobStatus(t, jobs, id, corpus.JobStatusCancelled, 2*time.Second)
}

func TestLiveOwnerNotReconciledByAnotherExecutor(t *testing.T) {
	ctx := context.Background()
	svc := newJobTestService(t)
	jobsA := newJobExecutorOnService(t, svc, jobExecutorConfig{
		heartbeatInterval: 50 * time.Millisecond,
		pollInterval:      50 * time.Millisecond,
	})

	blocked := make(chan struct{})
	id, err := jobsA.Submit(ctx, "block", nil, func(ctx context.Context, report func(progress, statistics string) error) (any, error) {
		close(blocked)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	<-blocked
	waitForJobStatus(t, jobsA, id, corpus.JobStatusRunning, 1*time.Second)

	// A second process opens the database and reconciles with a 200ms lease.
	// Because A heartbeats every 50ms, its job must remain running.
	cB, err := corpus.Open(ctx, svc.databasePath())
	if err != nil {
		t.Fatalf("open second corpus: %v", err)
	}
	defer func() { _ = cB.Close() }()

	time.Sleep(100 * time.Millisecond)
	if err := cB.ReconcileInterruptedJobs(ctx, 200*time.Millisecond); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	job, err := jobsA.Get(ctx, id)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if job.Status != corpus.JobStatusRunning {
		t.Fatalf("live job was reconciled: status=%q", job.Status)
	}
}

func TestAbandonedOwnerReconciledByNewExecutor(t *testing.T) {
	ctx := context.Background()
	svc := newJobTestService(t)
	// A never heartbeats after its initial registration, so it will be
	// considered abandoned by a second process with a short lease.
	jobsA := newJobExecutorOnService(t, svc, jobExecutorConfig{
		leaseTimeout:      1 * time.Hour,
		heartbeatInterval: 1 * time.Hour,
		pollInterval:      50 * time.Millisecond,
	})

	blocked := make(chan struct{})
	id, err := jobsA.Submit(ctx, "block", nil, func(ctx context.Context, report func(progress, statistics string) error) (any, error) {
		close(blocked)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	<-blocked
	waitForJobStatus(t, jobsA, id, corpus.JobStatusRunning, 1*time.Second)

	// After the short lease expires, a second process opens the database and
	// reconciles abandoned owners.
	time.Sleep(300 * time.Millisecond)

	cB, err := corpus.Open(ctx, svc.databasePath())
	if err != nil {
		t.Fatalf("open second corpus: %v", err)
	}
	defer func() { _ = cB.Close() }()

	if err := cB.ReconcileInterruptedJobs(ctx, 200*time.Millisecond); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	job, err := cB.GetJob(ctx, id)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if job.Status != corpus.JobStatusFailed {
		t.Fatalf("abandoned job status = %q, want %q", job.Status, corpus.JobStatusFailed)
	}
	if job.Error != "interrupted by restart" {
		t.Fatalf("abandoned job error = %q", job.Error)
	}
}

func TestReadOnlyCorpusOpenDoesNotReconcileJobs(t *testing.T) {
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

	c, err := corpus.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	job, err := c.CreateJob(ctx, "sync", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	ownerID := "stale-owner"
	if err := c.RegisterJobOwner(ctx, ownerID, 1, time.Now().UTC().Add(-time.Hour)); err != nil {
		t.Fatalf("register owner: %v", err)
	}
	if err := c.StartJobAs(ctx, job.ID, ownerID); err != nil {
		t.Fatalf("start job: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("close corpus: %v", err)
	}

	// A read-only service operation must open the corpus without creating a
	// job executor and without reconciling running jobs.
	svc, err := New(paths, "test")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer func() { _ = svc.Close() }()

	if _, err := svc.Status(ctx); err != nil {
		t.Fatalf("status: %v", err)
	}

	c2, err := corpus.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("reopen corpus: %v", err)
	}
	defer func() { _ = c2.Close() }()

	j, err := c2.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if j.Status != corpus.JobStatusRunning {
		t.Fatalf("read-only open reconciled job: status=%q", j.Status)
	}
}

func TestReconcileConcurrentWithHeartbeat(t *testing.T) {
	ctx := context.Background()
	svc := newJobTestService(t)
	jobsA := newJobExecutorOnService(t, svc, jobExecutorConfig{
		heartbeatInterval: 20 * time.Millisecond,
		pollInterval:      50 * time.Millisecond,
	})

	blocked := make(chan struct{})
	id, err := jobsA.Submit(ctx, "block", nil, func(ctx context.Context, report func(progress, statistics string) error) (any, error) {
		close(blocked)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	<-blocked
	waitForJobStatus(t, jobsA, id, corpus.JobStatusRunning, 1*time.Second)

	cB, err := corpus.Open(ctx, svc.databasePath())
	if err != nil {
		t.Fatalf("open second corpus: %v", err)
	}
	defer func() { _ = cB.Close() }()

	for i := range 20 {
		if err := cB.ReconcileInterruptedJobs(ctx, 100*time.Millisecond); err != nil {
			t.Fatalf("reconcile iteration %d: %v", i, err)
		}
		time.Sleep(25 * time.Millisecond)
	}

	job, err := jobsA.Get(ctx, id)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if job.Status != corpus.JobStatusRunning {
		t.Fatalf("live job was reconciled during concurrent heartbeat: status=%q", job.Status)
	}
}
