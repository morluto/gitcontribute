package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

// ResolveLauncher returns a durable absolute MCP command. The setup application
// must first install an ephemeral package-runner executable into a stable
// product-owned location and pass that path explicitly.
func ResolveLauncher(opts Options) (Launcher, error) {
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
	return CanonicalLauncher(executable)
}

// CanonicalLauncher returns the current MCP launcher for an already resolved
// durable executable path.
func CanonicalLauncher(executable string) (Launcher, error) {
	launcher := Launcher{Command: executable, Args: canonicalMCPArgs()}
	if err := ValidateLauncher(launcher); err != nil {
		return Launcher{}, err
	}
	return launcher, nil
}

func canonicalMCPArgs() []string {
	return []string{"mcp", "serve", "--transport=stdio"}
}

// ResolveNPMVersion returns a registry-safe package version for CLI installation
// and private runtime directory names. It removes one release-tag "v" prefix
// and maps empty or development versions to the explicit "latest" tag.
func ResolveNPMVersion(version string) (string, error) {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	if version == "" || version == "dev" {
		version = "latest"
	}
	if !npmVersion.MatchString(version) {
		return "", fmt.Errorf("invalid npm version %q", version)
	}
	return version, nil
}

func configureClient(operation Operation, client Client, home string, launcher Launcher, dryRun bool) Result {
	adapter, err := clientAdapterFor(client)
	if err != nil {
		return Result{Client: client, Status: "failed", Error: err.Error()}
	}
	path := adapter.path(home)
	status, err := adapter.configure(path, operation, launcher, dryRun)
	result := Result{Client: client, Path: path, Status: status}
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
	}
	return result
}

// CodexSkillPath returns the managed discovery skill path for a home directory.
func CodexSkillPath(home string) string {
	return filepath.Join(home, ".codex", "skills", codexSkillDir, "SKILL.md")
}

// CodexSkillInstalled reports whether the managed discovery skill is current.
func CodexSkillInstalled(home string) (bool, string, error) {
	path := CodexSkillPath(home)
	state, err := inspectCodexSkill(path)
	return state == codexSkillCurrent, path, err
}

func configureCodexSkill(home string, operation Operation, dryRun bool) CodexSkillResult {
	path := CodexSkillPath(home)
	state, err := inspectCodexSkill(path)
	if err != nil {
		return CodexSkillResult{Path: path, Status: "failed", Error: err.Error()}
	}
	if operation == Remove {
		if state == codexSkillAbsent || state == codexSkillUnmanaged {
			return CodexSkillResult{Path: path, Status: "not configured"}
		}
		if dryRun {
			return CodexSkillResult{Path: path, Status: "would remove"}
		}
		if err := os.Remove(path); err != nil {
			return CodexSkillResult{Path: path, Status: "failed", Error: err.Error()}
		}
		if err := os.Remove(filepath.Dir(path)); err != nil && !errors.Is(err, os.ErrNotExist) {
			entries, readErr := os.ReadDir(filepath.Dir(path))
			if readErr != nil || len(entries) == 0 {
				return CodexSkillResult{Path: path, Status: "failed", Error: err.Error()}
			}
		}
		return CodexSkillResult{Path: path, Status: "removed"}
	}
	if state == codexSkillCurrent {
		return CodexSkillResult{Path: path, Status: "already configured"}
	}
	if state == codexSkillUnmanaged {
		return CodexSkillResult{Path: path, Status: "failed", Error: "discovery skill path exists but is not managed by GitContribute"}
	}
	if state == codexSkillManagedStale {
		if dryRun {
			return CodexSkillResult{Path: path, Status: "would update"}
		}
		if err := writeAtomic(path, codexSkillContent); err != nil {
			return CodexSkillResult{Path: path, Status: "failed", Error: err.Error()}
		}
		return CodexSkillResult{Path: path, Status: "updated"}
	}
	if dryRun {
		return CodexSkillResult{Path: path, Status: "would configure"}
	}
	if err := writeAtomic(path, codexSkillContent); err != nil {
		return CodexSkillResult{Path: path, Status: "failed", Error: err.Error()}
	}
	return CodexSkillResult{Path: path, Status: "configured"}
}

func inspectCodexSkill(path string) (codexSkillState, error) {
	// #nosec G304 -- path is the fixed managed skill path derived from the selected home.
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return codexSkillAbsent, nil
	}
	if err != nil {
		return "", err
	}
	if bytes.Equal(content, codexSkillContent) {
		return codexSkillCurrent, nil
	}
	if bytes.Contains(content, []byte(codexSkillOwnershipMarker)) {
		return codexSkillManagedStale, nil
	}
	return codexSkillUnmanaged, nil
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
	var parser unstable.Parser
	parser.Reset([]byte(text))
	start := -1
	for parser.NextExpression() {
		expression := parser.Expression()
		if expression.Kind != unstable.Table && expression.Kind != unstable.ArrayTable {
			continue
		}
		offset := tableHeaderOffset([]byte(text), expression)
		if offset < 0 {
			return 0, 0, false
		}
		if start >= 0 {
			return start, offset, true
		}
		if expression.Kind == unstable.Table && isCodexServerTable(expression) {
			start = offset
		}
	}
	if parser.Error() != nil || start < 0 {
		return 0, 0, false
	}
	return start, len(text), true
}

func tableHeaderOffset(document []byte, table *unstable.Node) int {
	key := table.Key()
	if !key.Next() {
		return -1
	}
	keyOffset := int(key.Node().Raw.Offset)
	lineStart := bytes.LastIndexByte(document[:keyOffset], '\n') + 1
	bracketOffset := bytes.IndexByte(document[lineStart:keyOffset], '[')
	if bracketOffset < 0 {
		return -1
	}
	return lineStart
}

func isCodexServerTable(table *unstable.Node) bool {
	key := table.Key()
	if !key.Next() || string(key.Node().Data) != "mcp_servers" {
		return false
	}
	if !key.Next() || string(key.Node().Data) != serverName {
		return false
	}
	return !key.Next()
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
