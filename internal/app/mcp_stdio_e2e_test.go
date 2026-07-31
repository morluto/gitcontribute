package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
	"github.com/morluto/gitcontribute/internal/mcpserver"
)

const (
	mcpE2EHomeEnv   = "GITCONTRIBUTE_MCP_E2E_HOME"
	mcpE2EGitHubEnv = "GITCONTRIBUTE_MCP_E2E_GITHUB_URL"
)

// TestMCPStdioHelper is the subprocess entry point used by
// TestMCPStdioScalableResearchFlow. It serves the real application over stdio
// and is not executed as a standalone test in the parent process.
func TestMCPStdioHelper(t *testing.T) {
	home := os.Getenv(mcpE2EHomeEnv)
	if home == "" {
		t.Skip("stdio helper subprocess only")
	}
	svc, err := New(config.NewPaths(&config.Env{Home: home}), "e2e", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	if endpoint := os.Getenv(mcpE2EGitHubEnv); endpoint != "" {
		reader, err := github.NewClient(github.Config{
			BaseURL:    endpoint,
			UploadURL:  endpoint,
			HTTPClient: &http.Client{Timeout: 5 * time.Second},
		})
		if err != nil {
			t.Fatal(err)
		}
		svc.SetGitHubReader(reader)
	}
	server, err := mcpserver.New(svc.MCPReader(), "e2e")
	if err != nil {
		t.Fatal(err)
	}
	if err := server.ServeStdio(context.Background()); err != nil && !strings.Contains(err.Error(), "EOF") {
		t.Fatal(err)
	}
}

// The end-to-end flow intentionally verifies catalog discovery and the complete
// research sequence through a single real stdio session.
//
//nolint:cyclop
func TestMCPStdioScalableResearchFlow(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	home := t.TempDir()
	seedMCPStdioCorpus(ctx, t, home)
	githubServer := newMCPGitHubServer(t)
	defer githubServer.Close()
	command := exec.Command(os.Args[0], "-test.run=^TestMCPStdioHelper$")
	command.Env = append(os.Environ(), mcpE2EHomeEnv+"="+home, mcpE2EGitHubEnv+"="+githubServer.URL+"/")
	client := mcp.NewClient(&mcp.Implementation{Name: "gitcontribute-e2e", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	initialized := session.InitializeResult()
	if initialized == nil || initialized.ServerInfo == nil || initialized.ServerInfo.Name != "gitcontribute" {
		t.Fatalf("initialize result = %+v", initialized)
	}
	for _, phrase := range []string{
		"Prefer corpus tools for offline reads", "never refresh data implicitly", "explicit network reads",
		"poll advertised job tools in batches", "Missing or truncated coverage is unknown",
		"Only advertised tools are available", "never mutates GitHub",
	} {
		if !strings.Contains(initialized.Instructions, phrase) {
			t.Errorf("instructions missing %q: %s", phrase, initialized.Instructions)
		}
	}

	tools := make(map[string]*mcp.Tool)
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatal(err)
		}
		tools[tool.Name] = tool
	}
	for _, name := range []string{mcpcontract.ToolGetRepositories, mcpcontract.ToolGetThreads, mcpcontract.ToolRankThreads, mcpcontract.ToolFindPrecedents, mcpcontract.ToolSyncPortfolio, mcpcontract.ToolListPullRequestPortfolio, mcpcontract.ToolSearchGitHubRepositories, mcpcontract.ToolSyncRepositoryContext, mcpcontract.ToolSyncThreads, mcpcontract.ToolHydrateThreads, mcpcontract.ToolEnsureCoverage, mcpcontract.ToolGetSourceAuditWorkflow, mcpcontract.ToolQueryDeepWiki, mcpcontract.ToolIndexRepositories, mcpcontract.ToolCheckMergeConflicts} {
		if tools[name] == nil {
			t.Errorf("tools/list missing %s", name)
		}
	}

	contextJob := callMCPTool[mcpcontract.JobReference](ctx, t, session, mcpcontract.ToolSyncRepositoryContext, map[string]any{"repositories": []any{map[string]any{"owner": "acme", "repo": "observed"}}})
	contextResult := waitMCPJob(ctx, t, session, contextJob.ID)

	repositories := callMCPTool[mcpcontract.GetRepositoriesOutput](ctx, t, session, mcpcontract.ToolGetRepositories, map[string]any{"repositories": []any{map[string]any{"owner": "acme", "repo": "observed"}, map[string]any{"owner": "acme", "repo": "placeholder"}}})
	if len(repositories.Items) != 2 || repositories.Items[0].Value == nil || repositories.Items[0].Value.Stars == nil || *repositories.Items[0].Value.Stars != 9001 {
		t.Fatalf("observed repository batch = %+v, value = %+v, context job = %+v", repositories, repositories.Items[0].Value, contextResult)
	}
	if repositories.Items[1].Value == nil || repositories.Items[1].Value.Metadata.Status != "missing" || repositories.Items[1].Value.Stars != nil {
		t.Fatalf("placeholder exposed false metadata: %+v", repositories.Items[1])
	}

	threads := callMCPTool[mcpcontract.GetThreadsOutput](ctx, t, session, mcpcontract.ToolGetThreads, map[string]any{"threads": []any{map[string]any{"owner": "acme", "repo": "observed", "number": 1}, map[string]any{"owner": "acme", "repo": "observed", "number": 2}}, "view": "compact"})
	if len(threads.Items) != 2 || threads.Items[0].Value == nil || threads.Items[0].Value.Body != "" {
		t.Fatalf("compact thread batch = %+v", threads)
	}

	ranked := callMCPTool[mcpcontract.RankOpportunitiesOutput](ctx, t, session, mcpcontract.ToolRankThreads, map[string]any{"repositories": []any{map[string]any{"owner": "acme", "repo": "observed"}}, "limit": 10, "max_results_per_repository": 10})
	if len(ranked.Candidates) == 0 || ranked.Candidates[0].Number != 1 {
		t.Fatalf("ranked opportunities = %+v", ranked)
	}

	precedents := callMCPTool[mcpcontract.FindPrecedentsOutput](ctx, t, session, mcpcontract.ToolFindPrecedents, map[string]any{"threads": []any{map[string]any{"owner": "acme", "repo": "observed", "number": 1}}, "limit": 10})
	if precedents.Total == 0 || precedents.Items[0].Value == nil || precedents.Items[0].Value.Matches[0].Ref != "acme/observed#2" {
		t.Fatalf("precedents = %+v", precedents)
	}

	portfolio := callMCPTool[mcpcontract.ListPullRequestPortfolioOutput](ctx, t, session, mcpcontract.ToolListPullRequestPortfolio, map[string]any{"authors": []any{"morluto"}, "state": "open", "limit": 10, "view": "full"})
	if len(portfolio.PullRequests) != 1 || portfolio.PullRequests[0].Attention != "unknown" {
		t.Fatalf("portfolio = %+v", portfolio)
	}

	job := callMCPTool[mcpcontract.JobReference](ctx, t, session, mcpcontract.ToolBuildRepositoryDossier, map[string]any{"owner": "acme", "repo": "observed"})
	waitMCPJob(ctx, t, session, job.ID)

	invalid, err := session.CallTool(ctx, &mcp.CallToolParams{Name: mcpcontract.ToolHydrateThreads, Arguments: map[string]any{"threads": []any{map[string]any{"owner": "acme", "repo": "observed", "number": 1}}, "facets": []any{}}})
	if err != nil {
		t.Fatal(err)
	}
	if !invalid.IsError {
		t.Fatalf("empty facets accepted: %+v", invalid.StructuredContent)
	}
}

func TestMCPStdioPullRequestPortfolioFlow(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	home := t.TempDir()
	seedMCPStdioEmptyCorpus(ctx, t, home)
	githubServer := newMCPGitHubServer(t)
	defer githubServer.Close()
	command := exec.Command(os.Args[0], "-test.run=^TestMCPStdioHelper$")
	command.Env = append(os.Environ(), mcpE2EHomeEnv+"="+home, mcpE2EGitHubEnv+"="+githubServer.URL+"/")
	client := mcp.NewClient(&mcp.Implementation{Name: "gitcontribute-e2e", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	sync := callMCPTool[mcpcontract.JobReference](ctx, t, session, mcpcontract.ToolSyncPortfolio, map[string]any{"selection": "authored", "state": "open", "limit": 10})
	syncResult := waitMCPJob(ctx, t, session, sync.ID)

	portfolio := callMCPTool[mcpcontract.ListPullRequestPortfolioOutput](ctx, t, session, mcpcontract.ToolListPullRequestPortfolio, map[string]any{"authors": []any{"morluto"}, "state": "open", "limit": 10, "view": "full"})
	if len(portfolio.PullRequests) != 1 {
		t.Fatalf("portfolio = %+v, sync job = %+v", portfolio, syncResult)
	}
	pr := portfolio.PullRequests[0]
	if pr.Ref != "lab/project#7" || pr.Attention != "approved" || pr.ReviewDecision != "approved" || pr.Mergeable == nil || !*pr.Mergeable {
		t.Fatalf("portfolio PR = %+v", pr)
	}
	if pr.HeadSHA != "head123" || pr.BaseSHA != "base123" || pr.StatusCoverage != "complete" {
		t.Fatalf("portfolio status coverage = %+v", pr)
	}
	if pr.ChecksStatus != "passing" || pr.ChecksTotal != 1 || pr.UnresolvedReviewThreads == nil || *pr.UnresolvedReviewThreads != 0 || len(pr.ChangedFiles) != 1 {
		t.Fatalf("portfolio health = %+v", pr)
	}
}

func TestMCPStdioExactThreadSyncFlow(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	home := t.TempDir()
	seedMCPStdioEmptyCorpus(ctx, t, home)
	githubServer := newMCPGitHubServer(t)
	defer githubServer.Close()
	command := exec.Command(os.Args[0], "-test.run=^TestMCPStdioHelper$")
	command.Env = append(os.Environ(), mcpE2EHomeEnv+"="+home, mcpE2EGitHubEnv+"="+githubServer.URL+"/")
	client := mcp.NewClient(&mcp.Implementation{Name: "gitcontribute-e2e", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	threads := []any{
		map[string]any{"owner": "lab", "repo": "project", "kind": "issue", "number": 8},
		map[string]any{"owner": "lab", "repo": "project", "kind": "pull_request", "number": 7},
	}
	contextJob := callMCPTool[mcpcontract.JobReference](ctx, t, session, mcpcontract.ToolSyncRepositoryContext, map[string]any{
		"repositories": []any{map[string]any{"owner": "lab", "repo": "project"}},
	})
	waitMCPJob(ctx, t, session, contextJob.ID)
	for range 2 {
		job := callMCPTool[mcpcontract.JobReference](ctx, t, session, mcpcontract.ToolSyncThreads, map[string]any{
			"selection": "threads", "threads": threads,
		})
		waitMCPJob(ctx, t, session, job.ID)
		detailed := callMCPTool[mcpcontract.GetJobsOutput](ctx, t, session, mcpcontract.ToolGetJob, map[string]any{
			"ids": []string{job.ID}, "response_format": "detailed",
		})
		assertExactThreadJobItems(t, detailed, []string{"lab/project/issue#8", "lab/project/pull_request#7"})
	}
	inspection, err := New(config.NewPaths(&config.Env{Home: home}), "e2e", nil)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := inspection.ListCorpusInventory(ctx)
	if closeErr := inspection.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Repositories) != 1 || inventory.Repositories[0].ThreadObservations != 2 {
		t.Fatalf("repeated exact sync duplicated observations: %+v", inventory.Repositories)
	}

	stored := callMCPTool[mcpcontract.GetThreadsOutput](ctx, t, session, mcpcontract.ToolGetThreads, map[string]any{"threads": threads})
	if len(stored.Items) != 2 || stored.Items[0].Value == nil || stored.Items[0].Value.Kind != "issue" || stored.Items[1].Value == nil || stored.Items[1].Value.Kind != "pull_request" {
		t.Fatalf("exact stored threads = %+v", stored)
	}
	status := callMCPTool[mcpcontract.JobReference](ctx, t, session, mcpcontract.ToolSyncPortfolio, map[string]any{
		"selection":     "explicit",
		"pull_requests": []any{map[string]any{"owner": "lab", "repo": "project", "kind": "pull_request", "number": 7}},
	})
	waitMCPJob(ctx, t, session, status.ID)

	feedback := callMCPTool[mcpcontract.JobReference](ctx, t, session, mcpcontract.ToolSyncPullRequestFeedback, map[string]any{
		"pull_requests": []any{map[string]any{"owner": "lab", "repo": "project", "kind": "pull_request", "number": 7}},
		"channels":      []any{"issue_comments", "submitted_reviews", "inline_comments", "review_threads"},
		"thread_state":  "all",
		"max_requests":  10,
	})
	waitMCPJob(ctx, t, session, feedback.ID)
	feedbackJobs := callMCPTool[mcpcontract.GetJobsOutput](ctx, t, session, mcpcontract.ToolGetJob, map[string]any{"ids": []string{feedback.ID}, "response_format": "detailed"})
	feedbackResult := *feedbackJobs.Items[0].Value
	if len(feedbackResult.Artifacts) != 1 || len(feedbackResult.Artifacts[0].References) != 1 {
		t.Fatalf("feedback job = %+v", feedbackResult)
	}
	if _, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: feedbackResult.Artifacts[0].References[0]}); err != nil {
		t.Fatalf("read feedback resource: %v", err)
	}

	ci := callMCPTool[mcpcontract.JobReference](ctx, t, session, mcpcontract.ToolSyncCIFailures, map[string]any{
		"pull_requests":         []any{map[string]any{"owner": "lab", "repo": "project", "kind": "pull_request", "number": 7}},
		"logs":                  "none",
		"max_runs_per_pr":       5,
		"max_jobs_per_run":      5,
		"max_requests":          10,
		"max_log_bytes_per_job": 1024,
	})
	waitMCPJob(ctx, t, session, ci.ID)
	ciJobs := callMCPTool[mcpcontract.GetJobsOutput](ctx, t, session, mcpcontract.ToolGetJob, map[string]any{"ids": []string{ci.ID}, "response_format": "detailed"})
	ciResult := *ciJobs.Items[0].Value
	if len(ciResult.Artifacts) != 1 || len(ciResult.Artifacts[0].References) != 1 {
		t.Fatalf("CI artifacts = %+v", ciResult.Artifacts)
	}
	if _, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: ciResult.Artifacts[0].References[0]}); err != nil {
		t.Fatalf("read CI resource: %v", err)
	}
}

func assertExactThreadJobItems(t *testing.T, jobs mcpcontract.GetJobsOutput, wantKeys []string) {
	t.Helper()
	if len(jobs.Items) != 1 || jobs.Items[0].Value == nil {
		t.Fatalf("detailed job response = %+v", jobs)
	}
	value := jobs.Items[0].Value
	if value.ExecutionState != "terminal" || value.Outcome != "succeeded" || len(value.Artifacts) != 1 ||
		value.Artifacts[0].Kind != "thread_batch" || value.Artifacts[0].Count == nil ||
		int(*value.Artifacts[0].Count) != len(wantKeys) ||
		value.FollowUp == nil || value.FollowUp.Action.Type != "get_threads" {
		t.Fatalf("typed thread job summary = %+v", value)
	}
	if !slices.Equal(value.Artifacts[0].References, wantKeys) {
		t.Fatalf("typed thread job references = %v, want %v", value.Artifacts[0].References, wantKeys)
	}
}

func TestMCPStdioEnsureCoverageBootstrapsAndReturnsSnapshot(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	home := t.TempDir()
	seedMCPStdioEmptyCorpus(ctx, t, home)
	githubServer := newMCPGitHubServer(t)
	defer githubServer.Close()
	command := exec.Command(os.Args[0], "-test.run=^TestMCPStdioHelper$")
	command.Env = append(os.Environ(), mcpE2EHomeEnv+"="+home, mcpE2EGitHubEnv+"="+githubServer.URL+"/")
	client := mcp.NewClient(&mcp.Implementation{Name: "gitcontribute-e2e", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	job := callMCPTool[mcpcontract.JobReference](ctx, t, session, mcpcontract.ToolEnsureCoverage, map[string]any{
		"target": map[string]any{"type": "exact_thread", "repository": map[string]any{"owner": "lab", "repo": "project"}, "thread": map[string]any{"kind": "pull_request", "number": 7}},
		"facets": []any{"pr_details"}, "max_requests": 20, "max_pages": 1,
	})
	waitMCPJob(ctx, t, session, job.ID)
	detailed := callMCPTool[mcpcontract.GetJobsOutput](ctx, t, session, mcpcontract.ToolGetJob, map[string]any{"ids": []string{job.ID}, "response_format": "detailed"})
	if len(detailed.Items) != 1 || detailed.Items[0].Value == nil {
		t.Fatalf("coverage job = %+v", detailed)
	}
	value := detailed.Items[0].Value
	if value.Outcome != "succeeded" || len(value.Artifacts) != 1 || value.Artifacts[0].Kind != "corpus_snapshot" || value.Artifacts[0].URI == "" {
		t.Fatalf("coverage handoff = %+v", value)
	}
	resource, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: value.Artifacts[0].URI})
	if err != nil || len(resource.Contents) != 1 {
		t.Fatalf("read coverage snapshot %q: %+v, %v", value.Artifacts[0].URI, resource, err)
	}
}

func TestMCPStdioCoverageRecoveryFollowsReturnedAction(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	home := t.TempDir()
	seedMCPStdioEmptyCorpus(ctx, t, home)
	githubServer := newMCPGitHubServer(t)
	defer githubServer.Close()
	command := exec.Command(os.Args[0], "-test.run=^TestMCPStdioHelper$")
	command.Env = append(os.Environ(), mcpE2EHomeEnv+"="+home, mcpE2EGitHubEnv+"="+githubServer.URL+"/")
	client := mcp.NewClient(&mcp.Implementation{Name: "gitcontribute-e2e", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	before := callMCPTool[mcpcontract.GetCoverageOutput](ctx, t, session, mcpcontract.ToolGetCoverage, map[string]any{
		"targets": []any{map[string]any{
			"type": "exact_thread", "repository": map[string]any{"owner": "lab", "repo": "project"},
			"thread": map[string]any{"kind": "pull_request", "number": 7},
		}},
	})
	if before.Status != "partial" || len(before.Items) != 1 || before.Items[0].Recovery == nil || len(before.Items[0].Recovery.Then) != 1 {
		t.Fatalf("initial coverage recovery = %+v", before)
	}
	name, arguments := replayMCPRecoveryAction(t, before.Items[0].Recovery.Then[0])
	job := callMCPTool[mcpcontract.JobReference](ctx, t, session, name, arguments)
	waitMCPJob(ctx, t, session, job.ID)

	threads := callMCPTool[mcpcontract.GetThreadsOutput](ctx, t, session, mcpcontract.ToolGetThreads, map[string]any{
		"threads": []any{map[string]any{"owner": "lab", "repo": "project", "kind": "pull_request", "number": 7}},
		"view":    "full",
	})
	if threads.Status != "complete" || len(threads.Items) != 1 || threads.Items[0].Value == nil || threads.Items[0].Value.Title != "Fix cache lifecycle" || threads.Items[0].Value.Body != "Make cleanup deterministic" {
		t.Fatalf("synchronized thread reread = %+v", threads)
	}
}

func replayMCPRecoveryAction(t *testing.T, action mcpcontract.ToolCall) (string, map[string]any) {
	t.Helper()
	var name string
	var value any
	switch action.Type {
	case "ensure_coverage":
		name, value = mcpcontract.ToolEnsureCoverage, action.EnsureCoverage
	case "sync_threads":
		name, value = mcpcontract.ToolSyncThreads, action.SyncThreads
	case "hydrate_threads":
		name, value = mcpcontract.ToolHydrateThreads, action.HydrateThreads
	default:
		t.Fatalf("unsupported recovery action in integration test: %q", action.Type)
	}
	if value == nil {
		t.Fatalf("recovery action %q has no typed input", action.Type)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var arguments map[string]any
	if err := json.Unmarshal(data, &arguments); err != nil {
		t.Fatal(err)
	}
	return name, arguments
}

func seedMCPStdioCorpus(ctx context.Context, t *testing.T, home string) {
	t.Helper()
	svc, err := New(config.NewPaths(&config.Env{Home: home}), "e2e", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	if _, err := svc.Init(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-time.Hour)
	observed, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: "acme", Name: "observed"}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: "acme", Name: "placeholder"}, `{}`); err != nil {
		t.Fatal(err)
	}
	rows := []corpus.Thread{
		{RepositoryID: observed.ID, Kind: corpus.ThreadKindIssue, Number: 1, State: "open", Title: "cache root ignores configured path", Body: "compiled cache artifacts unexpectedly use tmp", Labels: []string{"bug", "help wanted"}, SourceUpdatedAt: now},
		{RepositoryID: observed.ID, Kind: corpus.ThreadKindPullRequest, Number: 2, State: "closed", Title: "honor configured cache root", Body: "move compiled cache artifacts away from tmp", Merged: true, MergedAt: now.Add(-time.Hour), ClosedAt: now.Add(-time.Hour), SourceUpdatedAt: now.Add(-time.Hour)},
		{RepositoryID: observed.ID, Kind: corpus.ThreadKindPullRequest, Number: 3, State: "open", Title: "current contributor work", Body: "portfolio entry", Author: "morluto", SourceUpdatedAt: now},
	}
	for _, row := range rows {
		if _, err := svc.corpus.UpsertThread(ctx, row, `{}`); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.corpus.AdvanceFacet(ctx, observed.ID, nil, "threads", now, true, 0); err != nil {
		t.Fatal(err)
	}
}

func seedMCPStdioEmptyCorpus(ctx context.Context, t *testing.T, home string) {
	t.Helper()
	svc, err := New(config.NewPaths(&config.Env{Home: home}), "e2e", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	if _, err := svc.Init(ctx); err != nil {
		t.Fatal(err)
	}
}

func newMCPGitHubServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/graphql") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"repository":{"pullRequest":{
				"id":"PR_7","updatedAt":"2026-07-18T22:00:00Z","headRefOid":"head123",
				"mergeStateStatus":"CLEAN","mergeable":"MERGEABLE","mergeQueueEntry":null,
				"closingIssuesReferences":{"totalCount":1,"nodes":[{"id":"I_9","number":9,"url":"https://github.com/lab/project/issues/9","repository":{"nameWithOwner":"lab/project"}}],"pageInfo":{"hasNextPage":false,"endCursor":""}},
				"files":{"totalCount":1,"nodes":[{"path":"internal/cache.go","changeType":"MODIFIED","additions":4,"deletions":2}],"pageInfo":{"hasNextPage":false,"endCursor":""}},
				"reviewThreads":{"totalCount":1,"nodes":[{"id":"RT_1","isResolved":true,"isOutdated":false,"path":"internal/cache.go","line":12,"startLine":12}],"pageInfo":{"hasNextPage":false,"endCursor":""}},
				"commits":{"nodes":[{"commit":{"statusCheckRollup":{"contexts":{"totalCount":1,"nodes":[{"__typename":"CheckRun","name":"test","status":"COMPLETED","conclusion":"SUCCESS","detailsUrl":"https://github.com/lab/project/actions","startedAt":"2026-07-18T21:00:00Z","completedAt":"2026-07-18T21:05:00Z"}],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}]}
			}}}}`))
			return
		}
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/repos/acme/observed"):
			_, _ = w.Write([]byte(`{
			"id": 101,
			"node_id": "R_repo",
			"name": "observed",
			"full_name": "acme/observed",
			"owner": {"login": "acme"},
			"description": "agent runtime cache correctness",
			"default_branch": "main",
			"language": "Go",
			"stargazers_count": 9001,
			"watchers_count": 9001,
			"forks_count": 42,
			"open_issues_count": 3,
			"archived": false,
			"fork": false,
			"updated_at": "2026-07-18T20:00:00Z",
				"pushed_at": "2026-07-18T19:00:00Z"
			}`))
		case strings.HasSuffix(r.URL.Path, "/repos/lab/project"):
			_, _ = w.Write([]byte(`{"id":202,"node_id":"R_project","name":"project","full_name":"lab/project","owner":{"login":"lab"},"default_branch":"main","language":"Go","updated_at":"2026-07-18T20:00:00Z"}`))
		case strings.HasSuffix(r.URL.Path, "/user"):
			_, _ = w.Write([]byte(`{"login":"morluto","id":99,"node_id":"U_99"}`))
		case strings.HasSuffix(r.URL.Path, "/search/issues"):
			_, _ = w.Write([]byte(`{
				"total_count":1,
				"incomplete_results":false,
				"items":[{
					"id":700,"node_id":"PR_7","number":7,"state":"open","title":"Fix cache lifecycle",
					"body":"Make cleanup deterministic","user":{"login":"morluto"},
					"repository_url":"https://api.github.test/repos/lab/project",
					"html_url":"https://github.com/lab/project/pull/7",
					"pull_request":{"url":"https://api.github.test/repos/lab/project/pulls/7","html_url":"https://github.com/lab/project/pull/7"},
					"created_at":"2026-07-15T10:00:00Z","updated_at":"2026-07-18T20:00:00Z"
				}]
			}`))
		case strings.HasSuffix(r.URL.Path, "/repos/lab/project/issues/7"):
			_, _ = w.Write([]byte(`{
				"id":700,"node_id":"PR_7","number":7,"state":"open","title":"Fix cache lifecycle",
				"body":"Make cleanup deterministic","user":{"login":"morluto"},
				"repository_url":"https://api.github.test/repos/lab/project",
				"html_url":"https://github.com/lab/project/pull/7",
				"pull_request":{"url":"https://api.github.test/repos/lab/project/pulls/7","html_url":"https://github.com/lab/project/pull/7"},
				"created_at":"2026-07-15T10:00:00Z","updated_at":"2026-07-18T20:00:00Z"
			}`))
		case strings.HasSuffix(r.URL.Path, "/repos/lab/project/issues/8"):
			_, _ = w.Write([]byte(`{
				"id":800,"node_id":"I_8","number":8,"state":"open","title":"Document exact sync",
				"body":"Keep refresh scope narrow","user":{"login":"morluto"},
				"repository_url":"https://api.github.test/repos/lab/project",
				"html_url":"https://github.com/lab/project/issues/8",
				"created_at":"2026-07-15T10:00:00Z","updated_at":"2026-07-18T20:00:00Z"
			}`))
		case strings.HasSuffix(r.URL.Path, "/repos/lab/project/pulls/7/reviews"):
			_, _ = w.Write([]byte(`[{"id":701,"node_id":"R_701","state":"APPROVED","user":{"login":"reviewer"},"commit_id":"head123","submitted_at":"2026-07-18T21:00:00Z"}]`))
		case strings.HasSuffix(r.URL.Path, "/repos/lab/project/issues/7/comments"):
			_, _ = w.Write([]byte(`[{"id":702,"body":"top-level feedback","user":{"login":"reviewer"},"created_at":"2026-07-18T21:00:00Z","updated_at":"2026-07-18T21:00:00Z"}]`))
		case strings.HasSuffix(r.URL.Path, "/repos/lab/project/pulls/7/comments"):
			_, _ = w.Write([]byte(`[{"id":703,"node_id":"C_703","body":"inline feedback","path":"internal/cache.go","line":12,"commit_id":"head123","user":{"login":"reviewer"},"created_at":"2026-07-18T21:00:00Z","updated_at":"2026-07-18T21:00:00Z"}]`))
		case strings.HasSuffix(r.URL.Path, "/repos/lab/project/commits/head123/status"):
			_, _ = w.Write([]byte(`{"state":"success","statuses":[{"context":"external","state":"success"}]}`))
		case strings.HasSuffix(r.URL.Path, "/repos/lab/project/commits/head123/check-runs"):
			_, _ = w.Write([]byte(`{"total_count":1,"check_runs":[{"id":1,"name":"test","status":"completed","conclusion":"success"}]}`))
		case strings.HasSuffix(r.URL.Path, "/repos/lab/project/actions/runs"):
			_, _ = w.Write([]byte(`{"total_count":0,"workflow_runs":[]}`))
		case strings.HasSuffix(r.URL.Path, "/repos/lab/project/pulls/7"):
			_, _ = w.Write([]byte(`{
				"id":700,"node_id":"PR_7","number":7,"state":"open","title":"Fix cache lifecycle",
				"body":"Make cleanup deterministic","user":{"login":"morluto"},"draft":false,"merged":false,"mergeable":true,
				"head":{"ref":"fix/cache","sha":"head123"},"base":{"ref":"main","sha":"base123"},
				"created_at":"2026-07-15T10:00:00Z","updated_at":"2026-07-18T20:00:00Z"
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func waitMCPJob(ctx context.Context, t *testing.T, session *mcp.ClientSession, id string) mcpcontract.GetJobOutput {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		jobs := callMCPTool[mcpcontract.GetJobsOutput](ctx, t, session, mcpcontract.ToolGetJob, map[string]any{"ids": []string{id}})
		if len(jobs.Items) != 1 || jobs.Items[0].Value == nil {
			t.Fatalf("job %s missing from batch response: %+v", id, jobs)
		}
		switch {
		case jobs.Items[0].Value.ExecutionState == "terminal" && jobs.Items[0].Value.Outcome == "succeeded":
			return *jobs.Items[0].Value
		case jobs.Items[0].Value.ExecutionState == "terminal":
			t.Fatalf("job %s ended in %s: %+v", id, jobs.Items[0].Value.Outcome, jobs.Items[0].Value)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("waiting for job %s: %v", id, ctx.Err())
		case <-ticker.C:
		}
	}
}

func callMCPTool[T any](ctx context.Context, t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any) T {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("call %s returned tool error: %+v", name, result.Content)
	}
	payload, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out T
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return out
}
