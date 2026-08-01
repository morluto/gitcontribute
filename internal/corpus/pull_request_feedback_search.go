package corpus

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type feedbackSearchCursor struct {
	Scope  string `json:"scope"`
	Filter string `json:"filter"`
	Offset int    `json:"offset"`
}

type feedbackSearchFilterKey struct {
	RepositoryID      int64  `json:"repository_id"`
	FeedbackAuthor    string `json:"feedback_author,omitempty"`
	PullRequestAuthor string `json:"pull_request_author,omitempty"`
	State             string `json:"state,omitempty"`
	Merged            string `json:"merged,omitempty"`
	ThreadState       string `json:"thread_state,omitempty"`
	Channel           string `json:"channel,omitempty"`
	Text              string `json:"text,omitempty"`
	CreatedAfter      int64  `json:"created_after,omitempty"`
	CreatedBefore     int64  `json:"created_before,omitempty"`
	UpdatedAfter      int64  `json:"updated_after,omitempty"`
	UpdatedBefore     int64  `json:"updated_before,omitempty"`
	Sort              string `json:"sort,omitempty"`
	Order             string `json:"order,omitempty"`
	Limit             int    `json:"limit,omitempty"`
}

func encodeFeedbackCursor(value feedbackSearchCursor) string {
	body, _ := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(body)
}

func decodeFeedbackCursor(value, filter string) (int, error) {
	if value == "" {
		return 0, nil
	}
	body, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return 0, errors.New("invalid feedback search cursor")
	}
	var cursor feedbackSearchCursor
	if err := json.Unmarshal(body, &cursor); err != nil || cursor.Scope != "pull_request_feedback" || cursor.Filter != filter || cursor.Offset < 0 {
		return 0, errors.New("feedback search cursor does not match this query")
	}
	return cursor.Offset, nil
}

// SearchPullRequestFeedback performs a deterministic local read over the
// normalized feedback projection. It never refreshes facets or contacts GitHub.
func (c *Corpus) SearchPullRequestFeedback(ctx context.Context, filter FeedbackSearchFilter) (FeedbackSearchPage, error) {
	if filter.Limit == 0 {
		filter.Limit = 20
	}
	if filter.Limit < 1 || filter.Limit > 100 {
		return FeedbackSearchPage{}, errors.New("feedback search limit must be between 1 and 100")
	}
	if filter.State == "" {
		filter.State = "all"
	}
	if filter.State != "open" && filter.State != "closed" && filter.State != "all" {
		return FeedbackSearchPage{}, errors.New("feedback search state must be open, closed, or all")
	}
	if filter.Merged == "" {
		filter.Merged = "any"
	}
	if filter.Merged != "true" && filter.Merged != "false" && filter.Merged != "unknown" && filter.Merged != "any" {
		return FeedbackSearchPage{}, errors.New("feedback search merged must be true, false, unknown, or any")
	}
	if filter.ThreadState == "" {
		filter.ThreadState = "all"
	}
	if filter.ThreadState != "resolved" && filter.ThreadState != "unresolved" && filter.ThreadState != "all" {
		return FeedbackSearchPage{}, errors.New("feedback search thread_state must be resolved, unresolved, or all")
	}
	if filter.Sort == "" {
		filter.Sort = "updated"
	}
	switch filter.Sort {
	case "feedback_author", "pull_request_state", "merge_state", "created", "updated", "pull_request_number":
	default:
		return FeedbackSearchPage{}, errors.New("feedback search sort is unsupported")
	}
	if filter.Order == "" {
		filter.Order = "desc"
	}
	if filter.Order != "asc" && filter.Order != "desc" {
		return FeedbackSearchPage{}, errors.New("feedback search order must be asc or desc")
	}
	if filter.Channel != "" && !validFeedbackChannel(filter.Channel) {
		return FeedbackSearchPage{}, fmt.Errorf("unsupported feedback channel %q", filter.Channel)
	}
	if err := c.RequireProjection(ctx, ProjectionNamePullRequestFeedbackFTS, ProjectionVersionPullRequestFeedbackFTS); err != nil {
		return FeedbackSearchPage{}, err
	}
	filterKeyBytes, _ := json.Marshal(feedbackSearchFilterKey{
		RepositoryID: filter.RepositoryID, FeedbackAuthor: filter.FeedbackAuthor,
		PullRequestAuthor: filter.PullRequestAuthor, State: filter.State, Merged: filter.Merged,
		ThreadState: filter.ThreadState, Channel: filter.Channel, Text: filter.Text,
		CreatedAfter: encodeTime(filter.CreatedAfter), CreatedBefore: encodeTime(filter.CreatedBefore),
		UpdatedAfter: encodeTime(filter.UpdatedAfter), UpdatedBefore: encodeTime(filter.UpdatedBefore),
		Sort: filter.Sort, Order: filter.Order, Limit: filter.Limit,
	})
	offset, err := decodeFeedbackCursor(filter.Cursor, string(filterKeyBytes))
	if err != nil {
		return FeedbackSearchPage{}, err
	}

	where, args, ftsQuery := feedbackSearchWhere(filter)
	joinFTS := ""
	if ftsQuery != "" {
		joinFTS = " JOIN pull_request_feedback_fts ON pull_request_feedback_fts.rowid = p.id"
		args = append([]any{ftsQuery}, args...)
	}
	from := ` FROM pull_request_feedback_projection p JOIN threads t ON t.id = p.thread_id` + joinFTS
	orderExpr := feedbackSortExpression(filter.Sort)
	direction := "ASC"
	if filter.Order == "desc" {
		direction = "DESC"
	}
	statement := `SELECT p.id, p.repository_id, p.thread_id, t.number, t.author, t.state, t.merged, t.merged_known,
		p.channel, p.feedback_id, p.thread_external_id, p.author, p.body, p.path, p.line, p.start_line, p.commit_oid,
		p.created_at, p.updated_at, p.resolved_known, p.resolved, p.outdated, p.head_sha, p.source_updated_at,
		p.source_observation_sequence, p.source_observation_id` + from + where + ` ORDER BY ` + orderExpr + ` ` + direction + `, p.id ` + direction + ` LIMIT ? OFFSET ?`
	queryArgs := append([]any(nil), args...)
	queryArgs = append(queryArgs, filter.Limit+1, offset)
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return FeedbackSearchPage{}, fmt.Errorf("begin feedback search: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, statement, queryArgs...)
	if err != nil {
		return FeedbackSearchPage{}, fmt.Errorf("search pull-request feedback: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items, err := scanFeedbackProjectionRows(rows)
	if err != nil {
		return FeedbackSearchPage{}, err
	}
	countArgs := append([]any(nil), args...)
	var total int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*)`+from+where, countArgs...).Scan(&total); err != nil {
		return FeedbackSearchPage{}, fmt.Errorf("count pull-request feedback: %w", err)
	}
	page := FeedbackSearchPage{Items: items, Total: total}
	if len(items) > filter.Limit {
		page.Items = items[:filter.Limit]
		page.NextCursor = encodeFeedbackCursor(feedbackSearchCursor{Scope: "pull_request_feedback", Filter: string(filterKeyBytes), Offset: offset + filter.Limit})
		page.Truncated = true
	}
	page.Coverage, err = c.feedbackCoverageTx(ctx, tx, filter.RepositoryID, filter.Channel)
	if err != nil {
		return FeedbackSearchPage{}, err
	}
	if err := tx.Commit(); err != nil {
		return FeedbackSearchPage{}, fmt.Errorf("commit feedback search: %w", err)
	}
	return page, nil
}

func validFeedbackChannel(value string) bool {
	return feedbackFacetForChannel(value) != ""
}

func feedbackSearchWhere(filter FeedbackSearchFilter) (string, []any, string) {
	where := " WHERE 1=1"
	args := make([]any, 0, 12)
	if filter.RepositoryID != 0 {
		where += " AND p.repository_id = ?"
		args = append(args, filter.RepositoryID)
	}
	if filter.FeedbackAuthor != "" {
		where += " AND lower(p.author) = lower(?)"
		args = append(args, filter.FeedbackAuthor)
	}
	if filter.PullRequestAuthor != "" {
		where += " AND lower(t.author) = lower(?)"
		args = append(args, filter.PullRequestAuthor)
	}
	if filter.State != "all" {
		where += " AND t.state = ?"
		args = append(args, filter.State)
	}
	switch filter.Merged {
	case "true":
		where += " AND t.merged_known = 1 AND t.merged = 1"
	case "false":
		where += " AND t.merged_known = 1 AND t.merged = 0"
	case "unknown":
		where += " AND t.merged_known = 0"
	}
	if filter.ThreadState == "resolved" {
		where += " AND p.resolved_known = 1 AND p.resolved = 1"
	}
	if filter.ThreadState == "unresolved" {
		where += " AND p.resolved_known = 1 AND p.resolved = 0"
	}
	if filter.Channel != "" {
		where += " AND p.channel = ?"
		args = append(args, filter.Channel)
	}
	if !filter.CreatedAfter.IsZero() {
		where += " AND p.created_at >= ?"
		args = append(args, encodeTime(filter.CreatedAfter))
	}
	if !filter.CreatedBefore.IsZero() {
		where += " AND p.created_at <= ?"
		args = append(args, encodeTime(filter.CreatedBefore))
	}
	if !filter.UpdatedAfter.IsZero() {
		where += " AND p.updated_at >= ?"
		args = append(args, encodeTime(filter.UpdatedAfter))
	}
	if !filter.UpdatedBefore.IsZero() {
		where += " AND p.updated_at <= ?"
		args = append(args, encodeTime(filter.UpdatedBefore))
	}
	ftsQuery := literalFTSQueryMode(filter.Text, "all")
	if ftsQuery != "" {
		where = " WHERE pull_request_feedback_fts MATCH ?" + strings.TrimPrefix(where, " WHERE 1=1")
	}
	return where, args, ftsQuery
}

func feedbackSortExpression(sort string) string {
	switch sort {
	case "feedback_author":
		return "lower(p.author)"
	case "pull_request_state":
		return "t.state"
	case "merge_state":
		return "CASE WHEN t.merged_known = 0 THEN 0 WHEN t.merged = 0 THEN 1 ELSE 2 END"
	case "created":
		return "p.created_at"
	case "pull_request_number":
		return "t.number"
	default:
		return "p.updated_at"
	}
}

func scanFeedbackProjectionRows(rows *sql.Rows) ([]PullRequestFeedbackProjection, error) {
	var out []PullRequestFeedbackProjection
	for rows.Next() {
		var item PullRequestFeedbackProjection
		var number int
		var prAuthor, state string
		var prMerged, prMergedKnown int
		var line, startLine sql.NullInt64
		var created, updated, resolvedKnown, resolved, outdated, source, sequence int64
		if err := rows.Scan(&item.ID, &item.RepositoryID, &item.ThreadID, &number, &prAuthor, &state, &prMerged, &prMergedKnown, &item.Channel, &item.FeedbackID, &item.ThreadExternalID, &item.Author, &item.Body, &item.Path, &line, &startLine, &item.CommitOID, &created, &updated, &resolvedKnown, &resolved, &outdated, &item.HeadSHA, &source, &sequence, &item.SourceObservationID); err != nil {
			return nil, fmt.Errorf("scan feedback search row: %w", err)
		}
		item.PullRequestNumber, item.PullRequestAuthor, item.PullRequestState = number, prAuthor, state
		item.PullRequestMergedKnown, item.PullRequestMerged = prMergedKnown != 0, prMerged != 0
		item.Line, item.StartLine = nullableInt(line), nullableInt(startLine)
		item.CreatedAt, item.UpdatedAt = scanTime(created), scanTime(updated)
		item.ResolvedKnown, item.Resolved, item.Outdated = resolvedKnown != 0, resolved != 0, outdated != 0
		item.SourceUpdatedAt, item.SourceObservationSequence = scanTime(source), sequence
		out = append(out, item)
	}
	return out, rows.Err()
}

func nullableInt(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	out := int(value.Int64)
	return &out
}
