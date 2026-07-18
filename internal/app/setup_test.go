package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/config"
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
	report, err := svc.Setup(context.Background(), cli.SetupOptions{
		Clients: []string{"codex", "claude"}, TokenSource: "none", Repository: "morluto/gitcontribute", Executable: filepath.Join(home, "bin", "gitcontribute"),
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
	second, err := svc.Setup(context.Background(), cli.SetupOptions{Clients: []string{"codex", "claude"}, TokenSource: "none", Executable: filepath.Join(home, "bin", "gitcontribute")})
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range second.Steps {
		if (step.Name == "codex" || step.Name == "claude") && step.Status != "already configured" {
			t.Fatalf("step = %+v", step)
		}
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
	report, err := svc.Setup(context.Background(), cli.SetupOptions{Clients: []string{"codex"}, TokenSource: "none", DryRun: true, Executable: "/bin/gitcontribute"})
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

func TestSetupTerminalOnlyDryRunNeedsNoDetectedClientOrNPMProcess(t *testing.T) {
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
		InstallCLI:  true,
		SkipMCP:     true,
		TokenSource: "none",
		DryRun:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Launcher != "" {
		t.Fatalf("launcher = %q", report.Launcher)
	}
	foundTerminal := false
	for _, step := range report.Steps {
		if step.Name == "terminal" {
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
	_, err = svc.Setup(context.Background(), cli.SetupOptions{Clients: []string{"codex"}, TokenSource: "none", Repository: "not a repository", Executable: "/bin/gitcontribute"})
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
		Clients: []string{"codex"}, TokenSource: "none", Executable: filepath.Join(home, "bin", "gitcontribute"),
	}, observer)
	if err != nil {
		t.Fatal(err)
	}
	if report.HasFailures() {
		t.Fatalf("report = %+v", report)
	}
	for _, want := range []cli.SetupPhase{cli.SetupPhaseConfiguration, cli.SetupPhaseCorpus, cli.SetupPhaseClients, cli.SetupPhaseVerification} {
		if !slices.Contains(observer.started, want) {
			t.Fatalf("started = %v, missing %q", observer.started, want)
		}
	}
	for _, want := range []string{"configuration", "corpus", "codex", "verification"} {
		found := false
		for _, step := range observer.completed {
			found = found || step.Name == want
		}
		if !found {
			t.Fatalf("completed = %+v, missing %q", observer.completed, want)
		}
	}
}
