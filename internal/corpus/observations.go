package corpus

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ErrThreadObservationRevisionNotFound reports a projection revision whose
// immutable source observation is unavailable.
var ErrThreadObservationRevisionNotFound = errors.New("thread observation revision not found")

// ApplyRepositoryObservation records an immutable repository observation and
// updates the current projection only when the new observation wins the
// ordering (source_updated_at, then observation_sequence).
func (c *Corpus) ApplyRepositoryObservation(ctx context.Context, owner, name, externalID string, sourceUpdatedAt time.Time, payload string) (*Repository, error) {
	repo := Repository{
		Owner:           owner,
		Name:            name,
		ExternalID:      externalID,
		SourceUpdatedAt: sourceUpdatedAt,
	}
	return c.UpsertRepository(ctx, repo, payload)
}

// UpsertRepository records a repository observation and updates the projection
// with all fields when the source ordering is newer.
func (c *Corpus) UpsertRepository(ctx context.Context, repo Repository, payload string) (*Repository, error) {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin repository upsert: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := encodeTime(time.Now())
	seq, err := c.nextSequence(ctx, tx)
	if err != nil {
		return nil, err
	}

	srcSec := encodeTime(repo.SourceUpdatedAt)
	sourceCreated := encodeTime(repo.SourceCreatedAt)

	var repoID int64
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM repositories WHERE owner = ? AND name = ?
	`, repo.Owner, repo.Name).Scan(&repoID)
	if errors.Is(err, sql.ErrNoRows) {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO repositories (owner, name, external_id, description, default_branch, language, license, topics, stars, watchers, forks, open_issues, archived, fork, source_created_at, source_updated_at, observation_sequence, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, repo.Owner, repo.Name, repo.ExternalID, repo.Description, repo.DefaultBranch, repo.Language, repo.License, joinLabels(repo.Topics), repo.Stars, repo.Watchers, repo.Forks, repo.OpenIssues, boolToInt(repo.Archived), boolToInt(repo.Fork), sourceCreated, srcSec, seq, now, now)
		if err != nil {
			return nil, fmt.Errorf("insert repository: %w", err)
		}
		repoID, err = res.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("last repository id: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("select repository: %w", err)
	} else {
		if _, err := tx.ExecContext(ctx, `
			UPDATE repositories
			SET owner = ?,
			    name = ?,
			    external_id = ?,
			    description = ?,
			    default_branch = ?,
			    language = ?,
			    license = ?,
			    topics = ?,
			    stars = ?,
			    watchers = ?,
			    forks = ?,
			    open_issues = ?,
			    archived = ?,
			    fork = ?,
			    source_created_at = ?,
			    source_updated_at = ?,
			    observation_sequence = ?,
			    updated_at = ?
			WHERE id = ?
			  AND (source_updated_at < ? OR (source_updated_at = ? AND observation_sequence < ?))
		`, repo.Owner, repo.Name, repo.ExternalID, repo.Description, repo.DefaultBranch, repo.Language, repo.License, joinLabels(repo.Topics), repo.Stars, repo.Watchers, repo.Forks, repo.OpenIssues, boolToInt(repo.Archived), boolToInt(repo.Fork), sourceCreated, srcSec, seq, now, repoID, srcSec, srcSec, seq); err != nil {
			return nil, fmt.Errorf("update repository projection: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO repository_observations (repository_id, source_updated_at, observation_sequence, payload, observed_at)
		VALUES (?, ?, ?, ?, ?)
	`, repoID, srcSec, seq, payload, now); err != nil {
		return nil, fmt.Errorf("insert repository observation: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit repository upsert: %w", err)
	}
	return c.GetRepository(ctx, repo.Owner, repo.Name)
}

// GetRepository returns the current projection of a repository, or nil if it
// has not been observed.
func (c *Corpus) GetRepository(ctx context.Context, owner, name string) (*Repository, error) {
	var r Repository
	var sourceCreated, src, created, updated int64
	var archived, fork int
	var topics string
	err := c.db.QueryRowContext(ctx, `
		SELECT id, owner, name, external_id, description, default_branch, language, license, topics, stars, watchers, forks, open_issues, archived, fork, source_created_at, source_updated_at, observation_sequence, created_at, updated_at
		FROM repositories
		WHERE owner = ? AND name = ?
	`, owner, name).Scan(&r.ID, &r.Owner, &r.Name, &r.ExternalID, &r.Description, &r.DefaultBranch, &r.Language, &r.License, &topics, &r.Stars, &r.Watchers, &r.Forks, &r.OpenIssues, &archived, &fork, &sourceCreated, &src, &r.ObservationSequence, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get repository: %w", err)
	}
	r.Topics = splitLabels(topics)
	r.Archived = archived != 0
	r.Fork = fork != 0
	r.SourceCreatedAt = scanTime(sourceCreated)
	r.SourceUpdatedAt = scanTime(src)
	r.CreatedAt = scanTime(created)
	r.UpdatedAt = scanTime(updated)
	return &r, nil
}

// GetRepositoryByID returns the current projection of a repository by id.
func (c *Corpus) GetRepositoryByID(ctx context.Context, id int64) (*Repository, error) {
	var r Repository
	var sourceCreated, src, created, updated int64
	var archived, fork int
	var topics string
	err := c.db.QueryRowContext(ctx, `
		SELECT id, owner, name, external_id, description, default_branch, language, license, topics, stars, watchers, forks, open_issues, archived, fork, source_created_at, source_updated_at, observation_sequence, created_at, updated_at
		FROM repositories
		WHERE id = ?
	`, id).Scan(&r.ID, &r.Owner, &r.Name, &r.ExternalID, &r.Description, &r.DefaultBranch, &r.Language, &r.License, &topics, &r.Stars, &r.Watchers, &r.Forks, &r.OpenIssues, &archived, &fork, &sourceCreated, &src, &r.ObservationSequence, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get repository by id: %w", err)
	}
	r.Topics = splitLabels(topics)
	r.Archived = archived != 0
	r.Fork = fork != 0
	r.SourceCreatedAt = scanTime(sourceCreated)
	r.SourceUpdatedAt = scanTime(src)
	r.CreatedAt = scanTime(created)
	r.UpdatedAt = scanTime(updated)
	return &r, nil
}

// RepositorySearchOptions scopes a paginated repository search.
type RepositorySearchOptions struct {
	Limit  int
	Cursor string
}

// RepositorySearchPage is a paginated result of a repository keyword search.
type RepositorySearchPage struct {
	Repositories []Repository
	NextCursor   string
	Total        int
}

// ListRepositories returns repositories matching an optional name query.
// An empty query lists all repositories ordered by most recently updated.
func (c *Corpus) ListRepositories(ctx context.Context, query string, limit int) ([]Repository, error) {
	page, err := c.ListRepositoriesWithOptions(ctx, query, RepositorySearchOptions{Limit: limit})
	if err != nil {
		return nil, err
	}
	return page.Repositories, nil
}

// ListRepositoriesWithOptions returns repositories matching an optional name
// query with stable cursor pagination. Results are ordered by source_updated_at
// descending, then id descending, so the same cursor always returns the same
// next page on an unchanged corpus.
func (c *Corpus) ListRepositoriesWithOptions(ctx context.Context, query string, opts RepositorySearchOptions) (RepositorySearchPage, error) {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Limit > 100 {
		return RepositorySearchPage{}, errors.New("repository list limit cannot exceed 100")
	}

	cursor, err := c.decodeRepoCursor(opts.Cursor, query)
	if err != nil {
		return RepositorySearchPage{}, err
	}

	args := []any{}
	where := ""
	if query != "" {
		where = `WHERE (owner || '/' || name LIKE ? ESCAPE '\' OR description LIKE ? ESCAPE '\')`
		esc := escapeLike(query)
		args = append(args, "%"+esc+"%", "%"+esc+"%")
	}
	if cursor != nil {
		if where == "" {
			where = `WHERE `
		} else {
			where += ` AND `
		}
		where += `(source_updated_at < ? OR (source_updated_at = ? AND id < ?))`
		args = append(args, cursor.UpdatedAt, cursor.UpdatedAt, cursor.ID)
	}
	args = append(args, opts.Limit+1)

	rows, err := c.db.QueryContext(ctx, `
		SELECT id, owner, name, external_id, description, default_branch, language, license, topics, stars, watchers, forks, open_issues, archived, fork, source_created_at, source_updated_at, observation_sequence, created_at, updated_at
		FROM repositories
		`+where+`
		ORDER BY source_updated_at DESC, id DESC
		LIMIT ?
	`, args...)
	if err != nil {
		return RepositorySearchPage{}, fmt.Errorf("list repositories: %w", err)
	}
	defer rows.Close()

	var out []Repository
	for rows.Next() {
		var r Repository
		var sourceCreated, src, created, updated int64
		var archived, fork int
		var topics string
		if err := rows.Scan(&r.ID, &r.Owner, &r.Name, &r.ExternalID, &r.Description, &r.DefaultBranch, &r.Language, &r.License, &topics, &r.Stars, &r.Watchers, &r.Forks, &r.OpenIssues, &archived, &fork, &sourceCreated, &src, &r.ObservationSequence, &created, &updated); err != nil {
			return RepositorySearchPage{}, err
		}
		r.Topics = splitLabels(topics)
		r.Archived = archived != 0
		r.Fork = fork != 0
		r.SourceCreatedAt = scanTime(sourceCreated)
		r.SourceUpdatedAt = scanTime(src)
		r.CreatedAt = scanTime(created)
		r.UpdatedAt = scanTime(updated)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return RepositorySearchPage{}, err
	}

	page := RepositorySearchPage{Repositories: out}
	if len(out) > opts.Limit {
		page.Repositories = out[:opts.Limit]
		last := page.Repositories[len(page.Repositories)-1]
		page.NextCursor = encodeCursor(searchCursor{
			Scope:     "repos",
			Query:     query,
			Kind:      "repo",
			UpdatedAt: encodeTime(last.SourceUpdatedAt),
			ID:        last.ID,
		})
	}
	if len(out) > opts.Limit || opts.Cursor != "" {
		page.Total, err = c.countRepositories(ctx, query)
		if err != nil {
			return RepositorySearchPage{}, err
		}
	} else {
		page.Total = len(out)
	}

	return page, nil
}

func (c *Corpus) countRepositories(ctx context.Context, query string) (int, error) {
	args := []any{}
	where := ""
	if query != "" {
		where = `WHERE (owner || '/' || name LIKE ? ESCAPE '\' OR description LIKE ? ESCAPE '\')`
		esc := escapeLike(query)
		args = append(args, "%"+esc+"%", "%"+esc+"%")
	}
	var total int
	err := c.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM repositories
		`+where, args...).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("count repositories: %w", err)
	}
	return total, nil
}

func (c *Corpus) decodeRepoCursor(cursor, query string) (*searchCursor, error) {
	if cursor == "" {
		return nil, nil
	}
	sc, err := decodeCursor(cursor)
	if err != nil {
		return nil, err
	}
	if sc.Scope != "repos" || sc.Query != query || sc.Kind != "repo" {
		return nil, errors.New("invalid search cursor")
	}
	return &sc, nil
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
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
	thread := Thread{
		RepositoryID:    repoID,
		Kind:            kind,
		Number:          number,
		State:           state,
		Title:           title,
		Body:            body,
		Author:          author,
		SourceUpdatedAt: sourceUpdatedAt,
	}
	return c.UpsertThread(ctx, thread, payload)
}

// UpsertThread records a thread observation and updates the projection with
// all fields when the source ordering is newer.
func (c *Corpus) UpsertThread(ctx context.Context, thread Thread, payload string) (*Thread, error) {
	if thread.Kind == ThreadKindPullRequest && (thread.Merged || !thread.MergedAt.IsZero()) {
		thread.MergedKnown = true
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin thread upsert: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := encodeTime(time.Now())
	seq, err := c.nextSequence(ctx, tx)
	if err != nil {
		return nil, err
	}

	srcSec := encodeTime(thread.SourceUpdatedAt)
	sourceCreated := encodeTime(thread.SourceCreatedAt)
	closed := sql.NullInt64{}
	if !thread.ClosedAt.IsZero() {
		closed.Int64 = encodeTime(thread.ClosedAt)
		closed.Valid = true
	}
	merged := sql.NullInt64{}
	if !thread.MergedAt.IsZero() {
		merged.Int64 = encodeTime(thread.MergedAt)
		merged.Valid = true
	}
	assignees := deterministicAssignees(thread.Assignees)

	var threadID int64
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM threads WHERE repository_id = ? AND kind = ? AND number = ?
	`, thread.RepositoryID, thread.Kind, thread.Number).Scan(&threadID)
	if errors.Is(err, sql.ErrNoRows) {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO threads (repository_id, kind, number, state, state_reason, title, body, author, author_association, labels, assignees, draft, locked, milestone, source_created_at, source_updated_at, observation_sequence, created_at, updated_at, closed_at, merged_at, merged, merged_known)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, thread.RepositoryID, thread.Kind, thread.Number, thread.State, thread.StateReason, thread.Title, thread.Body, thread.Author, thread.AuthorAssociation, joinLabels(thread.Labels), joinLabels(assignees), boolToInt(thread.Draft), boolToInt(thread.Locked), thread.Milestone, sourceCreated, srcSec, seq, now, now, closed, merged, boolToInt(thread.Merged), boolToInt(thread.MergedKnown))
		if err != nil {
			return nil, fmt.Errorf("insert thread: %w", err)
		}
		threadID, err = res.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("last thread id: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("select thread: %w", err)
	} else {
		if _, err := tx.ExecContext(ctx, `
			UPDATE threads
			SET state = ?,
			    state_reason = ?,
			    title = ?,
			    body = ?,
			    author = ?,
			    author_association = ?,
			    labels = ?,
			    assignees = ?,
			    draft = ?,
			    locked = ?,
			    milestone = ?,
			    source_created_at = ?,
			    source_updated_at = ?,
			    observation_sequence = ?,
			    updated_at = ?,
			    closed_at = ?,
			    merged_at = CASE WHEN ? = 1 THEN ? ELSE merged_at END,
			    merged = CASE WHEN ? = 1 THEN ? ELSE merged END,
			    merged_known = CASE WHEN ? = 1 THEN 1 ELSE merged_known END
			WHERE id = ?
			  AND (source_updated_at < ? OR (source_updated_at = ? AND observation_sequence < ?))
		`, thread.State, thread.StateReason, thread.Title, thread.Body, thread.Author, thread.AuthorAssociation, joinLabels(thread.Labels), joinLabels(assignees), boolToInt(thread.Draft), boolToInt(thread.Locked), thread.Milestone, sourceCreated, srcSec, seq, now, closed, boolToInt(thread.MergedKnown), merged, boolToInt(thread.MergedKnown), boolToInt(thread.Merged), boolToInt(thread.MergedKnown), threadID, srcSec, srcSec, seq); err != nil {
			return nil, fmt.Errorf("update thread projection: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO thread_observations (thread_id, source_updated_at, observation_sequence, payload, observed_at)
		VALUES (?, ?, ?, ?, ?)
	`, threadID, srcSec, seq, payload, now); err != nil {
		return nil, fmt.Errorf("insert thread observation: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit thread upsert: %w", err)
	}
	return c.GetThread(ctx, thread.RepositoryID, thread.Kind, thread.Number)
}

// GetThreadByNumber returns the current projection of a thread by repository
// and number, regardless of kind, or nil if it has not been observed.
func (c *Corpus) GetThreadByNumber(ctx context.Context, repoID int64, number int) (*Thread, error) {
	var t Thread
	var body, author, labels, assignees, stateReason, authorAssociation, milestone sql.NullString
	var sourceCreated, src, created, updated int64
	var closed, mergedAt sql.NullInt64
	var merged, mergedKnown, draft, locked int
	err := c.db.QueryRowContext(ctx, `
		SELECT id, repository_id, kind, number, state, state_reason, title, body, author, author_association, labels, assignees, draft, locked, milestone,
		       source_created_at, source_updated_at, observation_sequence, created_at, updated_at, closed_at, merged_at, merged, merged_known
		FROM threads
		WHERE repository_id = ? AND number = ?
	`, repoID, number).Scan(&t.ID, &t.RepositoryID, &t.Kind, &t.Number, &t.State, &stateReason, &t.Title, &body, &author, &authorAssociation, &labels, &assignees, &draft, &locked, &milestone, &sourceCreated, &src, &t.ObservationSequence, &created, &updated, &closed, &mergedAt, &merged, &mergedKnown)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get thread by number: %w", err)
	}
	t.Body = body.String
	t.StateReason = stateReason.String
	t.Author = author.String
	t.AuthorAssociation = authorAssociation.String
	t.Labels = splitLabels(labels.String)
	t.Assignees = splitLabels(assignees.String)
	t.Draft = draft != 0
	t.Locked = locked != 0
	t.Milestone = milestone.String
	t.SourceCreatedAt = scanTime(sourceCreated)
	t.SourceUpdatedAt = scanTime(src)
	t.CreatedAt = scanTime(created)
	t.UpdatedAt = scanTime(updated)
	t.ClosedAt = scanTime(closed.Int64)
	t.MergedAt = scanTime(mergedAt.Int64)
	t.Merged = merged != 0
	t.MergedKnown = mergedKnown != 0
	return &t, nil
}

// GetThread returns the current projection of a thread, or nil if it has not
// been observed.
func (c *Corpus) GetThread(ctx context.Context, repoID int64, kind string, number int) (*Thread, error) {
	var t Thread
	var body, author, labels, assignees, stateReason, authorAssociation, milestone sql.NullString
	var sourceCreated, src, created, updated int64
	var closed, mergedAt sql.NullInt64
	var merged, mergedKnown, draft, locked int
	err := c.db.QueryRowContext(ctx, `
		SELECT id, repository_id, kind, number, state, state_reason, title, body, author, author_association, labels, assignees, draft, locked, milestone,
		       source_created_at, source_updated_at, observation_sequence, created_at, updated_at, closed_at, merged_at, merged, merged_known
		FROM threads
		WHERE repository_id = ? AND kind = ? AND number = ?
	`, repoID, kind, number).Scan(&t.ID, &t.RepositoryID, &t.Kind, &t.Number, &t.State, &stateReason, &t.Title, &body, &author, &authorAssociation, &labels, &assignees, &draft, &locked, &milestone, &sourceCreated, &src, &t.ObservationSequence, &created, &updated, &closed, &mergedAt, &merged, &mergedKnown)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get thread: %w", err)
	}
	t.Body = body.String
	t.StateReason = stateReason.String
	t.Author = author.String
	t.AuthorAssociation = authorAssociation.String
	t.Labels = splitLabels(labels.String)
	t.Assignees = splitLabels(assignees.String)
	t.Draft = draft != 0
	t.Locked = locked != 0
	t.Milestone = milestone.String
	t.SourceCreatedAt = scanTime(sourceCreated)
	t.SourceUpdatedAt = scanTime(src)
	t.CreatedAt = scanTime(created)
	t.UpdatedAt = scanTime(updated)
	t.ClosedAt = scanTime(closed.Int64)
	t.MergedAt = scanTime(mergedAt.Int64)
	t.Merged = merged != 0
	t.MergedKnown = mergedKnown != 0
	return &t, nil
}

// ListThreads returns threads for a repository, optionally filtered by kind,
// ordered by source update time descending and then number descending.
func (c *Corpus) ListThreads(ctx context.Context, repoID int64, kind string, limit int) ([]Thread, error) {
	if limit <= 0 {
		limit = 1000
	}
	if limit > 10_000 {
		return nil, fmt.Errorf("thread list limit cannot exceed 10000")
	}
	sql := `
		SELECT id, repository_id, kind, number, state, state_reason, title, body, author, author_association, labels, assignees, draft, locked, milestone,
		       source_created_at, source_updated_at, observation_sequence, created_at, updated_at, closed_at, merged_at, merged, merged_known
		FROM threads
		WHERE repository_id = ?`
	args := []any{repoID}
	if kind != "" {
		sql += ` AND kind = ?`
		args = append(args, kind)
	}
	sql += ` ORDER BY source_updated_at DESC, number DESC`
	sql += ` LIMIT ?`
	args = append(args, limit)

	rows, err := c.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("list threads: %w", err)
	}
	defer rows.Close()

	return scanThreads(rows)
}

// ListThreadsFiltered returns threads for a repository, optionally filtered by
// kind and state, ordered by source update time descending and then number
// descending. Filtering happens at the corpus boundary before any limit is
// applied, so bounded callers do not silently drop matching rows.
func (c *Corpus) ListThreadsFiltered(ctx context.Context, repoID int64, kind, state string, limit int) ([]Thread, error) {
	if limit <= 0 {
		limit = 1000
	}
	if limit > 10_000 {
		return nil, fmt.Errorf("thread list limit cannot exceed 10000")
	}
	sql := `
		SELECT id, repository_id, kind, number, state, state_reason, title, body, author, author_association, labels, assignees, draft, locked, milestone,
		       source_created_at, source_updated_at, observation_sequence, created_at, updated_at, closed_at, merged_at, merged, merged_known
		FROM threads
		WHERE repository_id = ?`
	args := []any{repoID}
	if kind != "" {
		sql += ` AND kind = ?`
		args = append(args, kind)
	}
	if state != "" && state != "all" {
		sql += ` AND state = ?`
		args = append(args, state)
	}
	sql += ` ORDER BY source_updated_at DESC, number DESC`
	sql += ` LIMIT ?`
	args = append(args, limit)

	rows, err := c.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("list threads: %w", err)
	}
	defer rows.Close()

	return scanThreads(rows)
}

// CountThreadsFiltered counts threads after applying the same kind and state
// predicates as ListThreadsFiltered.
func (c *Corpus) CountThreadsFiltered(ctx context.Context, repoID int64, kind, state string) (int, error) {
	query := `SELECT COUNT(*) FROM threads WHERE repository_id = ?`
	args := []any{repoID}
	if kind != "" {
		query += ` AND kind = ?`
		args = append(args, kind)
	}
	if state != "" && state != "all" {
		query += ` AND state = ?`
		args = append(args, state)
	}
	var total int
	if err := c.db.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count threads: %w", err)
	}
	return total, nil
}

// LatestThreadObservation returns the most recent observation for a thread
// by source time and observation sequence.
func (c *Corpus) LatestThreadObservation(ctx context.Context, threadID int64) (*ThreadObservation, error) {
	var o ThreadObservation
	var src, observed int64
	err := c.db.QueryRowContext(ctx, `
		SELECT id, thread_id, source_updated_at, observation_sequence, payload, observed_at
		FROM thread_observations
		WHERE thread_id = ?
		ORDER BY source_updated_at DESC, observation_sequence DESC
		LIMIT 1
	`, threadID).Scan(&o.ID, &o.ThreadID, &src, &o.ObservationSequence, &o.Payload, &observed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("latest thread observation: %w", err)
	}
	o.SourceUpdatedAt = scanTime(src)
	o.ObservedAt = scanTime(observed)
	return &o, nil
}

// GetThreadObservationRevision returns the immutable observation matching a
// projection revision. It lets callers bind copied projection fields to the
// exact observation even if a newer projection is written concurrently.
func (c *Corpus) GetThreadObservationRevision(ctx context.Context, threadID int64, sourceUpdatedAt time.Time, observationSequence int64) (*ThreadObservation, error) {
	var o ThreadObservation
	var src, observed int64
	err := c.db.QueryRowContext(ctx, `
		SELECT id, thread_id, source_updated_at, observation_sequence, payload, observed_at
		FROM thread_observations
		WHERE thread_id=? AND source_updated_at=? AND observation_sequence=?
		LIMIT 1
	`, threadID, encodeTime(sourceUpdatedAt), observationSequence).Scan(
		&o.ID, &o.ThreadID, &src, &o.ObservationSequence, &o.Payload, &observed,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrThreadObservationRevisionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get thread observation revision: %w", err)
	}
	o.SourceUpdatedAt = scanTime(src)
	o.ObservedAt = scanTime(observed)
	return &o, nil
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

func scanThreads(rows *sql.Rows) ([]Thread, error) {
	var out []Thread
	for rows.Next() {
		var t Thread
		var body, author, labels, assignees, stateReason, authorAssociation, milestone sql.NullString
		var sourceCreated, src, created, updated int64
		var closed, mergedAt sql.NullInt64
		var merged, mergedKnown, draft, locked int
		if err := rows.Scan(&t.ID, &t.RepositoryID, &t.Kind, &t.Number, &t.State, &stateReason, &t.Title, &body, &author, &authorAssociation, &labels, &assignees, &draft, &locked, &milestone, &sourceCreated, &src, &t.ObservationSequence, &created, &updated, &closed, &mergedAt, &merged, &mergedKnown); err != nil {
			return nil, err
		}
		t.Body = body.String
		t.StateReason = stateReason.String
		t.Author = author.String
		t.AuthorAssociation = authorAssociation.String
		t.Labels = splitLabels(labels.String)
		t.Assignees = splitLabels(assignees.String)
		t.Draft = draft != 0
		t.Locked = locked != 0
		t.Milestone = milestone.String
		t.SourceCreatedAt = scanTime(sourceCreated)
		t.SourceUpdatedAt = scanTime(src)
		t.CreatedAt = scanTime(created)
		t.UpdatedAt = scanTime(updated)
		t.ClosedAt = scanTime(closed.Int64)
		t.MergedAt = scanTime(mergedAt.Int64)
		t.Merged = merged != 0
		t.MergedKnown = mergedKnown != 0
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func joinLabels(v []string) string {
	if len(v) == 0 {
		return ""
	}
	encoded, _ := json.Marshal(v)
	return string(encoded)
}

func deterministicAssignees(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	out := append([]string(nil), v...)
	sort.Strings(out)
	return out
}

func splitLabels(s string) []string {
	if s == "" {
		return nil
	}
	var decoded []string
	if json.Unmarshal([]byte(s), &decoded) == nil {
		return decoded
	}
	// Read comma-delimited values written by early development builds.
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
