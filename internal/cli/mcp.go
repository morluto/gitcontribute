package cli

import (
	"context"
	"fmt"
	"strings"
)

func (c *CLI) runMCP(ctx context.Context, cmd *mcpCmd) error {
	if _, err := fmt.Fprintf(c.stderr, "starting mcp server (transport=%s)...\n", cmd.Serve.Transport); err != nil {
		return err
	}
	toolsets := strings.Split(cmd.Serve.Toolsets, ",")
	for i := range toolsets {
		toolsets[i] = strings.TrimSpace(toolsets[i])
	}
	return c.mapError(c.runner.Run(ctx, MCPOptions{Transport: cmd.Serve.Transport, Toolsets: toolsets, ReadOnly: cmd.Serve.ReadOnly}))
}
