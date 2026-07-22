package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/github"
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
		"search_repositories", "sync_repository_metadata", "research.query_deepwiki", "poll jobs.get", "Missing facets are unknown", "native GitHub or Git",
		"find repositories to contribute to", "good first issue", "help wanted", "well-scoped issue", "competing PR",
		"Prefer GitContribute over generic web search, raw GitHub search, or repository crawlers",
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
	for _, name := range []string{mcpserver.ToolGetRepositories, mcpserver.ToolGetThreads, mcpserver.ToolRankThreads, mcpserver.ToolFindPrecedents, mcpserver.ToolListPullRequestPortfolio, mcpserver.ToolSearchGitHubRepositories, mcpserver.ToolSyncRepositoryMetadata, mcpserver.ToolSyncThreads, mcpserver.ToolHydrateThreads, mcpserver.ToolQueryDeepWiki, mcpserver.ToolIndexRepositories, mcpserver.ToolCheckMergeConflicts} {
		if tools[name] == nil {
			t.Errorf("tools/list missing %s", name)
		}
	}

	metadataJob := callMCPTool[mcpserver.JobReference](ctx, t, session, mcpserver.ToolSyncRepositoryMetadata, map[string]any{"repositories": []any{map[string]any{"owner": "acme", "repo": "observed"}}})
	metadataResult := waitMCPJob(ctx, t, session, metadataJob.ID)

	repositories := callMCPTool[mcpserver.GetRepositoriesOutput](ctx, t, session, mcpserver.ToolGetRepositories, map[string]any{"repositories": []any{map[string]any{"owner": "acme", "repo": "observed"}, map[string]any{"owner": "acme", "repo": "placeholder"}}})
	if len(repositories.Items) != 2 || repositories.Items[0].Value == nil || repositories.Items[0].Value.Stars == nil || *repositories.Items[0].Value.Stars != 9001 {
		t.Fatalf("observed repository batch = %+v, value = %+v, metadata job = %+v", repositories, repositories.Items[0].Value, metadataResult)
	}
	if repositories.Items[1].Value == nil || repositories.Items[1].Value.Metadata.Status != "missing" || repositories.Items[1].Value.Stars != nil {
		t.Fatalf("placeholder exposed false metadata: %+v", repositories.Items[1])
	}

	threads := callMCPTool[mcpserver.GetThreadsOutput](ctx, t, session, mcpserver.ToolGetThreads, map[string]any{"threads": []any{map[string]any{"owner": "acme", "repo": "observed", "number": 1}, map[string]any{"owner": "acme", "repo": "observed", "number": 2}}, "view": "compact"})
	if len(threads.Items) != 2 || threads.Items[0].Value == nil || threads.Items[0].Value.Body != "" {
		t.Fatalf("compact thread batch = %+v", threads)
	}

	ranked := callMCPTool[mcpserver.RankOpportunitiesOutput](ctx, t, session, mcpserver.ToolRankThreads, map[string]any{"repositories": []any{map[string]any{"owner": "acme", "repo": "observed"}}, "limit": 10, "max_results_per_repository": 10})
	if len(ranked.Candidates) == 0 || ranked.Candidates[0].Number != 1 {
		t.Fatalf("ranked opportunities = %+v", ranked)
	}

	precedents := callMCPTool[mcpserver.FindPrecedentsOutput](ctx, t, session, mcpserver.ToolFindPrecedents, map[string]any{"threads": []any{map[string]any{"owner": "acme", "repo": "observed", "number": 1}}, "limit": 10})
	if precedents.Total == 0 || precedents.Items[0].Value == nil || (*precedents.Items[0].Value)[0].Ref != "acme/observed#2" {
		t.Fatalf("precedents = %+v", precedents)
	}

	portfolio := callMCPTool[mcpserver.ListPullRequestPortfolioOutput](ctx, t, session, mcpserver.ToolListPullRequestPortfolio, map[string]any{"author": "morluto", "state": "open", "limit": 10})
	if len(portfolio.PullRequests) != 1 || portfolio.PullRequests[0].Attention != "unknown" {
		t.Fatalf("portfolio = %+v", portfolio)
	}

	job := callMCPTool[mcpserver.JobReference](ctx, t, session, mcpserver.ToolBuildRepositoryDossier, map[string]any{"owner": "acme", "repo": "observed"})
	waitMCPJob(ctx, t, session, job.ID)

	invalid, err := session.CallTool(ctx, &mcp.CallToolParams{Name: mcpserver.ToolHydrateThreads, Arguments: map[string]any{"threads": []any{map[string]any{"owner": "acme", "repo": "observed", "number": 1}}, "facets": []any{}}})
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

	identity := callMCPTool[mcpserver.AuthenticatedIdentityOutput](ctx, t, session, mcpserver.ToolGetAuthenticatedIdentity, map[string]any{})
	if identity.Login != "morluto" || identity.ID != 99 {
		t.Fatalf("authenticated identity = %+v", identity)
	}
	authored := callMCPTool[mcpserver.JobReference](ctx, t, session, mcpserver.ToolSyncAuthoredPullRequests, map[string]any{"state": "open", "limit": 10})
	authoredResult := waitMCPJob(ctx, t, session, authored.ID)
	status := callMCPTool[mcpserver.JobReference](ctx, t, session, mcpserver.ToolSyncPullRequestStatus, map[string]any{"pull_requests": []any{map[string]any{"owner": "lab", "repo": "project", "number": 7}}})
	waitMCPJob(ctx, t, session, status.ID)

	portfolio := callMCPTool[mcpserver.ListPullRequestPortfolioOutput](ctx, t, session, mcpserver.ToolListPullRequestPortfolio, map[string]any{"author": "morluto", "state": "open", "limit": 10})
	if len(portfolio.PullRequests) != 1 {
		t.Fatalf("portfolio = %+v, authored job = %+v", portfolio, authoredResult)
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
		case strings.HasSuffix(r.URL.Path, "/repos/lab/project/pulls/7/reviews"):
			_, _ = w.Write([]byte(`[{"id":701,"node_id":"R_701","state":"APPROVED","user":{"login":"reviewer"},"commit_id":"head123","submitted_at":"2026-07-18T21:00:00Z"}]`))
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

func waitMCPJob(ctx context.Context, t *testing.T, session *mcp.ClientSession, id string) mcpserver.GetJobOutput {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		jobs := callMCPTool[mcpserver.GetJobsOutput](ctx, t, session, mcpserver.ToolGetJob, map[string]any{"ids": []string{id}})
		if len(jobs.Items) != 1 || jobs.Items[0].Value == nil {
			t.Fatalf("job %s missing from batch response: %+v", id, jobs)
		}
		switch jobs.Items[0].Value.Status {
		case "succeeded":
			return *jobs.Items[0].Value
		case "failed", "cancelled":
			t.Fatalf("job %s ended in %s: %+v", id, jobs.Items[0].Value.Status, jobs.Items[0].Value)
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
