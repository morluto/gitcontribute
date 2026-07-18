package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

func (c *CLI) runSetupCommand(ctx context.Context, cmd *setupCmd) error {
	clients, err := c.prepareSetupCommand(ctx, cmd)
	if err != nil {
		if errors.Is(err, ErrSetupCancelled) {
			return c.writeSetupCancellation()
		}
		return err
	}
	opts := SetupOptions{
		Mode: setupCommandMode(cmd, clients), Clients: clients, AllClients: cmd.AllClients,
		TokenSource: cmd.TokenSource, TokenSourceKey: cmd.TokenSourceKey, Repository: cmd.Repository,
		DryRun: cmd.DryRun,
	}
	if !cmd.Yes && !cmd.DryRun {
		apply, err := c.confirmSetupPlan(ctx, opts, cmd.JSON)
		if err != nil {
			if errors.Is(err, ErrSetupCancelled) {
				return c.writeSetupCancellation()
			}
			return err
		}
		if !apply {
			return nil
		}
	}
	return c.executeSetup(ctx, opts, cmd.JSON)
}

func (c *CLI) prepareSetupCommand(ctx context.Context, cmd *setupCmd) ([]string, error) {
	clients := selectedSetupClients(cmd.Codex, cmd.Claude)
	if err := validateSetupSelection(cmd, clients); err != nil {
		return nil, err
	}
	interactive := c.setupPrompter != nil || (c.interactiveInput() && c.interactiveOutput() && c.interactivePromptOutput())
	if cmd.Mode == nil && (cmd.JSON || (!interactive && (len(clients) > 0 || cmd.AllClients))) {
		return nil, NewCLIError(ExitUsage, errors.New("non-interactive setup requires --mode mcp, --mode cli, or --mode both"))
	}
	questions := setupQuestions(cmd, clients)
	needsPrompt := !cmd.Yes && (len(questions) > 0 || !cmd.DryRun)
	if needsPrompt && !interactive {
		return nil, NewCLIError(ExitUsage, errors.New("interactive setup requires terminal input and visible output; pass explicit setup flags and --yes"))
	}
	if len(questions) > 0 {
		selection, err := c.promptSetupSelection(ctx, cmd, clients, questions)
		if err != nil {
			return nil, err
		}
		cmd.Mode = &selection.Mode
		clients = selection.Clients
		cmd.TokenSource = selection.TokenSource
		cmd.TokenSourceKey = selection.TokenSourceKey
	}
	if setupCommandMode(cmd, clients) == "" {
		return nil, NewCLIError(ExitUsage, errors.New("setup has no selected access mode"))
	}
	return clients, nil
}

func setupCommandMode(cmd *setupCmd, clients []string) SetupMode {
	if cmd.Mode != nil {
		return *cmd.Mode
	}
	if len(clients) > 0 || cmd.AllClients {
		return SetupModeMCP
	}
	return ""
}

func validateSetupSelection(cmd *setupCmd, clients []string) error {
	if cmd.Mode != nil && *cmd.Mode == SetupModeCLI && (len(clients) > 0 || cmd.AllClients) {
		return NewCLIError(ExitUsage, errors.New("--mode cli cannot be combined with MCP client flags"))
	}
	if cmd.Yes {
		if cmd.Mode == nil {
			return NewCLIError(ExitUsage, errors.New("--yes requires an explicit setup mode; pass --mode mcp, --mode cli, or --mode both"))
		}
		if cmd.Mode.ConfiguresMCP() && len(clients) == 0 && !cmd.AllClients {
			return NewCLIError(ExitUsage, fmt.Errorf("--mode %s with --yes requires --codex, --claude, or --all-clients", *cmd.Mode))
		}
		if strings.TrimSpace(cmd.TokenSource) == "" {
			return NewCLIError(ExitUsage, errors.New("--yes requires --token-source so authentication configuration is explicit"))
		}
	}
	return nil
}

func setupQuestions(cmd *setupCmd, clients []string) []SetupPromptQuestion {
	questions := make([]SetupPromptQuestion, 0, 3)
	mode := setupCommandMode(cmd, clients)
	if !cmd.Yes && mode == "" {
		questions = append(questions, SetupQuestionAccess)
	}
	if !cmd.Yes && mode != SetupModeCLI && len(clients) == 0 && !cmd.AllClients {
		questions = append(questions, SetupQuestionClients)
	}
	if !cmd.Yes && cmd.TokenSource == "" {
		questions = append(questions, SetupQuestionAuth)
	}
	return questions
}

func containsSetupQuestion(questions []SetupPromptQuestion, want SetupPromptQuestion) bool {
	for _, question := range questions {
		if question == want {
			return true
		}
	}
	return false
}

func (c *CLI) promptSetupSelection(ctx context.Context, cmd *setupCmd, clients []string, questions []SetupPromptQuestion) (SetupSelection, error) {
	service, err := c.setupService()
	if err != nil {
		return SetupSelection{}, err
	}
	discovery, err := service.DiscoverSetup(ctx)
	if err != nil {
		return SetupSelection{}, NewCLIError(ExitGeneral, fmt.Errorf("inspect setup state: %w", err))
	}
	prompter := c.setupPrompter
	if prompter == nil {
		prompter = newSetupPrompter(c.stdin, c.stderr)
	}
	return prompter.Select(ctx, SetupPromptRequest{
		Discovery: *discovery, PackageRunner: runningThroughNpx(), Questions: questions,
		Mode: setupCommandMode(cmd, clients), Clients: clients, TokenSource: cmd.TokenSource, TokenSourceKey: cmd.TokenSourceKey,
	})
}

func (c *CLI) writeSetupCancellation() error {
	if _, err := fmt.Fprintln(c.stderr, "Setup cancelled; no changes were made."); err != nil {
		return NewCLIError(ExitGeneral, fmt.Errorf("write setup cancellation: %w", err))
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
	if _, err := fmt.Fprintln(planOutput, renderSetupPlan(plan)); err != nil {
		return false, NewCLIError(ExitGeneral, fmt.Errorf("write setup plan: %w", err))
	}
	if plan.HasFailures() {
		return false, NewCLIError(ExitGeneral, errors.New("setup plan contains one or more failed steps"))
	}
	prompter := c.setupPrompter
	if prompter == nil {
		prompter = newSetupPrompter(c.stdin, c.stderr)
	}
	apply, err := prompter.Confirm(ctx, "Apply these changes?")
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

func (c *CLI) executeSetup(ctx context.Context, opts SetupOptions, jsonOutput bool) error {
	service, err := c.setupService()
	if err != nil {
		return err
	}
	var report *SetupReport
	if !jsonOutput && setupProgressEnabled(opts, c.stderr) {
		progress := startSetupProgress(c.stderr)
		report, err = service.SetupWithProgress(ctx, opts, progress)
		progressErr := progress.Close()
		if err == nil && progressErr != nil {
			err = progressErr
		}
	} else {
		report, err = service.Setup(ctx, opts)
	}
	if err != nil {
		return NewCLIError(ExitGeneral, err)
	}
	if jsonOutput {
		enc := json.NewEncoder(c.stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return NewCLIError(ExitGeneral, err)
		}
	} else {
		var output string
		if opts.Remove {
			output = setupHuman(report)
		} else if report.DryRun {
			output = renderSetupPlan(report)
		} else {
			output = renderSetupResult(report, opts)
		}
		if _, err := fmt.Fprintln(c.stdout, output); err != nil {
			return NewCLIError(ExitGeneral, fmt.Errorf("write setup result: %w", err))
		}
	}
	if report.HasFailures() {
		return NewCLIError(ExitGeneral, errors.New("one or more setup steps failed"))
	}
	return nil
}

func setupProgressEnabled(opts SetupOptions, output io.Writer) bool {
	return setupProgressAnimationAllowed(opts) && interactiveWriter(output)
}

func setupProgressAnimationAllowed(opts SetupOptions) bool {
	if opts.DryRun || os.Getenv("GITCONTRIBUTE_ACCESSIBLE") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	return true
}

func setupHuman(report *SetupReport) string {
	var b strings.Builder
	operation := report.Operation
	if operation == "" {
		operation = "setup"
	}
	fmt.Fprintf(&b, "%s%s", strings.ToUpper(operation[:1]), operation[1:])
	if report.DryRun {
		b.WriteString(" plan")
	}
	for _, step := range report.Steps {
		fmt.Fprintf(&b, "\n- %s [%s]", step.Name, step.Status)
		if step.Path != "" {
			fmt.Fprintf(&b, ": %s", step.Path)
		}
		if step.Message != "" {
			fmt.Fprintf(&b, " — %s", step.Message)
		}
	}
	if report.MCPCommand != nil {
		fmt.Fprintf(&b, "\nMCP executable: %s", report.MCPCommand.Command)
		if len(report.MCPCommand.Args) > 0 {
			fmt.Fprintf(&b, "\nMCP arguments: %s", strings.Join(report.MCPCommand.Args, " "))
		}
	}
	return b.String()
}

// runningThroughNpx lets the interactive heading disclose the bootstrap
// mechanism. Package-runner state is never stored in coding-agent MCP
// configuration.
func runningThroughNpx() bool {
	return os.Getenv("npm_execpath") != "" || os.Getenv("npm_lifecycle_event") == "npx" || os.Getenv("npm_command") == "exec"
}
