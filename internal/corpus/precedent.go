package corpus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/precedent"
)

const threadProjectionColumns = `id, repository_id, kind, number, state, state_reason, title, body, author, author_association, labels, assignees, draft, locked, milestone,
       source_created_at, source_updated_at, observation_sequence, created_at, updated_at, closed_at, merged_at, merged, merged_known`

// LoadPrecedentRepositories loads each unique repository once, batches all
// requested source numbers for it, and loads its bounded closed history once.
// The returned snapshots follow first repository appearance in refs.
func (c *Corpus) LoadPrecedentRepositories(ctx context.Context, refs []precedent.SourceRef, closedLimit int) (result []precedent.RepositorySnapshot, err error) {
	if closedLimit < 1 || closedLimit > 10_000 {
		return nil, errors.New("closed thread limit must be between 1 and 10000")
	}
	type request struct {
		ref     domain.RepoRef
		numbers []int
	}
	requests := make([]request, 0, len(refs))
	indexByKey := make(map[string]int, len(refs))
	seenNumbers := make(map[string]map[int]struct{}, len(refs))
	for _, ref := range refs {
		key := precedent.RepositoryKey(ref.Repository)
		index, ok := indexByKey[key]
		if !ok {
			index = len(requests)
			indexByKey[key] = index
			requests = append(requests, request{ref: ref.Repository})
			seenNumbers[key] = make(map[int]struct{})
		}
		if _, ok := seenNumbers[key][ref.Number]; !ok {
			seenNumbers[key][ref.Number] = struct{}{}
			requests[index].numbers = append(requests[index].numbers, ref.Number)
		}
	}

	out := make([]precedent.RepositorySnapshot, 0, len(requests))
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer rollbackSQLOnReturn(tx, &err)
	for _, request := range requests {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		snapshot := precedent.RepositorySnapshot{Repository: request.ref, Sources: make(map[int]precedent.Thread)}
		var repositoryID int64
		err = tx.QueryRowContext(ctx, `SELECT id FROM repositories WHERE owner=? AND name=?`, request.ref.Owner, request.ref.Repo).Scan(&repositoryID)
		if errors.Is(err, sql.ErrNoRows) {
			out = append(out, snapshot)
			continue
		}
		if err != nil {
			return nil, err
		}
		snapshot.Available = true
		sources, err := loadThreadsByNumbersTx(ctx, tx, repositoryID, request.numbers)
		if err != nil {
			return nil, err
		}
		for _, source := range sources {
			snapshot.Sources[source.Number] = precedentThread(source)
		}
		closed, err := loadClosedPrecedentsTx(ctx, tx, repositoryID, closedLimit)
		if err != nil {
			return nil, err
		}
		for _, candidate := range closed {
			snapshot.Closed = append(snapshot.Closed, precedentThread(candidate))
		}
		out = append(out, snapshot)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func loadThreadsByNumbersTx(ctx context.Context, tx *sql.Tx, repositoryID int64, numbers []int) (result []Thread, err error) {
	if len(numbers) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(numbers)), ",")
	query := `SELECT ` + threadProjectionColumns + ` FROM threads WHERE repository_id = ? AND number IN (` + placeholders + `)`
	args := make([]any, 0, len(numbers)+1)
	args = append(args, repositoryID)
	for _, number := range numbers {
		args = append(args, number)
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("load precedent sources: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	return scanThreads(rows)
}

func loadClosedPrecedentsTx(ctx context.Context, tx *sql.Tx, repositoryID int64, limit int) (result []Thread, err error) {
	rows, err := tx.QueryContext(ctx, `SELECT `+threadProjectionColumns+`
		FROM threads WHERE repository_id=? AND state='closed'
		ORDER BY source_updated_at DESC, number DESC LIMIT ?`, repositoryID, limit)
	if err != nil {
		return nil, fmt.Errorf("load closed precedents: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	return scanThreads(rows)
}

func precedentThread(thread Thread) precedent.Thread {
	return precedent.Thread{
		ID:          thread.ID,
		Kind:        thread.Kind,
		Number:      thread.Number,
		State:       thread.State,
		StateReason: thread.StateReason,
		Title:       thread.Title,
		Body:        thread.Body,
		Labels:      append([]string(nil), thread.Labels...),
		ClosedAt:    thread.ClosedAt,
		MergedAt:    thread.MergedAt,
		Merged:      thread.Merged,
	}
}
