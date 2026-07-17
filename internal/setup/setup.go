// Package setup owns local coding-client detection and MCP registration.
package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const serverName = "gitcontribute"

type Operation string

const (
	Configure Operation = "setup"
	Remove    Operation = "remove"
)

type Client string

const (
	Codex  Client = "codex"
	Claude Client = "claude"
)

var AllClients = []Client{Codex, Claude}

type Launcher struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type Options struct {
	Operation  Operation
	Clients    []Client
	All        bool
	DryRun     bool
	Home       string
	Version    string
	Env        map[string]string
	Executable string
}

type Result struct {
	Client Client `json:"client"`
	Path   string `json:"path"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type Report struct {
	Operation Operation `json:"operation"`
	DryRun    bool      `json:"dry_run"`
	Launcher  Launcher  `json:"launcher"`
	Results   []Result  `json:"results"`
}

func Detect(home string) []Client {
	var out []Client
	if exists(filepath.Join(home, ".codex")) {
		out = append(out, Codex)
	}
	if exists(filepath.Join(home, ".claude")) || exists(filepath.Join(home, ".claude.json")) {
		out = append(out, Claude)
	}
	return out
}

// CheckRegistration reports whether the selected client has a GitContribute
// MCP entry without changing its configuration.
func CheckRegistration(client Client, home string) (bool, string, error) {
	switch client {
	case Codex:
		path := filepath.Join(home, ".codex", "config.toml")
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			return false, path, nil
		}
		if err != nil {
			return false, path, err
		}
		_, _, present := findCodexBlock(string(data))
		return present, path, nil
	case Claude:
		path := filepath.Join(home, ".claude.json")
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			return false, path, nil
		}
		if err != nil {
			return false, path, err
		}
		var root map[string]any
		if err := json.Unmarshal(data, &root); err != nil {
			return false, path, err
		}
		servers, _ := root["mcpServers"].(map[string]any)
		_, present := servers[serverName]
		return present, path, nil
	default:
		return false, "", fmt.Errorf("unsupported setup client %q", client)
	}
}

func Run(opts Options) (Report, error) {
	if opts.Operation == "" {
		opts.Operation = Configure
	}
	if opts.Operation != Configure && opts.Operation != Remove {
		return Report{}, fmt.Errorf("unsupported setup operation %q", opts.Operation)
	}
	if opts.Home == "" {
		var err error
		opts.Home, err = os.UserHomeDir()
		if err != nil {
			return Report{}, fmt.Errorf("resolve home directory: %w", err)
		}
	}
	clients, err := selectedClients(opts)
	if err != nil {
		return Report{}, err
	}
	launcher, err := ResolveLauncher(opts)
	if err != nil {
		return Report{}, err
	}
	report := Report{Operation: opts.Operation, DryRun: opts.DryRun, Launcher: launcher}
	for _, client := range clients {
		result := configureClient(opts.Operation, client, opts.Home, launcher, opts.DryRun)
		report.Results = append(report.Results, result)
	}
	return report, nil
}

func selectedClients(opts Options) ([]Client, error) {
	wanted := opts.Clients
	if opts.All {
		wanted = AllClients
	}
	seen := map[Client]bool{}
	for _, client := range wanted {
		if client != Codex && client != Claude {
			return nil, fmt.Errorf("unsupported setup client %q", client)
		}
		seen[client] = true
	}
	var out []Client
	for _, client := range AllClients {
		if seen[client] {
			out = append(out, client)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no setup clients selected")
	}
	return out, nil
}

func ResolveLauncher(opts Options) (Launcher, error) {
	getenv := func(key string) string {
		if opts.Env != nil {
			return opts.Env[key]
		}
		return os.Getenv(key)
	}
	if getenv("npm_execpath") != "" || getenv("npm_lifecycle_event") == "npx" || getenv("npm_command") == "exec" {
		version := strings.TrimPrefix(strings.TrimSpace(opts.Version), "v")
		if version == "" || version == "dev" {
			version = "latest"
		}
		if !npmVersion.MatchString(version) {
			return Launcher{}, fmt.Errorf("invalid MCP npm version %q", version)
		}
		return Launcher{Command: npmCommand(), Args: []string{"--yes", "--package=gitcontribute@" + version, "--", "gitcontribute", "mcp"}}, nil
	}
	executable := opts.Executable
	if executable == "" {
		var err error
		executable, err = os.Executable()
		if err != nil {
			return Launcher{}, fmt.Errorf("resolve executable: %w", err)
		}
	}
	executable, err := filepath.Abs(executable)
	if err != nil {
		return Launcher{}, fmt.Errorf("resolve executable path: %w", err)
	}
	return Launcher{Command: executable, Args: []string{"mcp"}}, nil
}

func npmCommand() string {
	if runtime.GOOS == "windows" {
		return "npx.cmd"
	}
	return "npx"
}

func configureClient(operation Operation, client Client, home string, launcher Launcher, dryRun bool) Result {
	var path string
	var status string
	var err error
	switch client {
	case Codex:
		path = filepath.Join(home, ".codex", "config.toml")
		status, err = editCodex(path, operation, launcher, dryRun)
	case Claude:
		path = filepath.Join(home, ".claude.json")
		status, err = editClaude(path, operation, launcher, dryRun)
	}
	result := Result{Client: client, Path: path, Status: status}
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
	}
	return result
}

func editClaude(path string, operation Operation, launcher Launcher, dryRun bool) (string, error) {
	root := map[string]any{}
	original, err := os.ReadFile(path)
	if err == nil && len(bytes.TrimSpace(original)) > 0 {
		if err := json.Unmarshal(original, &root); err != nil {
			return "", fmt.Errorf("parse %s: %w", path, err)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	servers, validServers := root["mcpServers"].(map[string]any)
	if _, exists := root["mcpServers"]; exists && !validServers {
		return "", fmt.Errorf("%s: mcpServers must be an object", path)
	}
	if servers == nil {
		servers = map[string]any{}
	}
	_, present := servers[serverName]
	if operation == Remove {
		if !present {
			return "not configured", nil
		}
		delete(servers, serverName)
		root["mcpServers"] = servers
		if dryRun {
			return "would remove", nil
		}
		return "removed", writeJSON(path, root)
	}
	want := map[string]any{"command": launcher.Command, "args": launcher.Args}
	if present && equalJSON(servers[serverName], want) {
		return "already configured", nil
	}
	servers[serverName] = want
	root["mcpServers"] = servers
	if dryRun {
		if present {
			return "would update", nil
		}
		return "would configure", nil
	}
	if err := writeJSON(path, root); err != nil {
		return "", err
	}
	if present {
		return "updated", nil
	}
	return "configured", nil
}

var tomlSection = regexp.MustCompile(`(?m)^\[[^\n]+\]\r?$`)
var npmVersion = regexp.MustCompile(`^(latest|[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?)$`)

func editCodex(path string, operation Operation, launcher Launcher, dryRun bool) (string, error) {
	original, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	text := string(original)
	if strings.TrimSpace(text) != "" {
		var parsed map[string]any
		if err := toml.Unmarshal(original, &parsed); err != nil {
			return "", fmt.Errorf("parse %s: %w", path, err)
		}
	}
	start, end, present := findCodexBlock(text)
	if operation == Remove {
		if !present {
			return "not configured", nil
		}
		if dryRun {
			return "would remove", nil
		}
		updated := strings.TrimSpace(text[:start] + text[end:])
		if updated != "" {
			updated += "\n"
		}
		return "removed", writeAtomic(path, []byte(updated))
	}
	block := codexTOMLBlock(launcher)
	if present && strings.TrimSpace(text[start:end]) == strings.TrimSpace(block) {
		return "already configured", nil
	}
	updated := text
	if present {
		updated = text[:start] + block + text[end:]
	} else {
		if updated != "" && !strings.HasSuffix(updated, "\n") {
			updated += "\n"
		}
		if strings.TrimSpace(updated) != "" {
			updated += "\n"
		}
		updated += block
	}
	if dryRun {
		if present {
			return "would update", nil
		}
		return "would configure", nil
	}
	if err := writeAtomic(path, []byte(updated)); err != nil {
		return "", err
	}
	if present {
		return "updated", nil
	}
	return "configured", nil
}

func findCodexBlock(text string) (int, int, bool) {
	header := "[mcp_servers.gitcontribute]"
	start := strings.Index(text, header)
	if start < 0 || (start > 0 && text[start-1] != '\n') {
		return 0, 0, false
	}
	rest := text[start+len(header):]
	locations := tomlSection.FindAllStringIndex(rest, -1)
	end := len(text)
	for _, location := range locations {
		candidate := start + len(header) + location[0]
		if candidate > start {
			end = candidate
			break
		}
	}
	return start, end, true
}

func codexTOMLBlock(launcher Launcher) string {
	args := make([]string, len(launcher.Args))
	for i, arg := range launcher.Args {
		args[i] = fmt.Sprintf("%q", arg)
	}
	return fmt.Sprintf("[mcp_servers.%s]\ncommand = %q\nargs = [%s]\n", serverName, launcher.Command, strings.Join(args, ", "))
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(data, '\n'))
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".gitcontribute-setup-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(name)
		}
	}()
	if err := tmp.Chmod(0600); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := replaceFile(name, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func equalJSON(a, b any) bool {
	aa, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return bytes.Equal(aa, bb)
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }
