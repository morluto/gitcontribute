package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/github"
)

func TestEndToEndSyncSearchDossier(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := newTestServer("octocat", "test")
	defer srv.Close()

	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()

	syncRes, err := svc.Sync(ctx, cli.RepoRef{Owner: "octocat", Repo: "test"})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if syncRes.Updated != 2 {
		t.Fatalf("updated = %d, want 2", syncRes.Updated)
	}

	searchRes, err := svc.Search(ctx, "searchable", cli.SearchOptions{Kind: "issues", Limit: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if searchRes.Total != 1 || len(searchRes.Matches) != 1 {
		t.Fatalf("search results = %+v", searchRes)
	}
	if searchRes.Matches[0].Number != 1 {
		t.Fatalf("unexpected match: %+v", searchRes.Matches[0])
	}

	dossierRes, err := svc.Dossier(ctx, cli.RepoRef{Owner: "octocat", Repo: "test"})
	if err != nil {
		t.Fatalf("dossier: %v", err)
	}
	if dossierRes.Stars != 42 {
		t.Fatalf("stars = %d, want 42", dossierRes.Stars)
	}
	if dossierRes.OpenIssues != 1 {
		t.Fatalf("open issues = %d, want 1", dossierRes.OpenIssues)
	}
	if dossierRes.Summary != "A test repository" {
		t.Fatalf("summary = %q", dossierRes.Summary)
	}
}

func TestTailSourceRunsOneIdempotentIteration(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := newTestServer("octocat", "tail")
	defer srv.Close()
	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()

	if _, err := svc.AddRepoSource(ctx, "explicit", []cli.RepoRef{{Owner: "octocat", Repo: "tail"}}); err != nil {
		t.Fatalf("add source: %v", err)
	}
	result, err := svc.TailSource(ctx, "explicit", cli.TailOptions{
		Since: time.Hour, Budget: 1, Interval: time.Minute, Once: true,
	})
	if err != nil {
		t.Fatalf("tail source: %v", err)
	}
	if result.Iterations != 1 || result.Last == nil || result.Last.Repositories != 1 {
		t.Fatalf("tail result = %+v", result)
	}
}

func TestDiscoveryCrawlDoesNotAdvanceCheckpointWhenBudgetExhausted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := newTestServer("octocat", "discovered")
	defer srv.Close()
	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()
	if _, err := svc.AddSearchSource(ctx, "bounded", "language:go"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Crawl(ctx, "bounded", cli.CrawlOptions{Since: time.Hour, Budget: 1}); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("crawl error = %v, want budget exhaustion", err)
	}
	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint, exists, err := c.GetTime(ctx, "source:bounded"); err != nil || exists {
		t.Fatalf("checkpoint = %v exists=%v err=%v", checkpoint, exists, err)
	}
}

func TestAddSearchSourceRejectsUnstableName(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := newTestServer("octocat", "test")
	defer srv.Close()
	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()
	if _, err := svc.AddSearchSource(ctx, "contains spaces", "language:go"); err == nil {
		t.Fatal("expected invalid source name error")
	}
}

func TestSearchReportsDefaultLimit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := newTestServer("octocat", "test")
	defer srv.Close()
	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()
	if _, err := svc.Sync(ctx, cli.RepoRef{Owner: "octocat", Repo: "test"}); err != nil {
		t.Fatal(err)
	}
	result, err := svc.Search(ctx, "searchable", cli.SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Limit != 20 {
		t.Fatalf("reported limit = %d, want 20", result.Limit)
	}
}

func TestLocalInitializationDoesNotResolveKeyringAuth(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	configPath, err := paths.ConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.TokenSource.Method = "keyring"
	cfg.TokenSource.Key = "test-account"
	if err := config.ApplyDefaults(cfg, paths); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	svc, err := New(paths, "test", nil)
	if err != nil {
		t.Fatalf("local service construction resolved GitHub auth: %v", err)
	}
	defer func() { _ = svc.Close() }()
	if _, err := svc.Init(ctx); err != nil {
		t.Fatalf("local init resolved GitHub auth: %v", err)
	}
}

func TestConfiguredAuthenticationIsRequired(t *testing.T) {
	t.Setenv("GITCONTRIBUTE_TEST_MISSING_TOKEN", "")
	cfg := config.Default()
	cfg.TokenSource = config.TokenSource{
		Method: "env",
		Key:    "GITCONTRIBUTE_TEST_MISSING_TOKEN",
	}

	_, err := tokenSource(cfg).Token(context.Background())
	if !errors.Is(err, github.ErrRequiredToken) {
		t.Fatalf("configured auth error = %v, want ErrRequiredToken", err)
	}
}

func TestNewRejectsInvalidConfiguredTokenSource(t *testing.T) {
	t.Parallel()
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	configPath, err := paths.ConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.TokenSource.Method = "keyrign"
	if err := config.ApplyDefaults(cfg, paths); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	_, err = New(paths, "test", nil)
	if err == nil || !strings.Contains(err.Error(), "invalid token_source method") {
		t.Fatalf("New error = %v, want invalid token source", err)
	}
}
