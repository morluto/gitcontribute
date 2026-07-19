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
	for _, want := range []string{"How should future GitHub syncs authenticate?", "Environment variable name"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("accessible output %q does not contain %q", output.String(), want)
		}
	}
	if strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("accessible output contains ANSI escapes: %q", output.String())
	}

	output.Reset()
	prompter = &huhSetupPrompter{input: strings.NewReader("1\n"), output: &output}
	confirmed, err := prompter.Confirm(context.Background(), "Apply these changes?")
	if err != nil {
		t.Fatal(err)
	}
	if confirmed || !strings.Contains(output.String(), "Apply these changes?") {
		t.Fatalf("confirmed=%v output=%q", confirmed, output.String())
	}
}

func TestSetupFinalConsentDefaultsToApply(t *testing.T) {
	t.Setenv("GITCONTRIBUTE_ACCESSIBLE", "1")
	var output bytes.Buffer
	prompter := &huhSetupPrompter{input: strings.NewReader("\n"), output: &output}
	confirmed, err := prompter.Confirm(context.Background(), "Apply these changes?")
	if err != nil {
		t.Fatal(err)
	}
	if !confirmed {
		t.Fatalf("default confirmation cancelled setup: %q", output.String())
	}
}

func TestSetupFieldsRenderAsSeparatePages(t *testing.T) {
	selection := SetupSelection{}
	request := SetupPromptRequest{
		Questions: []SetupPromptQuestion{SetupQuestionAccess, SetupQuestionClients, SetupQuestionAuth},
		Discovery: SetupDiscovery{Clients: []SetupClientDiscovery{
			{Name: "codex", Path: "/home/test/.codex/config.toml", Detected: true},
		}},
	}
	fields := setupPromptFields(request, &selection)
	groups := setupPromptGroups(fields)
	if len(fields) != 3 || len(groups) != 3 {
		t.Fatalf("fields=%d groups=%d, want one group for each of 3 stages", len(fields), len(groups))
	}
	for index, group := range groups {
		if group == nil {
			t.Fatalf("group %d is nil", index)
		}
	}
}

func TestSetupQuestionsSkipAccessModeWhenAnMCPClientWasExplicitlySelected(t *testing.T) {
	t.Setenv("npm_command", "exec")
	questions := setupQuestions(&setupCmd{}, []string{"codex"})
	if containsSetupQuestion(questions, SetupQuestionAccess) {
		t.Fatalf("questions = %v", questions)
	}
}

func TestSetupQuestionsRespectExplicitAccessMode(t *testing.T) {
	for _, test := range []struct {
		mode        SetupMode
		wantClients bool
	}{
		{mode: SetupModeMCP, wantClients: true},
		{mode: SetupModeCLI, wantClients: false},
		{mode: SetupModeBoth, wantClients: true},
	} {
		t.Run(string(test.mode), func(t *testing.T) {
			mode := test.mode
			questions := setupQuestions(&setupCmd{Mode: &mode}, nil)
			if containsSetupQuestion(questions, SetupQuestionAccess) {
				t.Fatalf("explicit mode prompted for access: %v", questions)
			}
			if got := containsSetupQuestion(questions, SetupQuestionClients); got != test.wantClients {
				t.Fatalf("client question = %v, want %v; questions = %v", got, test.wantClients, questions)
			}
			if !containsSetupQuestion(questions, SetupQuestionAuth) {
				t.Fatalf("questions = %v", questions)
			}
		})
	}
}

func TestSetupOffersAccessModesInsteadOfPackageRunnerMechanics(t *testing.T) {
	var output bytes.Buffer
	t.Setenv("GITCONTRIBUTE_ACCESSIBLE", "1")
	prompter := &huhSetupPrompter{input: strings.NewReader("2\n"), output: &output}
	selection, err := prompter.Select(context.Background(), SetupPromptRequest{
		Questions: []SetupPromptQuestion{SetupQuestionAccess},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"How do you want to use GitContribute?", "MCP", "CLI", "Both"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output %q does not contain %q", output.String(), want)
		}
	}
	if strings.Contains(output.String(), "Keep using npx") {
		t.Fatalf("output exposes package-runner mechanics: %q", output.String())
	}
	if strings.Contains(output.String(), "Coding agents · MCP") || strings.Contains(output.String(), "Terminal app") {
		t.Fatalf("output does not use the simple MCP / CLI / Both labels: %q", output.String())
	}
	if selection.Mode != SetupModeCLI {
		t.Fatal("CLI did not select persistent installation")
	}
}

func TestCLIAccessModeSkipsCodingAgentTargets(t *testing.T) {
	var output bytes.Buffer
	t.Setenv("GITCONTRIBUTE_ACCESSIBLE", "1")
	prompter := &huhSetupPrompter{input: strings.NewReader("2\n4\n"), output: &output}
	selection, err := prompter.Select(context.Background(), SetupPromptRequest{
		Questions: []SetupPromptQuestion{SetupQuestionAccess, SetupQuestionClients, SetupQuestionAuth},
		Discovery: SetupDiscovery{
			ConfiguredTokenSource: "none",
			Clients:               []SetupClientDiscovery{{Name: "codex", Path: "/home/test/.codex/config.toml", Detected: true}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if selection.Mode != SetupModeCLI || len(selection.Clients) != 0 || selection.TokenSource != "none" {
		t.Fatalf("selection = %+v", selection)
	}
	if strings.Contains(output.String(), "/home/test/.codex/config.toml") {
		t.Fatalf("CLI-only flow displayed MCP targets: %q", output.String())
	}
}

func TestMCPAccessModeRequiresExplicitCodingAgentTarget(t *testing.T) {
	var output bytes.Buffer
	t.Setenv("GITCONTRIBUTE_ACCESSIBLE", "1")
	prompter := &huhSetupPrompter{input: strings.NewReader("1\n0\n4\n"), output: &output}
	_, err := prompter.Select(context.Background(), SetupPromptRequest{
		Questions: []SetupPromptQuestion{SetupQuestionAccess, SetupQuestionClients, SetupQuestionAuth},
		Discovery: SetupDiscovery{
			ConfiguredTokenSource: "none",
			Clients:               []SetupClientDiscovery{{Name: "codex", Path: "/home/test/.codex/config.toml", Detected: true}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "select at least one coding agent") {
		t.Fatalf("error = %v", err)
	}
}

func TestBothAccessModeSelectsCLIAndExplicitMCPClient(t *testing.T) {
	var output bytes.Buffer
	t.Setenv("GITCONTRIBUTE_ACCESSIBLE", "1")
	prompter := &huhSetupPrompter{input: &oneByteReader{reader: strings.NewReader("3\n1\n0\n4\n")}, output: &output}
	selection, err := prompter.Select(context.Background(), SetupPromptRequest{
		Questions: []SetupPromptQuestion{SetupQuestionAccess, SetupQuestionClients, SetupQuestionAuth},
		Discovery: SetupDiscovery{
			ConfiguredTokenSource: "none",
			Clients:               []SetupClientDiscovery{{Name: "codex", Path: "/home/test/.codex/config.toml", Detected: true}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if selection.Mode != SetupModeBoth || len(selection.Clients) != 1 || selection.Clients[0] != "codex" {
		t.Fatalf("selection = %+v", selection)
	}
}

func TestRenderSetupDiscoveryUsesEvidenceLanguage(t *testing.T) {
	got := renderSetupDiscovery(SetupDiscovery{
		Version:            "0.3.2",
		GitHubCLIAvailable: true,
		Clients:            []SetupClientDiscovery{{Name: "codex", Detected: true}},
	}, true)
	for _, want := range []string{"GitContribute setup · v0.3.2 · running with npx", "Detected Codex, GitHub CLI", "Local inspection only", "no changes made"} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary %q does not contain %q", got, want)
		}
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

func TestSetupDetectionDoesNotSelectClientWithoutPriorIntent(t *testing.T) {
	clients := []string{}
	setupClientsField("Coding agents", SetupDiscovery{Clients: []SetupClientDiscovery{
		{Name: "codex", Path: "/home/test/.codex/config.toml", Detected: true},
	}}, &clients)
	if len(clients) != 0 {
		t.Fatalf("detected clients were selected without consent: %v", clients)
	}
}

func TestExistingRegistrationEstablishesClientSelectionIntent(t *testing.T) {
	clients := []string{}
	setupClientsField("Coding agents", SetupDiscovery{Clients: []SetupClientDiscovery{
		{Name: "codex", Path: "/home/test/.codex/config.toml", Detected: true, Registered: true},
		{Name: "claude", Path: "/home/test/.claude.json", Detected: true},
	}}, &clients)
	if len(clients) != 1 || clients[0] != "codex" {
		t.Fatalf("existing registration selection = %v", clients)
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
	wants := []string{"✓ Configuration — configured", "✓ Local corpus — initialized", "✓ Verification — verified"}
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

func TestSetupProgressIsDisabledForDryRunAndAccessibleMode(t *testing.T) {
	if setupProgressAnimationAllowed(SetupOptions{DryRun: true}) {
		t.Fatal("dry-run unexpectedly enabled animated progress")
	}
	t.Setenv("GITCONTRIBUTE_ACCESSIBLE", "1")
	if setupProgressAnimationAllowed(SetupOptions{}) {
		t.Fatal("accessible mode unexpectedly enabled animated progress")
	}
}

func TestSetupProgressModelSettlesActiveWorkIntoResult(t *testing.T) {
	model := newSetupProgressModel()
	updated, _ := model.Update(setupStartedMsg(SetupPhaseClients))
	model = updated.(setupProgressModel)
	if got := model.View(); !strings.Contains(got, "Configuring coding agents…") {
		t.Fatalf("active progress = %q", got)
	}

	updated, _ = model.Update(setupCompletedMsg(SetupStep{Name: "codex", Status: "configured"}))
	model = updated.(setupProgressModel)
	got := model.View()
	if got != "✓ Codex — configured" || strings.Contains(got, "Configuring coding agents") {
		t.Fatalf("settled progress = %q", got)
	}
}

func TestRenderSetupPlanIncludesEffectsAndSafetyBoundary(t *testing.T) {
	report := &SetupReport{DryRun: true, MCPCommandPending: true, Authentication: &SetupAuthentication{Method: "gh-cli"}, Steps: []SetupStep{
		{Name: "cli", Status: "would install", Message: "npm install --global gitcontribute@0.2.2"},
		{Name: "codex", Status: "would configure", Path: "/home/test/.codex/config.toml"},
	}}
	got := renderSetupPlan(report)
	for _, want := range []string{
		"Setup plan", "Review these changes", "CLI", "Action: Install",
		"Command: npm install --global gitcontribute@0.2.2",
		"Path: /home/test/.codex/config.toml",
		"MCP command", "Resolved after the CLI installation succeeds",
		"GitHub credentials", "Record: GitHub CLI credential helper", "will not be read or validated",
		"Process execution", "npm prefix --global", "git --version · local verification",
		"Safety", "will not contact GitHub",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("plan %q does not contain %q", got, want)
		}
	}
	if strings.Contains(got, "npx") {
		t.Fatalf("plan presents npx as a persistent runtime: %q", got)
	}
}

func TestRenderSetupPlanPresentsManagedRuntimeForMCPOnly(t *testing.T) {
	got := renderSetupPlan(&SetupReport{DryRun: true, MCPCommand: &SetupMCPCommand{Command: "/home/test/.local/share/gitcontribute/bin/1.2.3/gitcontribute", Args: []string{"mcp", "serve", "--transport=stdio"}}, Steps: []SetupStep{
		{Name: "mcp-runtime", Status: "would install", Path: "/home/test/.local/share/gitcontribute/bin/1.2.3/gitcontribute"},
		{Name: "codex", Status: "would configure", Path: "/home/test/.codex/config.toml"},
	}})
	if !strings.Contains(got, "Private MCP runtime") || !strings.Contains(got, "MCP command\n  Executable: /home/test/.local/share/gitcontribute/bin/1.2.3/gitcontribute\n  Arguments: mcp serve --transport=stdio") {
		t.Fatalf("managed MCP plan = %q", got)
	}
	if strings.Contains(got, "npx") || strings.Contains(got, "Fallback:") {
		t.Fatalf("managed MCP plan contains package-runner runtime language: %q", got)
	}
}

func TestRenderSetupResultTailorsNextCommand(t *testing.T) {
	report := &SetupReport{Authentication: &SetupAuthentication{Method: "gh-cli"}, Steps: []SetupStep{{Name: "codex", Status: "configured"}, {Name: "verification", Status: "verified"}}}
	if got := renderSetupResult(report, SetupOptions{Mode: SetupModeBoth}); !strings.Contains(got, "Next\n  gitcontribute\n") || !strings.Contains(got, "Restart Codex") || !strings.Contains(got, "not read or validated") {
		t.Fatalf("installed result = %q", got)
	}
	if got := renderSetupResult(report, SetupOptions{TokenSource: "none"}); !strings.Contains(got, "Next\n  Use GitContribute from your coding agent.") || strings.Contains(got, "npx") {
		t.Fatalf("MCP-only result = %q", got)
	}
}

func TestRenderSetupResultOmitsAuthenticationWhenSelectionIsUnavailable(t *testing.T) {
	report := &SetupReport{Steps: []SetupStep{{Name: "verification", Status: "verified"}}}
	got := renderSetupResult(report, SetupOptions{Mode: SetupModeCLI})
	if strings.Contains(got, "GitHub credentials") || strings.Contains(got, "Configure later") {
		t.Fatalf("result makes an authentication claim without a selected source: %q", got)
	}
}

func TestRenderSetupPlanNamesEnvironmentAuthenticationWithoutSecret(t *testing.T) {
	report := &SetupReport{DryRun: true, Authentication: &SetupAuthentication{Method: "env", Key: "GH_CUSTOM"}}
	got := renderSetupPlan(report)
	if !strings.Contains(got, "Record: Environment variable GH_CUSTOM") || !strings.Contains(got, "will not be read or validated") {
		t.Fatalf("environment authentication plan = %q", got)
	}
}

func TestRenderSetupResultReportsPartialFailureTruthfully(t *testing.T) {
	report := &SetupReport{MCPCommand: &SetupMCPCommand{Command: "/home/test/.local/share/gitcontribute/bin/1.2.3/gitcontribute", Args: []string{"mcp", "serve", "--transport=stdio"}}, Steps: []SetupStep{
		{Name: "configuration", Status: "configured", Path: "/home/test/.config/gitcontribute/config.json"},
		{Name: "codex", Status: "failed", Path: "/home/test/.codex/config.toml", Message: "invalid TOML"},
	}}
	got := renderSetupResult(report, SetupOptions{})
	for _, want := range []string{
		"Setup needs attention", "✓ Configuration — configured", "✗ Codex — failed",
		"Path: /home/test/.codex/config.toml", "Details: invalid TOML", "Fix the failed steps",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("partial result %q does not contain %q", got, want)
		}
	}
	for _, unwanted := range []string{"GitContribute is ready", "\nNext\n", "Restart configured"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("partial result %q contains misleading %q", got, unwanted)
		}
	}
}
