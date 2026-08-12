package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/deepwiki"
	"github.com/morluto/gitcontribute/internal/discovery"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/dossier"
	"github.com/morluto/gitcontribute/internal/github"
)

// Service is the product-owned application layer that satisfies
// contracts.Service.
// MCP reads are exposed through MCPReader.
var (
	_ contracts.Service         = (*Service)(nil)
	_ contracts.WorkflowService = (*Service)(nil)
	_ contracts.DossierService  = (*Service)(nil)
)

type Service struct {
	mu              sync.Mutex
	cfg             *config.Config
	paths           *config.Paths
	corpus          *corpus.Corpus
	readCorpus      *corpus.Corpus
	jobs            *JobExecutor
	feedbackWriter  workflowGate
	ghReader        github.Reader
	archiveFetcher  discovery.ArchiveFetcher
	deepWikiReader  deepwiki.Reader
	clock           func() time.Time
	executable      func() (string, error)
	upgradeEnv      upgradeEnvironment
	version         string
	logger          *slog.Logger
	lifecycleCtx    context.Context
	cancelLifecycle context.CancelFunc
}

func (s *Service) acquireFeedbackWorkflow(ctx context.Context) (func(), error) {
	return s.feedbackWriter.acquire(ctx)
}

// New creates a Service and resolves local configuration. GitHub credentials
// are resolved lazily only when a network-reading operation is requested.
func New(paths *config.Paths, version string, logger *slog.Logger) (*Service, error) {
	// Library callers that do not provide a process lifecycle still receive an
	// explicit service lifetime bounded by Close.
	return NewWithContext(context.Background(), paths, version, logger)
}

// NewWithContext creates a Service bounded by ctx and Close.
func NewWithContext(ctx context.Context, paths *config.Paths, version string, logger *slog.Logger) (*Service, error) {
	if paths == nil {
		paths = config.NewPaths(nil)
	}
	lifecycleCtx, cancelLifecycle := context.WithCancel(ctx)
	s := &Service{
		paths: paths, version: version, clock: time.Now, executable: os.Executable, logger: logger,
		upgradeEnv:   productionUpgradeEnvironment(),
		lifecycleCtx: lifecycleCtx, cancelLifecycle: cancelLifecycle,
	}
	if _, err := s.loadConfig(); err != nil {
		cancelLifecycle()
		return nil, err
	}
	return s, nil
}

func (s *Service) now() time.Time {
	s.mu.Lock()
	clock := s.clock
	s.mu.Unlock()
	if clock == nil {
		return time.Now()
	}
	return clock()
}

func (s *Service) deepWiki() deepwiki.Reader {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deepWikiReader == nil {
		s.deepWikiReader = &deepwiki.Client{}
	}
	return s.deepWikiReader
}

func (s *Service) getArchiveFetcher() discovery.ArchiveFetcher {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.archiveFetcher != nil {
		return s.archiveFetcher
	}
	s.archiveFetcher = discovery.NewArchiveClient()
	return s.archiveFetcher
}

// Close cancels and waits for active jobs, then closes the corpus database connection.
func (s *Service) Close() error {
	s.mu.Lock()
	jobs := s.jobs
	c := s.corpus
	readCorpus := s.readCorpus
	s.jobs = nil
	s.corpus = nil
	s.readCorpus = nil
	s.mu.Unlock()
	if s.cancelLifecycle != nil {
		s.cancelLifecycle()
	}
	var closeErr error
	if jobs != nil {
		closeErr = jobs.Close()
	}
	if c != nil {
		closeErr = errors.Join(closeErr, c.Close())
	}
	if readCorpus != nil {
		closeErr = errors.Join(closeErr, readCorpus.Close())
	}
	return closeErr
}

type configSource uint8

const (
	defaultConfig configSource = iota
	storedConfig
)

type loadedConfig struct {
	value  *config.Config
	path   string
	source configSource
}

func (s *Service) readConfig() (loadedConfig, error) {
	cfgFile, err := s.paths.ConfigFile()
	if err != nil {
		return loadedConfig{}, err
	}
	var cfg *config.Config
	source := defaultConfig
	if _, err := os.Stat(cfgFile); err == nil {
		cfg, err = config.LoadFile(cfgFile)
		if err != nil {
			return loadedConfig{}, fmt.Errorf("load config: %w", err)
		}
		source = storedConfig
	} else if errors.Is(err, os.ErrNotExist) {
		cfg = config.Default()
	} else {
		return loadedConfig{}, fmt.Errorf("inspect config: %w", err)
	}
	if err := config.ApplyDefaults(cfg, s.paths); err != nil {
		return loadedConfig{}, err
	}
	if err := config.ApplyEnv(cfg, os.Getenv); err != nil {
		return loadedConfig{}, err
	}
	if err := config.Validate(cfg); err != nil {
		return loadedConfig{}, fmt.Errorf("validate config: %w", err)
	}
	return loadedConfig{value: cfg, path: cfgFile, source: source}, nil
}

func (s *Service) cacheConfig(cfg *config.Config) {
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
}

func (s *Service) loadConfig() (*config.Config, error) {
	loaded, err := s.readConfig()
	if err != nil {
		return nil, err
	}
	s.cacheConfig(loaded.value)
	return loaded.value, nil
}

func (s *Service) loadConfigForInitialization() (*config.Config, error) {
	loaded, err := s.readConfig()
	if err != nil {
		return nil, err
	}
	if loaded.source == defaultConfig {
		if err := config.Save(loaded.path, loaded.value); err != nil {
			return nil, fmt.Errorf("save config: %w", err)
		}
	}
	s.cacheConfig(loaded.value)
	return loaded.value, nil
}

func (s *Service) openCorpus(ctx context.Context) (*corpus.Corpus, error) {
	s.mu.Lock()
	if s.corpus != nil {
		c := s.corpus
		s.mu.Unlock()
		return c, nil
	}
	s.mu.Unlock()
	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	if cfg.Database == "" {
		return nil, errors.New("database path not configured")
	}
	if err := ensureDatabaseDir(cfg.Database); err != nil {
		return nil, err
	}
	inspection, err := corpus.InspectSchema(ctx, cfg.Database)
	if err != nil {
		return nil, err
	}
	if inspection.Exists() {
		switch inspection.State {
		case corpus.SchemaMigrationRequired:
			return nil, &corpus.MigrationRequiredError{Current: inspection.Current, Target: inspection.Target}
		case corpus.SchemaNewer:
			return nil, &corpus.UnsupportedSchemaError{Current: inspection.Current, Target: inspection.Target}
		case corpus.SchemaIncompatible:
			return nil, &corpus.IncompatibleSchemaError{Current: inspection.Current, Target: inspection.Target}
		}
	}
	c, err := corpus.Open(ctx, cfg.Database)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.corpus != nil {
		existing := s.corpus
		s.mu.Unlock()
		_ = c.Close()
		return existing, nil
	}
	s.corpus = c
	s.mu.Unlock()
	return c, nil
}

// openReadOnlyCorpus opens the configured corpus without creating or
// migrating it. Read-facing application capabilities use this path so an
// offline read never implies schema-migration authority.
func (s *Service) openReadOnlyCorpus(ctx context.Context) (*corpus.Corpus, error) {
	s.mu.Lock()
	if s.readCorpus != nil {
		c := s.readCorpus
		s.mu.Unlock()
		return c, nil
	}
	s.mu.Unlock()
	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	if cfg.Database == "" {
		return nil, errors.New("database path not configured")
	}
	c, err := corpus.OpenReadOnly(ctx, cfg.Database)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.readCorpus != nil {
		existing := s.readCorpus
		s.mu.Unlock()
		if err := c.Close(); err != nil {
			return nil, fmt.Errorf("close duplicate read-only corpus: %w", err)
		}
		return existing, nil
	}
	s.readCorpus = c
	s.mu.Unlock()
	return c, nil
}

// Jobs returns the durable job executor, opening the corpus if needed.
func (s *Service) Jobs(ctx context.Context) (*JobExecutor, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.jobs != nil {
		return s.jobs, nil
	}
	jobConfig := defaultJobExecutorConfig()
	if s.cfg != nil && s.cfg.Crawl.Concurrency > 0 {
		jobConfig.maxConcurrentJobs = int64(s.cfg.Crawl.Concurrency)
	}
	// Jobs outlive this request and remain bounded by the service lifecycle.
	//nolint:contextcheck
	jobs, err := newJobExecutorWithConfig(s.lifecycleCtx, c, jobConfig)
	if err != nil {
		return nil, err
	}
	s.jobs = jobs
	return jobs, nil
}

func ensurePrivateDir(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	return os.Chmod(path, 0700)
}

func ensureDatabaseDir(database string) error {
	if database == ":memory:" || strings.HasPrefix(database, "file:") {
		return nil
	}
	dir := filepath.Dir(database)
	info, err := os.Stat(dir)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("database parent %q is not a directory", dir)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.MkdirAll(dir, 0700)
}

func (s *Service) newGitHubReader() (github.Reader, error) {
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	if cfg == nil {
		return nil, errors.New("configuration is not loaded")
	}
	tokenSrc := tokenSource(cfg)
	retry := github.DefaultRetryConfig()
	retry.MaxAttempts = cfg.Crawl.RetryLimit + 1
	retry.OnAttempt = func(observation github.RetryObservation) {
		s.mu.Lock()
		c := s.corpus
		s.mu.Unlock()
		if c == nil {
			return
		}
		obsCtx := observation.Context
		if obsCtx == nil {
			obsCtx = s.lifecycleCtx
		}
		ctx, cancel := context.WithTimeout(obsCtx, 2*time.Second)
		defer cancel()
		_ = c.RecordRateLimitObservation(ctx, corpus.RateLimitObservation{
			Attempt: observation.Attempt, StatusCode: observation.StatusCode,
			Resource: observation.RateLimit.Resource, Limit: observation.RateLimit.Limit,
			Remaining: observation.RateLimit.Remaining, Used: observation.RateLimit.Used,
			ResetAt: observation.RateLimit.Reset, Delay: observation.Delay,
			APIVersion: observation.APIVersion, SourceURL: observation.SourceURL, ObservedAt: s.now(),
		})
	}
	timeout, err := time.ParseDuration(cfg.Crawl.Timeout)
	if err != nil {
		return nil, fmt.Errorf("parse GitHub request timeout: %w", err)
	}
	client, err := github.NewClient(github.Config{
		TokenSource: tokenSrc,
		Retry:       retry,
		HTTPClient:  &http.Client{Timeout: timeout},
	})
	if err != nil {
		return nil, fmt.Errorf("create github reader: %w", err)
	}
	return client, nil
}

func tokenSource(cfg *config.Config) github.TokenSource {
	switch cfg.TokenSource.Method {
	case config.TokenSourceEnv:
		name := cfg.TokenSource.Key
		if name == "" {
			name = github.DefaultEnvToken
		}
		return github.RequireToken(github.EnvTokenSource(name))
	case config.TokenSourceGHCLI:
		return github.RequireToken(github.GhCLITokenSource())
	case config.TokenSourceKeyring:
		return github.RequireToken(github.KeyringTokenSource(cfg.TokenSource.Key))
	}
	return github.StaticTokenSource("")
}

func (s *Service) githubReader() (github.Reader, error) {
	s.mu.Lock()
	if s.ghReader != nil {
		reader := s.ghReader
		s.mu.Unlock()
		return reader, nil
	}
	s.mu.Unlock()
	reader, err := s.newGitHubReader()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.ghReader == nil {
		s.ghReader = reader
	}
	reader = s.ghReader
	s.mu.Unlock()
	return reader, nil
}

func (s *Service) databasePath() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg == nil {
		return ""
	}
	return s.cfg.Database
}

// Init opens or creates the configured corpus and persists a default
// configuration if one does not already exist.
func (s *Service) Init(ctx context.Context) (*contracts.InitResult, error) {
	cfg, err := s.loadConfigForInitialization()
	if err != nil {
		return nil, err
	}
	if err := ensureDatabaseDir(cfg.Database); err != nil {
		return nil, err
	}
	inspection, err := corpus.InspectSchema(ctx, cfg.Database)
	if err != nil {
		return nil, err
	}
	if inspection.Exists() {
		switch inspection.State {
		case corpus.SchemaMigrationRequired:
			return nil, &corpus.MigrationRequiredError{Current: inspection.Current, Target: inspection.Target}
		case corpus.SchemaNewer:
			return nil, &corpus.UnsupportedSchemaError{Current: inspection.Current, Target: inspection.Target}
		case corpus.SchemaIncompatible:
			return nil, &corpus.IncompatibleSchemaError{Current: inspection.Current, Target: inspection.Target}
		}
	}
	_, err = s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	return &contracts.InitResult{Path: cfg.Database, Message: "corpus initialized"}, nil
}

// Status reports whether the corpus is healthy and counts local records.
func (s *Service) Status(ctx context.Context) (*contracts.StatusResult, error) {
	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return &contracts.StatusResult{Healthy: false, Corpus: s.databasePath(), Version: s.version, Message: err.Error()}, nil
	}
	st, err := c.Status(ctx)
	if err != nil {
		return &contracts.StatusResult{Healthy: false, Corpus: s.databasePath(), Version: s.version, Message: err.Error()}, nil
	}
	return &contracts.StatusResult{
		Healthy: true,
		Corpus:  s.databasePath(),
		Version: s.version,
		Message: fmt.Sprintf("%d repositories, %d threads", st.Repositories, st.Threads),
	}, nil
}

const (
	defaultSyncMaxRequests = 100
	maxSyncRequests        = 1000
)

type syncRequestBudget struct {
	limit int
	used  int
}

func newSyncRequestBudget(limit int) *syncRequestBudget {
	return &syncRequestBudget{limit: limit}
}

func (b *syncRequestBudget) available() bool {
	return b != nil && b.used < b.limit
}

func (b *syncRequestBudget) take() error {
	if b == nil {
		return errors.New("GitHub request budget is required")
	}
	if !b.available() {
		return fmt.Errorf("GitHub request budget of %d exhausted", b.limit)
	}
	b.used++
	return nil
}

func threadFromIssue(issue github.Issue) (corpus.Thread, string, error) {
	kind, err := domain.ParseThreadKind(string(issue.Kind))
	if err != nil {
		return corpus.Thread{}, "", err
	}
	state, err := domain.ParseThreadState(issue.State)
	if err != nil {
		return corpus.Thread{}, "", err
	}
	thread := corpus.Thread{
		Kind:              kind,
		Number:            issue.Number,
		State:             state,
		StateReason:       issue.StateReason,
		Title:             issue.Title,
		Body:              issue.Body,
		Author:            issue.Author,
		AuthorAssociation: issue.AuthorAssociation,
		Labels:            issue.Labels,
		Assignees:         issue.Assignees,
		Draft:             issue.Draft,
		Locked:            issue.Locked,
		Milestone:         issue.Milestone,
		SourceCreatedAt:   issue.CreatedAt,
		SourceUpdatedAt:   issue.UpdatedAt,
	}
	if issue.ClosedAt != nil {
		thread.ClosedAt = *issue.ClosedAt
	}

	payload, err := json.Marshal(issue)
	if err != nil {
		return corpus.Thread{}, "", fmt.Errorf("marshal issue: %w", err)
	}

	return thread, string(payload), nil
}

func corpusRepoFromGitHub(r github.Repository) corpus.Repository {
	return corpus.Repository{
		Owner:           r.Owner,
		Name:            r.Name,
		ExternalID:      r.NodeID,
		Description:     r.Description,
		DefaultBranch:   r.DefaultBranch,
		Language:        r.Language,
		License:         r.License,
		Topics:          r.Topics,
		Stars:           r.Stars,
		Watchers:        r.Watchers,
		Forks:           r.Forks,
		OpenIssues:      r.OpenIssues,
		Archived:        r.Archived,
		Fork:            r.Fork,
		SourceCreatedAt: r.CreatedAt,
		SourceUpdatedAt: r.UpdatedAt,
	}
}

// Dossier builds a deterministic, local-corpus-backed repository dossier.
func (s *Service) Dossier(ctx context.Context, repo contracts.RepoRef) (*contracts.DossierResult, error) {
	ref, err := domain.NewRepoRef(repo.Owner, repo.Repo)
	if err != nil {
		return nil, err
	}
	if _, err := s.openReadOnlyCorpus(ctx); err != nil {
		return nil, err
	}
	d, err := s.buildDossier(ctx, ref)
	if err != nil {
		return nil, err
	}
	return &contracts.DossierResult{
		Repo:       repo,
		Summary:    d.Repository.Description,
		Language:   firstLanguage(d.Repository.Languages),
		Stars:      d.Repository.Stars,
		OpenIssues: d.Repository.OpenIssueCount,
		Coverage:   coverageNames(d.Coverage),
		Freshness:  d.AsOf.Format(time.RFC3339),
	}, nil
}

func (s *Service) buildDossier(ctx context.Context, ref domain.RepoRef) (*domain.Dossier, error) {
	reader := &corpusReader{s: s}
	builder := dossier.NewBuilder(reader, dossier.DefaultRecentLimit)
	return builder.Build(ctx, ref)
}
