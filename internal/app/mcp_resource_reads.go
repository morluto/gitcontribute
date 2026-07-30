package app

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/morluto/gitcontribute/internal/failure"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
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
	facets := []string{facetPRFeedbackIssueComments, facetPRFeedbackReviews, facetPRFeedbackInlineComments, facetPRFeedbackReviewThreads}
	channels := make(map[string]any, len(facets))
	out := map[string]any{
		"schema_version": "gitcontribute.pull-request-feedback.v1",
		"owner":          owner, "repo": repo, "number": number, "channels": channels,
	}
	for _, facet := range facets {
		value, err := r.pullRequestWorkflowFacet(ctx, owner, repo, number, facet)
		if err != nil {
			if isCorpusNotFound(err) {
				continue
			}
			return nil, err
		}
		channels[facet] = value
	}
	if len(channels) == 0 {
		return nil, failure.NotFound(errors.New("pull-request feedback is not stored"))
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
	return value, nil
}

func isCorpusNotFound(err error) bool {
	return failure.Is(err, failure.KindNotFound)
}
