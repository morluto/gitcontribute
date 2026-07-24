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

// GetJob reads a durable job by ID.
func (r *MCPReader) GetJob(ctx context.Context, in mcpcontract.GetJobInput) (mcpcontract.GetJobOutput, error) {
	job, err := r.Service.GetJob(ctx, in.ID)
	if err != nil {
		return mcpcontract.GetJobOutput{}, err
	}
	return jobResultToMCP(job)
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
	value, err := jobResultToMCP(job)
	if err != nil {
		item.Status, item.Reason, item.Message = "failed", "decode_failed", err.Error()
		return item
	}
	item.Value = &value
	if value.Status == "running" {
		item.NextAction = "Poll jobs.get until this job reaches a terminal state."
	}
	return item
}

func jobResultToMCP(job *contracts.JobResult) (mcpcontract.GetJobOutput, error) {
	request, err := decodeJobJSON("request", job.Request)
	if err != nil {
		return mcpcontract.GetJobOutput{}, err
	}
	result, err := decodeJobJSON("result", job.Result)
	if err != nil {
		return mcpcontract.GetJobOutput{}, err
	}
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
	return mcpcontract.GetJobOutput{
		ID: job.ID, Kind: job.Kind, Status: job.Status, Request: request, Result: result, Error: job.Error,
		Phase: phase, CompletedItems: completed, TotalItems: total, ProgressPercent: percent, RetryAfterMS: retryAfter,
		CreatedAt: job.CreatedAt, StartedAt: job.StartedAt, CompletedAt: job.CompletedAt, CancelledAt: job.CancelledAt,
		CancellationRequested: job.Cancellation,
	}, nil
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
	// Older durable rows used percentages and key=value statistics. Keep them
	// readable while exposing only the structured MCP contract.
	if strings.HasSuffix(phase, "%") {
		phase = job.Kind
	}
	for _, field := range strings.Fields(job.Statistics) {
		var n int
		if _, err := fmt.Sscanf(field, "completed=%d", &n); err == nil {
			counts.CompletedItems = n
		}
		if _, err := fmt.Sscanf(field, "total=%d", &n); err == nil {
			counts.TotalItems = n
		}
	}
	return phase, counts.CompletedItems, counts.TotalItems
}

func decodeJobJSON(field, value string) (any, error) {
	var decoded any
	if strings.TrimSpace(value) != "" {
		if err := json.Unmarshal([]byte(value), &decoded); err != nil {
			return nil, fmt.Errorf("decode job %s: %w", field, err)
		}
	}
	return decoded, nil
}
