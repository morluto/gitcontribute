package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/mcpserver"
)

// RunRepeatedValidation submits a durable repeat/stress validation group.
func (r *MCPReader) RunRepeatedValidation(ctx context.Context, in mcpserver.RunRepeatedValidationInput) (mcpserver.JobReference, error) {
	if !in.Execute {
		return mcpserver.JobReference{}, errors.New("execute must be true to authorize host command execution")
	}
	if in.RunCount == 0 {
		in.RunCount = 3
	}
	if in.Concurrency == 0 {
		in.Concurrency = 1
	}
	if in.SampleInterval == "" {
		in.SampleInterval = "100ms"
	}
	kinds := []string{in.Target}
	if in.Target == "both" {
		kinds = []string{"base", "candidate"}
	}
	perRunTimeout, err := parseOptionalDuration(in.PerRunTimeout)
	if err != nil {
		return mcpserver.JobReference{}, fmt.Errorf("per_run_timeout: %w", err)
	}
	overallTimeout, err := parseOptionalDuration(in.OverallTimeout)
	if err != nil {
		return mcpserver.JobReference{}, fmt.Errorf("overall_timeout: %w", err)
	}
	sampleInterval, err := parseOptionalDuration(in.SampleInterval)
	if err != nil {
		return mcpserver.JobReference{}, fmt.Errorf("sample_interval: %w", err)
	}
	opts := cli.RepeatValidationOptions{
		Kinds: kinds, RunCount: in.RunCount, Concurrency: in.Concurrency,
		PerRunTimeout: perRunTimeout, OverallTimeout: overallTimeout, SampleInterval: sampleInterval, Execute: true,
	}
	id, err := r.submitJob(ctx, "run_validation_group", in, func(ctx context.Context, report func(progress, statistics string) error) (any, error) {
		if err := report("validation", jobProgressCounts(0, in.RunCount*len(kinds))); err != nil {
			return nil, err
		}
		result, err := r.RunValidationGroup(ctx, in.ID, opts)
		if err != nil {
			return nil, err
		}
		if err := report("validation", jobProgressCounts(result.CompletedRuns, result.RequestedRuns)); err != nil {
			return nil, err
		}
		return result, nil
	})
	if err != nil {
		return mcpserver.JobReference{}, err
	}
	return queuedJobReference(id, "run_validation_group", "repeat validation job started"), nil
}

func parseOptionalDuration(value string) (time.Duration, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	return time.ParseDuration(value)
}
