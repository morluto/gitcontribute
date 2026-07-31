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
		item.Recovery = recoveryPlan("blocked", "Poll jobs.get until this job reaches a terminal state.", mcpcontract.RecoveryAction(mcpcontract.GetJobsInput{IDs: []string{value.ID}}))
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
				Action: mcpcontract.FollowUpAction{Type: "poll_job", PollJob: &mcpcontract.GetJobsInput{IDs: []string{job.ID}}}, RetryAfterMS: mcpcontract.NonNegativeInt(retryAfter), Reason: "Poll this job until execution_state is terminal.",
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
		switch jobResultStatus(job) {
		case "failed":
			return "terminal", "failed"
		case "partial":
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
		switch jobResultStatus(job) {
		case "failed":
			return job.Kind + " failed with no usable item outcomes"
		case "partial":
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

func portfolioReadFollowUpArguments(request mcpcontract.SyncPortfolioInput, login string, references []string) *mcpcontract.ListPullRequestPortfolioInput {
	limit := request.Limit
	state := request.State
	if request.Selection == "authored" {
		if login != "" {
			return &mcpcontract.ListPullRequestPortfolioInput{Authors: []string{login}, State: state, Limit: limit, View: "compact"}
		}
	} else {
		state = "all"
		if limit == 0 || limit > len(references) {
			limit = len(references)
		}
	}
	if limit == 0 {
		limit = 20
	}
	return &mcpcontract.ListPullRequestPortfolioInput{State: state, Limit: limit, View: "compact"}
}

func facetBatchArtifact(refs []mcpcontract.ThreadRef, facetNames []string) ([]mcpcontract.JobArtifactReference, *mcpcontract.JobFollowUp) {
	value := mcpcontract.NonNegativeInt(len(refs))
	follow := &mcpcontract.JobFollowUp{
		Action: mcpcontract.FollowUpAction{Type: "get_thread_facets", GetThreadFacets: &mcpcontract.GetThreadFacetsInput{Threads: refs, Facets: append([]string(nil), facetNames...)}},
		Reason: "Read the synchronized facet coverage and canonical facet resources from the offline corpus.",
	}
	return []mcpcontract.JobArtifactReference{{Kind: "thread_facet_batch", Count: &value, References: threadRefKeys(refs)}}, follow
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
