package app

import (
	"bytes"
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
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/managedbinary"
	clientsetup "github.com/morluto/gitcontribute/internal/setup"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/mod/semver"
)

var (
	upgradeCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, name, args...).CombinedOutput()
	}
	runtimeContractCommand = func(ctx context.Context, path string) ([]byte, error) {
		return exec.CommandContext(ctx, path, "runtime-contract").CombinedOutput()
	}
	osExecutable = os.Executable
	upgradeGOOS  = runtime.GOOS
)

type installDetails struct {
	context    string
	executable string
	npmRoot    string
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

	schema, err := s.schemaStage(ctx)
	if err != nil {
		return nil, err
	}
	report.Stages = append(report.Stages, schema)

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
			Message: fmt.Sprintf("updated global npm package to %s", latest),
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

func (s *Service) validateNewerCorpusTarget(ctx context.Context, report *cli.UpgradeReport, candidate, target string) bool {
	fail := func(err error) bool {
		stage := upgradeStage(report, "activation")
		stage.Status = "target_validation_failed"
		stage.Message = err.Error()
		setStage(report, stage)
		report.Status = "target validation failed"
		report.Action = "install a release whose runtime contract supports the configured corpus"
		return false
	}
	if candidate == "" {
		return fail(errors.New("installed target executable is unavailable for schema validation"))
	}
	if err := verifySetupExecutable(candidate); err != nil {
		return fail(fmt.Errorf("installed target executable is not usable: %w", err))
	}
	contract, err := readRuntimeContract(ctx, candidate)
	if err != nil {
		return fail(fmt.Errorf("installed target runtime contract is unreadable: %w", err))
	}
	if normalizeVersion(contract.Version) != normalizeVersion(target) {
		return fail(fmt.Errorf("installed target reports version %s, not target %s", contract.Version, target))
	}
	if contract.SupportedSchemaVersion <= 0 {
		return fail(errors.New("installed target does not report a supported schema version"))
	}

	currentSchema, corpusExists, err := corpus.InspectSchemaVersion(ctx, s.databasePath())
	if err != nil {
		return fail(fmt.Errorf("inspect configured corpus schema: %w", err))
	}
	if !corpusExists {
		return fail(errors.New("configured corpus disappeared during target validation"))
	}
	stage := cli.UpgradeStage{
		Name:    "corpus-schema",
		Path:    s.databasePath(),
		Version: fmt.Sprintf("%d", currentSchema),
		Target:  fmt.Sprintf("%d", contract.SupportedSchemaVersion),
	}
	switch {
	case currentSchema < contract.SupportedSchemaVersion:
		stage.Status = "migration_required"
		stage.Message = fmt.Sprintf("corpus schema %d is older than installed target schema %d", currentSchema, contract.SupportedSchemaVersion)
		setStage(report, stage)
		report.Status = "schema migration required"
		report.Action = "migrate the corpus before running the new version"
		return false
	case currentSchema > contract.SupportedSchemaVersion:
		stage.Status = "newer"
		stage.Message = fmt.Sprintf("corpus schema %d is newer than installed target schema %d", currentSchema, contract.SupportedSchemaVersion)
		setStage(report, stage)
		report.Status = "corpus newer than binary"
		report.Action = "upgrade to a release that supports the current corpus schema"
		return false
	default:
		stage.Status = "current"
		stage.Message = fmt.Sprintf("installed target supports corpus schema %d", currentSchema)
		setStage(report, stage)
		activation := upgradeStage(report, "activation")
		activation.Status = "compatible"
		activation.Message = "installed target runtime is compatible with the configured corpus"
		setStage(report, activation)
		return true
	}
}

func (s *Service) activatePrivateRuntime(ctx context.Context, report *cli.UpgradeReport, details installDetails) {
	clients := outdatedPrivateRuntimeClients(report)
	if len(clients) == 0 {
		return
	}

	target := report.Latest
	candidate := details.executable
	if candidate == "" {
		stage := upgradeStage(report, "activation")
		stage.Status = "target_runtime_unavailable"
		stage.Message = "no staged target executable is available to evaluate"
		setStage(report, stage)
		report.Status = "target runtime unavailable"
		report.Action = stage.Message
		report.RestartClients = nil
		return
	}

	if err := verifySetupExecutable(candidate); err != nil {
		s.setPrivateActivationFailure(report, len(clients), fmt.Errorf("staged target executable is not usable: %w", err))
		return
	}

	contract, err := readRuntimeContract(ctx, candidate)
	if err != nil {
		s.setPrivateActivationFailure(report, len(clients), fmt.Errorf("runtime contract is unreadable: %w", err))
		return
	}
	if normalizeVersion(contract.Version) != normalizeVersion(target) {
		s.setPrivateActivationFailure(report, len(clients), fmt.Errorf("staged executable reports version %s, not target %s", contract.Version, target))
		return
	}
	if contract.SupportedSchemaVersion <= 0 {
		s.setPrivateActivationFailure(report, len(clients), errors.New("staged executable does not report a supported schema version"))
		return
	}

	currentSchema, corpusExists, err := corpus.InspectSchemaVersion(ctx, s.databasePath())
	if err != nil {
		s.setPrivateActivationFailure(report, len(clients), fmt.Errorf("inspect configured corpus schema: %w", err))
		return
	}
	if corpusExists {
		switch {
		case currentSchema < contract.SupportedSchemaVersion:
			msg := fmt.Sprintf("corpus schema %d is older than target schema %d; migrate the corpus before activating", currentSchema, contract.SupportedSchemaVersion)
			s.setPrivateActivationFailure(report, len(clients), errors.New(msg))
			setStage(report, cli.UpgradeStage{
				Name:    "corpus-schema",
				Status:  "migration_required",
				Version: fmt.Sprintf("%d", currentSchema),
				Target:  fmt.Sprintf("%d", contract.SupportedSchemaVersion),
				Message: msg,
			})
			report.Status = "schema migration required"
			report.Action = "migrate the corpus before running the new version"
			report.Rollback = "prior client registrations remain unchanged"
			return
		case currentSchema > contract.SupportedSchemaVersion:
			msg := fmt.Sprintf("corpus schema %d is newer than target schema %d", currentSchema, contract.SupportedSchemaVersion)
			s.setPrivateActivationFailure(report, len(clients), errors.New(msg))
			setStage(report, cli.UpgradeStage{
				Name:    "corpus-schema",
				Status:  "newer",
				Version: fmt.Sprintf("%d", currentSchema),
				Target:  fmt.Sprintf("%d", contract.SupportedSchemaVersion),
				Message: msg,
			})
			report.Status = "corpus newer than binary"
			report.Action = "upgrade to a release that supports the current corpus schema"
			report.Rollback = "prior client registrations remain unchanged"
			return
		}
	}

	dataDir, err := s.paths.DataDir()
	if err != nil {
		s.setPrivateActivationFailure(report, len(clients), err)
		return
	}
	destination, err := managedbinary.Destination(dataDir, target)
	if err != nil {
		s.setPrivateActivationFailure(report, len(clients), err)
		return
	}
	if _, err := managedbinary.Install(candidate, destination); err != nil {
		s.setPrivateActivationFailure(report, len(clients), fmt.Errorf("stage private MCP runtime: %w", err))
		return
	}
	if err := verifySetupExecutable(destination); err != nil {
		s.setPrivateActivationFailure(report, len(clients), fmt.Errorf("verify staged private MCP runtime: %w", err))
		return
	}
	destinationContract, err := readRuntimeContract(ctx, destination)
	if err != nil {
		s.setPrivateActivationFailure(report, len(clients), fmt.Errorf("installed runtime contract is unreadable: %w", err))
		return
	}
	if *destinationContract != *contract {
		s.setPrivateActivationFailure(report, len(clients), fmt.Errorf("installed runtime contract disagrees with source candidate: source=%+v destination=%+v", *contract, *destinationContract))
		return
	}
	setStage(report, cli.UpgradeStage{
		Name: "private-mcp-runtime", Status: "staged", Path: destination,
		Version: target, Target: target, Message: fmt.Sprintf("private MCP runtime %s is staged; client activation is pending", target),
	})

	setupClients := make([]clientsetup.Client, 0, len(clients))
	for _, name := range clients {
		setupClients = append(setupClients, clientsetup.Client(name))
	}
	_, err = clientsetup.ActivateExistingAndVerify(ctx, clientsetup.Options{
		Clients: setupClients, Home: s.paths.HomeDir(), Executable: destination,
	}, func() error { return s.verifyPrivateActivation(ctx, report, setupClients, destination, target) })
	if err != nil {
		s.setPrivateActivationFailure(report, len(clients), fmt.Errorf("activate private MCP runtime: %w", err))
		return
	}

	setStage(report, cli.UpgradeStage{
		Name: "private-mcp-runtime", Status: "verified", Path: destination,
		Version: target, Target: target, Message: fmt.Sprintf("private MCP runtime %s is staged and verified", target),
	})
	setStage(report, cli.UpgradeStage{
		Name: "configured-runtime", Status: "activated",
		Message: fmt.Sprintf("%d registered client(s) now reference runtime %s", len(clients), target),
	})
	setStage(report, cli.UpgradeStage{
		Name: "activation", Status: "restart_required",
		Message: "activation is verified; restart the configured clients to replace their running MCP processes",
	})
	report.Status = "restart required"
	report.Action = "restart the configured clients to activate the verified MCP runtime"
	report.RestartClients = append([]string(nil), clients...)
	report.Rollback = "the previous versioned runtime remains installed; rerun setup with that compatible release to reactivate it"
	setStage(report, cli.UpgradeStage{Name: "rollback", Status: "available", Message: report.Rollback})
}

func readRuntimeContract(ctx context.Context, path string) (*cli.RuntimeContractResult, error) {
	out, err := runtimeContractCommand(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("execute %s runtime-contract: %w", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	var meta cli.RuntimeContractResult
	if err := dec.Decode(&meta); err != nil {
		return nil, fmt.Errorf("parse runtime contract output: %w", err)
	}
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); err != io.EOF {
		return nil, errors.New("trailing data after runtime contract")
	}
	if meta.Name != "gitcontribute" {
		return nil, fmt.Errorf("runtime contract name is %q, want %q", meta.Name, "gitcontribute")
	}
	if meta.Version == "" {
		return nil, errors.New("runtime contract is missing version")
	}
	return &meta, nil
}

func (s *Service) verifyPrivateActivation(ctx context.Context, report *cli.UpgradeReport, clients []clientsetup.Client, destination, target string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, client := range clients {
		configured, err := inspectConfiguredClient(s.paths.HomeDir(), client, target, target)
		if err != nil {
			return fmt.Errorf("verify %s registration: %w", client, err)
		}
		if configured.Status != "target" || filepath.Clean(configured.Path) != filepath.Clean(destination) {
			return fmt.Errorf("verify %s registration: configured runtime does not match %s", client, destination)
		}
		for i := range report.ConfiguredClients {
			if report.ConfiguredClients[i].Name == string(client) {
				report.ConfiguredClients[i] = configured
			}
		}
	}
	return nil
}

func (s *Service) setPrivateActivationFailure(report *cli.UpgradeReport, clientCount int, err error) {
	stage := upgradeStage(report, "activation")
	stage.Status = "failed"
	stage.Message = fmt.Sprintf("%s; prior registrations for %d client(s) were preserved", err, clientCount)
	setStage(report, stage)
	report.Status = "activation failed"
	report.Action = "resolve the activation error and run upgrade --yes again"
	report.RestartClients = nil
}

func upgradeStage(report *cli.UpgradeReport, name string) cli.UpgradeStage {
	for _, stage := range report.Stages {
		if stage.Name == name {
			return stage
		}
	}
	return cli.UpgradeStage{Name: name}
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

func npmLauncherStage(details installDetails, current, latest string) cli.UpgradeStage {
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
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
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
		stage.Message = fmt.Sprintf("installed runtime %s", current)
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
	data, err := os.ReadFile(path)
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

func readClaudeCommand(path string) (string, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return "", nil, err
	}
	servers, _ := root["mcpServers"].(map[string]any)
	server, ok := servers["gitcontribute"].(map[string]any)
	if !ok {
		return "", nil, errors.New("gitcontribute server not found in claude config")
	}
	command, _ := server["command"].(string)
	argsIn, _ := server["args"].([]any)
	args := make([]string, 0, len(argsIn))
	for _, a := range argsIn {
		if s, ok := a.(string); ok {
			args = append(args, s)
		}
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

func (s *Service) schemaStage(ctx context.Context) (cli.UpgradeStage, error) {
	stage := cli.UpgradeStage{Name: "corpus-schema"}
	path := s.databasePath()
	if path == "" {
		stage.Status = "not_configured"
		stage.Message = "corpus database path is not configured"
		return stage, nil
	}
	stage.Path = path
	inspection, err := corpus.InspectSchema(ctx, path)
	if err != nil {
		stage.Status = "failed"
		stage.Message = err.Error()
		return stage, nil
	}
	stage.Version = fmt.Sprintf("%d", inspection.Current)
	stage.Target = fmt.Sprintf("%d", inspection.Target)
	switch inspection.State {
	case corpus.SchemaMissing:
		stage.Status = "missing"
		stage.Message = fmt.Sprintf("corpus has not been created (target schema %d)", inspection.Target)
	case corpus.SchemaCurrent:
		stage.Status = "current"
		stage.Message = fmt.Sprintf("corpus schema is current (%d)", inspection.Current)
	case corpus.SchemaMigrationRequired:
		stage.Status = "migration_required"
		stage.Message = fmt.Sprintf("corpus migration required: current %d, target %d", inspection.Current, inspection.Target)
	case corpus.SchemaNewer:
		stage.Status = "newer"
		stage.Message = fmt.Sprintf("corpus schema %d is newer than this binary supports (%d)", inspection.Current, inspection.Target)
	}
	return stage, nil
}

func activationStage(report *cli.UpgradeReport, opts cli.UpgradeOptions) cli.UpgradeStage {
	stage := cli.UpgradeStage{Name: "activation"}
	switch stageStatus(report, "corpus-schema") {
	case "migration_required":
		stage.Status = "migrate_first"
		stage.Message = "migrate the corpus before running the new version"
		report.Action = stage.Message
		return stage
	case "newer":
		stage.Status = "rollback_or_upgrade"
		stage.Message = "corpus was written by a newer binary; upgrade to a matching release first"
		report.Action = stage.Message
		return stage
	}
	if clients := outdatedPrivateRuntimeClients(report); len(clients) > 0 {
		stage.Status = "setup_required"
		stage.Message = "versioned private MCP runtimes are not replaced by npm launcher upgrade; rerun setup with the target release, then restart configured clients"
		report.Action = stage.Message
		report.RestartClients = clients
		return stage
	}
	switch report.Context {
	case "npx":
		stage.Status = "not_required"
		stage.Message = "npx resolves versions on demand; no activation needed"
	case "other":
		stage.Status = "manual"
		stage.Message = "installation method is not managed automatically"
	case "project-npm":
		stage.Status = "manual"
		stage.Message = "project npm installation; update with npm install --save-dev"
	case "global-npm":
		switch reportVersionDisposition(report) {
		case versionUnavailable:
			stage.Status = "awaiting_confirmation"
			stage.Message = "pass --check or --yes to evaluate the latest release"
		case versionCurrent:
			stage.Status = "current"
			stage.Message = "global npm installation is current"
		case versionDowngrade:
			stage.Status = "downgrade_blocked"
			stage.Message = "installed version is newer than the registry target; automatic downgrade is blocked"
		case versionInvalid:
			stage.Status = "manual"
			stage.Message = "installed and registry versions cannot be compared safely"
		case versionUpgrade, versionPrerelease:
			switch {
			case opts.Yes:
				if upgradeGOOS == "windows" {
					stage.Status = "manual"
					stage.Message = "close running GitContribute processes, then run the displayed command"
				} else {
					stage.Status = "install_and_restart"
					stage.Message = "install the latest release, then restart configured clients"
					report.RestartClients = registeredClients(report)
				}
			case opts.Check:
				stage.Status = "review"
				if reportVersionDisposition(report) == versionPrerelease {
					stage.Message = "a newer prerelease is available; pass --yes to install"
				} else {
					stage.Message = "latest release is available; pass --yes to install"
				}
			default:
				stage.Status = "awaiting_confirmation"
				stage.Message = "pass --yes to install the latest release and restart clients"
			}
		}
	}
	report.Action = stage.Message
	return stage
}

func outdatedPrivateRuntimeClients(report *cli.UpgradeReport) []string {
	var clients []string
	for _, client := range report.ConfiguredClients {
		if client.Status != "outdated" || strings.Contains(filepath.ToSlash(client.Path), "/node_modules/gitcontribute/") {
			continue
		}
		clients = append(clients, client.Name)
	}
	return clients
}

func registeredClients(report *cli.UpgradeReport) []string {
	var names []string
	for _, c := range report.ConfiguredClients {
		if c.Status != "not_configured" && c.Status != "failed" {
			names = append(names, c.Name)
		}
	}
	return names
}

func rollbackStage(report *cli.UpgradeReport) cli.UpgradeStage {
	stage := cli.UpgradeStage{Name: "rollback"}
	switch report.Context {
	case "npx":
		stage.Status = "not_applicable"
		stage.Message = "no persistent installation to roll back"
	case "global-npm":
		stage.Status = "limited"
		stage.Message = "npm global installs cannot be rolled back automatically; reinstall the previous version with npm if needed"
	case "project-npm":
		stage.Status = "manual"
		stage.Message = "roll back by reinstalling the previous version in the project"
	default:
		stage.Status = "manual"
		stage.Message = "roll back by replacing the executable or re-running setup with the previous version"
	}
	if stageStatus(report, "corpus-schema") == "migration_required" {
		stage.Message += "; migrating the corpus without a backup limits rollback"
	}
	report.Rollback = stage.Message
	return stage
}

func stageStatus(report *cli.UpgradeReport, name string) string {
	for _, s := range report.Stages {
		if s.Name == name {
			return s.Status
		}
	}
	return ""
}

func stagePath(report *cli.UpgradeReport, name string) string {
	for _, s := range report.Stages {
		if s.Name == name {
			return s.Path
		}
	}
	return ""
}

func stageVersion(report *cli.UpgradeReport, name string) string {
	for _, s := range report.Stages {
		if s.Name == name {
			return s.Version
		}
	}
	return ""
}

func setStage(report *cli.UpgradeReport, stage cli.UpgradeStage) {
	for i := range report.Stages {
		if report.Stages[i].Name == stage.Name {
			report.Stages[i] = stage
			return
		}
	}
	report.Stages = append(report.Stages, stage)
}
