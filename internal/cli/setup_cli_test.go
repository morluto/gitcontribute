package cli_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/cli"
)

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
	c, _, stderr := newTestCLI(svc, nil)
	c.SetInput(strings.NewReader("\nnone\nnone\n\n"))

	if err := c.Run(context.Background(), []string{"setup"}); err != nil {
		t.Fatal(err)
	}
	if !svc.lastSetup.InstallCLI || !svc.lastSetup.SkipMCP || len(svc.lastSetup.Clients) != 0 {
		t.Fatalf("options = %+v", svc.lastSetup)
	}
	for _, prompt := range []string{
		"Install the terminal app for CLI and TUI? [Y/n]:",
		"Set up which MCP clients? [codex,claude,none]:",
	} {
		if !strings.Contains(stderr.String(), prompt) {
			t.Fatalf("missing prompt %q in %q", prompt, stderr.String())
		}
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
	c.SetInput(strings.NewReader("\ncodex\nnone\nn\n"))

	if err := c.Run(context.Background(), []string{"setup"}); err != nil {
		t.Fatal(err)
	}
	if len(svc.setupCalls) != 1 || !svc.setupCalls[0].DryRun || !svc.setupCalls[0].InstallCLI {
		t.Fatalf("setup calls = %+v", svc.setupCalls)
	}
	if !strings.Contains(stdout.String(), "Setup plan") || !strings.Contains(stdout.String(), "npm install --global gitcontribute@0.1.1") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Setup cancelled; no changes were made.") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
