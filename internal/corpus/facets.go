package corpus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ThreadFacetKey identifies one thread-scoped hydration facet.
type ThreadFacetKey struct {
	ThreadID int64
	Facet    string
}

// RepositoryFacetKey identifies one repository-scoped hydration facet.
type RepositoryFacetKey struct {
	RepositoryID int64
	Facet        string
}

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

// ListThreadCoverageBatch returns stored coverage for a bounded set of thread
// facets in one query. Missing keys are intentionally absent from the result.
func (c *Corpus) ListThreadCoverageBatch(ctx context.Context, threadIDs []int64, facets []string) (map[ThreadFacetKey]*Coverage, error) {
	if err := validateThreadFacetBatch(threadIDs, facets); err != nil {
		return nil, err
	}
	if len(threadIDs) == 0 || len(facets) == 0 {
		return map[ThreadFacetKey]*Coverage{}, nil
	}
	threadPlaceholders := sqlPlaceholders(len(threadIDs))
	facetPlaceholders := sqlPlaceholders(len(facets))
	args := make([]any, 0, len(threadIDs)+len(facets))
	for _, threadID := range threadIDs {
		args = append(args, threadID)
	}
	for _, facet := range facets {
		args = append(args, facet)
	}
	rows, err := c.db.QueryContext(ctx, `
		SELECT id, repository_id, thread_id, facet, source_updated_at, observation_sequence, complete, run_id, updated_at
		FROM facet_coverage
		WHERE thread_id IN (`+threadPlaceholders+`) AND facet IN (`+facetPlaceholders+`)
		ORDER BY thread_id, facet
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list thread coverage batch: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[ThreadFacetKey]*Coverage)
	for rows.Next() {
		var cov Coverage
		var runID, threadID sql.NullInt64
		var sourceUpdatedAt, updatedAt int64
		if err := rows.Scan(&cov.ID, &cov.RepositoryID, &threadID, &cov.Facet, &sourceUpdatedAt, &cov.ObservationSequence, &cov.Complete, &runID, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan thread coverage batch: %w", err)
		}
		if !threadID.Valid {
			continue
		}
		cov.ThreadID = &threadID.Int64
		if runID.Valid {
			cov.RunID = &runID.Int64
		}
		cov.SourceUpdatedAt = scanTime(sourceUpdatedAt)
		cov.UpdatedAt = scanTime(updatedAt)
		out[ThreadFacetKey{ThreadID: threadID.Int64, Facet: cov.Facet}] = &cov
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate thread coverage batch: %w", err)
	}
	return out, nil
}

// ListRepositoryCoverageBatch returns repository-scoped coverage for a bounded
// set of repository facets in one query. Missing keys are absent.
func (c *Corpus) ListRepositoryCoverageBatch(ctx context.Context, repositoryIDs []int64, facets []string) (map[RepositoryFacetKey]*Coverage, error) {
	if err := validateFacetBatch(repositoryIDs, facets, "repository"); err != nil {
		return nil, err
	}
	if len(repositoryIDs) == 0 || len(facets) == 0 {
		return map[RepositoryFacetKey]*Coverage{}, nil
	}
	repositoryPlaceholders := sqlPlaceholders(len(repositoryIDs))
	facetPlaceholders := sqlPlaceholders(len(facets))
	args := make([]any, 0, len(repositoryIDs)+len(facets))
	for _, repositoryID := range repositoryIDs {
		args = append(args, repositoryID)
	}
	for _, facet := range facets {
		args = append(args, facet)
	}
	rows, err := c.db.QueryContext(ctx, `
		SELECT id, repository_id, thread_id, facet, source_updated_at, observation_sequence, complete, run_id, updated_at
		FROM facet_coverage
		WHERE repository_id IN (`+repositoryPlaceholders+`)
		  AND thread_id IS NULL
		  AND facet IN (`+facetPlaceholders+`)
		ORDER BY repository_id, facet
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list repository coverage batch: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[RepositoryFacetKey]*Coverage)
	for rows.Next() {
		var cov Coverage
		var runID, threadID sql.NullInt64
		var sourceUpdatedAt, updatedAt int64
		if err := rows.Scan(&cov.ID, &cov.RepositoryID, &threadID, &cov.Facet, &sourceUpdatedAt, &cov.ObservationSequence, &cov.Complete, &runID, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan repository coverage batch: %w", err)
		}
		if runID.Valid {
			cov.RunID = &runID.Int64
		}
		cov.SourceUpdatedAt = scanTime(sourceUpdatedAt)
		cov.UpdatedAt = scanTime(updatedAt)
		out[RepositoryFacetKey{RepositoryID: cov.RepositoryID, Facet: cov.Facet}] = &cov
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate repository coverage batch: %w", err)
	}
	return out, nil
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
	defer func() { _ = rows.Close() }()

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
