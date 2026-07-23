package corpus

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// SearchFilter scopes a thread keyword search.
type SearchFilter struct {
	RepoID       int64
	Repo         string
	Kind         string
	State        string
	StateReason  string
	Merged       *bool
	Author       string
	Association  string
	Assignee     string
	Labels       []string
	UpdatedAfter time.Time
	Limit        int
	Cursor       string
	Sort         string
}

// ThreadSearchPage is a paginated result of a thread keyword search.
type ThreadSearchPage struct {
	Threads    []Thread
	NextCursor string
	Total      int
}

const threadSearchMatchesSQL = `
	WITH facet_raw AS MATERIALIZED (
		SELECT fo.thread_id, fo.facet,
		       snippet(facet_observations_fts, 0, '', '', ' … ', 32) AS excerpt,
		       (SELECT MAX(snapshot.source_updated_at)
		        FROM facet_observations snapshot
		        WHERE snapshot.repository_id = fo.repository_id
		          AND snapshot.thread_id = fo.thread_id
		          AND snapshot.facet = fo.facet) AS source_updated_at,
		       bm25(facet_observations_fts) AS rank, fo.id
		FROM facet_observations_fts
		JOIN facet_observations fo ON fo.id = facet_observations_fts.rowid
		WHERE facet_observations_fts MATCH ? AND fo.thread_id IS NOT NULL
	), facet_evidence AS (
		SELECT *, ROW_NUMBER() OVER (PARTITION BY thread_id ORDER BY rank, id) AS source_position
		FROM facet_raw
	), bounded_facet_matches AS MATERIALIZED (
		SELECT d.thread_id,
		       snippet(threads_fts, 3, '', '', ' … ', 32) AS excerpt,
		       d.facets_updated_at AS source_updated_at
		FROM threads_fts
		JOIN thread_search_documents d ON d.thread_id = threads_fts.rowid
		WHERE threads_fts MATCH ?
	), search_matches AS (
		SELECT d.thread_id,
		       bm25(threads_fts, 10.0, 5.0, 2.0, 0.5) AS rank,
		       COALESCE(fe.facet, CASE WHEN bf.thread_id IS NOT NULL THEN 'hydrated_facets' ELSE 'thread' END) AS source,
		       COALESCE(fe.excerpt, bf.excerpt, snippet(threads_fts, -1, '', '', ' … ', 32)) AS excerpt,
		       d.title || char(10) || d.labels || char(10) || d.body || char(10) || d.facets AS search_text,
		       COALESCE(fe.source_updated_at, bf.source_updated_at, t.source_updated_at) AS source_updated_at,
		       d.facets_truncated AS search_truncated
		FROM threads_fts
		JOIN thread_search_documents d ON d.thread_id = threads_fts.rowid
		JOIN threads t ON t.id = d.thread_id
		LEFT JOIN facet_evidence fe ON fe.thread_id = d.thread_id AND fe.source_position = 1 AND d.facets_truncated = 0
		LEFT JOIN bounded_facet_matches bf ON bf.thread_id = d.thread_id
		WHERE threads_fts MATCH ?
	)`

func threadSearchArguments(ftsQuery string) []any {
	return []any{ftsQuery, "facets : (" + ftsQuery + ")", ftsQuery}
}

// SearchThreads performs an FTS5 keyword search over thread title, body, and
// searchable hydrated facet evidence.
// It returns matching threads ordered by FTS5 rank and limited to at most limit
// results. No network access occurs.
func (c *Corpus) SearchThreads(ctx context.Context, query string, limit int) ([]Thread, error) {
	page, err := c.SearchThreadsPage(ctx, query, SearchFilter{Limit: limit})
	if err != nil {
		return nil, err
	}
	return page.Threads, nil
}

// SearchThreadsWithFilter performs the same search as SearchThreads but
// supports filtering to a repository and thread kind.
func (c *Corpus) SearchThreadsWithFilter(ctx context.Context, query string, filter SearchFilter) ([]Thread, error) {
	page, err := c.SearchThreadsPage(ctx, query, filter)
	if err != nil {
		return nil, err
	}
	return page.Threads, nil
}

// SearchThreadsPage performs an FTS5 keyword search with stable cursor
// pagination. Results are ordered by FTS5 rank ascending, then thread id
// ascending, so the same cursor always returns the same next page on an
// unchanged corpus. No network access occurs.
func (c *Corpus) SearchThreadsPage(ctx context.Context, query string, filter SearchFilter) (ThreadSearchPage, error) {
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Limit > 100 {
		return ThreadSearchPage{}, errors.New("search limit cannot exceed 100")
	}
	if filter.Sort == "" {
		filter.Sort = "relevance"
	}
	if filter.Sort != "relevance" && filter.Sort != "updated" {
		return ThreadSearchPage{}, errors.New("search sort must be relevance or updated")
	}

	ftsQuery := literalFTSQuery(query)
	if ftsQuery == "" {
		return ThreadSearchPage{}, nil
	}
	if err := c.RequireProjection(ctx, ProjectionNameThreadsFTS, ProjectionVersionThreadsFTS); err != nil {
		return ThreadSearchPage{}, err
	}
	if err := c.RequireProjection(ctx, ProjectionNameFacetObservationsFTS, ProjectionVersionFacetObservationsFTS); err != nil {
		return ThreadSearchPage{}, err
	}

	filterKey := threadFilterKey(filter)
	cursor, err := c.decodeThreadCursor(filter.Cursor, query, filter.Repo, filter.Kind, filterKey)
	if err != nil {
		return ThreadSearchPage{}, err
	}

	sql := threadSearchMatchesSQL + `
		SELECT m.rank, t.id, t.repository_id, t.kind, t.number, t.state, t.state_reason, t.title, t.body, t.author, t.author_association, t.labels, t.assignees, t.draft, t.locked, t.milestone,
		       t.source_created_at, t.source_updated_at, t.observation_sequence, t.created_at, t.updated_at, t.closed_at, t.merged_at, t.merged, t.merged_known,
		       m.source, m.excerpt, m.source_updated_at, m.search_truncated
		FROM search_matches m
		JOIN threads t ON t.id = m.thread_id
		WHERE 1 = 1`
	args := threadSearchArguments(ftsQuery)
	if filter.RepoID != 0 {
		sql += ` AND t.repository_id = ?`
		args = append(args, filter.RepoID)
	}
	if filter.Kind != "" {
		sql += ` AND t.kind = ?`
		args = append(args, filter.Kind)
	}
	sql, args = appendThreadMetadataFilters(sql, args, filter)
	if cursor != nil {
		if filter.Sort == "updated" {
			sql += ` AND (t.source_updated_at < ? OR (t.source_updated_at = ? AND t.id < ?))`
			args = append(args, cursor.UpdatedAt, cursor.UpdatedAt, cursor.ID)
		} else {
			sql += ` AND (m.rank > ? OR (m.rank = ? AND (t.source_updated_at < ? OR (t.source_updated_at = ? AND t.id > ?))))`
			args = append(args, cursor.Rank, cursor.Rank, cursor.UpdatedAt, cursor.UpdatedAt, cursor.ID)
		}
	}
	if filter.Sort == "updated" {
		sql += ` ORDER BY t.source_updated_at DESC, t.id DESC LIMIT ?`
	} else {
		sql += ` ORDER BY m.rank, t.source_updated_at DESC, t.id LIMIT ?`
	}
	args = append(args, filter.Limit+1)

	rows, err := c.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return ThreadSearchPage{}, fmt.Errorf("search threads: %w", err)
	}
	defer rows.Close()

	threads, err := scanThreadsWithRank(rows)
	if err != nil {
		return ThreadSearchPage{}, err
	}

	page := ThreadSearchPage{Threads: threads}
	if len(threads) > filter.Limit {
		page.Threads = threads[:filter.Limit]
		last := page.Threads[len(page.Threads)-1]
		page.NextCursor = encodeCursor(searchCursor{
			Scope:     "threads",
			Query:     query,
			Repo:      filter.Repo,
			Kind:      filter.Kind,
			Filter:    filterKey,
			Rank:      last.Rank,
			UpdatedAt: encodeTime(last.SourceUpdatedAt),
			ID:        last.ID,
		})
	}
	if len(threads) > filter.Limit || filter.Cursor != "" {
		page.Total, err = c.countThreadMatches(ctx, ftsQuery, filter)
		if err != nil {
			return ThreadSearchPage{}, err
		}
	} else {
		page.Total = len(threads)
	}

	return page, nil
}

func (c *Corpus) countThreadMatches(ctx context.Context, ftsQuery string, filter SearchFilter) (int, error) {
	sql := threadSearchMatchesSQL + `
		SELECT COUNT(*)
		FROM search_matches m
		JOIN threads t ON t.id = m.thread_id
		WHERE 1 = 1`
	args := threadSearchArguments(ftsQuery)
	if filter.RepoID != 0 {
		sql += ` AND t.repository_id = ?`
		args = append(args, filter.RepoID)
	}
	if filter.Kind != "" {
		sql += ` AND t.kind = ?`
		args = append(args, filter.Kind)
	}
	sql, args = appendThreadMetadataFilters(sql, args, filter)
	var total int
	if err := c.db.QueryRowContext(ctx, sql, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count threads: %w", err)
	}
	return total, nil
}

// FindThreadSearchEvidence returns the best stored document matching query for
// one thread. It reads only the local FTS projections.
func (c *Corpus) FindThreadSearchEvidence(ctx context.Context, threadID int64, query string) (ThreadSearchEvidence, bool, error) {
	ftsQuery := literalFTSQuery(query)
	if ftsQuery == "" {
		return ThreadSearchEvidence{}, false, nil
	}
	statement := threadSearchMatchesSQL + `
		SELECT source, search_text, excerpt, source_updated_at, rank, search_truncated
		FROM search_matches
		WHERE thread_id = ?`
	var evidence ThreadSearchEvidence
	var sourceUpdatedAt int64
	var truncated int
	args := append(threadSearchArguments(ftsQuery), threadID)
	err := c.db.QueryRowContext(ctx, statement, args...).Scan(
		&evidence.Source, &evidence.Text, &evidence.Excerpt, &sourceUpdatedAt, &evidence.Rank, &truncated,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ThreadSearchEvidence{}, false, nil
	}
	if err != nil {
		return ThreadSearchEvidence{}, false, fmt.Errorf("find thread search evidence: %w", err)
	}
	evidence.SourceUpdatedAt = scanTime(sourceUpdatedAt)
	evidence.Truncated = truncated != 0
	return evidence, true, nil
}

func (c *Corpus) decodeThreadCursor(cursor, query, repo, kind, filter string) (*searchCursor, error) {
	if cursor == "" {
		return nil, nil
	}
	sc, err := decodeCursor(cursor)
	if err != nil {
		return nil, err
	}
	if sc.Scope != "threads" || sc.Query != query || sc.Repo != repo || sc.Kind != kind || sc.Filter != filter {
		return nil, errors.New("invalid search cursor")
	}
	return &sc, nil
}

func appendThreadMetadataFilters(query string, args []any, filter SearchFilter) (string, []any) {
	if filter.State != "" && filter.State != "all" {
		query += ` AND t.state = ?`
		args = append(args, filter.State)
	}
	if filter.StateReason != "" {
		query += ` AND t.state_reason = ?`
		args = append(args, filter.StateReason)
	}
	if filter.Merged != nil {
		merged := 0
		if *filter.Merged {
			merged = 1
		}
		query += ` AND t.merged = ? AND t.merged_known = 1`
		args = append(args, merged)
	}
	if filter.Author != "" {
		query += ` AND lower(t.author) = lower(?)`
		args = append(args, filter.Author)
	}
	if filter.Association != "" {
		query += ` AND lower(t.author_association) = lower(?)`
		args = append(args, filter.Association)
	}
	if filter.Assignee != "" {
		encoded, _ := json.Marshal(filter.Assignee)
		query += ` AND instr(lower(t.assignees), lower(?)) > 0`
		args = append(args, string(encoded))
	}
	for _, label := range filter.Labels {
		encoded, _ := json.Marshal(label)
		query += ` AND instr(lower(t.labels), lower(?)) > 0`
		args = append(args, string(encoded))
	}
	if !filter.UpdatedAfter.IsZero() {
		query += ` AND t.source_updated_at >= ?`
		args = append(args, encodeTime(filter.UpdatedAfter))
	}
	return query, args
}

func threadFilterKey(filter SearchFilter) string {
	labels := append([]string(nil), filter.Labels...)
	for i := range labels {
		labels[i] = strings.ToLower(strings.TrimSpace(labels[i]))
	}
	slices.Sort(labels)
	return strings.Join([]string{
		strings.ToLower(filter.State), strings.ToLower(filter.StateReason), fmt.Sprint(filter.Merged), strings.ToLower(filter.Author), strings.ToLower(filter.Association), strings.ToLower(filter.Assignee), strings.Join(labels, ","),
		strconv.FormatInt(encodeTime(filter.UpdatedAfter), 10), filter.Sort,
	}, "|")
}

// scanThreadsWithRank reads threads and the FTS5 rank value used for cursor
// pagination.
func scanThreadsWithRank(rows *sql.Rows) ([]Thread, error) {
	var out []Thread
	for rows.Next() {
		var t Thread
		var rank float64
		var body, author, labels, assignees, stateReason, authorAssociation, milestone sql.NullString
		var sourceCreated, src, created, updated, matchUpdated int64
		var closed, mergedAt sql.NullInt64
		var merged, mergedKnown, draft, locked, matchTruncated int
		if err := rows.Scan(&rank, &t.ID, &t.RepositoryID, &t.Kind, &t.Number, &t.State, &stateReason, &t.Title, &body, &author, &authorAssociation, &labels, &assignees, &draft, &locked, &milestone, &sourceCreated, &src, &t.ObservationSequence, &created, &updated, &closed, &mergedAt, &merged, &mergedKnown, &t.MatchSource, &t.MatchExcerpt, &matchUpdated, &matchTruncated); err != nil {
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
		t.MatchUpdatedAt = scanTime(matchUpdated)
		t.MatchTruncated = matchTruncated != 0
		t.ClosedAt = scanTime(closed.Int64)
		t.MergedAt = scanTime(mergedAt.Int64)
		t.Merged = merged != 0
		t.MergedKnown = mergedKnown != 0
		t.Rank = rank
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// literalFTSQuery treats user input as terms rather than exposing FTS5 query
// operators. This keeps ordinary punctuation and unmatched quotes searchable.
func literalFTSQuery(query string) string {
	terms := strings.Fields(query)
	for i, term := range terms {
		terms[i] = quoteFTSTerm(term)
	}
	return strings.Join(terms, " ")
}

func quoteFTSTerm(term string) string {
	return `"` + strings.ReplaceAll(term, `"`, `""`) + `"`
}

// searchCursor is the product-owned opaque pagination cursor. It is encoded as
// base64(JSON) and never interpreted by callers.
type searchCursor struct {
	Scope     string  `json:"s"`
	Query     string  `json:"q"`
	Repo      string  `json:"r,omitempty"`
	Kind      string  `json:"k,omitempty"`
	Filter    string  `json:"f,omitempty"`
	Rank      float64 `json:"rank,omitempty"`
	UpdatedAt int64   `json:"u,omitempty"`
	ID        int64   `json:"id"`
}

func encodeCursor(c searchCursor) string {
	b, _ := json.Marshal(c)
	return base64.URLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (searchCursor, error) {
	b, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return searchCursor{}, fmt.Errorf("invalid cursor: %w", err)
	}
	var c searchCursor
	if err := json.Unmarshal(b, &c); err != nil {
		return searchCursor{}, fmt.Errorf("invalid cursor: %w", err)
	}
	if c.Scope == "" || c.ID == 0 {
		return searchCursor{}, errors.New("invalid cursor")
	}
	return c, nil
}
