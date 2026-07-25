package app

import (
	"context"
	"fmt"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/failure"
)

func jobProgressCounts(completed, total int) string {
	return fmt.Sprintf(`{"completed_items":%d,"total_items":%d}`, completed, total)
}

// ListJobs returns bounded durable jobs for CLI and MCP adapters.
func (s *Service) ListJobs(ctx context.Context, status string, limit int) (*contracts.JobListResult, error) {
	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return nil, err
	}
	items, err := c.ListJobs(ctx, status, limit)
	if err != nil {
		return nil, err
	}
	result := &contracts.JobListResult{Jobs: make([]contracts.JobResult, len(items))}
	for i := range items {
		result.Jobs[i] = jobResult(&items[i])
	}
	return result, nil
}

// GetJob returns one durable job by opaque ID.
func (s *Service) GetJob(ctx context.Context, id string) (*contracts.JobResult, error) {
	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return nil, err
	}
	job, err := c.GetJob(ctx, id)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, failure.NotFound(fmt.Errorf("job %s not found", id))
	}
	result := jobResult(job)
	return &result, nil
}

// submitJob persists a job record and runs fn asynchronously in this process.
func (s *Service) submitJob(ctx context.Context, kind string, request any, fn JobFunc) (string, error) {
	jobs, err := s.Jobs(ctx)
	if err != nil {
		return "", err
	}
	return jobs.Submit(ctx, kind, request, fn)
}

// CancelJob records and applies a cancellation request, then returns current state.
func (s *Service) CancelJob(ctx context.Context, id string) (*contracts.JobResult, error) {
	jobs, err := s.Jobs(ctx)
	if err != nil {
		return nil, err
	}
	if err := jobs.Cancel(ctx, id); err != nil {
		return nil, err
	}
	return s.GetJob(ctx, id)
}

func jobResult(job *corpus.Job) contracts.JobResult {
	result := contracts.JobResult{
		ID: job.ID, Kind: job.Kind, Status: job.Status, Request: job.Request,
		Result: job.Result, Error: job.Error, Progress: job.Progress,
		Statistics: job.Statistics, CreatedAt: formatTime(job.CreatedAt),
		Cancellation: job.CancelledAt != nil,
	}
	if job.StartedAt != nil {
		result.StartedAt = formatTime(*job.StartedAt)
	}
	if job.CompletedAt != nil {
		result.CompletedAt = formatTime(*job.CompletedAt)
	}
	if job.CancelledAt != nil {
		result.CancelledAt = formatTime(*job.CancelledAt)
	}
	return result
}
