package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/morluto/gitcontribute/internal/corpus"
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
		MergeBase: ws.MergeBase, Ownership: string(ws.Ownership), Dirty: ws.Dirty(),
		HasUntracked: ws.HasUntracked(), CreatedAt: formatTime(ws.CreatedAt),
	}, nil
}

func (r *MCPReader) PullRequestFeedbackResource(ctx context.Context, owner, repo string, number int) (mcpcontract.PullRequestFeedbackResource, error) {
	out := mcpcontract.PullRequestFeedbackResource{
		SchemaVersion: "gitcontribute.pull-request-feedback.v1", Owner: owner, Repo: repo, Number: number,
	}
	count := 0
	for _, channel := range []corpus.FeedbackChannel{corpus.FeedbackIssueComments, corpus.FeedbackSubmittedReviews, corpus.FeedbackInlineComments, corpus.FeedbackReviewThreads} {
		observation, err := r.readPullRequestWorkflowFacet(ctx, owner, repo, number, channel.Facet())
		if err != nil {
			if isCorpusNotFound(err) {
				continue
			}
			return mcpcontract.PullRequestFeedbackResource{}, err
		}
		value, err := mcpcontract.NewStoredFacetResource(observation.payload, observation.coverage)
		if err != nil {
			return mcpcontract.PullRequestFeedbackResource{}, fmt.Errorf("decode %s feedback facet: %w", channel, err)
		}
		switch channel {
		case corpus.FeedbackIssueComments:
			out.Channels.IssueComments = &value
		case corpus.FeedbackSubmittedReviews:
			out.Channels.SubmittedReviews = &value
		case corpus.FeedbackInlineComments:
			out.Channels.InlineComments = &value
		case corpus.FeedbackReviewThreads:
			out.Channels.ReviewThreads = &value
		}
		count++
	}
	if count == 0 {
		return mcpcontract.PullRequestFeedbackResource{}, failure.NotFound(errors.New("pull-request feedback is not stored"))
	}
	return out, nil
}

// PullRequestFeedbackItemResource returns the exact normalized record named by
// a search match. The root feedback resource remains the canonical raw facet
// view; this child resource is the compact, identity-preserving follow-up.
func (r *MCPReader) PullRequestFeedbackItemResource(ctx context.Context, owner, repo string, number int, channel, feedbackID string) (mcpcontract.PullRequestFeedbackItemResource, error) {
	parsedChannel, err := corpus.ParseFeedbackChannel(channel)
	if err != nil {
		return mcpcontract.PullRequestFeedbackItemResource{}, failure.NotFound(err)
	}
	channel = parsedChannel.String()
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.PullRequestFeedbackItemResource{}, err
	}
	storedRepo, err := c.GetRepository(ctx, owner, repo)
	if err != nil || storedRepo == nil {
		if err == nil {
			err = failure.NotFound(errors.New("repository is not stored"))
		}
		return mcpcontract.PullRequestFeedbackItemResource{}, err
	}
	item, err := c.GetPullRequestFeedbackItem(ctx, storedRepo.ID, number, parsedChannel, feedbackID)
	if err != nil {
		return mcpcontract.PullRequestFeedbackItemResource{}, err
	}
	if item == nil {
		return mcpcontract.PullRequestFeedbackItemResource{}, failure.NotFound(fmt.Errorf("pull-request feedback item %s is not stored", feedbackID))
	}
	var merged *bool
	if value, known := item.PullRequestMerge.IsMerged(), item.PullRequestMerge.Known(); known {
		merged = &value
	}
	var resolved *bool
	resolutionState := "unknown"
	if value, known := item.Resolution.Value(); known {
		resolved = &value
		if value {
			resolutionState = "resolved"
		} else {
			resolutionState = "unresolved"
		}
	}
	coverage, err := c.GetCoverage(ctx, storedRepo.ID, &item.ThreadID, parsedChannel.Facet())
	if err != nil {
		return mcpcontract.PullRequestFeedbackItemResource{}, err
	}
	out := mcpcontract.PullRequestFeedbackItemResource{
		SchemaVersion: "gitcontribute.pull-request-feedback-item.v1", Owner: owner, Repo: repo, Number: number, Channel: channel,
		FeedbackID: item.FeedbackID, FeedbackNodeID: item.FeedbackNodeID, ThreadID: item.ThreadExternalID, InReplyToID: item.InReplyToID,
		FeedbackAuthor: item.Author, ReviewState: item.ReviewState, Body: item.Body, Path: item.Path, Line: item.Line, StartLine: item.StartLine,
		Side: item.Side, StartSide: item.StartSide, CommitOID: item.CommitOID, Outdated: item.Outdated, Resolved: resolved,
		ResolutionState: resolutionState, ResolvedBy: item.ResolvedBy, CreatedAt: formatTime(item.CreatedAt), UpdatedAt: formatTime(item.UpdatedAt),
		HeadSHA: item.HeadSHA, SourceObservationID: item.SourceObservationID,
		PullRequest: mcpcontract.PullRequestFeedbackPullRequestResource{
			Owner: owner, Repo: repo, Number: item.PullRequestNumber, Author: item.PullRequestAuthor, State: item.PullRequestState, Merged: merged,
		},
	}
	if coverage != nil {
		out.EffectiveCoverage = &mcpcontract.ResourceCoverage{Complete: coverage.Complete, SourceUpdatedAt: formatTime(coverage.SourceUpdatedAt)}
	}
	return out, nil
}

func (r *MCPReader) CIFailureResource(ctx context.Context, owner, repo string, number int) (mcpcontract.CIFailureResource, error) {
	observation, err := r.readPullRequestWorkflowFacet(ctx, owner, repo, number, facetPRCIReport)
	if err != nil {
		return mcpcontract.CIFailureResource{}, err
	}
	value, err := mcpcontract.NewCIFailureResource(observation.payload, owner, repo, number, observation.coverage)
	if err != nil {
		return mcpcontract.CIFailureResource{}, fmt.Errorf("decode CI failure report: %w", err)
	}
	return value, nil
}

func (r *MCPReader) CIJobLogResource(ctx context.Context, owner, repo string, number int, jobID int64) (mcpcontract.CIJobLogResource, error) {
	observation, err := r.readPullRequestWorkflowFacet(ctx, owner, repo, number, facetPRCIReport)
	if err != nil {
		return mcpcontract.CIJobLogResource{}, err
	}
	var report struct {
		WorkflowRuns []struct {
			Jobs []struct {
				ID  int64 `json:"id"`
				Log *struct {
					JobID     int64  `json:"job_id"`
					Body      string `json:"body"`
					Truncated bool   `json:"truncated"`
				} `json:"log"`
			} `json:"jobs"`
		} `json:"workflow_runs"`
	}
	if err := json.Unmarshal(observation.payload, &report); err != nil {
		return mcpcontract.CIJobLogResource{}, fmt.Errorf("decode CI job log index: %w", err)
	}
	for _, run := range report.WorkflowRuns {
		for _, job := range run.Jobs {
			if job.ID != jobID {
				continue
			}
			if job.Log == nil {
				return mcpcontract.CIJobLogResource{}, failure.NotFound(errors.New("CI job log is not stored"))
			}
			if job.Log.JobID != job.ID {
				return mcpcontract.CIJobLogResource{}, errors.New("stored CI job log identity does not match its job")
			}
			return mcpcontract.NewCIJobLogResource(job.Log.JobID, job.Log.Body, job.Log.Truncated)
		}
	}
	return mcpcontract.CIJobLogResource{}, failure.NotFound(errors.New("CI job log is not stored"))
}

type storedWorkflowFacetObservation struct {
	payload  json.RawMessage
	coverage *mcpcontract.ResourceCoverage
}

func (r *MCPReader) readPullRequestWorkflowFacet(ctx context.Context, owner, repo string, number int, facet string) (storedWorkflowFacetObservation, error) {
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return storedWorkflowFacetObservation{}, err
	}
	storedRepo, err := c.GetRepository(ctx, owner, repo)
	if err != nil || storedRepo == nil {
		if err == nil {
			err = failure.NotFound(errors.New("repository is not stored"))
		}
		return storedWorkflowFacetObservation{}, err
	}
	thread, err := c.GetThreadByNumber(ctx, storedRepo.ID, number)
	if err != nil || thread == nil {
		if err == nil {
			err = failure.NotFound(errors.New("pull request is not stored"))
		}
		return storedWorkflowFacetObservation{}, err
	}
	observations, _, err := c.ListFacetObservationsBounded(ctx, storedRepo.ID, &thread.ID, facet, 1)
	if err != nil {
		return storedWorkflowFacetObservation{}, err
	}
	if len(observations) == 0 {
		return storedWorkflowFacetObservation{}, failure.NotFound(errors.New("facet is not stored"))
	}
	payload := json.RawMessage(observations[0].Payload)
	if !json.Valid(payload) {
		return storedWorkflowFacetObservation{}, errors.New("stored workflow facet payload is invalid JSON")
	}
	coverage, err := c.GetCoverage(ctx, storedRepo.ID, &thread.ID, facet)
	if err != nil {
		return storedWorkflowFacetObservation{}, err
	}
	out := storedWorkflowFacetObservation{payload: payload}
	if coverage != nil {
		out.coverage = &mcpcontract.ResourceCoverage{Complete: coverage.Complete, SourceUpdatedAt: formatTime(coverage.SourceUpdatedAt)}
	}
	return out, nil
}

func isCorpusNotFound(err error) bool {
	return failure.Is(err, failure.KindNotFound)
}
