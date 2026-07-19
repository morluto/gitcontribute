package app

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/config"
	_ "modernc.org/sqlite"
)

func TestSetupInitializesAndRegistersWithoutNetwork(t *testing.T) {
	home := t.TempDir()
	paths := config.NewPaths(&config.Env{Home: home, Vars: map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "config"),
		"XDG_DATA_HOME": filepath.Join(home, "data"), "XDG_CACHE_HOME": filepath.Join(home, "cache"),
		"XDG_STATE_HOME": filepath.Join(home, "state"),
	}})
	svc, err := New(paths, "1.2.3", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	packagedExecutable := writeTestExecutable(t, filepath.Join(home, "bin"))
	report, err := svc.Setup(context.Background(), cli.SetupOptions{
		Mode: cli.SetupModeMCP, Clients: []string{"codex", "claude"}, TokenSource: "none", Repository: "morluto/gitcontribute", Executable: packagedExecutable,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.HasFailures() {
		t.Fatalf("report = %+v", report)
	}
	for _, path := range []string{filepath.Join(home, ".codex", "config.toml"), filepath.Join(home, ".claude.json")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected setup file %s: %v", path, err)
		}
	}
	sources, err := svc.ListSources(context.Background())
	if err != nil || len(sources.Sources) != 1 || sources.Sources[0].Name != "morluto-gitcontribute" {
		t.Fatalf("sources=%+v err=%v", sources, err)
	}
	second, err := svc.Setup(context.Background(), cli.SetupOptions{Mode: cli.SetupModeMCP, Clients: []string{"codex", "claude"}, TokenSource: "none", Executable: packagedExecutable})
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range second.Steps {
		if (step.Name == "codex" || step.Name == "claude") && step.Status != "already configured" {
			t.Fatalf("step = %+v", step)
		}
	}
}

func TestSetupRemoveStillUnregistersSelectedMCPClientsWithoutAnAccessMode(t *testing.T) {
	home := t.TempDir()
	paths := config.NewPaths(&config.Env{Home: home, Vars: map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "config"),
		"XDG_DATA_HOME": filepath.Join(home, "data"),
	}})
	svc, err := New(paths, "1.2.3", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	if _, err := svc.Setup(context.Background(), cli.SetupOptions{
		Mode: cli.SetupModeMCP, Clients: []string{"codex"}, TokenSource: "none",
		Executable: writeTestExecutable(t, filepath.Join(home, "bin")),
	}); err != nil {
		t.Fatal(err)
	}

	report, err := svc.Setup(context.Background(), cli.SetupOptions{Remove: true, Clients: []string{"codex"}})
	if err != nil {
		t.Fatal(err)
	}
	if report.HasFailures() {
		t.Fatalf("report = %+v", report)
	}
	configText, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(configText), "gitcontribute") {
		t.Fatalf("MCP registration remains after remove: %s", configText)
	}
}

func TestSetupMCPOnlyInstallsManagedBinaryAndRegistersItsAbsolutePath(t *testing.T) {
	home := t.TempDir()
	paths := config.NewPaths(&config.Env{Home: home, Vars: map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "config"),
		"XDG_DATA_HOME": filepath.Join(home, "data"),
	}})
	svc, err := New(paths, "1.2.3", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	source := filepath.Join(home, "npm-cache", "gitcontribute")
	if runtime.GOOS == "windows" {
		source += ".exe"
	}
	if err := os.MkdirAll(filepath.Dir(source), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("packaged-native-binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	report, err := svc.Setup(context.Background(), cli.SetupOptions{
		Mode: cli.SetupModeMCP, Clients: []string{"codex"}, TokenSource: "none", Executable: source,
	})
	if err != nil {
		t.Fatal(err)
	}
	dataDir, err := paths.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	binaryName := "gitcontribute"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	managed := filepath.Join(dataDir, "bin", "1.2.3", binaryName)
	if report.MCPCommand == nil || report.MCPCommand.Command != managed || !slices.Equal(report.MCPCommand.Args, []string{"mcp", "serve", "--transport=stdio"}) {
		t.Fatalf("MCP command = %+v", report.MCPCommand)
	}
	installed, err := os.ReadFile(managed)
	if err != nil {
		t.Fatalf("read managed binary: %v", err)
	}
	if string(installed) != "packaged-native-binary" {
		t.Fatalf("managed binary = %q", installed)
	}
	foundRuntime := false
	for _, step := range report.Steps {
		if step.Name == "mcp-runtime" {
			foundRuntime = step.Status == "installed" && step.Path == managed
		}
	}
	if !foundRuntime {
		t.Fatalf("report = %+v", report)
	}
}

func TestSetupMCPOnlyKeepsDevelopmentRuntimeDistinctFromLatestRelease(t *testing.T) {
	home := t.TempDir()
	paths := config.NewPaths(&config.Env{Home: home, Vars: map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "config"), "XDG_DATA_HOME": filepath.Join(home, "data"),
	}})
	svc, err := New(paths, "dev", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	report, err := svc.Setup(context.Background(), cli.SetupOptions{
		Mode: cli.SetupModeMCP, Clients: []string{"codex"}, TokenSource: "none", DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantSegment := string(filepath.Separator) + "bin" + string(filepath.Separator) + "dev" + string(filepath.Separator)
	if report.MCPCommand == nil || !strings.Contains(report.MCPCommand.Command, wantSegment) || strings.Contains(report.MCPCommand.Command, string(filepath.Separator)+"latest"+string(filepath.Separator)) {
		t.Fatalf("development MCP command = %+v", report.MCPCommand)
	}
}

func TestSetupMCPOnlyReusesMatchingManagedBinary(t *testing.T) {
	home := t.TempDir()
	paths := config.NewPaths(&config.Env{Home: home, Vars: map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "config"),
		"XDG_DATA_HOME": filepath.Join(home, "data"),
	}})
	svc, err := New(paths, "1.2.3", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	source := filepath.Join(home, "npm-cache", "gitcontribute")
	if runtime.GOOS == "windows" {
		source += ".exe"
	}
	if err := os.MkdirAll(filepath.Dir(source), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("packaged-native-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	opts := cli.SetupOptions{
		Mode: cli.SetupModeMCP, Clients: []string{"codex"}, TokenSource: "none", Executable: source,
	}
	if _, err := svc.Setup(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	dataDir, err := paths.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	binaryName := "gitcontribute"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	managed := filepath.Join(dataDir, "bin", "1.2.3", binaryName)
	stableTime := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(managed, stableTime, stableTime); err != nil {
		t.Fatal(err)
	}

	report, err := svc.Setup(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(managed)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(stableTime) {
		t.Fatalf("matching managed binary was rewritten at %s", info.ModTime())
	}
	for _, step := range report.Steps {
		if step.Name == "mcp-runtime" {
			if step.Status != "already installed" {
				t.Fatalf("runtime step = %+v", step)
			}
			return
		}
	}
	t.Fatalf("runtime step missing: %+v", report)
}

func TestSetupBothRegistersTheInstalledCLIWithoutASecondRuntime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test npm fixture uses a POSIX shell")
	}
	home := t.TempDir()
	paths := config.NewPaths(&config.Env{Home: home, Vars: map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "config"),
		"XDG_DATA_HOME": filepath.Join(home, "data"),
	}})
	svc, err := New(paths, "1.2.3", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	prefix := filepath.Join(home, "npm-global")
	installedCLI := filepath.Join(prefix, "bin", "gitcontribute")
	if err := os.MkdirAll(filepath.Dir(installedCLI), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installedCLI, []byte("installed-cli"), 0o755); err != nil {
		t.Fatal(err)
	}
	fixtureBin := filepath.Join(home, "fixture-bin")
	if err := os.MkdirAll(fixtureBin, 0o700); err != nil {
		t.Fatal(err)
	}
	npm := filepath.Join(fixtureBin, "npm")
	script := "#!/bin/sh\nif [ \"$1\" = install ]; then exit 0; fi\nif [ \"$1\" = prefix ]; then printf '%s\\n' \"$GITCONTRIBUTE_TEST_NPM_PREFIX\"; exit 0; fi\nexit 1\n"
	if err := os.WriteFile(npm, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fixtureBin)
	t.Setenv("GITCONTRIBUTE_TEST_NPM_PREFIX", prefix)

	report, err := svc.Setup(context.Background(), cli.SetupOptions{
		Mode: cli.SetupModeBoth, Clients: []string{"codex"}, TokenSource: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.MCPCommand == nil || report.MCPCommand.Command != installedCLI || !slices.Equal(report.MCPCommand.Args, []string{"mcp", "serve", "--transport=stdio"}) {
		t.Fatalf("MCP command = %+v", report.MCPCommand)
	}
	for _, step := range report.Steps {
		if step.Name == "mcp-runtime" {
			t.Fatalf("Both installed a second MCP runtime: %+v", step)
		}
	}
}

func TestSetupBothDryRunDoesNotPresentTheBootstrapExecutableAsFinalMCPCommand(t *testing.T) {
	home := t.TempDir()
	paths := config.NewPaths(&config.Env{Home: home, Vars: map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "config"),
		"XDG_DATA_HOME": filepath.Join(home, "data"),
	}})
	svc, err := New(paths, "1.2.3", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	t.Setenv("PATH", "")

	report, err := svc.Setup(context.Background(), cli.SetupOptions{
		Mode: cli.SetupModeBoth, Clients: []string{"codex"}, TokenSource: "none", DryRun: true,
		Executable: "/temporary/npm-cache/gitcontribute",
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.MCPCommand != nil || !report.MCPCommandPending {
		t.Fatalf("dry-run MCP command = %+v pending=%v", report.MCPCommand, report.MCPCommandPending)
	}
}

func TestSetupBothInstallationFailureLeavesNoPendingMCPCommandOrConfigWrites(t *testing.T) {
	home := t.TempDir()
	paths := config.NewPaths(&config.Env{Home: home, Vars: map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "config"), "XDG_DATA_HOME": filepath.Join(home, "data"),
	}})
	svc, err := New(paths, "1.2.3", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	t.Setenv("PATH", "")

	report, err := svc.Setup(context.Background(), cli.SetupOptions{
		Mode: cli.SetupModeBoth, Clients: []string{"codex"}, TokenSource: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.HasFailures() || report.MCPCommandPending || report.MCPCommand != nil {
		t.Fatalf("report = %+v", report)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("setup wrote agent configuration after CLI installation failure: %v", err)
	}
}

func TestSetupStopsBeforeConfigurationWhenManagedRuntimeCannotBeInstalled(t *testing.T) {
	home := t.TempDir()
	paths := config.NewPaths(&config.Env{Home: home, Vars: map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "config"),
		"XDG_DATA_HOME": filepath.Join(home, "data"),
	}})
	svc, err := New(paths, "1.2.3", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	report, err := svc.Setup(context.Background(), cli.SetupOptions{
		Mode: cli.SetupModeMCP, Clients: []string{"codex"}, TokenSource: "none",
		Executable: filepath.Join(home, "missing", "gitcontribute"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.HasFailures() {
		t.Fatalf("report = %+v", report)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("setup wrote agent configuration after runtime failure: %v", err)
	}
	configPath, err := paths.ConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("setup wrote application configuration after runtime failure: %v", err)
	}
}

func TestSetupDryRunWritesNothing(t *testing.T) {
	home := t.TempDir()
	paths := config.NewPaths(&config.Env{Home: home, Vars: map[string]string{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "config"), "XDG_DATA_HOME": filepath.Join(home, "data")}})
	svc, err := New(paths, "dev", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	report, err := svc.Setup(context.Background(), cli.SetupOptions{Mode: cli.SetupModeMCP, Clients: []string{"codex"}, TokenSource: "none", DryRun: true, Executable: "/bin/gitcontribute"})
	if err != nil {
		t.Fatal(err)
	}
	if !report.DryRun {
		t.Fatalf("report = %+v", report)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote client config: %v", err)
	}
}

func TestSetupDoesNotInferClientMutationFromDetection(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	paths := config.NewPaths(&config.Env{Home: home, Vars: map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "config"), "XDG_DATA_HOME": filepath.Join(home, "data"),
	}})
	svc, err := New(paths, "1.2.3", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	_, err = svc.Setup(context.Background(), cli.SetupOptions{Mode: cli.SetupModeMCP, TokenSource: "none", Executable: "/bin/gitcontribute"})
	if err == nil || !strings.Contains(err.Error(), "no coding-agent targets selected") {
		t.Fatalf("error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".codex", "config.toml")); !os.IsNotExist(statErr) {
		t.Fatalf("setup wrote detected client configuration: %v", statErr)
	}
}

func TestSetupVerificationDoesNotResolveCredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GITCONTRIBUTE_TEST_MISSING_TOKEN", "")
	paths := config.NewPaths(&config.Env{Home: home, Vars: map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "config"),
		"XDG_DATA_HOME": filepath.Join(home, "data"),
	}})
	svc, err := New(paths, "1.2.3", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	report, err := svc.Setup(context.Background(), cli.SetupOptions{
		Mode: cli.SetupModeMCP, Clients: []string{"codex"}, TokenSource: "env", TokenSourceKey: "GITCONTRIBUTE_TEST_MISSING_TOKEN",
		Executable: writeTestExecutable(t, filepath.Join(home, "bin")),
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Authentication == nil || report.Authentication.Method != "env" || report.Authentication.Key != "GITCONTRIBUTE_TEST_MISSING_TOKEN" {
		t.Fatalf("authentication report = %+v", report.Authentication)
	}
	for _, step := range report.Steps {
		if step.Name == "verification" {
			if step.Status != "verified" || strings.Contains(step.Message, "optional warning") {
				t.Fatalf("verification resolved the missing credential: %+v", step)
			}
			return
		}
	}
	t.Fatalf("verification step missing: %+v", report)
}

func TestSetupVerificationReportsFailedRequiredChecks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PATH", "")
	paths := config.NewPaths(&config.Env{Home: home, Vars: map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "config"),
		"XDG_DATA_HOME": filepath.Join(home, "data"),
	}})
	svc, err := New(paths, "1.2.3", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	report, err := svc.Setup(context.Background(), cli.SetupOptions{
		Mode: cli.SetupModeMCP, Clients: []string{"codex"}, TokenSource: "none",
		Executable: writeTestExecutable(t, filepath.Join(home, "bin")),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range report.Steps {
		if step.Name == "verification" {
			if step.Status != "failed" {
				t.Fatalf("verification step = %+v", step)
			}
			if !strings.HasPrefix(step.Message, "git: ") || strings.Contains(step.Message, "required installation checks failed") {
				t.Fatalf("verification message = %q", step.Message)
			}
			return
		}
	}
	t.Fatalf("verification step missing: %+v", report)
}

func TestSetupVerificationIgnoresConcurrentWriter(t *testing.T) {
	home := t.TempDir()
	paths := config.NewPaths(&config.Env{Home: home, Vars: map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "config"),
		"XDG_DATA_HOME": filepath.Join(home, "data"),
	}})
	executable := writeTestExecutable(t, filepath.Join(home, "bin"))
	setup := func() *cli.SetupReport {
		svc, err := New(paths, "1.2.3", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer svc.Close()
		report, err := svc.Setup(context.Background(), cli.SetupOptions{
			Mode: cli.SetupModeMCP, Clients: []string{"codex"}, TokenSource: "none", Executable: executable,
		})
		if err != nil {
			t.Fatal(err)
		}
		return report
	}
	if report := setup(); report.HasFailures() {
		t.Fatalf("initial setup failed: %+v", report)
	}
	database, err := paths.DatabasePath()
	if err != nil {
		t.Fatal(err)
	}
	locker, err := sql.Open("sqlite", database+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	defer locker.Close()
	conn, err := locker.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(context.Background(), `BEGIN IMMEDIATE`); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), `ROLLBACK`) }()

	report := setup()
	if report.HasFailures() {
		t.Fatalf("setup failed during normal write contention: %+v", report)
	}
	diagnosticService, err := New(paths, "1.2.3", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer diagnosticService.Close()
	diagnostics, err := diagnosticService.Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !diagnostics.Healthy {
		t.Fatalf("write contention made doctor unhealthy: %+v", diagnostics)
	}
	foundWriteWarning := false
	for _, check := range diagnostics.Checks {
		if check.Name == "database_write" {
			foundWriteWarning = check.Status == "warning" && !check.Required
		}
	}
	if !foundWriteWarning {
		t.Fatalf("database write warning missing: %+v", diagnostics)
	}
}

func TestSetupVerificationRejectsMismatchedClientRegistration(t *testing.T) {
	home := t.TempDir()
	paths := config.NewPaths(&config.Env{Home: home, Vars: map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "config"),
		"XDG_DATA_HOME": filepath.Join(home, "data"),
	}})
	svc, err := New(paths, "1.2.3", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	executable := writeTestExecutable(t, filepath.Join(home, "bin"))
	opts := cli.SetupOptions{Mode: cli.SetupModeMCP, Clients: []string{"codex"}, TokenSource: "none", Executable: executable}
	if report, err := svc.Setup(context.Background(), opts); err != nil || report.HasFailures() {
		t.Fatalf("setup = %+v, %v", report, err)
	}
	run, err := svc.newSetupRun(context.Background(), opts, nil)
	if err != nil {
		t.Fatal(err)
	}
	run.installedExecutable = run.managedRuntime
	configPath := filepath.Join(home, ".codex", "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	wantCommand := strconv.Quote(run.clientOptions.Executable)
	wrongCommand := strconv.Quote(filepath.Join(home, "wrong", "gitcontribute"))
	data = []byte(strings.Replace(string(data), wantCommand, wrongCommand, 1))
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	err = run.verifyAppliedSetup()
	if err == nil || !strings.Contains(err.Error(), "codex: registration does not match") {
		t.Fatalf("verifyAppliedSetup error = %v", err)
	}
}

func TestVerifySetupExecutableRejectsMissingAndNonExecutableCommands(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if err := verifySetupExecutable(missing); err == nil || !strings.Contains(err.Error(), "inspect installed command") {
		t.Fatalf("missing command error = %v", err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	nonExecutable := filepath.Join(t.TempDir(), "gitcontribute")
	if err := os.WriteFile(nonExecutable, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifySetupExecutable(nonExecutable); err == nil || !strings.Contains(err.Error(), "not executable") {
		t.Fatalf("non-executable command error = %v", err)
	}
}

func TestSetupReportsRestartOnlyWhenClientRegistrationChanges(t *testing.T) {
	home := t.TempDir()
	paths := config.NewPaths(&config.Env{Home: home, Vars: map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "config"),
		"XDG_DATA_HOME": filepath.Join(home, "data"),
	}})
	executable := writeTestExecutable(t, filepath.Join(home, "bin"))
	runSetup := func() *cli.SetupReport {
		svc, err := New(paths, "1.2.3", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer svc.Close()
		report, err := svc.Setup(context.Background(), cli.SetupOptions{
			Mode: cli.SetupModeMCP, Clients: []string{"codex"}, TokenSource: "none", Executable: executable,
		})
		if err != nil {
			t.Fatal(err)
		}
		return report
	}
	first := runSetup()
	if !slices.Equal(first.RestartClients, []string{"codex"}) {
		t.Fatalf("first restart clients = %v", first.RestartClients)
	}
	second := runSetup()
	if len(second.RestartClients) != 0 {
		t.Fatalf("idempotent setup restart clients = %v", second.RestartClients)
	}
}

func TestSetupCLIOnlyDryRunNeedsNoDetectedClientOrNPMProcess(t *testing.T) {
	home := t.TempDir()
	paths := config.NewPaths(&config.Env{Home: home, Vars: map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "config"),
		"XDG_DATA_HOME": filepath.Join(home, "data"),
	}})
	svc, err := New(paths, "1.2.3", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	t.Setenv("PATH", "")

	report, err := svc.Setup(context.Background(), cli.SetupOptions{
		Mode:        cli.SetupModeCLI,
		TokenSource: "none",
		DryRun:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.MCPCommand != nil {
		t.Fatalf("MCP command = %+v", report.MCPCommand)
	}
	foundTerminal := false
	for _, step := range report.Steps {
		if step.Name == "cli" {
			foundTerminal = step.Status == "would install" && strings.Contains(step.Message, "gitcontribute@1.2.3")
		}
	}
	if !foundTerminal {
		t.Fatalf("report = %+v", report)
	}
	if _, err := os.Stat(filepath.Join(home, "config")); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote configuration: %v", err)
	}
}

func TestSetupCLIRejectsMCPClientTargets(t *testing.T) {
	home := t.TempDir()
	paths := config.NewPaths(&config.Env{Home: home, Vars: map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "config"), "XDG_DATA_HOME": filepath.Join(home, "data"),
	}})
	svc, err := New(paths, "1.2.3", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	_, err = svc.Setup(context.Background(), cli.SetupOptions{
		Mode: cli.SetupModeCLI, Clients: []string{"codex"}, TokenSource: "none", DryRun: true,
	})
	if err == nil || !strings.Contains(err.Error(), "CLI mode cannot configure MCP clients") {
		t.Fatalf("error = %v", err)
	}
}

func TestSetupSourceNameIsValidAndBounded(t *testing.T) {
	short := setupSourceName(cli.RepoRef{Owner: "Morluto", Repo: "GitContribute"})
	if short != "morluto-gitcontribute" {
		t.Fatalf("short name = %q", short)
	}
	long := setupSourceName(cli.RepoRef{Owner: "owner", Repo: "this-is-a-very-long-repository-name-that-needs-a-stable-bounded-source-name-suffix"})
	if len(long) != 64 {
		t.Fatalf("long name length = %d: %q", len(long), long)
	}
}

func TestSetupRejectsRepositoryBeforeWriting(t *testing.T) {
	home := t.TempDir()
	paths := config.NewPaths(&config.Env{Home: home, Vars: map[string]string{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "config"), "XDG_DATA_HOME": filepath.Join(home, "data")}})
	svc, err := New(paths, "dev", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	_, err = svc.Setup(context.Background(), cli.SetupOptions{Mode: cli.SetupModeMCP, Clients: []string{"codex"}, TokenSource: "none", Repository: "not a repository", Executable: "/bin/gitcontribute"})
	if err == nil {
		t.Fatal("setup accepted invalid repository")
	}
	if _, statErr := os.Stat(filepath.Join(home, ".codex", "config.toml")); !os.IsNotExist(statErr) {
		t.Fatalf("setup wrote client config before validating repository: %v", statErr)
	}
}

func TestDiscoverSetupReportsDetectedClientsRegistrationAndPaths(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "config.toml"), []byte("[mcp_servers.gitcontribute]\ncommand = '/bin/gitcontribute'\nargs = ['mcp']\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	paths := config.NewPaths(&config.Env{Home: home, Vars: map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "config"),
		"XDG_DATA_HOME": filepath.Join(home, "data"), "GITHUB_TOKEN": "not-read-by-discovery",
	}})
	svc, err := New(paths, "1.2.3", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	discovery, err := svc.DiscoverSetup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !discovery.EnvironmentKeyPresent || len(discovery.Clients) != 2 {
		t.Fatalf("discovery = %+v", discovery)
	}
	if discovery.Clients[0].Name != "codex" || !discovery.Clients[0].Detected || !discovery.Clients[0].Registered || discovery.Clients[0].Path != filepath.Join(home, ".codex", "config.toml") {
		t.Fatalf("codex discovery = %+v", discovery.Clients[0])
	}
	if discovery.Clients[1].Name != "claude" || !discovery.Clients[1].Detected || discovery.Clients[1].Registered || discovery.Clients[1].Path != filepath.Join(home, ".claude.json") {
		t.Fatalf("claude discovery = %+v", discovery.Clients[1])
	}
}

type recordingSetupObserver struct {
	started   []cli.SetupPhase
	completed []cli.SetupStep
}

func (o *recordingSetupObserver) SetupStarted(phase cli.SetupPhase) {
	o.started = append(o.started, phase)
}

func (o *recordingSetupObserver) SetupCompleted(step cli.SetupStep) {
	o.completed = append(o.completed, step)
}

func TestSetupWithProgressReportsRealApplicationPhases(t *testing.T) {
	home := t.TempDir()
	paths := config.NewPaths(&config.Env{Home: home, Vars: map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "config"), "XDG_DATA_HOME": filepath.Join(home, "data"),
	}})
	svc, err := New(paths, "1.2.3", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	observer := &recordingSetupObserver{}
	report, err := svc.SetupWithProgress(context.Background(), cli.SetupOptions{
		Mode: cli.SetupModeMCP, Clients: []string{"codex"}, TokenSource: "none", Executable: writeTestExecutable(t, filepath.Join(home, "bin")),
	}, observer)
	if err != nil {
		t.Fatal(err)
	}
	if report.HasFailures() {
		t.Fatalf("report = %+v", report)
	}
	for _, want := range []cli.SetupPhase{cli.SetupPhaseMCPRuntime, cli.SetupPhaseConfiguration, cli.SetupPhaseCorpus, cli.SetupPhaseClients, cli.SetupPhaseVerification} {
		if !slices.Contains(observer.started, want) {
			t.Fatalf("started = %v, missing %q", observer.started, want)
		}
	}
	for _, want := range []string{"mcp-runtime", "configuration", "corpus", "codex", "verification"} {
		found := false
		for _, step := range observer.completed {
			found = found || step.Name == want
		}
		if !found {
			t.Fatalf("completed = %+v, missing %q", observer.completed, want)
		}
	}
}

func writeTestExecutable(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	name := "gitcontribute"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("test-packaged-executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
