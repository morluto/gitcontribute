package corpus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/domain"
)

// CodeMatch is one local code-search result at an immutable commit.
type CodeMatch struct {
	Repo              domain.RepoRef
	Commit            string
	Path              string
	Content           string
	Bytes             int
	Language          string
	SnapshotID        int64
	DocID             int64
	SnapshotCreatedAt time.Time
	Rank              float64
}

// CodeSearchOptions scopes a paginated code-document keyword search.
type CodeSearchOptions struct {
	Ref    domain.RepoRef
	Limit  int
	Cursor string
}

// CodeSearchPage is a paginated result of a code-document keyword search.
type CodeSearchPage struct {
	Matches    []CodeMatch
	NextCursor string
	Total      int
}

// StoreCodeSnapshot atomically stores one complete immutable code snapshot.
// Replaying the same repository commit returns the existing snapshot id.
func (c *Corpus) StoreCodeSnapshot(ctx context.Context, ref domain.RepoRef, snapshot codeindex.Snapshot) (int64, bool, error) {
	if err := ref.Validate(); err != nil {
		return 0, false, err
	}
	if snapshot.Commit == "" {
		return 0, false, errors.New("code snapshot commit is required")
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, fmt.Errorf("begin code snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var existing int64
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM code_snapshots WHERE repo_owner=? AND repo_name=? AND commit_sha=?
	`, ref.Owner, ref.Repo, snapshot.Commit).Scan(&existing)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, fmt.Errorf("find code snapshot: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO code_snapshots (repo_owner, repo_name, repo_path, commit_sha, total_bytes, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, ref.Owner, ref.Repo, snapshot.RepoPath, snapshot.Commit, snapshot.TotalBytes, encodeTime(snapshot.CreatedAt))
	if err != nil {
		return 0, false, fmt.Errorf("insert code snapshot: %w", err)
	}
	snapshotID, err := result.LastInsertId()
	if err != nil {
		return 0, false, fmt.Errorf("read code snapshot id: %w", err)
	}
	for _, document := range snapshot.Documents {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO code_documents (snapshot_id, path, content, bytes, language)
			VALUES (?, ?, ?, ?, ?)
		`, snapshotID, document.Path, document.Content, document.Bytes, document.LanguageHint); err != nil {
			return 0, false, fmt.Errorf("insert code document %q: %w", document.Path, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, false, fmt.Errorf("commit code snapshot: %w", err)
	}
	return snapshotID, true, nil
}

// LatestCodeSnapshot returns the most recently stored code snapshot for a
// repository, or nil if none exists.
func (c *Corpus) LatestCodeSnapshot(ctx context.Context, ref domain.RepoRef) (*struct {
	RepoPath  string
	CommitSHA string
	CreatedAt time.Time
}, error) {
	var snap struct {
		RepoPath  string
		CommitSHA string
		CreatedAt time.Time
	}
	var created int64
	err := c.db.QueryRowContext(ctx, `
		SELECT repo_path, commit_sha, created_at
		FROM code_snapshots
		WHERE repo_owner = ? AND repo_name = ?
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, ref.Owner, ref.Repo).Scan(&snap.RepoPath, &snap.CommitSHA, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("latest code snapshot: %w", err)
	}
	snap.CreatedAt = scanTime(created)
	return &snap, nil
}

// SearchCode searches only the latest indexed snapshot of each repository.
func (c *Corpus) SearchCode(ctx context.Context, query string, ref domain.RepoRef, limit int) ([]CodeMatch, error) {
	page, err := c.SearchCodeWithOptions(ctx, query, CodeSearchOptions{Ref: ref, Limit: limit})
	if err != nil {
		return nil, err
	}
	return page.Matches, nil
}

// SearchCodeWithOptions searches only the latest indexed snapshot of each
// repository with stable cursor pagination. Results are ordered by FTS5 rank
// ascending, then document id ascending. No network access occurs.
func (c *Corpus) SearchCodeWithOptions(ctx context.Context, query string, opts CodeSearchOptions) (CodeSearchPage, error) {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Limit > 100 {
		return CodeSearchPage{}, errors.New("code search limit cannot exceed 100")
	}

	ftsQuery := literalFTSQuery(query)
	if ftsQuery == "" {
		return CodeSearchPage{}, nil
	}

	repo := opts.Ref.String()
	cursor, err := c.decodeCodeCursor(opts.Cursor, query, repo)
	if err != nil {
		return CodeSearchPage{}, err
	}

	if opts.Ref.Owner != "" || opts.Ref.Repo != "" {
		if err := opts.Ref.Validate(); err != nil {
			return CodeSearchPage{}, err
		}
	}

	statement := `
		SELECT code_documents_fts.rank, d.id, s.repo_owner, s.repo_name, s.commit_sha, d.path, d.content, d.bytes, d.language, s.id, s.created_at
		FROM code_documents_fts
		JOIN code_documents d ON d.id = code_documents_fts.rowid
		JOIN code_snapshots s ON s.id = d.snapshot_id
		WHERE code_documents_fts MATCH ?
		  AND s.id = (SELECT newest.id FROM code_snapshots newest
		              WHERE newest.repo_owner = s.repo_owner AND newest.repo_name = s.repo_name
		              ORDER BY newest.created_at DESC, newest.id DESC LIMIT 1)`
	args := []any{ftsQuery}
	if opts.Ref.Owner != "" || opts.Ref.Repo != "" {
		statement += ` AND s.repo_owner = ? AND s.repo_name = ?`
		args = append(args, opts.Ref.Owner, opts.Ref.Repo)
	}
	if cursor != nil {
		statement += ` AND (code_documents_fts.rank > ? OR (code_documents_fts.rank = ? AND d.id > ?))`
		args = append(args, cursor.Rank, cursor.Rank, cursor.ID)
	}
	statement += ` ORDER BY code_documents_fts.rank, d.id LIMIT ?`
	args = append(args, opts.Limit+1)

	rows, err := c.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return CodeSearchPage{}, fmt.Errorf("search code: %w", err)
	}
	defer rows.Close()

	var matches []CodeMatch
	for rows.Next() {
		var match CodeMatch
		var createdAt int64
		if err := rows.Scan(&match.Rank, &match.DocID, &match.Repo.Owner, &match.Repo.Repo, &match.Commit,
			&match.Path, &match.Content, &match.Bytes, &match.Language, &match.SnapshotID, &createdAt); err != nil {
			return CodeSearchPage{}, err
		}
		match.SnapshotCreatedAt = scanTime(createdAt)
		matches = append(matches, match)
	}
	if err := rows.Err(); err != nil {
		return CodeSearchPage{}, err
	}

	page := CodeSearchPage{Matches: matches}
	if len(matches) > opts.Limit {
		page.Matches = matches[:opts.Limit]
		last := page.Matches[len(page.Matches)-1]
		page.NextCursor = encodeCursor(searchCursor{
			Scope: "code",
			Query: query,
			Repo:  repo,
			Kind:  "code",
			Rank:  last.Rank,
			ID:    last.DocID,
		})
	}
	if len(matches) > opts.Limit || opts.Cursor != "" {
		page.Total, err = c.countCodeMatches(ctx, ftsQuery, opts.Ref)
		if err != nil {
			return CodeSearchPage{}, err
		}
	} else {
		page.Total = len(matches)
	}

	return page, nil
}

func (c *Corpus) countCodeMatches(ctx context.Context, ftsQuery string, ref domain.RepoRef) (int, error) {
	statement := `
		SELECT COUNT(*)
		FROM code_documents_fts
		JOIN code_documents d ON d.id = code_documents_fts.rowid
		JOIN code_snapshots s ON s.id = d.snapshot_id
		WHERE code_documents_fts MATCH ?
		  AND s.id = (SELECT newest.id FROM code_snapshots newest
		              WHERE newest.repo_owner = s.repo_owner AND newest.repo_name = s.repo_name
		              ORDER BY newest.created_at DESC, newest.id DESC LIMIT 1)`
	args := []any{ftsQuery}
	if ref.Owner != "" || ref.Repo != "" {
		statement += ` AND s.repo_owner = ? AND s.repo_name = ?`
		args = append(args, ref.Owner, ref.Repo)
	}
	var total int
	if err := c.db.QueryRowContext(ctx, statement, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count code matches: %w", err)
	}
	return total, nil
}

func (c *Corpus) decodeCodeCursor(cursor, query, repo string) (*searchCursor, error) {
	if cursor == "" {
		return nil, nil
	}
	sc, err := decodeCursor(cursor)
	if err != nil {
		return nil, err
	}
	if sc.Scope != "code" || sc.Query != query || sc.Repo != repo || sc.Kind != "code" {
		return nil, errors.New("invalid search cursor")
	}
	return &sc, nil
}
