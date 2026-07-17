package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
)

func (c *CLI) runSetupCommand(ctx context.Context, cmd *setupCmd) error {
	clients, err := c.prepareSetupCommand(cmd)
	if err != nil {
		return err
	}
	opts := SetupOptions{
		Clients: clients, AllClients: cmd.AllClients, InstallCLI: cmd.InstallCLI, SkipMCP: cmd.NoMCP,
		TokenSource: cmd.TokenSource, TokenSourceKey: cmd.TokenSourceKey, Repository: cmd.Repository,
		DryRun: cmd.DryRun, Version: cmd.MCPVersion,
	}
	if !cmd.Yes && !cmd.DryRun {
		apply, err := c.confirmSetupPlan(ctx, opts, cmd.JSON)
		if err != nil {
			return err
		}
		if !apply {
			return nil
		}
	}
	return c.executeSetup(ctx, opts, cmd.JSON)
}

func (c *CLI) prepareSetupCommand(cmd *setupCmd) ([]string, error) {
	clients := selectedSetupClients(cmd.Codex, cmd.Claude)
	if cmd.NoMCP && (len(clients) > 0 || cmd.AllClients) {
		return nil, NewCLIError(ExitUsage, errors.New("--no-mcp cannot be combined with client flags"))
	}
	needsPrompt := !cmd.Yes && ((!cmd.NoMCP && len(clients) == 0 && !cmd.AllClients) || cmd.TokenSource == "" || !cmd.DryRun)
	if needsPrompt && !c.interactiveInput() {
		return nil, NewCLIError(ExitUsage, errors.New("interactive setup requires a terminal; pass client flags and --yes"))
	}
	if err := c.offerTerminalInstall(cmd); err != nil {
		return nil, err
	}
	selected, err := c.selectSetupClients(cmd, clients)
	if err != nil {
		return nil, err
	}
	if cmd.NoMCP && !cmd.InstallCLI {
		return nil, NewCLIError(ExitUsage, errors.New("setup has no selected capability"))
	}
	if err := c.selectSetupTokenSource(cmd); err != nil {
		return nil, err
	}
	return selected, nil
}

func (c *CLI) offerTerminalInstall(cmd *setupCmd) error {
	if cmd.Yes || cmd.InstallCLI || !runningThroughNpx() || !c.interactiveInput() {
		return nil
	}
	// Only an ephemeral bootstrap needs this offer. A globally installed or
	// source-built executable is already a persistent terminal command.
	install, err := c.confirmSetup("Install the terminal app for CLI and TUI")
	if err != nil {
		return NewCLIError(ExitUsage, err)
	}
	cmd.InstallCLI = install
	return nil
}

func (c *CLI) selectSetupClients(cmd *setupCmd, clients []string) ([]string, error) {
	if cmd.NoMCP || len(clients) > 0 || cmd.AllClients || cmd.Yes {
		return clients, nil
	}
	selected, err := c.promptClients("Set up", true)
	if err != nil {
		return nil, NewCLIError(ExitUsage, err)
	}
	cmd.NoMCP = len(selected) == 0
	return selected, nil
}

func (c *CLI) selectSetupTokenSource(cmd *setupCmd) error {
	if cmd.TokenSource != "" || cmd.Yes {
		return nil
	}
	value, err := c.promptTokenSource()
	if err != nil {
		return NewCLIError(ExitUsage, err)
	}
	cmd.TokenSource = value
	if value == "env" && cmd.TokenSourceKey == "" {
		cmd.TokenSourceKey = "GITHUB_TOKEN"
	}
	return nil
}

func (c *CLI) confirmSetupPlan(ctx context.Context, opts SetupOptions, jsonOutput bool) (bool, error) {
	service, err := c.setupService()
	if err != nil {
		return false, err
	}
	planOptions := opts
	planOptions.DryRun = true
	plan, err := service.Setup(ctx, planOptions)
	if err != nil {
		return false, NewCLIError(ExitGeneral, err)
	}
	// JSON callers keep stdout machine-readable, so their human preview goes
	// to stderr.
	planOutput := c.stdout
	if jsonOutput {
		planOutput = c.stderr
	}
	if _, err := fmt.Fprintln(planOutput, setupHuman(plan)); err != nil {
		return false, NewCLIError(ExitGeneral, fmt.Errorf("write setup plan: %w", err))
	}
	if plan.HasFailures() {
		return false, NewCLIError(ExitGeneral, errors.New("setup plan contains one or more failed steps"))
	}
	apply, err := c.confirmSetup("Apply setup changes")
	if err != nil {
		return false, NewCLIError(ExitUsage, err)
	}
	if !apply {
		if _, err := fmt.Fprintln(c.stderr, "Setup cancelled; no changes were made."); err != nil {
			return false, NewCLIError(ExitGeneral, fmt.Errorf("write setup cancellation: %w", err))
		}
	}
	return apply, nil
}

// runningThroughNpx is used only to decide whether the interactive adapter
// should offer persistent installation. The application layer independently
// evaluates the same evidence when selecting and reporting MCP launchers.
func runningThroughNpx() bool {
	return os.Getenv("npm_execpath") != "" || os.Getenv("npm_lifecycle_event") == "npx" || os.Getenv("npm_command") == "exec"
}
