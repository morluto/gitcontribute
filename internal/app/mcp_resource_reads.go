package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/morluto/gitcontribute/internal/failure"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
	"github.com/morluto/gitcontribute/internal/workspace"
)

// Workspace returns a host-path-free workspace projection for offline resource
// reads.
func (r *MCPReader) Workspace(ctx context.Context, id string) (mcpcontract.WorkspaceResource, error) {
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.WorkspaceResource{}, err
	}
	ws, err := c.GetWorkspace(ctx, id)
	if err != nil {
		if errors.Is(err, workspace.ErrNotFound) {
			return mcpcontract.WorkspaceResource{}, failure.NotFound(err)
		}
		return mcpcontract.WorkspaceResource{}, err
	}
	return mcpcontract.WorkspaceResource{
		SchemaVersion: "gitcontribute.workspace.v1", ID: ws.Name, InvestigationID: ws.InvestigationID,
		Owner: ws.RepoOwner, Repo: ws.RepoName, BaseSHA: ws.BaseSHA, HeadSHA: ws.CandidateSHA,
		MergeBase: ws.MergeBase, Ownership: string(ws.Ownership), Dirty: ws.Dirty,
		HasUntracked: ws.HasUntracked, CreatedAt: formatTime(ws.CreatedAt),
	}, nil
}

func (r *MCPReader) PullRequestFeedbackResource(ctx context.Context, owner, repo string, number int) (map[string]any, error) {
	facets := []struct {
		channel string
		facet   string
	}{
		{"issue_comments", facetPRFeedbackIssueComments},
		{"submitted_reviews", facetPRFeedbackReviews},
		{"inline_comments", facetPRFeedbackInlineComments},
		{"review_threads", facetPRFeedbackReviewThreads},
	}
	channels := make(map[string]any, len(facets))
	out := map[string]any{
		"schema_version": "gitcontribute.pull-request-feedback.v1",
		"owner":          owner, "repo": repo, "number": number, "channels": channels,
	}
	for _, target := range facets {
		value, err := r.pullRequestWorkflowFacet(ctx, owner, repo, number, target.facet)
		if err != nil {
			if isCorpusNotFound(err) {
				continue
			}
			return nil, err
		}
		channels[target.channel] = value
	}
	if len(channels) == 0 {
		return nil, failure.NotFound(errors.New("pull-request feedback is not stored"))
	}
	return out, nil
}

// PullRequestFeedbackItemResource returns the exact normalized record named by
// a search match. The root feedback resource remains the canonical raw facet
// view; this child resource is the compact, identity-preserving follow-up.
func (r *MCPReader) PullRequestFeedbackItemResource(ctx context.Context, owner, repo string, number int, channel, feedbackID string) (map[string]any, error) {
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return nil, err
	}
	storedRepo, err := c.GetRepository(ctx, owner, repo)
	if err != nil || storedRepo == nil {
		if err == nil {
			err = failure.NotFound(errors.New("repository is not stored"))
		}
		return nil, err
	}
	item, err := c.GetPullRequestFeedbackItem(ctx, storedRepo.ID, number, channel, feedbackID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, failure.NotFound(fmt.Errorf("pull-request feedback item %s is not stored", feedbackID))
	}
	merged := any(nil)
	if item.PullRequestMergedKnown {
		merged = item.PullRequestMerged
	}
	resolved := any(nil)
	resolutionState := "unknown"
	if item.ResolvedKnown {
		resolved = item.Resolved
		if item.Resolved {
			resolutionState = "resolved"
		} else {
			resolutionState = "unresolved"
		}
	}
	facet := map[string]string{
		"issue_comments":    facetPRFeedbackIssueComments,
		"submitted_reviews": facetPRFeedbackReviews,
		"inline_comments":   facetPRFeedbackInlineComments,
		"review_threads":    facetPRFeedbackReviewThreads,
	}[channel]
	if facet == "" {
		return nil, failure.NotFound(fmt.Errorf("unknown pull-request feedback channel %q", channel))
	}
	coverage, err := c.GetCoverage(ctx, storedRepo.ID, &item.ThreadID, facet)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"schema_version": "gitcontribute.pull-request-feedback-item.v1",
		"owner":          owner, "repo": repo, "number": number, "channel": channel,
		"feedback_id": item.FeedbackID, "feedback_node_id": item.FeedbackNodeID,
		"thread_id": item.ThreadExternalID, "in_reply_to_id": item.InReplyToID,
		"feedback_author": item.Author, "review_state": item.ReviewState,
		"body": item.Body, "path": item.Path, "line": item.Line, "start_line": item.StartLine,
		"side": item.Side, "start_side": item.StartSide, "commit_oid": item.CommitOID,
		"outdated": item.Outdated, "resolved": resolved, "resolution_state": resolutionState, "resolved_by": item.ResolvedBy,
		"created_at": formatTime(item.CreatedAt), "updated_at": formatTime(item.UpdatedAt),
		"head_sha": item.HeadSHA, "source_observation_id": item.SourceObservationID,
		"pull_request": map[string]any{
			"owner": owner, "repo": repo, "number": item.PullRequestNumber,
			"author": item.PullRequestAuthor, "state": item.PullRequestState, "merged": merged,
		},
	}
	if coverage != nil {
		out["effective_coverage"] = map[string]any{"complete": coverage.Complete, "source_updated_at": formatTime(coverage.SourceUpdatedAt)}
	}
	return out, nil
}

func (r *MCPReader) CIFailureResource(ctx context.Context, owner, repo string, number int) (map[string]any, error) {
	value, err := r.pullRequestWorkflowFacet(ctx, owner, repo, number, facetPRCIReport)
	if err != nil {
		return nil, err
	}
	value["schema_version"] = "gitcontribute.ci-failure-report.v1"
	value["owner"], value["repo"], value["number"] = owner, repo, number
	return value, nil
}

func (r *MCPReader) CIJobLogResource(ctx context.Context, owner, repo string, number int, jobID int64) (map[string]any, error) {
	report, err := r.CIFailureResource(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	runs, _ := report["workflow_runs"].([]any)
	for _, rawRun := range runs {
		run, _ := rawRun.(map[string]any)
		jobs, _ := run["jobs"].([]any)
		for _, rawJob := range jobs {
			job, _ := rawJob.(map[string]any)
			id, _ := job["id"].(float64)
			if int64(id) != jobID {
				continue
			}
			log, ok := job["log"].(map[string]any)
			if !ok {
				return nil, failure.NotFound(errors.New("CI job log is not stored"))
			}
			log["schema_version"] = "gitcontribute.ci-job-log.v1"
			return log, nil
		}
	}
	return nil, failure.NotFound(errors.New("CI job log is not stored"))
}

func (r *MCPReader) pullRequestWorkflowFacet(ctx context.Context, owner, repo string, number int, facet string) (map[string]any, error) {
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return nil, err
	}
	storedRepo, err := c.GetRepository(ctx, owner, repo)
	if err != nil || storedRepo == nil {
		if err == nil {
			err = failure.NotFound(errors.New("repository is not stored"))
		}
		return nil, err
	}
	thread, err := c.GetThreadByNumber(ctx, storedRepo.ID, number)
	if err != nil || thread == nil {
		if err == nil {
			err = failure.NotFound(errors.New("pull request is not stored"))
		}
		return nil, err
	}
	observations, _, err := c.ListFacetObservationsBounded(ctx, storedRepo.ID, &thread.ID, facet, 1)
	if err != nil {
		return nil, err
	}
	if len(observations) == 0 {
		return nil, failure.NotFound(errors.New("facet is not stored"))
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(observations[0].Payload), &value); err != nil {
		return nil, err
	}
	coverage, err := c.GetCoverage(ctx, storedRepo.ID, &thread.ID, facet)
	if err != nil {
		return nil, err
	}
	if coverage != nil {
		value["effective_coverage"] = map[string]any{
			"complete":          coverage.Complete,
			"source_updated_at": formatTime(coverage.SourceUpdatedAt),
		}
	}
	return value, nil
}

func isCorpusNotFound(err error) bool {
	return failure.Is(err, failure.KindNotFound)
}
