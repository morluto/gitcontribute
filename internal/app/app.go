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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/deepwiki"
	"github.com/morluto/gitcontribute/internal/discovery"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/dossier"
	"github.com/morluto/gitcontribute/internal/github"
)

// Service is the product-owned application layer that satisfies cli.Service.
// MCP reads are exposed through MCPReader.
type Service struct {
	mu              sync.Mutex
	cfg             *config.Config
	paths           *config.Paths
	corpus          *corpus.Corpus
	readCorpus      *corpus.Corpus
	jobs            *JobExecutor
	ghReader        github.Reader
	archiveFetcher  discovery.ArchiveFetcher
	deepWikiReader  deepwiki.Reader
	clock           func() time.Time
	version         string
	logger          *slog.Logger
	lifecycleCtx    context.Context
	cancelLifecycle context.CancelFunc
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
		paths: paths, version: version, clock: time.Now, logger: logger,
		lifecycleCtx: lifecycleCtx, cancelLifecycle: cancelLifecycle,
	}
	if _, err := s.loadConfig(false); err != nil {
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

// SetClock overrides the time source. It is intended for tests.
func (s *Service) SetClock(clock func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clock = clock
}

// SetGitHubReader overrides the GitHub reader. It is intended for tests.
func (s *Service) SetGitHubReader(r github.Reader) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ghReader = r
}

// SetDeepWikiReader overrides the derived external knowledge reader. It is
// intended for tests and embedding.
func (s *Service) SetDeepWikiReader(r deepwiki.Reader) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deepWikiReader = r
}

func (s *Service) deepWiki() deepwiki.Reader {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deepWikiReader == nil {
		s.deepWikiReader = &deepwiki.Client{}
	}
	return s.deepWikiReader
}

// SetArchiveFetcher overrides the GH Archive fetcher. It is intended for tests.
func (s *Service) SetArchiveFetcher(f discovery.ArchiveFetcher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.archiveFetcher = f
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

func (s *Service) loadConfig(save bool) (*config.Config, error) {
	cfgFile, err := s.paths.ConfigFile()
	if err != nil {
		return nil, err
	}
	var cfg *config.Config
	exists := false
	if _, err := os.Stat(cfgFile); err == nil {
		cfg, err = config.LoadFile(cfgFile)
		if err != nil {
			return nil, fmt.Errorf("load config: %w", err)
		}
		exists = true
	} else if errors.Is(err, os.ErrNotExist) {
		cfg = config.Default()
	} else {
		return nil, fmt.Errorf("inspect config: %w", err)
	}
	if err := config.ApplyDefaults(cfg, s.paths); err != nil {
		return nil, err
	}
	if err := config.ApplyEnv(cfg, os.Getenv); err != nil {
		return nil, err
	}
	if err := config.Validate(cfg); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}
	if save && !exists {
		dir := filepath.Dir(cfgFile)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
		if err := config.Save(cfgFile, cfg); err != nil {
			return nil, fmt.Errorf("save config: %w", err)
		}
	}
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	return cfg, nil
}

func (s *Service) openCorpus(ctx context.Context) (*corpus.Corpus, error) {
	s.mu.Lock()
	if s.corpus != nil {
		c := s.corpus
		s.mu.Unlock()
		return c, nil
	}
	s.mu.Unlock()
	cfg, err := s.loadConfig(false)
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
	if inspection.Exists {
		switch inspection.State {
		case corpus.SchemaMigrationRequired:
			return nil, &corpus.MigrationRequiredError{Current: inspection.Current, Target: inspection.Target}
		case corpus.SchemaNewer:
			return nil, &corpus.UnsupportedSchemaError{Current: inspection.Current, Target: inspection.Target}
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
	cfg, err := s.loadConfig(false)
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
	// Jobs outlive this request and remain bounded by the service lifecycle.
	//nolint:contextcheck
	jobs, err := newJobExecutor(s.lifecycleCtx, c)
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
	return ensurePrivateDir(filepath.Dir(database))
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
	method := strings.ToLower(cfg.TokenSource.Method)
	switch method {
	case "env":
		name := cfg.TokenSource.Key
		if name == "" {
			name = github.DefaultEnvToken
		}
		return github.RequireToken(github.EnvTokenSource(name))
	case "gh-cli":
		return github.RequireToken(github.GhCLITokenSource(nil))
	case "keyring":
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
func (s *Service) Init(ctx context.Context) (*cli.InitResult, error) {
	cfg, err := s.loadConfig(true)
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
	if inspection.Exists {
		switch inspection.State {
		case corpus.SchemaMigrationRequired:
			return nil, &corpus.MigrationRequiredError{Current: inspection.Current, Target: inspection.Target}
		case corpus.SchemaNewer:
			return nil, &corpus.UnsupportedSchemaError{Current: inspection.Current, Target: inspection.Target}
		}
	}
	_, err = s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	return &cli.InitResult{Path: cfg.Database, Message: "corpus initialized"}, nil
}

// Status reports whether the corpus is healthy and counts local records.
func (s *Service) Status(ctx context.Context) (*cli.StatusResult, error) {
	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return &cli.StatusResult{Healthy: false, Corpus: s.databasePath(), Version: s.version, Message: err.Error()}, nil
	}
	st, err := c.Status(ctx)
	if err != nil {
		return &cli.StatusResult{Healthy: false, Corpus: s.databasePath(), Version: s.version, Message: err.Error()}, nil
	}
	return &cli.StatusResult{
		Healthy: true,
		Corpus:  s.databasePath(),
		Version: s.version,
		Message: fmt.Sprintf("%d repositories, %d threads", st.Repositories, st.Threads),
	}, nil
}

// SyncOptions bounds and filters an explicit repository synchronization.
type SyncOptions struct {
	Kind        string
	State       string
	Since       time.Time
	Numbers     []int
	MaxPages    int
	MaxRequests int
}

const (
	defaultSyncMaxRequests = 100
	maxSyncRequests        = 1000
)

// Sync fetches all repository threads. It preserves the original archive API
// while SyncWithOptions provides bounded incremental and exact refreshes.
func (s *Service) Sync(ctx context.Context, repo cli.RepoRef) (*cli.SyncResult, error) {
	return s.SyncWithOptions(ctx, repo, SyncOptions{})
}

// SyncWithOptions fetches a repository and a bounded thread selection from
// GitHub, then writes ordered observations to the local corpus.
func (s *Service) SyncWithOptions(ctx context.Context, repo cli.RepoRef, syncOpts SyncOptions) (*cli.SyncResult, error) {
	return s.syncWithOptions(ctx, repo, syncOpts, nil)
}

func (s *Service) syncProvidedThreadHeaders(ctx context.Context, repo cli.RepoRef, issues []github.Issue, maxRequests int) (*cli.SyncResult, error) {
	return s.syncWithOptions(ctx, repo, SyncOptions{Kind: "pull_request", State: "all", MaxPages: 1, MaxRequests: maxRequests}, issues)
}

func (s *Service) syncWithOptions(ctx context.Context, repo cli.RepoRef, syncOpts SyncOptions, provided []github.Issue) (*cli.SyncResult, error) {
	ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Repo}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	var plan syncRequestPlan
	var err error
	syncOpts, plan, err = planSyncOptions(syncOpts)
	if err != nil {
		return nil, err
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	budget := newSyncRequestBudget(syncOpts.MaxRequests)
	reader, err := s.githubReader()
	if err != nil {
		return nil, err
	}

	run, err := c.StartRun(ctx, "sync")
	if err != nil {
		return nil, err
	}
	var syncErr error
	defer func() {
		if syncErr != nil {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = c.FailRun(cleanupCtx, run.ID, syncErr.Error())
		}
	}()

	if err := c.RecordRunEvent(ctx, run.ID, "info", fmt.Sprintf("syncing %s", ref)); err != nil {
		syncErr = err
		return nil, syncErr
	}

	repoProjection, lastSourceUpdated, err := syncRepositoryHeader(ctx, c, reader, ref, run.ID, budget)
	if err != nil {
		syncErr = err
		return nil, syncErr
	}

	selection, err := syncThreadHeaderSelection(ctx, c, reader, ref, repoProjection.ID, lastSourceUpdated, syncOpts, provided, budget)
	if err != nil {
		syncErr = err
		return nil, syncErr
	}
	updated, pages := selection.updated, selection.requests
	lastSourceUpdated, complete, requestCapped := selection.sourceUpdatedAt, selection.complete, selection.requestCapped

	if err := c.AdvanceFacet(ctx, repoProjection.ID, nil, "threads", lastSourceUpdated, complete, run.ID); err != nil {
		syncErr = fmt.Errorf("advance threads facet: %w", err)
		return nil, syncErr
	}

	stats, err := json.Marshal(map[string]any{
		"pages": pages, "threads": updated, "complete": complete, "requests": budget.used,
		"request_budget": budget.limit, "request_capped": requestCapped,
	})
	if err != nil {
		syncErr = fmt.Errorf("marshal sync statistics: %w", err)
		return nil, syncErr
	}
	if err := c.FinishRun(ctx, run.ID, string(stats)); err != nil {
		syncErr = err
		return nil, syncErr
	}

	return &cli.SyncResult{
		Repo: repo, Updated: updated, Requests: budget.used, PlannedRequests: plan.plannedRequests, RequestBudget: syncOpts.MaxRequests, Capped: requestCapped,
		Message: fmt.Sprintf("fetched %d thread headers across %d thread requests", updated, pages),
	}, nil
}

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

func syncFixedRequestCost() int {
	return 1 + len(contributionGuidancePaths)
}

type syncRequestPlan struct {
	fixedRequests        int
	threadRequestCeiling int
	plannedRequests      int
}

func planSyncOptions(opts SyncOptions) (SyncOptions, syncRequestPlan, error) {
	normalized, err := normalizeSyncOptions(opts)
	if err != nil {
		return SyncOptions{}, syncRequestPlan{}, err
	}
	fixed := syncFixedRequestCost()
	remaining := normalized.MaxRequests - fixed
	threadCeiling := normalized.MaxPages
	if len(normalized.Numbers) > 0 {
		threadCeiling = len(normalized.Numbers)
		if threadCeiling > remaining {
			required := fixed + threadCeiling
			return SyncOptions{}, syncRequestPlan{}, fmt.Errorf("exact thread selection requires at least %d requests; max requests is %d", required, normalized.MaxRequests)
		}
	} else if threadCeiling > remaining {
		threadCeiling = remaining
	}
	return normalized, syncRequestPlan{
		fixedRequests: fixed, threadRequestCeiling: threadCeiling, plannedRequests: fixed + threadCeiling,
	}, nil
}

func normalizeSyncOptions(opts SyncOptions) (SyncOptions, error) {
	if opts.Kind == "" {
		opts.Kind = "both"
	}
	if opts.Kind != "issue" && opts.Kind != "pull_request" && opts.Kind != "both" {
		return SyncOptions{}, errors.New("kind must be issue, pull_request, or both")
	}
	if opts.State == "" {
		opts.State = "all"
	}
	if opts.State != "open" && opts.State != "closed" && opts.State != "all" {
		return SyncOptions{}, fmt.Errorf("state must be open, closed, or all")
	}
	if opts.MaxPages <= 0 {
		opts.MaxPages = 1000
	}
	if opts.MaxPages > 1000 {
		return SyncOptions{}, errors.New("max pages cannot exceed 1000")
	}
	if opts.MaxRequests == 0 {
		opts.MaxRequests = defaultSyncMaxRequests
	}
	if opts.MaxRequests < syncFixedRequestCost() || opts.MaxRequests > maxSyncRequests {
		return SyncOptions{}, fmt.Errorf("max requests must be between %d and %d", syncFixedRequestCost(), maxSyncRequests)
	}
	if len(opts.Numbers) > 100 {
		return SyncOptions{}, errors.New("exact thread selection cannot exceed 100 numbers")
	}
	if len(opts.Numbers) > 0 && (opts.State != "all" || !opts.Since.IsZero()) {
		return SyncOptions{}, errors.New("state and since filters cannot be combined with exact thread numbers")
	}
	seen := make(map[int]struct{}, len(opts.Numbers))
	numbers := make([]int, 0, len(opts.Numbers))
	for _, number := range opts.Numbers {
		if number <= 0 {
			return SyncOptions{}, errors.New("thread numbers must be positive")
		}
		if _, ok := seen[number]; ok {
			continue
		}
		seen[number] = struct{}{}
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)
	opts.Numbers = numbers
	return opts, nil
}

func threadFromIssue(issue github.Issue) (corpus.Thread, string, error) {
	thread := corpus.Thread{
		Kind:              string(issue.Kind),
		Number:            issue.Number,
		State:             issue.State,
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
func (s *Service) Dossier(ctx context.Context, repo cli.RepoRef) (*cli.DossierResult, error) {
	ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Repo}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	if _, err := s.openReadOnlyCorpus(ctx); err != nil {
		return nil, err
	}
	d, err := s.buildDossier(ctx, ref)
	if err != nil {
		return nil, err
	}
	return &cli.DossierResult{
		Repo:       repo,
		Summary:    d.Repository.Description,
		Language:   firstLanguage(d.Repository.Languages),
		Stars:      d.Repository.Stars,
		OpenIssues: d.Repository.OpenIssueCount,
		Coverage:   coverageNames(d.Coverage),
		Freshness:  d.AsOf.Format(time.RFC3339),
	}, nil
}

// Index records a bounded immutable code snapshot from a clean local checkout.
func (s *Service) Index(ctx context.Context, repo cli.RepoRef, path string) (*cli.IndexResult, error) {
	ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Repo}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	snapshot, err := codeindex.Index(ctx, path, codeindex.Options{})
	if err != nil {
		return nil, err
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	_, inserted, err := c.StoreCodeSnapshot(ctx, ref, snapshot)
	if err != nil {
		return nil, err
	}
	message := "snapshot already indexed"
	if inserted {
		message = "snapshot stored"
	}
	return &cli.IndexResult{
		Repo: repo, Path: snapshot.RepoPath, Commit: snapshot.Commit,
		Files: len(snapshot.Documents), Bytes: snapshot.TotalBytes, Inserted: inserted, Message: message,
	}, nil
}

func (s *Service) buildDossier(ctx context.Context, ref domain.RepoRef) (*domain.Dossier, error) {
	reader := &corpusReader{s: s}
	builder := dossier.NewBuilder(reader, dossier.DefaultRecentLimit)
	return builder.Build(ctx, ref)
}
