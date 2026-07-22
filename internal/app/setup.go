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

	"github.com/morluto/gitcontribute/internal/cli"
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
func (s *Service) Setup(ctx context.Context, opts cli.SetupOptions) (*cli.SetupReport, error) {
	return s.setup(ctx, opts, nil)
}

// SetupWithProgress applies setup while reporting phase changes to an optional
// observer owned by the interactive CLI adapter.
func (s *Service) SetupWithProgress(ctx context.Context, opts cli.SetupOptions, observer cli.SetupObserver) (*cli.SetupReport, error) {
	return s.setup(ctx, opts, observer)
}

func (s *Service) setup(ctx context.Context, opts cli.SetupOptions, observer cli.SetupObserver) (*cli.SetupReport, error) {
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
		return nil, err
	}
	if run.report.HasFailures() {
		return run.report, nil
	}
	if run.operation == clientsetup.Configure {
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
	opts                cli.SetupOptions
	observer            cli.SetupObserver
	operation           clientsetup.Operation
	report              *cli.SetupReport
	clientOptions       clientsetup.Options
	clientReport        clientsetup.Report
	managedRuntime      string
	installedExecutable string
	mcpCommandPending   bool
	configurationOK     bool
}

func (s *Service) newSetupRun(ctx context.Context, opts cli.SetupOptions, observer cli.SetupObserver) (*setupRun, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opts.Version == "" {
		opts.Version = s.version
	}
	operation := clientsetup.Configure
	if opts.Remove {
		operation = clientsetup.Remove
	}
	if opts.Remove && opts.Mode != "" {
		return nil, errors.New("an access mode is not supported by remove")
	}
	if operation == clientsetup.Configure && opts.Mode != cli.SetupModeMCP && opts.Mode != cli.SetupModeCLI && opts.Mode != cli.SetupModeBoth {
		return nil, errors.New("setup has no selected access mode")
	}
	if operation == clientsetup.Configure && opts.Mode == cli.SetupModeCLI && (len(opts.Clients) > 0 || opts.AllClients) {
		return nil, errors.New("CLI mode cannot configure MCP clients")
	}
	clients, err := s.setupClients(opts)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(opts.Repository) != "" {
		if _, err := setupRepoRef(opts.Repository); err != nil {
			return nil, err
		}
	}
	run := &setupRun{
		service: s, ctx: ctx, opts: opts, observer: observer, operation: operation,
		report: &cli.SetupReport{Operation: string(operation), DryRun: opts.DryRun},
		clientOptions: clientsetup.Options{
			Operation: operation, Clients: clients, All: opts.AllClients, DryRun: opts.DryRun,
			Home: s.paths.HomeDir(), Executable: opts.Executable,
		},
		configurationOK: true,
	}
	if operation == clientsetup.Configure && opts.Mode == cli.SetupModeMCP {
		dataDir, err := s.paths.DataDir()
		if err != nil {
			return nil, err
		}
		run.managedRuntime, err = managedbinary.Destination(dataDir, opts.Version)
		if err != nil {
			return nil, err
		}
		run.clientOptions.Executable = run.managedRuntime
	} else if operation == clientsetup.Configure && opts.Mode == cli.SetupModeBoth {
		run.mcpCommandPending = true
	}
	return run, nil
}

func (s *Service) setupClients(opts cli.SetupOptions) ([]clientsetup.Client, error) {
	if !opts.Remove && !opts.Mode.ConfiguresMCP() {
		return nil, nil
	}
	clients := make([]clientsetup.Client, 0, len(opts.Clients))
	for _, value := range opts.Clients {
		clients = append(clients, clientsetup.Client(strings.ToLower(strings.TrimSpace(value))))
	}
	if len(clients) == 0 && !opts.AllClients {
		return nil, errors.New("no coding-agent targets selected; pass --codex, --claude, or --all-clients")
	}
	return clients, nil
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
	if r.operation != clientsetup.Configure {
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
	step := cli.SetupStep{Name: "corpus", Path: inspection.Path, Status: "failed"}
	switch inspection.State {
	case "migration_required":
		step.Message = fmt.Sprintf("database schema version %d requires migration to %d; run gitcontribute corpus migrate --yes", inspection.Current, inspection.Target)
	case "newer":
		return true, fmt.Errorf("setup cannot continue: database schema version %d is newer than this binary supports (%d) at %s; run a matching GitContribute release, gitcontribute upgrade, or gitcontribute corpus inspect; no changes were made", inspection.Current, inspection.Target, inspection.Path)
	case "damaged":
		return true, fmt.Errorf("setup cannot continue: local corpus at %s is damaged: %s; run gitcontribute corpus inspect; no changes were made", inspection.Path, inspection.Problem)
	default:
		step.Message = "the corpus cannot be initialized in its current state"
	}
	r.report.Steps = append(r.report.Steps, step)
	return true, nil
}

func (r *setupRun) setupRuntime() error {
	if r.operation != clientsetup.Configure {
		return nil
	}
	if !r.opts.Mode.InstallsCLI() {
		return r.installManagedRuntime()
	}
	setupStarted(r.observer, cli.SetupPhaseCLI)
	step, executable := installCLI(r.ctx, r.opts.Version, r.opts.DryRun)
	r.report.Steps = append(r.report.Steps, step)
	setupCompleted(r.observer, step)
	r.installedExecutable = executable
	if executable == "" {
		if !r.opts.DryRun {
			r.mcpCommandPending = false
			r.report.MCPCommandPending = false
		}
		return nil
	}
	if !r.opts.Mode.ConfiguresMCP() {
		return nil
	}
	r.mcpCommandPending = false
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
	step := cli.SetupStep{Name: "mcp-runtime", Path: r.managedRuntime, Status: "installed"}
	if r.opts.DryRun {
		step.Status = "would install"
		r.report.Steps = append(r.report.Steps, step)
		return nil
	}
	setupStarted(r.observer, cli.SetupPhaseMCPRuntime)
	r.installedExecutable = r.managedRuntime
	source := r.opts.Executable
	if source == "" {
		var err error
		source, err = os.Executable()
		if err != nil {
			return fmt.Errorf("resolve packaged executable: %w", err)
		}
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

func (r *setupRun) configure() {
	setupStarted(r.observer, cli.SetupPhaseConfiguration)
	configPath, pathErr := r.service.paths.ConfigFile()
	configExisted := pathErr == nil
	if configExisted {
		_, statErr := os.Stat(configPath)
		configExisted = statErr == nil
	}
	tokenSource := strings.TrimSpace(r.opts.TokenSource)
	if tokenSource == "" {
		tokenSource = autoTokenSource()
	}
	if tokenSource == "env" && strings.TrimSpace(r.opts.TokenSourceKey) == "" {
		r.opts.TokenSourceKey = "GITHUB_TOKEN"
	}
	r.report.Authentication = &cli.SetupAuthentication{Method: tokenSource, Key: r.opts.TokenSourceKey}
	options := cli.ConfigureOptions{DryRun: r.opts.DryRun, TokenSource: &tokenSource}
	if r.opts.TokenSourceKey != "" {
		options.TokenSourceKey = &r.opts.TokenSourceKey
	}
	configured, err := r.service.Configure(r.ctx, options)
	step := configurationStep(configured, err, configExisted, r.opts.DryRun)
	r.report.Steps = append(r.report.Steps, step)
	setupCompleted(r.observer, step)
	if err != nil {
		r.configurationOK = false
	}
	r.initializeCorpus(err == nil)
}

func configurationStep(configured *cli.ConfigureResult, err error, existed, dryRun bool) cli.SetupStep {
	step := cli.SetupStep{Name: "configuration", Status: "configured"}
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
	if r.opts.DryRun {
		step := cli.SetupStep{Name: "corpus", Status: "would initialize"}
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
	setupStarted(r.observer, cli.SetupPhaseCorpus)
	initialized, err := r.service.Init(r.ctx)
	step := cli.SetupStep{Name: "corpus", Status: "initialized"}
	if initialized != nil {
		step.Path = initialized.Path
		step.Message = initialized.Message
	}
	if err != nil {
		step.Status = "failed"
		step.Message = err.Error()
		r.configurationOK = false
	}
	r.report.Steps = append(r.report.Steps, step)
	setupCompleted(r.observer, step)
}

func (r *setupRun) registerClients() error {
	if !r.configuresClients() {
		return nil
	}
	if !r.opts.DryRun && r.configurationOK {
		setupStarted(r.observer, cli.SetupPhaseClients)
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
	return r.operation == clientsetup.Remove || r.opts.Mode.ConfiguresMCP()
}

func (r *setupRun) setClientReport(report clientsetup.Report) {
	r.clientReport = report
	if r.mcpCommandPending {
		r.report.MCPCommand = nil
		r.report.MCPCommandPending = true
		return
	}
	r.report.MCPCommand = &cli.SetupMCPCommand{
		Command: report.Launcher.Command,
		Args:    append([]string(nil), report.Launcher.Args...),
	}
	r.report.MCPCommandPending = false
}

func (r *setupRun) appendClientResults() {
	for _, result := range r.clientReport.Results {
		step := cli.SetupStep{Name: string(result.Client), Path: result.Path, Status: result.Status, Message: result.Error}
		r.report.Steps = append(r.report.Steps, step)
		if !r.opts.DryRun && r.operation == clientsetup.Configure && (result.Status == "configured" || result.Status == "updated") {
			r.report.RestartClients = append(r.report.RestartClients, string(result.Client))
		}
		setupCompleted(r.observer, step)
	}
	if skill := r.clientReport.CodexSkill; skill.Status != "" {
		step := cli.SetupStep{Name: "codex-skill", Path: skill.Path, Status: skill.Status, Message: skill.Error}
		r.report.Steps = append(r.report.Steps, step)
		setupCompleted(r.observer, step)
	}
}

func (r *setupRun) addRepository() {
	if r.operation != clientsetup.Configure || strings.TrimSpace(r.opts.Repository) == "" {
		return
	}
	setupStarted(r.observer, cli.SetupPhaseRepository)
	ref, err := setupRepoRef(r.opts.Repository)
	step := cli.SetupStep{Name: "repository", Status: "added", Message: r.opts.Repository}
	if err != nil {
		step.Status = "failed"
		step.Message = err.Error()
	} else if r.opts.DryRun {
		step.Status = "would add"
	} else if _, err := r.service.AddRepoSource(r.ctx, setupSourceName(ref), []cli.RepoRef{ref}); err != nil {
		step.Status = "failed"
		step.Message = err.Error()
	}
	r.report.Steps = append(r.report.Steps, step)
	setupCompleted(r.observer, step)
}

func (r *setupRun) verify() {
	if r.operation != clientsetup.Configure || r.opts.DryRun {
		return
	}
	setupStarted(r.observer, cli.SetupPhaseVerification)
	step := cli.SetupStep{Name: "verification", Status: "verified"}
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
				} else if result.Status != "already configured" {
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

func setupStarted(observer cli.SetupObserver, phase cli.SetupPhase) {
	if observer != nil {
		observer.SetupStarted(phase)
	}
}

func setupCompleted(observer cli.SetupObserver, step cli.SetupStep) {
	if observer != nil {
		observer.SetupCompleted(step)
	}
}

// DiscoverSetup inspects local onboarding state without writes, network access,
// credential resolution, or process execution.
func (s *Service) DiscoverSetup(ctx context.Context) (*cli.SetupDiscovery, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	home := s.paths.HomeDir()
	detected := make(map[clientsetup.Client]bool)
	for _, client := range clientsetup.Detect(home) {
		detected[client] = true
	}

	result := &cli.SetupDiscovery{Version: s.version}
	for _, client := range clientsetup.AllClients {
		registered, path, err := clientsetup.CheckRegistration(client, home)
		item := cli.SetupClientDiscovery{
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
	result.ConfiguredTokenSource = cfg.TokenSource.Method
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
func installCLI(ctx context.Context, version string, dryRun bool) (cli.SetupStep, string) {
	resolvedVersion, err := clientsetup.ResolveNPMVersion(version)
	step := cli.SetupStep{Name: "cli", Status: "installed", Message: "npm install --global gitcontribute@" + resolvedVersion}
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

func setupSourceName(ref cli.RepoRef) string {
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

func setupRepoRef(value string) (cli.RepoRef, error) {
	value = strings.TrimSpace(strings.TrimSuffix(value, "/"))
	value = strings.TrimPrefix(value, "https://github.com/")
	parts := strings.Split(value, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return cli.RepoRef{}, fmt.Errorf("repository must be OWNER/REPO")
	}
	ref := cli.RepoRef{Owner: parts[0], Repo: strings.TrimSuffix(parts[1], ".git")}
	if err := (domain.RepoRef{Owner: ref.Owner, Repo: ref.Repo}).Validate(); err != nil {
		return cli.RepoRef{}, err
	}
	return ref, nil
}
