package cli_test

import (
	"context"
	"testing"
)

func TestMCP(t *testing.T) {
	t.Parallel()
	runner := &fakeMCPRunner{}
	c, stdout, stderr := newTestCLI(nil, runner)

	requireNoErr(t, c.Run(context.Background(), []string{"mcp", "serve"}))
	if !runner.called {
		t.Fatal("MCP Run was not called")
	}
	if runner.opts.Transport != "stdio" {
		t.Fatalf("transport=%q, want stdio", runner.opts.Transport)
	}
	if stdout.String() != "" {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if stderr.String() != "starting mcp server (transport=stdio)...\n" {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestMCPReadOnly(t *testing.T) {
	t.Parallel()
	runner := &fakeMCPRunner{}
	c, _, _ := newTestCLI(nil, runner)

	requireNoErr(t, c.Run(context.Background(), []string{"mcp", "serve", "--read-only"}))
	if !runner.opts.ReadOnly {
		t.Fatal("read-only option was not forwarded")
	}
}
