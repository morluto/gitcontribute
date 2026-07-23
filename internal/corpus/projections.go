package corpus

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"strconv"
	"time"
)

// Product-owned names for derived SQLite search projections.
const (
	ProjectionNameThreadsFTS           = "threads_fts"
	ProjectionNameRepositoriesFTS      = "repositories_fts"
	ProjectionNameFacetObservationsFTS = "facet_observations_fts"
	ProjectionNameCodeDocumentsFTS     = "code_documents_fts"
)

// Product-owned versions for derived SQLite search projections.
const (
	ProjectionVersionThreadsFTS           = "threads-fts-v3"
	ProjectionVersionRepositoriesFTS      = "repositories-fts-v1"
	ProjectionVersionFacetObservationsFTS = "facet-observations-fts-v1"
	ProjectionVersionCodeDocumentsFTS     = "code-documents-fts-v1"
)

// ProjectionStatus describes the durability state of a derived projection.
type ProjectionStatus string

const (
	// ProjectionStatusAbsent indicates that a derived projection is not built.
	ProjectionStatusAbsent ProjectionStatus = "absent"
	// ProjectionStatusBuilding indicates that a projection build is in progress.
	ProjectionStatusBuilding ProjectionStatus = "building"
	// ProjectionStatusCurrent indicates that the projection matches its source.
	ProjectionStatusCurrent ProjectionStatus = "current"
	// ProjectionStatusStale indicates that the projection source has changed.
	ProjectionStatusStale ProjectionStatus = "stale"
	// ProjectionStatusFailed indicates that the latest projection build failed.
	ProjectionStatusFailed ProjectionStatus = "failed"
)

// ProjectionAttemptStatus describes the most recent explicit rebuild attempt.
type ProjectionAttemptStatus string

const (
	// ProjectionAttemptNone indicates that no explicit rebuild has been attempted.
	ProjectionAttemptNone ProjectionAttemptStatus = ""
	// ProjectionAttemptBuilding indicates that a rebuild is in progress.
	ProjectionAttemptBuilding ProjectionAttemptStatus = "building"
	// ProjectionAttemptSucceeded indicates that a rebuild completed successfully.
	ProjectionAttemptSucceeded ProjectionAttemptStatus = "succeeded"
	// ProjectionAttemptFailed indicates that a rebuild failed.
	ProjectionAttemptFailed ProjectionAttemptStatus = "failed"
)

// ProjectionState is the durable identity and freshness of a derived SQLite
// projection. It is owned by the corpus and read by offline search readers.
type ProjectionState struct {
	Name              string
	Version           string
	Status            ProjectionStatus
	RefreshedAt       time.Time
	RowCount          int64
	SourceRevision    string
	ContentHash       string
	AttemptStatus     ProjectionAttemptStatus
	AttemptStartedAt  time.Time
	AttemptFinishedAt time.Time
	AttemptError      string
}

// Projection errors.
var (
	ErrProjectionNotFound = errors.New("projection state not found")
	ErrProjectionStale    = errors.New("search projection is stale or missing")
)

// RequireProjection verifies that a read can consume the named derived
// projection. It never rebuilds or mutates projection state.
func (c *Corpus) RequireProjection(ctx context.Context, name, version string) error {
	state, err := c.GetProjectionState(ctx, name)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrProjectionStale, name, err)
	}
	if state.Version != version || state.RefreshedAt.IsZero() ||
		(state.Status != ProjectionStatusCurrent && state.Status != ProjectionStatusBuilding && state.Status != ProjectionStatusFailed) {
		return fmt.Errorf("%w: %s is %s at version %s; expected current version %s", ErrProjectionStale, name, state.Status, state.Version, version)
	}
	return nil
}

// GetProjectionState returns the durable state for one derived projection.
func (c *Corpus) GetProjectionState(ctx context.Context, name string) (ProjectionState, error) {
	var state ProjectionState
	var refreshed, attemptStarted, attemptFinished sql.NullInt64
	err := c.db.QueryRowContext(ctx, `
		SELECT name, version, status, refreshed_at, row_count,
		       source_revision, content_hash, attempt_status,
		       attempt_started_at, attempt_finished_at, attempt_error
		FROM projection_states
		WHERE name = ?
	`, name).Scan(&state.Name, &state.Version, &state.Status, &refreshed, &state.RowCount,
		&state.SourceRevision, &state.ContentHash, &state.AttemptStatus,
		&attemptStarted, &attemptFinished, &state.AttemptError)
	if errors.Is(err, sql.ErrNoRows) {
		if isSearchProjection(name) {
			return ProjectionState{Name: name, Status: ProjectionStatusAbsent}, nil
		}
		return ProjectionState{}, fmt.Errorf("%w: %s", ErrProjectionNotFound, name)
	}
	if err != nil {
		return ProjectionState{}, fmt.Errorf("get projection state %s: %w", name, err)
	}
	setProjectionTimes(&state, refreshed, attemptStarted, attemptFinished)
	return state, nil
}

// ListProjectionStates returns all durable derived-projection states.
func (c *Corpus) ListProjectionStates(ctx context.Context) (_ []ProjectionState, returnErr error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT name, version, status, refreshed_at, row_count,
		       source_revision, content_hash, attempt_status,
		       attempt_started_at, attempt_finished_at, attempt_error
		FROM projection_states
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("list projection states: %w", err)
	}
	defer func() { returnErr = errors.Join(returnErr, rows.Close()) }()

	var out []ProjectionState
	for rows.Next() {
		var state ProjectionState
		var refreshed, attemptStarted, attemptFinished sql.NullInt64
		if err := rows.Scan(&state.Name, &state.Version, &state.Status, &refreshed, &state.RowCount,
			&state.SourceRevision, &state.ContentHash, &state.AttemptStatus,
			&attemptStarted, &attemptFinished, &state.AttemptError); err != nil {
			return nil, err
		}
		setProjectionTimes(&state, refreshed, attemptStarted, attemptFinished)
		out = append(out, state)
	}
	return out, rows.Err()
}

// RebuildThreadSearchProjection atomically rebuilds the threads_fts index and
// advances the durable projection state. It is explicit: search never calls it.
func (c *Corpus) RebuildThreadSearchProjection(ctx context.Context) (ProjectionState, error) {
	return c.rebuildSearchProjection(ctx, ProjectionNameThreadsFTS, ProjectionVersionThreadsFTS)
}

// RebuildRepositorySearchProjection atomically rebuilds repository search.
func (c *Corpus) RebuildRepositorySearchProjection(ctx context.Context) (ProjectionState, error) {
	return c.rebuildSearchProjection(ctx, ProjectionNameRepositoriesFTS, ProjectionVersionRepositoriesFTS)
}

// RebuildCodeSearchProjection atomically rebuilds the code_documents_fts index
// and advances the durable projection state. It is explicit: search never calls it.
func (c *Corpus) RebuildCodeSearchProjection(ctx context.Context) (ProjectionState, error) {
	return c.rebuildSearchProjection(ctx, ProjectionNameCodeDocumentsFTS, ProjectionVersionCodeDocumentsFTS)
}

// RebuildFacetSearchProjection atomically rebuilds searchable hydrated facet
// evidence. Thread search requires both thread and facet projections.
func (c *Corpus) RebuildFacetSearchProjection(ctx context.Context) (ProjectionState, error) {
	return c.rebuildSearchProjection(ctx, ProjectionNameFacetObservationsFTS, ProjectionVersionFacetObservationsFTS)
}

func (c *Corpus) rebuildSearchProjection(ctx context.Context, name, version string) (result ProjectionState, err error) {
	_, contentHash, err := c.projectionSourceIdentity(ctx, c.db, name)
	if err != nil {
		return ProjectionState{}, err
	}
	state, err := c.GetProjectionState(ctx, name)
	if err != nil {
		return ProjectionState{}, err
	}
	if state.Status == ProjectionStatusCurrent && state.Version == version && state.ContentHash == contentHash {
		return state, nil
	}

	started, err := c.startSearchProjectionRebuild(ctx, name)
	if err != nil {
		return ProjectionState{}, err
	}
	return c.executeSearchProjectionRebuild(ctx, name, version, started)
}

func (c *Corpus) startSearchProjectionRebuild(ctx context.Context, name string) (time.Time, error) {
	started := time.Now().UTC()
	if _, err := c.db.ExecContext(ctx, `
		INSERT INTO projection_states
		    (name, version, status, refreshed_at, row_count, source_revision, content_hash,
		     attempt_status, attempt_started_at, attempt_finished_at, attempt_error)
		VALUES (?, '', ?, NULL, 0, '', '', ?, ?, NULL, '')
		ON CONFLICT(name) DO UPDATE SET
		    status = excluded.status,
		    attempt_status = excluded.attempt_status,
		    attempt_started_at = excluded.attempt_started_at,
		    attempt_finished_at = NULL,
		    attempt_error = ''
	`, name, string(ProjectionStatusBuilding), string(ProjectionAttemptBuilding), encodeTime(started)); err != nil {
		return time.Time{}, fmt.Errorf("mark %s rebuild building: %w", name, err)
	}
	return started, nil
}

func (c *Corpus) executeSearchProjectionRebuild(ctx context.Context, name, version string, started time.Time) (result ProjectionState, err error) {
	failed := true
	defer func() {
		if failed {
			// The caller's context may be cancelled. Recording the failed attempt is
			// best-effort and must not modify the last successful projection fields.
			failureCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_, failureErr := c.db.ExecContext(failureCtx, `
				UPDATE projection_states
				SET status = ?, attempt_status = ?, attempt_finished_at = ?, attempt_error = ?
				WHERE name = ?
			`, string(ProjectionStatusFailed), string(ProjectionAttemptFailed), encodeTime(time.Now().UTC()), projectionAttemptError(err), name)
			err = errors.Join(err, failureErr)
		}
	}()

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return ProjectionState{}, fmt.Errorf("begin %s rebuild: %w", name, err)
	}
	defer rollbackSQLOnReturn(tx, &err)

	sourceRevision, contentHash, err := c.projectionSourceIdentity(ctx, tx, name)
	if err != nil {
		return ProjectionState{}, err
	}

	if _, err := tx.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (%s) VALUES ('rebuild')", name, name)); err != nil {
		return ProjectionState{}, fmt.Errorf("rebuild %s: %w", name, err)
	}

	var rowCount int64
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+name).Scan(&rowCount); err != nil {
		return ProjectionState{}, fmt.Errorf("count %s: %w", name, err)
	}

	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO projection_states
		    (name, version, status, refreshed_at, row_count, source_revision, content_hash,
		     attempt_status, attempt_started_at, attempt_finished_at, attempt_error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '')
		ON CONFLICT(name) DO UPDATE SET
		    version = excluded.version,
		    status = excluded.status,
		    refreshed_at = excluded.refreshed_at,
		    row_count = excluded.row_count,
		    source_revision = excluded.source_revision,
		    content_hash = excluded.content_hash,
		    attempt_status = excluded.attempt_status,
		    attempt_started_at = excluded.attempt_started_at,
		    attempt_finished_at = excluded.attempt_finished_at,
		    attempt_error = ''
	`, name, version, string(ProjectionStatusCurrent), encodeTime(now), rowCount,
		sourceRevision, contentHash, string(ProjectionAttemptSucceeded), encodeTime(started), encodeTime(now)); err != nil {
		return ProjectionState{}, fmt.Errorf("update %s state: %w", name, err)
	}

	if err := tx.Commit(); err != nil {
		return ProjectionState{}, fmt.Errorf("commit %s rebuild: %w", name, err)
	}
	failed = false

	return ProjectionState{
		Name:              name,
		Version:           version,
		Status:            ProjectionStatusCurrent,
		RefreshedAt:       now,
		RowCount:          rowCount,
		SourceRevision:    sourceRevision,
		ContentHash:       contentHash,
		AttemptStatus:     ProjectionAttemptSucceeded,
		AttemptStartedAt:  started,
		AttemptFinishedAt: now,
	}, nil
}

type projectionSourceQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (c *Corpus) projectionSourceIdentity(ctx context.Context, q projectionSourceQuerier, name string) (_ string, _ string, returnErr error) {
	var query string
	switch name {
	case ProjectionNameThreadsFTS:
		query = `SELECT thread_id, title || char(10) || labels, body || char(10) || facets FROM thread_search_documents ORDER BY thread_id`
	case ProjectionNameRepositoriesFTS:
		query = `SELECT id, owner || char(10) || name, COALESCE(topics, '') || char(10) || COALESCE(description, '') FROM repositories ORDER BY id`
	case ProjectionNameFacetObservationsFTS:
		query = `SELECT id, COALESCE(search_text, ''), '' FROM facet_observations ORDER BY id`
	case ProjectionNameCodeDocumentsFTS:
		query = `SELECT id, path, content FROM code_documents ORDER BY id`
	default:
		return "", "", fmt.Errorf("unknown search projection %q", name)
	}
	rows, err := q.QueryContext(ctx, query)
	if err != nil {
		return "", "", fmt.Errorf("read %s source: %w", name, err)
	}
	defer func() { returnErr = errors.Join(returnErr, rows.Close()) }()

	h := sha256.New()
	writeProjectionHashField(h, name)
	for rows.Next() {
		var id int64
		var first, second string
		if err := rows.Scan(&id, &first, &second); err != nil {
			return "", "", fmt.Errorf("hash %s source: %w", name, err)
		}
		writeProjectionHashField(h, strconv.FormatInt(id, 10))
		writeProjectionHashField(h, first)
		writeProjectionHashField(h, second)
	}
	if err := rows.Err(); err != nil {
		return "", "", fmt.Errorf("hash %s source: %w", name, err)
	}
	digest := hex.EncodeToString(h.Sum(nil))
	return "sha256:" + digest, digest, nil
}

func writeProjectionHashField(h hash.Hash, value string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = h.Write(size[:])
	_, _ = h.Write([]byte(value))
}

func isSearchProjection(name string) bool {
	return name == ProjectionNameThreadsFTS || name == ProjectionNameRepositoriesFTS || name == ProjectionNameFacetObservationsFTS || name == ProjectionNameCodeDocumentsFTS
}

func setProjectionTimes(state *ProjectionState, refreshed, attemptStarted, attemptFinished sql.NullInt64) {
	if refreshed.Valid {
		state.RefreshedAt = scanTime(refreshed.Int64)
	}
	if attemptStarted.Valid {
		state.AttemptStartedAt = scanTime(attemptStarted.Int64)
	}
	if attemptFinished.Valid {
		state.AttemptFinishedAt = scanTime(attemptFinished.Int64)
	}
}

func projectionAttemptError(err error) string {
	if err == nil {
		return "projection rebuild failed"
	}
	return err.Error()
}
