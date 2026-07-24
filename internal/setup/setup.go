// Package setup owns local coding-client detection and MCP registration.
package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofrs/flock"
)

const serverName = "gitcontribute"

const codexSkillDir = "gitcontribute"

const codexSkillOwnershipMarker = "<!-- Managed by gitcontribute setup. Manual edits may be replaced. -->"

var codexSkillContent = []byte(`---
name: gitcontribute
description: >
  Use for source-backed GitHub contribution workflows: repository and code research, issue drafting and triage, pre-filing duplicate checks, pull request review, contributor portfolio analysis, contribution preparation, competing-work detection, investigations, workspaces, and validation evidence. Trigger on requests such as "check for a related issue before filing", "avoid duplicate reports", or "triage these findings". Do not use for simple one-off GitHub lookups, ordinary local git commands, or GitHub mutations.
---

<!-- Managed by gitcontribute setup. Manual edits may be replaced. -->

When the user's request matches the description above, prefer the GitContribute MCP server. Discover its tools (names prefixed with mcp__gitcontribute__) and choose the narrowest tool for the task. Let the tool schemas and contracts guide arguments; do not invent unsupported fields.

For issue drafting, triage, and duplicate checks: inspect corpus coverage and freshness; perform one bounded sync only when coverage is missing or stale; search stored threads offline; broaden strict multi-term searches when needed; hydrate only exact finalists; then verify current state with live GitHub before filing. Never treat zero matches as absence when coverage is incomplete or the query may be too strict.

Use native GitHub tools for final live-state verification and every mutation. If no GitContribute tool fits, fall back to ordinary tools.
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
