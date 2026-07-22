package cli

import (
	"fmt"
	"strings"
)

func renderSetupPlan(report *SetupReport) string {
	var b strings.Builder
	b.WriteString("Setup plan\n\nReview these changes")
	for _, step := range report.Steps {
		writeSetupStep(&b, step, true)
	}
	if report.MCPCommandPending {
		b.WriteString("\nMCP command\n  Resolved after the CLI installation succeeds.\n")
	} else if report.MCPCommand != nil {
		writeSetupMCPCommand(&b, report.MCPCommand)
	}
	writeSetupAuthentication(&b, report.Authentication, true)
	b.WriteString("\nProcess execution\n")
	if setupPlanInstallsCLI(report) {
		fmt.Fprintf(&b, "  %s\n", setupCLIInstallCommand(report))
		b.WriteString("  npm prefix --global\n")
	} else if setupPlanInstallsManagedRuntime(report) {
		b.WriteString("  No package-manager process; setup copies the running executable locally.\n")
	} else {
		b.WriteString("  No package-manager process.\n")
	}
	b.WriteString("  git --version · local verification\n")
	b.WriteString("\nSafety\n")
	b.WriteString("  It will not contact GitHub, synchronize repositories, execute repository code,\n")
	b.WriteString("  or mutate GitHub.")
	return b.String()
}

func setupCLIInstallCommand(report *SetupReport) string {
	for _, step := range report.Steps {
		if step.Name == "cli" && step.Message != "" {
			return step.Message
		}
	}
	return "npm install --global gitcontribute"
}

func renderSetupResult(report *SetupReport, opts SetupOptions) string {
	var b strings.Builder
	if report.HasFailures() {
		b.WriteString("✗ Setup needs attention\n\n")
		b.WriteString("Completed steps are listed below; failed steps need attention.\n\nResults")
	} else {
		b.WriteString("✓ GitContribute is ready\n\nResults")
	}
	for _, step := range report.Steps {
		writeSetupStep(&b, step, false)
	}
	if report.MCPCommand != nil {
		writeSetupMCPCommand(&b, report.MCPCommand)
	}
	if !report.HasFailures() {
		writeSetupAuthentication(&b, report.Authentication, false)
	}
	if len(report.RestartClients) > 0 && !report.HasFailures() {
		clients := make([]string, len(report.RestartClients))
		for i, client := range report.RestartClients {
			clients[i] = setupStepLabel(client)
		}
		fmt.Fprintf(&b, "\nRestart %s to replace active MCP processes with the configured runtime.\n", strings.Join(clients, " and "))
	}
	if !report.HasFailures() {
		b.WriteString("\nNext\n")
		if opts.Mode.InstallsCLI() {
			b.WriteString("  gitcontribute\n  gitcontribute sync owner/repo")
		} else {
			b.WriteString("  Use GitContribute from your coding agent.")
		}
	} else {
		b.WriteString("\nFix the failed steps, then run setup again.")
	}
	return strings.TrimRight(b.String(), "\n")
}

func writeSetupMCPCommand(b *strings.Builder, command *SetupMCPCommand) {
	if command == nil {
		return
	}
	fmt.Fprintf(b, "\nMCP command\n  Executable: %s\n", command.Command)
	if len(command.Args) > 0 {
		fmt.Fprintf(b, "  Arguments: %s\n", strings.Join(command.Args, " "))
	}
}

func writeSetupStep(b *strings.Builder, step SetupStep, plan bool) {
	symbol := "✓"
	if step.Status == "failed" {
		symbol = "✗"
	} else if step.Status == "not installed" || strings.HasPrefix(step.Status, "skipped") {
		symbol = "○"
	} else if plan {
		symbol = "•"
	}
	if plan {
		label := "Action"
		if step.Status == "failed" {
			label = "Status"
		}
		fmt.Fprintf(b, "\n  %s %s\n    %s: %s\n", symbol, setupStepLabel(step.Name), label, setupPlanAction(step.Status))
	} else {
		fmt.Fprintf(b, "\n  %s %s — %s\n", symbol, setupStepLabel(step.Name), step.Status)
	}
	if step.Path != "" {
		fmt.Fprintf(b, "    Path: %s\n", step.Path)
	}
	if step.Message != "" {
		label := "Details"
		if plan && step.Name == "cli" && step.Status == "would install" {
			label = "Command"
		}
		fmt.Fprintf(b, "    %s: %s\n", label, step.Message)
	}
}

func setupPlanAction(status string) string {
	switch status {
	case "would install":
		return "Install"
	case "would configure":
		return "Configure"
	case "would update":
		return "Update"
	case "would remove":
		return "Remove"
	case "would initialize":
		return "Initialize"
	case "would add":
		return "Add"
	case "already configured":
		return "Keep · already configured"
	case "not installed", "not configured":
		return "Skip"
	case "failed":
		return "Blocked"
	default:
		return status
	}
}

func setupPlanInstallsCLI(report *SetupReport) bool {
	for _, step := range report.Steps {
		if step.Name == "cli" && step.Status == "would install" {
			return true
		}
	}
	return false
}

func setupPlanInstallsManagedRuntime(report *SetupReport) bool {
	for _, step := range report.Steps {
		if step.Name == "mcp-runtime" && step.Status == "would install" {
			return true
		}
	}
	return false
}

func writeSetupAuthentication(b *strings.Builder, auth *SetupAuthentication, plan bool) {
	if auth == nil || strings.TrimSpace(auth.Method) == "" {
		return
	}
	label := "Configure later"
	switch auth.Method {
	case "gh-cli":
		label = "GitHub CLI credential helper"
	case "env":
		key := auth.Key
		if key == "" {
			key = "GITHUB_TOKEN"
		}
		label = "Environment variable " + key
	case "keyring":
		label = "System keyring"
		if auth.Key != "" {
			label += " entry " + auth.Key
		}
	}
	if plan {
		fmt.Fprintf(b, "\nGitHub credentials\n  Record: %s\n  Credentials will not be read or validated during setup.\n", label)
		return
	}
	if auth.Method == "none" {
		fmt.Fprint(b, "\nGitHub credentials\n  Configure later · offline features are ready\n")
		return
	}
	fmt.Fprintf(b, "\nGitHub credentials\n  %s selected · not read or validated during setup\n", label)
}

func setupStepLabel(name string) string {
	switch name {
	case "cli":
		return "CLI"
	case "mcp-runtime":
		return "Private MCP runtime"
	case "configuration":
		return "Configuration"
	case "corpus":
		return "Local corpus"
	case "codex":
		return "Codex"
	case "codex-skill":
		return "Codex skill"
	case "claude":
		return "Claude Code"
	case "repository":
		return "Repository source"
	case "verification":
		return "Verification"
	default:
		return name
	}
}
