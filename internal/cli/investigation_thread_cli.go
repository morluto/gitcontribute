package cli

import (
	"context"
	"fmt"

	"github.com/morluto/gitcontribute/internal/research"
)

type startThreadInvestigationCmd struct {
	Thread string `arg:"" name:"thread" help:"Thread as OWNER/REPO#NUMBER, issue:OWNER/REPO#NUMBER, or pr:OWNER/REPO#NUMBER"`
	JSON   bool   `name:"json" help:"Print the result as JSON"`
}

// ThreadInvestigationService is the optional local-write capability for
// starting an investigation and seed hypothesis from one stored thread.
type ThreadInvestigationService interface {
	StartInvestigationFromThread(ctx context.Context, ref research.ThreadRef) (*ThreadInvestigationResult, error)
}

func (c *CLI) runStartThreadInvestigation(ctx context.Context, cmd *startThreadInvestigationCmd) error {
	ref, err := research.ParseThreadRef(cmd.Thread)
	if err != nil {
		return NewCLIError(ExitUsage, err)
	}
	service, ok := c.svc.(ThreadInvestigationService)
	if !ok {
		return NewCLIError(ExitNotWired, ErrNotWired)
	}
	if _, err := fmt.Fprintf(c.stderr, "starting investigation from stored thread %s...\n", ref); err != nil {
		return c.mapError(err)
	}
	result, err := service.StartInvestigationFromThread(ctx, ref)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}
