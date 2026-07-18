package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var selectionSynonyms = map[string]string{
	"execute": "run",
	"fetch":   "hydrate",
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
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
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

	if len(tools) != len(canonicalToolNames) {
		t.Fatalf("listed %d tools, want canonical catalog of %d", len(tools), len(canonicalToolNames))
	}
	titles := make(map[string]string)
	for _, name := range canonicalToolNames {
		tool := tools[name]
		if tool == nil {
			t.Errorf("canonical tool %q is not registered", name)
			continue
		}
		if !strings.HasPrefix(name, "gitcontribute.") {
			t.Errorf("tool %q is not namespaced", name)
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
	for _, legacy := range []string{"search", "get_dossier", "get_thread", "prepare_contribution", "cancel_job"} {
		if tools[legacy] != nil {
			t.Errorf("legacy unnamespaced tool %q is still advertised", legacy)
		}
	}
}

func TestToolSchemasExposeMachineReadableContracts(t *testing.T) {
	tools, closeSessions := listedTools(t)
	defer closeSessions()

	assertSchemaValue(t, tools[ToolSearchThreads].InputSchema, []string{"properties", "kind", "enum"}, []any{"issue", "pull_request"})
	assertSchemaValue(t, tools[ToolSearchThreads].InputSchema, []string{"properties", "limit", "default"}, float64(20))
	assertSchemaValue(t, tools[ToolSearchThreads].InputSchema, []string{"properties", "limit", "maximum"}, float64(100))
	assertSchemaValue(t, tools[ToolHydrateThread].InputSchema, []string{"properties", "max_pages", "default"}, float64(50))
	assertSchemaValue(t, tools[ToolStartCrawl].InputSchema, []string{"properties", "budget", "maximum"}, float64(5000))
	assertSchemaValue(t, tools[ToolRunValidation].InputSchema, []string{"properties", "execute", "const"}, true)
	assertSchemaValue(t, tools[ToolPromoteOpportunity].InputSchema, []string{"properties", "confidence", "maximum"}, float64(1))

	for name, tool := range tools {
		output, ok := tool.OutputSchema.(map[string]any)
		if !ok {
			t.Errorf("tool %q output schema type = %T", name, tool.OutputSchema)
			continue
		}
		if strings.TrimSpace(stringValue(output["description"])) == "" {
			t.Errorf("tool %q output schema has no root description", name)
		}
		assertPropertiesDescribed(t, name, output)
	}
}

func TestAgentToolSelectionProxy(t *testing.T) {
	tools, closeSessions := listedTools(t)
	defer closeSessions()

	cases := []struct {
		prompt string
		want   string
	}{
		{"Search locally stored issue titles for a retry deadlock", ToolSearchThreads},
		{"Read the complete stored body of pull request 42", ToolGetThread},
		{"Refresh repository issues and pull requests from GitHub", ToolSyncRepository},
		{"Fetch comments and reviews for one stored pull request from GitHub", ToolHydrateThread},
		{"Create a local investigation without cloning a worktree", ToolStartInvestigation},
		{"Clone the remote and create a managed Git worktree", ToolCreateWorkspace},
		{"Render and persist a pull request draft from supplied changes", ToolPrepareContribution},
		{"Execute the stored validation command against the candidate workspace", ToolRunValidation},
		{"Stop a running durable job", ToolCancelJob},
		{"Review readiness blockers and warnings for an opportunity", ToolGetReadiness},
		{"Rebuild and persist the repository dossier from the local corpus", ToolBuildRepositoryDossier},
		{"Read the existing persisted repository dossier without rebuilding it", ToolGetRepositoryDossier},
		{"Find open pull requests that might conflict with this opportunity", ToolCheckCollisions},
		{"Find issues that may duplicate this hypothesis", ToolCheckDuplicates},
	}

	correct := 0
	for _, tc := range cases {
		got := selectToolByWords(tc.prompt, tools)
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

func TestInvalidToolCallEvaluation(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	cases := []struct {
		name string
		args map[string]any
	}{
		{ToolSearchThreads, map[string]any{"query": "race", "kind": "discussion"}},
		{ToolSearchThreads, map[string]any{"query": "race", "limit": 101}},
		{ToolSearchCode, map[string]any{"query": "race", "owner": "acme"}},
		{ToolGetThread, map[string]any{"owner": "acme", "repo": "rocket", "kind": "issue", "number": 0}},
		{ToolGetEvidence, map[string]any{}},
		{ToolGetEvidence, map[string]any{"investigation_id": "inv-1", "opportunity_id": "opp-1"}},
		{ToolHydrateThread, map[string]any{"owner": "acme", "repo": "rocket", "number": 1, "facets": []string{"unknown"}}},
		{ToolSyncRepository, map[string]any{"owner": "acme", "repo": "rocket", "numbers": []int{1}, "state": "open"}},
		{ToolRunValidation, map[string]any{"id": "val-1", "kind": "candidate", "execute": false}},
		{ToolPromoteOpportunity, map[string]any{"hypothesis_id": "hyp-1", "problem_statement": "p", "scope": "s", "impact": "i", "expected_effort": "e", "confidence": 1.1}},
		{ToolPrepareContribution, map[string]any{"opportunity_id": "opp-1", "kind": "pull_request", "workspace_id": "ws-1", "approach": "focused"}},
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

func TestSideEffectAuthorizationEvaluation(t *testing.T) {
	tools, closeSessions := listedTools(t)
	defer closeSessions()

	cancel := tools[ToolCancelJob].Annotations
	if cancel == nil || cancel.ReadOnlyHint || cancel.DestructiveHint == nil || !*cancel.DestructiveHint || !cancel.IdempotentHint {
		t.Fatalf("cancel annotations = %+v", cancel)
	}
	run := tools[ToolRunValidation].Annotations
	if run == nil || run.ReadOnlyHint || run.DestructiveHint == nil || !*run.DestructiveHint {
		t.Fatalf("validation annotations = %+v", run)
	}
	prepare := tools[ToolPrepareContribution]
	if prepare.Annotations == nil || prepare.Annotations.ReadOnlyHint || prepare.Annotations.OpenWorldHint == nil || *prepare.Annotations.OpenWorldHint {
		t.Fatalf("prepare contribution annotations = %+v", prepare.Annotations)
	}
	for _, phrase := range []string{"never inspects a workspace", "runs Git", "never", "mutates GitHub"} {
		if !strings.Contains(prepare.Description, phrase) {
			t.Errorf("prepare contribution description does not disclose boundary phrase %q", phrase)
		}
	}
}

func assertSchemaValue(t *testing.T, raw any, path []string, want any) {
	t.Helper()
	current := raw
	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("schema path %v: %T is not an object", path, current)
		}
		current = object[key]
	}
	if fmt.Sprint(current) != fmt.Sprint(want) {
		t.Errorf("schema path %v = %#v, want %#v", path, current, want)
	}
}

func assertPropertiesDescribed(t *testing.T, toolName string, schema map[string]any) {
	t.Helper()
	if properties, ok := schema["properties"].(map[string]any); ok {
		for name, raw := range properties {
			property, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if strings.TrimSpace(stringValue(property["description"])) == "" {
				t.Errorf("tool %q output property %q has no description", toolName, name)
			}
			assertPropertiesDescribed(t, toolName, property)
		}
	}
	if definitions, ok := schema["$defs"].(map[string]any); ok {
		for _, raw := range definitions {
			if definition, ok := raw.(map[string]any); ok {
				assertPropertiesDescribed(t, toolName, definition)
			}
		}
	}
	if items, ok := schema["items"].(map[string]any); ok {
		assertPropertiesDescribed(t, toolName, items)
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
