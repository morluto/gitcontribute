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
	"github.com/morluto/gitcontribute/internal/corpus"
	"golang.org/x/sync/semaphore"
)

// JobFunc performs asynchronous work for a job. It receives a context that is
// cancelled when the job is cancelled or the executor closes, and a report
// callback that records progress and statistics in the corpus.
type JobFunc func(ctx context.Context, report func(progress, statistics string) error) (any, error)

const jobCleanupTimeout = 5 * time.Second

var ErrJobQueueFull = errors.New("job executor queue is full")

type jobStore interface {
	StoppedJobIDs(context.Context, []string) (map[string]struct{}, error)
	CreateJob(context.Context, string, string) (*corpus.Job, error)
	DeleteJobOwner(context.Context, string) error
	GetJob(context.Context, string) (*corpus.Job, error)
	HeartbeatJobOwner(context.Context, string, time.Time) error
	ListJobs(context.Context, string, int) ([]corpus.Job, error)
	RecordJobEvent(context.Context, string, string, string) error
	RegisterJobOwner(context.Context, string, int, time.Time) error
	ReconcileInterruptedJobs(context.Context, time.Duration) error
	RequestJobCancellation(context.Context, string) error
	StartJobAs(context.Context, string, string) error
	TransitionJob(context.Context, string, string, string, string, string) error
	UpdateJobProgress(context.Context, string, string, string) error
}

// jobExecutorConfig tunes the owner/lease/heartbeat protocol. Use
// defaultJobExecutorConfig for production values.
type jobExecutorConfig struct {
	leaseTimeout      time.Duration
	heartbeatInterval time.Duration
	pollInterval      time.Duration
	maxConcurrentJobs int64
	maxAdmittedJobs   int64
}

func defaultJobExecutorConfig() jobExecutorConfig {
	return jobExecutorConfig{
		leaseTimeout:      10 * time.Second,
		heartbeatInterval: 2 * time.Second,
		pollInterval:      200 * time.Millisecond,
		maxConcurrentJobs: 4,
		maxAdmittedJobs:   256,
	}
}

// JobExecutor persists job records, runs work asynchronously in this process,
// and supports cancellation, progress recording, and safe shutdown. It does
// not replay interrupted host or network operations after restart.
//
// Each executor registers a unique owner in the corpus and heartbeats while it
// is open. Startup only reconciles jobs whose owner is missing or has a stale
// heartbeat; live owners from other processes are never failed. One executor
// watcher polls admitted IDs together so cross-process cancellation stays prompt
// without one SQLite query loop per job.
type JobExecutor struct {
	corpus  jobStore
	ownerID string
	rootCtx context.Context
	cancel  context.CancelFunc
	cfg     jobExecutorConfig
	slots   *semaphore.Weighted

	mu            sync.Mutex
	cond          *sync.Cond
	closed        bool
	admitted      map[string]context.CancelFunc
	admittedCount int64
	backgroundWG  sync.WaitGroup
}

func newJobExecutorWithConfig(ctx context.Context, c jobStore, cfg jobExecutorConfig) (*JobExecutor, error) {
	if cfg.leaseTimeout <= 0 {
		cfg.leaseTimeout = defaultJobExecutorConfig().leaseTimeout
	}
	if cfg.heartbeatInterval <= 0 {
		cfg.heartbeatInterval = defaultJobExecutorConfig().heartbeatInterval
	}
	if cfg.pollInterval <= 0 {
		cfg.pollInterval = defaultJobExecutorConfig().pollInterval
	}
	if cfg.maxConcurrentJobs <= 0 {
		cfg.maxConcurrentJobs = defaultJobExecutorConfig().maxConcurrentJobs
	}
	if cfg.maxAdmittedJobs <= 0 {
		cfg.maxAdmittedJobs = defaultJobExecutorConfig().maxAdmittedJobs
	}
	if cfg.maxAdmittedJobs < cfg.maxConcurrentJobs {
		return nil, errors.New("maximum admitted jobs cannot be lower than maximum concurrent jobs")
	}

	ownerID := uuid.NewString()
	e := &JobExecutor{
		corpus:   c,
		ownerID:  ownerID,
		cfg:      cfg,
		admitted: make(map[string]context.CancelFunc),
		slots:    semaphore.NewWeighted(cfg.maxConcurrentJobs),
	}
	e.rootCtx, e.cancel = context.WithCancel(ctx)
	e.cond = sync.NewCond(&e.mu)

	now := time.Now().UTC()
	if err := c.RegisterJobOwner(ctx, ownerID, os.Getpid(), now); err != nil {
		e.cancel()
		return nil, fmt.Errorf("register job owner: %w", err)
	}

	e.backgroundWG.Add(2)
	go e.heartbeat()
	go e.watchCancellations()

	if err := c.ReconcileInterruptedJobs(ctx, cfg.leaseTimeout); err != nil {
		e.cancel()
		e.backgroundWG.Wait()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), jobCleanupTimeout)
		defer cleanupCancel()
		cleanupErr := c.DeleteJobOwner(cleanupCtx, ownerID)
		if cleanupErr != nil {
			cleanupErr = fmt.Errorf("delete job owner after reconciliation failure: %w", cleanupErr)
		}
		return nil, errors.Join(fmt.Errorf("reconcile interrupted jobs: %w", err), cleanupErr)
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
	if e.admittedCount >= e.cfg.maxAdmittedJobs {
		e.mu.Unlock()
		return "", ErrJobQueueFull
	}
	e.admittedCount++
	e.mu.Unlock()

	reqJSON, err := json.Marshal(request)
	if err != nil {
		e.releaseAdmission()
		return "", fmt.Errorf("marshal job request: %w", err)
	}

	job, err := e.corpus.CreateJob(ctx, kind, string(reqJSON))
	if err != nil {
		e.releaseAdmission()
		return "", err
	}

	jobCtx, cancel := context.WithCancel(e.rootCtx)
	e.mu.Lock()
	e.admitted[job.ID] = cancel
	e.mu.Unlock()

	//nolint:contextcheck // Job lifetime belongs to the executor, not the Submit request.
	go e.run(jobCtx, job.ID, cancel, fn)
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
	cancel, ok := e.admitted[id]
	e.mu.Unlock()
	if ok {
		cancel()
	}
	return nil
}

// Close stops running and waiting jobs and waits for their goroutines.
func (e *JobExecutor) Close() error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil
	}
	e.closed = true
	admitted := make([]context.CancelFunc, 0, len(e.admitted))
	for _, cancel := range e.admitted {
		admitted = append(admitted, cancel)
	}
	e.mu.Unlock()

	for _, cancel := range admitted {
		cancel()
	}
	e.cancel()

	e.mu.Lock()
	for e.admittedCount > 0 {
		e.cond.Wait()
	}
	e.mu.Unlock()

	e.backgroundWG.Wait()
	cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(e.rootCtx), jobCleanupTimeout)
	defer cleanupCancel()
	return e.corpus.DeleteJobOwner(cleanupCtx, e.ownerID)
}

func (e *JobExecutor) releaseAdmission() {
	e.mu.Lock()
	e.admittedCount--
	if e.admittedCount == 0 {
		e.cond.Broadcast()
	}
	e.mu.Unlock()
}

func (e *JobExecutor) heartbeat() {
	defer e.backgroundWG.Done()
	timer := time.NewTimer(e.cfg.heartbeatInterval)
	defer timer.Stop()
	for {
		select {
		case <-e.rootCtx.Done():
			return
		case <-timer.C:
			err := e.corpus.HeartbeatJobOwner(e.rootCtx, e.ownerID, time.Now().UTC())
			if err == nil {
				timer.Reset(e.cfg.heartbeatInterval)
				continue
			}
			if errors.Is(err, corpus.ErrJobOwnerNotFound) || errors.Is(err, context.Canceled) {
				// The owner row was removed (abandoned) or the executor is shutting
				// down; stop heartbeating.
				return
			}
			// Wait a full interval after a transient failure. A fixed-rate ticker
			// can have another tick waiting after a slow SQLite call and turn a
			// temporary stall into sustained writer pressure.
			timer.Reset(e.cfg.heartbeatInterval)
		}
	}
}

func (e *JobExecutor) watchCancellations() {
	defer e.backgroundWG.Done()
	ticker := time.NewTicker(e.cfg.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-e.rootCtx.Done():
			return
		case <-ticker.C:
			e.mu.Lock()
			ids := make([]string, 0, len(e.admitted))
			admitted := make(map[string]context.CancelFunc, len(e.admitted))
			for id, cancel := range e.admitted {
				ids = append(ids, id)
				admitted[id] = cancel
			}
			e.mu.Unlock()
			if len(ids) == 0 {
				continue
			}
			stopped, err := e.corpus.StoppedJobIDs(e.rootCtx, ids)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				// Transient DB errors (for example SQLITE_BUSY) retry next tick.
				continue
			}
			for id := range stopped {
				if cancel := admitted[id]; cancel != nil {
					cancel()
				}
			}
		}
	}
}

func (e *JobExecutor) run(jobCtx context.Context, id string, cancel context.CancelFunc, fn JobFunc) {
	defer e.releaseAdmission()
	defer cancel()
	defer func() {
		e.mu.Lock()
		delete(e.admitted, id)
		e.mu.Unlock()
	}()

	if err := e.slots.Acquire(jobCtx, 1); err != nil {
		if e.rootCtx.Err() != nil {
			_ = e.corpus.TransitionJob(
				context.WithoutCancel(jobCtx), id,
				corpus.JobStatusQueued, corpus.JobStatusFailed, "", "executor closed before start",
			)
		}
		return
	}
	defer e.slots.Release(1)

	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		_ = e.corpus.TransitionJob(context.WithoutCancel(jobCtx), id, corpus.JobStatusQueued, corpus.JobStatusFailed, "", "executor closed before start")
		return
	}
	e.mu.Unlock()

	if err := e.corpus.StartJobAs(jobCtx, id, e.ownerID); err != nil {
		writeCtx := context.WithoutCancel(jobCtx)
		job, getErr := e.corpus.GetJob(writeCtx, id)
		if getErr != nil {
			message := errors.Join(err, fmt.Errorf("get job after start failure: %w", getErr)).Error()
			// Best effort: there is no synchronous caller after the executor goroutine starts.
			//nolint:errcheck
			_ = e.corpus.TransitionJob(writeCtx, id, corpus.JobStatusQueued, corpus.JobStatusFailed, "", message)
			return
		}
		if job != nil && !isTerminalJobStatus(job.Status) {
			// Best effort: preserve the original start error in durable job state.
			//nolint:errcheck
			_ = e.corpus.TransitionJob(writeCtx, id, job.Status, corpus.JobStatusFailed, "", err.Error())
		}
		return
	}

	_ = e.corpus.RecordJobEvent(context.WithoutCancel(jobCtx), id, "info", "job started")

	result, runErr := fn(jobCtx, func(progress, statistics string) error {
		return e.corpus.UpdateJobProgress(jobCtx, id, progress, statistics)
	})

	writeCtx := context.WithoutCancel(jobCtx)

	job, err := e.corpus.GetJob(writeCtx, id)
	if err != nil {
		// Best effort: preserve the read error in durable job state.
		//nolint:errcheck
		_ = e.finishJob(writeCtx, id, corpus.JobStatusFailed, "", fmt.Errorf("get job after execution: %w", err).Error())
		return
	}
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
