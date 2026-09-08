package corpus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

const repositorySearchRankSQL = "bm25(repositories_fts, 10.0, 10.0, 5.0, 2.0)"

// RepositorySearchOptions scopes a paginated repository search.
type RepositorySearchOptions struct {
	Page  SearchPage
	Order SearchOrder
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
	request, err := ParseSearchPage(limit, "")
	if err != nil {
		return nil, err
	}
	page, err := c.ListRepositoriesWithOptions(ctx, query, RepositorySearchOptions{Page: request})
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
	if len(out) > opts.Page.Limit() {
		page.Repositories = out[:opts.Page.Limit()]
		last := page.Repositories[len(page.Repositories)-1]
		page.NextCursor = encodeCursor(searchCursor{
			Scope: "repos", Query: ftsQuery, Kind: "repo", Filter: opts.Order.String(),
			Rank: last.Rank, UpdatedAt: encodeTime(last.SourceUpdatedAt), ID: last.ID,
		})
	}
	if len(out) > opts.Page.Limit() || opts.Page.Cursor() != "" {
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
	ftsQuery := repositoryFTSQuery(query)
	if ftsQuery != "" {
		if err := c.RequireProjection(ctx, ProjectionNameRepositoriesFTS, ProjectionVersionRepositoriesFTS); err != nil {
			return opts, "", nil, err
		}
	}
	cursor, err := c.decodeRepoCursor(opts.Page.Cursor(), ftsQuery, opts.Order.String())
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
		rankSelect = repositorySearchRankSQL
		args = append(args, ftsQuery)
	}
	if cursor != nil {
		if where == "" {
			where = `WHERE `
		} else {
			where += ` AND `
		}
		if ftsQuery != "" && !opts.Order.IsUpdated() {
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
		ORDER BY ` + repositoryOrder(ftsQuery, opts.Order) + `
		LIMIT ?`
	return statement, append(args, opts.Page.Limit()+1)
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

func repositoryOrder(ftsQuery string, order SearchOrder) string {
	if ftsQuery != "" && !order.IsUpdated() {
		return repositorySearchRankSQL + ", repositories.source_updated_at DESC, repositories.id"
	}
	return "repositories.source_updated_at DESC, repositories.id DESC"
}

// RepositorySearchEvidence is the ranked excerpt for one repository match.
type RepositorySearchEvidence struct {
	Rank    float64
	Excerpt string
}

// FindRepositorySearchEvidence returns the weighted FTS5 rank and matching
// repository metadata excerpt.
func (c *Corpus) FindRepositorySearchEvidence(ctx context.Context, id int64, query string) (RepositorySearchEvidence, bool, error) {
	ftsQuery := repositoryFTSQuery(query)
	if ftsQuery == "" {
		return RepositorySearchEvidence{}, false, nil
	}
	var evidence RepositorySearchEvidence
	err := c.db.QueryRowContext(ctx, `
		SELECT `+repositorySearchRankSQL+`,
		       snippet(repositories_fts, -1, '', '', ' … ', 48)
		FROM repositories_fts
		WHERE repositories_fts MATCH ? AND rowid = ?
	`, ftsQuery, id).Scan(&evidence.Rank, &evidence.Excerpt)
	if errors.Is(err, sql.ErrNoRows) {
		return RepositorySearchEvidence{}, false, nil
	}
	if err != nil {
		return RepositorySearchEvidence{}, false, fmt.Errorf("find repository search evidence: %w", err)
	}
	return evidence, true, nil
}

func repositoryFTSQuery(query string) string {
	query = strings.TrimSpace(query)
	if strings.Count(query, "/") == 1 && !strings.ContainsAny(query, " \t\r\n") {
		owner, repo, _ := strings.Cut(query, "/")
		if owner != "" && repo != "" {
			return `owner : ` + quoteFTSTerm(owner) + ` AND name : ` + quoteFTSTerm(repo)
		}
	}
	return literalFTSQuery(query)
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
