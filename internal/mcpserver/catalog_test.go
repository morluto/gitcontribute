package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/facets"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

var selectionSynonyms = map[string]string{
	"execute": "run",
	"read":    "get",
	"rebuild": "build",
	"refresh": "sync",
	"review":  "get",
	"stop":    "cancel",
}

var selectionStopWords = map[string]bool{
	"a": true, "an": true, "and": true, "for": true, "from": true, "in": true,
	"it": true, "of": true, "one": true, "or": true, "the": true, "this": true,
	"to": true, "tool": true, "use": true, "with": true, "without": true,
	"gitcontribute": true, "local": true, "stored": true,
}

func listedTools(t *testing.T) (map[string]*mcp.Tool, func()) {
	t.Helper()
	return listedToolsFromReader(t, &fakeReader{searchStarted: make(chan struct{})})
}

func listedToolsFromReader(t *testing.T, reader mcpcontract.Reader) (map[string]*mcp.Tool, func()) {
	t.Helper()
	client, closeSessions := connectServer(t, reader, false)
	tools := make(map[string]*mcp.Tool)
	for tool, err := range client.Tools(context.Background(), nil) {
		if err != nil {
			closeSessions()
			t.Fatalf("list tools: %v", err)
		}
		tools[tool.Name] = tool
	}
	return tools, closeSessions
}

func TestCanonicalToolCatalogIsNamespacedAndUnambiguous(t *testing.T) {
	tools, closeSessions := listedTools(t)
	defer closeSessions()

	canonicalToolNames := make([]string, 0, len(tools))
	for name := range tools {
		canonicalToolNames = append(canonicalToolNames, name)
	}
	sort.Strings(canonicalToolNames)
	titles := make(map[string]string)
	for _, name := range canonicalToolNames {
		tool := tools[name]
		if tool == nil {
			t.Errorf("canonical tool %q is not registered", name)
			continue
		}
		capability, action, namespaced := strings.Cut(name, ".")
		if !namespaced || capability == "" || action == "" {
			t.Errorf("tool %q is not capability-namespaced", name)
		}
		if strings.HasPrefix(name, "gitcontribute.") {
			t.Errorf("tool %q redundantly includes the server name", name)
		}
		if strings.TrimSpace(tool.Title) == "" {
			t.Errorf("tool %q has no display title", name)
		} else if previous := titles[tool.Title]; previous != "" {
			t.Errorf("tools %q and %q share display title %q", previous, name, tool.Title)
		} else {
			titles[tool.Title] = name
		}
		if len(strings.Fields(tool.Description)) < 12 {
			t.Errorf("tool %q description lacks selection context: %q", name, tool.Description)
		}
	}
	for _, legacy := range []string{
		"search", "get_dossier", "get_thread", "prepare_contribution", "cancel_job",
		"corpus.get_repository", "corpus.get_thread", "github.sync_repository",
		"github.hydrate_thread", "github.hydrate_repository", "github.start_crawl",
		"workflow.check_collisions",
	} {
		if tools[legacy] != nil {
			t.Errorf("legacy unnamespaced tool %q is still advertised", legacy)
		}
	}
}

func TestUnifiedCatalogReportsSerializedContextMeasurements(t *testing.T) {
	tools, closeSessions := listedTools(t)
	defer closeSessions()
	payload, err := json.Marshal(tools)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("MCP unified catalog tools=%d serialized_bytes=%d", len(tools), len(payload))
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		payload, err := json.Marshal(tools[name])
		if err != nil {
			t.Fatalf("marshal tool %s: %v", name, err)
		}
		t.Logf("MCP catalog tool=%s serialized_bytes=%d", name, len(payload))
	}
}

func TestStructuredCancellationIsNotRetryable(t *testing.T) {
	handler := structuredToolErrors(func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, struct{}, error) {
		return nil, struct{}{}, context.Canceled
	})
	_, _, err := handler(context.Background(), nil, struct{}{})
	toolErr := &mcpcontract.ToolError{}
	ok := errors.As(err, &toolErr)
	if !ok || toolErr.Code != "cancelled" || toolErr.Retryable {
		t.Fatalf("cancellation error = %#v", err)
	}
}

func TestReadOnlyModeFiltersEverySideEffectingTool(t *testing.T) {
	server, err := NewReadOnly(&fakeReader{searchStarted: make(chan struct{})}, "test")
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	t1, t2 := mcp.NewInMemoryTransports()
	serverSession, err := server.MCP().Connect(context.Background(), t1, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = serverSession.Close() }()
	clientSession, err := client.Connect(context.Background(), t2, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = clientSession.Close() }()
	for tool, err := range clientSession.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Fatalf("non-read-only tool advertised: %s (%+v)", tool.Name, tool.Annotations)
		}
	}
}

func TestUnsupportedOptionalCapabilitiesAreNotAdvertised(t *testing.T) {
	base := &fakeReader{searchStarted: make(chan struct{})}
	server, err := New(struct{ mcpcontract.Reader }{Reader: base}, "test")
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	t1, t2 := mcp.NewInMemoryTransports()
	serverSession, err := server.MCP().Connect(context.Background(), t1, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = serverSession.Close() }()
	clientSession, err := client.Connect(context.Background(), t2, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = clientSession.Close() }()
	names := map[string]bool{}
	for tool, err := range clientSession.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		names[tool.Name] = true
	}
	if !names[mcpcontract.ToolSearchThreads] || !names[mcpcontract.ToolGetJob] {
		t.Fatalf("core reader tools missing: %v", names)
	}
	for _, name := range []string{mcpcontract.ToolFindNeighbors, mcpcontract.ToolGetRepositories, mcpcontract.ToolSearchGitHubRepositories, mcpcontract.ToolLinkPullRequest, mcpcontract.ToolStartInvestigation} {
		if names[name] {
			t.Errorf("unsupported optional tool %q was advertised", name)
		}
	}
}

func TestOptionalCapabilitiesAreAdvertisedIndependently(t *testing.T) {
	base := &fakeReader{searchStarted: make(chan struct{})}
	research := &fakeOptionalCapabilities{base: base}
	reader := struct {
		mcpcontract.Reader
		ResearchReader
	}{Reader: base, ResearchReader: research}
	tools, closeSessions := listedToolsFromReader(t, reader)
	defer closeSessions()

	if tools[mcpcontract.ToolQueryDeepWiki] != nil {
		t.Fatal("removed derived-research workflow was advertised")
	}
	for _, name := range []string{mcpcontract.ToolSearchGitHubRepositories, mcpcontract.ToolIndexRepositories, mcpcontract.ToolCheckMergeConflicts, mcpcontract.ToolListPullRequestPortfolio} {
		if tools[name] != nil {
			t.Errorf("unrelated unsupported tool %q was advertised", name)
		}
	}
}

func TestToolSchemasExposeMachineReadableContracts(t *testing.T) {
	tools, closeSessions := listedTools(t)
	defer closeSessions()

	assertSchemaValue(t, tools[mcpcontract.ToolSearchThreads].InputSchema, []string{"properties", "kind", "enum"}, []any{"issue", "pull_request"})
	assertSchemaValue(t, tools[mcpcontract.ToolSearchThreads].InputSchema, []string{"properties", "limit", "default"}, float64(20))
	assertSchemaValue(t, tools[mcpcontract.ToolSearchThreads].InputSchema, []string{"properties", "limit", "maximum"}, float64(100))
	assertSchemaValue(t, tools[mcpcontract.ToolGetThreadFacets].InputSchema, []string{"properties", "threads", "maxItems"}, float64(100))
	assertSchemaValue(t, tools[mcpcontract.ToolGetThreadFacets].InputSchema, []string{"properties", "facets", "maxItems"}, float64(10))
	assertSchemaValue(t, tools[mcpcontract.ToolGetThreadFacets].InputSchema, []string{"properties", "facets", "items", "enum"}, facets.AllNames())
	if !strings.Contains(tools[mcpcontract.ToolGetCoverage].Description, "typed recovery action") {
		t.Fatalf("coverage description does not expose recovery routing: %q", tools[mcpcontract.ToolGetCoverage].Description)
	}
	if !strings.Contains(tools[mcpcontract.ToolSyncThreads].Description, "poll jobs.get and reread") {
		t.Fatalf("thread sync description does not expose the follow-up route: %q", tools[mcpcontract.ToolSyncThreads].Description)
	}
	assertSchemaValue(t, tools[mcpcontract.ToolHydrateThreads].InputSchema, []string{"properties", "max_pages", "default"}, float64(3))
	assertSchemaValue(t, tools[mcpcontract.ToolCreateWorkspace].InputSchema, []string{"required"}, []any{"investigation_id"})
	assertSchemaValue(t, tools[mcpcontract.ToolAdoptWorkspace].InputSchema, []string{"required"}, []any{"investigation_id", "path", "base_ref"})
	assertSchemaValue(t, tools[mcpcontract.ToolFindPrecedents].OutputSchema, []string{"properties", "items", "items", "properties", "value", "properties", "matches", "items", "properties", "score", "maximum"}, float64(1))
	assertSchemaValue(t, tools[mcpcontract.ToolGetJob].OutputSchema, []string{"properties", "items", "items", "properties", "item_status", "enum"}, []any{"complete", "retryable", "unavailable", "failed"})
	assertSchemaValue(t, tools[mcpcontract.ToolGetJob].OutputSchema, []string{"properties", "items", "items", "properties", "value", "properties", "execution_state", "enum"}, []any{"queued", "running", "terminal"})
	assertSchemaValue(t, tools[mcpcontract.ToolGetJob].OutputSchema, []string{"properties", "items", "items", "properties", "value", "properties", "outcome", "enum"}, []any{"succeeded", "partial", "failed", "cancelled"})
	assertSchemaValue(t, tools[mcpcontract.ToolGetJob].OutputSchema, []string{"properties", "items", "items", "properties", "value", "properties", "progress_percent", "maximum"}, float64(100))
	assertSchemaValue(t, tools[mcpcontract.ToolRunValidation].InputSchema, []string{"properties", "run_count", "default"}, float64(1))
	assertSchemaValue(t, tools[mcpcontract.ToolRunValidation].InputSchema, []string{"properties", "run_count", "maximum"}, float64(100))
	assertSchemaValue(t, tools[mcpcontract.ToolRunValidation].InputSchema, []string{"properties", "execute", "const"}, true)
	assertSchemaValue(t, tools[mcpcontract.ToolDefineValidation].InputSchema, []string{"properties", "protocol", "enum"}, []any{"mcp_stdio"})
	assertSchemaValue(t, tools[mcpcontract.ToolPromoteOpportunity].InputSchema, []string{"properties", "confidence", "maximum"}, float64(1))
	validationSchema, err := json.Marshal(tools[mcpcontract.ToolDefineValidation].InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"observation"`, `"intent"`, `"observations"`, `"run"`, `"occurrence"`, `artifact`, `"path"`} {
		if !strings.Contains(string(validationSchema), field) {
			t.Errorf("validation schema missing %s: %s", field, validationSchema)
		}
	}
	observationPath := []string{"properties", "observation", "properties", "observations"}
	assertSchemaValue(t, tools[mcpcontract.ToolDefineValidation].InputSchema, append(observationPath, "minItems"), float64(2))
	assertSchemaValue(t, tools[mcpcontract.ToolDefineValidation].InputSchema, append(observationPath, "maxItems"), float64(16))
	assertSchemaValue(t, tools[mcpcontract.ToolDefineValidation].InputSchema, append(observationPath, "items", "properties", "run", "enum"), []any{"base", "candidate"})
	assertSchemaValue(t, tools[mcpcontract.ToolDefineValidation].InputSchema, append(observationPath, "items", "properties", "source", "enum"), []any{"stdout", "stderr", "artifact"})
	assertSchemaValue(t, tools[mcpcontract.ToolDefineValidation].InputSchema, append(observationPath, "items", "properties", "matcher", "enum"), []any{"exact", "regexp"})
	assertSchemaValue(t, tools[mcpcontract.ToolDefineValidation].InputSchema, append(observationPath, "items", "properties", "occurrence", "enum"), []any{"present", "absent"})
	assertSchemaValue(t, tools[mcpcontract.ToolDefineValidation].InputSchema, append(observationPath, "items", "properties", "occurrence", "default"), "present")
	assertSchemaValue(t, tools[mcpcontract.ToolDefineValidation].InputSchema, append(observationPath, "allOf", "0", "contains", "properties", "run", "const"), "base")
	assertSchemaValue(t, tools[mcpcontract.ToolDefineValidation].InputSchema, append(observationPath, "allOf", "1", "contains", "properties", "run", "const"), "candidate")

	for name, tool := range tools {
		output, ok := tool.OutputSchema.(map[string]any)
		if !ok {
			t.Errorf("tool %q output schema type = %T", name, tool.OutputSchema)
			continue
		}
		if strings.TrimSpace(stringValue(output["description"])) == "" {
			t.Errorf("tool %q output schema has no root description", name)
		}
	}
}

func TestActorSelectorSchemaIsDiscriminated(t *testing.T) {
	definition := inputSchema[mcpcontract.SyncUsersInput](func(builder *schemaBuilder) {
		configureActorSelectorModes(builder)
	})
	if definition.err != nil {
		t.Fatal(definition.err)
	}
	actor := definition.schema.Defs["ActorSelector"]
	if actor == nil {
		for name, candidate := range definition.schema.Defs {
			if strings.HasSuffix(name, "ActorSelector") {
				actor = candidate
				break
			}
		}
	}
	if actor == nil {
		actor = definition.schema.Properties["users"].Items
	}
	if actor == nil || len(actor.OneOf) != 2 {
		t.Fatalf("actor selector schema = %+v defs=%d users=%+v", actor, len(definition.schema.Defs), definition.schema.Properties["users"])
	}
	if actor.OneOf[0].ID != "urn:gitcontribute:actor-selector:login" || actor.OneOf[1].ID != "urn:gitcontribute:actor-selector:node-id" {
		t.Fatalf("actor selector modes = %+v", actor.OneOf)
	}
}

func TestSchemaCustomizationErrorsAreReturned(t *testing.T) {
	tests := []struct {
		name      string
		customize func(*schemaBuilder)
		want      string
	}{
		{name: "missing property", customize: func(schema *schemaBuilder) { setEnum(schema, "missing", "x") }, want: `property "missing" not found`},
		{name: "non-array enum", customize: func(schema *schemaBuilder) { setArrayEnum(schema, "owner", "x") }, want: `array property "owner" has no items schema`},
		{name: "invalid default", customize: func(schema *schemaBuilder) { setDefault(schema, "owner", func() {}) }, want: `marshal MCP schema default for "owner"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			definition := inputSchema[mcpcontract.RepoInput](tc.customize)
			if definition.err == nil || !strings.Contains(definition.err.Error(), tc.want) {
				t.Fatalf("schema error = %v, want %q", definition.err, tc.want)
			}
		})
	}
}

func TestCatalogRegistrationReportsToolSchemaError(t *testing.T) {
	server := &Server{}
	addCatalogTool(server, catalogTool[mcpcontract.RepoInput, mcpcontract.RepositoryOutput]{
		name: "broken.tool",
		input: inputSchema[mcpcontract.RepoInput](func(schema *schemaBuilder) {
			setEnum(schema, "missing", "x")
		}),
		output: outputSchema[mcpcontract.RepositoryOutput]("Repository."),
	})
	if server.registrationErr == nil || !strings.Contains(server.registrationErr.Error(), `register MCP tool "broken.tool" input schema`) {
		t.Fatalf("registration error = %v", server.registrationErr)
	}
}

func TestAgentToolSelectionProxy(t *testing.T) {
	tools, closeSessions := listedTools(t)
	defer closeSessions()
	// Keep this historical proxy corpus stable; repository-wide feedback has
	// its own focused routing assertions below.
	proxyTools := make(map[string]*mcp.Tool, len(tools))
	for name, tool := range tools {
		if name != mcpcontract.ToolIndexPullRequestFeedback && name != mcpcontract.ToolSearchPullRequestFeedback {
			proxyTools[name] = tool
		}
	}

	cases := []struct {
		prompt string
		want   string
	}{
		{"Search locally stored issue titles for a retry deadlock", mcpcontract.ToolSearchThreads},
		{"Search live GitHub for highly starred inference repositories", mcpcontract.ToolSearchGitHubRepositories},
		{"Read metadata for twelve repositories already stored in the corpus", mcpcontract.ToolGetRepositories},
		{"Fetch current GitHub stars, metadata, and contribution guidance for twelve repositories", mcpcontract.ToolSyncRepositoryContext},
		{"Read the complete stored body of pull request 42", mcpcontract.ToolGetThreads},
		{"Refresh issue and pull request thread headers for selected repositories from GitHub", mcpcontract.ToolSyncThreads},
		{"Fetch comments and reviews for one stored pull request from GitHub", mcpcontract.ToolSyncPullRequestFeedback},
		{"Find similar completed and rejected historical work for this issue", mcpcontract.ToolFindPrecedents},
		{"List my stored pull requests that need contributor attention", mcpcontract.ToolListPullRequestPortfolio},
		{"Acquire and index code for several repositories", mcpcontract.ToolIndexRepositories},
		{"Check actual Git merge conflicts between fetched revisions", mcpcontract.ToolCheckMergeConflicts},
		{"Create a local investigation without cloning a worktree", mcpcontract.ToolStartInvestigation},
		{"Clone the remote and create a managed Git worktree", mcpcontract.ToolCreateWorkspace},
		{"Render and persist a pull request draft from a verified managed workspace diff", mcpcontract.ToolPrepareContribution},
		{"Execute the stored validation command against the candidate workspace", mcpcontract.ToolRunValidation},
		{"Run a repeat stress validation group with concurrency and telemetry", mcpcontract.ToolRunValidation},
		{"Stop a running durable job", mcpcontract.ToolCancelJob},
		{"Poll several durable jobs together with structured progress", mcpcontract.ToolGetJob},
		{"Read stored facet coverage for several exact threads", mcpcontract.ToolGetThreadFacets},
		{"Read repository and thread coverage across several targets", mcpcontract.ToolGetCoverage},
		{"Compare contribution candidates with my authored pull requests for overlap", mcpcontract.ToolFindPortfolioOverlaps},
		{"Link an authored pull request to a local opportunity", mcpcontract.ToolLinkPullRequest},
	}

	correct := 0
	for _, tc := range cases {
		got := selectToolByWords(tc.prompt, proxyTools)
		if got == tc.want {
			correct++
			continue
		}
		t.Errorf("prompt %q selected %q, want %q", tc.prompt, got, tc.want)
	}
	if correct != len(cases) {
		t.Fatalf("tool-selection proxy accuracy = %d/%d", correct, len(cases))
	}
}

func TestFeedbackToolSelectionProxy(t *testing.T) {
	tools, closeSessions := listedTools(t)
	defer closeSessions()
	feedbackTools := map[string]*mcp.Tool{
		mcpcontract.ToolFindPrecedents:            tools[mcpcontract.ToolFindPrecedents],
		mcpcontract.ToolIndexPullRequestFeedback:  tools[mcpcontract.ToolIndexPullRequestFeedback],
		mcpcontract.ToolSearchPullRequestFeedback: tools[mcpcontract.ToolSearchPullRequestFeedback],
		mcpcontract.ToolSyncPullRequestFeedback:   tools[mcpcontract.ToolSyncPullRequestFeedback],
	}
	if got := selectToolByWords("Find every pull-request comment written by chatgpt-codex-connector[bot] across one repository", feedbackTools); got != mcpcontract.ToolIndexPullRequestFeedback {
		t.Fatalf("repository-wide feedback discovery selected %q", got)
	}
	if got := selectToolByWords("Search indexed pull-request feedback by exact commenter login", feedbackTools); got != mcpcontract.ToolSearchPullRequestFeedback {
		t.Fatalf("exact feedback author search selected %q", got)
	}
}

func TestInvalidToolCallEvaluation(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	cases := []struct {
		name string
		args map[string]any
	}{
		{mcpcontract.ToolSearchThreads, map[string]any{"query": "race", "kind": "discussion"}},
		{mcpcontract.ToolSearchThreads, map[string]any{"query": "race", "limit": 101}},
		{mcpcontract.ToolSearchGitHubRepositories, map[string]any{"limit": 20}},
		{mcpcontract.ToolSearchCode, map[string]any{"query": "race", "owner": "acme"}},
		{mcpcontract.ToolGetThreads, map[string]any{"threads": []any{map[string]any{"owner": "acme", "repo": "rocket", "kind": "issue", "number": 0}}}},
		{mcpcontract.ToolGetCoverage, map[string]any{"targets": []any{}}},
		{mcpcontract.ToolCancelJob, map[string]any{"ids": []any{}}},
		{mcpcontract.ToolFindPortfolioOverlaps, map[string]any{"candidates": []any{map[string]any{"kind": "thread", "ref": "1"}}, "pull_requests": []any{map[string]any{"owner": "acme", "repo": "rocket", "number": 1}}}},
		{mcpcontract.ToolLinkPullRequest, map[string]any{"pull_request": map[string]any{"owner": "acme", "repo": "rocket", "number": 1}}},
		{mcpcontract.ToolHydrateThreads, map[string]any{"threads": []any{map[string]any{"owner": "acme", "repo": "rocket", "number": 1}}, "facets": []string{"unknown"}}},
		{mcpcontract.ToolSyncThreads, map[string]any{"selection": "threads", "threads": []any{map[string]any{"owner": "acme", "repo": "rocket", "number": 1}}, "state": "open"}},
		{mcpcontract.ToolRunValidation, map[string]any{"id": "val-1", "target": "candidate", "execute": false}},
		{mcpcontract.ToolRunValidation, map[string]any{"id": "val-1", "target": "both", "run_count": 101, "execute": true}},
		{mcpcontract.ToolDefineValidation, map[string]any{"investigation_id": "inv-1", "kind": "test", "command": "server", "workspace_id": "ws-1", "readiness_timeout": "30s"}},
		{mcpcontract.ToolDefineValidation, map[string]any{"investigation_id": "inv-1", "kind": "test", "command": "go test ./...", "workspace_id": "ws-1", "max_output_bytes": -1}},
		{mcpcontract.ToolPromoteOpportunity, map[string]any{"hypothesis_id": "hyp-1", "problem_statement": "p", "scope": "s", "impact": "i", "expected_effort": "e", "confidence": 1.1}},
		{mcpcontract.ToolPrepareContribution, map[string]any{"opportunity_id": "opp-1", "kind": "pull_request", "approach": "focused"}},
	}

	accepted := 0
	for _, tc := range cases {
		result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
		if err == nil && result != nil && !result.IsError {
			accepted++
			t.Errorf("invalid call to %q was accepted: %#v", tc.name, tc.args)
		}
	}
	if accepted != 0 {
		t.Fatalf("invalid-call acceptance rate = %d/%d, want 0/%d", accepted, len(cases), len(cases))
	}
}

func TestCoverageTargetSchemaAcceptsRepositoryAndExactThreadOnly(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	valid := []map[string]any{
		{"targets": []any{map[string]any{"type": "repository", "repository": map[string]any{"owner": "acme", "repo": "rocket"}}}},
		{"targets": []any{map[string]any{"type": "exact_thread", "repository": map[string]any{"owner": "acme", "repo": "rocket"}, "thread": map[string]any{"kind": "pull_request", "number": 7}}}},
	}
	for _, args := range valid {
		result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: mcpcontract.ToolGetCoverage, Arguments: args})
		if err != nil || result == nil || result.IsError {
			message := ""
			if result != nil && len(result.Content) > 0 {
				if content, ok := result.Content[0].(*mcp.TextContent); ok {
					message = content.Text
				}
			}
			t.Fatalf("valid coverage target was rejected: args=%#v err=%v message=%q", args, err, message)
		}
	}

	invalid := []map[string]any{
		{"targets": []any{map[string]any{"type": "unknown", "repository": map[string]any{"owner": "acme", "repo": "rocket"}}}},
		{"targets": []any{map[string]any{"type": "exact_thread", "repository": map[string]any{"owner": "acme", "repo": "rocket"}, "thread": map[string]any{"kind": "discussion", "number": 7}}}},
		{"targets": []any{map[string]any{"type": "exact_thread", "repository": map[string]any{"owner": "acme", "repo": "rocket"}, "thread": map[string]any{"kind": "issue", "number": 0}}}},
		{"targets": []any{map[string]any{"type": "repository", "repository": map[string]any{"owner": "acme", "repo": "rocket"}, "extra": true}}},
	}
	for _, args := range invalid {
		result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: mcpcontract.ToolGetCoverage, Arguments: args})
		if err == nil && result != nil && !result.IsError {
			t.Errorf("invalid coverage target was accepted: %#v", args)
		}
	}
}

func TestFixPatternWorkflowSchemaRejectsInvalidNestedInputBeforeHandler(t *testing.T) {
	tools, closeSessions := listedTools(t)
	defer closeSessions()
	if tools[mcpcontract.ToolMineRepositoryFixPatterns] != nil {
		t.Fatal("removed fix-pattern workflow was advertised")
	}
}

func TestSyncThreadsDefaultsRemainSelectionSpecific(t *testing.T) {
	base := &fakeReader{searchStarted: make(chan struct{})}
	optional := &fakeOptionalCapabilities{base: base}
	reader := completeTestReader{
		Reader: base, NeighborReader: optional, ScalableReader: optional,
		PortfolioReader: optional, GitHubOperator: optional, PullRequestFeedbackOperator: optional, CIFailureOperator: optional, CodeIndexer: optional,
		MergeConflictReader: optional, ResearchReader: optional,
		PortfolioOperator: optional, Operator: base,
	}
	client, closeSessions := connect(t, reader)
	defer closeSessions()

	thread := map[string]any{"owner": "acme", "repo": "rocket", "kind": "pull_request", "number": 7}
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name: mcpcontract.ToolSyncThreads, Arguments: map[string]any{"selection": "threads", "threads": []any{thread}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("exact thread sync rejected: %+v", result.Content)
	}
	if optional.syncThreadsInput.LimitPerRepository != 0 {
		t.Fatalf("thread mode received repository default: %+v", optional.syncThreadsInput)
	}

	result, err = client.CallTool(context.Background(), &mcp.CallToolParams{
		Name: mcpcontract.ToolSyncThreads, Arguments: map[string]any{
			"selection": "threads", "threads": []any{thread}, "limit_per_repository": 10,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("thread mode accepted explicit repository-only limit")
	}

	result, err = client.CallTool(context.Background(), &mcp.CallToolParams{
		Name: mcpcontract.ToolSyncThreads, Arguments: map[string]any{
			"selection": "repositories", "repositories": []any{map[string]any{"owner": "acme", "repo": "rocket"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || optional.syncThreadsInput.LimitPerRepository != 100 {
		t.Fatalf("repository mode default = %+v, result = %+v", optional.syncThreadsInput, result.Content)
	}
}

func TestSideEffectAuthorizationEvaluation(t *testing.T) {
	tools, closeSessions := listedTools(t)
	defer closeSessions()

	cancel := tools[mcpcontract.ToolCancelJob].Annotations
	if cancel == nil || cancel.ReadOnlyHint || cancel.DestructiveHint == nil || !*cancel.DestructiveHint || !cancel.IdempotentHint {
		t.Fatalf("cancel annotations = %+v", cancel)
	}
	run := tools[mcpcontract.ToolRunValidation].Annotations
	if run == nil || run.ReadOnlyHint || run.DestructiveHint == nil || !*run.DestructiveHint {
		t.Fatalf("validation annotations = %+v", run)
	}
	prepare := tools[mcpcontract.ToolPrepareContribution]
	if prepare.Annotations == nil || prepare.Annotations.ReadOnlyHint || prepare.Annotations.OpenWorldHint == nil || *prepare.Annotations.OpenWorldHint {
		t.Fatalf("prepare contribution annotations = %+v", prepare.Annotations)
	}
	for _, phrase := range []string{"inspects the managed workspace", "non-mutating Git", "Never posts", "mutates GitHub"} {
		if !strings.Contains(prepare.Description, phrase) {
			t.Errorf("prepare contribution description does not disclose boundary phrase %q", phrase)
		}
	}
}

func assertSchemaValue(t *testing.T, raw any, path []string, want any) {
	t.Helper()
	current := raw
	for _, key := range path {
		switch value := current.(type) {
		case map[string]any:
			current = value[key]
		case []any:
			index, err := strconv.Atoi(key)
			if err != nil || index < 0 || index >= len(value) {
				t.Fatalf("schema path %v: invalid array index %q", path, key)
			}
			current = value[index]
		default:
			t.Fatalf("schema path %v: %T is not traversable", path, current)
		}
	}
	if fmt.Sprint(current) != fmt.Sprint(want) {
		t.Errorf("schema path %v = %#v, want %#v", path, current, want)
	}
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func selectToolByWords(prompt string, tools map[string]*mcp.Tool) string {
	promptWords := meaningfulWords(prompt)
	intent := firstIntentWord(prompt)
	bestName := ""
	bestScore := -1
	for name, tool := range tools {
		nameAndTitle := meaningfulWords(strings.ReplaceAll(name, ".", " ") + " " + tool.Title)
		description := meaningfulWords(tool.Description)
		score := 0
		if intent != "" && nameAndTitle[intent] {
			score += 5
		}
		for word := range promptWords {
			if nameAndTitle[word] {
				score += 3
			} else if description[word] {
				score++
			}
		}
		if score > bestScore || score == bestScore && name < bestName {
			bestName, bestScore = name, score
		}
	}
	return bestName
}

func firstIntentWord(text string) string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	if len(fields) == 0 {
		return ""
	}
	return selectionSynonyms[strings.TrimSuffix(fields[0], "s")]
}

func meaningfulWords(text string) map[string]bool {
	words := make(map[string]bool)
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	for _, word := range fields {
		word = normalizeSelectionWord(word)
		if len(word) > 1 && !selectionStopWords[word] {
			words[word] = true
		}
	}
	return words
}

func normalizeSelectionWord(word string) string {
	word = strings.TrimSuffix(word, "s")
	if synonym := selectionSynonyms[word]; synonym != "" {
		return synonym
	}
	return word
}
