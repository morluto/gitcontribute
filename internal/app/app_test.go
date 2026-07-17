package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpserver"
)

type noopLimiter struct{}

func (noopLimiter) WaitN(context.Context, int) error { return nil }

type testServer struct {
	owner         string
	repo          string
	mu            sync.Mutex
	searchQueries []string
}

func (ts *testServer) recordSearch(query string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.searchQueries = append(ts.searchQueries, query)
}

func (ts *testServer) searches() []string {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return append([]string(nil), ts.searchQueries...)
}

func (ts *testServer) repoPayload() map[string]any {
	return map[string]any{
		"id":                123,
		"node_id":           "R_123",
		"name":              ts.repo,
		"full_name":         ts.owner + "/" + ts.repo,
		"owner":             map[string]any{"login": ts.owner, "id": 1},
		"private":           false,
		"fork":              false,
		"archived":          false,
		"is_template":       false,
		"default_branch":    "main",
		"html_url":          fmt.Sprintf("https://github.com/%s/%s", ts.owner, ts.repo),
		"description":       "A test repository",
		"stargazers_count":  42,
		"watchers_count":    7,
		"forks_count":       3,
		"open_issues_count": 2,
		"open_issues":       2,
		"language":          "Go",
		"license":           map[string]any{"name": "MIT", "spdx_id": "MIT"},
		"topics":            []string{"go", "test"},
		"created_at":        "2020-01-01T00:00:00Z",
		"updated_at":        "2024-01-01T00:00:00Z",
		"pushed_at":         "2024-06-01T00:00:00Z",
	}
}

func (ts *testServer) issuePayload() []map[string]any {
	return []map[string]any{
		{
			"id":         1,
			"node_id":    "I_1",
			"number":     1,
			"title":      "searchable bug",
			"state":      "open",
			"body":       "the bug is here",
			"user":       map[string]any{"login": "alice"},
			"labels":     []map[string]any{{"name": "bug"}},
			"created_at": "2024-01-01T00:00:00Z",
			"updated_at": "2024-02-01T00:00:00Z",
		},
		{
			"id":      2,
			"node_id": "PR_2",
			"number":  2,
			"title":   "Add feature",
			"state":   "closed",
			"body":    "PR body",
			"user":    map[string]any{"login": "bob"},
			"labels":  []map[string]any{{"name": "enhancement"}},
			"pull_request": map[string]any{
				"url":      fmt.Sprintf("%s/repos/%s/%s/pulls/2", "https://api.github.com", ts.owner, ts.repo),
				"html_url": fmt.Sprintf("https://github.com/%s/%s/pull/2", ts.owner, ts.repo),
			},
			"created_at": "2024-01-01T00:00:00Z",
			"updated_at": "2024-02-01T00:00:00Z",
			"closed_at":  "2024-03-01T00:00:00Z",
		},
	}
}

func (ts *testServer) prPayload() map[string]any {
	return map[string]any{
		"id":         2,
		"node_id":    "PR_2",
		"number":     2,
		"state":      "closed",
		"title":      "Add feature",
		"body":       "PR body",
		"merged":     true,
		"merged_at":  "2024-03-01T00:00:00Z",
		"closed_at":  "2024-03-01T00:00:00Z",
		"user":       map[string]any{"login": "bob"},
		"head":       map[string]any{"ref": "feature", "sha": "abc123"},
		"base":       map[string]any{"ref": "main", "sha": "def456"},
		"created_at": "2024-01-01T00:00:00Z",
		"updated_at": "2024-02-01T00:00:00Z",
		"html_url":   fmt.Sprintf("https://github.com/%s/%s/pull/2", ts.owner, ts.repo),
	}
}

func (ts *testServer) handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Ratelimit-Limit", "5000")
	w.Header().Set("X-Ratelimit-Remaining", "4999")
	w.Header().Set("X-Ratelimit-Used", "1")
	w.Header().Set("X-Ratelimit-Reset", fmt.Sprintf("%d", time.Now().Add(time.Hour).Unix()))

	switch r.URL.Path {
	case "/api/v3/search/repositories":
		ts.recordSearch(r.URL.Query().Get("q"))
		json.NewEncoder(w).Encode(map[string]any{
			"total_count":        1,
			"incomplete_results": false,
			"items":              []map[string]any{ts.repoPayload()},
		})
	case fmt.Sprintf("/api/v3/repos/%s/%s", ts.owner, ts.repo):
		json.NewEncoder(w).Encode(ts.repoPayload())
	case fmt.Sprintf("/api/v3/repos/%s/%s/issues", ts.owner, ts.repo):
		page := r.URL.Query().Get("page")
		if page == "" || page == "1" {
			json.NewEncoder(w).Encode(ts.issuePayload())
		} else {
			json.NewEncoder(w).Encode([]map[string]any{})
		}
	case fmt.Sprintf("/api/v3/repos/%s/%s/pulls/2", ts.owner, ts.repo):
		json.NewEncoder(w).Encode(ts.prPayload())
	default:
		http.NotFound(w, r)
	}
}

func newTestServer(owner, repo string) *httptest.Server {
	ts := &testServer{owner: owner, repo: repo}
	return httptest.NewServer(http.HandlerFunc(ts.handler))
}

func newTrackedTestServer(owner, repo string) (*httptest.Server, *testServer) {
	ts := &testServer{owner: owner, repo: repo}
	return httptest.NewServer(http.HandlerFunc(ts.handler)), ts
}

func newTestService(t *testing.T, srv *httptest.Server) *Service {
	t.Helper()
	ctx := context.Background()
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	client, err := github.NewClient(github.Config{
		BaseURL:     srv.URL,
		UploadURL:   srv.URL,
		TokenSource: github.StaticTokenSource(""),
		Limiter:     noopLimiter{},
	})
	if err != nil {
		t.Fatalf("new github client: %v", err)
	}
	svc.SetGitHubReader(client)

	if _, err := svc.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}
	return svc
}

func TestEndToEndSyncSearchDossier(t *testing.T) {
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

func TestDiscoveryCrawlPersistsRepositoryFrontierAndCheckpoint(t *testing.T) {
	ctx := context.Background()
	srv, tracked := newTrackedTestServer("octocat", "discovered")
	defer srv.Close()

	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()

	source, err := svc.AddSearchSource(ctx, "active-go", "language:go stars:>50")
	if err != nil {
		t.Fatalf("add source: %v", err)
	}
	if source.Name != "active-go" || source.Kind != "search" {
		t.Fatalf("source = %+v", source)
	}

	listed, err := svc.ListSources(ctx)
	if err != nil {
		t.Fatalf("list sources: %v", err)
	}
	if len(listed.Sources) != 1 || listed.Sources[0].Name != "active-go" {
		t.Fatalf("sources = %+v", listed.Sources)
	}

	result, err := svc.Crawl(ctx, "active-go", cli.CrawlOptions{Since: 24 * time.Hour, Budget: 10})
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}
	if result.Repositories != 1 || result.Windows != 1 || result.Requests != 2 || result.Checkpoint == "" {
		t.Fatalf("crawl result = %+v", result)
	}

	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := c.GetRepository(ctx, "octocat", "discovered")
	if err != nil {
		t.Fatal(err)
	}
	if repo == nil || repo.ExternalID != "R_123" {
		t.Fatalf("repository = %+v", repo)
	}
	frontier, err := c.GetFrontierItem(ctx, "repository:octocat/discovered:threads")
	if err != nil {
		t.Fatal(err)
	}
	if frontier == nil || frontier.Source != "active-go" {
		t.Fatalf("frontier = %+v", frontier)
	}
	checkpoint, exists, err := c.GetTime(ctx, "source:active-go")
	if err != nil || !exists || checkpoint.IsZero() {
		t.Fatalf("checkpoint = %v exists=%v err=%v", checkpoint, exists, err)
	}

	second, err := svc.Crawl(ctx, "active-go", cli.CrawlOptions{Since: 24 * time.Hour, Budget: 10})
	if err != nil {
		t.Fatalf("incremental crawl: %v", err)
	}
	if second.Repositories != 1 {
		t.Fatalf("incremental crawl result = %+v", second)
	}
	queries := tracked.searches()
	if len(queries) != 4 || !strings.Contains(queries[0], "created:") || !strings.Contains(queries[2], "updated:") {
		t.Fatalf("search queries = %q", queries)
	}
	status, err := c.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Repositories != 1 {
		t.Fatalf("repositories = %d, want canonical deduplication", status.Repositories)
	}
}

func TestDiscoveryCrawlDoesNotAdvanceCheckpointWhenBudgetExhausted(t *testing.T) {
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

func TestLocalInitializationDoesNotResolveUnsupportedAuth(t *testing.T) {
	ctx := context.Background()
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	configPath, err := paths.ConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.TokenSource.Method = "keyring"
	if err := config.ApplyDefaults(cfg, paths); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	svc, err := New(paths, "test")
	if err != nil {
		t.Fatalf("local service construction resolved GitHub auth: %v", err)
	}
	defer func() { _ = svc.Close() }()
	if _, err := svc.Init(ctx); err != nil {
		t.Fatalf("local init resolved GitHub auth: %v", err)
	}
	if _, err := svc.Sync(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}); err == nil || !strings.Contains(err.Error(), "keyring") {
		t.Fatalf("network sync error = %v, want explicit unsupported auth", err)
	}
}

func TestContributionGuidanceDoesNotClaimUnfetchedSource(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer("octocat", "test")
	defer srv.Close()
	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()
	if _, err := svc.Sync(ctx, cli.RepoRef{Owner: "octocat", Repo: "test"}); err != nil {
		t.Fatal(err)
	}
	guidance, refs, err := (&corpusReader{s: svc}).ReadContributionGuidance(ctx, domain.RepoRef{Owner: "octocat", Repo: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if guidance != "" || len(refs) != 0 {
		t.Fatalf("unfetched guidance = %q refs=%+v", guidance, refs)
	}
}

func TestMCPReaderLocalReads(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer("acme", "rocket")
	defer srv.Close()

	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()

	if _, err := svc.Sync(ctx, cli.RepoRef{Owner: "acme", Repo: "rocket"}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	reader := svc.MCPReader()

	repo, err := reader.Repository(ctx, mcpserver.RepoInput{Owner: "acme", Repo: "rocket"})
	if err != nil {
		t.Fatalf("mcp repository: %v", err)
	}
	if repo.Owner != "acme" || repo.Repo != "rocket" || repo.Fields["stars"] != 42 {
		t.Fatalf("unexpected repository output: %+v", repo)
	}

	thread, err := reader.Thread(ctx, mcpserver.ThreadInput{Owner: "acme", Repo: "rocket", Kind: "issue", Number: 1})
	if err != nil {
		t.Fatalf("mcp thread: %v", err)
	}
	if thread.Number != 1 || thread.State != "open" {
		t.Fatalf("unexpected thread output: %+v", thread)
	}

	search, err := reader.Search(ctx, mcpserver.SearchInput{Query: "searchable", Kind: "issue", Limit: 10})
	if err != nil {
		t.Fatalf("mcp search: %v", err)
	}
	if search.Total != 1 {
		t.Fatalf("search total = %d, want 1", search.Total)
	}

	dossier, err := reader.Dossier(ctx, mcpserver.RepoInput{Owner: "acme", Repo: "rocket"})
	if err != nil {
		t.Fatalf("mcp dossier: %v", err)
	}
	if dossier.Owner != "acme" || dossier.Repo != "rocket" {
		t.Fatalf("unexpected dossier output: %+v", dossier)
	}
	if _, ok := dossier.Sections["stars"]; !ok {
		t.Fatalf("dossier missing stars section: %+v", dossier.Sections)
	}

	_, err = reader.Thread(ctx, mcpserver.ThreadInput{Owner: "acme", Repo: "rocket", Kind: "issue", Number: 404})
	if err == nil {
		t.Fatal("expected not found for missing thread")
	}
	if !errors.Is(err, mcpserver.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMCPSearchRequiresCompleteRepositoryFilter(t *testing.T) {
	svc := &Service{}
	_, err := svc.MCPReader().Search(context.Background(), mcpserver.SearchInput{Query: "bug", Owner: "owner"})
	if err == nil || !strings.Contains(err.Error(), "provided together") {
		t.Fatalf("Search error = %v", err)
	}
}

func TestSearchCodeUsesStoredSnapshotWithoutNetwork(t *testing.T) {
	ctx := context.Background()
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	if _, err := svc.Init(ctx); err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.corpus.StoreCodeSnapshot(ctx, domain.RepoRef{Owner: "owner", Repo: "repo"}, codeindex.Snapshot{
		RepoPath: "/repo", Commit: "abc", CreatedAt: time.Now(), TotalBytes: 20,
		Documents: []codeindex.Document{{Path: "parser.go", Content: "func searchableParser() {}", Bytes: 25, LanguageHint: "go"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.Search(ctx, "searchableParser", cli.SearchOptions{Kind: "code", Repo: "owner/repo", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 1 || result.Matches[0].Title != "parser.go" {
		t.Fatalf("code search = %+v", result)
	}
}

func TestInvestigationAndOpportunityFlow(t *testing.T) {
	ctx := context.Background()
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	if _, err := svc.Init(ctx); err != nil {
		t.Fatal(err)
	}

	inv, err := svc.StartInvestigation(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, "abc", "go")
	if err != nil {
		t.Fatalf("start investigation: %v", err)
	}
	if inv.ID == "" || inv.Repo.String() != "owner/repo" || inv.CommitSHA != "abc" || inv.Lens != "go" || inv.Status != "open" {
		t.Fatalf("unexpected investigation: %+v", inv)
	}

	h, err := svc.AddHypothesis(ctx, inv.ID, "race in parser", "data race under load", "bug")
	if err != nil {
		t.Fatalf("add hypothesis: %v", err)
	}
	if h.ID == "" || h.InvestigationID != inv.ID || h.Status != "proposed" {
		t.Fatalf("unexpected hypothesis: %+v", h)
	}

	hypotheses, err := svc.ListHypotheses(ctx, inv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(hypotheses.Hypotheses) != 1 {
		t.Fatalf("expected 1 hypothesis, got %+v", hypotheses)
	}

	opp, err := svc.PromoteOpportunity(ctx, h.ID, "parser panics on valid input", "pkg/parser", "crash", "small", 0.8)
	if err != nil {
		t.Fatalf("promote opportunity: %v", err)
	}
	if opp.ID == "" || opp.HypothesisID != h.ID || opp.Status != "hypothesis" || opp.Confidence != 0.8 {
		t.Fatalf("unexpected opportunity: %+v", opp)
	}

	opps, err := svc.ListOpportunities(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(opps.Opportunities) != 1 {
		t.Fatalf("expected 1 opportunity, got %+v", opps)
	}

	filtered, err := svc.ListOpportunities(ctx, inv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Opportunities) != 1 || filtered.Opportunities[0].ID != opp.ID {
		t.Fatalf("expected filtered opportunity, got %+v", filtered)
	}

	updated, err := svc.SetOpportunityStatus(ctx, opp.ID, "reproduced", "base branch fails")
	if err != nil {
		t.Fatalf("set opportunity status: %v", err)
	}
	if updated.Status != "reproduced" {
		t.Fatalf("expected status reproduced, got %s", updated.Status)
	}

	shown, err := svc.ShowOpportunity(ctx, opp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if shown.Status != "reproduced" {
		t.Fatalf("unexpected shown opportunity status: %s", shown.Status)
	}

	investigations, err := svc.ListInvestigations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(investigations.Investigations) != 1 {
		t.Fatalf("expected 1 investigation, got %+v", investigations)
	}
}
