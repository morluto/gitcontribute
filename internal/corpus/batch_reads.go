package corpus

import (
	"context"
	"fmt"
	"strings"
)

// RepositoryKey identifies a repository projection in a batch result.
type RepositoryKey struct {
	Owner string
	Name  string
}

// ThreadKey identifies a thread projection in a batch result. An empty Kind
// requests the thread regardless of whether it is an issue or pull request.
type ThreadKey struct {
	RepositoryID int64
	Kind         string
	Number       int
}

// GetRepositoriesBatch reads up to 100 repository projections in one query.
// Missing repositories are absent from the returned map.
func (c *Corpus) GetRepositoriesBatch(ctx context.Context, keys []RepositoryKey) (map[RepositoryKey]*Repository, error) {
	if len(keys) > maxBatchReadItems {
		return nil, fmt.Errorf("repository batch cannot exceed %d items", maxBatchReadItems)
	}
	if len(keys) == 0 {
		return map[RepositoryKey]*Repository{}, nil
	}
	clauses := make([]string, len(keys))
	args := make([]any, 0, len(keys)*2)
	for i, key := range keys {
		clauses[i] = "(owner = ? AND name = ?)"
		args = append(args, key.Owner, key.Name)
	}
	rows, err := c.db.QueryContext(ctx, `
		SELECT id, owner, name, external_id, description, default_branch, language, license, topics, stars, watchers, forks, open_issues, archived, fork, source_created_at, source_updated_at, observation_sequence, created_at, updated_at
		FROM repositories
		WHERE `+strings.Join(clauses, " OR "), args...)
	if err != nil {
		return nil, fmt.Errorf("get repositories batch: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[RepositoryKey]*Repository, len(keys))
	for rows.Next() {
		repo, err := scanRepository(rows)
		if err != nil {
			return nil, fmt.Errorf("scan repositories batch: %w", err)
		}
		out[RepositoryKey{Owner: repo.Owner, Name: repo.Name}] = repo
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate repositories batch: %w", err)
	}
	return out, nil
}

// GetLatestDossierMetadataBatch returns the latest persisted dossier metadata
// for each requested repository ID without loading dossier snapshots.
func (c *Corpus) GetLatestDossierMetadataBatch(ctx context.Context, repositoryIDs []int64) (map[int64]DossierMetadata, error) {
	if len(repositoryIDs) > maxBatchReadItems {
		return nil, fmt.Errorf("dossier metadata batch cannot exceed %d items", maxBatchReadItems)
	}
	if len(repositoryIDs) == 0 {
		return map[int64]DossierMetadata{}, nil
	}
	placeholders := make([]string, len(repositoryIDs))
	args := make([]any, len(repositoryIDs))
	for i, repositoryID := range repositoryIDs {
		placeholders[i] = "?"
		args[i] = repositoryID
	}
	rows, err := c.db.QueryContext(ctx, `
		SELECT d.repository_id, d.as_of, d.generated_at
		FROM dossiers d
		WHERE d.repository_id IN (`+strings.Join(placeholders, ",")+`)
		  AND d.id = (
			  SELECT d2.id
			  FROM dossiers d2
			  WHERE d2.repository_id = d.repository_id
			  ORDER BY d2.generated_at DESC, d2.id DESC
			  LIMIT 1
		  )
		ORDER BY d.repository_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("get dossier metadata batch: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[int64]DossierMetadata, len(repositoryIDs))
	for rows.Next() {
		var metadata DossierMetadata
		var asOf, generatedAt int64
		if err := rows.Scan(&metadata.RepositoryID, &asOf, &generatedAt); err != nil {
			return nil, fmt.Errorf("scan dossier metadata batch: %w", err)
		}
		metadata.AsOf = scanTime(asOf)
		metadata.GeneratedAt = scanTime(generatedAt)
		out[metadata.RepositoryID] = metadata
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dossier metadata batch: %w", err)
	}
	return out, nil
}

// GetThreadsBatch reads up to 100 exact thread projections in one query.
// Missing threads are absent from the returned map.
func (c *Corpus) GetThreadsBatch(ctx context.Context, keys []ThreadKey) (map[ThreadKey]*Thread, error) {
	if len(keys) > maxBatchReadItems {
		return nil, fmt.Errorf("thread batch cannot exceed %d items", maxBatchReadItems)
	}
	if len(keys) == 0 {
		return map[ThreadKey]*Thread{}, nil
	}
	clauses := make([]string, len(keys))
	args := make([]any, 0, len(keys)*3)
	for i, key := range keys {
		if key.Kind == "" {
			clauses[i] = "(repository_id = ? AND number = ?)"
			args = append(args, key.RepositoryID, key.Number)
		} else {
			clauses[i] = "(repository_id = ? AND kind = ? AND number = ?)"
			args = append(args, key.RepositoryID, key.Kind, key.Number)
		}
	}
	rows, err := c.db.QueryContext(ctx, `
		SELECT id, repository_id, kind, number, state, state_reason, title, body, author, author_association, labels, assignees, draft, locked, milestone,
		       source_created_at, source_updated_at, observation_sequence, created_at, updated_at, closed_at, merged_at, merged, merged_known
		FROM threads
		WHERE `+strings.Join(clauses, " OR "), args...)
	if err != nil {
		return nil, fmt.Errorf("get threads batch: %w", err)
	}
	defer func() { _ = rows.Close() }()

	threads, err := scanThreads(rows)
	if err != nil {
		return nil, fmt.Errorf("scan threads batch: %w", err)
	}
	out := make(map[ThreadKey]*Thread, len(keys))
	requested := make(map[ThreadKey]struct{}, len(keys))
	for _, key := range keys {
		requested[key] = struct{}{}
	}
	for i := range threads {
		thread := &threads[i]
		exact := ThreadKey{RepositoryID: thread.RepositoryID, Kind: thread.Kind, Number: thread.Number}
		if _, ok := requested[exact]; ok {
			out[exact] = thread
		}
		anyKind := ThreadKey{RepositoryID: thread.RepositoryID, Number: thread.Number}
		if _, ok := requested[anyKind]; ok {
			out[anyKind] = thread
		}
	}
	return out, nil
}

func scanRepository(row rowScanner) (*Repository, error) {
	var repo Repository
	var sourceCreated, sourceUpdated, created, updated int64
	var archived, fork int
	var topics string
	if err := row.Scan(
		&repo.ID, &repo.Owner, &repo.Name, &repo.ExternalID, &repo.Description,
		&repo.DefaultBranch, &repo.Language, &repo.License, &topics, &repo.Stars,
		&repo.Watchers, &repo.Forks, &repo.OpenIssues, &archived, &fork,
		&sourceCreated, &sourceUpdated, &repo.ObservationSequence, &created, &updated,
	); err != nil {
		return nil, err
	}
	repo.Topics = splitLabels(topics)
	repo.Archived = archived != 0
	repo.Fork = fork != 0
	repo.SourceCreatedAt = scanTime(sourceCreated)
	repo.SourceUpdatedAt = scanTime(sourceUpdated)
	repo.CreatedAt = scanTime(created)
	repo.UpdatedAt = scanTime(updated)
	return &repo, nil
}
