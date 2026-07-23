package corpus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// RepositorySearchOptions scopes a paginated repository search.
type RepositorySearchOptions struct {
	Limit  int
	Cursor string
	Sort   string
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

// ListRepositoriesWithOptions returns repositories matching weighted owner,
// name, topic, and description text with stable cursor pagination. Relevance
// is the default; updated order is explicit. Both orders use deterministic
// tie-breakers on an unchanged corpus.
func (c *Corpus) ListRepositoriesWithOptions(ctx context.Context, query string, opts RepositorySearchOptions) (_ RepositorySearchPage, err error) {
	opts, ftsQuery, cursor, err := c.prepareRepositorySearch(ctx, query, opts)
	if err != nil {
		return RepositorySearchPage{}, err
	}
	statement, args := repositorySearchStatement(ftsQuery, opts, cursor)
	rows, err := c.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return RepositorySearchPage{}, fmt.Errorf("list repositories: %w", err)
	}
	defer closeSQLOnReturn(rows, &err)
	out, err := scanRepositorySearchRows(rows)
	if err != nil {
		return RepositorySearchPage{}, err
	}

	page := RepositorySearchPage{Repositories: out}
	if len(out) > opts.Limit {
		page.Repositories = out[:opts.Limit]
		last := page.Repositories[len(page.Repositories)-1]
		page.NextCursor = encodeCursor(searchCursor{
			Scope: "repos", Query: ftsQuery, Kind: "repo", Filter: opts.Sort,
			Rank: last.Rank, UpdatedAt: encodeTime(last.SourceUpdatedAt), ID: last.ID,
		})
	}
	if len(out) > opts.Limit || opts.Cursor != "" {
		page.Total, err = c.countRepositories(ctx, ftsQuery)
		if err != nil {
			return RepositorySearchPage{}, err
		}
	} else {
		page.Total = len(out)
	}
	return page, nil
}

func (c *Corpus) prepareRepositorySearch(ctx context.Context, query string, opts RepositorySearchOptions) (RepositorySearchOptions, string, *searchCursor, error) {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Limit > 100 {
		return opts, "", nil, errors.New("repository list limit cannot exceed 100")
	}
	if opts.Sort == "" {
		opts.Sort = "relevance"
	}
	if opts.Sort != "relevance" && opts.Sort != "updated" {
		return opts, "", nil, errors.New("repository sort must be relevance or updated")
	}
	ftsQuery := literalFTSQuery(query)
	if ftsQuery != "" {
		if err := c.RequireProjection(ctx, ProjectionNameRepositoriesFTS, ProjectionVersionRepositoriesFTS); err != nil {
			return opts, "", nil, err
		}
	}
	cursor, err := c.decodeRepoCursor(opts.Cursor, ftsQuery, opts.Sort)
	return opts, ftsQuery, cursor, err
}

func repositorySearchStatement(ftsQuery string, opts RepositorySearchOptions, cursor *searchCursor) (string, []any) {
	args := []any{}
	where := ""
	from := "FROM repositories"
	rankSelect := "0.0"
	if ftsQuery != "" {
		from = "FROM repositories_fts JOIN repositories ON repositories.id = repositories_fts.rowid"
		where = `WHERE repositories_fts MATCH ?`
		rankSelect = "bm25(repositories_fts, 10.0, 10.0, 5.0, 2.0)"
		args = append(args, ftsQuery)
	}
	if cursor != nil {
		if where == "" {
			where = `WHERE `
		} else {
			where += ` AND `
		}
		if ftsQuery != "" && opts.Sort == "relevance" {
			where += `(` + rankSelect + ` > ? OR (` + rankSelect + ` = ? AND (repositories.source_updated_at < ? OR (repositories.source_updated_at = ? AND repositories.id > ?))))`
			args = append(args, cursor.Rank, cursor.Rank, cursor.UpdatedAt, cursor.UpdatedAt, cursor.ID)
		} else {
			where += `(repositories.source_updated_at < ? OR (repositories.source_updated_at = ? AND repositories.id < ?))`
			args = append(args, cursor.UpdatedAt, cursor.UpdatedAt, cursor.ID)
		}
	}
	statement := `
		SELECT ` + rankSelect + `, repositories.id, repositories.owner, repositories.name, repositories.external_id, repositories.description, repositories.default_branch, repositories.language, repositories.license, repositories.topics, repositories.stars, repositories.watchers, repositories.forks, repositories.open_issues, repositories.archived, repositories.fork, repositories.source_created_at, repositories.source_updated_at, repositories.observation_sequence, repositories.created_at, repositories.updated_at
		` + from + `
		` + where + `
		ORDER BY ` + repositoryOrder(ftsQuery, opts.Sort) + `
		LIMIT ?`
	return statement, append(args, opts.Limit+1)
}

func scanRepositorySearchRows(rows *sql.Rows) ([]Repository, error) {
	var out []Repository
	for rows.Next() {
		var repository Repository
		var sourceCreated, sourceUpdated, created, updated int64
		var archived, fork int
		var topics string
		if err := rows.Scan(&repository.Rank, &repository.ID, &repository.Owner, &repository.Name, &repository.ExternalID, &repository.Description, &repository.DefaultBranch, &repository.Language, &repository.License, &topics, &repository.Stars, &repository.Watchers, &repository.Forks, &repository.OpenIssues, &archived, &fork, &sourceCreated, &sourceUpdated, &repository.ObservationSequence, &created, &updated); err != nil {
			return nil, err
		}
		repository.Topics = splitLabels(topics)
		repository.Archived = archived != 0
		repository.Fork = fork != 0
		repository.SourceCreatedAt = scanTime(sourceCreated)
		repository.SourceUpdatedAt = scanTime(sourceUpdated)
		repository.CreatedAt = scanTime(created)
		repository.UpdatedAt = scanTime(updated)
		out = append(out, repository)
	}
	return out, rows.Err()
}

func repositoryOrder(ftsQuery, sort string) string {
	if ftsQuery != "" && sort == "relevance" {
		return "bm25(repositories_fts, 10.0, 10.0, 5.0, 2.0), repositories.source_updated_at DESC, repositories.id"
	}
	return "repositories.source_updated_at DESC, repositories.id DESC"
}

// RepositorySearchRank returns the weighted FTS5 rank for one repository.
func (c *Corpus) RepositorySearchRank(ctx context.Context, id int64, query string) (float64, bool, error) {
	ftsQuery := literalFTSQuery(query)
	if ftsQuery == "" {
		return 0, false, nil
	}
	var rank float64
	err := c.db.QueryRowContext(ctx, `
		SELECT bm25(repositories_fts, 10.0, 10.0, 5.0, 2.0)
		FROM repositories_fts
		WHERE repositories_fts MATCH ? AND rowid = ?
	`, ftsQuery, id).Scan(&rank)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("rank repository search match: %w", err)
	}
	return rank, true, nil
}

func (c *Corpus) countRepositories(ctx context.Context, ftsQuery string) (int, error) {
	args := []any{}
	where := ""
	from := "repositories"
	if ftsQuery != "" {
		from = "repositories_fts JOIN repositories ON repositories.id = repositories_fts.rowid"
		where = `WHERE repositories_fts MATCH ?`
		args = append(args, ftsQuery)
	}
	var total int
	err := c.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM `+from+`
		`+where, args...).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("count repositories: %w", err)
	}
	return total, nil
}

func (c *Corpus) decodeRepoCursor(cursor, query, sort string) (*searchCursor, error) {
	if cursor == "" {
		return nil, nil //nolint:nilnil // A missing cursor denotes the first page.
	}
	sc, err := decodeCursor(cursor)
	if err != nil {
		return nil, err
	}
	if sc.Scope != "repos" || sc.Query != query || sc.Kind != "repo" || sc.Filter != sort {
		return nil, errors.New("invalid search cursor")
	}
	return &sc, nil
}
