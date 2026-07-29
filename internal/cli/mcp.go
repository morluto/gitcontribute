package cli

import (
	"context"
	"fmt"

	"github.com/morluto/gitcontribute/internal/contracts"
)

func (c *CLI) runMCP(ctx context.Context, cmd *mcpCmd) error {
	if _, err := fmt.Fprintf(c.stderr, "starting mcp server (transport=%s)...\n", cmd.Serve.Transport); err != nil {
		return err
	}
	return c.mapError(c.runner.Run(ctx, contracts.MCPOptions{
		Transport: cmd.Serve.Transport,
		ReadOnly:  cmd.Serve.ReadOnly,
	}))
}
