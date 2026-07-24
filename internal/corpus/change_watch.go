package corpus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ChangeWatch detects commits made while a multi-read application snapshot is
// assembled. It owns a dedicated SQLite connection because data_version only
// changes for commits made by other connections.
type ChangeWatch struct {
	db       *sql.DB
	baseline int64
}

func corpusWatchDSN(path string) string {
	_, dsn, inspectable, err := schemaInspectionTarget(path)
	if err != nil || !inspectable {
		return ""
	}
	return dsn
}

// BeginChangeWatch captures the database revision on an independent read-only
// connection and keeps that connection alive until Close.
func (c *Corpus) BeginChangeWatch(ctx context.Context) (*ChangeWatch, error) {
	if c == nil || c.watchDSN == "" {
		return nil, errors.New("consistent change watch requires a file-backed corpus")
	}
	db, err := sql.Open("sqlite", c.watchDSN)
	if err != nil {
		return nil, fmt.Errorf("open corpus change watch: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	var baseline int64
	if err := db.QueryRowContext(ctx, `PRAGMA data_version`).Scan(&baseline); err != nil {
		return nil, errors.Join(fmt.Errorf("read corpus change version: %w", err), db.Close())
	}
	return &ChangeWatch{db: db, baseline: baseline}, nil
}

// Unchanged reports whether no other connection committed after the watch
// began.
func (w *ChangeWatch) Unchanged(ctx context.Context) (bool, error) {
	if w == nil || w.db == nil {
		return false, errors.New("corpus change watch is closed")
	}
	var current int64
	if err := w.db.QueryRowContext(ctx, `PRAGMA data_version`).Scan(&current); err != nil {
		return false, fmt.Errorf("read corpus change version: %w", err)
	}
	return current == w.baseline, nil
}

// Close releases the dedicated watcher connection.
func (w *ChangeWatch) Close() error {
	if w == nil || w.db == nil {
		return nil
	}
	db := w.db
	w.db = nil
	return db.Close()
}
