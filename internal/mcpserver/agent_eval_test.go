package mcpserver

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// These evaluations deliberately use scripted calls. They measure the MCP
// contract an agent sees, not a model's ability to choose the right tool.
func TestAgentEvalBaselineArtifact(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(filepath.Join("testdata", "agent-eval", "baseline.json"))
	if err != nil {
		t.Fatal(err)
	}
	var baseline struct {
		Version   string `json:"version"`
		Scenarios []struct {
			Name    string `json:"name"`
			Metrics struct {
				Completed             bool `json:"completed"`
				ToolCalls             int  `json:"tool_calls"`
				ToolErrors            int  `json:"tool_errors"`
				InvalidArgumentErrors int  `json:"invalid_argument_errors"`
				ResponseBytes         int  `json:"response_bytes"`
				PollCalls             int  `json:"poll_calls"`
			} `json:"metrics"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(data, &baseline); err != nil {
		t.Fatalf("decode baseline: %v", err)
	}
	if baseline.Version != "agent-tool-eval.v1" {
		t.Fatalf("baseline version = %q", baseline.Version)
	}
	if len(baseline.Scenarios) < 3 {
		t.Fatalf("baseline has %d scenarios, want at least 3", len(baseline.Scenarios))
	}
	seen := map[string]bool{}
	for _, scenario := range baseline.Scenarios {
		if scenario.Name == "" || seen[scenario.Name] {
			t.Fatalf("missing or duplicate scenario name %q", scenario.Name)
		}
		seen[scenario.Name] = true
		if scenario.Metrics.ToolCalls < 1 || scenario.Metrics.ResponseBytes < 1 {
			t.Fatalf("scenario %q has incomplete metrics: %+v", scenario.Name, scenario.Metrics)
		}
	}
}

func TestAgentEvalV2PublicAndOracleStayPaired(t *testing.T) {
	t.Parallel()
	type scenario struct {
		ID              string   `json:"id"`
		Prompt          string   `json:"prompt"`
		Toolsets        []string `json:"toolsets"`
		AcceptableTools []string `json:"acceptable_tools"`
	}
	type fixture struct {
		Version         string     `json:"version"`
		FixtureRevision string     `json:"fixture_revision"`
		Scenarios       []scenario `json:"scenarios"`
	}
	read := func(name string) fixture {
		data, err := os.ReadFile(filepath.Join("testdata", "agent-eval", name))
		if err != nil {
			t.Fatal(err)
		}
		var value fixture
		if err := json.Unmarshal(data, &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	public, oracle := read("public-v2.json"), read("oracle-v2.json")
	if public.Version != "agent-tool-eval.v2" || oracle.Version != "agent-tool-eval-oracle.v2" || public.FixtureRevision != oracle.FixtureRevision {
		t.Fatalf("mismatched eval fixtures: public=%+v oracle=%+v", public, oracle)
	}
	if len(public.Scenarios) != len(oracle.Scenarios) || len(public.Scenarios) < 3 {
		t.Fatalf("scenario counts differ: public=%d oracle=%d", len(public.Scenarios), len(oracle.Scenarios))
	}
	for i := range public.Scenarios {
		if public.Scenarios[i].ID == "" || public.Scenarios[i].ID != oracle.Scenarios[i].ID || strings.TrimSpace(public.Scenarios[i].Prompt) == "" {
			t.Fatalf("unpaired scenario %d: public=%+v oracle=%+v", i, public.Scenarios[i], oracle.Scenarios[i])
		}
		enabled := enabledToolNames(public.Scenarios[i].Toolsets)
		for _, tool := range oracle.Scenarios[i].AcceptableTools {
			if _, ok := enabled[tool]; !ok {
				t.Errorf("scenario %q cannot call acceptable tool %q with toolsets %v", public.Scenarios[i].ID, tool, public.Scenarios[i].Toolsets)
			}
		}
	}
}

const agentEvalV3Root = "testdata/agent-eval/v3"

type agentEvalV3Condition struct {
	ID        string   `json:"id"`
	Artifacts []string `json:"artifacts"`
}

type agentEvalV3PublicScenario struct {
	ID                string                 `json:"id"`
	Prompt            string                 `json:"prompt"`
	StartingArtifacts []string               `json:"starting_artifacts"`
	Conditions        []agentEvalV3Condition `json:"conditions"`
}

type agentEvalV3Public struct {
	Version                   string                      `json:"version"`
	FixtureRevision           string                      `json:"fixture_revision"`
	MinimumTrialsPerCondition int                         `json:"minimum_trials_per_condition"`
	Scenarios                 []agentEvalV3PublicScenario `json:"scenarios"`
	ReviewerHandoff           struct {
		Input string `json:"input"`
	} `json:"reviewer_handoff"`
}

type agentEvalV3OracleScenario struct {
	ID               string         `json:"id"`
	RequiredClaims   []string       `json:"required_claims"`
	RequiredEvidence []string       `json:"required_evidence"`
	HardFailures     []string       `json:"hard_failures"`
	Rubric           map[string]int `json:"rubric"`
}

type agentEvalV3Trajectory struct {
	ScenarioID string   `json:"scenario_id"`
	Claims     []string `json:"claims"`
	Evidence   []string `json:"evidence"`
}

func TestAgentEvalV3EvidenceBoundaryFixtures(t *testing.T) {
	t.Parallel()
	public := loadAgentEvalV3Public(t)
	oracles := loadAgentEvalV3Oracles(t, public)
	visibility := validateAgentEvalV3Manifest(t, public.FixtureRevision)
	validateAgentEvalV3ArtifactVisibility(t, public, oracles, visibility)
	validateAgentEvalV3Calibrations(t, public, oracles)
}

func readAgentEvalV3JSON(t *testing.T, path string, target any) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(agentEvalV3Root, path))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return data
}

func loadAgentEvalV3Public(t *testing.T) agentEvalV3Public {
	t.Helper()
	var public agentEvalV3Public
	data := readAgentEvalV3JSON(t, "public.json", &public)
	if public.Version != "agent-tool-eval.v3" || public.MinimumTrialsPerCondition < 5 || len(public.Scenarios) != 3 || strings.TrimSpace(public.ReviewerHandoff.Input) == "" {
		t.Fatalf("incomplete public fixture: %+v", public)
	}
	for _, leaked := range []string{"critical_discriminator", "tempting_wrong_path", "hard_failures", "correct_conclusion"} {
		if strings.Contains(string(data), leaked) {
			t.Fatalf("public fixture leaks oracle field %q", leaked)
		}
	}
	return public
}

func loadAgentEvalV3Oracles(t *testing.T, public agentEvalV3Public) map[string]agentEvalV3OracleScenario {
	t.Helper()
	var oracle struct {
		Version         string                      `json:"version"`
		FixtureRevision string                      `json:"fixture_revision"`
		Scenarios       []agentEvalV3OracleScenario `json:"scenarios"`
	}
	readAgentEvalV3JSON(t, filepath.Join("private", "oracle.json"), &oracle)
	if oracle.Version != "agent-tool-eval-oracle.v3" || oracle.FixtureRevision != public.FixtureRevision || len(oracle.Scenarios) != len(public.Scenarios) {
		t.Fatalf("public/oracle mismatch: public=%+v oracle=%+v", public, oracle)
	}
	oracles := make(map[string]agentEvalV3OracleScenario, len(oracle.Scenarios))
	for _, scenario := range oracle.Scenarios {
		totalWeight := 0
		for _, weight := range scenario.Rubric {
			totalWeight += weight
		}
		if scenario.ID == "" || len(scenario.RequiredClaims) == 0 || len(scenario.RequiredEvidence) == 0 || len(scenario.HardFailures) == 0 || totalWeight != 100 {
			t.Fatalf("incomplete oracle scenario: %+v", scenario)
		}
		oracles[scenario.ID] = scenario
	}
	return oracles
}

func validateAgentEvalV3Manifest(t *testing.T, fixtureRevision string) map[string]string {
	t.Helper()
	var manifest struct {
		FixtureRevision string `json:"fixture_revision"`
		Artifacts       []struct {
			Path       string `json:"path"`
			Visibility string `json:"visibility"`
			SHA256     string `json:"sha256"`
		} `json:"artifacts"`
	}
	readAgentEvalV3JSON(t, "manifest.json", &manifest)
	if manifest.FixtureRevision != fixtureRevision || len(manifest.Artifacts) < 8 {
		t.Fatalf("incomplete manifest: %+v", manifest)
	}
	visibility := make(map[string]string, len(manifest.Artifacts))
	for _, artifact := range manifest.Artifacts {
		if filepath.IsAbs(artifact.Path) || strings.Contains(artifact.Path, "..") {
			t.Fatalf("unsafe manifest path %q", artifact.Path)
		}
		data, err := os.ReadFile(filepath.Join(agentEvalV3Root, artifact.Path))
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != artifact.SHA256 {
			t.Fatalf("artifact %s hash = %s, want %s", artifact.Path, got, artifact.SHA256)
		}
		if _, exists := visibility[artifact.Path]; exists {
			t.Fatalf("duplicate manifest artifact %q", artifact.Path)
		}
		visibility[artifact.Path] = artifact.Visibility
	}
	return visibility
}

func validateAgentEvalV3ArtifactVisibility(t *testing.T, public agentEvalV3Public, oracles map[string]agentEvalV3OracleScenario, visibility map[string]string) {
	t.Helper()
	for _, scenario := range public.Scenarios {
		if strings.TrimSpace(scenario.Prompt) == "" || len(scenario.StartingArtifacts) != 1 || len(scenario.Conditions) < 3 {
			t.Fatalf("incomplete public scenario: %+v", scenario)
		}
		if _, ok := oracles[scenario.ID]; !ok {
			t.Fatalf("public scenario %q has no oracle", scenario.ID)
		}
		if visibility[scenario.StartingArtifacts[0]] != "candidate" {
			t.Fatalf("scenario %q artifact is not candidate-visible", scenario.ID)
		}
		conditionArtifacts := make(map[string]bool, len(scenario.Conditions))
		for _, condition := range scenario.Conditions {
			if condition.ID == "" || len(condition.Artifacts) != 1 {
				t.Fatalf("scenario %q has incomplete condition: %+v", scenario.ID, condition)
			}
			artifact := condition.Artifacts[0]
			if visibility[artifact] != "candidate" {
				t.Fatalf("scenario %q condition %q artifact is not candidate-visible", scenario.ID, condition.ID)
			}
			if conditionArtifacts[artifact] {
				t.Fatalf("scenario %q reuses condition artifact %q", scenario.ID, artifact)
			}
			conditionArtifacts[artifact] = true
		}
	}
}

func validateAgentEvalV3Calibrations(t *testing.T, public agentEvalV3Public, oracles map[string]agentEvalV3OracleScenario) {
	t.Helper()
	for _, name := range []string{"calibration-good.json", "calibration-bad.json"} {
		var fixture struct {
			ExpectedPass bool                    `json:"expected_pass"`
			Trajectories []agentEvalV3Trajectory `json:"trajectories"`
		}
		readAgentEvalV3JSON(t, filepath.Join("private", name), &fixture)
		if len(fixture.Trajectories) != len(public.Scenarios) {
			t.Fatalf("%s trajectory count = %d", name, len(fixture.Trajectories))
		}
		for _, trajectory := range fixture.Trajectories {
			if got := agentEvalV3TrajectoryPasses(trajectory, oracles); got != fixture.ExpectedPass {
				t.Fatalf("%s scenario %q pass = %v, want %v", name, trajectory.ScenarioID, got, fixture.ExpectedPass)
			}
		}
	}
}

func agentEvalV3TrajectoryPasses(value agentEvalV3Trajectory, oracles map[string]agentEvalV3OracleScenario) bool {
	scenario, ok := oracles[value.ScenarioID]
	if !ok {
		return false
	}
	claims := make(map[string]bool, len(value.Claims))
	for _, claim := range value.Claims {
		claims[claim] = true
	}
	evidence := make(map[string]bool, len(value.Evidence))
	for _, item := range value.Evidence {
		evidence[item] = true
	}
	for _, required := range scenario.RequiredClaims {
		if !claims[required] {
			return false
		}
	}
	for _, required := range scenario.RequiredEvidence {
		if !evidence[required] {
			return false
		}
	}
	for _, failure := range scenario.HardFailures {
		if claims[failure] {
			return false
		}
	}
	return true
}

func TestAgentEvalScriptedCurrentContracts(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	t.Run("known repository search is one bounded structured call", func(t *testing.T) {
		result := callAgentEvalTool(t, client, ToolSearchRepositories, map[string]any{
			"owner": "acme", "repo": "rocket", "limit": 5,
		})
		if result.IsError || result.StructuredContent == nil {
			t.Fatalf("search result = %+v", result)
		}
	})

	t.Run("ambiguous repository identity is rejected visibly", func(t *testing.T) {
		result, err := client.CallTool(context.Background(), &mcp.CallToolParams{
			Name: ToolSearchRepositories, Arguments: map[string]any{"owner": "acme"},
		})
		if err == nil && (result == nil || !result.IsError) {
			t.Fatalf("partial repository identity was accepted: result=%+v err=%v", result, err)
		}
		if result != nil && agentEvalToolError(t, result).Code != "invalid_argument" {
			t.Fatalf("error is not actionable: %+v", result.Content)
		}
	})

	t.Run("raw and structured search modes are rejected", func(t *testing.T) {
		result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: ToolSearchGitHubRepositories, Arguments: map[string]any{"raw_query": "topic:cuda", "language": "Go"}})
		if err == nil && (result == nil || !result.IsError) {
			t.Fatalf("ambiguous search was accepted: result=%+v err=%v", result, err)
		}
		if result != nil && agentEvalToolError(t, result).Code != "invalid_argument" {
			t.Fatalf("error is not actionable: %+v", result.Content)
		}
	})

	t.Run("durable operation exposes a pollable job", func(t *testing.T) {
		started := callAgentEvalTool(t, client, ToolBuildRepositoryDossier, map[string]any{"owner": "acme", "repo": "rocket"})
		payload, err := json.Marshal(started.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var job JobReference
		if err := json.Unmarshal(payload, &job); err != nil {
			t.Fatal(err)
		}
		if job.ID == "" || job.Ref != "job:"+job.ID || job.Status == "" || job.PollAfterMS < 1 || len(job.SuggestedActions) != 1 || job.SuggestedActions[0].Tool != ToolGetJob {
			t.Fatalf("job reference is not pollable: %+v", job)
		}
		polled := callAgentEvalTool(t, client, ToolGetJob, map[string]any{"ids": []string{job.ID}})
		if polled.IsError || polled.StructuredContent == nil {
			t.Fatalf("poll result = %+v", polled)
		}
	})
}

func TestAgentEvalToolSchemasAreLegible(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	for tool, err := range client.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("list tools: %v", err)
		}
		t.Run(tool.Name, func(t *testing.T) {
			if strings.TrimSpace(tool.Description) == "" {
				t.Fatal("description is empty")
			}
			schema, ok := tool.InputSchema.(map[string]any)
			if !ok {
				payload, err := json.Marshal(tool.InputSchema)
				if err != nil {
					t.Fatalf("marshal input schema: %v", err)
				}
				if err := json.Unmarshal(payload, &schema); err != nil {
					t.Fatalf("decode input schema: %v", err)
				}
			}
			if schema["type"] != "object" {
				t.Errorf("root schema type = %v, want object", schema["type"])
			}
			if _, exists := schema["allOf"]; exists {
				t.Error("root schema uses allOf; clients may render this as an unreadable intersection")
			}
			properties, ok := schema["properties"].(map[string]any)
			if !ok && schema["properties"] != nil {
				t.Fatal("schema properties are opaque")
			}
			for name, value := range properties {
				property, ok := value.(map[string]any)
				if !ok || len(property) == 0 {
					t.Errorf("property %q has an empty or opaque schema", name)
					continue
				}
				if strings.TrimSpace(agentEvalString(property["description"])) == "" {
					t.Errorf("property %q has no description", name)
				}
			}
		})
	}
}

func callAgentEvalTool(t *testing.T, client *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	return result
}

func agentEvalString(value any) string {
	text, _ := value.(string)
	return text
}

func agentEvalToolError(t *testing.T, result *mcp.CallToolResult) ToolError {
	t.Helper()
	for _, content := range result.Content {
		text, ok := content.(*mcp.TextContent)
		if !ok {
			continue
		}
		var toolError ToolError
		if json.Unmarshal([]byte(text.Text), &toolError) == nil && toolError.Code != "" {
			return toolError
		}
	}
	t.Fatalf("tool error content is not a ToolError JSON object: %+v", result.Content)
	return ToolError{}
}
