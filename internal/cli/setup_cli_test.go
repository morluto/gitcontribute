package cli_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/cli"
)

type fakeSetupPrompter struct {
	selection cli.SetupSelection
	confirmed bool
	request   cli.SetupPromptRequest
}

func (f *fakeService) SetupWithProgress(ctx context.Context, opts cli.SetupOptions, _ cli.SetupObserver) (*cli.SetupReport, error) {
	return f.Setup(ctx, opts)
}

func (f *fakeService) DiscoverSetup(context.Context) (*cli.SetupDiscovery, error) {
	return &cli.SetupDiscovery{
		Clients: []cli.SetupClientDiscovery{
			{Name: "codex", Path: "/home/test/.codex/config.toml", Detected: true},
			{Name: "claude", Path: "/home/test/.claude.json", Detected: true},
		},
		ConfiguredTokenSource: "none",
	}, nil
}

func (p *fakeSetupPrompter) Select(_ context.Context, request cli.SetupPromptRequest) (cli.SetupSelection, error) {
	p.request = request
	return p.selection, nil
}

func (p *fakeSetupPrompter) Confirm(context.Context, string) (bool, error) {
	return p.confirmed, nil
}

func TestRemoveAllNonInteractive(t *testing.T) {
	svc := &fakeService{setupResult: &cli.SetupReport{Operation: "remove", Steps: []cli.SetupStep{{Name: "codex", Status: "removed"}}}}
	c := cli.New(svc, &fakeMCPRunner{}, io.Discard, io.Discard)
	if err := c.Run(context.Background(), []string{"remove", "--all-clients", "--yes"}); err != nil {
		t.Fatal(err)
	}
	if !svc.lastSetup.Remove || !svc.lastSetup.AllClients {
		t.Fatalf("options = %+v", svc.lastSetup)
	}
}

func TestSetupWizardCanInstallTerminalWithoutMCP(t *testing.T) {
	t.Setenv("npm_command", "exec")
	svc := &fakeService{setupResult: &cli.SetupReport{Operation: "setup", Steps: []cli.SetupStep{{Name: "cli", Status: "installed"}}}}
	c, _, _ := newTestCLI(svc, nil)
	prompter := &fakeSetupPrompter{selection: cli.SetupSelection{Mode: cli.SetupModeCLI, TokenSource: "none"}, confirmed: true}
	c.SetSetupPrompter(prompter)

	if err := c.Run(context.Background(), []string{"setup"}); err != nil {
		t.Fatal(err)
	}
	if svc.lastSetup.Mode != cli.SetupModeCLI || len(svc.lastSetup.Clients) != 0 {
		t.Fatalf("options = %+v", svc.lastSetup)
	}
	if len(prompter.request.Questions) != 3 {
		t.Fatalf("prompt request = %+v", prompter.request)
	}
}

func TestSetupWizardPreviewsPlanBeforeConfirmation(t *testing.T) {
	t.Setenv("npm_command", "exec")
	svc := &fakeService{setupResult: &cli.SetupReport{
		Operation: "setup",
		DryRun:    true,
		Steps: []cli.SetupStep{{
			Name: "cli", Status: "would install",
			Message: "npm install --global gitcontribute@0.1.1",
		}},
	}}
	c, stdout, stderr := newTestCLI(svc, nil)
	prompter := &fakeSetupPrompter{
		selection: cli.SetupSelection{Mode: cli.SetupModeBoth, Clients: []string{"codex"}, TokenSource: "none"},
		confirmed: false,
	}
	c.SetSetupPrompter(prompter)

	if err := c.Run(context.Background(), []string{"setup"}); err != nil {
		t.Fatal(err)
	}
	if len(svc.setupCalls) != 1 || !svc.setupCalls[0].DryRun || svc.setupCalls[0].Mode != cli.SetupModeBoth {
		t.Fatalf("setup calls = %+v", svc.setupCalls)
	}
	if !containsAll(stdout.String(), "Setup plan", "npm install --global gitcontribute@0.1.1", "will not contact GitHub") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !containsAll(stderr.String(), "Setup cancelled; no changes were made.") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestSetupDryRunHumanOutputRemainsAPlan(t *testing.T) {
	svc := &fakeService{setupResult: &cli.SetupReport{
		Operation:  "setup",
		DryRun:     true,
		MCPCommand: &cli.SetupMCPCommand{Command: "/home/test/.local/share/gitcontribute/bin/0.1.1/gitcontribute", Args: []string{"mcp", "serve", "--transport=stdio"}},
		Steps:      []cli.SetupStep{{Name: "codex", Status: "would configure"}},
	}}
	var out bytes.Buffer
	c := cli.New(svc, &fakeMCPRunner{}, &out, io.Discard)
	if err := c.Run(context.Background(), []string{"setup", "--mode", "mcp", "--codex", "--token-source", "none", "--dry-run", "--yes"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Setup plan") {
		t.Fatalf("output does not identify a plan: %s", out.String())
	}
	if strings.Contains(out.String(), "GitContribute is ready") || strings.Contains(out.String(), "Next:") {
		t.Fatalf("dry-run output implies setup was applied: %s", out.String())
	}
}

func TestSetupYesDoesNotInferDetectedClients(t *testing.T) {
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)
	err := c.Run(context.Background(), []string{"setup", "--token-source", "none", "--yes"})
	if err == nil || !strings.Contains(err.Error(), "explicit setup mode") {
		t.Fatalf("error = %v", err)
	}
	if len(svc.setupCalls) != 0 {
		t.Fatalf("setup was called with inferred targets: %+v", svc.setupCalls)
	}
}

func TestSetupYesRequiresExplicitAuthenticationChoice(t *testing.T) {
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)
	err := c.Run(context.Background(), []string{"setup", "--mode", "mcp", "--codex", "--yes"})
	if err == nil || !strings.Contains(err.Error(), "--token-source") {
		t.Fatalf("error = %v", err)
	}
	if len(svc.setupCalls) != 0 {
		t.Fatalf("setup was called with inferred authentication: %+v", svc.setupCalls)
	}
}

func TestSetupYesAcceptsExplicitCLIMode(t *testing.T) {
	svc := &fakeService{setupResult: &cli.SetupReport{Operation: "setup", Steps: []cli.SetupStep{{Name: "cli", Status: "installed"}}}}
	c, _, _ := newTestCLI(svc, nil)
	if err := c.Run(context.Background(), []string{"setup", "--mode", "cli", "--token-source", "none", "--yes"}); err != nil {
		t.Fatal(err)
	}
	if svc.lastSetup.Mode != cli.SetupModeCLI || len(svc.lastSetup.Clients) != 0 {
		t.Fatalf("options = %+v", svc.lastSetup)
	}
}

func TestSetupYesMCPModeRequiresExplicitTarget(t *testing.T) {
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)
	err := c.Run(context.Background(), []string{"setup", "--mode", "mcp", "--token-source", "none", "--yes"})
	if err == nil || !strings.Contains(err.Error(), "requires --codex, --claude, or --all-clients") {
		t.Fatalf("error = %v", err)
	}
	if len(svc.setupCalls) != 0 {
		t.Fatalf("setup was called without an MCP target: %+v", svc.setupCalls)
	}
}

func TestSetupCLIModeRejectsMCPClientFlags(t *testing.T) {
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)
	err := c.Run(context.Background(), []string{"setup", "--mode", "cli", "--codex", "--token-source", "none", "--yes"})
	if err == nil || !strings.Contains(err.Error(), "--mode cli cannot be combined") {
		t.Fatalf("error = %v", err)
	}
	if len(svc.setupCalls) != 0 {
		t.Fatalf("setup was called with incompatible flags: %+v", svc.setupCalls)
	}
}

func TestSetupJSONDryRunRequiresExplicitMode(t *testing.T) {
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)
	err := c.Run(context.Background(), []string{"setup", "--codex", "--token-source", "none", "--dry-run", "--json"})
	if err == nil || !strings.Contains(err.Error(), "non-interactive setup requires --mode") {
		t.Fatalf("error = %v", err)
	}
	if len(svc.setupCalls) != 0 {
		t.Fatalf("setup was called with an inferred mode: %+v", svc.setupCalls)
	}
}

func TestSetupYesAcceptsExplicitMCPModes(t *testing.T) {
	for _, mode := range []cli.SetupMode{cli.SetupModeMCP, cli.SetupModeBoth} {
		t.Run(string(mode), func(t *testing.T) {
			svc := &fakeService{setupResult: &cli.SetupReport{Operation: "setup", Steps: []cli.SetupStep{{Name: "codex", Status: "configured"}}}}
			c, _, _ := newTestCLI(svc, nil)
			err := c.Run(context.Background(), []string{"setup", "--mode", string(mode), "--codex", "--token-source", "none", "--yes"})
			if err != nil {
				t.Fatal(err)
			}
			if svc.lastSetup.Mode != mode || !containsAll(strings.Join(svc.lastSetup.Clients, ","), "codex") {
				t.Fatalf("options = %+v", svc.lastSetup)
			}
		})
	}
}

func TestSetupModeFlagRejectsInvalidAndLegacyInputs(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "invalid mode", args: []string{"setup", "--mode", "server"}, want: `must be one of "mcp","cli","both"`},
		{name: "install cli", args: []string{"setup", "--install-cli"}, want: "unknown flag --install-cli"},
		{name: "no mcp", args: []string{"setup", "--no-mcp"}, want: "unknown flag --no-mcp"},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, _, _ := newTestCLI(&fakeService{}, nil)
			err := c.Run(context.Background(), test.args)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestSetupDoesNotStartWizardWhenPromptOutputIsRedirected(t *testing.T) {
	t.Setenv("npm_command", "exec")
	redirected, err := os.CreateTemp(t.TempDir(), "setup-stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer redirected.Close()

	var stdout bytes.Buffer
	c := cli.New(&fakeService{}, &fakeMCPRunner{}, &stdout, redirected)
	c.SetInput(strings.NewReader(""))
	err = c.Run(context.Background(), []string{"setup"})
	if err == nil || !strings.Contains(err.Error(), "terminal input and visible output") {
		t.Fatalf("error = %v", err)
	}
}

func TestSetupDoesNotAskForConsentWhenPlanOutputIsRedirected(t *testing.T) {
	t.Setenv("npm_command", "exec")
	redirected, err := os.CreateTemp(t.TempDir(), "setup-stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer redirected.Close()

	var stderr bytes.Buffer
	c := cli.New(&fakeService{}, &fakeMCPRunner{}, redirected, &stderr)
	c.SetInput(strings.NewReader(""))
	err = c.Run(context.Background(), []string{"setup"})
	if err == nil || !strings.Contains(err.Error(), "terminal input and visible output") {
		t.Fatalf("error = %v", err)
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
