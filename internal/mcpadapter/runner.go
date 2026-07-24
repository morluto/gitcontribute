// Package mcpadapter wires the application boundary to the MCP transport.
package mcpadapter

import (
	"context"
	"fmt"

	"github.com/morluto/gitcontribute/internal/app"
	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/mcpserver"
)

// Runner starts an MCP server backed by the application service.
type Runner struct {
	service *app.Service
	version string
}

// New returns a transport runner for service.
func New(service *app.Service, version string) *Runner {
	return &Runner{service: service, version: version}
}

// Run starts the configured MCP transport.
func (r *Runner) Run(ctx context.Context, opts contracts.MCPOptions) error {
	if opts.Transport != "stdio" {
		return fmt.Errorf("unsupported mcp transport %q", opts.Transport)
	}
	server, err := mcpserver.NewWithOptions(
		r.service.MCPReader(),
		r.version,
		mcpserver.Options{Toolsets: opts.Toolsets, ReadOnly: opts.ReadOnly},
	)
	if err != nil {
		return err
	}
	return server.ServeStdio(ctx)
}
