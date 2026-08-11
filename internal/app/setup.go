package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/managedbinary"
	clientsetup "github.com/morluto/gitcontribute/internal/setup"
	"github.com/morluto/gitcontribute/internal/terminalinstall"
)

// Setup initializes local state for one of three access modes. MCP-only setup
// copies the running native executable into a private product-owned directory;
// CLI-only setup installs the published command globally through npm; Both
// registers that verified global executable with selected coding clients.
// Installation failures stop before later configuration writes.
//
// Dry-run setup validates and reports the same access-mode plan without invoking
// npm or writing local state. Setup performs no GitHub access and never executes
// repository-controlled code.
func (s *Service) Setup(ctx context.Context, opts contracts.SetupOptions) (*contracts.SetupReport, error) {
	return s.setup(ctx, opts, nil)
}

// SetupWithProgress applies setup while reporting phase changes to an optional
// observer owned by the interactive CLI adapter.
func (s *Service) SetupWithProgress(ctx context.Context, opts contracts.SetupOptions, observer contracts.SetupObserver) (*contracts.SetupReport, error) {
	return s.setup(ctx, opts, observer)
}

func (s *Service) setup(ctx context.Context, opts contracts.SetupOptions, observer contracts.SetupObserver) (*contracts.SetupReport, error) {
	run, err := s.newSetupRun(ctx, opts, observer)
	if err != nil {
		return nil, err
	}
	if stop, err := run.preflightCorpus(); err != nil || stop {
		return run.report, err
	}
	if stop, err := run.preflightClients(); err != nil || stop {
		return run.report, err
	}
	if err := run.setupRuntime(); err != nil {
		return run.report, err
	}
	if run.report.HasFailures() {
		return run.report, nil
	}
	if run.request.kind.configuresProduct() {
		run.configure()
	}
	if err := run.registerClients(); err != nil {
		return nil, err
	}
	run.addRepository()
	run.verify()
	return run.report, nil
}

type setupRun struct {
	service             *Service
	ctx                 context.Context
	request             setupRequest
	observer            contracts.SetupObserver
	report              *contracts.SetupReport
	clientOptions       clientsetup.Options
	clientReport        clientsetup.Report
	managedRuntime      string
	installedExecutable string
	mcpCommandState     setupMCPCommandState
	readiness           setupReadiness
}

type setupMCPCommandState uint8

const (
	setupMCPCommandReady setupMCPCommandState = iota
	setupMCPCommandPending
)

type setupReadiness uint8

const (
	setupReady setupReadiness = iota
	setupBlocked
)

func (s *Service) newSetupRun(ctx context.Context, opts contracts.SetupOptions, observer contracts.SetupObserver) (*setupRun, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	request, err := parseSetupRequest(opts, s.version)
	if err != nil {
		return nil, err
	}
	operation := request.kind.clientOperation()
	run := &setupRun{
		service: s, ctx: ctx, request: request, observer: observer,
		report: &contracts.SetupReport{Operation: string(operation), DryRun: request.execution.dryRun()},
		clientOptions: clientsetup.Options{
			Operation: operation, Clients: append([]clientsetup.Client(nil), request.clients...), DryRun: request.execution.dryRun(),
			Home: s.paths.HomeDir(),
		},
	}
	switch request.kind {
	case setupMCP:
		dataDir, err := s.paths.DataDir()
		if err != nil {
			return nil, err
		}
		run.managedRuntime, err = managedbinary.Destination(dataDir, request.version)
		if err != nil {
			return nil, err
		}
		run.clientOptions.Executable = run.managedRuntime
	case setupBoth:
		run.mcpCommandState = setupMCPCommandPending
	}
	return run, nil
}

func (r *setupRun) preflightClients() (bool, error) {
	if !r.configuresClients() {
		return false, nil
	}
	planOptions := r.clientOptions
	planOptions.DryRun = true
	report, err := clientsetup.Run(planOptions)
	if err != nil {
		return false, err
	}
	r.setClientReport(report)
	for _, result := range report.Results {
		if result.Error != "" {
			r.appendClientResults()
			return true, nil
		}
	}
	return false, nil
}

func (r *setupRun) preflightCorpus() (bool, error) {
	if !r.request.kind.configuresProduct() {
		return false, nil
	}
	inspection, err := r.service.InspectCorpus(r.ctx)
	if err != nil {
		return false, err
	}
	r.report.Corpus = inspection
	if inspection.State == "missing" || inspection.State == "current" {
		return false, nil
	}
	step := contracts.SetupStep{Name: "corpus", Path: inspection.Path, Status: "failed"}
	switch inspection.State {
	case "migration_required":
		step.Message = fmt.Sprintf("database schema version %d requires migration to %d; run `npx --yes gitcontribute@latest corpus migrate --yes`", inspection.Current, inspection.Target)
	case "newer":
		return true, fmt.Errorf("setup cannot continue: database schema version %d is newer than this binary supports (%d) at %s; run `npx --yes gitcontribute@latest upgrade`, or use a matching GitContribute release; no changes were made", inspection.Current, inspection.Target, inspection.Path)
	case "incompatible":
		return true, fmt.Errorf("setup cannot continue: database schema version %d has an incompatible schema identity at %s and cannot be migrated in place; use a matching GitContribute release, or archive or move the corpus out of the configured path and rerun `npx --yes gitcontribute@latest setup`; no changes were made", inspection.Current, inspection.Path)
	case "damaged":
		return true, fmt.Errorf("setup cannot continue: local corpus at %s is damaged: %s; run `npx --yes gitcontribute@latest corpus inspect`; no changes were made", inspection.Path, inspection.Problem)
	default:
		step.Message = "the corpus cannot be initialized in its current state"
	}
	r.report.Steps = append(r.report.Steps, step)
	return true, nil
}

func (r *setupRun) setupRuntime() error {
	if !r.request.kind.configuresProduct() {
		return nil
	}
	if !r.request.kind.installsCLI() {
		return r.installManagedRuntime()
	}
	setupStarted(r.observer, contracts.SetupPhaseCLI)
	step, executable := installCLI(r.ctx, r.request.version, r.request.execution.dryRun())
	r.report.Steps = append(r.report.Steps, step)
	setupCompleted(r.observer, step)
	r.installedExecutable = executable
	if executable == "" {
		if !r.request.execution.dryRun() {
			r.mcpCommandState = setupMCPCommandReady
			r.report.MCPCommandPending = false
		}
		return nil
	}
	if !r.request.kind.configuresClients() {
		return nil
	}
	r.mcpCommandState = setupMCPCommandReady
	r.clientOptions.Executable = executable
	planOptions := r.clientOptions
	planOptions.DryRun = true
	report, err := clientsetup.Run(planOptions)
	if err != nil {
		return err
	}
	r.setClientReport(report)
	return nil
}

func (r *setupRun) installManagedRuntime() error {
	step := contracts.SetupStep{Name: "mcp-runtime", Path: r.managedRuntime, Status: "installed"}
	newer, found, err := r.newerManagedRuntime()
	if err != nil {
		step.Status = "failed"
		step.Message = err.Error()
		r.report.Steps = append(r.report.Steps, step)
		return err
	}
	if found {
		step.Status = "failed"
		step.Message = fmt.Sprintf("newer private MCP runtime %s is already installed at %s; this bootstrap is %s; no changes were made; run `npx --yes gitcontribute@latest setup`", newer.Version, newer.Path, normalizeVersion(r.request.version))
		r.report.Steps = append(r.report.Steps, step)
		return nil
	}
	if r.request.execution.dryRun() {
		step.Status = "would install"
		r.report.Steps = append(r.report.Steps, step)
		return nil
	}
	setupStarted(r.observer, contracts.SetupPhaseMCPRuntime)
	r.installedExecutable = r.managedRuntime
	executable := r.service.executable
	if executable == nil {
		executable = os.Executable
	}
	source, err := executable()
	if err != nil {
		return fmt.Errorf("resolve packaged executable: %w", err)
	}
	installed, err := managedbinary.Install(source, r.managedRuntime)
	if err != nil {
		step.Status = "failed"
		step.Message = err.Error()
	} else if !installed {
		step.Status = "already installed"
	}
	r.report.Steps = append(r.report.Steps, step)
	setupCompleted(r.observer, step)
	return nil
}

func (r *setupRun) newerManagedRuntime() (managedbinary.InstalledRuntime, bool, error) {
	dataDir, err := r.service.paths.DataDir()
	if err != nil {
		return managedbinary.InstalledRuntime{}, false, err
	}
	runtimes, err := managedbinary.List(dataDir)
	if err != nil {
		return managedbinary.InstalledRuntime{}, false, err
	}
	requested := normalizeVersion(r.request.version)
	for i := range runtimes {
		if isNewerVersion(requested, runtimes[i].Version) {
			return runtimes[i], true, nil
		}
	}
	return managedbinary.InstalledRuntime{}, false, nil
}

func (r *setupRun) configure() {
	setupStarted(r.observer, contracts.SetupPhaseConfiguration)
	configPath, pathErr := r.service.paths.ConfigFile()
	configExisted := pathErr == nil
	if configExisted {
		_, statErr := os.Stat(configPath)
		configExisted = statErr == nil
	}
	tokenSource := strings.TrimSpace(r.request.tokenSource)
	if tokenSource == "" {
		tokenSource = autoTokenSource()
	}
	tokenSourceKey := r.request.tokenSourceKey
	if tokenSource == "env" && strings.TrimSpace(tokenSourceKey) == "" {
		tokenSourceKey = "GITHUB_TOKEN"
	}
	r.report.Authentication = &contracts.SetupAuthentication{Method: tokenSource, Key: tokenSourceKey}
	options := contracts.ConfigureOptions{DryRun: r.request.execution.dryRun(), TokenSource: &tokenSource}
	if tokenSourceKey != "" {
		options.TokenSourceKey = &tokenSourceKey
	}
	configured, err := r.service.Configure(r.ctx, options)
	step := configurationStep(configured, err, configExisted, r.request.execution.dryRun())
	r.report.Steps = append(r.report.Steps, step)
	setupCompleted(r.observer, step)
	if err != nil {
		r.readiness = setupBlocked
	}
	r.initializeCorpus(err == nil)
}

func configurationStep(configured *contracts.ConfigureResult, err error, existed, dryRun bool) contracts.SetupStep {
	step := contracts.SetupStep{Name: "configuration", Status: "configured"}
	if configured != nil {
		step.Path = configured.Path
		if dryRun && (!existed || configured.Changed) {
			step.Status = "would configure"
		} else if existed && !configured.Changed {
			step.Status = "already configured"
		}
	}
	if err != nil {
		step.Status = "failed"
		step.Message = err.Error()
	}
	return step
}

func (r *setupRun) initializeCorpus(configured bool) {
	if r.request.execution.dryRun() {
		step := contracts.SetupStep{Name: "corpus", Status: "would initialize"}
		inspection := r.report.Corpus
		if inspection == nil {
			step.Status = "failed"
			step.Message = "corpus compatibility was not inspected"
			r.report.Steps = append(r.report.Steps, step)
			return
		}
		step.Path = inspection.Path
		switch inspection.State {
		case "missing":
			step.Status = "would initialize"
		case "current":
			step.Status = "already initialized"
		}
		r.report.Steps = append(r.report.Steps, step)
		return
	}
	if !configured {
		return
	}
	setupStarted(r.observer, contracts.SetupPhaseCorpus)
	initialized, err := r.service.Init(r.ctx)
	step := contracts.SetupStep{Name: "corpus", Status: "initialized"}
	if initialized != nil {
		step.Path = initialized.Path
		step.Message = initialized.Message
	}
	if err != nil {
		step.Status = "failed"
		step.Message = err.Error()
		r.readiness = setupBlocked
	}
	r.report.Steps = append(r.report.Steps, step)
	setupCompleted(r.observer, step)
}

func (r *setupRun) registerClients() error {
	if !r.configuresClients() {
		return nil
	}
	if !r.request.execution.dryRun() && r.readiness == setupReady {
		setupStarted(r.observer, contracts.SetupPhaseClients)
		r.clientOptions.DryRun = false
		report, err := clientsetup.Run(r.clientOptions)
		if err != nil {
			return err
		}
		r.setClientReport(report)
	}
	r.appendClientResults()
	return nil
}

func (r *setupRun) configuresClients() bool {
	return r.request.kind.configuresClients()
}

func (r *setupRun) setClientReport(report clientsetup.Report) {
	r.clientReport = report
	if r.mcpCommandState == setupMCPCommandPending {
		r.report.MCPCommand = nil
		r.report.MCPCommandPending = true
		return
	}
	r.report.MCPCommand = &contracts.SetupMCPCommand{
		Command: report.Launcher.Command,
		Args:    append([]string(nil), report.Launcher.Args...),
	}
	r.report.MCPCommandPending = false
}

func (r *setupRun) appendClientResults() {
	for _, result := range r.clientReport.Results {
		step := contracts.SetupStep{Name: string(result.Client), Path: result.Path, Status: string(result.Status), Message: result.Error}
		r.report.Steps = append(r.report.Steps, step)
		if !r.request.execution.dryRun() && r.request.kind.configuresProduct() && (result.Status == clientsetup.ChangeConfigured || result.Status == clientsetup.ChangeUpdated) {
			r.report.RestartClients = append(r.report.RestartClients, string(result.Client))
		}
		setupCompleted(r.observer, step)
	}
	if skill := r.clientReport.CodexSkill; skill.Status != "" {
		step := contracts.SetupStep{Name: "codex-skill", Path: skill.Path, Status: string(skill.Status), Message: skill.Error}
		r.report.Steps = append(r.report.Steps, step)
		setupCompleted(r.observer, step)
	}
}

func (r *setupRun) addRepository() {
	if !r.request.kind.configuresProduct() || r.request.repository == nil {
		return
	}
	setupStarted(r.observer, contracts.SetupPhaseRepository)
	ref := *r.request.repository
	step := contracts.SetupStep{Name: "repository", Status: "added", Message: ref.String()}
	if r.request.execution.dryRun() {
		step.Status = "would add"
	} else if _, err := r.service.AddRepoSource(r.ctx, setupSourceName(ref), []contracts.RepoRef{ref}); err != nil {
		step.Status = "failed"
		step.Message = err.Error()
	}
	r.report.Steps = append(r.report.Steps, step)
	setupCompleted(r.observer, step)
}

func (r *setupRun) verify() {
	if !r.request.kind.configuresProduct() || r.request.execution.dryRun() {
		return
	}
	setupStarted(r.observer, contracts.SetupPhaseVerification)
	step := contracts.SetupStep{Name: "verification", Status: "verified"}
	if err := r.verifyAppliedSetup(); err != nil {
		step.Status = "failed"
		step.Message = err.Error()
	}
	r.report.Steps = append(r.report.Steps, step)
	setupCompleted(r.observer, step)
}

func (r *setupRun) verifyAppliedSetup() error {
	failures := make([]string, 0, 5)
	if executableErr := verifySetupExecutable(r.installedExecutable); executableErr != nil {
		failures = append(failures, "executable: "+executableErr.Error())
	}
	c, err := r.service.openReadOnlyCorpus(r.ctx)
	if err != nil {
		failures = append(failures, "database: "+err.Error())
	} else {
		current, target, schemaErr := c.SchemaVersions(r.ctx)
		if schemaErr != nil {
			failures = append(failures, "schema: "+schemaErr.Error())
		} else if current != target {
			failures = append(failures, fmt.Sprintf("schema: database version %d does not match expected version %d", current, target))
		}
	}
	if gitErr := commandAvailable(r.ctx, "git", "--version"); gitErr != nil {
		failures = append(failures, "git: "+redactDiagnostic(gitErr.Error()))
	}
	if r.configuresClients() {
		opts := r.clientOptions
		opts.DryRun = true
		report, clientErr := clientsetup.Run(opts)
		if clientErr != nil {
			failures = append(failures, "mcp registration: "+clientErr.Error())
		} else {
			for _, result := range report.Results {
				if result.Error != "" {
					failures = append(failures, string(result.Client)+": "+result.Error)
				} else if result.Status != clientsetup.ChangeAlreadyConfigured {
					failures = append(failures, fmt.Sprintf("%s: registration does not match the configured MCP command", result.Client))
				}
			}
		}
	}
	if len(failures) == 0 {
		return nil
	}
	return errors.New(strings.Join(failures, "; "))
}

func verifySetupExecutable(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("installed command path is unavailable")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect installed command: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("installed command is not a regular file: %s", path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("installed command is not executable: %s", path)
	}
	return nil
}

func setupStarted(observer contracts.SetupObserver, phase contracts.SetupPhase) {
	if observer != nil {
		observer.SetupStarted(phase)
	}
}

func setupCompleted(observer contracts.SetupObserver, step contracts.SetupStep) {
	if observer != nil {
		observer.SetupCompleted(step)
	}
}

// DiscoverSetup inspects local onboarding state without writes, network access,
// credential resolution, or process execution.
func (s *Service) DiscoverSetup(ctx context.Context) (*contracts.SetupDiscovery, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	home := s.paths.HomeDir()
	detected := make(map[clientsetup.Client]bool)
	for _, client := range clientsetup.Detect(home) {
		detected[client] = true
	}

	result := &contracts.SetupDiscovery{Version: s.version}
	for _, client := range clientsetup.SupportedClients() {
		registered, path, err := clientsetup.CheckRegistration(client, home)
		item := contracts.SetupClientDiscovery{
			Name:       string(client),
			Path:       path,
			Detected:   detected[client],
			Registered: registered,
		}
		if err != nil {
			item.Error = err.Error()
		}
		result.Clients = append(result.Clients, item)
	}

	configPath, err := s.paths.ConfigFile()
	if err != nil {
		return nil, err
	}
	cfg, err := s.persistedConfig(configPath)
	if err != nil {
		return nil, err
	}
	result.ConfiguredTokenSource = string(cfg.TokenSource.Method)
	result.ConfiguredTokenKey = cfg.TokenSource.Key
	_, ghErr := exec.LookPath("gh")
	result.GitHubCLIAvailable = ghErr == nil
	envKey := cfg.TokenSource.Key
	if envKey == "" {
		envKey = "GITHUB_TOKEN"
	}
	if s.paths.Env != nil {
		_, result.EnvironmentKeyPresent = s.paths.Env.Vars[envKey]
	} else {
		_, result.EnvironmentKeyPresent = os.LookupEnv(envKey)
	}
	return result, nil
}

// installCLI converts the requested release into a safe npm package
// specifier and reports installation as an independent setup step. The returned
// path is non-empty only after npm succeeded and the command shim was verified.
func installCLI(ctx context.Context, version string, dryRun bool) (contracts.SetupStep, string) {
	resolvedVersion, err := clientsetup.ResolveNPMVersion(version)
	step := contracts.SetupStep{Name: "cli", Status: "installed", Message: "npm install --global gitcontribute@" + resolvedVersion}
	if err != nil {
		step.Status = "failed"
		step.Message = err.Error()
		return step, ""
	}
	if dryRun {
		step.Status = "would install"
		return step, ""
	}
	commandPath, err := terminalinstall.GlobalNPM(ctx, "gitcontribute@"+resolvedVersion)
	if err != nil {
		step.Status = "failed"
		step.Message = err.Error()
		return step, ""
	}
	step.Path = commandPath
	return step, commandPath
}

func setupSourceName(ref contracts.RepoRef) string {
	name := strings.ToLower(ref.Owner + "-" + ref.Repo)
	if len(name) <= 64 {
		return name
	}
	sum := sha256.Sum256([]byte(ref.String()))
	return fmt.Sprintf("%s-%x", name[:55], sum[:4])
}

func autoTokenSource() string {
	if _, err := exec.LookPath("gh"); err == nil {
		return "gh-cli"
	}
	return "none"
}

func setupRepoRef(value string) (contracts.RepoRef, error) {
	value = strings.TrimSpace(strings.TrimSuffix(value, "/"))
	value = strings.TrimPrefix(value, "https://github.com/")
	parts := strings.Split(value, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return contracts.RepoRef{}, fmt.Errorf("repository must be OWNER/REPO")
	}
	parsed, err := domain.NewRepoRef(parts[0], strings.TrimSuffix(parts[1], ".git"))
	if err != nil {
		return contracts.RepoRef{}, err
	}
	return contracts.RepoRef{Owner: parsed.Owner(), Repo: parsed.Repo()}, nil
}
