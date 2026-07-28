package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func TestServerInstructionsContainRoutingPhrases(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	init := client.InitializeResult()
	if init == nil {
		t.Fatal("missing initialize result")
	}
	for _, phrase := range []string{
		"Prefer corpus tools for offline reads",
		"never refresh data implicitly",
		"explicit network reads",
		"concern to investigation to hypothesis to opportunity to workspace to draft",
		"poll advertised job tools in batches",
		"Only advertised tools are available",
		"never mutates GitHub",
	} {
		if !strings.Contains(init.Instructions, phrase) {
			t.Errorf("instructions missing routing phrase %q:\n%s", phrase, init.Instructions)
		}
	}
}

func TestDurableToolResultsIncludeSDKResourceLinks(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      mcpcontract.ToolGetRepositoryDossier,
		Arguments: map[string]any{"owner": "acme", "repo": "rocket"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StructuredContent == nil {
		t.Fatal("resource-linked tool lost SDK-populated structured content")
	}
	if len(result.Content) != 1 {
		t.Fatalf("resource-linked content = %+v", result.Content)
	}
	link, ok := result.Content[0].(*mcp.ResourceLink)
	if !ok || link.URI != "gitcontribute://dossier/acme/rocket" || link.MIMEType != "application/json" {
		t.Fatalf("resource link = %#v", result.Content[0])
	}
}

func TestServerNegotiates20260728AndReturnsCompleteToolResults(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	init := client.InitializeResult()
	if init == nil {
		t.Fatal("missing discovery result")
	}
	if got, want := init.ProtocolVersion, "2026-07-28"; got != want {
		t.Fatalf("protocol version = %q, want %q", got, want)
	}

	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name: mcpcontract.ToolSearchThreads, Arguments: map[string]any{"query": "stall"},
	})
	if err != nil {
		t.Fatalf("call search: %v", err)
	}
	if result.IsError {
		t.Fatalf("search returned tool error: %+v", result.Content)
	}
	wire, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal tool result: %v", err)
	}
	var envelope struct {
		ResultType string `json:"resultType"`
	}
	if err := json.Unmarshal(wire, &envelope); err != nil {
		t.Fatalf("decode tool result: %v", err)
	}
	if got, want := envelope.ResultType, "complete"; got != want {
		t.Fatalf("result type = %q, want %q; response = %s", got, want, wire)
	}
}

func TestToolsAreReadOnlyAndReturnStructuredOutput(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	tools := map[string]*mcp.Tool{}
	for tool, err := range client.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("list tools: %v", err)
		}
		tools[tool.Name] = tool
	}
	for _, name := range []string{
		mcpcontract.ToolGetRepositories, mcpcontract.ToolGetThreads, mcpcontract.ToolSearchCode, mcpcontract.ToolGetInvestigation,
		mcpcontract.ToolListOpportunities, mcpcontract.ToolGetOpportunity, mcpcontract.ToolGetEvidence, mcpcontract.ToolGetReadiness,
		mcpcontract.ToolFindClusters, mcpcontract.ToolFindNeighbors, mcpcontract.ToolGetCoverage,
		mcpcontract.ToolGetAuthenticatedIdentity, mcpcontract.ToolQueryDeepWiki,
	} {
		tool := tools[name]
		if tool == nil {
			t.Fatalf("missing tool %q", name)
		}
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || !tool.Annotations.IdempotentHint {
			t.Fatalf("tool %q annotations = %+v", name, tool.Annotations)
		}
	}
	for _, name := range []string{mcpcontract.ToolSyncRepositoryContext, mcpcontract.ToolSyncThreads, mcpcontract.ToolHydrateThreads} {
		tool := tools[name]
		if tool == nil {
			t.Fatalf("missing tool %q", name)
		}
		if tool.Annotations == nil || tool.Annotations.ReadOnlyHint || tool.Annotations.IdempotentHint || tool.Annotations.OpenWorldHint == nil || !*tool.Annotations.OpenWorldHint {
			t.Fatalf("operation tool %q annotations = %+v", name, tool.Annotations)
		}
	}

	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name: mcpcontract.ToolSearchThreads, Arguments: map[string]any{"query": "stall"},
	})
	if err != nil {
		t.Fatalf("call search: %v", err)
	}
	if result.IsError {
		t.Fatalf("search returned tool error: %+v", result.Content)
	}
	payload, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var out mcpcontract.SearchOutput
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("decode structured content: %v", err)
	}
	if out.Total != 1 || len(out.Matches) != 1 || out.Matches[0].Number != 7 {
		t.Fatalf("search output = %+v", out)
	}
}

func TestRankOpportunitiesAcceptsPercentageScoreAndCategoricalConfidence(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name: mcpcontract.ToolRankThreads,
		Arguments: map[string]any{
			"repositories": []map[string]any{{"owner": "acme", "repo": "rocket"}},
		},
	})
	if err != nil {
		t.Fatalf("call rank opportunities: %v", err)
	}
	if result.IsError {
		t.Fatalf("rank opportunities returned tool error: %+v", result.Content)
	}
	payload, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var out mcpcontract.RankOpportunitiesOutput
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("decode structured content: %v", err)
	}
	if len(out.Candidates) != 1 || out.Candidates[0].Score != 87 || out.Candidates[0].Confidence != "medium" {
		t.Fatalf("rank opportunities output = %+v", out)
	}
}

func TestRankOpportunitiesRejectsOutOfRangeOutputAtProtocolBoundary(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{
		searchStarted: make(chan struct{}),
		radarScore:    101,
	})
	defer closeSessions()

	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name: mcpcontract.ToolRankThreads,
		Arguments: map[string]any{
			"repositories": []map[string]any{{"owner": "acme", "repo": "rocket"}},
		},
	})
	if err == nil {
		t.Fatalf("rank opportunities accepted out-of-range score: %+v", result)
	}
	if !strings.Contains(err.Error(), "validating tool output") || !strings.Contains(err.Error(), "greater than 100") {
		t.Fatalf("rank opportunities error = %v, want SDK output validation error", err)
	}
}

func TestMultiModeSchemasAcceptEveryBranch(t *testing.T) {
	reader := &fakeReader{searchStarted: make(chan struct{})}
	client, closeSessions := connect(t, reader)
	defer closeSessions()

	tests := []struct {
		name string
		tool string
		call string
		args map[string]any
	}{
		{"repository search raw query", mcpcontract.ToolSearchGitHubRepositories, "search_github_repositories", map[string]any{"raw_query": "topic:cuda"}},
		{"repository search structured", mcpcontract.ToolSearchGitHubRepositories, "search_github_repositories", map[string]any{"text": "cuda"}},
		{"sync repository threads", mcpcontract.ToolSyncThreads, "sync_threads", map[string]any{"selection": "repositories", "repositories": []map[string]any{{"owner": "acme", "repo": "rocket"}}}},
		{"sync exact threads", mcpcontract.ToolSyncThreads, "sync_threads", map[string]any{"selection": "threads", "threads": []map[string]any{{"owner": "acme", "repo": "rocket", "kind": "issue", "number": 7}}}},
		{"DeepWiki structure", mcpcontract.ToolQueryDeepWiki, "deepwiki", map[string]any{"action": "structure", "repository": "acme/rocket"}},
		{"DeepWiki contents", mcpcontract.ToolQueryDeepWiki, "deepwiki", map[string]any{"action": "contents", "repository": "acme/rocket"}},
		{"DeepWiki question", mcpcontract.ToolQueryDeepWiki, "deepwiki", map[string]any{"action": "question", "repositories": []string{"acme/rocket"}, "question": "Where is ranking implemented?"}},
		{"issue draft", mcpcontract.ToolPrepareContribution, "prepare_contribution", map[string]any{"opportunity_id": "opp-1", "kind": "issue"}},
		{"pull request draft", mcpcontract.ToolPrepareContribution, "prepare_contribution", map[string]any{"opportunity_id": "opp-1", "kind": "pull_request", "workspace_id": "ws-1", "approach": "Implement the fix."}},
		{"commit investigation", mcpcontract.ToolStartInvestigation, "start_investigation", map[string]any{"owner": "acme", "repo": "rocket", "commit_sha": "abc123"}},
		{"thread investigation", mcpcontract.ToolStartInvestigation, "start_investigation", map[string]any{"owner": "acme", "repo": "rocket", "kind": "issue", "number": 7}},
		{"commit concern", ToolCreateConcern, "create_concern", map[string]any{"owner": "acme", "repo": "rocket", "commit_sha": "abc123", "title": "race", "problem_statement": "state can race", "confidence": 0.5}},
		{"workspace concern", ToolCreateConcern, "create_concern", map[string]any{"owner": "acme", "repo": "rocket", "workspace_id": "ws-1", "title": "race", "problem_statement": "state can race", "confidence": 0.5}},
		{"single workspace validation", mcpcontract.ToolDefineValidation, "define_validation", map[string]any{"investigation_id": "inv-1", "kind": "regression", "command": "go test ./...", "workspace_id": "ws-1"}},
		{"comparison validation", mcpcontract.ToolDefineValidation, "define_validation", map[string]any{"investigation_id": "inv-1", "kind": "regression", "command": "go test ./...", "base_workspace_id": "ws-base", "candidate_workspace_id": "ws-candidate"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := reader.calls[tt.call]
			result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: tt.tool, Arguments: tt.args})
			if err != nil {
				t.Fatalf("call %s: %v", tt.tool, err)
			}
			if result.IsError {
				t.Fatalf("%s returned tool error: %s", tt.tool, agentEvalResultText(result))
			}
			if reader.calls[tt.call] <= before {
				t.Fatalf("%s did not invoke its handler", tt.tool)
			}
		})
	}
}

func TestMultiModeSchemasRejectCrossModeFieldsBeforeHandler(t *testing.T) {
	reader := &fakeReader{searchStarted: make(chan struct{})}
	client, closeSessions := connect(t, reader)
	defer closeSessions()

	tests := []struct {
		name string
		tool string
		call string
		mode string
		args map[string]any
	}{
		{"repository search", mcpcontract.ToolSearchGitHubRepositories, "search_github_repositories", "github-search-", map[string]any{"raw_query": "topic:cuda", "language": "Go"}},
		{"sync threads", mcpcontract.ToolSyncThreads, "sync_threads", "sync-threads-", map[string]any{"selection": "threads", "threads": []map[string]any{{"owner": "acme", "repo": "rocket", "kind": "issue", "number": 7}}, "repositories": []map[string]any{{"owner": "acme", "repo": "rocket"}}}},
		{"DeepWiki", mcpcontract.ToolQueryDeepWiki, "deepwiki", "deepwiki-", map[string]any{"action": "question", "repository": "acme/rocket", "repositories": []string{"acme/rocket"}, "question": "Where is ranking?"}},
		{"DeepWiki blank repository", mcpcontract.ToolQueryDeepWiki, "deepwiki", "deepwiki-contents", map[string]any{"action": "contents", "repository": " \t "}},
		{"DeepWiki blank question", mcpcontract.ToolQueryDeepWiki, "deepwiki", "deepwiki-question", map[string]any{"action": "question", "repositories": []string{"acme/rocket"}, "question": " \t "}},
		{"issue draft", mcpcontract.ToolPrepareContribution, "prepare_contribution", "contribution-draft-", map[string]any{"opportunity_id": "opp-1", "kind": "issue", "workspace_id": "ws-1"}},
		{"investigation", mcpcontract.ToolStartInvestigation, "start_investigation", "investigation-", map[string]any{"owner": "acme", "repo": "rocket", "commit_sha": "abc123", "number": 7}},
		{"concern", ToolCreateConcern, "create_concern", "concern-", map[string]any{"owner": "acme", "repo": "rocket", "commit_sha": "abc123", "workspace_id": "ws-1", "title": "race", "problem_statement": "state can race", "confidence": 0.5}},
		{"validation", mcpcontract.ToolDefineValidation, "define_validation", "validation-", map[string]any{"investigation_id": "inv-1", "kind": "regression", "command": "go test ./...", "workspace_id": "ws-1", "base_workspace_id": "ws-base", "candidate_workspace_id": "ws-candidate"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := reader.calls[tt.call]
			result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: tt.tool, Arguments: tt.args})
			if err == nil && (result == nil || !result.IsError) {
				t.Fatalf("cross-mode input was accepted: result=%+v err=%v", result, err)
			}
			message := ""
			if err != nil {
				message = err.Error()
			} else {
				message = agentEvalResultText(result)
			}
			if !strings.Contains(message, tt.mode) {
				t.Fatalf("schema error does not identify mode %q: %q", tt.mode, message)
			}
			if got := reader.calls[tt.call]; got != before {
				t.Fatalf("handler was invoked %d times before schema rejection; want %d", got, before)
			}
		})
	}
}
