package app

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/managedbinary"
	_ "modernc.org/sqlite"
)

func TestUpgradeNpxDoesNotInstallGlobalPackage(t *testing.T) {
	original := upgradeCommand
	t.Cleanup(func() { upgradeCommand = original })
	var calls [][]string
	upgradeCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		return []byte("1.2.4\n"), nil
	}
	t.Setenv("npm_command", "exec")
	svc := &Service{version: "1.2.3", paths: config.NewPaths(&config.Env{Home: t.TempDir()})}
	report, err := svc.Upgrade(context.Background(), cli.UpgradeOptions{Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.Context != "npx" || len(calls) != 1 {
		t.Fatalf("report=%+v calls=%v", report, calls)
	}
	want := []string{"npm", "view", "gitcontribute", "version"}
	if !reflect.DeepEqual(calls[0], want) {
		t.Fatalf("call = %v, want %v", calls[0], want)
	}
	if report.Status != "npx" {
		t.Fatalf("status = %q, want npx", report.Status)
	}
	if report.Command != "" {
		t.Fatalf("command = %q, want empty for npx", report.Command)
	}
	assertStage(t, report, "installation", "npx")
}

func TestUpgradeDoesNotInstallAcrossSchemaIncompatibility(t *testing.T) {
	report := &cli.UpgradeReport{
		Context: "global-npm", Current: "1.2.3", Latest: "1.2.4",
		Stages: []cli.UpgradeStage{{Name: "corpus-schema", Status: "migration_required"}},
	}
	if shouldInstall(report, cli.UpgradeOptions{Yes: true}) {
		t.Fatal("schema-incompatible upgrade was authorized for installation")
	}
}

func TestUpgradeUsesSemanticVersionOrdering(t *testing.T) {
	originalGOOS := upgradeGOOS
	t.Cleanup(func() { upgradeGOOS = originalGOOS })
	upgradeGOOS = "linux"

	tests := []struct {
		name        string
		current     string
		target      string
		wantStatus  string
		wantStage   string
		wantInstall bool
	}{
		{name: "no-op", current: "1.2.3", target: "1.2.3", wantStatus: "already current", wantStage: "current"},
		{name: "build metadata is no-op", current: "1.2.3+local", target: "1.2.3+registry", wantStatus: "already current", wantStage: "current"},
		{name: "upgrade", current: "1.2.3", target: "1.2.4", wantStatus: "update available", wantStage: "update_available", wantInstall: true},
		{name: "prerelease", current: "1.2.3", target: "1.3.0-rc.1", wantStatus: "prerelease available", wantStage: "prerelease_available", wantInstall: true},
		{name: "downgrade", current: "2.0.0", target: "1.9.0", wantStatus: "newer version installed", wantStage: "newer_installed"},
		{name: "prerelease downgrade", current: "2.0.0-rc.2", target: "2.0.0-rc.1", wantStatus: "newer version installed", wantStage: "newer_installed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globalRoot := t.TempDir()
			packageRoot := filepath.Join(globalRoot, "gitcontribute")
			writePackageJSON(t, packageRoot, tt.current)
			details := installDetails{context: "global-npm", npmRoot: globalRoot}
			stage := npmLauncherStage(details, tt.current, tt.target)
			if stage.Status != tt.wantStage {
				t.Fatalf("npm launcher status = %q, want %q", stage.Status, tt.wantStage)
			}
			report := &cli.UpgradeReport{
				Context: "global-npm",
				Current: tt.current,
				Latest:  tt.target,
				Stages: []cli.UpgradeStage{
					stage,
					{Name: "corpus-schema", Status: "current"},
				},
			}
			setCommandAndStatus(report)
			if report.Status != tt.wantStatus {
				t.Fatalf("status = %q, want %q", report.Status, tt.wantStatus)
			}
			if got := shouldInstall(report, cli.UpgradeOptions{Yes: true}); got != tt.wantInstall {
				t.Fatalf("shouldInstall = %t, want %t", got, tt.wantInstall)
			}
		})
	}
}

func TestDetectInstallContextDefaultsOther(t *testing.T) {
	t.Setenv("npm_command", "")
	t.Setenv("npm_lifecycle_event", "")
	if got := detectInstallContext(context.Background()); got == "npx" {
		t.Fatalf("context = %s", got)
	}
}

func TestClassifyNPMExecutableDistinguishesProjectAndGlobalInstalls(t *testing.T) {
	globalRoot := filepath.Join(string(filepath.Separator), "usr", "local", "lib", "node_modules")
	global := filepath.Join(globalRoot, "gitcontribute", "npm", "bin", "native", "linux-x64", "gitcontribute")
	project := filepath.Join(string(filepath.Separator), "work", "project", "node_modules", "gitcontribute", "npm", "bin", "native", "linux-x64", "gitcontribute")
	if got := classifyNPMExecutable(global, globalRoot); got != "global-npm" {
		t.Fatalf("global context = %q", got)
	}
	if got := classifyNPMExecutable(project, globalRoot); got != "project-npm" {
		t.Fatalf("project context = %q", got)
	}
}

func TestUpgradeReportsInspectableStagesForGlobalNPM(t *testing.T) {
	originalCmd := upgradeCommand
	originalExec := osExecutable
	t.Cleanup(func() {
		upgradeCommand = originalCmd
		osExecutable = originalExec
	})

	home := t.TempDir()
	globalRoot := filepath.Join(home, "global", "lib", "node_modules")
	pkgRoot := filepath.Join(globalRoot, "gitcontribute")
	exe := filepath.Join(pkgRoot, "npm", "bin", "native", "linux-x64", "gitcontribute")
	if err := os.MkdirAll(filepath.Dir(exe), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	writePackageJSON(t, pkgRoot, "1.2.3")

	osExecutable = func() (string, error) { return exe, nil }
	upgradeCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		if len(args) >= 2 && args[0] == "root" && args[1] == "--global" {
			return []byte(globalRoot + "\n"), nil
		}
		if len(args) >= 2 && args[0] == "view" && args[1] == "gitcontribute" {
			return []byte("1.2.4\n"), nil
		}
		t.Fatalf("unexpected command: %v %v", name, args)
		return nil, nil
	}

	svc := testService(t, home, "1.2.3", "")
	report, err := svc.Upgrade(context.Background(), cli.UpgradeOptions{Check: true})
	if err != nil {
		t.Fatal(err)
	}

	if report.Context != "global-npm" {
		t.Fatalf("context = %q", report.Context)
	}
	if report.Status != "update available" {
		t.Fatalf("status = %q, want update available", report.Status)
	}
	if report.Command != "npm install --global gitcontribute@1.2.4" {
		t.Fatalf("command = %q", report.Command)
	}

	assertStage(t, report, "installation", "global-npm")
	assertStage(t, report, "npm-launcher", "update_available")
	assertStage(t, report, "private-mcp-runtime", "not_installed")
	assertStage(t, report, "configured-runtime", "not_configured")
	assertStage(t, report, "corpus-schema", "not_configured")
	assertStage(t, report, "activation", "review")
	assertStage(t, report, "rollback", "limited")
}

func TestUpgradeGlobalNPMInstallsLatest(t *testing.T) {
	originalCmd := upgradeCommand
	originalExec := osExecutable
	originalGOOS := upgradeGOOS
	t.Cleanup(func() {
		upgradeCommand = originalCmd
		osExecutable = originalExec
		upgradeGOOS = originalGOOS
	})
	upgradeGOOS = "linux"

	home := t.TempDir()
	globalRoot := filepath.Join(home, "global", "lib", "node_modules")
	pkgRoot := filepath.Join(globalRoot, "gitcontribute")
	exe := filepath.Join(pkgRoot, "npm", "bin", "native", "linux-x64", "gitcontribute")
	if err := os.MkdirAll(filepath.Dir(exe), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	writePackageJSON(t, pkgRoot, "1.2.3")

	osExecutable = func() (string, error) { return exe, nil }
	var installArgs []string
	upgradeCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		if len(args) >= 2 && args[0] == "root" && args[1] == "--global" {
			return []byte(globalRoot + "\n"), nil
		}
		if len(args) >= 2 && args[0] == "view" && args[1] == "gitcontribute" {
			return []byte("1.2.4\n"), nil
		}
		if len(args) >= 2 && args[0] == "install" && args[1] == "--global" {
			installArgs = append([]string{name}, args...)
			writePackageJSON(t, pkgRoot, "1.2.4")
			return nil, nil
		}
		t.Fatalf("unexpected command: %v %v", name, args)
		return nil, nil
	}

	svc := testService(t, home, "1.2.3", "")
	report, err := svc.Upgrade(context.Background(), cli.UpgradeOptions{Yes: true})
	if err != nil {
		t.Fatal(err)
	}

	if report.Status != "updated" {
		t.Fatalf("status = %q, want updated", report.Status)
	}
	if report.Command != "" {
		t.Fatalf("command = %q, want empty after install", report.Command)
	}
	if len(installArgs) == 0 || installArgs[len(installArgs)-1] != "gitcontribute@1.2.4" {
		t.Fatalf("install args = %v", installArgs)
	}
	assertStage(t, report, "npm-launcher", "updated")
}

func TestUpgradeWindowsGlobalNPMDoesNotInstall(t *testing.T) {
	originalCmd := upgradeCommand
	originalExec := osExecutable
	originalGOOS := upgradeGOOS
	t.Cleanup(func() {
		upgradeCommand = originalCmd
		osExecutable = originalExec
		upgradeGOOS = originalGOOS
	})

	home := t.TempDir()
	globalRoot := filepath.Join(home, "global", "lib", "node_modules")
	pkgRoot := filepath.Join(globalRoot, "gitcontribute")
	exe := filepath.Join(pkgRoot, "npm", "bin", "native", "linux-x64", "gitcontribute")
	if err := os.MkdirAll(filepath.Dir(exe), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	writePackageJSON(t, pkgRoot, "1.2.3")

	osExecutable = func() (string, error) { return exe, nil }
	upgradeGOOS = "windows"
	var calls [][]string
	upgradeCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		if len(args) >= 2 && args[0] == "root" && args[1] == "--global" {
			return []byte(globalRoot + "\n"), nil
		}
		if len(args) >= 2 && args[0] == "view" && args[1] == "gitcontribute" {
			return []byte("1.2.4\n"), nil
		}
		t.Fatalf("unexpected command: %v %v", name, args)
		return nil, nil
	}

	svc := testService(t, home, "1.2.3", "")
	report, err := svc.Upgrade(context.Background(), cli.UpgradeOptions{Yes: true})
	if err != nil {
		t.Fatal(err)
	}

	if report.Status != "update available" {
		t.Fatalf("status = %q", report.Status)
	}
	if report.Command != "npm install --global gitcontribute@1.2.4" {
		t.Fatalf("command = %q", report.Command)
	}
	if strings.Contains(joinCalls(calls), "install") {
		t.Fatal("npm install should not run on windows")
	}
	assertStage(t, report, "activation", "manual")
}

func TestUpgradeProjectNPMReportsManualUpdate(t *testing.T) {
	originalCmd := upgradeCommand
	originalExec := osExecutable
	t.Cleanup(func() {
		upgradeCommand = originalCmd
		osExecutable = originalExec
	})

	home := t.TempDir()
	projectRoot := filepath.Join(home, "project")
	pkgRoot := filepath.Join(projectRoot, "node_modules", "gitcontribute")
	exe := filepath.Join(pkgRoot, "npm", "bin", "native", "linux-x64", "gitcontribute")
	if err := os.MkdirAll(filepath.Dir(exe), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	writePackageJSON(t, pkgRoot, "1.2.3")

	osExecutable = func() (string, error) { return exe, nil }
	upgradeCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		if len(args) >= 2 && args[0] == "root" && args[1] == "--global" {
			return []byte("/other/global/lib/node_modules\n"), nil
		}
		if len(args) >= 2 && args[0] == "view" && args[1] == "gitcontribute" {
			return []byte("1.2.4\n"), nil
		}
		t.Fatalf("unexpected command: %v %v", name, args)
		return nil, nil
	}

	svc := testService(t, home, "1.2.3", "")
	report, err := svc.Upgrade(context.Background(), cli.UpgradeOptions{Check: true})
	if err != nil {
		t.Fatal(err)
	}

	if report.Context != "project-npm" {
		t.Fatalf("context = %q", report.Context)
	}
	if report.Status != "update available" {
		t.Fatalf("status = %q", report.Status)
	}
	if report.Command != "npm install --save-dev gitcontribute@1.2.4" {
		t.Fatalf("command = %q", report.Command)
	}
	assertStage(t, report, "npm-launcher", "update_available")
}

func TestUpgradePrivateMCPRuntimeDetected(t *testing.T) {
	home := t.TempDir()
	paths := config.NewPaths(&config.Env{Home: home})
	dataDir, err := paths.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	runtimePath, err := managedbinary.Destination(dataDir, "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(runtimePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runtimePath, []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}

	svc := testService(t, home, "1.2.3", "")
	report, err := svc.Upgrade(context.Background(), cli.UpgradeOptions{})
	if err != nil {
		t.Fatal(err)
	}

	stage := stageByName(report, "private-mcp-runtime")
	if stage.Status != "installed" {
		t.Fatalf("private runtime status = %q, want installed", stage.Status)
	}
	if stage.Version != "1.2.3" {
		t.Fatalf("private runtime version = %q", stage.Version)
	}
	if stage.Path != runtimePath {
		t.Fatalf("private runtime path = %q, want %q", stage.Path, runtimePath)
	}
}

func TestUpgradeConfiguredRuntimeOutdated(t *testing.T) {
	home := t.TempDir()
	runtimePath := filepath.Join(home, ".local", "share", "gitcontribute", "bin", "1.2.3", "gitcontribute")
	writeCodexConfig(t, home, runtimePath)

	svc := testService(t, home, "1.2.4", "")
	report, err := svc.Upgrade(context.Background(), cli.UpgradeOptions{Check: true})
	if err != nil {
		t.Fatal(err)
	}

	if len(report.ConfiguredClients) == 0 {
		t.Fatal("expected configured clients")
	}
	codex := report.ConfiguredClients[0]
	if codex.Status != "outdated" {
		t.Fatalf("codex status = %q, want outdated", codex.Status)
	}
	if codex.Version != "1.2.3" {
		t.Fatalf("codex version = %q", codex.Version)
	}
	assertStage(t, report, "configured-runtime", "restart_required")
	assertStage(t, report, "activation", "setup_required")
	if len(report.RestartClients) != 1 || report.RestartClients[0] != "codex" {
		t.Fatalf("restart clients = %v", report.RestartClients)
	}
}

func TestUpgradeActivatesPrivateMCPRuntimeFromTargetRelease(t *testing.T) {
	home, _, _, _, svc := setupUpgradeActivationTest(t, "1.2.3", "1.2.4", "1.2.4")
	setRuntimeContract(t, "1.2.4", 1)

	report, err := svc.Upgrade(context.Background(), cli.UpgradeOptions{Yes: true})
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
	command, _, err := readCodexCommand(filepath.Join(home, ".codex", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if command != wantRuntime {
		t.Fatalf("configured command = %q, want %q", command, wantRuntime)
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

func TestUpgradeCombinedInstallActivatesPrivateRuntimeFromInstalledPackage(t *testing.T) {
	originalCmd := upgradeCommand
	originalExec := osExecutable
	originalGOOS := upgradeGOOS
	t.Cleanup(func() {
		upgradeCommand = originalCmd
		osExecutable = originalExec
		upgradeGOOS = originalGOOS
	})
	upgradeGOOS = "linux"
	setRuntimeContract(t, "1.2.4", 1)

	home := t.TempDir()
	globalRoot := filepath.Join(home, "global", "lib", "node_modules")
	pkgRoot := filepath.Join(globalRoot, "gitcontribute")
	source := filepath.Join(pkgRoot, "npm", "bin", "native", "linux-x64", "gitcontribute")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("release-1.2.3"), 0o755); err != nil {
		t.Fatal(err)
	}
	writePackageJSON(t, pkgRoot, "1.2.3")
	osExecutable = func() (string, error) { return source, nil }
	upgradeCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		switch {
		case name == "npm" && len(args) >= 2 && args[0] == "root":
			return []byte(globalRoot + "\n"), nil
		case name == "npm" && len(args) >= 2 && args[0] == "view":
			return []byte("1.2.4\n"), nil
		case name == "npm" && len(args) >= 2 && args[0] == "install":
			writePackageJSON(t, pkgRoot, "1.2.4")
			if err := os.WriteFile(source, []byte("release-1.2.4"), 0o755); err != nil {
				t.Fatal(err)
			}
			return nil, nil
		default:
			t.Fatalf("unexpected command: %s %v", name, args)
			return nil, nil
		}
	}
	oldRuntime := filepath.Join(home, ".local", "share", "gitcontribute", "bin", "1.2.3", "gitcontribute")
	writeCodexConfig(t, home, oldRuntime)

	svc := testService(t, home, "1.2.3", "")
	report, err := svc.Upgrade(context.Background(), cli.UpgradeOptions{Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	assertStage(t, report, "npm-launcher", "updated")
	assertStage(t, report, "private-mcp-runtime", "verified")
	assertStage(t, report, "activation", "restart_required")
	if report.Command != "" || report.Status != "restart required" {
		t.Fatalf("report = %+v", report)
	}
}

func TestUpgradeOlderUnmanagedBinaryDoesNotChangePrivateRegistration(t *testing.T) {
	_, _, configPath, want, svc := setupUpgradeActivationTest(t, "1.2.3", "1.2.4", "1.2.3")
	setRuntimeContract(t, "1.2.3", 1)

	report, err := svc.Upgrade(context.Background(), cli.UpgradeOptions{Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	assertStage(t, report, "activation", "failed")
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("client registration changed without target release bytes")
	}
}

func TestUpgradeCorpusSchemaMigrationRequired(t *testing.T) {
	home := t.TempDir()
	dbPath := filepath.Join(home, "gitcontribute.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE goose_db_version (id INTEGER PRIMARY KEY, version_id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO goose_db_version (id, version_id) VALUES (1, 0)"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	svc := testService(t, home, "1.2.3", dbPath)
	report, err := svc.Upgrade(context.Background(), cli.UpgradeOptions{Check: true})
	if err != nil {
		t.Fatal(err)
	}

	if report.Status != "schema migration required" {
		t.Fatalf("status = %q", report.Status)
	}
	assertStage(t, report, "corpus-schema", "migration_required")
	if stageStatus(report, "activation") != "migrate_first" {
		t.Fatalf("activation = %q", stageStatus(report, "activation"))
	}
	if !strings.Contains(report.Rollback, "corpus") {
		t.Fatalf("rollback = %q", report.Rollback)
	}
}

func TestUpgradeCorpusSchemaNewer(t *testing.T) {
	home := t.TempDir()
	dbPath := filepath.Join(home, "gitcontribute.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE goose_db_version (id INTEGER PRIMARY KEY, version_id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO goose_db_version (id, version_id) VALUES (1, 9999)"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	svc := testService(t, home, "1.2.3", dbPath)
	report, err := svc.Upgrade(context.Background(), cli.UpgradeOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if report.Status != "corpus newer than binary" {
		t.Fatalf("status = %q", report.Status)
	}
	assertStage(t, report, "corpus-schema", "newer")
	assertStage(t, report, "activation", "rollback_or_upgrade")
}

func TestUpgradeUsesTargetContractToRecoverNewerCorpus(t *testing.T) {
	for _, tt := range []struct {
		name         string
		targetSchema int64
		wantStatus   string
		wantStage    string
	}{
		{name: "matching target", targetSchema: 9999, wantStatus: "updated", wantStage: "current"},
		{name: "mismatched target", targetSchema: 1, wantStatus: "corpus newer than binary", wantStage: "newer"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			originalCmd := upgradeCommand
			originalExec := osExecutable
			originalContract := runtimeContractCommand
			originalGOOS := upgradeGOOS
			t.Cleanup(func() {
				upgradeCommand = originalCmd
				osExecutable = originalExec
				runtimeContractCommand = originalContract
				upgradeGOOS = originalGOOS
			})
			upgradeGOOS = "linux"
			t.Setenv("npm_command", "")
			t.Setenv("npm_lifecycle_event", "")

			home := t.TempDir()
			dbPath := filepath.Join(home, "gitcontribute.db")
			db, err := sql.Open("sqlite", dbPath)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("CREATE TABLE goose_db_version (id INTEGER PRIMARY KEY, version_id INTEGER)"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("INSERT INTO goose_db_version (id, version_id) VALUES (1, 9999)"); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}

			globalRoot := filepath.Join(home, "global", "lib", "node_modules")
			packageRoot := filepath.Join(globalRoot, "gitcontribute")
			executable := filepath.Join(packageRoot, "npm", "bin", "native", "linux-x64", "gitcontribute")
			if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(executable, []byte("release-1.2.3"), 0o755); err != nil {
				t.Fatal(err)
			}
			writePackageJSON(t, packageRoot, "1.2.3")
			osExecutable = func() (string, error) { return executable, nil }
			upgradeCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
				switch {
				case name == "npm" && len(args) >= 2 && args[0] == "root" && args[1] == "--global":
					return []byte(globalRoot + "\n"), nil
				case name == "npm" && len(args) >= 2 && args[0] == "view" && args[1] == "gitcontribute":
					return []byte("1.2.4\n"), nil
				case name == "npm" && len(args) >= 2 && args[0] == "install" && args[1] == "--global":
					writePackageJSON(t, packageRoot, "1.2.4")
					return nil, nil
				default:
					t.Fatalf("unexpected command: %s %v", name, args)
					return nil, nil
				}
			}
			runtimeContractCommand = func(_ context.Context, _ string) ([]byte, error) {
				return []byte(fmt.Sprintf(`{"name":"gitcontribute","version":"1.2.4","supported_schema_version":%d}`, tt.targetSchema)), nil
			}

			svc := testService(t, home, "1.2.3", dbPath)
			report, err := svc.Upgrade(context.Background(), cli.UpgradeOptions{Yes: true})
			if err != nil {
				t.Fatal(err)
			}
			if report.Status != tt.wantStatus {
				t.Fatalf("status = %q, want %q", report.Status, tt.wantStatus)
			}
			assertStage(t, report, "corpus-schema", tt.wantStage)
			if clients := outdatedPrivateRuntimeClients(report); len(clients) != 0 {
				t.Fatalf("outdated clients = %v, want none", clients)
			}
		})
	}
}

func setRuntimeContract(t *testing.T, version string, supportedSchema int64) {
	t.Helper()
	original := runtimeContractCommand
	t.Cleanup(func() { runtimeContractCommand = original })
	runtimeContractCommand = func(_ context.Context, _ string) ([]byte, error) {
		return []byte(fmt.Sprintf(`{"name":"gitcontribute","version":%q,"supported_schema_version":%d,"schema_version":0}`, version, supportedSchema)), nil
	}
}

func setRuntimeContractOutput(t *testing.T, output string) {
	t.Helper()
	original := runtimeContractCommand
	t.Cleanup(func() { runtimeContractCommand = original })
	runtimeContractCommand = func(_ context.Context, _ string) ([]byte, error) {
		return []byte(output), nil
	}
}

func setupUpgradeActivationTest(t *testing.T, currentVersion, targetVersion, candidateVersion string) (home, source, configPath string, want []byte, svc *Service) {
	t.Helper()
	originalCmd := upgradeCommand
	originalExec := osExecutable
	t.Cleanup(func() {
		upgradeCommand = originalCmd
		osExecutable = originalExec
	})

	home = t.TempDir()
	if candidateVersion != "" {
		source = filepath.Join(home, "release", "gitcontribute")
		if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(source, []byte("release-"+candidateVersion), 0o755); err != nil {
			t.Fatal(err)
		}
		osExecutable = func() (string, error) { return source, nil }
	} else {
		osExecutable = func() (string, error) { return "", errors.New("no executable") }
	}
	upgradeCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "npm" && len(args) >= 2 && args[0] == "view" && args[1] == "gitcontribute" {
			return []byte(targetVersion + "\n"), nil
		}
		t.Fatalf("unexpected command: %s %v", name, args)
		return nil, nil
	}

	oldRuntime := filepath.Join(home, ".local", "share", "gitcontribute", "bin", currentVersion, "gitcontribute")
	writeCodexConfig(t, home, oldRuntime)
	configPath = filepath.Join(home, ".codex", "config.toml")
	want, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	svc = testService(t, home, currentVersion, "")
	return
}

func testService(t *testing.T, home, version, database string) *Service {
	t.Helper()
	paths := config.NewPaths(&config.Env{Home: home})
	cfg := config.Default()
	cfg.Database = database
	return &Service{version: version, paths: paths, cfg: cfg}
}

func writePackageJSON(t *testing.T, root, version string) {
	t.Helper()
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	data := fmt.Sprintf(`{"name":"gitcontribute","version":"%s"}`, version)
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
}

func writeCodexConfig(t *testing.T, home, command string) {
	t.Helper()
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf("[mcp_servers.gitcontribute]\ncommand = %q\nargs = [\"mcp\", \"serve\", \"--transport=stdio\"]\n", command)
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func assertStage(t *testing.T, report *cli.UpgradeReport, name, wantStatus string) {
	t.Helper()
	stage := stageByName(report, name)
	if stage.Name == "" {
		t.Fatalf("stage %q not found in report", name)
	}
	if stage.Status != wantStatus {
		t.Fatalf("stage %q status = %q, want %q", name, stage.Status, wantStatus)
	}
}

func stageByName(report *cli.UpgradeReport, name string) cli.UpgradeStage {
	for _, s := range report.Stages {
		if s.Name == name {
			return s
		}
	}
	return cli.UpgradeStage{}
}

func joinCalls(calls [][]string) string {
	parts := make([]string, 0, len(calls))
	for _, c := range calls {
		parts = append(parts, strings.Join(c, " "))
	}
	return strings.Join(parts, "; ")
}
