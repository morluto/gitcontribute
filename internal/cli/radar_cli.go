package cli

import (
	"context"
	"fmt"

	"github.com/morluto/gitcontribute/internal/radar"
)

type radarCmd struct {
	OwnerRepo string `arg:"" name:"owner/repo" help:"Repository as OWNER/REPO"`
	Limit     int    `name:"limit" default:"20" help:"Maximum number of ranked candidates"`
	JSON      bool   `name:"json" help:"Print the result as JSON"`
}

func (c *CLI) radarService() (RadarService, error) {
	service, ok := c.svc.(RadarService)
	if !ok {
		return nil, NewCLIError(ExitNotWired, ErrNotWired)
	}
	return service, nil
}

func (c *CLI) runRadar(ctx context.Context, cmd *radarCmd) error {
	repo, err := parseRepo(cmd.OwnerRepo)
	if err != nil {
		return err
	}
	if cmd.Limit < 1 || cmd.Limit > radar.MaxLimit {
		return NewCLIError(ExitUsage, fmt.Errorf("limit must be between 1 and %d", radar.MaxLimit))
	}
	service, err := c.radarService()
	if err != nil {
		return err
	}
	result, err := service.ContributionRadar(ctx, RadarOptions{Repo: repo, Limit: cmd.Limit})
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}
