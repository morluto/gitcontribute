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

	"github.com/morluto/gitcontribute/internal/domain"
)

// SearchFilter scopes a thread keyword search.
type SearchFilter struct {
	Repository    ThreadRepositoryScope
	Kind          ThreadKindFilter
	State         ThreadStateFilter
	StateReason   ThreadStateReason
	Merge         MergeFilter
	Author        string
	Association   string
	Assignee      string
	Labels        []string
	UpdatedAfter  time.Time
	UpdatedBefore time.Time
	Page          SearchPage
	Order         SearchOrder
	TermMatch     TermMatch
}

// ThreadSearchPage is a paginated result of a thread keyword search.
type ThreadSearchPage struct {
	Threads           []Thread
	NextCursor        string
	Total             int
	UnknownMergeCount int
}

// SearchThreads performs an FTS5 keyword search over thread title, body, and
// searchable hydrated facet evidence.
// It prioritizes all-term title matches, then FTS5 rank, and returns at most limit
// results. No network access occurs.
func (c *Corpus) SearchThreads(ctx context.Context, query string, limit int) ([]Thread, error) {
	request, err := ParseSearchPage(limit, "")
	if err != nil {
		return nil, err
	}
	page, err := c.SearchThreadsPage(ctx, query, SearchFilter{Page: request})
	if err != nil {
		return nil, err
	}
	return page.Threads, nil
}

// SearchThreadsPage performs an FTS5 keyword search with stable cursor
// pagination. Relevance orders all-term title matches first, then FTS5 rank,
// newest source timestamp, and thread ID. A cursor returns the same next page on an
// unchanged corpus. No network access occurs.
func (c *Corpus) SearchThreadsPage(ctx context.Context, query string, filter SearchFilter) (ThreadSearchPage, error) {
	limit := filter.Page.Limit()
	ftsQuery := literalFTSQueryMode(query, filter.TermMatch)
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
	cursor, err := c.decodeThreadCursor(filter.Page.Cursor(), query, filter.Repository.String(), filter.Kind.String(), filterKey)
	if err != nil {
		return ThreadSearchPage{}, err
	}

	statement, args := threadSearchPageStatement(ftsQuery, literalFTSQuery(query), filter, cursor)

	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ThreadSearchPage{}, fmt.Errorf("begin thread search snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, statement, args...)
	if err != nil {
		return ThreadSearchPage{}, fmt.Errorf("search threads: %w", err)
	}
	defer func() { _ = rows.Close() }()

	threads, err := scanThreadsWithRank(rows)
	closeErr := rows.Close()
	if err != nil {
		return ThreadSearchPage{}, err
	}
	if closeErr != nil {
		return ThreadSearchPage{}, fmt.Errorf("close thread search rows: %w", closeErr)
	}

	page := ThreadSearchPage{Threads: threads}
	if len(threads) > limit {
		page.Threads = threads[:limit]
		last := page.Threads[len(page.Threads)-1]
		page.NextCursor = encodeCursor(searchCursor{
			Scope:      "threads",
			Query:      query,
			Repo:       filter.Repository.String(),
			Kind:       filter.Kind.String(),
			Filter:     filterKey,
			Rank:       last.Rank,
			TitleMatch: last.MatchTitle,
			UpdatedAt:  encodeTime(last.SourceUpdatedAt),
			ID:         last.ID,
		})
	}
	if filter.Merge.IsAny() && len(threads) <= limit && cursor == nil {
		page.Total = len(threads)
	} else {
		page.Total, page.UnknownMergeCount, err = countThreadMatchSummary(ctx, tx, ftsQuery, filter)
		if err != nil {
			return ThreadSearchPage{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ThreadSearchPage{}, fmt.Errorf("commit thread search snapshot: %w", err)
	}

	return page, nil
}

func countThreadMatchSummary(ctx context.Context, tx *sql.Tx, ftsQuery string, filter SearchFilter) (int, int, error) {
	statement := `
		WITH matching_threads AS (
			SELECT d.thread_id
			FROM threads_fts
			JOIN thread_search_documents d ON d.thread_id = threads_fts.rowid
			WHERE threads_fts MATCH ?
		)
		SELECT `
	args := []any{ftsQuery}
	if filter.Merge.IsAny() {
		statement += `COUNT(*)`
	} else {
		statement += `COALESCE(SUM(CASE WHEN ` + threadMergePredicate(filter.Merge) + ` THEN 1 ELSE 0 END), 0)`
	}
	statement += `,
		       COALESCE(SUM(CASE
		           WHEN ? = 1 AND t.kind = 'pull_request' AND t.merged_known = 0 THEN 1
		           ELSE 0
		       END), 0)
		FROM matching_threads m
		JOIN threads t ON t.id = m.thread_id
		WHERE 1 = 1`
	if filter.Merge.IsAny() || filter.Merge.IsUnknown() {
		args = append(args, 0)
	} else {
		args = append(args, 1)
	}
	if filter.Repository.IsScoped() {
		statement += ` AND t.repository_id = ?`
		args = append(args, filter.Repository.ID())
	}
	if !filter.Kind.IsAny() {
		statement += ` AND t.kind = ?`
		args = append(args, filter.Kind.String())
	}
	summaryFilter := filter
	summaryFilter.Merge = AnyMergeState()
	statement, args = appendThreadMetadataFilters(statement, args, summaryFilter)
	var total, unknownMerge int
	if err := tx.QueryRowContext(ctx, statement, args...).Scan(&total, &unknownMerge); err != nil {
		return 0, 0, fmt.Errorf("count thread matches: %w", err)
	}
	return total, unknownMerge, nil
}

// FindThreadSearchEvidence returns the best stored document matching query for
// one thread. It reads only the local FTS projections.
func (c *Corpus) FindThreadSearchEvidence(ctx context.Context, threadID int64, query string) (ThreadSearchEvidence, bool, error) {
	ftsQuery := literalFTSQuery(query)
	if ftsQuery == "" {
		return ThreadSearchEvidence{}, false, nil
	}
	statement := `WITH page_threads AS MATERIALIZED (
		SELECT rowid AS thread_id, ` + threadSearchRankSQL + ` AS rank
		FROM threads_fts WHERE threads_fts MATCH ? AND rowid = ?
	)` + threadSearchEvidenceSQL + `
		SELECT source, search_text, excerpt, source_updated_at, rank, search_truncated
		FROM search_matches`
	var evidence ThreadSearchEvidence
	var sourceUpdatedAt int64
	var truncated int
	args := append([]any{ftsQuery, threadID}, threadSearchEvidenceArguments(ftsQuery)...)
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
	if !filter.State.IsAny() {
		query += ` AND t.state = ?`
		args = append(args, filter.State.String())
	}
	if !filter.StateReason.IsAny() {
		query += ` AND t.state_reason = ?`
		args = append(args, filter.StateReason.String())
	}
	if !filter.Merge.IsAny() {
		query += ` AND ` + threadMergePredicate(filter.Merge)
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
	if !filter.UpdatedBefore.IsZero() {
		query += ` AND t.source_updated_at <= ?`
		args = append(args, encodeTime(filter.UpdatedBefore))
	}
	return query, args
}

// threadMergePredicate is shared by row selection and exact match counts.
func threadMergePredicate(merge MergeFilter) string {
	switch {
	case merge.IsUnknown():
		return "t.kind = 'pull_request' AND t.merged_known = 0"
	case merge.IsMerged():
		return "t.kind = 'pull_request' AND t.merged_known = 1 AND t.merged = 1"
	case merge.IsUnmerged():
		return "t.kind = 'pull_request' AND t.merged_known = 1 AND t.merged = 0"
	default:
		return "1 = 1"
	}
}

func threadFilterKey(filter SearchFilter) string {
	labels := append([]string(nil), filter.Labels...)
	slices.Sort(labels)
	encodedLabels, _ := json.Marshal(labels)
	key, _ := json.Marshal([]string{
		filter.State.String(), filter.StateReason.String(), filter.Merge.String(), filter.Author, filter.Association, filter.Assignee, string(encodedLabels),
		strconv.FormatInt(encodeTime(filter.UpdatedAfter), 10), strconv.FormatInt(encodeTime(filter.UpdatedBefore), 10), filter.Order.String(), filter.TermMatch.String(),
	})
	return string(key)
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
		if err := rows.Scan(&t.MatchTitle, &rank, &t.ID, &t.RepositoryID, &t.Kind, &t.Number, &t.State, &stateReason, &t.Title, &body, &author, &authorAssociation, &labels, &assignees, &draft, &locked, &milestone, &sourceCreated, &src, &t.ObservationSequence, &created, &updated, &closed, &mergedAt, &merged, &mergedKnown, &t.MatchSource, &t.MatchExcerpt, &matchUpdated, &matchTruncated); err != nil {
			return nil, err
		}
		if err := parseThreadProjection(&t); err != nil {
			return nil, fmt.Errorf("parse stored search thread: %w", err)
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
		merge, err := domain.ParseMergeStatus(mergedKnown != 0, merged != 0, scanTime(mergedAt.Int64))
		if err != nil {
			return nil, fmt.Errorf("parse stored merge status: %w", err)
		}
		t.Merge = merge
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
	return literalFTSQueryMode(query, MatchAllTerms())
}

func literalFTSQueryMode(query string, mode TermMatch) string {
	terms := strings.Fields(query)
	for i, term := range terms {
		terms[i] = quoteFTSTerm(term)
	}
	if mode.IsAny() {
		return strings.Join(terms, " OR ")
	}
	return strings.Join(terms, " ")
}

func quoteFTSTerm(term string) string {
	return `"` + strings.ReplaceAll(term, `"`, `""`) + `"`
}

// searchCursor is the product-owned opaque pagination cursor. It is encoded as
// base64(JSON) and never interpreted by callers.
type searchCursor struct {
	Scope      string  `json:"s"`
	Query      string  `json:"q"`
	Repo       string  `json:"r,omitempty"`
	Kind       string  `json:"k,omitempty"`
	Filter     string  `json:"f,omitempty"`
	Rank       float64 `json:"rank,omitempty"`
	TitleMatch bool    `json:"title_match,omitempty"`
	UpdatedAt  int64   `json:"u,omitempty"`
	ID         int64   `json:"id"`
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
