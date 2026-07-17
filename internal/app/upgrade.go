package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/morluto/gitcontribute/internal/cli"
)

var upgradeCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// Upgrade checks npm for the latest release and updates persistent npm
// installations when explicitly authorized.
func (s *Service) Upgrade(ctx context.Context, opts cli.UpgradeOptions) (*cli.UpgradeReport, error) {
	installation := detectInstallContext(ctx)
	command := "npm install --global gitcontribute@latest"
	if installation == "project-npm" {
		command = "npm install --save-dev gitcontribute@latest"
	}
	report := &cli.UpgradeReport{Context: installation, Current: s.version, Command: command}
	output, err := upgradeCommand(ctx, "npm", "view", "gitcontribute", "version")
	if err != nil {
		return nil, fmt.Errorf("check latest npm release: %w", err)
	}
	report.Latest = strings.TrimSpace(string(output))
	if report.Latest == strings.TrimPrefix(s.version, "v") {
		report.Status = "already current"
	} else {
		report.Status = "update available"
	}
	if opts.Check || !opts.Yes {
		return report, nil
	}
	if installation == "npx" {
		report.Status = "npx uses the requested package version; nothing is installed globally"
		return report, nil
	}
	if installation != "global-npm" {
		report.Status = "installation method is not managed automatically"
		return report, nil
	}
	if runtime.GOOS == "windows" {
		report.Status = "close running GitContribute processes, then run the displayed command"
		return report, nil
	}
	if _, err := upgradeCommand(ctx, "npm", "install", "--global", "gitcontribute@latest"); err != nil {
		return nil, fmt.Errorf("install latest npm release: %w", err)
	}
	report.Status = "updated"
	return report, nil
}

func detectInstallContext(ctx context.Context) string {
	if os.Getenv("npm_command") == "exec" || os.Getenv("npm_lifecycle_event") == "npx" {
		return "npx"
	}
	executable, err := os.Executable()
	if err != nil {
		return "other"
	}
	normalized := filepath.ToSlash(executable)
	if !strings.Contains(normalized, "/node_modules/gitcontribute/") {
		return "other"
	}
	globalRoot, err := upgradeCommand(ctx, "npm", "root", "--global")
	if err != nil {
		return "project-npm"
	}
	return classifyNPMExecutable(executable, strings.TrimSpace(string(globalRoot)))
}

func classifyNPMExecutable(executable, globalRoot string) string {
	executable = filepath.Clean(executable)
	globalPackage := filepath.Join(filepath.Clean(globalRoot), "gitcontribute")
	relative, err := filepath.Rel(globalPackage, executable)
	if err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "global-npm"
	}
	return "project-npm"
}
