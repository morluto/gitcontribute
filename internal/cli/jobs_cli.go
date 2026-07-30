package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/morluto/gitcontribute/internal/contracts"
)

type jobsCmd struct {
	List   jobsListCmd   `cmd:"" help:"List durable jobs"`
	Get    jobsGetCmd    `cmd:"" help:"Get one or more durable jobs"`
	Cancel jobsCancelCmd `cmd:"" help:"Request bounded cancellation"`
}

type jobsListCmd struct {
	Status string `name:"status" help:"Filter by status (queued, running, succeeded, failed, or cancelled)"`
	Limit  int    `name:"limit" default:"50" help:"Maximum jobs to return"`
	JSON   bool   `name:"json" help:"Print the result as JSON"`
}

type jobsGetCmd struct {
	IDs  []string `arg:"" name:"id" help:"One to 100 opaque job IDs"`
	JSON bool     `name:"json" help:"Print the result as JSON"`
}

type jobsCancelCmd struct {
	IDs  []string `arg:"" name:"id" help:"One to 100 opaque job IDs"`
	JSON bool     `name:"json" help:"Print the result as JSON"`
}

func (c *CLI) jobService() (contracts.JobService, error) {
	service, ok := c.svc.(contracts.JobService)
	if !ok {
		return nil, NewCLIError(ExitNotWired, ErrNotWired)
	}
	return service, nil
}

func (c *CLI) runJobs(ctx context.Context, command string, cmd *jobsCmd) error {
	service, err := c.jobService()
	if err != nil {
		return err
	}
	switch command {
	case "jobs list":
		return c.runJobsList(ctx, service, &cmd.List)
	case "jobs get":
		return c.runJobsBatch(ctx, service, cmd.Get.IDs, false, cmd.Get.JSON)
	case "jobs cancel":
		return c.runJobsBatch(ctx, service, cmd.Cancel.IDs, true, cmd.Cancel.JSON)
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown jobs command: %s", command))
	}
}

func (c *CLI) runJobsList(ctx context.Context, service contracts.JobService, cmd *jobsListCmd) error {
	if cmd.Limit <= 0 || cmd.Limit > 1000 {
		return NewCLIError(ExitUsage, errors.New("limit must be between 1 and 1000"))
	}
	if cmd.Status != "" {
		switch cmd.Status {
		case "queued", "running", "succeeded", "failed", "cancelled":
		default:
			return NewCLIError(ExitUsage, fmt.Errorf("invalid job status %q", cmd.Status))
		}
	}
	result, err := service.ListJobs(ctx, cmd.Status, cmd.Limit)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}

func (c *CLI) runJobsBatch(ctx context.Context, service contracts.JobService, ids []string, cancel, jsonOutput bool) error {
	if len(ids) < 1 || len(ids) > 100 {
		return NewCLIError(ExitUsage, errors.New("ids must contain 1 to 100 items"))
	}
	results := make([]*contracts.JobResult, 0, len(ids))
	for _, id := range ids {
		var (
			result *contracts.JobResult
			err    error
		)
		if cancel {
			result, err = service.CancelJob(ctx, id)
		} else {
			result, err = service.GetJob(ctx, id)
		}
		if err != nil {
			return c.mapError(err)
		}
		results = append(results, result)
	}
	return c.render(jsonOutput, results)
}
