package corpus

import (
	"context"
	"database/sql"
	"fmt"
)

// SearchThreads performs an FTS5 keyword search over thread titles and bodies.
// It returns matching threads ordered by FTS5 rank and limited to at most limit
// results. No network access occurs.
func (c *Corpus) SearchThreads(ctx context.Context, query string, limit int) ([]Thread, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := c.db.QueryContext(ctx, `
		SELECT t.id, t.repository_id, t.kind, t.number, t.state, t.title, t.body, t.author,
		       t.source_updated_at, t.observation_sequence, t.created_at, t.updated_at
		FROM threads_fts fts
		JOIN threads t ON t.id = fts.rowid
		WHERE threads_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search threads: %w", err)
	}
	defer rows.Close()

	return scanThreads(rows)
}

func scanThreads(rows *sql.Rows) ([]Thread, error) {
	var out []Thread
	for rows.Next() {
		var t Thread
		var body, author sql.NullString
		var src, created, updated int64
		if err := rows.Scan(&t.ID, &t.RepositoryID, &t.Kind, &t.Number, &t.State, &t.Title, &body, &author,
			&src, &t.ObservationSequence, &created, &updated); err != nil {
			return nil, err
		}
		t.Body = body.String
		t.Author = author.String
		t.SourceUpdatedAt = scanTime(src)
		t.CreatedAt = scanTime(created)
		t.UpdatedAt = scanTime(updated)
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
