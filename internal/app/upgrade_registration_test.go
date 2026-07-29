package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/contracts"
)

func TestUpgradeCheckReportsStaleRegistration(t *testing.T) {
	originalCmd := upgradeCommand
	originalExec := osExecutable
	t.Cleanup(func() {
		upgradeCommand = originalCmd
		osExecutable = originalExec
	})
	upgradeCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "npm" && reflect.DeepEqual(args, []string{"view", "gitcontribute", "version"}) {
			return []byte("1.2.3\n"), nil
		}
		t.Fatalf("unexpected command: %s %v", name, args)
		return nil, nil
	}
	osExecutable = func() (string, error) { return "/opt/gitcontribute", nil }

	home := t.TempDir()
	command := filepath.Join(home, "bin", "1.2.3", "gitcontribute")
	writeStaleCodexConfig(t, home, command)
	svc := testService(t, home, "1.2.3", "")

	report, err := svc.Upgrade(context.Background(), contracts.UpgradeOptions{Check: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.ConfiguredClients[0].Status != "stale" {
		t.Fatalf("configured client = %+v", report.ConfiguredClients[0])
	}
	if report.Status != "registration repair required" {
		t.Fatalf("status = %q", report.Status)
	}
	assertStage(t, report, "configured-runtime", "repair_required")
	assertStage(t, report, "activation", "repair_required")
	if !strings.Contains(report.Action, "pass --yes") {
		t.Fatalf("action = %q", report.Action)
	}
}

func TestUpgradeYesRepairsStaleRegistrationAndRequiresRestart(t *testing.T) {
	originalCmd := upgradeCommand
	originalExec := osExecutable
	t.Cleanup(func() {
		upgradeCommand = originalCmd
		osExecutable = originalExec
	})
	upgradeCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "npm" && reflect.DeepEqual(args, []string{"view", "gitcontribute", "version"}) {
			return []byte("1.2.3\n"), nil
		}
		t.Fatalf("unexpected command: %s %v", name, args)
		return nil, nil
	}
	osExecutable = func() (string, error) { return "/opt/gitcontribute", nil }

	home := t.TempDir()
	command := filepath.Join(home, "bin", "1.2.3", "gitcontribute")
	writeStaleCodexConfig(t, home, command)
	svc := testService(t, home, "1.2.3", "")

	report, err := svc.Upgrade(context.Background(), contracts.UpgradeOptions{Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	assertStage(t, report, "configured-runtime", "repaired")
	assertStage(t, report, "activation", "restart_required")
	if report.Status != "restart required" || !reflect.DeepEqual(report.RestartClients, []string{"codex"}) {
		t.Fatalf("report = %+v", report)
	}
	if report.ConfiguredClients[0].Status == "stale" {
		t.Fatalf("configured client was not refreshed: %+v", report.ConfiguredClients[0])
	}
	data, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `args = ["mcp", "serve", "--transport=stdio"]`) {
		t.Fatalf("registration was not repaired:\n%s", data)
	}
}

func writeStaleCodexConfig(t *testing.T, home, command string) {
	t.Helper()
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf("[mcp_servers.gitcontribute]\ncommand = %q\nargs = [\"mcp\"]\n", command)
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
