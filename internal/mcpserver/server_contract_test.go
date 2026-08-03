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
		"corpus.* tools are offline reads",
		"never refresh implicitly",
		"explicit bounded network reads",
		"missing, stale, paginated, or truncated observations are unknown",
		"polling through jobs.get",
		"exact returned resource URIs",
		"github.search_users",
		"github.sync_user_*",
		"Only advertised tools are available",
		"never mutates GitHub",
	} {
		if !strings.Contains(init.Instructions, phrase) {
			t.Errorf("instructions missing routing phrase %q:\n%s", phrase, init.Instructions)
		}
	}
}

func TestCatalogContractMatchesAdvertisedFeedbackRoute(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	tools := make(map[string]bool)
	for tool, err := range client.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		tools[tool.Name] = true
	}
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: mcpcontract.ToolGetCatalogContract, Arguments: map[string]any{}})
	if err != nil || result == nil || result.IsError {
		t.Fatalf("catalog contract call failed: result=%+v err=%v", result, err)
	}
	var contract mcpcontract.CatalogContract
	data, err := json.Marshal(result.StructuredContent)
	if err != nil || json.Unmarshal(data, &contract) != nil {
		t.Fatalf("decode catalog contract: result=%#v err=%v", result.StructuredContent, err)
	}
	if contract.SchemaVersion != mcpcontract.CatalogContractVersion || contract.ServerName != "gitcontribute" || contract.ServerVersion != "test" || contract.CatalogMode != "all" || contract.CatalogFingerprint == "" || contract.ToolCount != len(tools) {
		t.Fatalf("catalog contract identity = %+v, advertised tools=%d", contract, len(tools))
	}
	route := contract.PullRequestFeedback
	if !route.IndexAdvertised || !route.SyncAdvertised || !route.SearchAdvertised || len(route.CanonicalAsyncChain) != 3 || route.CanonicalAsyncChain[0] != mcpcontract.ToolIndexPullRequestFeedback || route.CanonicalAsyncChain[1] != mcpcontract.ToolGetJob || route.CanonicalAsyncChain[2] != mcpcontract.ToolSearchPullRequestFeedback {
		t.Fatalf("feedback route = %+v", route)
	}
	for _, name := range []string{route.IndexTool, route.SyncTool, route.SearchTool} {
		if !tools[name] {
			t.Errorf("catalog contract names unadvertised tool %q", name)
		}
	}
}

func TestCatalogContractExplainsReadOnlyFeedbackFiltering(t *testing.T) {
	client, closeSessions := connectServer(t, &fakeReader{searchStarted: make(chan struct{})}, true)
	defer closeSessions()

	tools := make(map[string]*mcp.Tool)
	for tool, err := range client.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		tools[tool.Name] = tool
	}
	catalog := callCatalogContract(t, client)
	if catalog.CatalogMode != "read_only" || catalog.ToolCount != len(tools) {
		t.Fatalf("read-only catalog contract = %+v, tools = %d", catalog, len(tools))
	}
	route := catalog.PullRequestFeedback
	if route.IndexAdvertised || route.SyncAdvertised || !route.SearchAdvertised {
		t.Fatalf("read-only feedback route = %+v", route)
	}
}

func callCatalogContract(t *testing.T, client *mcp.ClientSession) mcpcontract.CatalogContract {
	t.Helper()
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: mcpcontract.ToolGetCatalogContract, Arguments: map[string]any{}})
	if err != nil || result == nil || result.IsError {
		t.Fatalf("catalog contract call failed: result=%+v err=%v", result, err)
	}
	var catalog mcpcontract.CatalogContract
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestFeedbackToolDescriptionsDeclareRoutingBoundaries(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()
	tools := make(map[string]*mcp.Tool)
	for tool, err := range client.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		tools[tool.Name] = tool
	}
	cases := map[string][]string{
		mcpcontract.ToolIndexPullRequestFeedback:  {"repository-wide audits", "bounded GitHub reads", "Poll jobs.get", mcpcontract.ToolSearchPullRequestFeedback, "never mutates GitHub"},
		mcpcontract.ToolSyncPullRequestFeedback:   {"exact pull requests", "bounded GitHub network reads", "Poll jobs.get", mcpcontract.ToolIndexPullRequestFeedback, "never mutates GitHub"},
		mcpcontract.ToolSearchPullRequestFeedback: {"offline search", "after github.index_pull_request_feedback and jobs.get", "never contacts GitHub", "partial/unknown coverage"},
		mcpcontract.ToolSearchThreads:             {"not a comment-level feedback search", mcpcontract.ToolSearchPullRequestFeedback},
	}
	for name, phrases := range cases {
		tool := tools[name]
		if tool == nil {
			t.Fatalf("missing tool %q", name)
		}
		for _, phrase := range phrases {
			if !strings.Contains(tool.Description, phrase) {
				t.Errorf("tool %q description missing %q: %s", name, phrase, tool.Description)
			}
		}
	}
}

func TestSourceAuditContractUsesAdvertisedOperations(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	tools := make(map[string]bool)
	for tool, err := range client.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		tools[tool.Name] = true
	}
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: mcpcontract.ToolGetSourceAuditWorkflow, Arguments: map[string]any{}})
	if err != nil || result == nil || result.IsError {
		t.Fatalf("source-audit contract call failed: result=%+v err=%v", result, err)
	}
	var workflow mcpcontract.SourceAuditWorkflow
	data, err := json.Marshal(result.StructuredContent)
	if err != nil || json.Unmarshal(data, &workflow) != nil {
		t.Fatalf("decode source-audit contract: result=%#v err=%v", result.StructuredContent, err)
	}
	for _, transition := range workflow.Transitions {
		for _, operation := range append([]string{transition.Operation}, transition.AllowedNextActions...) {
			if !tools[operation] {
				t.Errorf("source-audit transition %q references unadvertised operation %q", transition.ID, operation)
			}
		}
	}
}

func TestDurableToolResultsIncludeSDKResourceLinks(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name: mcpcontract.ToolStartInvestigation,
		Arguments: map[string]any{
			"owner": "acme", "repo": "rocket", "commit_sha": "abc123",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StructuredContent == nil {
		t.Fatal("resource-linked tool lost SDK-populated structured content")
	}
	if len(result.Content) != 2 {
		t.Fatalf("resource-linked content = %+v", result.Content)
	}
	instruction, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("resource instruction = %#v", result.Content[0])
	}
	for _, phrase := range []string{
		"perform MCP `resources/read` with this server",
		"in Codex, call `read_mcp_resource`",
		`exact URI "gitcontribute://investigation/inv-1"`,
		"copy it verbatim without shortening, pluralizing, or reconstructing it",
		"Do not substitute structured tool output for the resource read",
	} {
		if !strings.Contains(instruction.Text, phrase) {
			t.Errorf("resource instruction missing %q: %q", phrase, instruction.Text)
		}
	}
	link, ok := result.Content[1].(*mcp.ResourceLink)
	if !ok || link.URI != "gitcontribute://investigation/inv-1" || link.MIMEType != "application/json" {
		t.Fatalf("resource link = %#v", result.Content[1])
	}
	for _, phrase := range []string{"exact opaque URI unchanged", "do not shorten, pluralize, or reconstruct it"} {
		if !strings.Contains(link.Description, phrase) {
			t.Errorf("resource link description missing %q: %q", phrase, link.Description)
		}
	}
}

func TestDurableProducerReferencesRoundTripThroughResources(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	tests := []struct {
		name string
		tool string
		args map[string]any
		uri  string
		kind string
	}{
		{
			name: "record hypothesis returns parent investigation",
			tool: mcpcontract.ToolRecordHypothesis,
			args: map[string]any{"investigation_id": "inv-1", "title": "leak", "description": "memory leak", "category": "bug"},
			uri:  "gitcontribute://investigation/inv-1",
			kind: "investigation",
		},
		{
			name: "create concern",
			tool: ToolCreateConcern,
			args: map[string]any{"owner": "acme", "repo": "rocket", "commit_sha": "abc123", "title": "stall", "problem_statement": "requests stall", "confidence": 0.5},
			uri:  "gitcontribute://concern/concern-1",
			kind: "concern",
		},
		{
			name: "prepare immutable draft",
			tool: mcpcontract.ToolPrepareContribution,
			args: map[string]any{"opportunity_id": "opp-1", "kind": "issue"},
			uri:  "gitcontribute://draft/draft-1/1",
			kind: "draft",
		},
		{
			name: "export manifest",
			tool: mcpcontract.ToolExportManifest,
			args: map[string]any{"opportunity_id": "opp-1"},
			uri:  "gitcontribute://manifest/sha256%3Atest",
			kind: "manifest",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: tt.tool, Arguments: tt.args})
			if err != nil || result.IsError {
				t.Fatalf("call %s: err=%v result=%+v", tt.tool, err, result)
			}
			payload, err := json.Marshal(result.StructuredContent)
			if err != nil {
				t.Fatal(err)
			}
			var ref mcpcontract.DurableArtifactReference
			if err := json.Unmarshal(payload, &ref); err != nil {
				t.Fatal(err)
			}
			if ref.URI != tt.uri || ref.Kind != tt.kind || ref.ID == "" {
				t.Fatalf("reference = %+v, want kind=%q uri=%q", ref, tt.kind, tt.uri)
			}
			if len(result.Content) != 2 {
				t.Fatalf("content = %+v", result.Content)
			}
			link, ok := result.Content[1].(*mcp.ResourceLink)
			if !ok || link.URI != tt.uri {
				t.Fatalf("resource link = %#v", result.Content[1])
			}
			resource, err := client.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: tt.uri})
			if err != nil {
				t.Fatalf("read %s: %v", tt.uri, err)
			}
			if len(resource.Contents) != 1 || resource.Contents[0].Text == "" {
				t.Fatalf("resource %s = %+v", tt.uri, resource)
			}
		})
	}
}

func TestStartInvestigationReturnsCompactResourceReference(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name: mcpcontract.ToolStartInvestigation,
		Arguments: map[string]any{
			"owner": "acme", "repo": "rocket", "commit_sha": "abc123",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var ref mcpcontract.DurableArtifactReference
	if err := json.Unmarshal(payload, &ref); err != nil {
		t.Fatal(err)
	}
	if ref.Kind != "investigation" || ref.ID != "inv-1" || ref.URI != "gitcontribute://investigation/inv-1" {
		t.Fatalf("compact reference = %+v", ref)
	}
	for _, forbidden := range []string{`"owner"`, `"repo"`, `"status"`, `"hypotheses"`} {
		if strings.Contains(string(payload), forbidden) {
			t.Errorf("compact reference leaked investigation field %s: %s", forbidden, payload)
		}
	}
}

func TestPromoteOpportunityReturnsCompactResourceReference(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name: mcpcontract.ToolPromoteOpportunity,
		Arguments: map[string]any{
			"hypothesis_id": "hyp-1", "problem_statement": "leak", "scope": "small",
			"impact": "high", "expected_effort": "1h", "confidence": 0.8,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var ref mcpcontract.DurableArtifactReference
	if err := json.Unmarshal(payload, &ref); err != nil {
		t.Fatal(err)
	}
	if ref.Kind != "opportunity" || ref.ID != "opp-1" || ref.URI != "gitcontribute://opportunity/opp-1" {
		t.Fatalf("compact reference = %+v", ref)
	}
	for _, forbidden := range []string{`"problem_statement"`, `"scope"`, `"impact"`, `"confidence"`, `"source_refs"`} {
		if strings.Contains(string(payload), forbidden) {
			t.Errorf("compact reference leaked opportunity field %s: %s", forbidden, payload)
		}
	}
}

func TestCanonicalResourcesReplaceScalarArtifactGetters(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	tools := map[string]bool{}
	for tool, err := range client.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		tools[tool.Name] = true
	}
	for _, retired := range []string{
		"corpus.get_repository_dossier",
		"corpus.get_investigation",
		"corpus.list_opportunities",
		"corpus.get_opportunity",
		"corpus.get_evidence",
		"corpus.get_readiness",
	} {
		if tools[retired] {
			t.Errorf("retired scalar artifact getter remains advertised: %s", retired)
		}
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
		mcpcontract.ToolGetRepositories, mcpcontract.ToolGetThreads,
		mcpcontract.ToolFindClusters, mcpcontract.ToolFindNeighbors, mcpcontract.ToolGetCoverage,
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
	tools, closeSessions := listedTools(t)
	defer closeSessions()
	if tools[mcpcontract.ToolRankThreads] != nil {
		t.Fatal("removed ranking workflow was advertised")
	}
}

func TestRankOpportunitiesRejectsOutOfRangeOutputAtProtocolBoundary(t *testing.T) {
	tools, closeSessions := listedTools(t)
	defer closeSessions()
	if tools[mcpcontract.ToolRankThreads] != nil {
		t.Fatal("removed ranking workflow was advertised")
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
		{"sync portfolio", mcpcontract.ToolSyncPortfolio, "sync_portfolio", "sync-pull-request-portfolio-", map[string]any{"selection": "explicit", "pull_requests": []map[string]any{{"owner": "acme", "repo": "rocket", "kind": "pull_request", "number": 7}}, "state": "open"}},
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
