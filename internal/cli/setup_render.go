package cli

import (
	"fmt"
	"strings"
)

func renderSetupPlan(report *SetupReport) string {
	var b strings.Builder
	b.WriteString("Setup plan\n")
	for _, step := range report.Steps {
		writeSetupStep(&b, step, true)
	}
	if report.Launcher != "" {
		fmt.Fprintf(&b, "\n  MCP launcher\n    %s\n", report.Launcher)
	}
	b.WriteString("\n  GitContribute will not contact GitHub, synchronize repositories,\n")
	b.WriteString("  execute repository code, or mutate GitHub during setup.")
	return b.String()
}

func renderSetupResult(report *SetupReport, opts SetupOptions, persistentCommand bool) string {
	var b strings.Builder
	if report.HasFailures() {
		b.WriteString("GitContribute setup finished with errors\n")
	} else {
		b.WriteString("◆ GitContribute is ready\n")
	}
	for _, step := range report.Steps {
		writeSetupStep(&b, step, false)
	}
	if !opts.SkipMCP && !report.HasFailures() {
		b.WriteString("\n  Restart the configured coding agents to load the MCP server.\n")
	}
	if !report.HasFailures() {
		b.WriteString("\nNext:\n")
		if persistentCommand {
			b.WriteString("  gitcontribute tui\n  gitcontribute sync owner/repo")
		} else {
			b.WriteString("  npx gitcontribute@latest tui\n  npx gitcontribute@latest sync owner/repo")
		}
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
	fmt.Fprintf(b, "\n  %s %-18s %s\n", symbol, setupStepLabel(step.Name), step.Status)
	if step.Path != "" {
		fmt.Fprintf(b, "    %s\n", step.Path)
	}
	if step.Message != "" {
		fmt.Fprintf(b, "    %s\n", step.Message)
	}
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
