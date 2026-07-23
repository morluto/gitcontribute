package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/managedbinary"
	clientsetup "github.com/morluto/gitcontribute/internal/setup"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/mod/semver"
)

var (
	upgradeCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name != "npm" {
			return nil, fmt.Errorf("unsupported upgrade command %q", name)
		}
		return runNPMCommand(ctx, args)
	}
	runtimeContractCommand = func(ctx context.Context, path string) ([]byte, error) {
		return exec.CommandContext(ctx, path, "runtime-contract").CombinedOutput()
	}
	osExecutable = os.Executable
	upgradeGOOS  = runtime.GOOS
)

func runNPMCommand(ctx context.Context, args []string) ([]byte, error) {
	var command *exec.Cmd
	switch {
	case len(args) == 3 && args[0] == "view" && args[1] == "gitcontribute" && args[2] == "version":
		command = exec.CommandContext(ctx, "npm", "view", "gitcontribute", "version")
	case len(args) == 2 && args[0] == "root" && args[1] == "--global":
		command = exec.CommandContext(ctx, "npm", "root", "--global")
	case len(args) == 3 && args[0] == "install" && args[1] == "--global":
		version, err := clientsetup.ResolveNPMVersion(strings.TrimPrefix(args[2], "gitcontribute@"))
		if err != nil || args[2] != "gitcontribute@"+version {
			return nil, fmt.Errorf("unsupported npm install target %q", args[2])
		}
		command = exec.CommandContext(ctx, "npm")
		command.Args = []string{"npm", "install", "--global", "gitcontribute@" + version}
	default:
		return nil, fmt.Errorf("unsupported npm arguments %q", args)
	}
	return command.CombinedOutput()
}

// Upgrade checks npm for the latest release and updates persistent npm
// installations when explicitly authorized. It reports inspectable stages
// covering the npm launcher, private MCP runtime, configured client runtime,
// corpus schema compatibility, activation/restart action, and rollback
// limitations. It does not migrate the corpus or silently update an npx
// invocation.
func (s *Service) Upgrade(ctx context.Context, opts cli.UpgradeOptions) (*cli.UpgradeReport, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.paths == nil {
		s.paths = config.NewPaths(nil)
	}

	latest := ""
	if opts.Check || opts.Yes {
		var err error
		latest, err = latestNPMVersion(ctx)
		if err != nil {
			return nil, err
		}
	}

	current := normalizeVersion(s.version)
	details := discoverInstallation(ctx)

	report := &cli.UpgradeReport{
		Context: details.context,
		Current: current,
		Latest:  latest,
	}

	report.Stages = append(report.Stages, installationStage(details, current))
	report.Stages = append(report.Stages, npmLauncherStage(details, current, latest))
	report.Stages = append(report.Stages, s.privateRuntimeStage(current, latest))

	clients, cfgStage, err := s.configuredRuntimesStage(ctx, current, latest)
	if err != nil {
		return nil, err
	}
	report.ConfiguredClients = clients
	report.Stages = append(report.Stages, cfgStage)

	report.Stages = append(report.Stages, s.schemaStage(ctx))

	report.Stages = append(report.Stages, activationStage(report, opts))
	report.Stages = append(report.Stages, rollbackStage(report))

	setCommandAndStatus(report)
	recoveringNewerCorpus := stageStatus(report, "corpus-schema") == "newer"

	if shouldInstall(report, opts) {
		if err := runNPMInstall(ctx, latest); err != nil {
			return nil, err
		}
		if err := verifyGlobalNPMVersion(ctx, latest); err != nil {
			return nil, err
		}
		setStage(report, cli.UpgradeStage{
			Name:    "npm-launcher",
			Status:  "updated",
			Path:    stagePath(report, "npm-launcher"),
			Version: latest,
			Target:  latest,
			Message: "updated global npm package to " + latest,
		})
		report.Status = "updated"
		report.Command = ""
		if recoveringNewerCorpus && !s.validateNewerCorpusTarget(ctx, report, details.executable, latest) {
			return report, nil
		}
	}

	if opts.Yes && len(outdatedPrivateRuntimeClients(report)) > 0 {
		s.activatePrivateRuntime(ctx, report, details)
	}

	return report, nil
}

func verifyGlobalNPMVersion(ctx context.Context, want string) error {
	root, err := upgradeCommand(ctx, "npm", "root", "--global")
	if err != nil {
		return fmt.Errorf("verify global npm root: %w", err)
	}
	packageRoot := filepath.Join(strings.TrimSpace(string(root)), "gitcontribute")
	if got := readPackageVersion(packageRoot); got != want {
		return fmt.Errorf("verify installed npm release: got %q, want %q", got, want)
	}
	return nil
}

func latestNPMVersion(ctx context.Context) (string, error) {
	output, err := upgradeCommand(ctx, "npm", "view", "gitcontribute", "version")
	if err != nil {
		return "", fmt.Errorf("check latest npm release: %w", err)
	}
	version := normalizeVersion(string(output))
	resolved, err := clientsetup.ResolveNPMVersion(version)
	if err != nil {
		return "", fmt.Errorf("validate latest npm release: %w", err)
	}
	return resolved, nil
}

func runNPMInstall(ctx context.Context, version string) error {
	resolved, err := clientsetup.ResolveNPMVersion(version)
	if err != nil {
		return fmt.Errorf("validate latest npm release: %w", err)
	}
	if _, err := upgradeCommand(ctx, "npm", "install", "--global", "gitcontribute@"+resolved); err != nil {
		return fmt.Errorf("install latest npm release: %w", err)
	}
	return nil
}

func shouldInstall(report *cli.UpgradeReport, opts cli.UpgradeOptions) bool {
	if !opts.Yes {
		return false
	}
	if report.Context != "global-npm" {
		return false
	}
	if upgradeGOOS == "windows" {
		return false
	}
	if status := stageStatus(report, "corpus-schema"); status == "migration_required" || status == "failed" {
		return false
	}
	disposition := reportVersionDisposition(report)
	return disposition == versionUpgrade || disposition == versionPrerelease
}

func setCommandAndStatus(report *cli.UpgradeReport) {
	if stageStatus(report, "corpus-schema") == "migration_required" {
		report.Status = "schema migration required"
		return
	}
	if stageStatus(report, "corpus-schema") == "newer" {
		report.Status = "corpus newer than binary"
		return
	}

	switch report.Context {
	case "npx":
		report.Status = "npx"
	case "other":
		report.Status = "not managed"
	case "project-npm":
		switch reportVersionDisposition(report) {
		case versionUnavailable:
			report.Status = "awaiting confirmation"
		case versionCurrent:
			report.Status = "already current"
		case versionUpgrade:
			report.Status = "update available"
			report.Command = "npm install --save-dev gitcontribute@" + report.Latest
		case versionPrerelease:
			report.Status = "prerelease available"
			report.Command = "npm install --save-dev gitcontribute@" + report.Latest
		case versionDowngrade:
			report.Status = "newer version installed"
		case versionInvalid:
			report.Status = "version comparison unavailable"
		}
	case "global-npm":
		switch reportVersionDisposition(report) {
		case versionUnavailable:
			report.Status = "awaiting confirmation"
		case versionCurrent:
			report.Status = "already current"
		case versionUpgrade:
			report.Status = "update available"
			report.Command = "npm install --global gitcontribute@" + report.Latest
		case versionPrerelease:
			report.Status = "prerelease available"
			report.Command = "npm install --global gitcontribute@" + report.Latest
		case versionDowngrade:
			report.Status = "newer version installed"
		case versionInvalid:
			report.Status = "version comparison unavailable"
		}
	default:
		report.Status = "unknown"
	}
}

func normalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

type versionDisposition uint8

const (
	versionUnavailable versionDisposition = iota
	versionInvalid
	versionCurrent
	versionUpgrade
	versionPrerelease
	versionDowngrade
)

func compareVersions(current, target string) versionDisposition {
	current = normalizeVersion(current)
	target = normalizeVersion(target)
	if target == "" {
		return versionUnavailable
	}
	currentSemver := "v" + current
	targetSemver := "v" + target
	if !semver.IsValid(currentSemver) || !semver.IsValid(targetSemver) {
		return versionInvalid
	}
	switch semver.Compare(targetSemver, currentSemver) {
	case 0:
		return versionCurrent
	case -1:
		return versionDowngrade
	default:
		if semver.Prerelease(targetSemver) != "" {
			return versionPrerelease
		}
		return versionUpgrade
	}
}

func reportVersionDisposition(report *cli.UpgradeReport) versionDisposition {
	current := report.Current
	if report.Context == "global-npm" || report.Context == "project-npm" {
		if installed := stageVersion(report, "npm-launcher"); installed != "" {
			current = installed
		}
	}
	return compareVersions(current, report.Latest)
}

func installationStage(details installDetails, current string) cli.UpgradeStage {
	return cli.UpgradeStage{
		Name:    "installation",
		Status:  details.context,
		Path:    details.executable,
		Version: current,
		Message: installMessage(details.context),
	}
}

func installMessage(context string) string {
	switch context {
	case "npx":
		return "npx resolves versions on demand; no persistent installation to update"
	case "project-npm":
		return "project-local npm installation"
	case "global-npm":
		return "global npm installation"
	case "other":
		return "executable is not inside a managed npm package"
	default:
		return ""
	}
}

func discoverInstallation(ctx context.Context) installDetails {
	if os.Getenv("npm_command") == "exec" || os.Getenv("npm_lifecycle_event") == "npx" {
		return installDetails{context: "npx"}
	}
	executable, err := osExecutable()
	if err != nil {
		return installDetails{context: "other"}
	}
	normalized := filepath.ToSlash(executable)
	if !strings.Contains(normalized, "/node_modules/gitcontribute/") {
		return installDetails{context: "other", executable: executable}
	}
	globalRoot, err := upgradeCommand(ctx, "npm", "root", "--global")
	if err != nil {
		return installDetails{context: "project-npm", executable: executable}
	}
	root := strings.TrimSpace(string(globalRoot))
	return installDetails{
		context:    classifyNPMExecutable(executable, root),
		executable: executable,
		npmRoot:    root,
	}
}

func detectInstallContext(ctx context.Context) string {
	return discoverInstallation(ctx).context
}

func classifyNPMExecutable(executable, globalRoot string) string {
	executable = filepath.Clean(executable)
	globalPackage := filepath.Join(filepath.Clean(globalRoot), "gitcontribute")
	relative, err := filepath.Rel(globalPackage, executable)
	if err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "global-npm"
	}
	return "project-npm"
}

func npmPackageRoot(details installDetails) string {
	if details.context == "global-npm" && details.npmRoot != "" {
		return filepath.Join(filepath.Clean(details.npmRoot), "gitcontribute")
	}
	normalized := filepath.ToSlash(details.executable)
	const marker = "/node_modules/gitcontribute/"
	if idx := strings.Index(normalized, marker); idx >= 0 {
		return filepath.FromSlash(normalized[:idx+len(marker)-1])
	}
	return ""
}

func npmLauncherStage(details installDetails, _ string, latest string) cli.UpgradeStage {
	stage := cli.UpgradeStage{Name: "npm-launcher"}
	root := npmPackageRoot(details)
	if root == "" {
		stage.Status = details.context
		stage.Message = installMessage(details.context)
		return stage
	}
	stage.Path = root
	installed := readPackageVersion(root)
	stage.Version = installed
	if installed == "" {
		stage.Status = "unknown"
		stage.Message = "could not read installed npm package version"
		return stage
	}
	if latest == "" {
		stage.Status = "installed"
		stage.Message = fmt.Sprintf("npm package %s is installed", installed)
		return stage
	}
	stage.Target = latest
	switch compareVersions(installed, latest) {
	case versionCurrent:
		stage.Status = "current"
		stage.Message = fmt.Sprintf("npm package %s is the latest release", installed)
	case versionUpgrade:
		stage.Status = "update_available"
		stage.Message = fmt.Sprintf("npm package %s is installed; latest is %s", installed, latest)
	case versionPrerelease:
		stage.Status = "prerelease_available"
		stage.Message = fmt.Sprintf("npm package %s is installed; prerelease %s is available", installed, latest)
	case versionDowngrade:
		stage.Status = "newer_installed"
		stage.Message = fmt.Sprintf("npm package %s is newer than registry target %s; downgrade blocked", installed, latest)
	default:
		stage.Status = "unknown"
		stage.Message = fmt.Sprintf("cannot safely compare npm package %s with registry target %s", installed, latest)
	}
	return stage
}

func readPackageVersion(root string) string {
	data, err := readUpgradeFile(filepath.Join(root, "package.json"))
	if err != nil {
		return ""
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ""
	}
	return normalizeVersion(pkg.Version)
}

func (s *Service) privateRuntimeStage(current, latest string) cli.UpgradeStage {
	stage := cli.UpgradeStage{Name: "private-mcp-runtime"}
	dataDir, err := s.paths.DataDir()
	if err != nil {
		stage.Status = "failed"
		stage.Message = err.Error()
		return stage
	}
	path, err := managedbinary.Destination(dataDir, current)
	if err != nil {
		stage.Status = "failed"
		stage.Message = err.Error()
		return stage
	}
	stage.Path = path
	stage.Version = current
	_, statErr := os.Stat(path)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			stage.Status = "not_installed"
			stage.Message = "private MCP runtime is not installed"
		} else {
			stage.Status = "failed"
			stage.Message = statErr.Error()
		}
		return stage
	}
	stage.Status = "installed"
	switch compareVersions(current, latest) {
	case versionUpgrade:
		stage.Status = "update_available"
		stage.Target = latest
		stage.Message = fmt.Sprintf("installed runtime %s; latest release is %s", current, latest)
	case versionPrerelease:
		stage.Status = "prerelease_available"
		stage.Target = latest
		stage.Message = fmt.Sprintf("installed runtime %s; prerelease %s is available", current, latest)
	case versionDowngrade:
		stage.Status = "newer_installed"
		stage.Target = latest
		stage.Message = fmt.Sprintf("installed runtime %s is newer than registry target %s", current, latest)
	default:
		stage.Message = "installed runtime " + current
	}
	return stage
}

func (s *Service) configuredRuntimesStage(ctx context.Context, current, latest string) ([]cli.UpgradeConfiguredClient, cli.UpgradeStage, error) {
	stage := cli.UpgradeStage{Name: "configured-runtime"}
	if err := ctx.Err(); err != nil {
		return nil, stage, err
	}
	home := ""
	if s.paths != nil {
		home = s.paths.HomeDir()
	}
	if home == "" {
		stage.Status = "failed"
		stage.Message = "home directory is unavailable"
		return nil, stage, nil
	}
	var clients []cli.UpgradeConfiguredClient
	registered := 0
	outdated := 0
	for _, client := range clientsetup.AllClients {
		c, err := inspectConfiguredClient(home, client, current, latest)
		if err != nil {
			c = cli.UpgradeConfiguredClient{Name: string(client), Status: "failed", Message: err.Error()}
		}
		clients = append(clients, c)
		if c.Status != "not_configured" && c.Status != "failed" {
			registered++
		}
		if c.Status == "outdated" {
			outdated++
		}
	}
	switch {
	case registered == 0:
		stage.Status = "not_configured"
		stage.Message = "no coding clients are registered"
	case outdated > 0:
		stage.Status = "restart_required"
		stage.Message = fmt.Sprintf("%d registered client(s) reference an outdated runtime", outdated)
	default:
		stage.Status = "current"
		stage.Message = "all registered clients reference the current runtime"
	}
	return clients, stage, nil
}

func inspectConfiguredClient(home string, client clientsetup.Client, current, latest string) (cli.UpgradeConfiguredClient, error) {
	result := cli.UpgradeConfiguredClient{Name: string(client)}
	registered, path, err := clientsetup.CheckRegistration(client, home)
	if err != nil {
		return result, err
	}
	if !registered {
		result.Status = "not_configured"
		result.Path = path
		return result, nil
	}
	command, args, err := readClientCommand(client, home)
	if err != nil {
		return result, err
	}
	version := runtimeVersionFromPath(command)
	target := current
	if disposition := compareVersions(current, latest); disposition == versionUpgrade || disposition == versionPrerelease {
		target = latest
	}
	result.Path = command
	result.Version = version
	result.Message = strings.Join(args, " ")
	switch compareVersions(version, target) {
	case versionCurrent:
		result.Status = "target"
	case versionUpgrade, versionPrerelease:
		result.Status = "outdated"
	case versionDowngrade:
		result.Status = "newer"
	case versionUnavailable, versionInvalid:
		if compareVersions(version, current) == versionCurrent {
			result.Status = "current"
		} else {
			result.Status = "configured"
		}
	}
	return result, nil
}

func readClientCommand(client clientsetup.Client, home string) (string, []string, error) {
	switch client {
	case clientsetup.Codex:
		return readCodexCommand(filepath.Join(home, ".codex", "config.toml"))
	case clientsetup.Claude:
		return readClaudeCommand(filepath.Join(home, ".claude.json"))
	default:
		return "", nil, fmt.Errorf("unsupported client %q", client)
	}
}

func readCodexCommand(path string) (string, []string, error) {
	data, err := readUpgradeFile(path)
	if err != nil {
		return "", nil, err
	}
	var cfg struct {
		MCPServers map[string]struct {
			Command string   `toml:"command"`
			Args    []string `toml:"args"`
		} `toml:"mcp_servers"`
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return "", nil, err
	}
	server, ok := cfg.MCPServers["gitcontribute"]
	if !ok {
		return "", nil, errors.New("gitcontribute server not found in codex config")
	}
	return server.Command, server.Args, nil
}

func readUpgradeFile(path string) (_ []byte, err error) {
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

func readClaudeCommand(path string) (string, []string, error) {
	data, err := readUpgradeFile(path)
	if err != nil {
		return "", nil, err
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return "", nil, err
	}
	servers, ok := root["mcpServers"].(map[string]any)
	if !ok {
		return "", nil, errors.New("mcpServers is missing from claude config")
	}
	server, ok := servers["gitcontribute"].(map[string]any)
	if !ok {
		return "", nil, errors.New("gitcontribute server not found in claude config")
	}
	command, ok := server["command"].(string)
	if !ok || command == "" {
		return "", nil, errors.New("gitcontribute command is missing from claude config")
	}
	argsIn, ok := server["args"].([]any)
	if !ok {
		return "", nil, errors.New("gitcontribute args are missing from claude config")
	}
	args := make([]string, 0, len(argsIn))
	for i, a := range argsIn {
		s, ok := a.(string)
		if !ok {
			return "", nil, fmt.Errorf("gitcontribute args[%d] must be a string", i)
		}
		args = append(args, s)
	}
	return command, args, nil
}

func runtimeVersionFromPath(command string) string {
	if command == "" {
		return ""
	}
	parent := filepath.Base(filepath.Dir(command))
	if v, err := clientsetup.ResolveNPMVersion(parent); err == nil {
		return v
	}
	normalized := filepath.ToSlash(command)
	const marker = "/node_modules/gitcontribute/"
	if idx := strings.Index(normalized, marker); idx >= 0 {
		root := filepath.FromSlash(normalized[:idx+len(marker)-1])
		return readPackageVersion(root)
	}
	return ""
}
