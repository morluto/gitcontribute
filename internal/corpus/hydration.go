package corpus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// FacetObservationInput is an unpersisted facet observation page.
type FacetObservationInput struct {
	SourceUpdatedAt time.Time
	Payload         string
}

// ApplyFacetObservationSet records a complete ordered set of facet observations
// and advances coverage for the facet in a single transaction. The existing
// facet observations are replaced only when the new set wins the
// (source_updated_at, observation_sequence) ordering, so an interrupted or
// stale fetch leaves previous complete data in place.
//
// sourceUpdatedAt is the authoritative source timestamp for the set as a whole.
// It is used when the set is empty and also combined with the per-page timestamps
// so the latest source timestamp always controls the ordering. Callers should
// pass the most recent source timestamp available for the facet (for example,
// the latest item update time, falling back to the thread's source_updated_at).
func (c *Corpus) ApplyFacetObservationSet(ctx context.Context, repoID int64, threadID *int64, facet string, sourceUpdatedAt time.Time, pages []FacetObservationInput, complete bool, runID int64) error {
	latest := sourceUpdatedAt
	for _, p := range pages {
		if p.SourceUpdatedAt.After(latest) {
			latest = p.SourceUpdatedAt
		}
	}
	srcSec := encodeTime(latest)
	if srcSec == 0 {
		return nil
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin facet observation set: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	tid := sql.NullInt64{}
	if threadID != nil {
		tid.Int64 = *threadID
		tid.Valid = true
	}

	var replace bool
	var existingSrc, existingSeq int64
	err = tx.QueryRowContext(ctx, `
		SELECT source_updated_at, observation_sequence
		FROM facet_coverage
		WHERE repository_id = ? AND COALESCE(thread_id, -1) = COALESCE(?, -1) AND facet = ?
	`, repoID, tid, facet).Scan(&existingSrc, &existingSeq)
	if errors.Is(err, sql.ErrNoRows) {
		replace = true
	} else if err != nil {
		return fmt.Errorf("select facet coverage: %w", err)
	} else if srcSec > existingSrc {
		replace = true
	} else if srcSec == existingSrc && complete {
		replace = true
	}

	if !replace {
		return nil
	}

	now := encodeTime(time.Now())

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM facet_observations
		WHERE repository_id = ? AND COALESCE(thread_id, -1) = COALESCE(?, -1) AND facet = ?
	`, repoID, tid, facet); err != nil {
		return fmt.Errorf("delete facet observations: %w", err)
	}

	for _, p := range pages {
		if _, err := c.applyFacetObservationTx(ctx, tx, repoID, threadID, facet, p.SourceUpdatedAt, p.Payload, now); err != nil {
			return err
		}
	}

	seq, err := c.nextSequence(ctx, tx)
	if err != nil {
		return err
	}
	if err := c.advanceFacetTx(ctx, tx, repoID, threadID, facet, latest, complete, runID, seq, now); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit facet observation set: %w", err)
	}
	return nil
}

func (c *Corpus) applyFacetObservationTx(ctx context.Context, tx *sql.Tx, repoID int64, threadID *int64, facet string, sourceUpdatedAt time.Time, payload string, observedAt int64) (*FacetObservation, error) {
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
	`, repoID, tid, facet, srcSec, seq, payload, observedAt)
	if err != nil {
		return nil, fmt.Errorf("insert facet observation: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last facet observation id: %w", err)
	}

	return &FacetObservation{
		ID:                  id,
		RepositoryID:        repoID,
		ThreadID:            threadID,
		Facet:               facet,
		SourceUpdatedAt:     sourceUpdatedAt,
		ObservationSequence: seq,
		Payload:             payload,
		ObservedAt:          scanTime(observedAt),
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
