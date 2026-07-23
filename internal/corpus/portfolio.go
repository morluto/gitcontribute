package corpus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ListPullRequestPortfolio returns pull requests across all stored
// repositories. Author login and state are optional, case-insensitive filters;
// state "all" is equivalent to no state filter. The read is bounded and
// deterministic so callers can build portfolio views without repository-level
// N+1 queries.
func (c *Corpus) ListPullRequestPortfolio(ctx context.Context, author, state string, limit int) (_ []PortfolioPullRequest, err error) {
	page, err := c.ListPullRequestPortfolioPage(ctx, author, state, limit)
	if err != nil {
		return nil, err
	}
	return page.PullRequests, nil
}

// ListPullRequestPortfolioPage returns a bounded portfolio and the exact
// matching population so callers never mistake the page size for the total.
func (c *Corpus) ListPullRequestPortfolioPage(ctx context.Context, author, state string, limit int) (PortfolioPage, error) {
	if limit <= 0 {
		limit = 1000
	}
	if limit > 1000 {
		return PortfolioPage{}, errors.New("pull request portfolio limit cannot exceed 1000")
	}
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return PortfolioPage{}, fmt.Errorf("begin pull request portfolio snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	query := `
		SELECT r.owner, r.name,
		       t.id, t.repository_id, t.kind, t.number, t.state, t.state_reason, t.title, t.body, t.author, t.author_association, t.labels, t.assignees, t.draft, t.locked, t.milestone,
		       t.source_created_at, t.source_updated_at, t.observation_sequence, t.created_at, t.updated_at, t.closed_at, t.merged_at, t.merged, t.merged_known
		FROM threads t
		JOIN repositories r ON r.id = t.repository_id
		WHERE t.kind = ?`
	args := []any{ThreadKindPullRequest}
	if author != "" {
		query += ` AND lower(t.author) = lower(?)`
		args = append(args, author)
	}
	if state != "" && !strings.EqualFold(state, "all") {
		query += ` AND lower(t.state) = lower(?)`
		args = append(args, state)
	}
	countQuery := `SELECT COUNT(*) FROM threads t WHERE t.kind = ?`
	countArgs := []any{ThreadKindPullRequest}
	if author != "" {
		countQuery += ` AND lower(t.author) = lower(?)`
		countArgs = append(countArgs, author)
	}
	if state != "" && !strings.EqualFold(state, "all") {
		countQuery += ` AND lower(t.state) = lower(?)`
		countArgs = append(countArgs, state)
	}
	var total int
	if err := tx.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return PortfolioPage{}, fmt.Errorf("count pull request portfolio: %w", err)
	}
	query += ` ORDER BY t.source_updated_at DESC, r.owner ASC, r.name ASC, t.number ASC LIMIT ?`
	args = append(args, limit)

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return PortfolioPage{}, fmt.Errorf("list pull request portfolio: %w", err)
	}
	var out []PortfolioPullRequest
	for rows.Next() {
		var item PortfolioPullRequest
		var body, authorValue, labels, assignees, stateReason, authorAssociation, milestone sql.NullString
		var sourceCreated, sourceUpdated, created, updated int64
		var closed, mergedAt sql.NullInt64
		var merged, mergedKnown, draft, locked int
		if err := rows.Scan(
			&item.Owner, &item.Repo,
			&item.Thread.ID, &item.Thread.RepositoryID, &item.Thread.Kind, &item.Thread.Number, &item.Thread.State, &stateReason,
			&item.Thread.Title, &body, &authorValue, &authorAssociation, &labels, &assignees, &draft, &locked, &milestone,
			&sourceCreated, &sourceUpdated, &item.Thread.ObservationSequence, &created, &updated, &closed, &mergedAt, &merged, &mergedKnown,
		); err != nil {
			_ = rows.Close()
			return PortfolioPage{}, fmt.Errorf("scan pull request portfolio: %w", err)
		}
		item.Thread.Body = body.String
		item.Thread.StateReason = stateReason.String
		item.Thread.Author = authorValue.String
		item.Thread.AuthorAssociation = authorAssociation.String
		item.Thread.Labels = splitLabels(labels.String)
		item.Thread.Assignees = splitLabels(assignees.String)
		item.Thread.Draft = draft != 0
		item.Thread.Locked = locked != 0
		item.Thread.Milestone = milestone.String
		item.Thread.SourceCreatedAt = scanTime(sourceCreated)
		item.Thread.SourceUpdatedAt = scanTime(sourceUpdated)
		item.Thread.CreatedAt = scanTime(created)
		item.Thread.UpdatedAt = scanTime(updated)
		item.Thread.ClosedAt = scanTime(closed.Int64)
		item.Thread.MergedAt = scanTime(mergedAt.Int64)
		item.Thread.Merged = merged != 0
		item.Thread.MergedKnown = mergedKnown != 0
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return PortfolioPage{}, fmt.Errorf("list pull request portfolio: %w", err)
	}
	if err := rows.Close(); err != nil {
		return PortfolioPage{}, fmt.Errorf("close pull request portfolio rows: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return PortfolioPage{}, fmt.Errorf("commit pull request portfolio snapshot: %w", err)
	}
	return PortfolioPage{PullRequests: out, Total: total, Truncated: len(out) < total}, nil
}
