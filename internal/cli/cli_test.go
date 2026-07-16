package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/morluto/gitcontribute/internal/cli"
)

type fakeService struct {
	initCalled    bool
	statusCalled  bool
	syncCalled    bool
	searchCalled  bool
	dossierCalled bool

	initResult    *cli.InitResult
	statusResult  *cli.StatusResult
	syncResult    *cli.SyncResult
	searchResult  *cli.SearchResult
	dossierResult *cli.DossierResult

	lastSyncArg    cli.RepoRef
	lastSearchArgs struct {
		Query string
		Opts  cli.SearchOptions
	}
	lastDossierArg cli.RepoRef

	err error
}

func (f *fakeService) Init(ctx context.Context) (*cli.InitResult, error) {
	f.initCalled = true
	return f.initResult, f.err
}

func (f *fakeService) Status(ctx context.Context) (*cli.StatusResult, error) {
	f.statusCalled = true
	return f.statusResult, f.err
}

func (f *fakeService) Sync(ctx context.Context, repo cli.RepoRef) (*cli.SyncResult, error) {
	f.syncCalled = true
	f.lastSyncArg = repo
	return f.syncResult, f.err
}

func (f *fakeService) Search(ctx context.Context, query string, opts cli.SearchOptions) (*cli.SearchResult, error) {
	f.searchCalled = true
	f.lastSearchArgs.Query = query
	f.lastSearchArgs.Opts = opts
	return f.searchResult, f.err
}

func (f *fakeService) Dossier(ctx context.Context, repo cli.RepoRef) (*cli.DossierResult, error) {
	f.dossierCalled = true
	f.lastDossierArg = repo
	return f.dossierResult, f.err
}

type fakeMCPRunner struct {
	called bool
	opts   cli.MCPOptions
	err    error
}

func (f *fakeMCPRunner) Run(ctx context.Context, opts cli.MCPOptions) error {
	f.called = true
	f.opts = opts
	return f.err
}

func newTestCLI(svc cli.Service, runner cli.MCPRunner) (*cli.CLI, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	return cli.New(svc, runner, &stdout, &stderr), &stdout, &stderr
}

func requireNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func requireCLIError(t *testing.T, err error, wantCode int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected CLIError with code %d, got nil", wantCode)
	}
	var ce *cli.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *CLIError, got %T: %v", err, err)
	}
	if ce.Code != wantCode {
		t.Fatalf("exit code=%d, want %d", ce.Code, wantCode)
	}
}

func TestInit(t *testing.T) {
	svc := &fakeService{initResult: &cli.InitResult{Path: "/tmp/gc", Message: "ready"}}
	c, stdout, stderr := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"init"})
	requireNoErr(t, err)

	if !svc.initCalled {
		t.Fatal("Init was not called")
	}
	want := "Initialized corpus at /tmp/gc.\nready\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout=%q, want %q", got, want)
	}
	if got := stderr.String(); got != "initializing...\n" {
		t.Fatalf("stderr=%q, want progress message", got)
	}
}

func TestInitJSON(t *testing.T) {
	svc := &fakeService{initResult: &cli.InitResult{Path: "/tmp/gc", Message: "ready"}}
	c, stdout, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"init", "--json"})
	requireNoErr(t, err)

	var got cli.InitResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}
	if got.Path != "/tmp/gc" || got.Message != "ready" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestStatus(t *testing.T) {
	svc := &fakeService{statusResult: &cli.StatusResult{Healthy: true, Corpus: "gc", Version: "0.0.1", Message: "ok"}}
	c, stdout, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"status"})
	requireNoErr(t, err)

	if !svc.statusCalled {
		t.Fatal("Status was not called")
	}
	want := "Status: healthy (corpus=gc version=0.0.1)\nok\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout=%q, want %q", got, want)
	}
}

func TestSync(t *testing.T) {
	svc := &fakeService{syncResult: &cli.SyncResult{Repo: cli.RepoRef{Owner: "o", Repo: "r"}, Updated: 7, Message: "ok"}}
	c, stdout, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"sync", "o/r"})
	requireNoErr(t, err)

	if !svc.syncCalled {
		t.Fatal("Sync was not called")
	}
	if svc.lastSyncArg != (cli.RepoRef{Owner: "o", Repo: "r"}) {
		t.Fatalf("sync repo=%+v, want o/r", svc.lastSyncArg)
	}
	want := "Synced o/r: 7 updated.\nok\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout=%q, want %q", got, want)
	}
}

func TestSyncInvalidRepo(t *testing.T) {
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"sync", "invalid"})
	requireCLIError(t, err, cli.ExitUsage)

	if svc.syncCalled {
		t.Fatal("Sync should not be called with invalid repo")
	}
}

func TestSearchDefaults(t *testing.T) {
	svc := &fakeService{searchResult: &cli.SearchResult{
		Query: "test",
		Kind:  "all",
		Limit: 20,
		Total: 1,
		Matches: []cli.SearchMatch{{
			Kind:   "issue",
			Repo:   cli.RepoRef{Owner: "o", Repo: "r"},
			Title:  "foo",
			Number: 42,
			Score:  0.9,
		}},
	}}
	c, stdout, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"search", "test"})
	requireNoErr(t, err)

	if !svc.searchCalled {
		t.Fatal("Search was not called")
	}
	if svc.lastSearchArgs.Query != "test" {
		t.Fatalf("query=%q, want test", svc.lastSearchArgs.Query)
	}
	if svc.lastSearchArgs.Opts.Kind != "all" {
		t.Fatalf("kind=%q, want all", svc.lastSearchArgs.Opts.Kind)
	}
	if svc.lastSearchArgs.Opts.Limit != 20 {
		t.Fatalf("limit=%d, want 20", svc.lastSearchArgs.Opts.Limit)
	}
	want := "Search: test (kind=all, limit=20)\n1 matches:\n- issue o/r#42: foo (0.90)\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout=%q, want %q", got, want)
	}
}

func TestSearchJSONWithFlags(t *testing.T) {
	svc := &fakeService{searchResult: &cli.SearchResult{
		Query:   "good first issue",
		Kind:    "issues",
		Repo:    "o/r",
		Limit:   5,
		Total:   0,
		Matches: []cli.SearchMatch{},
	}}
	c, stdout, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"search", "good first issue", "--kind", "issues", "--repo", "o/r", "--limit", "5", "--json"})
	requireNoErr(t, err)

	if !svc.searchCalled {
		t.Fatal("Search was not called")
	}
	if svc.lastSearchArgs.Query != "good first issue" {
		t.Fatalf("query=%q", svc.lastSearchArgs.Query)
	}
	opts := svc.lastSearchArgs.Opts
	if opts.Kind != "issues" || opts.Repo != "o/r" || opts.Limit != 5 {
		t.Fatalf("unexpected options: %+v", opts)
	}

	var got cli.SearchResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}
	if got.Query != "good first issue" || got.Kind != "issues" || got.Repo != "o/r" || got.Limit != 5 {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestSearchNoNetworkImplied(t *testing.T) {
	// Search must be local; the CLI dispatches to the injected service without
	// any hidden network access.
	svc := &fakeService{searchResult: &cli.SearchResult{Query: "local", Total: 0, Matches: []cli.SearchMatch{}}}
	c, _, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"search", "local"})
	requireNoErr(t, err)
	if !svc.searchCalled {
		t.Fatal("Search was not called")
	}
}

func TestSearchInvalidLimit(t *testing.T) {
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"search", "x", "--limit", "0"})
	requireCLIError(t, err, cli.ExitUsage)
}

func TestSearchInvalidRepoFilter(t *testing.T) {
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"search", "x", "--repo", "bad"})
	requireCLIError(t, err, cli.ExitUsage)
}

func TestDossier(t *testing.T) {
	svc := &fakeService{dossierResult: &cli.DossierResult{
		Repo:       cli.RepoRef{Owner: "o", Repo: "r"},
		Summary:    "A Go CLI",
		Language:   "Go",
		Stars:      100,
		OpenIssues: 5,
		Coverage:   []string{"metadata", "threads"},
		Freshness:  "2026-07-16T00:00:00Z",
	}}
	c, stdout, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"dossier", "o/r"})
	requireNoErr(t, err)

	if !svc.dossierCalled {
		t.Fatal("Dossier was not called")
	}
	if svc.lastDossierArg != (cli.RepoRef{Owner: "o", Repo: "r"}) {
		t.Fatalf("dossier repo=%+v", svc.lastDossierArg)
	}
	want := "Dossier: o/r\nSummary: A Go CLI\nLanguage: Go\nStars: 100\nOpen issues: 5\nCoverage: metadata, threads\nFreshness: 2026-07-16T00:00:00Z\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout=%q, want %q", got, want)
	}
}

func TestDossierJSON(t *testing.T) {
	svc := &fakeService{dossierResult: &cli.DossierResult{
		Repo:       cli.RepoRef{Owner: "o", Repo: "r"},
		Summary:    "A Go CLI",
		Language:   "Go",
		Stars:      100,
		OpenIssues: 5,
		Coverage:   []string{"metadata"},
		Freshness:  "now",
	}}
	c, stdout, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"dossier", "o/r", "--json"})
	requireNoErr(t, err)

	var got cli.DossierResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}
	if got.Repo.Owner != "o" || got.Summary != "A Go CLI" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestMCP(t *testing.T) {
	runner := &fakeMCPRunner{}
	c, stdout, stderr := newTestCLI(nil, runner)

	err := c.Run(context.Background(), []string{"mcp"})
	requireNoErr(t, err)

	if !runner.called {
		t.Fatal("MCP Run was not called")
	}
	if runner.opts.Transport != "stdio" {
		t.Fatalf("transport=%q, want stdio", runner.opts.Transport)
	}
	if stdout.String() != "" {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if stderr.String() != "starting mcp server (transport=stdio)...\n" {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestContextCancellation(t *testing.T) {
	svc := &fakeService{err: context.Canceled}
	c, _, _ := newTestCLI(svc, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := c.Run(ctx, []string{"status"})
	requireCLIError(t, err, cli.ExitCancelled)
}

func TestUnknownCommand(t *testing.T) {
	c, _, _ := newTestCLI(&fakeService{}, nil)
	err := c.Run(context.Background(), []string{"nope"})
	requireCLIError(t, err, cli.ExitUsage)
}
