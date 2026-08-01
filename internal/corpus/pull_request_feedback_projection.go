package corpus

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

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
	channel := feedbackChannelForFacet(facet)
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
