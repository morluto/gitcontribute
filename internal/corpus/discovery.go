package corpus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// GetTime implements discovery.CheckpointStore using the local corpus.
func (c *Corpus) GetTime(ctx context.Context, key string) (time.Time, bool, error) {
	var encoded int64
	err := c.db.QueryRowContext(ctx, `
		SELECT checkpoint_at FROM discovery_checkpoints WHERE key = ?
	`, key).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("get discovery checkpoint: %w", err)
	}
	return scanTime(encoded), true, nil
}

// SetTime atomically advances a discovery timestamp checkpoint. Older replayed
// checkpoints cannot move it backwards.
func (c *Corpus) SetTime(ctx context.Context, key string, checkpoint time.Time) error {
	if key == "" {
		return errors.New("discovery checkpoint key is required")
	}
	now := encodeTime(time.Now())
	_, err := c.db.ExecContext(ctx, `
		INSERT INTO discovery_checkpoints (key, checkpoint_at, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET
			checkpoint_at = MAX(discovery_checkpoints.checkpoint_at, excluded.checkpoint_at),
			updated_at = excluded.updated_at
	`, key, encodeTime(checkpoint), now)
	if err != nil {
		return fmt.Errorf("set discovery checkpoint: %w", err)
	}
	return nil
}

// IsImported implements discovery.CheckpointStore for GH Archive hours.
func (c *Corpus) IsImported(ctx context.Context, hour string) (bool, error) {
	var exists int
	err := c.db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM archive_imports WHERE hour_key = ?)
	`, hour).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check archive import: %w", err)
	}
	return exists != 0, nil
}

// MarkImported records an imported GH Archive hour idempotently.
func (c *Corpus) MarkImported(ctx context.Context, hour string) error {
	if hour == "" {
		return errors.New("archive hour key is required")
	}
	_, err := c.db.ExecContext(ctx, `
		INSERT INTO archive_imports (hour_key, imported_at)
		VALUES (?, ?)
		ON CONFLICT (hour_key) DO NOTHING
	`, hour, encodeTime(time.Now()))
	if err != nil {
		return fmt.Errorf("mark archive import: %w", err)
	}
	return nil
}
