package setup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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
	if !strings.Contains(string(codex), "model = \"test\"") ||
		strings.Count(string(codex), "[mcp_servers.gitcontribute]") != 1 ||
		!strings.Contains(string(codex), `args = ["mcp", "serve", "--transport=stdio"]`) {
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

func TestLauncherUsesTheExplicitManagedExecutable(t *testing.T) {
	managed := filepath.Join(t.TempDir(), "managed", "gitcontribute")
	report, err := Run(Options{Operation: Configure, Clients: []Client{Codex}, Home: t.TempDir(), Executable: managed, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.Launcher.Command != managed {
		t.Fatalf("launcher = %+v", report.Launcher)
	}
	wantArgs := []string{"mcp", "serve", "--transport=stdio"}
	if strings.Join(report.Launcher.Args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("launcher = %+v", report.Launcher)
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

func TestRunInstallsAndRemovesCodexSkillIdempotently(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("model = \"test\"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	report, err := Run(Options{Operation: Configure, Clients: []Client{Codex}, Home: home, Executable: "/bin/gitcontribute"})
	if err != nil {
		t.Fatal(err)
	}
	if report.CodexSkill.Status != "configured" {
		t.Fatalf("codex skill status = %q, want configured", report.CodexSkill.Status)
	}
	skillPath := CodexSkillPath(home)
	if _, err := os.Stat(skillPath); err != nil {
		t.Fatalf("skill missing: %v", err)
	}
	written, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(written, codexSkillContent) {
		t.Fatalf("skill content mismatch")
	}

	report, err = Run(Options{Operation: Configure, Clients: []Client{Codex}, Home: home, Executable: "/bin/gitcontribute"})
	if err != nil {
		t.Fatal(err)
	}
	if report.CodexSkill.Status != "already configured" {
		t.Fatalf("codex skill status = %q, want already configured", report.CodexSkill.Status)
	}

	report, err = Run(Options{Operation: Remove, Clients: []Client{Codex}, Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if report.CodexSkill.Status != "removed" {
		t.Fatalf("codex skill status = %q, want removed", report.CodexSkill.Status)
	}
	if _, err := os.Stat(skillPath); !os.IsNotExist(err) {
		t.Fatalf("skill still present after remove: %v", err)
	}

	report, err = Run(Options{Operation: Remove, Clients: []Client{Codex}, Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if report.CodexSkill.Status != "not configured" {
		t.Fatalf("codex skill status = %q, want not configured", report.CodexSkill.Status)
	}
}

func TestRunDoesNotInstallCodexSkillForClaudeOnly(t *testing.T) {
	home := t.TempDir()
	report, err := Run(Options{Operation: Configure, Clients: []Client{Claude}, Home: home, Executable: "/bin/gitcontribute"})
	if err != nil {
		t.Fatal(err)
	}
	if report.CodexSkill.Status != "" {
		t.Fatalf("unexpected codex skill result: %+v", report.CodexSkill)
	}
	if _, err := os.Stat(CodexSkillPath(home)); !os.IsNotExist(err) {
		t.Fatalf("codex skill installed for claude-only")
	}
}

func TestCodexSkillPreservesUnmanagedContent(t *testing.T) {
	home := t.TempDir()
	path := CodexSkillPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	want := []byte("---\nname: gitcontribute\ndescription: user-authored\n---\n")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}

	configured := configureCodexSkill(home, Configure, false)
	if configured.Status != "failed" {
		t.Fatalf("configure status = %q, want failed conflict", configured.Status)
	}
	removed := configureCodexSkill(home, Remove, false)
	if removed.Status != "not configured" {
		t.Fatalf("remove status = %q, want not configured", removed.Status)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("unmanaged skill changed: %q", got)
	}
}

func TestCodexSkillRemovalPreservesSiblingFiles(t *testing.T) {
	home := t.TempDir()
	if result := configureCodexSkill(home, Configure, false); result.Status != "configured" {
		t.Fatalf("configure = %+v", result)
	}
	sibling := filepath.Join(filepath.Dir(CodexSkillPath(home)), "notes.txt")
	if err := os.WriteFile(sibling, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if result := configureCodexSkill(home, Remove, false); result.Status != "removed" {
		t.Fatalf("remove = %+v", result)
	}
	if got, err := os.ReadFile(sibling); err != nil || string(got) != "keep" {
		t.Fatalf("sibling file = %q, %v", got, err)
	}
}
