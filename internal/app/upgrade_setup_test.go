package app

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/managedbinary"
	clientsetup "github.com/morluto/gitcontribute/internal/setup"
)

func TestUpgradeActivatesPrivateMCPRuntimeFromTargetRelease(t *testing.T) {
	home, _, _, _, svc := setupUpgradeActivationTest(t, "1.2.3", "1.2.4", "1.2.4")
	setRuntimeContract(t, "1.2.4", 1)

	report, err := svc.Upgrade(context.Background(), contracts.UpgradeOptions{Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	dataDir, err := svc.paths.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	wantRuntime, err := managedbinary.Destination(dataDir, "1.2.4")
	if err != nil {
		t.Fatal(err)
	}
	launcher, err := clientsetup.ReadCommandFile(clientsetup.Codex, filepath.Join(home, ".codex", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if launcher.Command != wantRuntime {
		t.Fatalf("configured command = %q, want %q", launcher.Command, wantRuntime)
	}
	if got, err := os.ReadFile(wantRuntime); err != nil || string(got) != "release-1.2.4" {
		t.Fatalf("staged runtime = %q, %v", got, err)
	}
	assertStage(t, report, "private-mcp-runtime", "verified")
	assertStage(t, report, "configured-runtime", "activated")
	assertStage(t, report, "activation", "restart_required")
	if report.Status != "restart required" || !reflect.DeepEqual(report.RestartClients, []string{"codex"}) {
		t.Fatalf("report = %+v", report)
	}
}

func TestUpgradeNpxActivatesPrivateMCPRuntimeFromLatestRelease(t *testing.T) {
	t.Setenv("npm_command", "exec")
	home, _, _, _, svc := setupUpgradeActivationTest(t, "1.2.3", "1.2.4", "1.2.4")
	setRuntimeContract(t, "1.2.4", 1)

	report, err := svc.Upgrade(context.Background(), contracts.UpgradeOptions{Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.Context != "npx" {
		t.Fatalf("context = %q, want npx", report.Context)
	}
	dataDir, err := svc.paths.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	wantRuntime, err := managedbinary.Destination(dataDir, "1.2.4")
	if err != nil {
		t.Fatal(err)
	}
	launcher, err := clientsetup.ReadCommandFile(clientsetup.Codex, filepath.Join(home, ".codex", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if launcher.Command != wantRuntime {
		t.Fatalf("configured command = %q, want %q", launcher.Command, wantRuntime)
	}
	if got, err := os.ReadFile(wantRuntime); err != nil || string(got) != "release-1.2.4" {
		t.Fatalf("staged runtime = %q, %v", got, err)
	}
	assertStage(t, report, "private-mcp-runtime", "verified")
	assertStage(t, report, "configured-runtime", "activated")
	assertStage(t, report, "activation", "restart_required")
	if report.Status != "restart required" || !reflect.DeepEqual(report.RestartClients, []string{"codex"}) {
		t.Fatalf("report = %+v", report)
	}
}
