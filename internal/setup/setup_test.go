package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestRunConfiguresAndRemovesClientsIdempotently(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "config.toml"), []byte("model = \"test\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte("{\"theme\":\"dark\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	opts := Options{Operation: Configure, All: true, Home: home, Executable: filepath.Join(home, "bin", "gitcontribute")}
	report, err := Run(opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range report.Results {
		if result.Status != "configured" {
			t.Fatalf("result = %+v", result)
		}
	}
	report, err = Run(opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range report.Results {
		if result.Status != "already configured" {
			t.Fatalf("result = %+v", result)
		}
	}
	codex, _ := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if !strings.Contains(string(codex), "model = \"test\"") || strings.Count(string(codex), "[mcp_servers.gitcontribute]") != 1 {
		t.Fatalf("codex config:\n%s", codex)
	}
	claude, _ := os.ReadFile(filepath.Join(home, ".claude.json"))
	var value map[string]any
	if err := json.Unmarshal(claude, &value); err != nil {
		t.Fatal(err)
	}
	if value["theme"] != "dark" {
		t.Fatalf("claude config lost unrelated value: %s", claude)
	}
	opts.Operation = Remove
	report, err = Run(opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range report.Results {
		if result.Status != "removed" {
			t.Fatalf("result = %+v", result)
		}
	}
	codex, _ = os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if !strings.Contains(string(codex), "model = \"test\"") || strings.Contains(string(codex), "gitcontribute") {
		t.Fatalf("codex removal:\n%s", codex)
	}
}

func TestDryRunDoesNotWrite(t *testing.T) {
	home := t.TempDir()
	report, err := Run(Options{Operation: Configure, Clients: []Client{Codex}, Home: home, Executable: "/bin/gitcontribute", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.Results[0].Status != "would configure" {
		t.Fatalf("report = %+v", report)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote config: %v", err)
	}
}

func TestNpxLauncherDoesNotCaptureCacheExecutable(t *testing.T) {
	report, err := Run(Options{Operation: Configure, Clients: []Client{Codex}, Home: t.TempDir(), Version: "v1.2.3", Env: map[string]string{"npm_command": "exec"}, Executable: "/tmp/npm-cache/gitcontribute", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.Launcher.Command != npmCommand() {
		t.Fatalf("launcher = %+v", report.Launcher)
	}
	wantArgs := []string{"--yes", "gitcontribute@1.2.3", "mcp"}
	if !slices.Equal(report.Launcher.Args, wantArgs) {
		t.Fatalf("launcher = %+v", report.Launcher)
	}
}

func TestNpxLauncherRejectsInvalidPackageVersion(t *testing.T) {
	_, err := Run(Options{Operation: Configure, Clients: []Client{Codex}, Home: t.TempDir(), Version: "../../local", Env: map[string]string{"npm_command": "exec"}, DryRun: true})
	if err == nil {
		t.Fatal("setup accepted an invalid npm version")
	}
}

func TestDetect(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, ".codex"), 0700); err != nil {
		t.Fatal(err)
	}
	got := Detect(home)
	if len(got) != 1 || got[0] != Codex {
		t.Fatalf("detected = %v", got)
	}
}

func TestMalformedClientConfigFailsWithoutOverwrite(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte("[broken\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	report, err := Run(Options{Operation: Configure, Clients: []Client{Codex}, Home: home, Executable: "/bin/gitcontribute"})
	if err != nil {
		t.Fatal(err)
	}
	if report.Results[0].Status != "failed" {
		t.Fatalf("report = %+v", report)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatalf("malformed config was overwritten: %q", after)
	}
}
