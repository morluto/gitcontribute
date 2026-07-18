package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/domain"
	clientsetup "github.com/morluto/gitcontribute/internal/setup"
	"github.com/morluto/gitcontribute/internal/terminalinstall"
)

// Setup initializes local state, optionally installs the published CLI through
// npm, and registers the MCP server with selected coding clients. Terminal and
// MCP setup are independent capabilities: either can be selected alone, and a
// terminal-install failure does not prevent MCP from using its pinned npx
// fallback. Any failed step still makes the overall CLI command unsuccessful.
//
// Dry-run setup validates and reports the same capability plan without invoking
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
	if stop, err := run.preflightClients(); err != nil || stop {
		return run.report, err
	}
	if err := run.setupTerminal(); err != nil {
		return nil, err
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
	service         *Service
	ctx             context.Context
	opts            cli.SetupOptions
	observer        cli.SetupObserver
	operation       clientsetup.Operation
	report          *cli.SetupReport
	clientOptions   clientsetup.Options
	clientReport    clientsetup.Report
	configurationOK bool
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
	if opts.Remove && (opts.InstallCLI || opts.SkipMCP) {
		return nil, errors.New("terminal installation options are not supported by remove")
	}
	if operation == clientsetup.Configure && opts.SkipMCP && !opts.InstallCLI {
		return nil, errors.New("setup has no selected capability")
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
	return &setupRun{
		service: s, ctx: ctx, opts: opts, observer: observer, operation: operation,
		report: &cli.SetupReport{Operation: string(operation), DryRun: opts.DryRun},
		clientOptions: clientsetup.Options{
			Operation: operation, Clients: clients, All: opts.AllClients, DryRun: opts.DryRun,
			Home: s.paths.HomeDir(), Version: opts.Version, Executable: opts.Executable, Env: opts.Environment,
		},
		configurationOK: true,
	}, nil
}

func (s *Service) setupClients(opts cli.SetupOptions) ([]clientsetup.Client, error) {
	if opts.SkipMCP {
		return nil, nil
	}
	clients := make([]clientsetup.Client, 0, len(opts.Clients))
	for _, value := range opts.Clients {
		clients = append(clients, clientsetup.Client(strings.ToLower(strings.TrimSpace(value))))
	}
	if len(clients) == 0 && !opts.AllClients {
		clients = clientsetup.Detect(s.paths.HomeDir())
		if len(clients) == 0 {
			return nil, errors.New("no supported clients detected; pass --codex, --claude, or --all-clients")
		}
	}
	return clients, nil
}

func (r *setupRun) preflightClients() (bool, error) {
	if r.opts.SkipMCP {
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

func (r *setupRun) setupTerminal() error {
	if r.operation != clientsetup.Configure {
		return nil
	}
	if !r.opts.InstallCLI {
		return r.reportMissingTerminal()
	}
	setupStarted(r.observer, cli.SetupPhaseTerminal)
	step, executable := installTerminal(r.ctx, r.opts.Version, r.opts.DryRun)
	r.report.Steps = append(r.report.Steps, step)
	setupCompleted(r.observer, step)
	if executable == "" || r.opts.SkipMCP {
		return nil
	}
	r.clientOptions.Executable = executable
	r.clientOptions.Env = map[string]string{}
	planOptions := r.clientOptions
	planOptions.DryRun = true
	report, err := clientsetup.Run(planOptions)
	if err != nil {
		return err
	}
	r.setClientReport(report)
	return nil
}

func (r *setupRun) reportMissingTerminal() error {
	if !clientsetup.IsNpxEnvironment(r.opts.Environment) {
		return nil
	}
	version, err := clientsetup.ResolveNPMVersion(r.opts.Version)
	if err != nil {
		return err
	}
	r.report.Steps = append(r.report.Steps, cli.SetupStep{
		Name: "terminal", Status: "not installed",
		Message: "MCP works without it; run npm install --global gitcontribute@" + version + " for the CLI and TUI",
	})
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
		r.report.Steps = append(r.report.Steps, cli.SetupStep{Name: "corpus", Status: "would initialize"})
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
	if r.opts.SkipMCP {
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

func (r *setupRun) setClientReport(report clientsetup.Report) {
	r.clientReport = report
	r.report.Launcher = strings.Join(append([]string{report.Launcher.Command}, report.Launcher.Args...), " ")
}

func (r *setupRun) appendClientResults() {
	for _, result := range r.clientReport.Results {
		step := cli.SetupStep{Name: string(result.Client), Path: result.Path, Status: result.Status, Message: result.Error}
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
	diagnostics, err := r.service.doctor(r.ctx, false)
	step := cli.SetupStep{Name: "verification", Status: "verified"}
	if err != nil || diagnostics == nil || !diagnostics.Healthy {
		step.Status = "failed"
		if err != nil {
			step.Message = err.Error()
		} else {
			step.Message = "required installation checks failed"
		}
	} else if warnings := diagnosticWarningCount(diagnostics); warnings > 0 {
		step.Message = fmt.Sprintf("verified with %d optional warning(s); run gitcontribute doctor for details", warnings)
	}
	r.report.Steps = append(r.report.Steps, step)
	setupCompleted(r.observer, step)
}

func diagnosticWarningCount(diagnostics *cli.DoctorResult) int {
	warnings := 0
	for _, check := range diagnostics.Checks {
		if check.Status == "warning" {
			warnings++
		}
	}
	return warnings
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

	result := &cli.SetupDiscovery{}
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

// installTerminal converts the requested release into a safe npm package
// specifier and reports installation as an independent setup step. The returned
// path is non-empty only after npm succeeded and the command shim was verified.
func installTerminal(ctx context.Context, version string, dryRun bool) (cli.SetupStep, string) {
	resolvedVersion, err := clientsetup.ResolveNPMVersion(version)
	step := cli.SetupStep{Name: "terminal", Status: "installed", Message: "npm install --global gitcontribute@" + resolvedVersion}
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
