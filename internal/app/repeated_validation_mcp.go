package app

import (
	"context"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// RunValidation submits one bounded validation execution group.
func (r *MCPReader) RunValidation(ctx context.Context, in mcpcontract.RunValidationInput) (mcpcontract.JobReference, error) {
	request, canonical, err := parseMCPRepeatValidationInput(in)
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	total := request.options.RunCount * len(request.options.Kinds)
	id, err := r.submitJob(ctx, "run_validation_group", canonical, func(ctx context.Context, report func(progress, statistics string) error) (any, error) {
		if err := report("validation", jobProgressCounts(0, total)); err != nil {
			return nil, err
		}
		result, err := r.runValidationGroup(ctx, request)
		if err != nil {
			return nil, err
		}
		if err := report("validation", jobProgressCounts(result.CompletedRuns, result.RequestedRuns)); err != nil {
			return nil, err
		}
		return result, nil
	})
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	return queuedJobReference(id, "run_validation_group", "repeat validation job started"), nil
}

func parseOptionalDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	return time.ParseDuration(value)
}
