package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/dossier"
	"github.com/morluto/gitcontribute/internal/github"
)

// Service is the product-owned application layer that satisfies cli.Service.
// MCP reads are exposed through MCPReader.
type Service struct {
	mu       sync.Mutex
	cfg      *config.Config
	paths    *config.Paths
	corpus   *corpus.Corpus
	ghReader github.Reader
	version  string
}

// New creates a Service and resolves local configuration. GitHub credentials
// are resolved lazily only when a network-reading operation is requested.
func New(paths *config.Paths, version string) (*Service, error) {
	if paths == nil {
		paths = config.NewPaths(nil)
	}
	s := &Service{paths: paths, version: version}
	if _, err := s.loadConfig(false); err != nil {
		return nil, err
	}
	return s, nil
}

// SetGitHubReader overrides the GitHub reader. It is intended for tests.
func (s *Service) SetGitHubReader(r github.Reader) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ghReader = r
}

// Close closes the corpus database connection.
func (s *Service) Close() error {
	s.mu.Lock()
	c := s.corpus
	s.corpus = nil
	s.mu.Unlock()
	if c != nil {
		return c.Close()
	}
	return nil
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
	client, err := github.NewClient(github.Config{TokenSource: tokenSrc})
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
	_, err = s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	return &cli.InitResult{Path: cfg.Database, Message: "corpus initialized"}, nil
}

// Status reports whether the corpus is healthy and counts local records.
func (s *Service) Status(ctx context.Context) (*cli.StatusResult, error) {
	c, err := s.openCorpus(ctx)
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

// Sync fetches a repository and all issue-list pages from GitHub and writes
// ordered observations to the local corpus.
func (s *Service) Sync(ctx context.Context, repo cli.RepoRef) (*cli.SyncResult, error) {
	ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Repo}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
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

	ghRepo, _, err := reader.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil {
		syncErr = fmt.Errorf("get repository: %w", err)
		return nil, syncErr
	}
	repoProjection := corpusRepoFromGitHub(ghRepo)
	repoPayload, err := json.Marshal(ghRepo)
	if err != nil {
		syncErr = fmt.Errorf("marshal repository: %w", err)
		return nil, syncErr
	}
	upsertedRepo, err := c.UpsertRepository(ctx, repoProjection, string(repoPayload))
	if err != nil {
		syncErr = fmt.Errorf("upsert repository: %w", err)
		return nil, syncErr
	}
	repoProjection = *upsertedRepo
	if err := c.AdvanceFacet(ctx, repoProjection.ID, nil, "metadata", ghRepo.UpdatedAt, true, run.ID); err != nil {
		syncErr = fmt.Errorf("advance metadata facet: %w", err)
		return nil, syncErr
	}

	opts := github.ListIssueOptions{
		State:     "all",
		Sort:      "updated",
		Direction: "desc",
		PageOptions: github.PageOptions{
			Page:    1,
			PerPage: 100,
		},
	}

	updated := 0
	pages := 0
	lastSourceUpdated := ghRepo.UpdatedAt
	for {
		res, err := reader.ListIssues(ctx, ref.Owner, ref.Repo, opts)
		if err != nil {
			syncErr = fmt.Errorf("list issues page %d: %w", opts.Page, err)
			return nil, syncErr
		}
		pages++

		for _, issue := range res.Items {
			thread, payload, err := s.threadFromIssue(ctx, reader, ref, issue)
			if err != nil {
				syncErr = err
				return nil, syncErr
			}
			thread.RepositoryID = repoProjection.ID
			if _, err := c.UpsertThread(ctx, thread, payload); err != nil {
				syncErr = fmt.Errorf("upsert thread: %w", err)
				return nil, syncErr
			}
			updated++
			if thread.SourceUpdatedAt.After(lastSourceUpdated) {
				lastSourceUpdated = thread.SourceUpdatedAt
			}
		}

		if !res.Page.HasNext {
			break
		}
		opts.Page = res.Page.NextPage
	}

	if err := c.AdvanceFacet(ctx, repoProjection.ID, nil, "threads", lastSourceUpdated, true, run.ID); err != nil {
		syncErr = fmt.Errorf("advance threads facet: %w", err)
		return nil, syncErr
	}

	stats := fmt.Sprintf(`{"pages":%d,"threads":%d}`, pages, updated)
	if err := c.FinishRun(ctx, run.ID, stats); err != nil {
		syncErr = err
		return nil, syncErr
	}

	return &cli.SyncResult{
		Repo:    repo,
		Updated: updated,
		Message: fmt.Sprintf("fetched %d threads across %d pages", updated, pages),
	}, nil
}

func (s *Service) threadFromIssue(ctx context.Context, reader github.Reader, ref domain.RepoRef, issue github.Issue) (corpus.Thread, string, error) {
	thread := corpus.Thread{
		Kind:            string(issue.Kind),
		Number:          issue.Number,
		State:           issue.State,
		Title:           issue.Title,
		Body:            issue.Body,
		Author:          issue.Author,
		Labels:          issue.Labels,
		SourceCreatedAt: issue.CreatedAt,
		SourceUpdatedAt: issue.UpdatedAt,
	}
	if issue.ClosedAt != nil {
		thread.ClosedAt = *issue.ClosedAt
	}

	payload, err := json.Marshal(issue)
	if err != nil {
		return corpus.Thread{}, "", fmt.Errorf("marshal issue: %w", err)
	}

	if issue.Kind == github.ThreadKindPullRequest {
		pr, _, err := reader.GetPullRequestDetails(ctx, ref.Owner, ref.Repo, issue.Number)
		if err != nil {
			return corpus.Thread{}, "", fmt.Errorf("get pull request %d details: %w", issue.Number, err)
		}
		thread.State = pr.State
		thread.Merged = pr.Merged
		if pr.MergedAt != nil {
			thread.MergedAt = *pr.MergedAt
		}
		if pr.ClosedAt != nil {
			thread.ClosedAt = *pr.ClosedAt
		}
		if !pr.UpdatedAt.IsZero() {
			thread.SourceUpdatedAt = pr.UpdatedAt
		}
		payload, err = json.Marshal(pr)
		if err != nil {
			return corpus.Thread{}, "", fmt.Errorf("marshal pull request details: %w", err)
		}
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

// Search performs a local-only corpus search and supports repo and kind filters.
func (s *Service) Search(ctx context.Context, query string, opts cli.SearchOptions) (*cli.SearchResult, error) {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Limit > 1000 {
		return nil, errors.New("search limit cannot exceed 1000")
	}
	res, err := s.searchCorpus(ctx, query, opts)
	if err != nil {
		return nil, err
	}
	matches := make([]cli.SearchMatch, len(res.Matches))
	for i, m := range res.Matches {
		matches[i] = cli.SearchMatch{
			Kind:   m.Kind,
			Repo:   cli.RepoRef{Owner: m.Repo.Owner, Repo: m.Repo.Repo},
			Title:  m.Title,
			Number: m.Number,
			URL:    m.URL,
			Score:  m.Score,
		}
	}
	return &cli.SearchResult{
		Query:   query,
		Kind:    opts.Kind,
		Repo:    opts.Repo,
		Limit:   opts.Limit,
		Total:   res.Total,
		Matches: matches,
	}, nil
}

// Dossier builds a deterministic, local-corpus-backed repository dossier.
func (s *Service) Dossier(ctx context.Context, repo cli.RepoRef) (*cli.DossierResult, error) {
	ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Repo}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	if _, err := s.openCorpus(ctx); err != nil {
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
