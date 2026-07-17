package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/morluto/gitcontribute/internal/health"
)

type healthCmd struct {
	OwnerRepo  string        `arg:"" name:"owner/repo" help:"Repository as OWNER/REPO"`
	Start      string        `name:"start" help:"Window start as RFC3339"`
	End        string        `name:"end" help:"Window end as RFC3339"`
	StaleAfter time.Duration `name:"stale-after" default:"336h" help:"Open PR inactivity threshold"`
	JSON       bool          `name:"json" help:"Print the result as JSON"`
}

func (c *CLI) healthService() (HealthService, error) {
	service, ok := c.svc.(HealthService)
	if !ok {
		return nil, NewCLIError(ExitNotWired, ErrNotWired)
	}
	return service, nil
}

func (c *CLI) runHealth(ctx context.Context, cmd *healthCmd) error {
	repo, err := parseRepo(cmd.OwnerRepo)
	if err != nil {
		return err
	}
	if cmd.StaleAfter <= 0 {
		return NewCLIError(ExitUsage, errors.New("stale-after must be positive"))
	}
	parseBound := func(name, value string) (time.Time, error) {
		if value == "" {
			return time.Time{}, nil
		}
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return time.Time{}, NewCLIError(ExitUsage, fmt.Errorf("invalid --%s value: %w", name, err))
		}
		return parsed, nil
	}
	start, err := parseBound("start", cmd.Start)
	if err != nil {
		return err
	}
	end, err := parseBound("end", cmd.End)
	if err != nil {
		return err
	}
	if !start.IsZero() && !end.IsZero() && end.Before(start) {
		return NewCLIError(ExitUsage, errors.New("end cannot be before start"))
	}
	service, err := c.healthService()
	if err != nil {
		return err
	}
	result, err := service.RepositoryHealthWithOptions(ctx, repo, health.Options{Start: start, End: end, StaleThreshold: cmd.StaleAfter})
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}
