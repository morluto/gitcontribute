package corpus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// StartRun creates and returns a new run record in the running state.
func (c *Corpus) StartRun(ctx context.Context, kind string) (*Run, error) {
	now := encodeTime(time.Now())
	res, err := c.db.ExecContext(ctx, `
		INSERT INTO runs (kind, status, started_at)
		VALUES (?, ?, ?)
	`, kind, RunStatusRunning, now)
	if err != nil {
		return nil, fmt.Errorf("start run: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last run id: %w", err)
	}
	return c.GetRun(ctx, id)
}

// GetRun returns a run record by id.
func (c *Corpus) GetRun(ctx context.Context, id int64) (*Run, error) {
	var r Run
	var completed sql.NullInt64
	var started int64
	var stats, errStr sql.NullString
	err := c.db.QueryRowContext(ctx, `
		SELECT id, kind, status, started_at, completed_at, stats, error
		FROM runs
		WHERE id = ?
	`, id).Scan(&r.ID, &r.Kind, &r.Status, &started, &completed, &stats, &errStr)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get run: %w", err)
	}
	r.StartedAt = scanTime(started)
	if completed.Valid {
		t := scanTime(completed.Int64)
		r.CompletedAt = &t
	}
	r.Stats = stats.String
	r.Error = errStr.String
	return &r, nil
}

// FinishRun marks a run as completed with optional statistics.
func (c *Corpus) FinishRun(ctx context.Context, id int64, stats string) error {
	now := encodeTime(time.Now())
	res, err := c.db.ExecContext(ctx, `
		UPDATE runs
		SET status = ?, completed_at = ?, stats = ?
		WHERE id = ?
	`, RunStatusCompleted, now, stats, id)
	if err != nil {
		return fmt.Errorf("finish run: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("run %d not found", id)
	}
	return nil
}

// FinishRunPartial records a completed run that made progress but encountered
// retryable gaps.
func (c *Corpus) FinishRunPartial(ctx context.Context, id int64, stats, message string) error {
	now := encodeTime(time.Now())
	res, err := c.db.ExecContext(ctx, `
		UPDATE runs
		SET status = ?, completed_at = ?, stats = ?, error = ?
		WHERE id = ?
	`, RunStatusPartial, now, stats, message, id)
	if err != nil {
		return fmt.Errorf("finish partial run: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("run %d not found", id)
	}
	return nil
}

// FailRun marks a run as failed and stores an error message.
func (c *Corpus) FailRun(ctx context.Context, id int64, message string) error {
	now := encodeTime(time.Now())
	res, err := c.db.ExecContext(ctx, `
		UPDATE runs
		SET status = ?, completed_at = ?, error = ?
		WHERE id = ?
	`, RunStatusFailed, now, message, id)
	if err != nil {
		return fmt.Errorf("fail run: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("run %d not found", id)
	}
	return nil
}

// RecordRunEvent appends a durable event to a run.
func (c *Corpus) RecordRunEvent(ctx context.Context, runID int64, level, message string) error {
	now := encodeTime(time.Now())
	if _, err := c.db.ExecContext(ctx, `
		INSERT INTO run_events (run_id, level, message, recorded_at)
		VALUES (?, ?, ?, ?)
	`, runID, level, message, now); err != nil {
		return fmt.Errorf("record run event: %w", err)
	}
	return nil
}

// ListRuns returns the most recent run records bounded by limit.
func (c *Corpus) ListRuns(ctx context.Context, limit int) ([]Run, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, err := c.db.QueryContext(ctx, `
		SELECT id, kind, status, started_at, completed_at, stats, error
		FROM runs
		ORDER BY started_at DESC, id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()

	var out []Run
	for rows.Next() {
		var r Run
		var completed sql.NullInt64
		var started int64
		var stats, errStr sql.NullString
		if err := rows.Scan(&r.ID, &r.Kind, &r.Status, &started, &completed, &stats, &errStr); err != nil {
			return nil, err
		}
		r.StartedAt = scanTime(started)
		if completed.Valid {
			t := scanTime(completed.Int64)
			r.CompletedAt = &t
		}
		r.Stats = stats.String
		r.Error = errStr.String
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListRunEvents returns events for a run in chronological order.
func (c *Corpus) ListRunEvents(ctx context.Context, runID int64) ([]RunEvent, error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT id, run_id, level, message, recorded_at
		FROM run_events
		WHERE run_id = ?
		ORDER BY id
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("list run events: %w", err)
	}
	defer rows.Close()

	var out []RunEvent
	for rows.Next() {
		var e RunEvent
		var recorded int64
		if err := rows.Scan(&e.ID, &e.RunID, &e.Level, &e.Message, &recorded); err != nil {
			return nil, err
		}
		e.RecordedAt = scanTime(recorded)
		out = append(out, e)
	}
	return out, rows.Err()
}
