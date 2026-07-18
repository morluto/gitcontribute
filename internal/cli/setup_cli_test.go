package cli_test

import (
	"bytes"
	"context"
	"io"
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
	svc := &fakeService{setupResult: &cli.SetupReport{Operation: "setup", Steps: []cli.SetupStep{{Name: "terminal", Status: "installed"}}}}
	c, _, _ := newTestCLI(svc, nil)
	prompter := &fakeSetupPrompter{selection: cli.SetupSelection{InstallCLI: true, TokenSource: "none"}, confirmed: true}
	c.SetSetupPrompter(prompter)

	if err := c.Run(context.Background(), []string{"setup"}); err != nil {
		t.Fatal(err)
	}
	if !svc.lastSetup.InstallCLI || !svc.lastSetup.SkipMCP || len(svc.lastSetup.Clients) != 0 {
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
			Name: "terminal", Status: "would install",
			Message: "npm install --global gitcontribute@0.1.1",
		}},
	}}
	c, stdout, stderr := newTestCLI(svc, nil)
	prompter := &fakeSetupPrompter{
		selection: cli.SetupSelection{InstallCLI: true, Clients: []string{"codex"}, TokenSource: "none"},
		confirmed: false,
	}
	c.SetSetupPrompter(prompter)

	if err := c.Run(context.Background(), []string{"setup"}); err != nil {
		t.Fatal(err)
	}
	if len(svc.setupCalls) != 1 || !svc.setupCalls[0].DryRun || !svc.setupCalls[0].InstallCLI {
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
		Operation: "setup",
		DryRun:    true,
		Launcher:  "npx gitcontribute@latest mcp",
		Steps:     []cli.SetupStep{{Name: "codex", Status: "would configure"}},
	}}
	var out bytes.Buffer
	c := cli.New(svc, &fakeMCPRunner{}, &out, io.Discard)
	if err := c.Run(context.Background(), []string{"setup", "--codex", "--token-source", "none", "--dry-run", "--yes"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Setup plan") {
		t.Fatalf("output does not identify a plan: %s", out.String())
	}
	if strings.Contains(out.String(), "GitContribute is ready") || strings.Contains(out.String(), "Next:") {
		t.Fatalf("dry-run output implies setup was applied: %s", out.String())
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
