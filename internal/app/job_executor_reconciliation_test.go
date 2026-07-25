package app

import (
	"context"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/corpus"
)

func TestStartupReconciliation(t *testing.T) {
	t.Parallel()
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
	svc, err := New(paths, "test", nil)
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

func TestLiveOwnerNotReconciledByAnotherExecutor(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

	// Make the owner stale explicitly instead of waiting for wall-clock time.
	// The executor's one-hour heartbeat interval keeps the fixture stable.
	if err := svc.corpus.RegisterJobOwner(ctx, jobsA.ownerID, 1, time.Now().UTC().Add(-time.Hour)); err != nil {
		t.Fatalf("age job owner: %v", err)
	}

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
	t.Parallel()
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
	svc, err := New(paths, "test", nil)
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
	t.Parallel()
	ctx := context.Background()
	svc := newJobTestService(t)
	const (
		heartbeatInterval = 20 * time.Millisecond
		leaseTimeout      = 500 * time.Millisecond
	)
	jobsA := newJobExecutorOnService(t, svc, jobExecutorConfig{
		heartbeatInterval: heartbeatInterval,
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

	// Reconcile for longer than the lease so the test still fails if the owner
	// never heartbeats. The wider lease avoids treating ordinary scheduler and
	// coverage-instrumentation stalls as an abandoned process.
	deadline := time.Now().Add(2 * leaseTimeout)
	for i := 0; time.Now().Before(deadline); i++ {
		if err := cB.ReconcileInterruptedJobs(ctx, leaseTimeout); err != nil {
			t.Fatalf("reconcile iteration %d: %v", i, err)
		}
		time.Sleep(heartbeatInterval)
	}

	job, err := jobsA.Get(ctx, id)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if job.Status != corpus.JobStatusRunning {
		t.Fatalf("live job was reconciled during concurrent heartbeat: status=%q", job.Status)
	}
}
