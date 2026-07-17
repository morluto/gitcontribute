package corpus

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// SearchFilter scopes a thread keyword search.
type SearchFilter struct {
	RepoID       int64
	Repo         string
	Kind         string
	State        string
	Author       string
	Labels       []string
	UpdatedAfter time.Time
	Limit        int
	Cursor       string
}

// ThreadSearchPage is a paginated result of a thread keyword search.
type ThreadSearchPage struct {
	Threads    []Thread
	NextCursor string
	Total      int
}

// SearchThreads performs an FTS5 keyword search over thread titles and bodies.
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

	ftsQuery := literalFTSQuery(query)
	if ftsQuery == "" {
		return ThreadSearchPage{}, nil
	}

	filterKey := threadFilterKey(filter)
	cursor, err := c.decodeThreadCursor(filter.Cursor, query, filter.Repo, filter.Kind, filterKey)
	if err != nil {
		return ThreadSearchPage{}, err
	}

	sql := `
		SELECT rank, t.id, t.repository_id, t.kind, t.number, t.state, t.title, t.body, t.author, t.labels,
		       t.source_created_at, t.source_updated_at, t.observation_sequence, t.created_at, t.updated_at, t.closed_at, t.merged_at, t.merged
		FROM threads_fts
		JOIN threads t ON t.id = threads_fts.rowid
		WHERE threads_fts MATCH ?`
	args := []any{ftsQuery}
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
		sql += ` AND (threads_fts.rank > ? OR (threads_fts.rank = ? AND t.id > ?))`
		args = append(args, cursor.Rank, cursor.Rank, cursor.ID)
	}
	sql += ` ORDER BY threads_fts.rank, t.id LIMIT ?`
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
			Scope:  "threads",
			Query:  query,
			Repo:   filter.Repo,
			Kind:   filter.Kind,
			Filter: filterKey,
			Rank:   last.Rank,
			ID:     last.ID,
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
	sql := `SELECT COUNT(*) FROM threads_fts JOIN threads t ON t.id = threads_fts.rowid WHERE threads_fts MATCH ?`
	args := []any{ftsQuery}
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
	if filter.Author != "" {
		query += ` AND lower(t.author) = lower(?)`
		args = append(args, filter.Author)
	}
	for _, label := range filter.Labels {
		encoded, _ := json.Marshal(label)
		query += ` AND instr(lower(t.labels), lower(?)) > 0`
		args = append(args, string(encoded))
	}
	if !filter.UpdatedAfter.IsZero() {
		query += ` AND t.source_updated_at >= ?`
		args = append(args, filter.UpdatedAfter.UTC().Unix())
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
		strings.ToLower(filter.State), strings.ToLower(filter.Author), strings.Join(labels, ","),
		fmt.Sprint(filter.UpdatedAfter.UTC().Unix()),
	}, "|")
}

// scanThreadsWithRank reads threads and the FTS5 rank value used for cursor
// pagination.
func scanThreadsWithRank(rows *sql.Rows) ([]Thread, error) {
	var out []Thread
	for rows.Next() {
		var t Thread
		var rank float64
		var body, author, labels sql.NullString
		var sourceCreated, src, created, updated int64
		var closed, mergedAt sql.NullInt64
		var merged int
		if err := rows.Scan(&rank, &t.ID, &t.RepositoryID, &t.Kind, &t.Number, &t.State, &t.Title, &body, &author, &labels, &sourceCreated, &src, &t.ObservationSequence, &created, &updated, &closed, &mergedAt, &merged); err != nil {
			return nil, err
		}
		t.Body = body.String
		t.Author = author.String
		t.Labels = splitLabels(labels.String)
		t.SourceCreatedAt = scanTime(sourceCreated)
		t.SourceUpdatedAt = scanTime(src)
		t.CreatedAt = scanTime(created)
		t.UpdatedAt = scanTime(updated)
		t.ClosedAt = scanTime(closed.Int64)
		t.MergedAt = scanTime(mergedAt.Int64)
		t.Merged = merged != 0
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
		terms[i] = `"` + strings.ReplaceAll(term, `"`, `""`) + `"`
	}
	return strings.Join(terms, " ")
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
