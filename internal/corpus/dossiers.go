package corpus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

// SaveDossier persists a deterministic dossier snapshot and its exact sources.
func (c *Corpus) SaveDossier(ctx context.Context, repoID int64, owner, name, commitSHA string, asOf time.Time, sectionMetadata, snapshot string, generatedAt time.Time, sources []domain.SourceRef) (int64, error) {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin save dossier: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	id, err := insertDossierTx(ctx, tx, repoID, owner, name, commitSHA, asOf, sectionMetadata, snapshot, generatedAt)
	if err != nil {
		return 0, err
	}
	if err := insertDossierSourcesTx(ctx, tx, id, sources); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit save dossier: %w", err)
	}
	return id, nil
}

// RefreshDossier stores a dossier only when it is newer than the latest stored
// snapshot for the repository, or has changed at the same as-of time.
// It returns the dossier id and whether a new row was inserted.
func (c *Corpus) RefreshDossier(ctx context.Context, repoID int64, owner, name, commitSHA string, asOf time.Time, sectionMetadata, snapshot string, generatedAt time.Time, sources []domain.SourceRef) (int64, bool, error) {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, fmt.Errorf("begin refresh dossier: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingID int64
	var existingAsOf int64
	var existingSnapshot string
	err = tx.QueryRowContext(ctx, `
		SELECT id, as_of, snapshot
		FROM dossiers
		WHERE repo_owner = ? AND repo_name = ?
		ORDER BY generated_at DESC, id DESC
		LIMIT 1
	`, owner, name).Scan(&existingID, &existingAsOf, &existingSnapshot)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, false, fmt.Errorf("select existing dossier: %w", err)
	}

	if err == nil {
		newAsOf := encodeTime(asOf)
		if existingAsOf > newAsOf || (existingAsOf == newAsOf && existingSnapshot == snapshot) {
			if rbErr := tx.Rollback(); rbErr != nil {
				return 0, false, fmt.Errorf("rollback dossier refresh: %w", rbErr)
			}
			return existingID, false, nil
		}
	}

	id, err := insertDossierTx(ctx, tx, repoID, owner, name, commitSHA, asOf, sectionMetadata, snapshot, generatedAt)
	if err != nil {
		return 0, false, err
	}
	if err := insertDossierSourcesTx(ctx, tx, id, sources); err != nil {
		return 0, false, err
	}

	if err := tx.Commit(); err != nil {
		return 0, false, fmt.Errorf("commit refresh dossier: %w", err)
	}
	return id, true, nil
}

func insertDossierTx(ctx context.Context, tx *sql.Tx, repoID int64, owner, name, commitSHA string, asOf time.Time, sectionMetadata, snapshot string, generatedAt time.Time) (int64, error) {
	asOfSec := encodeTime(asOf)
	genSec := encodeTime(generatedAt)

	res, err := tx.ExecContext(ctx, `
		INSERT INTO dossiers (repository_id, repo_owner, repo_name, commit_sha, as_of, section_metadata, snapshot, generated_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, repoID, owner, name, commitSHA, asOfSec, sectionMetadata, snapshot, genSec, genSec)
	if err != nil {
		return 0, fmt.Errorf("insert dossier: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last dossier id: %w", err)
	}
	return id, nil
}

func insertDossierSourcesTx(ctx context.Context, tx *sql.Tx, dossierID int64, sources []domain.SourceRef) error {
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO dossier_sources (dossier_id, source, url, commit_sha, observed_at, as_of)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare dossier sources: %w", err)
	}
	defer stmt.Close()

	for _, src := range sources {
		_, err := stmt.ExecContext(ctx, dossierID, src.Source, src.URL, src.CommitSHA, encodeTime(src.ObservedAt), encodeTime(src.AsOf))
		if err != nil {
			return fmt.Errorf("insert dossier source: %w", err)
		}
	}
	return nil
}

// GetDossier returns the most recent persisted dossier for a repository,
// including its exact source set.
func (c *Corpus) GetDossier(ctx context.Context, owner, name string) (*DossierRecord, []DossierSource, error) {
	record, err := c.getLatestDossierRecord(ctx, owner, name)
	if err != nil {
		return nil, nil, err
	}
	if record == nil {
		return nil, nil, nil
	}
	sources, err := c.getDossierSources(ctx, record.ID)
	if err != nil {
		return nil, nil, err
	}
	return record, sources, nil
}

func (c *Corpus) getLatestDossierRecord(ctx context.Context, owner, name string) (*DossierRecord, error) {
	var r DossierRecord
	var asOf, generated, created int64
	err := c.db.QueryRowContext(ctx, `
		SELECT id, repository_id, repo_owner, repo_name, commit_sha, as_of, section_metadata, snapshot, generated_at, created_at
		FROM dossiers
		WHERE repo_owner = ? AND repo_name = ?
		ORDER BY generated_at DESC, id DESC
		LIMIT 1
	`, owner, name).Scan(&r.ID, &r.RepositoryID, &r.RepoOwner, &r.RepoName, &r.CommitSHA, &asOf, &r.SectionMetadata, &r.Snapshot, &generated, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get dossier: %w", err)
	}
	r.AsOf = scanTime(asOf)
	r.GeneratedAt = scanTime(generated)
	r.CreatedAt = scanTime(created)
	return &r, nil
}

func (c *Corpus) getDossierSources(ctx context.Context, dossierID int64) ([]DossierSource, error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT id, dossier_id, source, url, commit_sha, observed_at, as_of
		FROM dossier_sources
		WHERE dossier_id = ?
		ORDER BY id
	`, dossierID)
	if err != nil {
		return nil, fmt.Errorf("list dossier sources: %w", err)
	}
	defer rows.Close()

	var out []DossierSource
	for rows.Next() {
		var s DossierSource
		var observed, asOf int64
		if err := rows.Scan(&s.ID, &s.DossierID, &s.Source, &s.URL, &s.CommitSHA, &observed, &asOf); err != nil {
			return nil, err
		}
		s.ObservedAt = scanTime(observed)
		s.AsOf = scanTime(asOf)
		out = append(out, s)
	}
	return out, rows.Err()
}

// ListDossiers returns the most recent dossier for each repository up to limit.
func (c *Corpus) ListDossiers(ctx context.Context, limit int) ([]DossierRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		return nil, errors.New("dossier list limit cannot exceed 1000")
	}

	rows, err := c.db.QueryContext(ctx, `
		SELECT id, repository_id, repo_owner, repo_name, commit_sha, as_of, section_metadata, snapshot, generated_at, created_at
		FROM dossiers d
		WHERE d.id = (
			SELECT id FROM dossiers d2
			WHERE d2.repo_owner = d.repo_owner AND d2.repo_name = d.repo_name
			ORDER BY generated_at DESC, id DESC
			LIMIT 1
		)
		ORDER BY generated_at DESC, id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list dossiers: %w", err)
	}
	defer rows.Close()

	var out []DossierRecord
	for rows.Next() {
		var r DossierRecord
		var asOf, generated, created int64
		if err := rows.Scan(&r.ID, &r.RepositoryID, &r.RepoOwner, &r.RepoName, &r.CommitSHA, &asOf, &r.SectionMetadata, &r.Snapshot, &generated, &created); err != nil {
			return nil, err
		}
		r.AsOf = scanTime(asOf)
		r.GeneratedAt = scanTime(generated)
		r.CreatedAt = scanTime(created)
		out = append(out, r)
	}
	return out, rows.Err()
}
