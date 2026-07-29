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
	newServer := mcpserver.New
	if opts.ReadOnly {
		newServer = mcpserver.NewReadOnly
	}
	server, err := newServer(r.service.MCPReader(), r.version)
	if err != nil {
		return err
	}
	return server.ServeStdio(ctx)
}
