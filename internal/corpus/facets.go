package corpus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// AdvanceFacet records progress on a hydration facet for a repository or thread.
// The update wins only when the new (source_updated_at, observation_sequence)
// ordering is greater, so facets advance independently from one another and
// from the parent projection.
func (c *Corpus) AdvanceFacet(ctx context.Context, repoID int64, threadID *int64, facet string, sourceUpdatedAt time.Time, complete bool, runID int64) error {
	_, err := c.advanceFacet(ctx, repoID, threadID, facet, sourceUpdatedAt, complete, runID, nil)
	return err
}

// AdvanceFacetCAS advances coverage only when the facet sequence captured
// before retrieval is still current.
func (c *Corpus) AdvanceFacetCAS(ctx context.Context, repoID int64, threadID *int64, facet string, sourceUpdatedAt time.Time, complete bool, runID, expectedSequence int64) (bool, error) {
	return c.advanceFacet(ctx, repoID, threadID, facet, sourceUpdatedAt, complete, runID, &expectedSequence)
}

func (c *Corpus) advanceFacet(ctx context.Context, repoID int64, threadID *int64, facet string, sourceUpdatedAt time.Time, complete bool, runID int64, expectedSequence *int64) (bool, error) {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin advance facet: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if expectedSequence != nil {
		tid := sql.NullInt64{}
		if threadID != nil {
			tid.Int64, tid.Valid = *threadID, true
		}
		var currentSequence int64
		err := tx.QueryRowContext(ctx, `SELECT observation_sequence FROM facet_coverage WHERE repository_id=? AND COALESCE(thread_id, -1)=COALESCE(?, -1) AND facet=?`, repoID, tid, facet).Scan(&currentSequence)
		if errors.Is(err, sql.ErrNoRows) {
			currentSequence = 0
		} else if err != nil {
			return false, fmt.Errorf("read facet CAS sequence: %w", err)
		}
		if currentSequence != *expectedSequence {
			return false, nil
		}
	}

	now := encodeTime(time.Now())
	seq, err := c.nextSequence(ctx, tx)
	if err != nil {
		return false, err
	}

	if err := c.advanceFacetTx(ctx, tx, repoID, threadID, facet, sourceUpdatedAt, complete, runID, seq, now); err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit advance facet: %w", err)
	}
	return true, nil
}

func (c *Corpus) advanceFacetTx(ctx context.Context, tx *sql.Tx, repoID int64, threadID *int64, facet string, sourceUpdatedAt time.Time, complete bool, runID int64, seq int64, now int64) error {
	tid := sql.NullInt64{}
	if threadID != nil {
		tid.Int64 = *threadID
		tid.Valid = true
	}
	rid := sql.NullInt64{}
	if runID != 0 {
		rid.Int64 = runID
		rid.Valid = true
	}

	var existing struct {
		id  int64
		src int64
		seq int64
	}
	err := tx.QueryRowContext(ctx, `
		SELECT id, source_updated_at, observation_sequence
		FROM facet_coverage
		WHERE repository_id = ? AND COALESCE(thread_id, -1) = COALESCE(?, -1) AND facet = ?
	`, repoID, tid, facet).Scan(&existing.id, &existing.src, &existing.seq)

	srcSec := encodeTime(sourceUpdatedAt)
	completeInt := 0
	if complete {
		completeInt = 1
	}

	if errors.Is(err, sql.ErrNoRows) {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO facet_coverage (repository_id, thread_id, facet, source_updated_at, observation_sequence, complete, run_id, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, repoID, tid, facet, srcSec, seq, completeInt, rid, now); err != nil {
			return fmt.Errorf("insert facet coverage: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("select facet coverage: %w", err)
	} else {
		if srcSec > existing.src || (srcSec == existing.src && seq > existing.seq) {
			if _, err := tx.ExecContext(ctx, `
				UPDATE facet_coverage
				SET source_updated_at = ?,
				    observation_sequence = ?,
				    complete = ?,
				    run_id = ?,
				    updated_at = ?
				WHERE id = ?
			`, srcSec, seq, completeInt, rid, now, existing.id); err != nil {
				return fmt.Errorf("update facet coverage: %w", err)
			}
		}
	}
	return nil
}

// GetCoverage returns the coverage fact for a single facet.
func (c *Corpus) GetCoverage(ctx context.Context, repoID int64, threadID *int64, facet string) (*Coverage, error) {
	tid := sql.NullInt64{}
	if threadID != nil {
		tid.Int64 = *threadID
		tid.Valid = true
	}
	var cov Coverage
	var runID, body sql.NullInt64
	var src, updated int64
	err := c.db.QueryRowContext(ctx, `
		SELECT id, repository_id, thread_id, facet, source_updated_at, observation_sequence, complete, run_id, updated_at
		FROM facet_coverage
		WHERE repository_id = ? AND COALESCE(thread_id, -1) = COALESCE(?, -1) AND facet = ?
	`, repoID, tid, facet).Scan(&cov.ID, &cov.RepositoryID, &body, &cov.Facet, &src, &cov.ObservationSequence, &cov.Complete, &runID, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get coverage: %w", err)
	}
	if body.Valid {
		cov.ThreadID = &body.Int64
	}
	if runID.Valid {
		cov.RunID = &runID.Int64
	}
	cov.SourceUpdatedAt = scanTime(src)
	cov.UpdatedAt = scanTime(updated)
	return &cov, nil
}

// ListCoverage returns all coverage facts for a repository or thread.
func (c *Corpus) ListCoverage(ctx context.Context, repoID int64, threadID *int64) ([]Coverage, error) {
	tid := sql.NullInt64{}
	if threadID != nil {
		tid.Int64 = *threadID
		tid.Valid = true
	}
	rows, err := c.db.QueryContext(ctx, `
		SELECT id, repository_id, thread_id, facet, source_updated_at, observation_sequence, complete, run_id, updated_at
		FROM facet_coverage
		WHERE repository_id = ? AND COALESCE(thread_id, -1) = COALESCE(?, -1)
		ORDER BY facet
	`, repoID, tid)
	if err != nil {
		return nil, fmt.Errorf("list coverage: %w", err)
	}
	defer rows.Close()

	var out []Coverage
	for rows.Next() {
		var cov Coverage
		var runID, body sql.NullInt64
		var src, updated int64
		if err := rows.Scan(&cov.ID, &cov.RepositoryID, &body, &cov.Facet, &src, &cov.ObservationSequence, &cov.Complete, &runID, &updated); err != nil {
			return nil, err
		}
		if body.Valid {
			cov.ThreadID = &body.Int64
		}
		if runID.Valid {
			cov.RunID = &runID.Int64
		}
		cov.SourceUpdatedAt = scanTime(src)
		cov.UpdatedAt = scanTime(updated)
		out = append(out, cov)
	}
	return out, rows.Err()
}
