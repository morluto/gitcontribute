package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/corpus"
)

type faultingJobStore struct {
	jobStore

	mu          sync.Mutex
	getErr      error
	failNextGet bool
	startErr    error
}

func (s *faultingJobStore) GetJob(ctx context.Context, id string) (*corpus.Job, error) {
	s.mu.Lock()
	if s.failNextGet {
		s.failNextGet = false
		err := s.getErr
		s.mu.Unlock()
		return nil, err
	}
	s.mu.Unlock()
	return s.jobStore.GetJob(ctx, id)
}

func (s *faultingJobStore) StartJobAs(ctx context.Context, id, ownerID string) error {
	if s.startErr != nil {
		return s.startErr
	}
	return s.jobStore.StartJobAs(ctx, id, ownerID)
}

func (s *faultingJobStore) failOneGet(err error) {
	s.mu.Lock()
	s.getErr = err
	s.failNextGet = true
	s.mu.Unlock()
}

func newJobTestService(t *testing.T) *Service {
	t.Helper()
	ctx := context.Background()
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test", nil)
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

func waitForCorpusJobStatus(t *testing.T, c *corpus.Corpus, id, want string, timeout time.Duration) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		job, err := c.GetJob(ctx, id)
		if err != nil {
			t.Fatalf("get job: %v", err)
		}
		if job != nil && job.Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job did not reach status %q within %s", want, timeout)
}

func TestSubmitAndCompleteJob(t *testing.T) {
	t.Parallel()
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

func TestJobExecutorBoundsConcurrentJobs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newJobTestService(t)
	jobs := newJobExecutorOnService(t, svc, jobExecutorConfig{pollInterval: time.Hour, maxConcurrentJobs: 1})

	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstID, err := jobs.Submit(ctx, "first", nil, func(context.Context, func(string, string) error) (any, error) {
		close(firstStarted)
		<-releaseFirst
		return map[string]bool{"ok": true}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-firstStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first job did not start")
	}

	secondStarted := make(chan struct{})
	secondID, err := jobs.Submit(ctx, "second", nil, func(context.Context, func(string, string) error) (any, error) {
		close(secondStarted)
		return map[string]bool{"ok": true}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-secondStarted:
		t.Fatal("second job started above configured concurrency")
	case <-time.After(100 * time.Millisecond):
	}
	second, err := jobs.Get(ctx, secondID)
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != corpus.JobStatusQueued {
		t.Fatalf("waiting job status = %q, want queued", second.Status)
	}

	close(releaseFirst)
	waitForJobStatus(t, jobs, firstID, corpus.JobStatusSucceeded, 2*time.Second)
	select {
	case <-secondStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("second job did not start after slot release")
	}
	waitForJobStatus(t, jobs, secondID, corpus.JobStatusSucceeded, 2*time.Second)
}

func TestJobExecutorRecordsReadErrorAfterExecution(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newJobTestService(t)
	store := &faultingJobStore{jobStore: svc.corpus}
	jobs, err := newJobExecutorWithConfig(ctx, store, jobExecutorConfig{pollInterval: time.Hour})
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}
	svc.jobs = jobs

	id, err := jobs.Submit(ctx, "read-failure", nil, func(context.Context, func(string, string) error) (any, error) {
		store.failOneGet(errors.New("injected read failure"))
		return "done", nil
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	waitForCorpusJobStatus(t, svc.corpus, id, corpus.JobStatusFailed, 2*time.Second)
	job, err := svc.corpus.GetJob(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !strings.Contains(job.Error, "get job after execution: injected read failure") {
		t.Fatalf("error = %q, want read failure", job.Error)
	}
}

func TestJobExecutorRecordsReadErrorAfterStartFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newJobTestService(t)
	store := &faultingJobStore{
		jobStore: svc.corpus,
		startErr: errors.New("injected start failure"),
	}
	store.failOneGet(errors.New("injected read failure"))
	jobs, err := newJobExecutorWithConfig(ctx, store, jobExecutorConfig{pollInterval: time.Hour})
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}
	svc.jobs = jobs

	id, err := jobs.Submit(ctx, "start-failure", nil, func(context.Context, func(string, string) error) (any, error) {
		t.Fatal("job function ran after start failure")
		return struct{}{}, nil
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	waitForCorpusJobStatus(t, svc.corpus, id, corpus.JobStatusFailed, 2*time.Second)
	job, err := svc.corpus.GetJob(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !strings.Contains(job.Error, "injected start failure") || !strings.Contains(job.Error, "get job after start failure: injected read failure") {
		t.Fatalf("error = %q, want start and read failures", job.Error)
	}
}

func TestJobCancellation(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

func TestJobExecutorBoundsRunningAndPendingJobs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newJobTestService(t)
	jobs := newJobExecutorOnService(t, svc, jobExecutorConfig{
		heartbeatInterval: time.Hour,
		pollInterval:      time.Hour,
		maxConcurrentJobs: 2,
		maxAdmittedJobs:   3,
	})

	release := make(chan struct{})
	started := make(chan struct{}, 2)
	block := func(jobCtx context.Context, _ func(string, string) error) (any, error) {
		started <- struct{}{}
		select {
		case <-release:
			return "done", nil
		case <-jobCtx.Done():
			return nil, jobCtx.Err()
		}
	}

	first, err := jobs.Submit(ctx, "first", nil, block)
	if err != nil {
		t.Fatalf("submit first: %v", err)
	}
	<-started
	second, err := jobs.Submit(ctx, "second", nil, block)
	if err != nil {
		t.Fatalf("submit second: %v", err)
	}
	<-started

	queuedRan := make(chan struct{}, 1)
	third, err := jobs.Submit(ctx, "third", nil, func(context.Context, func(string, string) error) (any, error) {
		queuedRan <- struct{}{}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("submit third: %v", err)
	}
	waitForJobStatus(t, jobs, third, corpus.JobStatusQueued, time.Second)

	if _, err := jobs.Submit(ctx, "overflow", nil, block); !errors.Is(err, ErrJobQueueFull) {
		t.Fatalf("overflow submit error = %v, want %v", err, ErrJobQueueFull)
	}
	if err := jobs.Cancel(ctx, third); err != nil {
		t.Fatalf("cancel queued job: %v", err)
	}
	waitForJobStatus(t, jobs, third, corpus.JobStatusCancelled, time.Second)

	deadline := time.Now().Add(time.Second)
	for {
		jobs.mu.Lock()
		pending := jobs.admittedCount
		jobs.mu.Unlock()
		if pending == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("cancelled queued job still consumes admission: %d jobs", pending)
		}
		time.Sleep(10 * time.Millisecond)
	}
	replacement, err := jobs.Submit(ctx, "replacement", nil, block)
	if err != nil {
		t.Fatalf("submit after queued cancellation: %v", err)
	}

	close(release)
	waitForJobStatus(t, jobs, first, corpus.JobStatusSucceeded, time.Second)
	waitForJobStatus(t, jobs, second, corpus.JobStatusSucceeded, time.Second)
	waitForJobStatus(t, jobs, replacement, corpus.JobStatusSucceeded, time.Second)

	deadline = time.Now().Add(time.Second)
	for {
		jobs.mu.Lock()
		pending := jobs.admittedCount
		jobs.mu.Unlock()
		if pending == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("executor still tracks %d jobs", pending)
		}
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case <-queuedRan:
		t.Fatal("cancelled queued job function ran")
	default:
	}
}

func TestConcurrentReadWhileJobRunning(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

func TestRemoteCancellationReleasesQueuedAdmission(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newJobTestService(t)
	jobs := newJobExecutorOnService(t, svc, jobExecutorConfig{
		pollInterval:      20 * time.Millisecond,
		maxConcurrentJobs: 1,
		maxAdmittedJobs:   2,
	})

	release := make(chan struct{})
	started := make(chan struct{}, 2)
	block := func(jobCtx context.Context, _ func(string, string) error) (any, error) {
		started <- struct{}{}
		select {
		case <-release:
			return "done", nil
		case <-jobCtx.Done():
			return nil, jobCtx.Err()
		}
	}
	first, err := jobs.Submit(ctx, "first", nil, block)
	if err != nil {
		t.Fatalf("submit first: %v", err)
	}
	<-started

	queuedRan := make(chan struct{}, 1)
	queued, err := jobs.Submit(ctx, "queued", nil, func(context.Context, func(string, string) error) (any, error) {
		queuedRan <- struct{}{}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("submit queued: %v", err)
	}

	c2, err := corpus.Open(ctx, svc.databasePath())
	if err != nil {
		t.Fatalf("open second corpus: %v", err)
	}
	defer func() { _ = c2.Close() }()
	if err := c2.RequestJobCancellation(ctx, queued); err != nil {
		t.Fatalf("remote cancel: %v", err)
	}
	waitForJobStatus(t, jobs, queued, corpus.JobStatusCancelled, time.Second)

	deadline := time.Now().Add(time.Second)
	for {
		jobs.mu.Lock()
		admitted := jobs.admittedCount
		jobs.mu.Unlock()
		if admitted == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("remote-cancelled queued job still consumes admission: %d jobs", admitted)
		}
		time.Sleep(10 * time.Millisecond)
	}

	replacement, err := jobs.Submit(ctx, "replacement", nil, block)
	if err != nil {
		t.Fatalf("submit replacement: %v", err)
	}
	close(release)
	waitForJobStatus(t, jobs, first, corpus.JobStatusSucceeded, time.Second)
	waitForJobStatus(t, jobs, replacement, corpus.JobStatusSucceeded, time.Second)
	select {
	case <-queuedRan:
		t.Fatal("remote-cancelled queued job function ran")
	default:
	}
}
