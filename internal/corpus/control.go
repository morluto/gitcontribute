package corpus

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/pressly/goose/v3"
)

// ControlStats is a bounded local snapshot used by status and diagnostics.
type ControlStats struct {
	Repositories  int
	Threads       int
	Sources       int
	FrontierReady int
	ActiveRuns    int
	ActiveJobs    int
	Freshest      time.Time
}

// SchemaVersion returns the applied Goose schema version.
func (c *Corpus) SchemaVersion(ctx context.Context) (int64, error) {
	version, err := goose.GetDBVersionContext(ctx, c.db)
	if err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return version, nil
}

// ControlStats returns local counts without triggering refresh or hydration.
func (c *Corpus) ControlStats(ctx context.Context, now time.Time) (ControlStats, error) {
	var out ControlStats
	queries := []struct {
		dst   *int
		query string
		args  []any
	}{
		{&out.Repositories, `SELECT COUNT(*) FROM repositories`, nil},
		{&out.Threads, `SELECT COUNT(*) FROM threads`, nil},
		{&out.Sources, `SELECT COUNT(*) FROM discovery_sources WHERE enabled = 1`, nil},
		{&out.FrontierReady, `SELECT COUNT(*) FROM frontier_items WHERE state = 'queued' AND earliest_run_at <= ?`, []any{encodeTime(now)}},
		{&out.ActiveRuns, `SELECT COUNT(*) FROM runs WHERE status = ?`, []any{RunStatusRunning}},
	}
	for _, item := range queries {
		if err := c.db.QueryRowContext(ctx, item.query, item.args...).Scan(item.dst); err != nil {
			return ControlStats{}, fmt.Errorf("read control statistics: %w", err)
		}
	}

	jobs, err := c.tableExists(ctx, "jobs")
	if err != nil {
		return ControlStats{}, err
	}
	if jobs {
		if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE status IN ('queued', 'running')`).Scan(&out.ActiveJobs); err != nil {
			return ControlStats{}, fmt.Errorf("count active jobs: %w", err)
		}
	}

	var freshest sql.NullInt64
	if err := c.db.QueryRowContext(ctx, `
		SELECT MAX(value) FROM (
			SELECT MAX(source_updated_at) AS value FROM repositories
			UNION ALL
			SELECT MAX(source_updated_at) AS value FROM threads
		)
	`).Scan(&freshest); err != nil {
		return ControlStats{}, fmt.Errorf("read corpus freshness: %w", err)
	}
	if freshest.Valid {
		out.Freshest = scanTime(freshest.Int64)
	}
	return out, nil
}

// CheckIntegrity performs bounded, local database health checks and verifies
// that an immediate write lock can be acquired. It does not mutate user data.
func (c *Corpus) CheckIntegrity(ctx context.Context) error {
	var result string
	if err := c.db.QueryRowContext(ctx, `PRAGMA quick_check(1)`).Scan(&result); err != nil {
		return fmt.Errorf("database quick check: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("database quick check: %s", result)
	}

	conn, err := c.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire database connection: %w", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return fmt.Errorf("acquire database write lock: %w", err)
	}
	if _, err := conn.ExecContext(context.WithoutCancel(ctx), `ROLLBACK`); err != nil {
		return fmt.Errorf("release database write lock: %w", err)
	}
	return nil
}

func (c *Corpus) tableExists(ctx context.Context, name string) (bool, error) {
	var count int
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&count); err != nil {
		return false, fmt.Errorf("inspect table %s: %w", name, err)
	}
	return count == 1, nil
}
