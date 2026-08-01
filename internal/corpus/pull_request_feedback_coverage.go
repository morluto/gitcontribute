package corpus

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (c *Corpus) feedbackCoverageTx(ctx context.Context, tx *sql.Tx, repositoryID int64, selectedChannel, threadState string) (FeedbackCoverageSummary, error) {
	coverage := FeedbackCoverageSummary{Status: "unknown", Channels: append([]string(nil), feedbackChannels...)}
	var discovery FeedbackDiscovery
	var channelsJSON string
	var complete, truncated int
	var source, updated int64
	err := tx.QueryRowContext(ctx, `SELECT repository_id,generation,state,next_page,complete,truncated,discovered_pull_requests,requests,channels_json,thread_state,last_error,source_updated_at,updated_at FROM pull_request_feedback_discovery WHERE repository_id=?`, repositoryID).Scan(&discovery.RepositoryID, &discovery.Generation, &discovery.State, &discovery.NextPage, &complete, &truncated, &discovery.DiscoveredPullRequests, &discovery.Requests, &channelsJSON, &discovery.ThreadState, &discovery.LastError, &source, &updated)
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
		predicate, args := feedbackCoveragePredicate(channel, threadState)
		if predicate == "" {
			continue
		}
		predicates = append(predicates, predicate)
		coverageArgs = append(coverageArgs, args...)
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
	return coverage, nil
}

// ListPullRequestsWithIncompleteFeedback returns exact local PR identities
// whose selected feedback facets are absent or incomplete. It is used only to
// construct a typed exact-sync recovery action for offline search.
func (c *Corpus) ListPullRequestsWithIncompleteFeedback(ctx context.Context, repositoryID int64, channels []string, threadState string, limit int) ([]Thread, error) {
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
		predicate, predicateArgs := feedbackCoveragePredicate(channel, threadState)
		if predicate == "" {
			continue
		}
		predicates = append(predicates, predicate)
		args = append(args, predicateArgs...)
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

func feedbackCoveragePredicate(channel, threadState string) (string, []any) {
	facet := feedbackFacetForChannel(channel)
	if facet == "" {
		return "", nil
	}
	predicate := "NOT EXISTS (SELECT 1 FROM facet_coverage fc WHERE fc.thread_id=t.id AND fc.facet=? AND fc.complete=1"
	args := []any{facet}
	if channel == "review_threads" {
		selections := []string{"all"}
		if threadState == "unresolved" {
			selections = append(selections, "unresolved")
		}
		placeholders := sqlPlaceholders(len(selections))
		predicate += ` AND EXISTS (
			SELECT 1 FROM facet_observations fo
			WHERE fo.repository_id=t.repository_id AND fo.thread_id=t.id AND fo.facet=?
			  AND COALESCE(NULLIF(json_extract(fo.payload, '$.selection'), ''), 'all') IN (` + placeholders + `)
		)`
		args = append(args, facet)
		for _, selection := range selections {
			args = append(args, selection)
		}
	}
	return predicate + ")", args
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
	result, err := c.db.ExecContext(ctx, `
		INSERT INTO pull_request_feedback_discovery
		    (repository_id, generation, state, next_page, complete, truncated, discovered_pull_requests, requests,
		     channels_json, thread_state, last_error, source_updated_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(repository_id) DO UPDATE SET
		    generation=excluded.generation, state=excluded.state, next_page=excluded.next_page, complete=excluded.complete,
		    truncated=excluded.truncated, discovered_pull_requests=excluded.discovered_pull_requests,
		    requests=excluded.requests, channels_json=excluded.channels_json,
		    thread_state=excluded.thread_state, last_error=excluded.last_error,
		    source_updated_at=excluded.source_updated_at, updated_at=excluded.updated_at
		WHERE excluded.generation > pull_request_feedback_discovery.generation
		   OR (excluded.generation = pull_request_feedback_discovery.generation
		       AND excluded.source_updated_at >= pull_request_feedback_discovery.source_updated_at
		       AND excluded.next_page >= pull_request_feedback_discovery.next_page)
	`, value.RepositoryID, value.Generation, value.State, value.NextPage, boolToInt(value.Complete), boolToInt(value.Truncated),
		value.DiscoveredPullRequests, value.Requests, string(channels), value.ThreadState, value.LastError,
		encodeTime(value.SourceUpdatedAt), encodeTime(value.UpdatedAt))
	if err != nil {
		return fmt.Errorf("upsert feedback discovery: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("feedback discovery rows affected: %w", err)
	} else if affected == 0 {
		return nil
	}
	if err := c.AdvanceFacet(ctx, value.RepositoryID, nil, "pull_request_feedback_discovery", value.SourceUpdatedAt, value.Complete, 0); err != nil {
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
		SELECT repository_id, generation, state, next_page, complete, truncated, discovered_pull_requests, requests,
		       channels_json, thread_state, last_error, source_updated_at, updated_at
		FROM pull_request_feedback_discovery WHERE repository_id = ?
	`, repositoryID).Scan(&value.RepositoryID, &value.Generation, &value.State, &value.NextPage, &complete, &truncated,
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
