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
	if report.Launcher != "" {
		b.WriteString("\nMCP launcher\n")
		if setupPlanInstallsTerminal(report) {
			fmt.Fprintf(&b, "  Fallback: %s\n", report.Launcher)
			b.WriteString("  If installation succeeds, setup registers the verified global executable instead.\n")
		} else {
			fmt.Fprintf(&b, "  Command: %s\n", report.Launcher)
		}
	}
	writeSetupAuthentication(&b, report.Authentication, true)
	b.WriteString("\nSafety\n")
	if setupPlanInstallsTerminal(report) {
		b.WriteString("  Setup changes local configuration and runs only the global npm install shown above.\n")
	} else {
		b.WriteString("  Setup only makes the local changes shown above.\n")
	}
	b.WriteString("  It will not contact GitHub, synchronize repositories, execute repository code,\n")
	b.WriteString("  or mutate GitHub.")
	return b.String()
}

func renderSetupResult(report *SetupReport, _ SetupOptions, persistentCommand bool) string {
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
	if report.Launcher != "" {
		fmt.Fprintf(&b, "\nMCP launcher\n  %s\n", report.Launcher)
	}
	if !report.HasFailures() {
		writeSetupAuthentication(&b, report.Authentication, false)
	}
	if clients := configuredSetupClients(report); len(clients) > 0 && !report.HasFailures() {
		fmt.Fprintf(&b, "\nRestart %s to load the MCP server.\n", strings.Join(clients, " and "))
	}
	if !report.HasFailures() {
		b.WriteString("\nNext\n")
		if persistentCommand {
			b.WriteString("  gitcontribute\n  gitcontribute sync owner/repo")
		} else {
			b.WriteString("  npx gitcontribute@latest\n  npx gitcontribute@latest sync owner/repo")
		}
	} else {
		b.WriteString("\nFix the failed steps, then run setup again.")
	}
	return strings.TrimRight(b.String(), "\n")
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
		fmt.Fprintf(b, "\n  %s %s\n    Action: %s\n", symbol, setupStepLabel(step.Name), setupPlanAction(step.Status))
	} else {
		fmt.Fprintf(b, "\n  %s %s — %s\n", symbol, setupStepLabel(step.Name), step.Status)
	}
	if step.Path != "" {
		fmt.Fprintf(b, "    Path: %s\n", step.Path)
	}
	if step.Message != "" {
		label := "Details"
		if plan && step.Name == "terminal" && step.Status == "would install" {
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
	case "would initialize":
		return "Initialize"
	case "would add":
		return "Add"
	case "already configured":
		return "Keep · already configured"
	case "not installed":
		return "Skip"
	default:
		return status
	}
}

func setupPlanInstallsTerminal(report *SetupReport) bool {
	for _, step := range report.Steps {
		if step.Name == "terminal" && step.Status == "would install" {
			return true
		}
	}
	return false
}

func configuredSetupClients(report *SetupReport) []string {
	clients := make([]string, 0, 2)
	for _, step := range report.Steps {
		if (step.Name == "codex" || step.Name == "claude") && step.Status != "failed" && !strings.HasPrefix(step.Status, "skipped") && !strings.HasPrefix(step.Status, "would ") {
			clients = append(clients, setupStepLabel(step.Name))
		}
	}
	return clients
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
	case "terminal":
		return "Terminal app"
	case "configuration":
		return "Configuration"
	case "corpus":
		return "Local corpus"
	case "codex":
		return "Codex"
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
