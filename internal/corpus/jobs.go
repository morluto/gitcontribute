package corpus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrJobCancelled is returned when a terminal transition is blocked because a
// cancellation has already been requested for the job.
var ErrJobCancelled = errors.New("job cancellation requested")

// ErrJobOwnerNotFound is returned when a heartbeat targets an owner row that
// no longer exists.
var ErrJobOwnerNotFound = errors.New("job owner not found")

// CreateJob creates a new job in the queued state with an opaque stable ID.
func (c *Corpus) CreateJob(ctx context.Context, kind, request string) (*Job, error) {
	if strings.TrimSpace(kind) == "" {
		return nil, errors.New("job kind is required")
	}
	if request == "" {
		request = "{}"
	}
	now := time.Now().UTC()
	id := uuid.NewString()
	if _, err := c.db.ExecContext(ctx, `
		INSERT INTO jobs (id, kind, status, request, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, id, kind, JobStatusQueued, request, encodeTime(now), encodeTime(now)); err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}
	return c.GetJob(ctx, id)
}

// GetJob returns a job by opaque ID, or nil when absent.
func (c *Corpus) GetJob(ctx context.Context, id string) (*Job, error) {
	row := c.db.QueryRowContext(ctx, jobSelect+` WHERE id = ?`, id)
	job, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get job: %w", err)
	}
	return job, nil
}

// ListJobs returns recent jobs bounded by limit, optionally filtered by status.
func (c *Corpus) ListJobs(ctx context.Context, status string, limit int) ([]Job, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	query := jobSelect + ` WHERE 1=1`
	var args []any
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	var out []Job
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *job)
	}
	return out, rows.Err()
}

// StartJob atomically transitions a queued job to running without an owner.
func (c *Corpus) StartJob(ctx context.Context, id string) error {
	return c.StartJobAs(ctx, id, "")
}

// StartJobAs atomically transitions a queued job to running and claims it for
// the given owner. An empty ownerID leaves owner_id NULL.
func (c *Corpus) StartJobAs(ctx context.Context, id, ownerID string) error {
	now := time.Now().UTC()
	res, err := c.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = ?, started_at = ?, updated_at = ?, owner_id = NULLIF(?, '')
		WHERE id = ? AND status = ? AND COALESCE(cancelled_at, 0) = 0
	`, JobStatusRunning, encodeTime(now), encodeTime(now), ownerID, id, JobStatusQueued)
	if err != nil {
		return fmt.Errorf("start job: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		job, _ := c.GetJob(ctx, id)
		if job == nil {
			return errors.New("job not found")
		}
		if isTerminalJobStatus(job.Status) {
			return fmt.Errorf("job is already %s", job.Status)
		}
		if job.CancelledAt != nil && !job.CancelledAt.IsZero() {
			return ErrJobCancelled
		}
		return fmt.Errorf("job is not queued")
	}
	return nil
}

// TransitionJob performs a safe atomic terminal transition for a job. The
// current status must match from, and cancellation requests block transitions
// to non-cancelled terminal states. Terminal transitions clear the owner.
func (c *Corpus) TransitionJob(ctx context.Context, id, from, to, result, errStr string) error {
	if !isValidJobTransition(from, to) {
		return fmt.Errorf("invalid job transition from %s to %s", from, to)
	}
	if from == to {
		return nil
	}
	now := time.Now().UTC()
	res, dbErr := c.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = ?, result = ?, error = ?, completed_at = ?, updated_at = ?,
		    owner_id = CASE
		        WHEN ? = ? OR ? = ? OR ? = ?
		        THEN NULL
		        ELSE owner_id
		    END
		WHERE id = ? AND status = ? AND (COALESCE(cancelled_at, 0) = 0 OR ? = ?)
	`, to, result, errStr, encodeTime(now), encodeTime(now),
		to, JobStatusSucceeded, to, JobStatusFailed, to, JobStatusCancelled,
		id, from, to, JobStatusCancelled)
	if dbErr != nil {
		return fmt.Errorf("transition job: %w", dbErr)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		job, _ := c.GetJob(ctx, id)
		if job == nil {
			return errors.New("job not found")
		}
		if job.Status != from {
			return fmt.Errorf("job status is %s, expected %s", job.Status, from)
		}
		if job.CancelledAt != nil && !job.CancelledAt.IsZero() && to != JobStatusCancelled {
			return ErrJobCancelled
		}
		return errors.New("transition not applied")
	}
	return nil
}

// UpdateJobProgress updates progress and statistics for a running job.
func (c *Corpus) UpdateJobProgress(ctx context.Context, id, progress, statistics string) error {
	now := time.Now().UTC()
	res, err := c.db.ExecContext(ctx, `
		UPDATE jobs
		SET progress = ?, statistics = ?, updated_at = ?
		WHERE id = ? AND status = ? AND COALESCE(cancelled_at, 0) = 0
	`, progress, statistics, encodeTime(now), id, JobStatusRunning)
	if err != nil {
		return fmt.Errorf("update job progress: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		job, _ := c.GetJob(ctx, id)
		if job == nil {
			return errors.New("job not found")
		}
		if isTerminalJobStatus(job.Status) {
			return fmt.Errorf("job is already %s", job.Status)
		}
		if job.CancelledAt != nil && !job.CancelledAt.IsZero() {
			return ErrJobCancelled
		}
		return errors.New("job is not running")
	}
	return nil
}

// RequestJobCancellation records a cancellation request. Queued jobs are
// moved directly to cancelled; running jobs have cancelled_at set so that they
// finish as cancelled.
func (c *Corpus) RequestJobCancellation(ctx context.Context, id string) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin cancel job: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM jobs WHERE id = ?`, id).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("job not found")
		}
		return fmt.Errorf("select job status: %w", err)
	}

	now := time.Now().UTC()
	nowEncoded := encodeTime(now)
	switch status {
	case JobStatusQueued:
		if _, err := tx.ExecContext(ctx, `
			UPDATE jobs
			SET status = ?, completed_at = ?, cancelled_at = ?, updated_at = ?
			WHERE id = ? AND status = ?
		`, JobStatusCancelled, nowEncoded, nowEncoded, nowEncoded, id, JobStatusQueued); err != nil {
			return fmt.Errorf("cancel queued job: %w", err)
		}
	case JobStatusRunning:
		if _, err := tx.ExecContext(ctx, `
			UPDATE jobs
			SET cancelled_at = ?, updated_at = ?
			WHERE id = ? AND status = ?
		`, nowEncoded, nowEncoded, id, JobStatusRunning); err != nil {
			return fmt.Errorf("request job cancellation: %w", err)
		}
	default:
		if isTerminalJobStatus(status) {
			return fmt.Errorf("job is already %s", status)
		}
		return fmt.Errorf("cannot cancel job in status %s", status)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit cancel job: %w", err)
	}
	return nil
}

// RecordJobEvent appends a durable event to a job.
func (c *Corpus) RecordJobEvent(ctx context.Context, jobID, level, message string) error {
	now := time.Now().UTC()
	if _, err := c.db.ExecContext(ctx, `
		INSERT INTO job_events (job_id, level, message, recorded_at)
		VALUES (?, ?, ?, ?)
	`, jobID, level, message, encodeTime(now)); err != nil {
		return fmt.Errorf("record job event: %w", err)
	}
	return nil
}

// ListJobEvents returns events for a job in chronological order.
func (c *Corpus) ListJobEvents(ctx context.Context, jobID string) ([]JobEvent, error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT id, job_id, level, message, recorded_at
		FROM job_events
		WHERE job_id = ?
		ORDER BY id
	`, jobID)
	if err != nil {
		return nil, fmt.Errorf("list job events: %w", err)
	}
	defer rows.Close()

	var out []JobEvent
	for rows.Next() {
		var e JobEvent
		var recorded int64
		if err := rows.Scan(&e.ID, &e.JobID, &e.Level, &e.Message, &recorded); err != nil {
			return nil, err
		}
		e.RecordedAt = scanTime(recorded)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ReconcileInterruptedJobs marks running jobs as failed or cancelled when
// their owning process has not heartbeated within leaseTimeout. Live owners
// are left untouched, and stale owner records are removed.
//
// It uses BEGIN IMMEDIATE so the write lock is acquired before any reads,
// avoiding a lock-upgrade race with concurrent heartbeats.
func (c *Corpus) ReconcileInterruptedJobs(ctx context.Context, leaseTimeout time.Duration) error {
	// Reserve a single connection and take the write lock immediately so that
	// heartbeats cannot interleave between our read and our writes.
	conn, err := c.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve reconcile connection: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("begin reconcile jobs: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
		}
	}()

	now := time.Now().UTC()
	threshold := encodeTime(now.Add(-leaseTimeout))

	rows, err := conn.QueryContext(ctx, `
		SELECT j.id, j.cancelled_at, j.owner_id, COALESCE(o.heartbeat_at, 0)
		FROM jobs j
		LEFT JOIN job_owners o ON j.owner_id = o.owner_id
		WHERE j.status = ?
	`, JobStatusRunning)
	if err != nil {
		return fmt.Errorf("select interrupted jobs: %w", err)
	}
	type interrupted struct {
		id        string
		cancelled bool
	}
	var jobs []interrupted
	for rows.Next() {
		var id string
		var ownerID sql.NullString
		var cancelled sql.NullInt64
		var heartbeat int64
		if err := rows.Scan(&id, &cancelled, &ownerID, &heartbeat); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan interrupted job: %w", err)
		}
		alive := ownerID.Valid && ownerID.String != "" && heartbeat >= threshold
		if alive {
			continue
		}
		jobs = append(jobs, interrupted{id: id, cancelled: cancelled.Valid && cancelled.Int64 != 0})
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close interrupted rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	nowEncoded := encodeTime(now)
	for _, j := range jobs {
		status := JobStatusFailed
		msg := "interrupted by restart"
		if j.cancelled {
			status = JobStatusCancelled
			msg = "interrupted by restart (cancellation requested)"
		}
		if _, err := conn.ExecContext(ctx, `
			UPDATE jobs
			SET status = ?, completed_at = ?, error = ?, updated_at = ?, owner_id = NULL
			WHERE id = ? AND status = ?
		`, status, nowEncoded, msg, nowEncoded, j.id, JobStatusRunning); err != nil {
			return fmt.Errorf("reconcile interrupted job %s: %w", j.id, err)
		}
		if _, err := conn.ExecContext(ctx, `
			INSERT INTO job_events (job_id, level, message, recorded_at)
			VALUES (?, ?, ?, ?)
		`, j.id, "warn", msg, nowEncoded); err != nil {
			return fmt.Errorf("record interrupted job event %s: %w", j.id, err)
		}
	}

	if _, err := conn.ExecContext(ctx, `
		DELETE FROM job_owners WHERE heartbeat_at < ?
	`, threshold); err != nil {
		return fmt.Errorf("delete stale job owners: %w", err)
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit reconcile jobs: %w", err)
	}
	committed = true
	return nil
}

const jobSelect = `
	SELECT id, kind, status, request, result, error, progress, statistics,
	       created_at, started_at, completed_at, updated_at, cancelled_at
	FROM jobs`

func scanJob(row rowScanner) (*Job, error) {
	var j Job
	var created, updated int64
	var started, completed, cancelled sql.NullInt64
	var result, errStr, progress, stats sql.NullString
	err := row.Scan(&j.ID, &j.Kind, &j.Status, &j.Request, &result, &errStr,
		&progress, &stats, &created, &started, &completed, &updated, &cancelled)
	if err != nil {
		return nil, err
	}
	j.Result = result.String
	j.Error = errStr.String
	j.Progress = progress.String
	j.Statistics = stats.String
	j.CreatedAt = scanTime(created)
	j.UpdatedAt = scanTime(updated)
	if started.Valid {
		t := scanTime(started.Int64)
		j.StartedAt = &t
	}
	if completed.Valid {
		t := scanTime(completed.Int64)
		j.CompletedAt = &t
	}
	if cancelled.Valid && cancelled.Int64 != 0 {
		t := scanTime(cancelled.Int64)
		j.CancelledAt = &t
	}
	return &j, nil
}

func isTerminalJobStatus(status string) bool {
	return status == JobStatusSucceeded || status == JobStatusFailed || status == JobStatusCancelled
}

func isValidJobTransition(from, to string) bool {
	if isTerminalJobStatus(from) && from != to {
		return false
	}
	switch from {
	case JobStatusQueued:
		return to == JobStatusRunning || to == JobStatusCancelled || to == JobStatusFailed
	case JobStatusRunning:
		return to == JobStatusSucceeded || to == JobStatusFailed || to == JobStatusCancelled
	}
	return false
}

// RegisterJobOwner records a process owner with an explicit heartbeat time.
// Calling it again for an existing owner updates its process_id and heartbeat.
func (c *Corpus) RegisterJobOwner(ctx context.Context, ownerID string, processID int, t time.Time) error {
	enc := encodeTime(t)
	if _, err := c.db.ExecContext(ctx, `
		INSERT INTO job_owners (owner_id, process_id, started_at, heartbeat_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (owner_id) DO UPDATE
		SET process_id = excluded.process_id, heartbeat_at = excluded.heartbeat_at
	`, ownerID, processID, enc, enc); err != nil {
		return fmt.Errorf("register job owner: %w", err)
	}
	return nil
}

// HeartbeatJobOwner refreshes the lease heartbeat for an owner.
func (c *Corpus) HeartbeatJobOwner(ctx context.Context, ownerID string, t time.Time) error {
	enc := encodeTime(t)
	res, err := c.db.ExecContext(ctx, `
		UPDATE job_owners SET heartbeat_at = ? WHERE owner_id = ?
	`, enc, ownerID)
	if err != nil {
		return fmt.Errorf("heartbeat job owner: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("heartbeat job owner: %w", ErrJobOwnerNotFound)
	}
	return nil
}

// DeleteJobOwner removes a process owner record.
func (c *Corpus) DeleteJobOwner(ctx context.Context, ownerID string) error {
	if _, err := c.db.ExecContext(ctx, `DELETE FROM job_owners WHERE owner_id = ?`, ownerID); err != nil {
		return fmt.Errorf("delete job owner: %w", err)
	}
	return nil
}
