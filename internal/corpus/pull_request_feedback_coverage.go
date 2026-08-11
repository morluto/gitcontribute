package corpus

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

func (c *Corpus) feedbackCoverageTx(ctx context.Context, tx *sql.Tx, repositoryID int64, selectedChannel FeedbackChannel, threadState feedbackThreadState) (FeedbackCoverageSummary, error) {
	coverage := FeedbackCoverageSummary{State: FeedbackCoverageUnknown, Channels: AllFeedbackSelection().Channels()}
	discovery, err := scanFeedbackDiscovery(tx.QueryRowContext(ctx, `SELECT repository_id,generation,state,next_page,complete,truncated,discovered_pull_requests,requests,channels_json,thread_state,last_error,source_updated_at,updated_at FROM pull_request_feedback_discovery WHERE repository_id=?`, repositoryID))
	if errors.Is(err, sql.ErrNoRows) {
		return coverage, nil
	}
	if err != nil {
		return coverage, err
	}
	if selectedChannel != 0 {
		coverage.Channels = []string{selectedChannel.String()}
	}
	var total int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM threads WHERE repository_id=? AND kind=?`, repositoryID, domain.PullRequestKind).Scan(&total); err != nil {
		return coverage, err
	}
	coverage.TotalPullRequests = total
	discoveryComplete := discovery.IsComplete()
	if (threadState.isAny() || threadState.isResolved()) && discovery.Selection.threadState != AllFeedbackThreads {
		discoveryComplete = false
	}
	channels := coverage.Channels
	var incomplete int
	predicates := make([]string, 0, len(channels))
	coverageArgs := []any{repositoryID, domain.PullRequestKind}
	for _, value := range channels {
		channel, err := ParseFeedbackChannel(value)
		if err != nil {
			return coverage, err
		}
		if !discovery.Selection.Includes(channel) {
			discoveryComplete = false
		}
		predicate, args := feedbackCoveragePredicate(channel, threadState)
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
	if discoveryComplete && incomplete == 0 {
		coverage.State = FeedbackCoverageComplete
	} else if discovery.IsComplete() || discovery.DiscoveredPullRequests > 0 {
		if discoveryComplete {
			coverage.State = FeedbackCoveragePartialFacets
		} else {
			coverage.State = FeedbackCoveragePartialDiscovery
		}
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
		channels = AllFeedbackSelection().Channels()
	}
	parsedThreadState, err := parseFeedbackThreadState(threadState)
	if err != nil {
		return nil, err
	}
	var predicates []string
	args := []any{repositoryID, domain.PullRequestKind}
	seen := make(map[FeedbackChannel]struct{}, len(channels))
	for _, value := range channels {
		channel, err := ParseFeedbackChannel(value)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[channel]; exists {
			return nil, fmt.Errorf("duplicate feedback channel %q", value)
		}
		seen[channel] = struct{}{}
		predicate, predicateArgs := feedbackCoveragePredicate(channel, parsedThreadState)
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

func feedbackCoveragePredicate(channel FeedbackChannel, threadState feedbackThreadState) (string, []any) {
	facet := feedbackFacet(channel)
	predicate := "NOT EXISTS (SELECT 1 FROM facet_coverage fc WHERE fc.thread_id=t.id AND fc.facet=? AND fc.complete=1"
	args := []any{facet}
	if channel == FeedbackReviewThreads {
		selections := []string{"all"}
		if !threadState.isAny() && !threadState.isResolved() {
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
	if value.RepositoryID <= 0 {
		return errors.New("feedback discovery requires a repository")
	}
	if !value.Selection.Valid() {
		return errors.New("feedback discovery requires a parsed selection")
	}
	if !value.State.valid() {
		return fmt.Errorf("feedback discovery has unsupported state %q", value.State)
	}
	if value.NextPage < 1 {
		value.NextPage = 1
	}
	channels, err := json.Marshal(value.Selection.Channels())
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
	`, value.RepositoryID, value.Generation, "all", value.NextPage, boolToInt(value.IsComplete()), boolToInt(value.IsTruncated()),
		value.DiscoveredPullRequests, value.Requests, string(channels), value.Selection.ThreadState(), value.LastError,
		encodeTime(value.SourceUpdatedAt), encodeTime(value.UpdatedAt))
	if err != nil {
		return fmt.Errorf("upsert feedback discovery: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("feedback discovery rows affected: %w", err)
	} else if affected == 0 {
		return nil
	}
	if err := c.AdvanceFacet(ctx, value.RepositoryID, nil, "pull_request_feedback_discovery", value.SourceUpdatedAt, value.IsComplete(), 0); err != nil {
		return fmt.Errorf("advance feedback discovery coverage: %w", err)
	}
	return nil
}

func (c *Corpus) GetFeedbackDiscovery(ctx context.Context, repositoryID int64) (*FeedbackDiscovery, error) {
	value, err := scanFeedbackDiscovery(c.db.QueryRowContext(ctx, `
		SELECT repository_id, generation, state, next_page, complete, truncated, discovered_pull_requests, requests,
		       channels_json, thread_state, last_error, source_updated_at, updated_at
		FROM pull_request_feedback_discovery WHERE repository_id = ?
	`, repositoryID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get feedback discovery: %w", err)
	}
	return value, nil
}

func scanFeedbackDiscovery(row rowScanner) (*FeedbackDiscovery, error) {
	var value FeedbackDiscovery
	var state, channels, threadState string
	var complete, truncated int
	var source, updated int64
	if err := row.Scan(&value.RepositoryID, &value.Generation, &state, &value.NextPage, &complete, &truncated,
		&value.DiscoveredPullRequests, &value.Requests, &channels, &threadState, &value.LastError, &source, &updated); err != nil {
		return nil, err
	}
	if state != "all" {
		return nil, fmt.Errorf("parse stored feedback discovery: unsupported pull-request state %q", state)
	}
	var channelValues []string
	if err := json.Unmarshal([]byte(channels), &channelValues); err != nil {
		return nil, fmt.Errorf("decode feedback discovery channels: %w", err)
	}
	selection, err := ParseFeedbackSelection(channelValues, threadState)
	if err != nil {
		return nil, fmt.Errorf("parse stored feedback discovery selection: %w", err)
	}
	value.Selection = selection
	switch {
	case complete == 1 && truncated == 0:
		value.State = FeedbackDiscoveryComplete
	case complete == 0 && truncated == 1:
		value.State = FeedbackDiscoveryTruncated
	case complete == 0 && truncated == 0:
		value.State = FeedbackDiscoveryPending
	default:
		return nil, fmt.Errorf("parse stored feedback discovery: invalid complete/truncated state %d/%d", complete, truncated)
	}
	value.SourceUpdatedAt, value.UpdatedAt = scanTime(source), scanTime(updated)
	return &value, nil
}
