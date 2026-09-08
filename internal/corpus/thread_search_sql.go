package corpus

// Rank values remain global FTS5 BM25 values even when metadata filters narrow
// the candidate set. Page selection must precede hydrated excerpt generation.
const threadSearchRankSQL = "bm25(threads_fts, 10.0, 5.0, 2.0, 0.5)"

// Both paginated and exact reads define page_threads, then use the same source
// attribution and truncation rules. Materialized evidence is limited to those IDs.
const threadSearchEvidenceSQL = `, facet_raw AS MATERIALIZED (
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
		WHERE facet_observations_fts MATCH ?
		  AND facet_observations_fts.rowid IN (
		      SELECT id FROM facet_observations
		      WHERE thread_id IN (SELECT thread_id FROM page_threads)
		  )
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
		  AND threads_fts.rowid IN (SELECT thread_id FROM page_threads)
	), search_matches AS (
		SELECT d.thread_id,
		       p.rank,
		       COALESCE(fe.facet, CASE WHEN bf.thread_id IS NOT NULL THEN 'hydrated_facets' ELSE 'thread' END) AS source,
		       COALESCE(fe.excerpt, bf.excerpt, snippet(threads_fts, -1, '', '', ' … ', 32)) AS excerpt,
		       d.title || char(10) || d.labels || char(10) || d.body || char(10) || d.facets AS search_text,
		       COALESCE(fe.source_updated_at, bf.source_updated_at, t.source_updated_at) AS source_updated_at,
		       d.facets_truncated AS search_truncated
		FROM threads_fts
		JOIN thread_search_documents d ON d.thread_id = threads_fts.rowid
		JOIN threads t ON t.id = d.thread_id
		JOIN page_threads p ON p.thread_id = d.thread_id
		LEFT JOIN facet_evidence fe ON fe.thread_id = d.thread_id AND fe.source_position = 1 AND d.facets_truncated = 0
		LEFT JOIN bounded_facet_matches bf ON bf.thread_id = d.thread_id
		WHERE threads_fts MATCH ?
		  AND threads_fts.rowid IN (SELECT thread_id FROM page_threads)
	)`

func threadSearchEvidenceArguments(ftsQuery string) []any {
	return []any{ftsQuery, "facets : (" + ftsQuery + ")", ftsQuery}
}

func threadSearchPageStatement(ftsQuery, titleQuery string, filter SearchFilter, cursor *searchCursor) (string, []any) {
	statement := `WITH title_matches AS MATERIALIZED (
		SELECT rowid AS thread_id FROM threads_fts WHERE threads_fts MATCH ?
		), ranked_threads AS (
		SELECT t.id AS thread_id, t.source_updated_at,
			tm.thread_id IS NOT NULL AS title_match, ` + threadSearchRankSQL + ` AS rank
		FROM threads_fts
		JOIN threads t ON t.id = threads_fts.rowid
		LEFT JOIN title_matches tm ON tm.thread_id = t.id
		WHERE threads_fts MATCH ?`
	args := []any{"title : (" + titleQuery + ")", ftsQuery}
	if filter.Repository.IsScoped() {
		statement += ` AND t.repository_id = ?`
		args = append(args, filter.Repository.ID())
	}
	if !filter.Kind.IsAny() {
		statement += ` AND t.kind = ?`
		args = append(args, filter.Kind.String())
	}
	statement, args = appendThreadMetadataFilters(statement, args, filter)
	statement += `), page_threads AS MATERIALIZED (
		SELECT * FROM ranked_threads m WHERE 1 = 1`
	if cursor != nil {
		if filter.Order.IsUpdated() {
			statement += ` AND (m.source_updated_at < ? OR (m.source_updated_at = ? AND m.thread_id < ?))`
			args = append(args, cursor.UpdatedAt, cursor.UpdatedAt, cursor.ID)
		} else {
			statement += ` AND (m.title_match < ? OR (m.title_match = ? AND (m.rank > ? OR (m.rank = ? AND (m.source_updated_at < ? OR (m.source_updated_at = ? AND m.thread_id > ?))))))`
			args = append(args, cursor.TitleMatch, cursor.TitleMatch, cursor.Rank, cursor.Rank, cursor.UpdatedAt, cursor.UpdatedAt, cursor.ID)
		}
	}
	order := threadSearchOrderSQL(filter.Order)
	statement += ` ORDER BY ` + order + ` LIMIT ?)` + threadSearchEvidenceSQL + `
		SELECT m.title_match, m.rank, t.id, t.repository_id, t.kind, t.number, t.state, t.state_reason, t.title, t.body, t.author, t.author_association, t.labels, t.assignees, t.draft, t.locked, t.milestone,
			t.source_created_at, t.source_updated_at, t.observation_sequence, t.created_at, t.updated_at, t.closed_at, t.merged_at, t.merged, t.merged_known,
			e.source, e.excerpt, e.source_updated_at, e.search_truncated
		FROM page_threads m
		JOIN threads t ON t.id = m.thread_id
		JOIN search_matches e ON e.thread_id = m.thread_id
		ORDER BY ` + order
	args = append(args, filter.Page.Limit()+1)
	return statement, append(args, threadSearchEvidenceArguments(ftsQuery)...)
}

func threadSearchOrderSQL(order SearchOrder) string {
	if order.IsUpdated() {
		return "m.source_updated_at DESC, m.thread_id DESC"
	}
	return "m.title_match DESC, m.rank, m.source_updated_at DESC, m.thread_id"
}
