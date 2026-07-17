package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
)

func (c *CLI) runSetupCommand(ctx context.Context, cmd *setupCmd) error {
	clients := selectedSetupClients(cmd.Codex, cmd.Claude)
	all := cmd.AllClients
	if cmd.NoMCP && (len(clients) > 0 || all) {
		return NewCLIError(ExitUsage, errors.New("--no-mcp cannot be combined with client flags"))
	}
	needsPrompt := !cmd.Yes && ((!cmd.NoMCP && len(clients) == 0 && !all) || cmd.TokenSource == "" || !cmd.DryRun)
	if needsPrompt && !c.interactiveInput() {
		return NewCLIError(ExitUsage, errors.New("interactive setup requires a terminal; pass client flags and --yes"))
	}
	if !cmd.Yes && !cmd.InstallCLI && runningThroughNpx() && c.interactiveInput() {
		// Only an ephemeral bootstrap needs this offer. A globally installed or
		// source-built executable is already a persistent terminal command.
		install, err := c.confirmSetup("Install the terminal app for CLI and TUI")
		if err != nil {
			return NewCLIError(ExitUsage, err)
		}
		cmd.InstallCLI = install
	}
	if !cmd.NoMCP && len(clients) == 0 && !all {
		if !cmd.Yes {
			selected, err := c.promptClients("Set up", true)
			if err != nil {
				return NewCLIError(ExitUsage, err)
			}
			clients = selected
			cmd.NoMCP = len(selected) == 0
		}
	}
	if cmd.NoMCP && !cmd.InstallCLI {
		return NewCLIError(ExitUsage, errors.New("setup has no selected capability"))
	}
	if cmd.TokenSource == "" && !cmd.Yes {
		value, err := c.promptTokenSource()
		if err != nil {
			return NewCLIError(ExitUsage, err)
		}
		cmd.TokenSource = value
		if value == "env" && cmd.TokenSourceKey == "" {
			cmd.TokenSourceKey = "GITHUB_TOKEN"
		}
	}
	opts := SetupOptions{
		Clients: clients, AllClients: all, InstallCLI: cmd.InstallCLI, SkipMCP: cmd.NoMCP,
		TokenSource: cmd.TokenSource, TokenSourceKey: cmd.TokenSourceKey, Repository: cmd.Repository,
		DryRun: cmd.DryRun, Version: cmd.MCPVersion,
	}
	if !cmd.Yes && !cmd.DryRun {
		// Show the real application plan before consent. JSON callers keep stdout
		// machine-readable, so their human preview is written to stderr.
		service, err := c.setupService()
		if err != nil {
			return err
		}
		planOptions := opts
		planOptions.DryRun = true
		plan, err := service.Setup(ctx, planOptions)
		if err != nil {
			return NewCLIError(ExitGeneral, err)
		}
		planOutput := c.stdout
		if cmd.JSON {
			planOutput = c.stderr
		}
		_, _ = fmt.Fprintln(planOutput, setupHuman(plan))
		if plan.HasFailures() {
			return NewCLIError(ExitGeneral, errors.New("setup plan contains one or more failed steps"))
		}
		ok, err := c.confirmSetup("Apply setup changes")
		if err != nil {
			return NewCLIError(ExitUsage, err)
		}
		if !ok {
			_, _ = fmt.Fprintln(c.stderr, "Setup cancelled; no changes were made.")
			return nil
		}
	}
	return c.executeSetup(ctx, opts, cmd.JSON)
}

// runningThroughNpx is used only to decide whether the interactive adapter
// should offer persistent installation. The application layer independently
// evaluates the same evidence when selecting and reporting MCP launchers.
func runningThroughNpx() bool {
	return os.Getenv("npm_execpath") != "" || os.Getenv("npm_lifecycle_event") == "npx" || os.Getenv("npm_command") == "exec"
}
