package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/contracts"
)

func TestInit(t *testing.T) {
	t.Parallel()
	svc := &fakeService{initResult: &contracts.InitResult{Path: "/tmp/gc", Message: "ready"}}
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
	t.Parallel()
	svc := &fakeService{initResult: &contracts.InitResult{Path: "/tmp/gc", Message: "ready"}}
	c, stdout, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"init", "--json"})
	requireNoErr(t, err)

	var got contracts.InitResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}
	if got.Path != "/tmp/gc" || got.Message != "ready" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestStatus(t *testing.T) {
	t.Parallel()
	svc := &fakeService{statusResult: &contracts.StatusResult{Healthy: true, Corpus: "gc", Version: "0.0.1", Message: "ok"}}
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

func TestRemovedSyncCommandIsRejected(t *testing.T) {
	t.Parallel()
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"sync", "o/r"})
	requireCLIError(t, err, cli.ExitUsage)
	if !strings.Contains(err.Error(), "sync was removed; use `gitcontribute archive sync OWNER/REPO`") {
		t.Fatalf("error = %q", err)
	}
}

func TestSearchDefaults(t *testing.T) {
	t.Parallel()
	svc := &fakeService{searchResult: &contracts.SearchResult{
		Query: "test",
		Kind:  "threads",
		Limit: 20,
		Total: 1,
		Matches: []contracts.SearchMatch{{
			Kind:   "issue",
			Repo:   contracts.RepoRef{Owner: "o", Repo: "r"},
			Title:  "foo",
			Number: 42,
			Score:  0.9,
		}},
	}}
	c, stdout, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"search", "threads", "test"})
	requireNoErr(t, err)

	if !svc.searchCalled {
		t.Fatal("Search was not called")
	}
	if svc.lastSearchArgs.Query != "test" {
		t.Fatalf("query=%q, want test", svc.lastSearchArgs.Query)
	}
	if svc.lastSearchArgs.Opts.Kind != "threads" {
		t.Fatalf("kind=%q, want threads", svc.lastSearchArgs.Opts.Kind)
	}
	if svc.lastSearchArgs.Opts.Limit != 20 {
		t.Fatalf("limit=%d, want 20", svc.lastSearchArgs.Opts.Limit)
	}
	want := "Search: test (kind=threads, limit=20)\n1 matches:\n- issue o/r#42: foo (relevance 0.9)\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout=%q, want %q", got, want)
	}
}

func TestSearchJSONWithFlags(t *testing.T) {
	t.Parallel()
	svc := &fakeService{searchResult: &contracts.SearchResult{
		Query:   "good first issue",
		Kind:    "issues",
		Repo:    "o/r",
		Limit:   5,
		Total:   0,
		Matches: []contracts.SearchMatch{},
	}}
	c, stdout, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"search", "issues", "good first issue", "--repo", "o/r", "--state", "open", "--author", "alice", "--label", "bug", "--updated-after", "2026-07-01T00:00:00Z", "--sort", "updated", "--limit", "5", "--cursor", "next-page", "--json"})
	requireNoErr(t, err)

	if !svc.searchCalled {
		t.Fatal("Search was not called")
	}
	if svc.lastSearchArgs.Query != "good first issue" {
		t.Fatalf("query=%q", svc.lastSearchArgs.Query)
	}
	opts := svc.lastSearchArgs.Opts
	if opts.Kind != "issues" || opts.Repo != "o/r" || opts.State != "open" || opts.Author != "alice" || len(opts.Labels) != 1 || opts.Labels[0] != "bug" || opts.UpdatedAfter.Format(time.RFC3339) != "2026-07-01T00:00:00Z" || opts.Sort != "updated" || opts.Limit != 5 || opts.Cursor != "next-page" {
		t.Fatalf("unexpected options: %+v", opts)
	}

	var got contracts.SearchResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}
	if got.Query != "good first issue" || got.Kind != "issues" || got.Repo != "o/r" || got.Limit != 5 {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestSearchNoNetworkImplied(t *testing.T) {
	t.Parallel()
	// Search must be local; the CLI dispatches to the injected service without
	// any hidden network access.
	svc := &fakeService{searchResult: &contracts.SearchResult{Query: "local", Total: 0, Matches: []contracts.SearchMatch{}}}
	c, _, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"search", "threads", "local"})
	requireNoErr(t, err)
	if !svc.searchCalled {
		t.Fatal("Search was not called")
	}
}

func TestSearchInvalidLimit(t *testing.T) {
	t.Parallel()
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"search", "threads", "x", "--limit", "0"})
	requireCLIError(t, err, cli.ExitUsage)
}

func TestSearchInvalidRepoFilter(t *testing.T) {
	t.Parallel()
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"search", "threads", "x", "--repo", "bad"})
	requireCLIError(t, err, cli.ExitUsage)
}

func TestSearchRejectsUnsupportedFilterCombinations(t *testing.T) {
	t.Parallel()
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)
	for _, args := range [][]string{
		{"search", "code", "x", "--state", "open"},
		{"search", "repos", "x", "--association", "OWNER"},
		{"search", "code", "x", "--assignee", "alice"},
	} {
		requireCLIError(t, c.Run(context.Background(), args), cli.ExitUsage)
	}
	if svc.searchCalled {
		t.Fatal("search should not be called for unsupported filter combinations")
	}
}

func TestSearchThreadMetadataFlags(t *testing.T) {
	t.Parallel()
	svc := &fakeService{searchResult: &contracts.SearchResult{
		Query:   "term",
		Kind:    "issues",
		Limit:   10,
		Total:   0,
		Matches: []contracts.SearchMatch{},
	}}
	c, _, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"search", "issues", "term", "--association", "OWNER", "--assignee", "alice"})
	requireNoErr(t, err)

	if !svc.searchCalled {
		t.Fatal("Search was not called")
	}
	opts := svc.lastSearchArgs.Opts
	if opts.Association != "OWNER" {
		t.Fatalf("association = %q, want OWNER", opts.Association)
	}
	if opts.Assignee != "alice" {
		t.Fatalf("assignee = %q, want alice", opts.Assignee)
	}
}

func TestSearchWithLensFlag(t *testing.T) {
	t.Parallel()
	svc := &fakeService{searchResult: &contracts.SearchResult{
		Query:   "test",
		Kind:    "issues",
		Limit:   10,
		Total:   1,
		Matches: []contracts.SearchMatch{},
	}}
	c, _, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"search", "issues", "test", "--lens", "active-go", "--limit", "10"})
	requireNoErr(t, err)

	if !svc.searchCalled {
		t.Fatal("Search was not called")
	}
	opts := svc.lastSearchArgs.Opts
	if opts.Lens != "active-go" {
		t.Fatalf("lens = %q, want active-go", opts.Lens)
	}
	if opts.Kind != "issues" || opts.Limit != 10 {
		t.Fatalf("unexpected options: %+v", opts)
	}
}

func TestSearchRejectsLensWithCursor(t *testing.T) {
	t.Parallel()
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)
	err := c.Run(context.Background(), []string{"search", "issues", "test", "--lens", "active-go", "--cursor", "abc"})
	requireCLIError(t, err, cli.ExitUsage)
	if svc.searchCalled {
		t.Fatal("search should not be called when lens and cursor are combined")
	}
}

func TestDossier(t *testing.T) {
	t.Parallel()
	svc := &fakeService{dossierResult: &contracts.DossierResult{
		Repo:       contracts.RepoRef{Owner: "o", Repo: "r"},
		Summary:    "A Go CLI",
		Language:   "Go",
		Stars:      100,
		OpenIssues: 5,
		Coverage:   []string{"metadata", "threads"},
		Freshness:  "2026-07-16T00:00:00Z",
	}}
	c, stdout, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"dossier", "show", "o/r"})
	requireNoErr(t, err)

	if !svc.dossierCalled {
		t.Fatal("Dossier was not called")
	}
	if svc.lastDossierArg != (contracts.RepoRef{Owner: "o", Repo: "r"}) {
		t.Fatalf("dossier repo=%+v", svc.lastDossierArg)
	}
	want := "Dossier: o/r\nSummary: A Go CLI\nLanguage: Go\nStars: 100\nOpen issues: 5\nCoverage: metadata, threads\nFreshness: 2026-07-16T00:00:00Z\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout=%q, want %q", got, want)
	}
}

func TestDossierJSON(t *testing.T) {
	t.Parallel()
	svc := &fakeService{dossierResult: &contracts.DossierResult{
		Repo:       contracts.RepoRef{Owner: "o", Repo: "r"},
		Summary:    "A Go CLI",
		Language:   "Go",
		Stars:      100,
		OpenIssues: 5,
		Coverage:   []string{"metadata"},
		Freshness:  "now",
	}}
	c, stdout, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"dossier", "show", "o/r", "--json"})
	requireNoErr(t, err)

	var got contracts.DossierResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}
	if got.Repo.Owner != "o" || got.Summary != "A Go CLI" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestContextCancellation(t *testing.T) {
	t.Parallel()
	svc := &fakeService{err: context.Canceled}
	c, _, _ := newTestCLI(svc, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := c.Run(ctx, []string{"status"})
	requireCLIError(t, err, cli.ExitCancelled)
}

func TestUnknownCommand(t *testing.T) {
	t.Parallel()
	c, _, _ := newTestCLI(coreOnlyService{}, nil)
	err := c.Run(context.Background(), []string{"nope"})
	requireCLIError(t, err, cli.ExitUsage)
}

func TestSetupNonInteractiveJSON(t *testing.T) {
	t.Parallel()
	svc := &fakeService{setupResult: &contracts.SetupReport{Operation: "setup", DryRun: true, MCPCommand: &contracts.SetupMCPCommand{Command: "/managed/gitcontribute", Args: []string{"mcp", "serve", "--transport=stdio"}}, Steps: []contracts.SetupStep{{Name: "codex", Status: "would configure"}}}}
	var out bytes.Buffer
	c := cli.New(svc, &fakeMCPRunner{}, &out, io.Discard)
	if err := c.Run(context.Background(), []string{"setup", "--mode", "mcp", "--codex", "--token-source", "none", "--dry-run", "--json"}); err != nil {
		t.Fatal(err)
	}
	if len(svc.lastSetup.Clients) != 1 || svc.lastSetup.Clients[0] != "codex" || !svc.lastSetup.DryRun {
		t.Fatalf("options = %+v", svc.lastSetup)
	}
	if !strings.Contains(out.String(), `"would configure"`) {
		t.Fatalf("output = %s", out.String())
	}
}

func TestInvestigationStartShowAndList(t *testing.T) {
	t.Parallel()
	svc := &fakeService{
		startInvResult: &contracts.InvestigationResult{
			ID: "inv-1", Repo: contracts.RepoRef{Owner: "o", Repo: "r"},
			CommitSHA: "abc", Lens: "go", Status: "open",
			CreatedAt: "2026-07-17T00:00:00Z", UpdatedAt: "2026-07-17T00:00:00Z",
		},
		showInvResult: &contracts.InvestigationResult{
			ID: "inv-1", Repo: contracts.RepoRef{Owner: "o", Repo: "r"},
			Status: "open", CreatedAt: "2026-07-17T00:00:00Z", UpdatedAt: "2026-07-17T00:00:00Z",
		},
		listInvResult: &contracts.InvestigationListResult{
			Investigations: []contracts.InvestigationResult{
				{ID: "inv-1", Repo: contracts.RepoRef{Owner: "o", Repo: "r"}, Status: "open", CreatedAt: "2026-07-17T00:00:00Z", UpdatedAt: "2026-07-17T00:00:00Z"},
			},
		},
	}
	c, stdout, stderr := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"investigation", "start", "o/r", "--commit", "abc", "--lens", "go"})
	requireNoErr(t, err)
	if !svc.startInvCalled || svc.lastStartInvArgs.Repo.String() != "o/r" || svc.lastStartInvArgs.Commit != "abc" || svc.lastStartInvArgs.Lens != "go" {
		t.Fatalf("start investigation args = %+v", svc.lastStartInvArgs)
	}
	if !strings.Contains(stdout.String(), "inv-1") || !strings.Contains(stderr.String(), "starting investigation") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	stdout.Reset()
	err = c.Run(context.Background(), []string{"investigation", "show", "inv-1"})
	requireNoErr(t, err)
	if !svc.showInvCalled || svc.lastShowInvArg != "inv-1" {
		t.Fatalf("show investigation not called correctly: called=%v arg=%q", svc.showInvCalled, svc.lastShowInvArg)
	}
	if !strings.Contains(stdout.String(), "inv-1") {
		t.Fatalf("stdout=%q", stdout.String())
	}

	stdout.Reset()
	err = c.Run(context.Background(), []string{"investigation", "list"})
	requireNoErr(t, err)
	if !svc.listInvCalled {
		t.Fatal("list investigations not called")
	}
	if !strings.Contains(stdout.String(), "1 investigation") {
		t.Fatalf("stdout=%q", stdout.String())
	}
}

func TestInvestigationStartRejectsInvalidRepo(t *testing.T) {
	t.Parallel()
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)
	err := c.Run(context.Background(), []string{"investigation", "start", "bad"})
	requireCLIError(t, err, cli.ExitUsage)
	if svc.startInvCalled {
		t.Fatal("start investigation should not be called with invalid repo")
	}
}

func TestHypothesisAddAndList(t *testing.T) {
	t.Parallel()
	svc := &fakeService{
		addHypResult: &contracts.HypothesisResult{
			ID: "hyp-1", InvestigationID: "inv-1", Title: "race", Description: "race desc", Category: "bug", Status: "proposed",
			CreatedAt: "2026-07-17T00:00:00Z", UpdatedAt: "2026-07-17T00:00:00Z",
		},
		listHypResult: &contracts.HypothesisListResult{
			Hypotheses: []contracts.HypothesisResult{
				{ID: "hyp-1", InvestigationID: "inv-1", Title: "race", Category: "bug", Status: "proposed"},
			},
		},
	}
	c, stdout, stderr := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"hypothesis", "add", "inv-1", "--title", "race", "--description", "race desc", "--category", "bug"})
	requireNoErr(t, err)
	if !svc.addHypCalled || svc.lastAddHypArgs.InvestigationID != "inv-1" || svc.lastAddHypArgs.Title != "race" || svc.lastAddHypArgs.Category != "bug" {
		t.Fatalf("add hypothesis args = %+v", svc.lastAddHypArgs)
	}
	if !strings.Contains(stdout.String(), "hyp-1") || !strings.Contains(stderr.String(), "recording hypothesis") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	stdout.Reset()
	err = c.Run(context.Background(), []string{"hypothesis", "list", "inv-1"})
	requireNoErr(t, err)
	if !svc.listHypCalled || svc.lastListHypArg != "inv-1" {
		t.Fatalf("list hypotheses not called correctly: called=%v arg=%q", svc.listHypCalled, svc.lastListHypArg)
	}
	if !strings.Contains(stdout.String(), "1 hypothesis") {
		t.Fatalf("stdout=%q", stdout.String())
	}
}

func TestOpportunityPromoteShowListAndSetStatus(t *testing.T) {
	t.Parallel()
	svc := &fakeService{
		promoteOppResult: &contracts.OpportunityResult{
			ID: "opp-1", InvestigationID: "inv-1", HypothesisID: "hyp-1", Title: "race",
			ProblemStatement: "data race", Scope: "pkg/foo", Impact: "crash", ExpectedEffort: "small",
			Confidence: 0.8, Category: "bug", CollisionStatus: "unknown", Status: "hypothesis",
			CreatedAt: "2026-07-17T00:00:00Z", UpdatedAt: "2026-07-17T00:00:00Z",
		},
		showOppResult: &contracts.OpportunityResult{
			ID: "opp-1", Title: "race", Status: "reproduced", Confidence: 0.8,
			CreatedAt: "2026-07-17T00:00:00Z", UpdatedAt: "2026-07-17T00:01:00Z",
		},
		listOppResult: &contracts.OpportunityListResult{
			Filter: "inv-1",
			Opportunities: []contracts.OpportunityResult{
				{ID: "opp-1", Title: "race", Status: "reproduced", Confidence: 0.8, Category: "bug"},
			},
		},
		setStatusOppResult: &contracts.OpportunityResult{
			ID: "opp-1", Title: "race", Status: "reproduced",
		},
	}
	c, stdout, stderr := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{
		"opportunity", "promote", "hyp-1",
		"--problem", "data race", "--scope", "pkg/foo", "--impact", "crash",
		"--effort", "small", "--confidence", "0.8",
	})
	requireNoErr(t, err)
	if !svc.promoteOppCalled || svc.lastPromoteArgs.HypothesisID != "hyp-1" || svc.lastPromoteArgs.Confidence != 0.8 {
		t.Fatalf("promote opportunity args = %+v", svc.lastPromoteArgs)
	}
	if !strings.Contains(stdout.String(), "opp-1") || !strings.Contains(stderr.String(), "promoting hypothesis") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	stdout.Reset()
	err = c.Run(context.Background(), []string{"opportunity", "show", "opp-1"})
	requireNoErr(t, err)
	if !svc.showOppCalled || svc.lastShowOppArg != "opp-1" {
		t.Fatalf("show opportunity not called correctly: called=%v arg=%q", svc.showOppCalled, svc.lastShowOppArg)
	}

	stdout.Reset()
	err = c.Run(context.Background(), []string{"opportunity", "list", "--investigation", "inv-1"})
	requireNoErr(t, err)
	if !svc.listOppCalled || svc.lastListOppFilter != "inv-1" {
		t.Fatalf("list opportunities filter = %q", svc.lastListOppFilter)
	}
	if !strings.Contains(stdout.String(), "1 opportunity") || !strings.Contains(stdout.String(), "(filter: inv-1)") {
		t.Fatalf("stdout=%q", stdout.String())
	}

	stdout.Reset()
	err = c.Run(context.Background(), []string{"opportunity", "set-status", "opp-1", "reproduced", "--rationale", "base branch fails"})
	requireNoErr(t, err)
	if !svc.setStatusOppCalled || svc.lastSetStatusArgs.ID != "opp-1" || svc.lastSetStatusArgs.Status != "reproduced" || svc.lastSetStatusArgs.Rationale != "base branch fails" {
		t.Fatalf("set-status args = %+v", svc.lastSetStatusArgs)
	}
}

func TestInvestigationCommandRequiresService(t *testing.T) {
	t.Parallel()
	c, _, _ := newTestCLI(coreOnlyService{}, nil)
	err := c.Run(context.Background(), []string{"investigation", "list"})
	requireCLIError(t, err, cli.ExitNotWired)
}
