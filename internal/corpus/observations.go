package corpus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ApplyRepositoryObservation records an immutable repository observation and
// updates the current projection only when the new observation wins the
// ordering (source_updated_at, then observation_sequence).
func (c *Corpus) ApplyRepositoryObservation(ctx context.Context, owner, name, externalID string, sourceUpdatedAt time.Time, payload string) (*Repository, error) {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin repository observation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Unix()
	seq, err := c.nextSequence(ctx, tx)
	if err != nil {
		return nil, err
	}

	var repoID int64
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM repositories WHERE owner = ? AND name = ?
	`, owner, name).Scan(&repoID)
	if errors.Is(err, sql.ErrNoRows) {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO repositories (owner, name, external_id, source_updated_at, observation_sequence, created_at, updated_at)
			VALUES (?, ?, ?, 0, 0, ?, ?)
		`, owner, name, externalID, now, now)
		if err != nil {
			return nil, fmt.Errorf("insert repository: %w", err)
		}
		repoID, err = res.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("last repository id: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("select repository: %w", err)
	}

	srcSec := sourceUpdatedAt.Unix()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO repository_observations (repository_id, source_updated_at, observation_sequence, payload, observed_at)
		VALUES (?, ?, ?, ?, ?)
	`, repoID, srcSec, seq, payload, now); err != nil {
		return nil, fmt.Errorf("insert repository observation: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE repositories
		SET source_updated_at = ?,
		    observation_sequence = ?,
		    external_id = ?,
		    updated_at = ?
		WHERE id = ?
		  AND (source_updated_at < ? OR (source_updated_at = ? AND observation_sequence < ?))
	`, srcSec, seq, externalID, now, repoID, srcSec, srcSec, seq); err != nil {
		return nil, fmt.Errorf("update repository projection: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit repository observation: %w", err)
	}
	return c.GetRepository(ctx, owner, name)
}

// GetRepository returns the current projection of a repository, or nil if it
// has not been observed.
func (c *Corpus) GetRepository(ctx context.Context, owner, name string) (*Repository, error) {
	var r Repository
	var src, created, updated int64
	err := c.db.QueryRowContext(ctx, `
		SELECT id, owner, name, external_id, source_updated_at, observation_sequence, created_at, updated_at
		FROM repositories
		WHERE owner = ? AND name = ?
	`, owner, name).Scan(&r.ID, &r.Owner, &r.Name, &r.ExternalID, &src, &r.ObservationSequence, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get repository: %w", err)
	}
	r.SourceUpdatedAt = scanTime(src)
	r.CreatedAt = scanTime(created)
	r.UpdatedAt = scanTime(updated)
	return &r, nil
}

// ListRepositoryObservations returns immutable observations for a repository
// in insertion order.
func (c *Corpus) ListRepositoryObservations(ctx context.Context, repoID int64) ([]RepositoryObservation, error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT id, repository_id, source_updated_at, observation_sequence, payload, observed_at
		FROM repository_observations
		WHERE repository_id = ?
		ORDER BY id
	`, repoID)
	if err != nil {
		return nil, fmt.Errorf("list repository observations: %w", err)
	}
	defer rows.Close()

	var out []RepositoryObservation
	for rows.Next() {
		var o RepositoryObservation
		var src, observed int64
		if err := rows.Scan(&o.ID, &o.RepositoryID, &src, &o.ObservationSequence, &o.Payload, &observed); err != nil {
			return nil, err
		}
		o.SourceUpdatedAt = scanTime(src)
		o.ObservedAt = scanTime(observed)
		out = append(out, o)
	}
	return out, rows.Err()
}

// ApplyThreadObservation records an immutable thread observation and updates
// the current projection only when the new observation wins the ordering.
func (c *Corpus) ApplyThreadObservation(ctx context.Context, repoID int64, kind string, number int, state, title, body, author string, sourceUpdatedAt time.Time, payload string) (*Thread, error) {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin thread observation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Unix()
	seq, err := c.nextSequence(ctx, tx)
	if err != nil {
		return nil, err
	}

	var threadID int64
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM threads WHERE repository_id = ? AND kind = ? AND number = ?
	`, repoID, kind, number).Scan(&threadID)
	if errors.Is(err, sql.ErrNoRows) {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO threads (repository_id, kind, number, state, title, body, author, source_updated_at, observation_sequence, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, 0, 0, ?, ?)
		`, repoID, kind, number, state, title, body, author, now, now)
		if err != nil {
			return nil, fmt.Errorf("insert thread: %w", err)
		}
		threadID, err = res.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("last thread id: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("select thread: %w", err)
	}

	srcSec := sourceUpdatedAt.Unix()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO thread_observations (thread_id, source_updated_at, observation_sequence, payload, observed_at)
		VALUES (?, ?, ?, ?, ?)
	`, threadID, srcSec, seq, payload, now); err != nil {
		return nil, fmt.Errorf("insert thread observation: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE threads
		SET state = ?,
		    title = ?,
		    body = ?,
		    author = ?,
		    source_updated_at = ?,
		    observation_sequence = ?,
		    updated_at = ?
		WHERE id = ?
		  AND (source_updated_at < ? OR (source_updated_at = ? AND observation_sequence < ?))
	`, state, title, body, author, srcSec, seq, now, threadID, srcSec, srcSec, seq); err != nil {
		return nil, fmt.Errorf("update thread projection: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit thread observation: %w", err)
	}
	return c.GetThread(ctx, repoID, kind, number)
}

// GetThread returns the current projection of a thread, or nil if it has not
// been observed.
func (c *Corpus) GetThread(ctx context.Context, repoID int64, kind string, number int) (*Thread, error) {
	var t Thread
	var body sql.NullString
	var author sql.NullString
	var src, created, updated int64
	err := c.db.QueryRowContext(ctx, `
		SELECT id, repository_id, kind, number, state, title, body, author, source_updated_at, observation_sequence, created_at, updated_at
		FROM threads
		WHERE repository_id = ? AND kind = ? AND number = ?
	`, repoID, kind, number).Scan(&t.ID, &t.RepositoryID, &t.Kind, &t.Number, &t.State, &t.Title, &body, &author, &src, &t.ObservationSequence, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get thread: %w", err)
	}
	t.Body = body.String
	t.Author = author.String
	t.SourceUpdatedAt = scanTime(src)
	t.CreatedAt = scanTime(created)
	t.UpdatedAt = scanTime(updated)
	return &t, nil
}

// ListThreadObservations returns immutable observations for a thread in
// insertion order.
func (c *Corpus) ListThreadObservations(ctx context.Context, threadID int64) ([]ThreadObservation, error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT id, thread_id, source_updated_at, observation_sequence, payload, observed_at
		FROM thread_observations
		WHERE thread_id = ?
		ORDER BY id
	`, threadID)
	if err != nil {
		return nil, fmt.Errorf("list thread observations: %w", err)
	}
	defer rows.Close()

	var out []ThreadObservation
	for rows.Next() {
		var o ThreadObservation
		var src, observed int64
		if err := rows.Scan(&o.ID, &o.ThreadID, &src, &o.ObservationSequence, &o.Payload, &observed); err != nil {
			return nil, err
		}
		o.SourceUpdatedAt = scanTime(src)
		o.ObservedAt = scanTime(observed)
		out = append(out, o)
	}
	return out, rows.Err()
}
