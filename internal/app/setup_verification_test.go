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

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/config"
	_ "modernc.org/sqlite"
)

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
