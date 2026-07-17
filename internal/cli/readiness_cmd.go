package cli

import (
	"context"
	"fmt"
)

type readinessCmd struct {
	Opportunity readinessOpportunityCmd `cmd:"" help:"Evaluate an opportunity readiness gate"`
	Explain     readinessExplainCmd     `cmd:"" help:"Explain one readiness check"`
}

type readinessOpportunityCmd struct {
	OpportunityID string `arg:"" help:"Opportunity ID"`
	JSON          bool   `name:"json" help:"Print the result as JSON"`
}

type readinessExplainCmd struct {
	CheckID string `arg:"" help:"Readiness check ID"`
	JSON    bool   `name:"json" help:"Print the result as JSON"`
}

func (c *CLI) readinessService() (ReadinessService, error) {
	service, ok := c.svc.(ReadinessService)
	if !ok {
		return nil, NewCLIError(ExitNotWired, ErrNotWired)
	}
	return service, nil
}

func (c *CLI) runReadiness(ctx context.Context, command string, cmd *readinessCmd) error {
	service, err := c.readinessService()
	if err != nil {
		return err
	}
	switch command {
	case "readiness opportunity":
		result, err := service.OpportunityReadiness(ctx, cmd.Opportunity.OpportunityID)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Opportunity.JSON, result)
	case "readiness explain":
		result, err := service.ExplainReadiness(ctx, cmd.Explain.CheckID)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Explain.JSON, result)
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown readiness command: %s", command))
	}
}
