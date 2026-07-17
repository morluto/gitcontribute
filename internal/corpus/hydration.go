package corpus

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ApplyFacetObservation records an immutable facet observation and returns
// the stored observation. The facet is identified by repository, optional
// thread, and a facet name such as "issue_comments" or "pr_reviews".
func (c *Corpus) ApplyFacetObservation(ctx context.Context, repoID int64, threadID *int64, facet string, sourceUpdatedAt time.Time, payload string) (*FacetObservation, error) {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin facet observation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := encodeTime(time.Now())
	seq, err := c.nextSequence(ctx, tx)
	if err != nil {
		return nil, err
	}

	tid := sql.NullInt64{}
	if threadID != nil {
		tid.Int64 = *threadID
		tid.Valid = true
	}
	srcSec := encodeTime(sourceUpdatedAt)

	res, err := tx.ExecContext(ctx, `
		INSERT INTO facet_observations (repository_id, thread_id, facet, source_updated_at, observation_sequence, payload, observed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, repoID, tid, facet, srcSec, seq, payload, now)
	if err != nil {
		return nil, fmt.Errorf("insert facet observation: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last facet observation id: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit facet observation: %w", err)
	}

	return &FacetObservation{
		ID:                  id,
		RepositoryID:        repoID,
		ThreadID:            threadID,
		Facet:               facet,
		SourceUpdatedAt:     sourceUpdatedAt,
		ObservationSequence: seq,
		Payload:             payload,
		ObservedAt:          scanTime(now),
	}, nil
}

// ListFacetObservations returns immutable observations for a facet, ordered by
// observation sequence.
func (c *Corpus) ListFacetObservations(ctx context.Context, repoID int64, threadID *int64, facet string) ([]FacetObservation, error) {
	tid := sql.NullInt64{}
	if threadID != nil {
		tid.Int64 = *threadID
		tid.Valid = true
	}
	rows, err := c.db.QueryContext(ctx, `
		SELECT id, repository_id, thread_id, facet, source_updated_at, observation_sequence, payload, observed_at
		FROM facet_observations
		WHERE repository_id = ? AND COALESCE(thread_id, -1) = COALESCE(?, -1) AND facet = ?
		ORDER BY observation_sequence
	`, repoID, tid, facet)
	if err != nil {
		return nil, fmt.Errorf("list facet observations: %w", err)
	}
	defer rows.Close()

	var out []FacetObservation
	for rows.Next() {
		var o FacetObservation
		var body sql.NullInt64
		var src, observed int64
		if err := rows.Scan(&o.ID, &o.RepositoryID, &body, &o.Facet, &src, &o.ObservationSequence, &o.Payload, &observed); err != nil {
			return nil, err
		}
		if body.Valid {
			o.ThreadID = &body.Int64
		}
		o.SourceUpdatedAt = scanTime(src)
		o.ObservedAt = scanTime(observed)
		out = append(out, o)
	}
	return out, rows.Err()
}
