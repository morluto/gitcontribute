package app

import (
	"context"
	"os"
	"path/filepath"
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
