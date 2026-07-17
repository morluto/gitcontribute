// Package terminalinstall owns the external package-manager capability used to
// make the GitContribute CLI and TUI persistently available.
package terminalinstall

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// GlobalNPM installs packageSpec into npm's global prefix and returns the
// verified command path. The caller must obtain explicit user authorization
// before calling it: this function executes npm and may access the registry and
// mutate files outside GitContribute's application directories.
//
// A successful npm exit is not sufficient. GlobalNPM resolves npm's actual
// prefix and verifies the platform-specific command shim so MCP registration
// never records an assumed or missing path. It does not modify shell startup
// files or the parent process's PATH.
func GlobalNPM(ctx context.Context, packageSpec string) (string, error) {
	output, err := exec.CommandContext(ctx, "npm", "install", "--global", packageSpec).CombinedOutput()
	if err != nil {
		return "", commandFailure("install persistent CLI", output, err)
	}
	prefixOutput, err := exec.CommandContext(ctx, "npm", "prefix", "--global").CombinedOutput()
	if err != nil {
		return "", commandFailure("resolve global npm prefix", prefixOutput, err)
	}
	prefix := strings.TrimSpace(string(prefixOutput))
	commandPath := filepath.Join(prefix, "bin", "gitcontribute")
	if runtime.GOOS == "windows" {
		commandPath = filepath.Join(prefix, "gitcontribute.cmd")
	}
	if _, err := os.Stat(commandPath); err != nil {
		return "", fmt.Errorf("verify persistent CLI at %s: %w", commandPath, err)
	}
	return commandPath, nil
}

func commandFailure(action string, output []byte, err error) error {
	detail := strings.TrimSpace(string(output))
	if detail == "" {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%s: %w: %s", action, err, detail)
}
