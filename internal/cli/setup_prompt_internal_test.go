package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

type oneByteReader struct{ reader *strings.Reader }

func (r *oneByteReader) Read(buffer []byte) (int, error) {
	return r.reader.Read(buffer[:1])
}

func TestAccessibleModeAppliesToSetupKeyAndConfirmationForms(t *testing.T) {
	t.Setenv("GITCONTRIBUTE_ACCESSIBLE", "1")
	var output bytes.Buffer
	prompter := &huhSetupPrompter{input: &oneByteReader{reader: strings.NewReader("2\nGH_CUSTOM\n")}, output: &output}
	selection, err := prompter.Select(context.Background(), SetupPromptRequest{
		Questions: []SetupPromptQuestion{SetupQuestionAuth},
		Discovery: SetupDiscovery{ConfiguredTokenSource: "none"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if selection.TokenSource != "env" || selection.TokenSourceKey != "GH_CUSTOM" {
		t.Fatalf("selection = %+v", selection)
	}
	for _, want := range []string{"GitHub authentication", "Environment variable name"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("accessible output %q does not contain %q", output.String(), want)
		}
	}

	output.Reset()
	prompter = &huhSetupPrompter{input: strings.NewReader("n\n"), output: &output}
	confirmed, err := prompter.Confirm(context.Background(), "Apply these changes?")
	if err != nil {
		t.Fatal(err)
	}
	if confirmed || !strings.Contains(output.String(), "Apply these changes?") {
		t.Fatalf("confirmed=%v output=%q", confirmed, output.String())
	}
}

func TestSetupClientLabelSurfacesDetectionRegistrationAndPath(t *testing.T) {
	label := setupClientLabel(SetupClientDiscovery{
		Name: "codex", Path: "/home/test/.codex/config.toml", Detected: true, Registered: true,
	})
	for _, want := range []string{"Codex", "detected", "configured", "/home/test/.codex/config.toml"} {
		if !strings.Contains(label, want) {
			t.Fatalf("label %q does not contain %q", label, want)
		}
	}
}

func TestDefaultSetupTokenSource(t *testing.T) {
	tests := []struct {
		name      string
		discovery SetupDiscovery
		want      string
	}{
		{name: "existing config wins", discovery: SetupDiscovery{ConfiguredTokenSource: "env", GitHubCLIAvailable: true}, want: "env"},
		{name: "available gh cli", discovery: SetupDiscovery{ConfiguredTokenSource: "none", GitHubCLIAvailable: true}, want: "gh-cli"},
		{name: "present environment", discovery: SetupDiscovery{ConfiguredTokenSource: "none", EnvironmentKeyPresent: true}, want: "env"},
		{name: "configure later", discovery: SetupDiscovery{ConfiguredTokenSource: "none"}, want: "none"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := defaultSetupTokenSource(test.discovery); got != test.want {
				t.Fatalf("default = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSetupAuthOptionsShowConfiguredEnvironmentKey(t *testing.T) {
	options := setupAuthOptions(SetupDiscovery{ConfiguredTokenSource: "env", ConfiguredTokenKey: "GH_TOKEN"})
	if len(options) < 2 || !strings.Contains(options[1].Key, "GH_TOKEN") {
		t.Fatalf("environment option = %+v", options)
	}
}

func TestSetupTokenSourceKeyDoesNotCrossAuthSemantics(t *testing.T) {
	tests := []struct {
		name     string
		request  SetupPromptRequest
		selected string
		want     string
	}{
		{
			name:     "keyring to environment",
			request:  SetupPromptRequest{Discovery: SetupDiscovery{ConfiguredTokenSource: "keyring", ConfiguredTokenKey: "account"}},
			selected: "env", want: "GITHUB_TOKEN",
		},
		{
			name:     "environment to keyring",
			request:  SetupPromptRequest{Discovery: SetupDiscovery{ConfiguredTokenSource: "env", ConfiguredTokenKey: "GH_TOKEN"}},
			selected: "keyring", want: "",
		},
		{
			name:     "same source preserves key",
			request:  SetupPromptRequest{Discovery: SetupDiscovery{ConfiguredTokenSource: "env", ConfiguredTokenKey: "GH_TOKEN"}},
			selected: "env", want: "GH_TOKEN",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := setupTokenSourceKey(test.request, test.selected); got != test.want {
				t.Fatalf("key = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSetupProgressModelKeepsCompletedLinesInOrder(t *testing.T) {
	model := newSetupProgressModel()
	for _, step := range []SetupStep{
		{Name: "configuration", Status: "configured"},
		{Name: "corpus", Status: "initialized"},
		{Name: "verification", Status: "verified"},
	} {
		updated, _ := model.Update(setupCompletedMsg(step))
		model = updated.(setupProgressModel)
	}
	view := model.View()
	wants := []string{"Configuration  configured", "Local corpus  initialized", "Verification  verified"}
	last := -1
	for _, want := range wants {
		index := strings.Index(view, want)
		if index <= last {
			t.Fatalf("progress view %q does not contain %q in order", view, want)
		}
		if strings.Count(view, want) != 1 {
			t.Fatalf("progress view %q contains %q more than once", view, want)
		}
		last = index
	}
}

func TestRenderSetupPlanIncludesEffectsAndSafetyBoundary(t *testing.T) {
	report := &SetupReport{DryRun: true, Steps: []SetupStep{
		{Name: "terminal", Status: "would install", Message: "npm install --global gitcontribute@0.2.2"},
		{Name: "codex", Status: "would configure", Path: "/home/test/.codex/config.toml"},
	}}
	got := renderSetupPlan(report)
	for _, want := range []string{"Setup plan", "Terminal app", "gitcontribute@0.2.2", "/home/test/.codex/config.toml", "will not contact GitHub"} {
		if !strings.Contains(got, want) {
			t.Fatalf("plan %q does not contain %q", got, want)
		}
	}
}

func TestRenderSetupResultTailorsNextCommand(t *testing.T) {
	report := &SetupReport{Steps: []SetupStep{{Name: "verification", Status: "verified"}}}
	if got := renderSetupResult(report, SetupOptions{InstallCLI: true, SkipMCP: true}, true); !strings.Contains(got, "gitcontribute tui") {
		t.Fatalf("installed result = %q", got)
	}
	if got := renderSetupResult(report, SetupOptions{SkipMCP: true}, false); !strings.Contains(got, "npx gitcontribute@latest tui") {
		t.Fatalf("npx result = %q", got)
	}
}
