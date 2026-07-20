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
	// SearchText is optional product-selected untrusted text. Callers collapse
	// transport pages into one semantic search document.
	SearchText string
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
	_, err := c.applyFacetObservationSet(ctx, repoID, threadID, facet, sourceUpdatedAt, pages, complete, runID, nil)
	return err
}

// ApplyFacetObservationSetIfNewer records a facet snapshot and reports whether
// it won the stored source ordering. Callers that maintain a derived projection
// must update it only when applied is true.
func (c *Corpus) ApplyFacetObservationSetIfNewer(ctx context.Context, repoID int64, threadID *int64, facet string, sourceUpdatedAt time.Time, pages []FacetObservationInput, complete bool, runID int64) (applied bool, err error) {
	return c.applyFacetObservationSet(ctx, repoID, threadID, facet, sourceUpdatedAt, pages, complete, runID, nil)
}

// ApplyFacetObservationSetCAS atomically replaces a facet only when its current
// coverage sequence still matches the sequence captured before retrieval.
func (c *Corpus) ApplyFacetObservationSetCAS(ctx context.Context, repoID int64, threadID *int64, facet string, sourceUpdatedAt time.Time, pages []FacetObservationInput, complete bool, runID, expectedSequence int64) (bool, error) {
	return c.applyFacetObservationSet(ctx, repoID, threadID, facet, sourceUpdatedAt, pages, complete, runID, &expectedSequence)
}

func (c *Corpus) applyFacetObservationSet(ctx context.Context, repoID int64, threadID *int64, facet string, sourceUpdatedAt time.Time, pages []FacetObservationInput, complete bool, runID int64, expectedSequence *int64) (bool, error) {
	latest := sourceUpdatedAt
	for _, p := range pages {
		if p.SourceUpdatedAt.After(latest) {
			latest = p.SourceUpdatedAt
		}
	}
	srcSec := encodeTime(latest)
	if srcSec == 0 {
		return false, nil
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin facet observation set: %w", err)
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
		existingSeq = 0
		replace = true
	} else if err != nil {
		return false, fmt.Errorf("select facet coverage: %w", err)
	} else if srcSec > existingSrc {
		replace = true
	} else if srcSec == existingSrc && complete {
		replace = true
	}
	if expectedSequence != nil && existingSeq != *expectedSequence {
		return false, nil
	}

	if !replace {
		return false, nil
	}

	now := encodeTime(time.Now())

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM facet_observations
		WHERE repository_id = ? AND COALESCE(thread_id, -1) = COALESCE(?, -1) AND facet = ?
	`, repoID, tid, facet); err != nil {
		return false, fmt.Errorf("delete facet observations: %w", err)
	}

	for _, p := range pages {
		if _, err := c.applyFacetObservationTx(ctx, tx, repoID, threadID, facet, p.SourceUpdatedAt, p.Payload, p.SearchText, now); err != nil {
			return false, err
		}
	}

	seq, err := c.nextSequence(ctx, tx)
	if err != nil {
		return false, err
	}
	if err := c.advanceFacetTx(ctx, tx, repoID, threadID, facet, latest, complete, runID, seq, now); err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit facet observation set: %w", err)
	}
	return true, nil
}

func (c *Corpus) applyFacetObservationTx(ctx context.Context, tx *sql.Tx, repoID int64, threadID *int64, facet string, sourceUpdatedAt time.Time, payload, searchText string, observedAt int64) (*FacetObservation, error) {
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
		INSERT INTO facet_observations (repository_id, thread_id, facet, source_updated_at, observation_sequence, payload, search_text, observed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, repoID, tid, facet, srcSec, seq, payload, searchText, observedAt)
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
	observations, _, err := c.listFacetObservations(ctx, repoID, threadID, facet, 0)
	return observations, err
}

// ListFacetObservationsBounded returns at most limit immutable observations
// and reports whether additional stored pages exist. Ordering matches
// ListFacetObservations. It lets offline readers enforce a memory bound before
// payload decoding.
func (c *Corpus) ListFacetObservationsBounded(ctx context.Context, repoID int64, threadID *int64, facet string, limit int) ([]FacetObservation, bool, error) {
	if limit <= 0 || limit > 1000 {
		return nil, false, errors.New("facet observation limit must be between 1 and 1000")
	}
	return c.listFacetObservations(ctx, repoID, threadID, facet, limit)
}

func (c *Corpus) listFacetObservations(ctx context.Context, repoID int64, threadID *int64, facet string, limit int) ([]FacetObservation, bool, error) {
	tid := sql.NullInt64{}
	if threadID != nil {
		tid.Int64 = *threadID
		tid.Valid = true
	}
	statement := `
		SELECT id, repository_id, thread_id, facet, source_updated_at, observation_sequence, payload, observed_at
		FROM facet_observations
		WHERE repository_id = ? AND COALESCE(thread_id, -1) = COALESCE(?, -1) AND facet = ?
		ORDER BY observation_sequence`
	args := []any{repoID, tid, facet}
	if limit > 0 {
		statement += ` LIMIT ?`
		args = append(args, limit+1)
	}
	rows, err := c.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, false, fmt.Errorf("list facet observations: %w", err)
	}
	defer rows.Close()

	var out []FacetObservation
	for rows.Next() {
		var o FacetObservation
		var body sql.NullInt64
		var src, observed int64
		if err := rows.Scan(&o.ID, &o.RepositoryID, &body, &o.Facet, &src, &o.ObservationSequence, &o.Payload, &observed); err != nil {
			return nil, false, err
		}
		if body.Valid {
			o.ThreadID = &body.Int64
		}
		o.SourceUpdatedAt = scanTime(src)
		o.ObservedAt = scanTime(observed)
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := limit > 0 && len(out) > limit
	if hasMore {
		out = out[:limit]
	}
	return out, hasMore, nil
}
