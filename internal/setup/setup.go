// Package setup owns local coding-client detection and MCP registration.
package setup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gofrs/flock"
	"github.com/pelletier/go-toml/v2"
)

const serverName = "gitcontribute"

const codexSkillDir = "gitcontribute"

const codexSkillOwnershipMarker = "<!-- Managed by gitcontribute setup. Manual edits may be replaced. -->"

var codexSkillContent = []byte(`---
name: gitcontribute
description: >
  Use for source-backed GitHub contribution workflows: repository and code research, issue triage, pull request review, contributor portfolio analysis, contribution preparation, duplicate and competing-work detection, investigations, workspaces, and validation evidence. Do not use for simple one-off GitHub lookups, ordinary local git commands, or GitHub mutations.
---

<!-- Managed by gitcontribute setup. Manual edits may be replaced. -->

When the user's request matches the description above, prefer the GitContribute MCP server. Discover its tools (names prefixed with mcp__gitcontribute__) and choose the narrowest tool for the task, such as portfolio research, repository search, investigation management, or workspace creation. Let the tool schemas and contracts guide arguments; do not invent unsupported fields. If no GitContribute tool fits, fall back to ordinary tools.
`)

type codexSkillState string

const (
	codexSkillAbsent       codexSkillState = "absent"
	codexSkillCurrent      codexSkillState = "current"
	codexSkillManagedStale codexSkillState = "managed_stale"
	codexSkillUnmanaged    codexSkillState = "unmanaged"
)

// Operation identifies whether client-owned MCP entries are configured or
// removed.
type Operation string

const (
	Configure Operation = "setup"
	Remove    Operation = "remove"
)

// Client identifies a supported coding-agent configuration adapter.
type Client string

const (
	Codex  Client = "codex"
	Claude Client = "claude"
)

// AllClients lists supported adapters in deterministic application order.
var AllClients = []Client{Codex, Claude}

// Launcher is the exact process command stored in a coding client's MCP
// configuration.
type Launcher struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// Options controls coding-client MCP registration.
type Options struct {
	Operation  Operation
	Clients    []Client
	All        bool
	DryRun     bool
	Home       string
	Executable string
}

// Result describes the registration effect for one coding client.
type Result struct {
	Client Client `json:"client"`
	Path   string `json:"path"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// CodexSkillResult reports the managed discovery-skill effect.
type CodexSkillResult struct {
	Path   string `json:"path,omitempty"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type Report struct {
	Operation  Operation        `json:"operation"`
	DryRun     bool             `json:"dry_run"`
	Launcher   Launcher         `json:"launcher"`
	Results    []Result         `json:"results"`
	CodexSkill CodexSkillResult `json:"codex_skill,omitempty"`
}

// Detect returns supported coding clients whose configuration directories are
// present under home. Detection performs no writes.
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
		rawServers, present := root["mcpServers"]
		if !present {
			return false, path, nil
		}
		servers, ok := rawServers.(map[string]any)
		if !ok {
			return false, path, errors.New("mcpServers must be an object in claude config")
		}
		rawServer, present := servers[serverName]
		if !present {
			return false, path, nil
		}
		if _, ok := rawServer.(map[string]any); !ok {
			return false, path, errors.New("gitcontribute server must be an object in claude config")
		}
		return true, path, nil
	default:
		return false, "", fmt.Errorf("unsupported setup client %q", client)
	}
}

// Run validates every selected client, resolves one shared launcher, and then
// applies or plans each client-owned registration. Dry-run mode performs no
// writes. Per-client parse/write failures are returned in Report.Results so
// independent clients can be reported together.
func Run(opts Options) (_ Report, returnErr error) {
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
	if !opts.DryRun {
		lease, err := acquireSetupLease(opts.Home)
		if err != nil {
			return Report{}, err
		}
		defer func() { returnErr = errors.Join(returnErr, lease.Unlock()) }()
	}
	report := Report{Operation: opts.Operation, DryRun: opts.DryRun, Launcher: launcher}
	for _, client := range clients {
		result := configureClient(opts.Operation, client, opts.Home, launcher, opts.DryRun)
		report.Results = append(report.Results, result)
	}
	if containsClient(clients, Codex) {
		report.CodexSkill = configureCodexSkill(opts.Home, opts.Operation, opts.DryRun)
	}
	return report, nil
}

// ActivateExisting updates a set of existing GitContribute registrations as
// one rollback-safe operation. It never creates a new client registration or
// changes the optional Codex discovery skill. If activation or verification is
// interrupted, every selected client configuration is restored.
func ActivateExisting(ctx context.Context, opts Options) (Report, error) {
	return ActivateExistingAndVerify(ctx, opts, nil)
}

// ActivateExistingAndVerify keeps the registration snapshots until verify
// succeeds, allowing callers to include executable and schema checks in the
// same rollback boundary.
func ActivateExistingAndVerify(ctx context.Context, opts Options, verify func() error) (_ Report, returnErr error) {
	if opts.Home == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Report{}, fmt.Errorf("resolve home directory: %w", err)
		}
		opts.Home = home
	}
	lease, err := acquireSetupLease(opts.Home)
	if err != nil {
		return Report{}, err
	}
	defer func() { returnErr = errors.Join(returnErr, lease.Unlock()) }()
	return activateExisting(ctx, opts, func(ctx context.Context, _ int) error { return ctx.Err() }, verify)
}

func acquireSetupLease(home string) (*flock.Flock, error) {
	lease := flock.New(filepath.Join(home, ".gitcontribute-setup.lock"))
	acquired, err := lease.TryLock()
	if err != nil {
		return nil, fmt.Errorf("acquire setup lock: %w", err)
	}
	if !acquired {
		return nil, errors.New("another GitContribute setup or activation is in progress")
	}
	return lease, nil
}

type registrationSnapshot struct {
	client      Client
	path        string
	mode        os.FileMode
	codexBlock  string
	claudeEntry any
	changed     bool
}

func activateExisting(ctx context.Context, opts Options, checkpoint func(context.Context, int) error, verify func() error) (Report, error) {
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	opts.Operation = Configure
	opts.DryRun = false
	clients, err := selectedClients(opts)
	if err != nil {
		return Report{}, err
	}
	launcher, err := ResolveLauncher(opts)
	if err != nil {
		return Report{}, err
	}

	snapshots, err := snapshotRegistrations(clients, opts.Home)
	if err != nil {
		return Report{}, err
	}

	report := Report{Operation: Configure, Launcher: launcher}
	rollback := func(cause error) (Report, error) {
		rollbackErr := restoreRegistrationSnapshots(snapshots, launcher)
		if rollbackErr != nil {
			return report, &ActivationRollbackError{Cause: cause, Rollback: rollbackErr}
		}
		return report, cause
	}

	if err := activateRegistrations(ctx, opts.Home, clients, launcher, checkpoint, &report, snapshots); err != nil {
		return rollback(err)
	}
	if err := verifyRegistrations(opts.Home, clients, launcher, verify); err != nil {
		return rollback(err)
	}
	return report, nil
}

func snapshotRegistrations(clients []Client, home string) ([]registrationSnapshot, error) {
	snapshots := make([]registrationSnapshot, 0, len(clients))
	for _, client := range clients {
		registered, path, err := CheckRegistration(client, home)
		if err != nil {
			return nil, fmt.Errorf("inspect %s registration: %w", client, err)
		}
		if !registered {
			return nil, fmt.Errorf("%s registration changed before activation", client)
		}
		data, err := readFileWithinParent(path)
		if err != nil {
			return nil, fmt.Errorf("snapshot %s registration: %w", client, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("inspect %s registration permissions: %w", client, err)
		}
		snapshot := registrationSnapshot{client: client, path: path, mode: info.Mode().Perm()}
		switch client {
		case Codex:
			start, end, present := findCodexBlock(string(data))
			if !present {
				return nil, errors.New("codex registration disappeared before activation")
			}
			snapshot.codexBlock = string(data[start:end])
		case Claude:
			var root map[string]any
			if err := json.Unmarshal(data, &root); err != nil {
				return nil, fmt.Errorf("parse Claude registration snapshot: %w", err)
			}
			servers, ok := root["mcpServers"].(map[string]any)
			if !ok {
				return nil, errors.New("claude mcpServers disappeared before activation")
			}
			snapshot.claudeEntry = servers[serverName]
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

func restoreRegistrationSnapshots(snapshots []registrationSnapshot, activated Launcher) error {
	var restoreErrs []error
	for i := len(snapshots) - 1; i >= 0; i-- {
		snapshot := snapshots[i]
		if !snapshot.changed {
			continue
		}
		currentInfo, err := os.Stat(snapshot.path)
		if err != nil {
			restoreErrs = append(restoreErrs, fmt.Errorf("inspect activated registration permissions %s: %w", snapshot.path, err))
			continue
		}
		if !registrationModeMatchesActivation(currentInfo.Mode()) {
			restoreErrs = append(restoreErrs, fmt.Errorf("preserve concurrently changed registration %s", snapshot.path))
			continue
		}
		var restoreErr error
		switch snapshot.client {
		case Codex:
			restoreErr = restoreCodexRegistration(snapshot, activated)
		case Claude:
			restoreErr = restoreClaudeRegistration(snapshot, activated)
		}
		if restoreErr != nil {
			restoreErrs = append(restoreErrs, fmt.Errorf("restore %s: %w", snapshot.path, restoreErr))
			continue
		}
		if err := restoreRegistrationMode(snapshot.path, snapshot.mode); err != nil {
			restoreErrs = append(restoreErrs, fmt.Errorf("restore permissions for %s: %w", snapshot.path, err))
		}
	}
	return errors.Join(restoreErrs...)
}

func restoreCodexRegistration(snapshot registrationSnapshot, activated Launcher) error {
	data, err := readFileWithinParent(snapshot.path)
	if err != nil {
		return err
	}
	text := string(data)
	start, end, present := findCodexBlock(text)
	if !present || strings.TrimSpace(text[start:end]) != strings.TrimSpace(codexTOMLBlock(activated)) {
		return errors.New("preserve concurrently changed GitContribute entry")
	}
	return writeAtomic(snapshot.path, []byte(text[:start]+snapshot.codexBlock+text[end:]))
}

func restoreClaudeRegistration(snapshot registrationSnapshot, activated Launcher) error {
	data, err := readFileWithinParent(snapshot.path)
	if err != nil {
		return err
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return err
	}
	servers, ok := root["mcpServers"].(map[string]any)
	if !ok || !equalJSON(servers[serverName], map[string]any{"command": activated.Command, "args": activated.Args}) {
		return errors.New("preserve concurrently changed GitContribute entry")
	}
	servers[serverName] = snapshot.claudeEntry
	root["mcpServers"] = servers
	return writeJSON(snapshot.path, root)
}

func activateRegistrations(ctx context.Context, home string, clients []Client, launcher Launcher, checkpoint func(context.Context, int) error, report *Report, snapshots []registrationSnapshot) error {
	for i, client := range clients {
		if err := ctx.Err(); err != nil {
			return err
		}
		result := configureClient(Configure, client, home, launcher, false)
		report.Results = append(report.Results, result)
		if result.Error != "" {
			return fmt.Errorf("activate %s registration: %s", client, result.Error)
		}
		snapshots[i].changed = true
		if err := checkpoint(ctx, i); err != nil {
			return err
		}
	}
	return nil
}

func verifyRegistrations(home string, clients []Client, launcher Launcher, verify func() error) error {
	for _, client := range clients {
		result := configureClient(Configure, client, home, launcher, true)
		if result.Error != "" || result.Status != "already configured" {
			return fmt.Errorf("verify %s registration: status %q: %s", client, result.Status, result.Error)
		}
	}
	if verify != nil {
		return verify()
	}
	return nil
}

func readFileWithinParent(path string) (_ []byte, err error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	file, err := root.Open(filepath.Base(path))
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	return io.ReadAll(file)
}

func containsClient(clients []Client, want Client) bool {
	for _, c := range clients {
		if c == want {
			return true
		}
	}
	return false
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
	return Launcher{Command: executable, Args: []string{"mcp", "serve", "--transport=stdio"}}, nil
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
