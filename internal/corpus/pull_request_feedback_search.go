package corpus

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
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
func (c *Corpus) SearchPullRequestFeedback(ctx context.Context, request FeedbackSearchRequest) (FeedbackSearchPage, error) {
	query := request.query
	if err := c.RequireFreshProjection(ctx, ProjectionNamePullRequestFeedbackFTS, ProjectionVersionPullRequestFeedbackFTS); err != nil {
		return FeedbackSearchPage{}, err
	}
	filterKeyBytes, _ := json.Marshal(feedbackSearchFilterKey{
		RepositoryID: request.repositoryID, FeedbackAuthor: query.feedbackAuthor,
		PullRequestAuthor: query.pullRequestAuthor, State: query.state.String(), Merged: query.merge.BooleanString(),
		ThreadState: query.threadState.String(), Channel: query.channel.String(), Text: query.text,
		CreatedAfter: encodeTime(query.createdAfter), CreatedBefore: encodeTime(query.createdBefore),
		UpdatedAfter: encodeTime(query.updatedAfter), UpdatedBefore: encodeTime(query.updatedBefore),
		Sort: query.sort.String(), Order: query.order.String(), Limit: query.page.Limit(),
	})
	offset, err := decodeFeedbackCursor(query.page.Cursor(), string(filterKeyBytes))
	if err != nil {
		return FeedbackSearchPage{}, err
	}

	where, args, ftsQuery := feedbackSearchWhere(request)
	joinFTS := ""
	if ftsQuery != "" {
		joinFTS = " JOIN pull_request_feedback_fts ON pull_request_feedback_fts.rowid = p.id"
		args = append([]any{ftsQuery}, args...)
	}
	from := ` FROM pull_request_feedback_projection p JOIN threads t ON t.id = p.thread_id` + joinFTS
	orderExpr := query.sort.expression()
	direction := query.order.sqlDirection()
	statement := `SELECT p.id, p.repository_id, p.thread_id, t.number, t.author, t.state, t.merged, t.merged_known,
		p.channel, p.feedback_id, p.feedback_node_id, p.thread_external_id, p.in_reply_to_id, p.author, p.body, p.path, p.line, p.start_line, p.side, p.start_side, p.commit_oid, p.review_state,
		p.created_at, p.updated_at, p.resolved_known, p.resolved, p.resolved_by, p.outdated, p.head_sha, p.source_updated_at,
		p.source_observation_sequence, p.source_observation_id` + from + where + ` ORDER BY ` + orderExpr + ` ` + direction + `, p.id ` + direction + ` LIMIT ? OFFSET ?`
	queryArgs := append([]any(nil), args...)
	queryArgs = append(queryArgs, query.page.Limit()+1, offset)
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
	if query.merge.IsMerged() || query.merge.IsUnmerged() {
		unknownRequest := request
		unknownRequest.query.merge = AnyMergeState()
		unknownWhere, unknownArgs, unknownFTSQuery := feedbackSearchWhere(unknownRequest)
		unknownWhere += " AND t.merged_known = 0"
		if unknownFTSQuery != "" {
			unknownArgs = append([]any{unknownFTSQuery}, unknownArgs...)
		}
		unknownRows, err := tx.QueryContext(ctx, `SELECT DISTINCT t.number`+from+unknownWhere+` ORDER BY t.number ASC LIMIT 50`, unknownArgs...)
		if err != nil {
			return FeedbackSearchPage{}, fmt.Errorf("find unknown pull-request merge states: %w", err)
		}
		defer func() { _ = unknownRows.Close() }()
		for unknownRows.Next() {
			var number int
			if err := unknownRows.Scan(&number); err != nil {
				return FeedbackSearchPage{}, fmt.Errorf("scan unknown pull-request merge state: %w", err)
			}
			page.UnknownMergePullRequests = append(page.UnknownMergePullRequests, number)
		}
		if err := unknownRows.Err(); err != nil {
			return FeedbackSearchPage{}, fmt.Errorf("iterate unknown pull-request merge states: %w", err)
		}
	}
	if len(items) > query.page.Limit() {
		page.Items = items[:query.page.Limit()]
		page.NextCursor = encodeFeedbackCursor(feedbackSearchCursor{Scope: "pull_request_feedback", Filter: string(filterKeyBytes), Offset: offset + query.page.Limit()})
		page.Truncated = true
	}
	page.Coverage, err = c.feedbackCoverageTx(ctx, tx, request.repositoryID, query.channel, query.threadState, query.state)
	if err != nil {
		return FeedbackSearchPage{}, err
	}
	if err := tx.Commit(); err != nil {
		return FeedbackSearchPage{}, fmt.Errorf("commit feedback search: %w", err)
	}
	return page, nil
}

// GetPullRequestFeedbackItem returns one exact normalized feedback record.
// It is an offline read over the same projection used by search; callers do
// not need to parse a raw facet payload to follow a search match.
func (c *Corpus) GetPullRequestFeedbackItem(ctx context.Context, repositoryID int64, number int, channel FeedbackChannel, feedbackID string) (*PullRequestFeedbackProjection, error) {
	if err := c.RequireFreshProjection(ctx, ProjectionNamePullRequestFeedbackFTS, ProjectionVersionPullRequestFeedbackFTS); err != nil {
		return nil, err
	}
	rows, err := c.db.QueryContext(ctx, `SELECT p.id, p.repository_id, p.thread_id, t.number, t.author, t.state, t.merged, t.merged_known,
		p.channel, p.feedback_id, p.feedback_node_id, p.thread_external_id, p.in_reply_to_id, p.author, p.body, p.path, p.line, p.start_line, p.side, p.start_side, p.commit_oid, p.review_state,
		p.created_at, p.updated_at, p.resolved_known, p.resolved, p.resolved_by, p.outdated, p.head_sha, p.source_updated_at,
		p.source_observation_sequence, p.source_observation_id
		FROM pull_request_feedback_projection p JOIN threads t ON t.id = p.thread_id
		WHERE p.repository_id = ? AND t.number = ? AND p.channel = ? AND p.feedback_id = ?
		ORDER BY p.id LIMIT 1`, repositoryID, number, channel.String(), feedbackID)
	if err != nil {
		return nil, fmt.Errorf("get pull-request feedback item: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items, err := scanFeedbackProjectionRows(rows)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	return &items[0], nil
}

func feedbackSearchWhere(request FeedbackSearchRequest) (string, []any, string) {
	query := request.query
	where := " WHERE 1=1"
	args := make([]any, 0, 12)
	where += " AND p.repository_id = ?"
	args = append(args, request.repositoryID)
	if query.feedbackAuthor != "" {
		where += " AND lower(p.author) = lower(?)"
		args = append(args, query.feedbackAuthor)
	}
	if query.pullRequestAuthor != "" {
		where += " AND lower(t.author) = lower(?)"
		args = append(args, query.pullRequestAuthor)
	}
	if !query.state.IsAny() {
		where += " AND t.state = ?"
		args = append(args, query.state.String())
	}
	switch {
	case query.merge.IsMerged():
		where += " AND t.merged_known = 1 AND t.merged = 1"
	case query.merge.IsUnmerged():
		where += " AND t.merged_known = 1 AND t.merged = 0"
	case query.merge.IsUnknown():
		where += " AND t.merged_known = 0"
	}
	if query.threadState.isResolved() {
		where += " AND p.resolved_known = 1 AND p.resolved = 1"
	}
	if !query.threadState.isAny() && !query.threadState.isResolved() {
		where += " AND p.resolved_known = 1 AND p.resolved = 0"
	}
	if query.channel != 0 {
		where += " AND p.channel = ?"
		args = append(args, query.channel.String())
	}
	if !query.createdAfter.IsZero() {
		where += " AND p.created_at >= ?"
		args = append(args, encodeTime(query.createdAfter))
	}
	if !query.createdBefore.IsZero() {
		where += " AND p.created_at <= ?"
		args = append(args, encodeTime(query.createdBefore))
	}
	if !query.updatedAfter.IsZero() {
		where += " AND p.updated_at >= ?"
		args = append(args, encodeTime(query.updatedAfter))
	}
	if !query.updatedBefore.IsZero() {
		where += " AND p.updated_at <= ?"
		args = append(args, encodeTime(query.updatedBefore))
	}
	ftsQuery := literalFTSQueryMode(query.text, MatchAllTerms())
	if ftsQuery != "" {
		where = " WHERE pull_request_feedback_fts MATCH ?" + strings.TrimPrefix(where, " WHERE 1=1")
	}
	return where, args, ftsQuery
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
		if err := rows.Scan(&item.ID, &item.RepositoryID, &item.ThreadID, &number, &prAuthor, &state, &prMerged, &prMergedKnown, &item.Channel, &item.FeedbackID, &item.FeedbackNodeID, &item.ThreadExternalID, &item.InReplyToID, &item.Author, &item.Body, &item.Path, &line, &startLine, &item.Side, &item.StartSide, &item.CommitOID, &item.ReviewState, &created, &updated, &resolvedKnown, &resolved, &item.ResolvedBy, &outdated, &item.HeadSHA, &source, &sequence, &item.SourceObservationID); err != nil {
			return nil, fmt.Errorf("scan feedback search row: %w", err)
		}
		item.PullRequestNumber, item.PullRequestAuthor, item.PullRequestState = number, prAuthor, state
		merge, err := domain.ParseMergeStatus(prMergedKnown != 0, prMerged != 0, time.Time{})
		if err != nil {
			return nil, fmt.Errorf("parse stored pull-request merge status: %w", err)
		}
		item.PullRequestMerge = merge
		item.Line, item.StartLine = nullableInt(line), nullableInt(startLine)
		item.CreatedAt, item.UpdatedAt = scanTime(created), scanTime(updated)
		item.Resolution = domain.UnknownBool()
		if resolvedKnown != 0 {
			item.Resolution = domain.ObservedBoolValue(resolved != 0)
		}
		item.Outdated = outdated != 0
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
