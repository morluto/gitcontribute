package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/config"
)

// Metadata reports deterministic application and local capability metadata.
// It neither opens the corpus nor performs network access.
func (s *Service) Metadata(ctx context.Context) (*cli.MetadataResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cfg, err := s.loadConfig(false)
	if err != nil {
		return nil, err
	}
	configPath, err := s.paths.ConfigFile()
	if err != nil {
		return nil, err
	}

	schemaVersion := int64(0)
	s.mu.Lock()
	c := s.corpus
	s.mu.Unlock()
	if c != nil {
		schemaVersion, _ = c.SchemaVersion(ctx)
	}

	capabilities := []string{
		"archive", "clustering", "collections", "dossiers", "evidence",
		"github-read", "investigations", "local-search", "mcp-stdio",
		"validation", "workspaces",
	}
	sort.Strings(capabilities)
	return &cli.MetadataResult{
		Name:          "gitcontribute",
		Version:       s.version,
		GoVersion:     runtime.Version(),
		OS:            runtime.GOOS,
		Architecture:  runtime.GOARCH,
		SchemaVersion: schemaVersion,
		ConfigPath:    configPath,
		CorpusPath:    cfg.Database,
		Capabilities:  capabilities,
		Features: map[string]bool{
			"github_mutations": false,
			"mcp_stdio":        true,
			"semantic_search":  false,
			"validation_exec":  true,
		},
	}, nil
}

// Configure validates and atomically saves supported typed settings. Runtime
// environment overrides are deliberately not persisted.
func (s *Service) Configure(ctx context.Context, opts cli.ConfigureOptions) (*cli.ConfigureResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := s.paths.ConfigFile()
	if err != nil {
		return nil, err
	}
	cfg, err := s.persistedConfig(path)
	if err != nil {
		return nil, err
	}
	before := *cfg
	applyConfigureOptions(cfg, opts)
	if err := config.Validate(cfg); err != nil {
		return nil, fmt.Errorf("validate configuration: %w", err)
	}
	changed := !reflect.DeepEqual(before, *cfg)

	if changed && !opts.DryRun {
		s.mu.Lock()
		corpusOpen := s.corpus != nil
		s.mu.Unlock()
		if corpusOpen && before.Database != cfg.Database {
			return nil, errors.New("cannot change database path while the corpus is open")
		}
		if err := config.Save(path, cfg); err != nil {
			return nil, err
		}
		if _, err := s.loadConfig(false); err != nil {
			return nil, fmt.Errorf("reload configuration: %w", err)
		}
	}
	return &cli.ConfigureResult{Path: path, DryRun: opts.DryRun, Changed: changed, Config: configResult(cfg)}, nil
}

// ControlStatus returns local corpus counts and freshness without network
// access or implicit hydration.
func (s *Service) ControlStatus(ctx context.Context) (*cli.ControlStatusResult, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	stats, err := c.ControlStats(ctx, s.now())
	if err != nil {
		return nil, err
	}
	version, err := c.SchemaVersion(ctx)
	if err != nil {
		return nil, err
	}
	warnings := make([]string, 0, 4)
	if stats.Repositories == 0 {
		warnings = append(warnings, "corpus has no repositories")
	}
	if stats.FrontierReady > 0 {
		warnings = append(warnings, fmt.Sprintf("%d frontier items are ready", stats.FrontierReady))
	}
	if stats.ActiveRuns > 0 || stats.ActiveJobs > 0 {
		warnings = append(warnings, "background work is active")
	}
	if !stats.Freshest.IsZero() && s.now().Sub(stats.Freshest) > 7*24*time.Hour {
		warnings = append(warnings, "freshest GitHub observation is older than 7 days")
	}
	return &cli.ControlStatusResult{
		Healthy:       true,
		Corpus:        s.databasePath(),
		Version:       s.version,
		SchemaVersion: version,
		Counts: cli.ControlCounts{
			Repositories:  stats.Repositories,
			Threads:       stats.Threads,
			Sources:       stats.Sources,
			FrontierReady: stats.FrontierReady,
			ActiveRuns:    stats.ActiveRuns,
			ActiveJobs:    stats.ActiveJobs,
		},
		FreshestSource: formatTime(stats.Freshest),
		Warnings:       warnings,
	}, nil
}

// Doctor performs bounded local diagnostics. It reports authentication source
// availability but never returns credential values or command output.
func (s *Service) Doctor(ctx context.Context) (*cli.DoctorResult, error) {
	checks := make([]cli.DoctorCheck, 0, 7)
	add := func(name string, required bool, err error, success string) {
		check := cli.DoctorCheck{Name: name, Required: required, Status: "ok", Message: success}
		if err != nil {
			check.Status = "error"
			if !required {
				check.Status = "warning"
			}
			check.Message = redactDiagnostic(err.Error())
		}
		checks = append(checks, check)
	}

	path, pathErr := s.paths.ConfigFile()
	var cfg *config.Config
	if pathErr == nil {
		cfg, pathErr = s.persistedConfig(path)
	}
	add("config", true, pathErr, "configuration is readable and valid")

	c, dbErr := s.openCorpus(ctx)
	add("database", true, dbErr, "corpus is readable")
	if dbErr == nil {
		_, schemaErr := c.SchemaVersion(ctx)
		add("schema", true, schemaErr, "schema is current")
		lockCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		lockErr := c.CheckIntegrity(lockCtx)
		cancel()
		add("database_lock", true, lockErr, "integrity and write lock checks passed")
	}

	gitErr := commandAvailable(ctx, "git", "--version")
	add("git", true, gitErr, "Git is available")

	var authErr error
	if cfg == nil {
		authErr = errors.New("authentication source unavailable because configuration is invalid")
	} else {
		authErr = checkAuthSource(ctx, cfg)
	}
	add("github_auth", false, authErr, "GitHub authentication source is available")

	add("rg", false, lookPathError("rg"), "ripgrep is available")

	healthy := true
	for _, check := range checks {
		if check.Required && check.Status == "error" {
			healthy = false
			break
		}
	}
	return &cli.DoctorResult{Healthy: healthy, Checks: checks}, nil
}

func (s *Service) persistedConfig(path string) (*config.Config, error) {
	var cfg *config.Config
	if _, err := os.Stat(path); err == nil {
		loaded, err := config.LoadFile(path)
		if err != nil {
			return nil, fmt.Errorf("load config: %w", err)
		}
		cfg = loaded
	} else if errors.Is(err, os.ErrNotExist) {
		cfg = config.Default()
	} else {
		return nil, fmt.Errorf("inspect config: %w", err)
	}
	if err := config.ApplyDefaults(cfg, s.paths); err != nil {
		return nil, err
	}
	if err := config.Validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func applyConfigureOptions(cfg *config.Config, opts cli.ConfigureOptions) {
	if opts.Database != nil {
		cfg.Database = strings.TrimSpace(*opts.Database)
	}
	if opts.TokenSource != nil {
		cfg.TokenSource.Method = strings.ToLower(strings.TrimSpace(*opts.TokenSource))
	}
	if opts.TokenSourceKey != nil {
		cfg.TokenSource.Key = strings.TrimSpace(*opts.TokenSourceKey)
	}
	if opts.CrawlBudget != nil {
		cfg.Crawl.Budget = *opts.CrawlBudget
	}
	if opts.CrawlConcurrency != nil {
		cfg.Crawl.Concurrency = *opts.CrawlConcurrency
	}
	if opts.CrawlRetryLimit != nil {
		cfg.Crawl.RetryLimit = *opts.CrawlRetryLimit
	}
	if opts.CrawlTimeout != nil {
		cfg.Crawl.Timeout = strings.TrimSpace(*opts.CrawlTimeout)
	}
	if opts.OutputFormat != nil {
		cfg.Output.Format = strings.ToLower(strings.TrimSpace(*opts.OutputFormat))
	}
	if opts.OutputMaxResults != nil {
		cfg.Output.MaxResults = *opts.OutputMaxResults
	}
}

func configResult(cfg *config.Config) cli.ConfigResult {
	return cli.ConfigResult{
		Database:         cfg.Database,
		TokenSource:      cfg.TokenSource.Method,
		TokenSourceKey:   cfg.TokenSource.Key,
		CrawlBudget:      cfg.Crawl.Budget,
		CrawlConcurrency: cfg.Crawl.Concurrency,
		CrawlRetryLimit:  cfg.Crawl.RetryLimit,
		CrawlTimeout:     cfg.Crawl.Timeout,
		OutputFormat:     cfg.Output.Format,
		OutputMaxResults: cfg.Output.MaxResults,
	}
}

func commandAvailable(ctx context.Context, name string, args ...string) error {
	path, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("%s is not available", name)
	}
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, path, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s check failed", name)
	}
	return nil
}

func lookPathError(name string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("%s is not available", name)
	}
	return nil
}

func checkAuthSource(ctx context.Context, cfg *config.Config) error {
	switch cfg.TokenSource.Method {
	case "none":
		return errors.New("no GitHub authentication source configured; public reads remain available")
	case "env":
		if value, ok := os.LookupEnv(cfg.TokenSource.Key); !ok || strings.TrimSpace(value) == "" {
			return fmt.Errorf("configured token environment variable %s is unset", cfg.TokenSource.Key)
		}
		return nil
	case "gh-cli":
		return commandAvailable(ctx, "gh", "auth", "status")
	case "keyring":
		return nil
	default:
		return errors.New("unsupported GitHub authentication source")
	}
}

func redactDiagnostic(message string) string {
	upper := strings.ToUpper(message)
	for _, marker := range []string{"TOKEN=", "AUTHORIZATION:", "BEARER "} {
		if strings.Contains(upper, marker) {
			return "diagnostic failed; sensitive credential detail was redacted"
		}
	}
	return message
}
