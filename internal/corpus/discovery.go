package corpus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// DiscoverySource is one durable repository-discovery definition.
type DiscoverySource struct {
	ID         int64
	Name       string
	Kind       string
	Definition string
	Enabled    bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// SourcePartition records one observed GitHub Search window.
type SourcePartition struct {
	SourceID     int64
	Key          string
	Query        string
	Qualifier    string
	Start        time.Time
	End          time.Time
	Total        int
	Incomplete   bool
	Unsplittable bool
	Retries      int
	ObservedAt   time.Time
}

// SaveDiscoverySource creates or updates a named source definition.
func (c *Corpus) SaveDiscoverySource(ctx context.Context, source DiscoverySource) (*DiscoverySource, error) {
	if source.Name == "" || source.Kind == "" {
		return nil, errors.New("discovery source name and kind are required")
	}
	now := encodeTime(time.Now())
	_, err := c.db.ExecContext(ctx, `
		INSERT INTO discovery_sources (name, kind, definition, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (name) DO UPDATE SET kind=excluded.kind, definition=excluded.definition,
			enabled=excluded.enabled, updated_at=excluded.updated_at
	`, source.Name, source.Kind, source.Definition, boolToInt(source.Enabled), now, now)
	if err != nil {
		return nil, fmt.Errorf("save discovery source: %w", err)
	}
	return c.GetDiscoverySource(ctx, source.Name)
}

// GetDiscoverySource returns a named source or nil.
func (c *Corpus) GetDiscoverySource(ctx context.Context, name string) (*DiscoverySource, error) {
	var source DiscoverySource
	var enabled int
	var created, updated int64
	err := c.db.QueryRowContext(ctx, `
		SELECT id, name, kind, definition, enabled, created_at, updated_at
		FROM discovery_sources WHERE name=?
	`, name).Scan(&source.ID, &source.Name, &source.Kind, &source.Definition, &enabled, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get discovery source: %w", err)
	}
	source.Enabled = enabled != 0
	source.CreatedAt = scanTime(created)
	source.UpdatedAt = scanTime(updated)
	return &source, nil
}

// ListDiscoverySources returns all sources in stable name order.
func (c *Corpus) ListDiscoverySources(ctx context.Context) ([]DiscoverySource, error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT id, name, kind, definition, enabled, created_at, updated_at
		FROM discovery_sources ORDER BY name LIMIT 1000
	`)
	if err != nil {
		return nil, fmt.Errorf("list discovery sources: %w", err)
	}
	defer rows.Close()
	var sources []DiscoverySource
	for rows.Next() {
		var source DiscoverySource
		var enabled int
		var created, updated int64
		if err := rows.Scan(&source.ID, &source.Name, &source.Kind, &source.Definition, &enabled, &created, &updated); err != nil {
			return nil, err
		}
		source.Enabled = enabled != 0
		source.CreatedAt = scanTime(created)
		source.UpdatedAt = scanTime(updated)
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

// RecordSourcePartition upserts the latest observation for one stable window.
func (c *Corpus) RecordSourcePartition(ctx context.Context, partition SourcePartition) error {
	if partition.SourceID == 0 || partition.Key == "" {
		return errors.New("source partition identity is required")
	}
	_, err := c.db.ExecContext(ctx, `
		INSERT INTO source_partitions (source_id, partition_key, query, qualifier, start_at, end_at,
			total_count, incomplete_results, unsplittable, retries, observed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (source_id, partition_key) DO UPDATE SET query=excluded.query,
			total_count=excluded.total_count, incomplete_results=excluded.incomplete_results,
			unsplittable=excluded.unsplittable, retries=excluded.retries, observed_at=excluded.observed_at
	`, partition.SourceID, partition.Key, partition.Query, partition.Qualifier,
		encodeTime(partition.Start), encodeTime(partition.End), partition.Total,
		boolToInt(partition.Incomplete), boolToInt(partition.Unsplittable), partition.Retries,
		encodeTime(partition.ObservedAt))
	if err != nil {
		return fmt.Errorf("record source partition: %w", err)
	}
	return nil
}

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
