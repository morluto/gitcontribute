package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/morluto/gitcontribute/internal/radar"
)

type radarCmd struct {
	OwnerRepo string `arg:"" optional:"" name:"owner/repo" help:"Repository as OWNER/REPO (one repository argument is required)"`
	Repo      string `name:"repo" help:"Repository as OWNER/REPO (alternative to the positional argument)"`
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
	repository := cmd.OwnerRepo
	if repository == "" {
		repository = cmd.Repo
	} else if cmd.Repo != "" {
		return NewCLIError(ExitUsage, errors.New("provide exactly one repository argument: OWNER/REPO or --repo"))
	}
	if repository == "" {
		return NewCLIError(ExitUsage, errors.New("repository is required"))
	}
	repo, err := parseRepo(repository)
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
