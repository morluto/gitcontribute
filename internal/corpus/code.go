package corpus

import (
	"context"
	"database/sql"
	"encoding/json"
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
	Snapshots  []CodeSnapshotInfo
	NextCursor string
	Total      int
}

// StoreCodeSnapshot atomically stores one complete code snapshot. Replaying the
// same repository commit replaces its documents and coverage metadata without
// changing its ordering relative to other commits.
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
	manifest, err := json.Marshal(snapshot.Manifest)
	if err != nil {
		return 0, false, fmt.Errorf("encode code index manifest: %w", err)
	}
	var existing int64
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM code_snapshots WHERE repo_owner=? AND repo_name=? AND commit_sha=?
	`, ref.Owner, ref.Repo, snapshot.Commit).Scan(&existing)
	if err == nil {
		if _, err := tx.ExecContext(ctx, `
			UPDATE code_snapshots
			SET repo_path = ?, total_bytes = ?, manifest_json = ?
			WHERE id = ?
		`, snapshot.RepoPath, snapshot.TotalBytes, string(manifest), existing); err != nil {
			return 0, false, fmt.Errorf("update code snapshot: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM code_documents WHERE snapshot_id = ?`, existing); err != nil {
			return 0, false, fmt.Errorf("replace code snapshot documents: %w", err)
		}
		if err := storeCodeDocuments(ctx, tx, existing, snapshot.Documents); err != nil {
			return 0, false, err
		}
		if err := tx.Commit(); err != nil {
			return 0, false, fmt.Errorf("commit replaced code snapshot: %w", err)
		}
		return existing, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, fmt.Errorf("find code snapshot: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO code_snapshots (repo_owner, repo_name, repo_path, commit_sha, total_bytes, created_at, manifest_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, ref.Owner, ref.Repo, snapshot.RepoPath, snapshot.Commit, snapshot.TotalBytes, encodeTime(snapshot.CreatedAt), string(manifest))
	if err != nil {
		return 0, false, fmt.Errorf("insert code snapshot: %w", err)
	}
	snapshotID, err := result.LastInsertId()
	if err != nil {
		return 0, false, fmt.Errorf("read code snapshot id: %w", err)
	}
	if err := storeCodeDocuments(ctx, tx, snapshotID, snapshot.Documents); err != nil {
		return 0, false, err
	}
	if err := tx.Commit(); err != nil {
		return 0, false, fmt.Errorf("commit code snapshot: %w", err)
	}
	return snapshotID, true, nil
}

func storeCodeDocuments(ctx context.Context, tx *sql.Tx, snapshotID int64, documents []codeindex.Document) error {
	for _, document := range documents {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO code_documents (snapshot_id, path, content, bytes, language)
			VALUES (?, ?, ?, ?, ?)
		`, snapshotID, document.Path, document.Content, document.Bytes, document.LanguageHint); err != nil {
			return fmt.Errorf("insert code document %q: %w", document.Path, err)
		}
	}
	return nil
}

// CodeSnapshotInfo describes one stored code snapshot and its coverage.
type CodeSnapshotInfo struct {
	Repo      domain.RepoRef
	RepoPath  string
	CommitSHA string
	CreatedAt time.Time
	Manifest  codeindex.Manifest
}

// LatestCodeSnapshot returns the latest source snapshot selected for a
// repository, or nil if none exists.
func (c *Corpus) LatestCodeSnapshot(ctx context.Context, ref domain.RepoRef) (*CodeSnapshotInfo, error) {
	return latestCodeSnapshot(ctx, c.db, ref)
}

type codeSnapshotQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func latestCodeSnapshot(ctx context.Context, queryer codeSnapshotQueryer, ref domain.RepoRef) (*CodeSnapshotInfo, error) {
	var snap CodeSnapshotInfo
	var created int64
	var manifest string
	err := queryer.QueryRowContext(ctx, `
		SELECT repo_path, commit_sha, created_at, manifest_json
		FROM code_snapshots
		WHERE repo_owner = ? AND repo_name = ?
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, ref.Owner, ref.Repo).Scan(&snap.RepoPath, &snap.CommitSHA, &created, &manifest)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("latest code snapshot: %w", err)
	}
	snap.CreatedAt = scanTime(created)
	snap.Repo = ref
	if err := json.Unmarshal([]byte(manifest), &snap.Manifest); err != nil {
		return nil, fmt.Errorf("decode code index manifest: %w", err)
	}
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
// repository with stable cursor pagination. It returns bounded FTS snippets,
// not complete files. Results are ordered by weighted FTS5 rank
// ascending, then document id ascending. No network access occurs.
func (c *Corpus) SearchCodeWithOptions(ctx context.Context, query string, opts CodeSearchOptions) (_ CodeSearchPage, err error) {
	opts, ftsQuery, repo, cursor, err := c.prepareCodeSearch(ctx, query, opts)
	if err != nil {
		return CodeSearchPage{}, err
	}
	if ftsQuery == "" {
		return CodeSearchPage{}, nil
	}
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return CodeSearchPage{}, fmt.Errorf("begin code search snapshot: %w", err)
	}
	defer rollbackSQLOnReturn(tx, &err)

	statement, args := codeSearchStatement(ftsQuery, opts, cursor)
	matches, err := queryCodeSearchMatches(ctx, tx, statement, args)
	if err != nil {
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
		page.Total, err = countCodeMatches(ctx, tx, ftsQuery, opts.Ref)
		if err != nil {
			return CodeSearchPage{}, err
		}
	} else {
		page.Total = len(matches)
	}
	page.Snapshots, err = loadCodeSearchSnapshots(ctx, tx, opts.Ref, page.Matches)
	if err != nil {
		return CodeSearchPage{}, err
	}
	if err := tx.Commit(); err != nil {
		return CodeSearchPage{}, fmt.Errorf("commit code search snapshot: %w", err)
	}

	return page, nil
}

func (c *Corpus) prepareCodeSearch(ctx context.Context, query string, opts CodeSearchOptions) (CodeSearchOptions, string, string, *searchCursor, error) {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Limit > 100 {
		return opts, "", "", nil, errors.New("code search limit cannot exceed 100")
	}
	ftsQuery := literalFTSQuery(query)
	if ftsQuery == "" {
		return opts, "", "", nil, nil
	}
	if err := c.RequireProjection(ctx, ProjectionNameCodeDocumentsFTS, ProjectionVersionCodeDocumentsFTS); err != nil {
		return opts, "", "", nil, err
	}
	if opts.Ref != (domain.RepoRef{}) {
		if err := opts.Ref.Validate(); err != nil {
			return opts, "", "", nil, err
		}
	}
	repo := opts.Ref.String()
	cursor, err := c.decodeCodeCursor(opts.Cursor, query, repo)
	return opts, ftsQuery, repo, cursor, err
}

func codeSearchStatement(ftsQuery string, opts CodeSearchOptions, cursor *searchCursor) (string, []any) {
	statement := `
		SELECT bm25(code_documents_fts, 5.0, 1.0), d.id, s.repo_owner, s.repo_name, s.commit_sha, d.path,
		       snippet(code_documents_fts, -1, '', '', ' … ', 48), d.bytes, d.language, s.id, s.created_at
		FROM code_documents_fts
		JOIN code_documents d ON d.id = code_documents_fts.rowid
		JOIN code_snapshots s ON s.id = d.snapshot_id
		WHERE code_documents_fts MATCH ?
		  AND s.id = (SELECT newest.id FROM code_snapshots newest
		              WHERE newest.repo_owner = s.repo_owner AND newest.repo_name = s.repo_name
		              ORDER BY newest.created_at DESC, newest.id DESC LIMIT 1)`
	args := []any{ftsQuery}
	if opts.Ref != (domain.RepoRef{}) {
		statement += ` AND s.repo_owner = ? AND s.repo_name = ?`
		args = append(args, opts.Ref.Owner, opts.Ref.Repo)
	}
	if cursor != nil {
		statement += ` AND (bm25(code_documents_fts, 5.0, 1.0) > ? OR (bm25(code_documents_fts, 5.0, 1.0) = ? AND d.id > ?))`
		args = append(args, cursor.Rank, cursor.Rank, cursor.ID)
	}
	statement += ` ORDER BY bm25(code_documents_fts, 5.0, 1.0), d.id LIMIT ?`
	return statement, append(args, opts.Limit+1)
}

func scanCodeSearchMatches(rows *sql.Rows) ([]CodeMatch, error) {
	var matches []CodeMatch
	for rows.Next() {
		var match CodeMatch
		var createdAt int64
		if err := rows.Scan(&match.Rank, &match.DocID, &match.Repo.Owner, &match.Repo.Repo, &match.Commit,
			&match.Path, &match.Content, &match.Bytes, &match.Language, &match.SnapshotID, &createdAt); err != nil {
			return nil, err
		}
		match.SnapshotCreatedAt = scanTime(createdAt)
		matches = append(matches, match)
	}
	return matches, rows.Err()
}

func queryCodeSearchMatches(ctx context.Context, tx *sql.Tx, statement string, args []any) (_ []CodeMatch, err error) {
	rows, err := tx.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("search code: %w", err)
	}
	defer closeSQLOnReturn(rows, &err)
	return scanCodeSearchMatches(rows)
}

func loadCodeSearchSnapshots(ctx context.Context, tx *sql.Tx, scoped domain.RepoRef, matches []CodeMatch) ([]CodeSnapshotInfo, error) {
	refs := []domain.RepoRef{scoped}
	if scoped == (domain.RepoRef{}) {
		refs = refs[:0]
		seen := make(map[domain.RepoRef]struct{}, len(matches))
		for _, match := range matches {
			if _, ok := seen[match.Repo]; ok {
				continue
			}
			seen[match.Repo] = struct{}{}
			refs = append(refs, match.Repo)
		}
	}
	var snapshots []CodeSnapshotInfo
	for _, ref := range refs {
		snapshot, err := latestCodeSnapshot(ctx, tx, ref)
		if err != nil {
			return nil, err
		}
		if snapshot != nil {
			snapshots = append(snapshots, *snapshot)
		}
	}
	return snapshots, nil
}

const codeListLimit = 10000

func (c *Corpus) latestCodeSnapshotID(ctx context.Context, ref domain.RepoRef) (int64, error) {
	var id int64
	err := c.db.QueryRowContext(ctx, `
		SELECT id FROM code_snapshots
		WHERE repo_owner = ? AND repo_name = ?
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, ref.Owner, ref.Repo).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("latest code snapshot: %w", err)
	}
	return id, nil
}

// GetCodeDocument returns a single code document from the latest snapshot of a
// repository, or nil when no snapshot or document exists.
func (c *Corpus) GetCodeDocument(ctx context.Context, ref domain.RepoRef, path string) (*CodeMatch, error) {
	snapshotID, err := c.latestCodeSnapshotID(ctx, ref)
	if err != nil {
		return nil, err
	}
	if snapshotID == 0 {
		return nil, nil
	}
	var match CodeMatch
	var createdAt int64
	err = c.db.QueryRowContext(ctx, `
		SELECT d.id, s.repo_owner, s.repo_name, s.commit_sha, d.path, d.content, d.bytes, d.language, s.id, s.created_at
		FROM code_documents d
		JOIN code_snapshots s ON s.id = d.snapshot_id
		WHERE d.snapshot_id = ? AND d.path = ?
	`, snapshotID, path).Scan(&match.DocID, &match.Repo.Owner, &match.Repo.Repo, &match.Commit,
		&match.Path, &match.Content, &match.Bytes, &match.Language, &match.SnapshotID, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get code document: %w", err)
	}
	match.SnapshotCreatedAt = scanTime(createdAt)
	return &match, nil
}

// ListCodeDocuments returns all documents from the latest snapshot of a
// repository. Results are bounded to avoid unbounded offline work.
func (c *Corpus) ListCodeDocuments(ctx context.Context, ref domain.RepoRef) ([]CodeMatch, error) {
	snapshotID, err := c.latestCodeSnapshotID(ctx, ref)
	if err != nil {
		return nil, err
	}
	if snapshotID == 0 {
		return nil, nil
	}
	rows, err := c.db.QueryContext(ctx, `
		SELECT d.id, s.repo_owner, s.repo_name, s.commit_sha, d.path, d.content, d.bytes, d.language, s.id, s.created_at
		FROM code_documents d
		JOIN code_snapshots s ON s.id = d.snapshot_id
		WHERE d.snapshot_id = ?
		ORDER BY d.path
		LIMIT ?
	`, snapshotID, codeListLimit)
	if err != nil {
		return nil, fmt.Errorf("list code documents: %w", err)
	}
	defer rows.Close()

	var out []CodeMatch
	for rows.Next() {
		var match CodeMatch
		var createdAt int64
		if err := rows.Scan(&match.DocID, &match.Repo.Owner, &match.Repo.Repo, &match.Commit,
			&match.Path, &match.Content, &match.Bytes, &match.Language, &match.SnapshotID, &createdAt); err != nil {
			return nil, err
		}
		match.SnapshotCreatedAt = scanTime(createdAt)
		out = append(out, match)
	}
	return out, rows.Err()
}

func countCodeMatches(ctx context.Context, queryer codeSnapshotQueryer, ftsQuery string, ref domain.RepoRef) (int, error) {
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
	if err := queryer.QueryRowContext(ctx, statement, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count code matches: %w", err)
	}
	return total, nil
}

// CodeSearchEvidence is the ranked excerpt for one indexed file revision.
type CodeSearchEvidence struct {
	Rank    float64
	Excerpt string
}

// FindCodeSearchEvidence returns the weighted FTS5 rank and matching excerpt
// for one exact indexed document.
func (c *Corpus) FindCodeSearchEvidence(ctx context.Context, docID int64, query string) (CodeSearchEvidence, bool, error) {
	ftsQuery := literalFTSQuery(query)
	if ftsQuery == "" {
		return CodeSearchEvidence{}, false, nil
	}
	var evidence CodeSearchEvidence
	err := c.db.QueryRowContext(ctx, `
		SELECT bm25(code_documents_fts, 5.0, 1.0),
		       snippet(code_documents_fts, -1, '', '', ' … ', 48)
		FROM code_documents_fts
		WHERE code_documents_fts MATCH ? AND rowid = ?
	`, ftsQuery, docID).Scan(&evidence.Rank, &evidence.Excerpt)
	if errors.Is(err, sql.ErrNoRows) {
		return CodeSearchEvidence{}, false, nil
	}
	if err != nil {
		return CodeSearchEvidence{}, false, fmt.Errorf("find code search evidence: %w", err)
	}
	return evidence, true, nil
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
