package cli_test

import (
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

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
