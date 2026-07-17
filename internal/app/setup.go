package app

import (
	"context"
	"crypto/sha256"
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
		return nil, fmt.Errorf("terminal installation options are not supported by remove")
	}
	if operation == clientsetup.Configure && opts.SkipMCP && !opts.InstallCLI {
		return nil, fmt.Errorf("setup has no selected capability")
	}
	clients := make([]clientsetup.Client, 0, len(opts.Clients))
	if !opts.SkipMCP {
		for _, value := range opts.Clients {
			clients = append(clients, clientsetup.Client(strings.ToLower(strings.TrimSpace(value))))
		}
		if len(clients) == 0 && !opts.AllClients {
			clients = clientsetup.Detect(s.paths.HomeDir())
			if len(clients) == 0 {
				return nil, fmt.Errorf("no supported clients detected; pass --codex, --claude, or --all-clients")
			}
		}
	}
	if strings.TrimSpace(opts.Repository) != "" {
		if _, err := setupRepoRef(opts.Repository); err != nil {
			return nil, err
		}
	}
	var err error
	report := &cli.SetupReport{Operation: string(operation), DryRun: opts.DryRun}
	clientOptions := clientsetup.Options{
		Operation: operation, Clients: clients, All: opts.AllClients, DryRun: opts.DryRun,
		Home: s.paths.HomeDir(), Version: opts.Version, Executable: opts.Executable, Env: opts.Environment,
	}
	var clientReport clientsetup.Report
	if !opts.SkipMCP {
		// Preflight every selected client before the global npm mutation. A
		// malformed existing client config must leave all setup targets untouched.
		planOptions := clientOptions
		planOptions.DryRun = true
		clientReport, err = clientsetup.Run(planOptions)
		if err != nil {
			return nil, err
		}
		report.Launcher = strings.Join(append([]string{clientReport.Launcher.Command}, clientReport.Launcher.Args...), " ")
		planFailed := false
		for _, result := range clientReport.Results {
			if result.Error != "" {
				planFailed = true
			}
		}
		if planFailed {
			for _, result := range clientReport.Results {
				report.Steps = append(report.Steps, cli.SetupStep{Name: string(result.Client), Path: result.Path, Status: result.Status, Message: result.Error})
			}
			return report, nil
		}
	}
	if operation == clientsetup.Configure && opts.InstallCLI {
		step, installedExecutable := installTerminal(ctx, opts.Version, opts.DryRun)
		report.Steps = append(report.Steps, step)
		if installedExecutable != "" && !opts.SkipMCP {
			// The process still carries npm_exec/npx environment markers from the
			// bootstrap invocation. Clear that evidence and re-plan so MCP uses the
			// verified persistent command rather than the ephemeral npx fallback.
			clientOptions.Executable = installedExecutable
			clientOptions.Env = map[string]string{}
			planOptions := clientOptions
			planOptions.DryRun = true
			clientReport, err = clientsetup.Run(planOptions)
			if err != nil {
				return nil, err
			}
			report.Launcher = strings.Join(append([]string{clientReport.Launcher.Command}, clientReport.Launcher.Args...), " ")
		}
	} else if operation == clientsetup.Configure && clientsetup.IsNpxEnvironment(opts.Environment) {
		// MCP-only setup is valid, but make the missing human-facing command
		// explicit so a successful MCP registration is not mistaken for a global
		// terminal installation.
		version, versionErr := clientsetup.ResolveNPMVersion(opts.Version)
		if versionErr != nil {
			return nil, versionErr
		}
		report.Steps = append(report.Steps, cli.SetupStep{
			Name:    "terminal",
			Status:  "not installed",
			Message: "MCP works without it; run npm install --global gitcontribute@" + version + " for the CLI and TUI",
		})
	}
	configurationOK := true
	if operation == clientsetup.Configure {
		configPath, pathErr := s.paths.ConfigFile()
		configExisted := pathErr == nil
		if configExisted {
			_, statErr := os.Stat(configPath)
			configExisted = statErr == nil
		}
		tokenSource := strings.TrimSpace(opts.TokenSource)
		if tokenSource == "" {
			tokenSource = autoTokenSource()
		}
		if tokenSource == "env" && strings.TrimSpace(opts.TokenSourceKey) == "" {
			opts.TokenSourceKey = "GITHUB_TOKEN"
		}
		configure := cli.ConfigureOptions{DryRun: opts.DryRun, TokenSource: &tokenSource}
		if opts.TokenSourceKey != "" {
			configure.TokenSourceKey = &opts.TokenSourceKey
		}
		configured, configureErr := s.Configure(ctx, configure)
		step := cli.SetupStep{Name: "configuration", Status: "configured"}
		if configured != nil {
			step.Path = configured.Path
			if !configExisted {
				if opts.DryRun {
					step.Status = "would configure"
				}
			} else if !configured.Changed {
				step.Status = "already configured"
			}
			if opts.DryRun && configured.Changed {
				step.Status = "would configure"
			}
		}
		if configureErr != nil {
			step.Status = "failed"
			step.Message = configureErr.Error()
			configurationOK = false
		}
		report.Steps = append(report.Steps, step)
		if configureErr == nil && !opts.DryRun {
			initialized, initErr := s.Init(ctx)
			step = cli.SetupStep{Name: "corpus", Status: "initialized"}
			if initialized != nil {
				step.Path = initialized.Path
				step.Message = initialized.Message
			}
			if initErr != nil {
				step.Status = "failed"
				step.Message = initErr.Error()
				configurationOK = false
			}
			report.Steps = append(report.Steps, step)
		} else if opts.DryRun {
			report.Steps = append(report.Steps, cli.SetupStep{Name: "corpus", Status: "would initialize"})
		}
	}
	if !opts.SkipMCP && !opts.DryRun && configurationOK {
		// Client registration happens only after shared configuration and corpus
		// initialization succeed. This prevents a client from pointing at setup
		// state that could not be initialized.
		clientOptions.DryRun = false
		clientReport, err = clientsetup.Run(clientOptions)
		if err != nil {
			return nil, err
		}
	}
	if !opts.SkipMCP {
		for _, result := range clientReport.Results {
			report.Steps = append(report.Steps, cli.SetupStep{Name: string(result.Client), Path: result.Path, Status: result.Status, Message: result.Error})
		}
	}
	if operation == clientsetup.Configure && strings.TrimSpace(opts.Repository) != "" {
		ref, parseErr := setupRepoRef(opts.Repository)
		step := cli.SetupStep{Name: "repository", Status: "added", Message: opts.Repository}
		if parseErr != nil {
			step.Status = "failed"
			step.Message = parseErr.Error()
		} else if opts.DryRun {
			step.Status = "would add"
		} else {
			name := setupSourceName(ref)
			_, addErr := s.AddRepoSource(ctx, name, []cli.RepoRef{ref})
			if addErr != nil {
				step.Status = "failed"
				step.Message = addErr.Error()
			}
		}
		report.Steps = append(report.Steps, step)
	}
	if operation == clientsetup.Configure && !opts.DryRun {
		diagnostics, doctorErr := s.Doctor(ctx)
		step := cli.SetupStep{Name: "verification", Status: "verified"}
		if doctorErr != nil || diagnostics == nil || !diagnostics.Healthy {
			step.Status = "failed"
			if doctorErr != nil {
				step.Message = doctorErr.Error()
			} else {
				step.Message = "required installation checks failed"
			}
		} else {
			warnings := 0
			for _, check := range diagnostics.Checks {
				if check.Status == "warning" {
					warnings++
				}
			}
			if warnings > 0 {
				step.Message = fmt.Sprintf("verified with %d optional warning(s); run gitcontribute doctor for details", warnings)
			}
		}
		report.Steps = append(report.Steps, step)
	}
	return report, nil
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
