package corpus

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	feedbackFacetIssueComments  = "pr_feedback_issue_comments"
	feedbackFacetReviews        = "pr_feedback_reviews"
	feedbackFacetInlineComments = "pr_feedback_inline_comments"
	feedbackFacetReviewThreads  = "pr_feedback_review_threads"
)

var feedbackChannels = []string{"issue_comments", "submitted_reviews", "inline_comments", "review_threads"}

// FeedbackDiscovery is the durable repository-wide discovery checkpoint. A
// next page is intentionally retained when the provider or request/item bound
// stops a job so a retry can safely replay that page without losing items.
type FeedbackDiscovery struct {
	RepositoryID           int64
	State                  string
	NextPage               int
	Complete               bool
	Truncated              bool
	DiscoveredPullRequests int
	Requests               int
	Channels               []string
	ThreadState            string
	LastError              string
	SourceUpdatedAt        time.Time
	UpdatedAt              time.Time
}

// PullRequestFeedbackProjection is one normalized, queryable feedback item.
// The raw facet observation remains canonical; this row is rebuildable.
type PullRequestFeedbackProjection struct {
	ID                        int64
	RepositoryID              int64
	ThreadID                  int64
	PullRequestNumber         int
	PullRequestAuthor         string
	PullRequestState          string
	PullRequestMergedKnown    bool
	PullRequestMerged         bool
	Channel                   string
	FeedbackID                string
	ThreadExternalID          string
	Author                    string
	Body                      string
	Path                      string
	Line                      *int
	StartLine                 *int
	CommitOID                 string
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
	ResolvedKnown             bool
	Resolved                  bool
	Outdated                  bool
	HeadSHA                   string
	SourceUpdatedAt           time.Time
	SourceObservationID       int64
	SourceObservationSequence int64
}

// FeedbackSearchFilter scopes an offline normalized feedback search.
type FeedbackSearchFilter struct {
	RepositoryID      int64
	FeedbackAuthor    string
	PullRequestAuthor string
	State             string
	Merged            string
	ThreadState       string
	Channel           string
	Text              string
	CreatedAfter      time.Time
	CreatedBefore     time.Time
	UpdatedAfter      time.Time
	UpdatedBefore     time.Time
	Sort              string
	Order             string
	Limit             int
	Cursor            string
}

type FeedbackSearchPage struct {
	Items      []PullRequestFeedbackProjection
	Total      int
	NextCursor string
	Truncated  bool
	Coverage   FeedbackCoverageSummary
}

type FeedbackCoverageSummary struct {
	Status            string
	DiscoveryComplete bool
	IncompletePRs     int
	TotalPullRequests int
	Channels          []string
}

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
	from := ` FROM pull_request_feedback_projection p JOIN repositories r ON r.id = p.repository_id JOIN threads t ON t.id = p.thread_id` + joinFTS
	orderExpr := feedbackSortExpression(filter.Sort)
	direction := "ASC"
	if filter.Order == "desc" {
		direction = "DESC"
	}
	statement := `SELECT p.id, p.repository_id, p.thread_id, r.owner, r.name, t.number, t.author, t.state, t.merged, t.merged_known,
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
	for _, channel := range feedbackChannels {
		if value == channel {
			return true
		}
	}
	return false
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
		var owner, repo string
		var number int
		var prAuthor, state string
		var prMerged, prMergedKnown int
		var line, startLine sql.NullInt64
		var created, updated, resolvedKnown, resolved, outdated, source, sequence int64
		if err := rows.Scan(&item.ID, &item.RepositoryID, &item.ThreadID, &owner, &repo, &number, &prAuthor, &state, &prMerged, &prMergedKnown, &item.Channel, &item.FeedbackID, &item.ThreadExternalID, &item.Author, &item.Body, &item.Path, &line, &startLine, &item.CommitOID, &created, &updated, &resolvedKnown, &resolved, &outdated, &item.HeadSHA, &source, &sequence, &item.SourceObservationID); err != nil {
			return nil, fmt.Errorf("scan feedback search row: %w", err)
		}
		// The first merged columns are scanned into temporary values below; the
		// projection's resolution fields are the only child-state fields exposed.
		_ = owner
		_ = repo
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

func (c *Corpus) feedbackCoverageTx(ctx context.Context, tx *sql.Tx, repositoryID int64, selectedChannel string) (FeedbackCoverageSummary, error) {
	coverage := FeedbackCoverageSummary{Status: "unknown", Channels: append([]string(nil), feedbackChannels...)}
	var discovery FeedbackDiscovery
	var channelsJSON string
	var complete, truncated int
	var source, updated int64
	err := tx.QueryRowContext(ctx, `SELECT repository_id,state,next_page,complete,truncated,discovered_pull_requests,requests,channels_json,thread_state,last_error,source_updated_at,updated_at FROM pull_request_feedback_discovery WHERE repository_id=?`, repositoryID).Scan(&discovery.RepositoryID, &discovery.State, &discovery.NextPage, &complete, &truncated, &discovery.DiscoveredPullRequests, &discovery.Requests, &channelsJSON, &discovery.ThreadState, &discovery.LastError, &source, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return coverage, nil
	}
	if err != nil {
		return coverage, err
	}
	discovery.Complete, discovery.Truncated = complete != 0, truncated != 0
	discovery.SourceUpdatedAt, discovery.UpdatedAt = scanTime(source), scanTime(updated)
	if err := json.Unmarshal([]byte(channelsJSON), &discovery.Channels); err != nil {
		return coverage, err
	}
	if selectedChannel != "" {
		coverage.Channels = []string{selectedChannel}
	}
	var total int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM threads WHERE repository_id=? AND kind=?`, repositoryID, ThreadKindPullRequest).Scan(&total); err != nil {
		return coverage, err
	}
	coverage.TotalPullRequests = total
	coverage.DiscoveryComplete = discovery.Complete
	channels := coverage.Channels
	var incomplete int
	predicates := make([]string, 0, len(channels))
	coverageArgs := []any{repositoryID, ThreadKindPullRequest}
	for _, channel := range channels {
		facet := map[string]string{"issue_comments": feedbackFacetIssueComments, "submitted_reviews": feedbackFacetReviews, "inline_comments": feedbackFacetInlineComments, "review_threads": feedbackFacetReviewThreads}[channel]
		if facet != "" {
			predicates = append(predicates, "NOT EXISTS (SELECT 1 FROM facet_coverage fc WHERE fc.thread_id=t.id AND fc.facet=? AND fc.complete=1)")
			coverageArgs = append(coverageArgs, facet)
		}
	}
	if len(predicates) > 0 {
		query := `SELECT COUNT(DISTINCT t.id) FROM threads t WHERE t.repository_id=? AND t.kind=? AND (` + strings.Join(predicates, " OR ") + `)`
		if err := tx.QueryRowContext(ctx, query, coverageArgs...).Scan(&incomplete); err != nil {
			return coverage, err
		}
	}
	coverage.IncompletePRs = incomplete
	if discovery.Complete && incomplete == 0 {
		coverage.Status = "complete"
	} else if discovery.Complete || discovery.DiscoveredPullRequests > 0 {
		coverage.Status = "partial"
	}
	_ = truncated
	return coverage, nil
}

// ListPullRequestsWithIncompleteFeedback returns exact local PR identities
// whose selected feedback facets are absent or incomplete. It is used only to
// construct a typed exact-sync recovery action for offline search.
func (c *Corpus) ListPullRequestsWithIncompleteFeedback(ctx context.Context, repositoryID int64, channels []string, limit int) ([]Thread, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		return nil, errors.New("incomplete feedback limit cannot exceed 1000")
	}
	if len(channels) == 0 {
		channels = feedbackChannels
	}
	var predicates []string
	args := []any{repositoryID, ThreadKindPullRequest}
	for _, channel := range channels {
		facet := map[string]string{"issue_comments": feedbackFacetIssueComments, "submitted_reviews": feedbackFacetReviews, "inline_comments": feedbackFacetInlineComments, "review_threads": feedbackFacetReviewThreads}[channel]
		if facet == "" {
			continue
		}
		predicates = append(predicates, "NOT EXISTS (SELECT 1 FROM facet_coverage fc WHERE fc.thread_id=t.id AND fc.facet=? AND fc.complete=1)")
		args = append(args, facet)
	}
	if len(predicates) == 0 {
		return nil, nil
	}
	query := `SELECT t.id, t.repository_id, t.kind, t.number, t.state, t.state_reason, t.title, t.body, t.author, t.author_association, t.labels, t.assignees, t.draft, t.locked, t.milestone,
		t.source_created_at, t.source_updated_at, t.observation_sequence, t.created_at, t.updated_at, t.closed_at, t.merged_at, t.merged, t.merged_known
		FROM threads t WHERE t.repository_id=? AND t.kind=? AND (` + strings.Join(predicates, " OR ") + `)
		ORDER BY t.number ASC LIMIT ?`
	args = append(args, limit)
	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list incomplete feedback pull requests: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanThreads(rows)
}

// UpsertFeedbackDiscovery persists a checkpoint and the coverage fact used by
// offline search. It is deliberately idempotent for a replayed provider page.
func (c *Corpus) UpsertFeedbackDiscovery(ctx context.Context, value FeedbackDiscovery) error {
	if value.RepositoryID == 0 {
		return errors.New("feedback discovery requires a repository")
	}
	if value.NextPage < 1 {
		value.NextPage = 1
	}
	if value.State == "" {
		value.State = "all"
	}
	if value.ThreadState == "" {
		value.ThreadState = "all"
	}
	channels, err := json.Marshal(value.Channels)
	if err != nil {
		return fmt.Errorf("encode feedback discovery channels: %w", err)
	}
	if value.UpdatedAt.IsZero() {
		value.UpdatedAt = time.Now().UTC()
	}
	_, err = c.db.ExecContext(ctx, `
		INSERT INTO pull_request_feedback_discovery
		    (repository_id, state, next_page, complete, truncated, discovered_pull_requests, requests,
		     channels_json, thread_state, last_error, source_updated_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(repository_id) DO UPDATE SET
		    state=excluded.state, next_page=excluded.next_page, complete=excluded.complete,
		    truncated=excluded.truncated, discovered_pull_requests=excluded.discovered_pull_requests,
		    requests=excluded.requests, channels_json=excluded.channels_json,
		    thread_state=excluded.thread_state, last_error=excluded.last_error,
		    source_updated_at=excluded.source_updated_at, updated_at=excluded.updated_at
		WHERE (pull_request_feedback_discovery.complete = 0 AND excluded.source_updated_at >= pull_request_feedback_discovery.source_updated_at)
		   OR (excluded.complete = 1 AND excluded.source_updated_at >= pull_request_feedback_discovery.source_updated_at)
	`, value.RepositoryID, value.State, value.NextPage, boolToInt(value.Complete), boolToInt(value.Truncated),
		value.DiscoveredPullRequests, value.Requests, string(channels), value.ThreadState, value.LastError,
		encodeTime(value.SourceUpdatedAt), encodeTime(value.UpdatedAt))
	if err != nil {
		return fmt.Errorf("upsert feedback discovery: %w", err)
	}
	complete := value.Complete
	if err := c.AdvanceFacet(ctx, value.RepositoryID, nil, "pull_request_feedback_discovery", value.SourceUpdatedAt, complete, 0); err != nil {
		return fmt.Errorf("advance feedback discovery coverage: %w", err)
	}
	return nil
}

func (c *Corpus) GetFeedbackDiscovery(ctx context.Context, repositoryID int64) (*FeedbackDiscovery, error) {
	var value FeedbackDiscovery
	var channels string
	var complete, truncated int
	var source, updated int64
	err := c.db.QueryRowContext(ctx, `
		SELECT repository_id, state, next_page, complete, truncated, discovered_pull_requests, requests,
		       channels_json, thread_state, last_error, source_updated_at, updated_at
		FROM pull_request_feedback_discovery WHERE repository_id = ?
	`, repositoryID).Scan(&value.RepositoryID, &value.State, &value.NextPage, &complete, &truncated,
		&value.DiscoveredPullRequests, &value.Requests, &channels, &value.ThreadState, &value.LastError, &source, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get feedback discovery: %w", err)
	}
	value.Complete, value.Truncated = complete != 0, truncated != 0
	value.SourceUpdatedAt, value.UpdatedAt = scanTime(source), scanTime(updated)
	if err := json.Unmarshal([]byte(channels), &value.Channels); err != nil {
		return nil, fmt.Errorf("decode feedback discovery channels: %w", err)
	}
	return &value, nil
}

// RebuildPullRequestFeedbackProjection atomically rebuilds normalized feedback
// rows and their FTS index from stored raw observations. Partial observations
// are ignored; a newer incomplete coverage therefore cannot erase the last
// complete searchable snapshot.
func (c *Corpus) RebuildPullRequestFeedbackProjection(ctx context.Context) (ProjectionState, error) {
	started := time.Now().UTC()
	if _, err := c.db.ExecContext(ctx, `
		INSERT INTO projection_states (name, version, status, refreshed_at, row_count, source_revision, content_hash, attempt_status, attempt_started_at, attempt_finished_at, attempt_error)
		VALUES (?, '', ?, NULL, 0, '', '', ?, ?, NULL, '')
		ON CONFLICT(name) DO UPDATE SET status=excluded.status, attempt_status=excluded.attempt_status,
		    attempt_started_at=excluded.attempt_started_at, attempt_finished_at=NULL, attempt_error=''
	`, ProjectionNamePullRequestFeedbackFTS, string(ProjectionStatusBuilding), string(ProjectionAttemptBuilding), encodeTime(started)); err != nil {
		return ProjectionState{}, fmt.Errorf("mark feedback projection building: %w", err)
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return ProjectionState{}, fmt.Errorf("begin feedback projection rebuild: %w", err)
	}
	failed := true
	defer func() {
		if failed {
			failureCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_, _ = c.db.ExecContext(failureCtx, `UPDATE projection_states SET status=?, attempt_status=?, attempt_finished_at=?, attempt_error=? WHERE name=?`, ProjectionStatusFailed, ProjectionAttemptFailed, encodeTime(time.Now().UTC()), "feedback projection rebuild failed", ProjectionNamePullRequestFeedbackFTS)
		}
	}()
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM pull_request_feedback_projection`); err != nil {
		return ProjectionState{}, fmt.Errorf("clear feedback projection: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT fo.id, fo.repository_id, fo.thread_id, fo.facet, fo.source_updated_at, fo.observation_sequence, fo.payload
		FROM facet_observations fo
		JOIN threads t ON t.id = fo.thread_id
		WHERE fo.thread_id IS NOT NULL AND fo.facet IN (?, ?, ?, ?)
		ORDER BY fo.id
	`, feedbackFacetIssueComments, feedbackFacetReviews, feedbackFacetInlineComments, feedbackFacetReviewThreads)
	if err != nil {
		return ProjectionState{}, fmt.Errorf("read feedback observations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var observationID, repositoryID, threadID, source, sequence int64
		var facet, payload string
		if err := rows.Scan(&observationID, &repositoryID, &threadID, &facet, &source, &sequence, &payload); err != nil {
			return ProjectionState{}, fmt.Errorf("scan feedback observation: %w", err)
		}
		items, ok, err := normalizeFeedbackPayload(facet, payload)
		if err != nil {
			return ProjectionState{}, fmt.Errorf("normalize feedback observation %d: %w", observationID, err)
		}
		if !ok {
			continue
		}
		for _, item := range items {
			if err := insertFeedbackProjection(ctx, tx, repositoryID, threadID, facet, observationID, sequence, source, item); err != nil {
				return ProjectionState{}, err
			}
		}
	}
	if err := rows.Err(); err != nil {
		return ProjectionState{}, fmt.Errorf("iterate feedback observations: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO pull_request_feedback_fts (pull_request_feedback_fts) VALUES ('rebuild')`); err != nil {
		return ProjectionState{}, fmt.Errorf("rebuild feedback FTS: %w", err)
	}
	var rowCount int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM pull_request_feedback_projection`).Scan(&rowCount); err != nil {
		return ProjectionState{}, fmt.Errorf("count feedback projection: %w", err)
	}
	sourceRevision, contentHash, err := c.projectionSourceIdentity(ctx, tx, ProjectionNamePullRequestFeedbackFTS)
	if err != nil {
		return ProjectionState{}, err
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `
		UPDATE projection_states SET version=?, status=?, refreshed_at=?, row_count=?, source_revision=?, content_hash=?,
		    attempt_status=?, attempt_started_at=?, attempt_finished_at=?, attempt_error=''
		WHERE name=?
	`, ProjectionVersionPullRequestFeedbackFTS, ProjectionStatusCurrent, encodeTime(now), rowCount, sourceRevision, contentHash,
		ProjectionAttemptSucceeded, encodeTime(started), encodeTime(now), ProjectionNamePullRequestFeedbackFTS); err != nil {
		return ProjectionState{}, fmt.Errorf("update feedback projection state: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE corpus_state SET revision = revision + 1 WHERE id = 1`); err != nil {
		return ProjectionState{}, fmt.Errorf("advance corpus revision after feedback projection: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ProjectionState{}, fmt.Errorf("commit feedback projection: %w", err)
	}
	failed = false
	return ProjectionState{Name: ProjectionNamePullRequestFeedbackFTS, Version: ProjectionVersionPullRequestFeedbackFTS, Status: ProjectionStatusCurrent, RefreshedAt: now, RowCount: rowCount, SourceRevision: sourceRevision, ContentHash: contentHash, AttemptStatus: ProjectionAttemptSucceeded, AttemptStartedAt: started, AttemptFinishedAt: now}, nil
}

type normalizedFeedbackItem struct {
	FeedbackID, ThreadExternalID, Author, Body, Path, CommitOID, HeadSHA string
	Line, StartLine                                                      *int
	CreatedAt, UpdatedAt                                                 time.Time
	ResolvedKnown, Resolved, Outdated                                    bool
}

type feedbackPayloadEnvelope struct {
	HeadSHA  string           `json:"head_sha"`
	Coverage *json.RawMessage `json:"coverage"`
	Items    json.RawMessage  `json:"items"`
}

type feedbackCommentJSON struct {
	ID        int64     `json:"id"`
	NodeID    string    `json:"node_id"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	Path      string    `json:"path"`
	Line      *int      `json:"line"`
	StartLine *int      `json:"start_line"`
	CommitOID string    `json:"commit_oid"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Outdated  bool      `json:"outdated"`
}

type feedbackReviewJSON struct {
	ID          int64     `json:"id"`
	NodeID      string    `json:"node_id"`
	Author      string    `json:"author"`
	Body        string    `json:"body"`
	State       string    `json:"state"`
	CommitOID   string    `json:"commit_oid"`
	SubmittedAt time.Time `json:"submitted_at"`
}

type feedbackThreadJSON struct {
	ID        string                `json:"id"`
	Resolved  bool                  `json:"resolved"`
	Outdated  bool                  `json:"outdated"`
	Path      string                `json:"path"`
	Line      *int                  `json:"line"`
	StartLine *int                  `json:"start_line"`
	Comments  []feedbackCommentJSON `json:"comments"`
}

func normalizeFeedbackPayload(facet, payload string) ([]normalizedFeedbackItem, bool, error) {
	var envelope feedbackPayloadEnvelope
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return nil, false, err
	}
	if envelope.Coverage != nil {
		var coverage struct {
			Complete bool `json:"complete"`
		}
		if err := json.Unmarshal(*envelope.Coverage, &coverage); err != nil {
			return nil, false, err
		}
		if !coverage.Complete {
			return nil, false, nil
		}
	}
	var out []normalizedFeedbackItem
	switch facet {
	case feedbackFacetIssueComments, feedbackFacetInlineComments:
		var values []feedbackCommentJSON
		if err := json.Unmarshal(envelope.Items, &values); err != nil {
			return nil, false, err
		}
		for index, value := range values {
			id := strconv.FormatInt(value.ID, 10)
			if value.ID == 0 {
				id = value.NodeID
			}
			if id == "" {
				id = fmt.Sprintf("item:%d", index)
			}
			out = append(out, normalizedFeedbackItem{FeedbackID: id, Author: value.Author, Body: value.Body, Path: value.Path, Line: value.Line, StartLine: value.StartLine, CommitOID: value.CommitOID, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, Outdated: value.Outdated, HeadSHA: envelope.HeadSHA})
		}
	case feedbackFacetReviews:
		var values []feedbackReviewJSON
		if err := json.Unmarshal(envelope.Items, &values); err != nil {
			return nil, false, err
		}
		for index, value := range values {
			id := strconv.FormatInt(value.ID, 10)
			if value.ID == 0 {
				id = value.NodeID
			}
			if id == "" {
				id = fmt.Sprintf("item:%d", index)
			}
			out = append(out, normalizedFeedbackItem{FeedbackID: id, Author: value.Author, Body: value.Body, CommitOID: value.CommitOID, CreatedAt: value.SubmittedAt, UpdatedAt: value.SubmittedAt, HeadSHA: envelope.HeadSHA})
		}
	case feedbackFacetReviewThreads:
		var values []feedbackThreadJSON
		if err := json.Unmarshal(envelope.Items, &values); err != nil {
			return nil, false, err
		}
		for _, thread := range values {
			if len(thread.Comments) == 0 {
				out = append(out, normalizedFeedbackItem{FeedbackID: "thread:" + thread.ID, ThreadExternalID: thread.ID, Path: thread.Path, Line: thread.Line, StartLine: thread.StartLine, ResolvedKnown: true, Resolved: thread.Resolved, Outdated: thread.Outdated, HeadSHA: envelope.HeadSHA})
				continue
			}
			for index, comment := range thread.Comments {
				id := strconv.FormatInt(comment.ID, 10)
				if comment.ID == 0 {
					id = comment.NodeID
				}
				if id == "" {
					id = fmt.Sprintf("thread:%s:%d", thread.ID, index)
				}
				path := comment.Path
				if path == "" {
					path = thread.Path
				}
				line, startLine := comment.Line, comment.StartLine
				if line == nil {
					line = thread.Line
				}
				if startLine == nil {
					startLine = thread.StartLine
				}
				out = append(out, normalizedFeedbackItem{FeedbackID: id, ThreadExternalID: thread.ID, Author: comment.Author, Body: comment.Body, Path: path, Line: line, StartLine: startLine, CommitOID: comment.CommitOID, CreatedAt: comment.CreatedAt, UpdatedAt: comment.UpdatedAt, ResolvedKnown: true, Resolved: thread.Resolved, Outdated: thread.Outdated || comment.Outdated, HeadSHA: envelope.HeadSHA})
			}
		}
	default:
		return nil, false, fmt.Errorf("unsupported feedback facet %q", facet)
	}
	return out, true, nil
}

func insertFeedbackProjection(ctx context.Context, tx *sql.Tx, repositoryID, threadID int64, facet string, observationID, sequence, source int64, item normalizedFeedbackItem) error {
	channel := map[string]string{
		feedbackFacetIssueComments:  "issue_comments",
		feedbackFacetReviews:        "submitted_reviews",
		feedbackFacetInlineComments: "inline_comments",
		feedbackFacetReviewThreads:  "review_threads",
	}[facet]
	if channel == "" {
		return fmt.Errorf("unsupported feedback facet %q", facet)
	}
	var line, startLine sql.NullInt64
	if item.Line != nil {
		line = sql.NullInt64{Int64: int64(*item.Line), Valid: true}
	}
	if item.StartLine != nil {
		startLine = sql.NullInt64{Int64: int64(*item.StartLine), Valid: true}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO pull_request_feedback_projection
		    (repository_id, thread_id, channel, feedback_id, thread_external_id, author, body, path, line, start_line,
		     commit_oid, created_at, updated_at, resolved_known, resolved, outdated, head_sha, source_updated_at,
		     source_observation_sequence, source_observation_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(repository_id, thread_id, channel, feedback_id, author) DO UPDATE SET
		    thread_external_id=excluded.thread_external_id, body=excluded.body, path=excluded.path,
		    line=excluded.line, start_line=excluded.start_line, commit_oid=excluded.commit_oid,
		    created_at=excluded.created_at, updated_at=excluded.updated_at, resolved_known=excluded.resolved_known,
		    resolved=excluded.resolved, outdated=excluded.outdated, head_sha=excluded.head_sha,
		    source_updated_at=excluded.source_updated_at, source_observation_sequence=excluded.source_observation_sequence,
		    source_observation_id=excluded.source_observation_id
	`, repositoryID, threadID, channel, item.FeedbackID, item.ThreadExternalID, item.Author, item.Body, item.Path,
		line, startLine, item.CommitOID, encodeTime(item.CreatedAt), encodeTime(item.UpdatedAt), boolToInt(item.ResolvedKnown),
		boolToInt(item.Resolved), boolToInt(item.Outdated), item.HeadSHA, source, sequence, observationID); err != nil {
		return fmt.Errorf("insert feedback projection item: %w", err)
	}
	return nil
}
