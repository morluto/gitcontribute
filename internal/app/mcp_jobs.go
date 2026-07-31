package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/failure"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

const (
	jobKindSyncPullRequestPortfolio = "sync_pull_request_portfolio"
	jobKindSyncThreadFacets         = "sync_thread_facets"
)

// GetJob reads a durable job by ID.
func (r *MCPReader) GetJob(ctx context.Context, in mcpcontract.GetJobInput) (mcpcontract.GetJobOutput, error) {
	job, err := r.Service.GetJob(ctx, in.ID)
	if err != nil {
		return mcpcontract.GetJobOutput{}, err
	}
	return jobResultToMCP(job, true), nil
}

// CancelJobs requests bounded cancellation in input order. Missing and terminal
// jobs remain item-level outcomes so one bad ID does not hide successful writes.
func (r *MCPReader) CancelJobs(ctx context.Context, in mcpcontract.CancelJobInput) (mcpcontract.GetJobsOutput, error) {
	if len(in.IDs) < 1 || len(in.IDs) > 100 {
		return mcpcontract.GetJobsOutput{}, errors.New("ids must contain 1 to 100 items")
	}
	out := mcpcontract.GetJobsOutput{Status: "complete", Items: make([]mcpcontract.BatchItem[mcpcontract.GetJobOutput], len(in.IDs))}
	for i, inputID := range in.IDs {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		item := r.cancelJobItem(ctx, inputID)
		if item.Status != "complete" {
			out.Status = "partial"
		}
		out.Items[i] = item
	}
	return out, nil
}

func (r *MCPReader) cancelJobItem(ctx context.Context, inputID string) mcpcontract.BatchItem[mcpcontract.GetJobOutput] {
	id := strings.TrimSpace(inputID)
	item := mcpcontract.BatchItem[mcpcontract.GetJobOutput]{Key: id, Status: "complete"}
	if id == "" {
		item.Status, item.Reason, item.Message = "failed", "invalid_id", "job ID must not be empty"
		return item
	}
	current, err := r.Service.GetJob(ctx, id)
	if err != nil {
		if failure.Is(err, failure.KindNotFound) {
			item.Status, item.Reason = "unavailable", "not_found"
		} else {
			item.Status, item.Reason = "failed", "read_failed"
		}
		item.Message = err.Error()
		return item
	}
	if current.Status == "cancelled" {
		return jobResultItem(item, current)
	}
	if current.Status == "succeeded" || current.Status == "failed" {
		item.Status, item.Reason, item.Message = "unavailable", "terminal", "job is already "+current.Status
		return item
	}
	job, err := r.CancelJob(ctx, id)
	if err != nil {
		// A terminal transition can race the request; report its latest durable
		// state as unavailable rather than failing unrelated cancellations.
		latest, getErr := r.Service.GetJob(ctx, id)
		if getErr == nil && (latest.Status == "succeeded" || latest.Status == "failed") {
			item.Status, item.Reason, item.Message = "unavailable", "terminal", "job is already "+latest.Status
		} else {
			item.Status, item.Reason, item.Message = "failed", "cancellation_failed", err.Error()
		}
		return item
	}
	return jobResultItem(item, job)
}

func jobResultItem(item mcpcontract.BatchItem[mcpcontract.GetJobOutput], job *contracts.JobResult) mcpcontract.BatchItem[mcpcontract.GetJobOutput] {
	value := jobResultToMCP(job, true)
	item.Value = &value
	if value.Status == "running" {
		item.Recovery = recoveryPlan("blocked", "Poll jobs.get until this job reaches a terminal state.", mcpcontract.ToolCall{Tool: mcpcontract.ToolGetJob, Arguments: &mcpcontract.ToolCallArguments{IDs: []string{value.ID}}})
	}
	return item
}

func jobResultToMCP(job *contracts.JobResult, includeDetails bool) mcpcontract.GetJobOutput {
	phase, completed, total := decodeJobProgress(job)
	percent := 0
	if total > 0 {
		percent = completed * 100 / total
		if percent > 100 {
			percent = 100
		}
	}
	retryAfter := 0
	if job.Status == "queued" || job.Status == "running" {
		retryAfter = 1000
	}
	out := mcpcontract.GetJobOutput{
		ID: job.ID, Kind: job.Kind, Status: mcpcontract.JobStatus(job.Status), Error: job.Error,
		Phase: phase, CompletedItems: mcpcontract.NonNegativeInt(completed), TotalItems: mcpcontract.NonNegativeInt(total),
		ProgressPercent: mcpcontract.ProgressPercent(percent), RetryAfterMS: mcpcontract.NonNegativeInt(retryAfter),
		CreatedAt: job.CreatedAt, StartedAt: job.StartedAt, CompletedAt: job.CompletedAt, CancelledAt: job.CancelledAt,
		CancellationRequested: job.Cancellation,
	}
	out.ExecutionState, out.Outcome = jobExecution(job)
	out.Summary = jobSummary(job, completed, total)
	if includeDetails {
		switch job.Status {
		case "succeeded":
			out.Artifacts, out.FollowUp = jobArtifactsAndFollowUp(job, total)
		case "queued", "running":
			out.FollowUp = &mcpcontract.JobFollowUp{
				Tool: mcpcontract.ToolGetJob, Arguments: &mcpcontract.ToolCallArguments{IDs: []string{job.ID}}, Reason: "Poll this job until execution_state is terminal.",
			}
		}
	}
	return out
}

func jobExecution(job *contracts.JobResult) (mcpcontract.JobExecutionState, mcpcontract.JobOutcome) {
	switch job.Status {
	case "queued":
		return "queued", ""
	case "running":
		return "running", ""
	case "succeeded":
		if jobResultStatus(job) == "partial" {
			return "terminal", "partial"
		}
		return "terminal", "succeeded"
	case "failed":
		return "terminal", "failed"
	case "cancelled":
		return "terminal", "cancelled"
	default:
		return "running", ""
	}
}

func jobSummary(job *contracts.JobResult, completed, total int) string {
	switch job.Status {
	case "succeeded":
		if jobResultStatus(job) == "partial" {
			return job.Kind + " completed with partial item outcomes"
		}
		return job.Kind + " completed successfully"
	case "failed":
		return job.Kind + " failed"
	case "cancelled":
		return job.Kind + " was cancelled"
	case "running":
		if total > 0 {
			return fmt.Sprintf("%s is running (%d of %d items complete)", job.Kind, completed, total)
		}
		return job.Kind + " is running"
	default:
		return job.Kind + " is queued"
	}
}

func jobResultStatus(job *contracts.JobResult) string {
	var result struct {
		Status      string `json:"status"`
		BatchStatus string `json:"batch_status"`
	}
	if json.Unmarshal([]byte(job.Result), &result) != nil {
		return ""
	}
	if result.BatchStatus != "" {
		return result.BatchStatus
	}
	return result.Status
}

func jobArtifactsAndFollowUp(job *contracts.JobResult, total int) ([]mcpcontract.JobArtifactReference, *mcpcontract.JobFollowUp) {
	followUp := func(tool, uri, reason string) *mcpcontract.JobFollowUp {
		return &mcpcontract.JobFollowUp{Tool: tool, ResourceURI: uri, Reason: reason}
	}
	batch := func(kind, tool, reason string) ([]mcpcontract.JobArtifactReference, *mcpcontract.JobFollowUp) {
		count := total
		var result struct {
			Items []struct {
				Key string `json:"key"`
			} `json:"items"`
		}
		if json.Unmarshal([]byte(job.Result), &result) == nil && result.Items != nil {
			count = len(result.Items)
		}
		references := make([]string, 0, min(len(result.Items), 100))
		for _, item := range result.Items {
			if item.Key != "" && len(references) < 100 {
				references = append(references, item.Key)
			}
		}
		value := mcpcontract.NonNegativeInt(count)
		follow := followUp(tool, "", reason)
		follow.Arguments = jobFollowUpArguments(job)
		return []mcpcontract.JobArtifactReference{{
			Kind: kind, Count: &value, References: references, ReferencesTruncated: len(result.Items) > len(references),
		}}, follow
	}
	switch job.Kind {
	case "mine_repository_fix_patterns":
		uri := "gitcontribute://fix-pattern-report/" + job.ID
		return []mcpcontract.JobArtifactReference{{Kind: "fix_pattern_report", ID: job.ID, URI: uri}},
			followUp("", uri, "Read the persisted typed fix-pattern report.")
	case "build_repository_dossier":
		var request struct {
			Owner string `json:"owner"`
			Repo  string `json:"repo"`
		}
		if json.Unmarshal([]byte(job.Request), &request) == nil && request.Owner != "" && request.Repo != "" {
			uri := fmt.Sprintf("gitcontribute://dossier/%s/%s", request.Owner, request.Repo)
			return []mcpcontract.JobArtifactReference{{Kind: "dossier", ID: request.Owner + "/" + request.Repo, URI: uri}},
				followUp("", uri, "Read the persisted typed dossier resource.")
		}
	case "create_workspace":
		var result struct {
			ID string `json:"id"`
		}
		if json.Unmarshal([]byte(job.Result), &result) == nil && result.ID != "" {
			return []mcpcontract.JobArtifactReference{{Kind: "workspace", ID: result.ID}},
				followUp(mcpcontract.ToolInspectCommitChanges, "", "Inspect the managed workspace before planning commits.")
		}
	case "run_validation":
		var result struct {
			ID string `json:"id"`
		}
		if json.Unmarshal([]byte(job.Result), &result) == nil && result.ID != "" {
			return []mcpcontract.JobArtifactReference{{Kind: "validation_run", ID: result.ID}}, nil
		}
	case "run_validation_group":
		var result struct {
			ID string `json:"id"`
		}
		if json.Unmarshal([]byte(job.Result), &result) == nil && result.ID != "" {
			return []mcpcontract.JobArtifactReference{{Kind: "validation_group", ID: result.ID}}, nil
		}
	case "sync_repository_context":
		return batch("repository_batch", mcpcontract.ToolGetRepositories, "Read synchronized repository facts and coverage from the offline corpus.")
	case "sync_threads":
		return batch("thread_batch", mcpcontract.ToolGetThreads, "Read synchronized thread facts and coverage from the offline corpus.")
	case jobKindSyncThreadFacets:
		var request mcpcontract.HydrateThreadsInput
		_ = json.Unmarshal([]byte(job.Request), &request)
		refs := append([]mcpcontract.ThreadRef(nil), request.Threads...)
		return facetBatchArtifact(refs, request.Facets)
	case jobKindSyncPullRequestPortfolio:
		var result struct {
			PullRequests []string `json:"pull_requests"`
			Refreshed    int      `json:"refreshed"`
			Failures     []struct {
				Reference    string `json:"reference"`
				Status       string `json:"status"`
				Reason       string `json:"reason"`
				Message      string `json:"message"`
				RetryAfterMS int    `json:"retry_after_ms"`
			} `json:"failures"`
		}
		if json.Unmarshal([]byte(job.Result), &result) == nil {
			value := mcpcontract.NonNegativeInt(result.Refreshed)
			failures := make([]mcpcontract.JobArtifactFailure, len(result.Failures))
			for i, failure := range result.Failures {
				failures[i] = mcpcontract.JobArtifactFailure{
					Reference: failure.Reference, Status: mcpcontract.BatchItemStatus(failure.Status), Reason: failure.Reason,
					Message: failure.Message, RetryAfterMS: mcpcontract.NonNegativeInt(failure.RetryAfterMS),
				}
			}
			return []mcpcontract.JobArtifactReference{{
				Kind: "pull_request_batch", Count: &value, References: append([]string(nil), result.PullRequests...), Failures: failures,
			}}, followUp(mcpcontract.ToolListPullRequestPortfolio, "", "Read these refreshed pull requests from the offline portfolio.")
		}
	case "sync_pull_request_feedback", "sync_ci_failures":
		var result pullRequestWorkflowResult
		if json.Unmarshal([]byte(job.Result), &result) == nil {
			kind := "pull_request_feedback"
			tool := mcpcontract.ToolSyncPullRequestFeedback
			reason := "Read the persisted feedback snapshots through their resource links."
			if job.Kind == "sync_ci_failures" {
				kind, tool = "ci_failure_report", mcpcontract.ToolSyncCIFailures
				reason = "Read the persisted CI reports and bounded job logs through their resource links."
			}
			artifact := mcpcontract.JobArtifactReference{Kind: kind}
			if len(result.Items) == 0 {
				var request struct {
					PullRequests []mcpcontract.ThreadRef `json:"pull_requests"`
				}
				if json.Unmarshal([]byte(job.Request), &request) == nil {
					for _, ref := range request.PullRequests {
						resourceKind := "pull-request-feedback"
						if job.Kind == "sync_ci_failures" {
							resourceKind = "ci-failure-report"
						}
						artifact.References = append(artifact.References, fmt.Sprintf(
							"gitcontribute://%s/%s/%s/%d", resourceKind, ref.Owner, ref.Repo, ref.Number,
						))
					}
				}
			}
			for _, item := range result.Items {
				if item.Status == "complete" {
					artifact.References = append(artifact.References, item.ResourceURI)
					continue
				}
				artifact.Failures = append(artifact.Failures, mcpcontract.JobArtifactFailure{
					Reference: item.Key, Status: item.Status, Reason: item.Code, Message: item.Message,
					RetryAfterMS: mcpcontract.NonNegativeInt(item.RetryAfterMS),
				})
			}
			count := mcpcontract.NonNegativeInt(len(artifact.References))
			artifact.Count = &count
			return []mcpcontract.JobArtifactReference{artifact}, followUp(tool, "", reason)
		}
	case "index_repositories":
		return batch("code_index", mcpcontract.ToolSearchCode, "Search the locally indexed code corpus.")
	}
	return nil, nil
}

func jobFollowUpArguments(job *contracts.JobResult) *mcpcontract.ToolCallArguments {
	switch job.Kind {
	case "sync_repository_context":
		var request mcpcontract.SyncRepositoryContextInput
		if json.Unmarshal([]byte(job.Request), &request) == nil {
			return &mcpcontract.ToolCallArguments{Repositories: append([]mcpcontract.RepositoryRef(nil), request.Repositories...)}
		}
	case "sync_threads":
		var request mcpcontract.SyncThreadsInput
		if json.Unmarshal([]byte(job.Request), &request) == nil {
			return &mcpcontract.ToolCallArguments{Selection: request.Selection, Repositories: append([]mcpcontract.RepositoryRef(nil), request.Repositories...), Threads: append([]mcpcontract.ThreadRef(nil), request.Threads...), Kind: request.Kind, State: request.State}
		}
	case "index_repositories":
		var request mcpcontract.IndexRepositoriesInput
		if json.Unmarshal([]byte(job.Request), &request) == nil {
			return &mcpcontract.ToolCallArguments{Repositories: indexRepositoriesToRefs(request.Repositories)}
		}
	}
	return nil
}

func facetBatchArtifact(refs []mcpcontract.ThreadRef, facetNames []string) ([]mcpcontract.JobArtifactReference, *mcpcontract.JobFollowUp) {
	value := mcpcontract.NonNegativeInt(len(refs))
	follow := &mcpcontract.JobFollowUp{
		Tool:      mcpcontract.ToolGetThreadFacets,
		Arguments: &mcpcontract.ToolCallArguments{Threads: refs, Facets: append([]string(nil), facetNames...)},
		Reason:    "Read the synchronized facet coverage and canonical facet resources from the offline corpus.",
	}
	return []mcpcontract.JobArtifactReference{{Kind: "thread_facet_batch", Count: &value, References: threadRefKeys(refs)}}, follow
}

func indexRepositoriesToRefs(values []mcpcontract.IndexRepositoryInput) []mcpcontract.RepositoryRef {
	refs := make([]mcpcontract.RepositoryRef, 0, len(values))
	for _, value := range values {
		refs = append(refs, mcpcontract.RepositoryRef{Owner: value.Owner, Repo: value.Repo})
	}
	return refs
}

func threadRefKeys(refs []mcpcontract.ThreadRef) []string {
	keys := make([]string, 0, len(refs))
	for _, ref := range refs {
		keys = append(keys, threadRefKey(ref))
	}
	return keys
}

func decodeJobProgress(job *contracts.JobResult) (string, int, int) {
	phase := strings.TrimSpace(job.Progress)
	var counts struct {
		CompletedItems int `json:"completed_items"`
		TotalItems     int `json:"total_items"`
	}
	if json.Unmarshal([]byte(job.Statistics), &counts) == nil && counts.TotalItems >= 0 && counts.CompletedItems >= 0 {
		return phase, counts.CompletedItems, counts.TotalItems
	}
	return phase, counts.CompletedItems, counts.TotalItems
}
