package corpus

import (
	"context"
	"fmt"
)

// SearchFilter scopes a thread keyword search.
type SearchFilter struct {
	RepoID int64
	Kind   string
	Limit  int
}

// SearchThreads performs an FTS5 keyword search over thread titles and bodies.
// It returns matching threads ordered by FTS5 rank and limited to at most limit
// results. No network access occurs.
func (c *Corpus) SearchThreads(ctx context.Context, query string, limit int) ([]Thread, error) {
	return c.SearchThreadsWithFilter(ctx, query, SearchFilter{Limit: limit})
}

// SearchThreadsWithFilter performs the same search as SearchThreads but
// supports filtering to a repository and thread kind.
func (c *Corpus) SearchThreadsWithFilter(ctx context.Context, query string, filter SearchFilter) ([]Thread, error) {
	if limit := filter.Limit; limit <= 0 {
		filter.Limit = 20
	}

	sql := `
		SELECT t.id, t.repository_id, t.kind, t.number, t.state, t.title, t.body, t.author, t.labels,
		       t.source_updated_at, t.observation_sequence, t.created_at, t.updated_at, t.closed_at, t.merged_at, t.merged
		FROM threads_fts fts
		JOIN threads t ON t.id = fts.rowid
		WHERE threads_fts MATCH ?`
	args := []any{query}
	if filter.RepoID != 0 {
		sql += ` AND t.repository_id = ?`
		args = append(args, filter.RepoID)
	}
	if filter.Kind != "" {
		sql += ` AND t.kind = ?`
		args = append(args, filter.Kind)
	}
	sql += ` ORDER BY rank LIMIT ?`
	args = append(args, filter.Limit)

	rows, err := c.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("search threads: %w", err)
	}
	defer rows.Close()

	return scanThreads(rows)
}
