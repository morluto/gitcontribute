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
	Repo       domain.RepoRef
	Commit     string
	Path       string
	Content    string
	Bytes      int
	Language   string
	SnapshotID int64
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
	if limit <= 0 {
		limit = 20
	}
	if limit > 1000 {
		return nil, errors.New("code search limit cannot exceed 1000")
	}
	query = literalFTSQuery(query)
	if query == "" {
		return []CodeMatch{}, nil
	}
	statement := `
		SELECT s.repo_owner, s.repo_name, s.commit_sha, d.path, d.content, d.bytes, d.language, s.id
		FROM code_documents_fts f
		JOIN code_documents d ON d.id=f.rowid
		JOIN code_snapshots s ON s.id=d.snapshot_id
		WHERE code_documents_fts MATCH ?
		  AND s.id=(SELECT newest.id FROM code_snapshots newest
		            WHERE newest.repo_owner=s.repo_owner AND newest.repo_name=s.repo_name
		            ORDER BY newest.created_at DESC, newest.id DESC LIMIT 1)`
	args := []any{query}
	if ref.Owner != "" || ref.Repo != "" {
		if err := ref.Validate(); err != nil {
			return nil, err
		}
		statement += ` AND s.repo_owner=? AND s.repo_name=?`
		args = append(args, ref.Owner, ref.Repo)
	}
	statement += ` ORDER BY rank, s.repo_owner, s.repo_name, d.path LIMIT ?`
	args = append(args, limit)
	rows, err := c.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("search code: %w", err)
	}
	defer rows.Close()
	var matches []CodeMatch
	for rows.Next() {
		var match CodeMatch
		if err := rows.Scan(&match.Repo.Owner, &match.Repo.Repo, &match.Commit, &match.Path,
			&match.Content, &match.Bytes, &match.Language, &match.SnapshotID); err != nil {
			return nil, err
		}
		matches = append(matches, match)
	}
	return matches, rows.Err()
}
