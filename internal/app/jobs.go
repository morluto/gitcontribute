package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
)

// JobFunc performs asynchronous work for a job. It receives a context that is
// cancelled when the job is cancelled or the executor closes, and a report
// callback that records progress and statistics in the corpus.
type JobFunc func(ctx context.Context, report func(progress, statistics string) error) (any, error)

func jobProgressCounts(completed, total int) string {
	return fmt.Sprintf(`{"completed_items":%d,"total_items":%d}`, completed, total)
}

type activeJob struct {
	id     string
	cancel context.CancelFunc
}

// jobExecutorConfig tunes the owner/lease/heartbeat protocol. Use
// defaultJobExecutorConfig for production values.
type jobExecutorConfig struct {
	leaseTimeout      time.Duration
	heartbeatInterval time.Duration
	pollInterval      time.Duration
}

func defaultJobExecutorConfig() jobExecutorConfig {
	return jobExecutorConfig{
		leaseTimeout:      10 * time.Second,
		heartbeatInterval: 2 * time.Second,
		pollInterval:      200 * time.Millisecond,
	}
}

// JobExecutor submits durable jobs, runs them asynchronously, and supports
// cancellation, progress recording, and safe shutdown.
//
// Each executor registers a unique owner in the corpus and heartbeats while it
// is open. Startup only reconciles jobs whose owner is missing or has a stale
// heartbeat; live owners from other processes are never failed. Workers poll
// the corpus for cancellation requests so a job cancelled by another process
// stops promptly.
type JobExecutor struct {
	corpus  *corpus.Corpus
	ownerID string
	rootCtx context.Context
	cancel  context.CancelFunc
	cfg     jobExecutorConfig

	mu          sync.Mutex
	cond        *sync.Cond
	closed      bool
	active      map[string]*activeJob
	activeCount int
	heartbeatWG sync.WaitGroup
}

func newJobExecutor(ctx context.Context, c *corpus.Corpus) (*JobExecutor, error) {
	return newJobExecutorWithConfig(ctx, c, defaultJobExecutorConfig())
}

func newJobExecutorWithConfig(ctx context.Context, c *corpus.Corpus, cfg jobExecutorConfig) (*JobExecutor, error) {
	if cfg.leaseTimeout <= 0 {
		cfg.leaseTimeout = defaultJobExecutorConfig().leaseTimeout
	}
	if cfg.heartbeatInterval <= 0 {
		cfg.heartbeatInterval = defaultJobExecutorConfig().heartbeatInterval
	}
	if cfg.pollInterval <= 0 {
		cfg.pollInterval = defaultJobExecutorConfig().pollInterval
	}

	ownerID := uuid.NewString()
	e := &JobExecutor{
		corpus:  c,
		ownerID: ownerID,
		cfg:     cfg,
		active:  make(map[string]*activeJob),
	}
	e.rootCtx, e.cancel = context.WithCancel(context.Background())
	e.cond = sync.NewCond(&e.mu)

	now := time.Now().UTC()
	if err := c.RegisterJobOwner(ctx, ownerID, os.Getpid(), now); err != nil {
		e.cancel()
		return nil, fmt.Errorf("register job owner: %w", err)
	}

	e.heartbeatWG.Add(1)
	go e.heartbeat()

	if err := c.ReconcileInterruptedJobs(ctx, cfg.leaseTimeout); err != nil {
		e.cancel()
		e.heartbeatWG.Wait()
		_ = c.DeleteJobOwner(context.WithoutCancel(ctx), ownerID)
		return nil, fmt.Errorf("reconcile interrupted jobs: %w", err)
	}

	return e, nil
}

// Submit persists a queued job and runs it asynchronously.
func (e *JobExecutor) Submit(ctx context.Context, kind string, request any, fn JobFunc) (string, error) {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return "", errors.New("job executor is closed")
	}
	e.activeCount++
	e.mu.Unlock()

	reqJSON, err := json.Marshal(request)
	if err != nil {
		e.decrement()
		return "", fmt.Errorf("marshal job request: %w", err)
	}

	job, err := e.corpus.CreateJob(ctx, kind, string(reqJSON))
	if err != nil {
		e.decrement()
		return "", err
	}

	go e.run(job.ID, fn)
	return job.ID, nil
}

// Get returns a job by opaque ID.
func (e *JobExecutor) Get(ctx context.Context, id string) (*corpus.Job, error) {
	return e.corpus.GetJob(ctx, id)
}

// List returns recent jobs, optionally filtered by status.
func (e *JobExecutor) List(ctx context.Context, status string, limit int) ([]corpus.Job, error) {
	return e.corpus.ListJobs(ctx, status, limit)
}

// Cancel requests cancellation for a job. Queued jobs are marked cancelled
// immediately; running jobs have their context cancelled and finish as
// cancelled.
func (e *JobExecutor) Cancel(ctx context.Context, id string) error {
	if err := e.corpus.RequestJobCancellation(ctx, id); err != nil {
		return err
	}
	e.mu.Lock()
	aj, ok := e.active[id]
	e.mu.Unlock()
	if ok {
		aj.cancel()
	}
	return nil
}

// Close cancels all running jobs and waits for their goroutines to finish
// before returning.
func (e *JobExecutor) Close() error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil
	}
	e.closed = true
	active := make([]*activeJob, 0, len(e.active))
	for _, aj := range e.active {
		active = append(active, aj)
	}
	e.mu.Unlock()

	for _, aj := range active {
		aj.cancel()
	}
	e.cancel()

	e.mu.Lock()
	for e.activeCount > 0 {
		e.cond.Wait()
	}
	e.mu.Unlock()

	e.heartbeatWG.Wait()
	_ = e.corpus.DeleteJobOwner(context.Background(), e.ownerID)
	return nil
}

func (e *JobExecutor) decrement() {
	e.mu.Lock()
	e.activeCount--
	if e.activeCount == 0 {
		e.cond.Broadcast()
	}
	e.mu.Unlock()
}

func (e *JobExecutor) heartbeat() {
	defer e.heartbeatWG.Done()
	ticker := time.NewTicker(e.cfg.heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-e.rootCtx.Done():
			return
		case <-ticker.C:
			err := e.corpus.HeartbeatJobOwner(e.rootCtx, e.ownerID, time.Now().UTC())
			if err == nil {
				continue
			}
			if errors.Is(err, corpus.ErrJobOwnerNotFound) || errors.Is(err, context.Canceled) {
				// The owner row was removed (abandoned) or the executor is shutting
				// down; stop heartbeating.
				return
			}
			// Transient errors (database locked, network, etc.) are retried on
			// the next tick so a temporary stall does not kill a live owner.
		}
	}
}

func (e *JobExecutor) pollCancellation(ctx context.Context, id string, cancel context.CancelFunc) {
	ticker := time.NewTicker(e.cfg.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			job, err := e.corpus.GetJob(ctx, id)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				// Transient DB errors (e.g. SQLITE_BUSY) are retried; only a
				// definitive absence or a persisted cancellation stops the job.
				continue
			}
			if job == nil {
				cancel()
				return
			}
			if job.CancelledAt != nil && !job.CancelledAt.IsZero() {
				cancel()
				return
			}
		}
	}
}

func (e *JobExecutor) run(id string, fn JobFunc) {
	defer e.decrement()

	jobCtx, cancel := context.WithCancel(e.rootCtx)
	defer cancel()

	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		_ = e.corpus.TransitionJob(context.WithoutCancel(jobCtx), id, corpus.JobStatusQueued, corpus.JobStatusFailed, "", "executor closed before start")
		return
	}
	e.active[id] = &activeJob{id: id, cancel: cancel}
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		delete(e.active, id)
		e.mu.Unlock()
		cancel()
	}()

	if err := e.corpus.StartJobAs(jobCtx, id, e.ownerID); err != nil {
		job, _ := e.corpus.GetJob(context.WithoutCancel(jobCtx), id)
		if job != nil && !isTerminalJobStatus(job.Status) {
			_ = e.corpus.TransitionJob(context.WithoutCancel(jobCtx), id, job.Status, corpus.JobStatusFailed, "", err.Error())
		}
		return
	}

	go e.pollCancellation(jobCtx, id, cancel)

	_ = e.corpus.RecordJobEvent(context.WithoutCancel(jobCtx), id, "info", "job started")

	result, runErr := fn(jobCtx, func(progress, statistics string) error {
		return e.corpus.UpdateJobProgress(jobCtx, id, progress, statistics)
	})

	writeCtx := context.WithoutCancel(jobCtx)

	job, _ := e.corpus.GetJob(writeCtx, id)
	if job != nil && job.CancelledAt != nil && !job.CancelledAt.IsZero() {
		_ = e.finishJob(writeCtx, id, corpus.JobStatusCancelled, "", "cancelled by request")
		return
	}

	if jobCtx.Err() != nil {
		_ = e.finishJob(writeCtx, id, corpus.JobStatusCancelled, "", jobCtx.Err().Error())
		return
	}

	if runErr != nil {
		_ = e.finishJob(writeCtx, id, corpus.JobStatusFailed, "", runErr.Error())
		return
	}

	resultJSON, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		_ = e.finishJob(writeCtx, id, corpus.JobStatusFailed, "", marshalErr.Error())
		return
	}

	if err := e.finishJob(writeCtx, id, corpus.JobStatusSucceeded, string(resultJSON), ""); err != nil {
		_ = e.finishJob(writeCtx, id, corpus.JobStatusFailed, "", err.Error())
	}
}

func (e *JobExecutor) finishJob(ctx context.Context, id, status, result, errStr string) error {
	err := e.corpus.TransitionJob(ctx, id, corpus.JobStatusRunning, status, result, errStr)
	if errors.Is(err, corpus.ErrJobCancelled) {
		// A cancellation request arrived during completion; finish as cancelled.
		_ = e.corpus.TransitionJob(ctx, id, corpus.JobStatusRunning, corpus.JobStatusCancelled, "", err.Error())
		return nil
	}
	if err != nil {
		return err
	}
	_ = e.corpus.RecordJobEvent(ctx, id, "info", "job "+status)
	return nil
}

func isTerminalJobStatus(status string) bool {
	return status == corpus.JobStatusSucceeded || status == corpus.JobStatusFailed || status == corpus.JobStatusCancelled
}

// ListJobs returns bounded durable jobs for CLI and MCP adapters.
func (s *Service) ListJobs(ctx context.Context, status string, limit int) (*cli.JobListResult, error) {
	jobs, err := s.Jobs(ctx)
	if err != nil {
		return nil, err
	}
	items, err := jobs.List(ctx, status, limit)
	if err != nil {
		return nil, err
	}
	result := &cli.JobListResult{Jobs: make([]cli.JobResult, len(items))}
	for i := range items {
		result.Jobs[i] = jobResult(&items[i])
	}
	return result, nil
}

// GetJob returns one durable job by opaque ID.
func (s *Service) GetJob(ctx context.Context, id string) (*cli.JobResult, error) {
	jobs, err := s.Jobs(ctx)
	if err != nil {
		return nil, err
	}
	job, err := jobs.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, cli.NewCLIError(cli.ExitNotFound, fmt.Errorf("job %s not found", id))
	}
	result := jobResult(job)
	return &result, nil
}

// submitJob persists a durable job and runs fn asynchronously.
func (s *Service) submitJob(ctx context.Context, kind string, request any, fn JobFunc) (string, error) {
	jobs, err := s.Jobs(ctx)
	if err != nil {
		return "", err
	}
	return jobs.Submit(ctx, kind, request, fn)
}

// CancelJob records and applies a cancellation request, then returns current state.
func (s *Service) CancelJob(ctx context.Context, id string) (*cli.JobResult, error) {
	jobs, err := s.Jobs(ctx)
	if err != nil {
		return nil, err
	}
	if err := jobs.Cancel(ctx, id); err != nil {
		return nil, err
	}
	return s.GetJob(ctx, id)
}

func jobResult(job *corpus.Job) cli.JobResult {
	result := cli.JobResult{
		ID: job.ID, Kind: job.Kind, Status: job.Status, Request: job.Request,
		Result: job.Result, Error: job.Error, Progress: job.Progress,
		Statistics: job.Statistics, CreatedAt: formatTime(job.CreatedAt),
		Cancellation: job.CancelledAt != nil,
	}
	if job.StartedAt != nil {
		result.StartedAt = formatTime(*job.StartedAt)
	}
	if job.CompletedAt != nil {
		result.CompletedAt = formatTime(*job.CompletedAt)
	}
	if job.CancelledAt != nil {
		result.CancelledAt = formatTime(*job.CancelledAt)
	}
	return result
}
