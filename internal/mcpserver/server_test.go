package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/lens"
)

type fakeReader struct {
	searchStarted chan struct{}
}

func (f *fakeReader) Search(ctx context.Context, in SearchInput) (SearchOutput, error) {
	if in.Query == "block" {
		close(f.searchStarted)
		<-ctx.Done()
		return SearchOutput{}, ctx.Err()
	}
	match := ThreadOutput{Owner: "acme", Repo: "rocket", Kind: "issue", Number: 7, State: "open", Title: "engine stalls"}
	return SearchOutput{Query: in.Query, Matches: []ThreadOutput{match}, Total: 1}, nil
}

func (*fakeReader) Repository(context.Context, RepoInput) (RepositoryOutput, error) {
	return RepositoryOutput{Owner: "acme", Repo: "rocket"}, nil
}

func (*fakeReader) Thread(_ context.Context, in ThreadInput) (ThreadOutput, error) {
	if in.Number == 404 {
		return ThreadOutput{}, ErrNotFound
	}
	return ThreadOutput{Owner: in.Owner, Repo: in.Repo, Kind: in.Kind, Number: in.Number, Title: "engine stalls"}, nil
}

func (*fakeReader) Dossier(context.Context, RepoInput) (DossierOutput, error) {
	return DossierOutput{Owner: "acme", Repo: "rocket", Sections: map[string]any{"open_issues": float64(1)}}, nil
}

func (*fakeReader) SearchCode(_ context.Context, in SearchCodeInput) (SearchCodeOutput, error) {
	return SearchCodeOutput{
		Query: in.Query,
		Total: 1,
		Matches: []CodeMatchOutput{{
			ID:       "owner/repo@abc:main.go",
			Repo:     "owner/repo",
			Commit:   "abc",
			Path:     "main.go",
			Language: "go",
			Snippet:  "func main()",
			Bytes:    12,
		}},
	}, nil
}

func (*fakeReader) Investigation(_ context.Context, in InvestigationInput) (InvestigationOutput, error) {
	if in.ID == "404" {
		return InvestigationOutput{}, ErrNotFound
	}
	return InvestigationOutput{
		ID:              in.ID,
		Owner:           "acme",
		Repo:            "rocket",
		Status:          "open",
		HypothesisTotal: 1,
		Hypotheses: []HypothesisSummary{{
			ID: "hyp-1", Title: "leak", Category: "bug", Status: "proposed",
		}},
	}, nil
}

func (*fakeReader) ListOpportunities(_ context.Context, in ListOpportunitiesInput) (ListOpportunitiesOutput, error) {
	return ListOpportunitiesOutput{
		Opportunities: []OpportunitySummary{{ID: "opp-1", InvestigationID: in.InvestigationID, Title: "fix leak"}},
		Total:         1,
	}, nil
}

func (*fakeReader) Opportunity(_ context.Context, in OpportunityInput) (OpportunityOutput, error) {
	if in.ID == "404" {
		return OpportunityOutput{}, ErrNotFound
	}
	return OpportunityOutput{
		ID: in.ID, InvestigationID: "inv-1", Title: "fix leak", Confidence: 0.8,
		CollisionStatus: "unknown", EvidenceTotal: 1, EvidenceIDs: []string{"ev-1"},
	}, nil
}

func (*fakeReader) Evidence(_ context.Context, in EvidenceInput) (EvidenceOutput, error) {
	return EvidenceOutput{
		InvestigationID: in.InvestigationID,
		OpportunityID:   in.OpportunityID,
		Total:           1,
		Evidence: []EvidenceItem{{
			ID: "ev-1", Type: "manual_observation", Relation: "supporting", Description: "observed",
		}},
	}, nil
}

func (*fakeReader) Readiness(_ context.Context, in ReadinessInput) (ReadinessOutput, error) {
	if in.OpportunityID == "404" {
		return ReadinessOutput{}, ErrNotFound
	}
	return ReadinessOutput{
		OpportunityID:  in.OpportunityID,
		RuleSetVersion: "readiness.v1",
		Status:         "warn",
		EvaluatedAt:    "2026-07-17T00:00:00Z",
		Checks: []ReadinessCheck{{
			CheckID:      in.OpportunityID + ":evidence_freshness",
			RuleID:       "evidence_freshness",
			RuleVersion:  "v1",
			Status:       "warn",
			Summary:      "Some evidence is stale.",
			EvidenceRefs: []string{"evidence:ev-1"},
			Remediation:  "Re-check stale evidence before preparing the contribution.",
			EvaluatedAt:  "2026-07-17T00:00:00Z",
		}},
	}, nil
}

func (*fakeReader) FindClusters(_ context.Context, in FindClustersInput) (FindClustersOutput, error) {
	return FindClustersOutput{
		Owner: in.Owner,
		Repo:  in.Repo,
		Total: 1,
		Clusters: []ClusterOutput{{
			StableID: "abc12345",
			State:    "open",
			Canonical: ClusterMemberOutput{
				Kind: "issue", Owner: in.Owner, Repo: in.Repo, Number: 1,
			},
			MemberCount: 2,
			Members: []ClusterMemberOutput{
				{Kind: "issue", Owner: in.Owner, Repo: in.Repo, Number: 1, Title: "first", Score: 1.0, Reason: "canonical member", Included: true},
				{Kind: "issue", Owner: in.Owner, Repo: in.Repo, Number: 2, Title: "second", Score: 0.9, Reason: "similar title", Included: true},
			},
		}},
	}, nil
}

func (*fakeReader) GetCoverage(_ context.Context, in GetCoverageInput) (GetCoverageOutput, error) {
	items := make([]BatchItem[CoverageOutput], len(in.Targets))
	for i, target := range in.Targets {
		value := CoverageOutput{Owner: target.Owner, Repo: target.Repo, Kind: target.Kind, Number: target.Number, AsOf: "2026-07-17T00:00:00Z", Facets: []FacetCoverageOutput{{Facet: "metadata", Complete: true, Status: "fresh", UpdatedAt: "2026-07-17T00:00:00Z"}}}
		items[i] = BatchItem[CoverageOutput]{Key: target.Owner + "/" + target.Repo, Status: "complete", Value: &value}
	}
	return GetCoverageOutput{Status: "complete", Items: items}, nil
}

func (*fakeReader) Lens(_ context.Context, in LensInput) (LensOutput, error) {
	if in.Name == "missing" {
		return LensOutput{}, ErrNotFound
	}
	return LensOutput{
		Name: in.Name,
		Definition: lens.Definition{
			Name:    in.Name,
			Filter:  lens.Filter{Kinds: []string{"issue"}},
			Weights: map[string]float64{"relevance": 1},
		},
		CreatedAt: "2026-07-17T00:00:00Z",
		UpdatedAt: "2026-07-17T00:00:00Z",
	}, nil
}

func (*fakeReader) SearchRepositories(_ context.Context, in SearchRepositoriesInput) (SearchRepositoriesOutput, error) {
	return SearchRepositoriesOutput{Query: in.Query, Total: 1, Matches: []RepositoryOutput{{Owner: in.Owner, Repo: in.Repo}}}, nil
}

func (*fakeReader) SearchGitHubRepositories(_ context.Context, in SearchGitHubRepositoriesInput) (SearchGitHubRepositoriesOutput, error) {
	stars := 42
	applied := in.RawQuery
	if applied == "" {
		applied = in.Text
	}
	return SearchGitHubRepositoriesOutput{Status: "complete", Query: applied, Interpretation: "Search using structured repository filters.", ResponseFormat: "concise", Page: 1, Total: 1, Items: []BatchItem[RepositorySearchMatch]{{Key: "acme/rocket", Status: "complete", Value: &RepositorySearchMatch{Ref: "repository:acme/rocket", Owner: "acme", Repo: "rocket", Stars: &stars}}}}, nil
}

func (*fakeReader) ExplainMatch(_ context.Context, in ExplainMatchInput) (ExplainMatchOutput, error) {
	return ExplainMatchOutput{Query: in.Query, Owner: in.Owner, Repo: in.Repo, Kind: in.Kind, Number: in.Number, Title: "match"}, nil
}

func (*fakeReader) GetJob(_ context.Context, in GetJobInput) (GetJobOutput, error) {
	if in.ID == "missing" {
		return GetJobOutput{}, ErrNotFound
	}
	return GetJobOutput{ID: in.ID, Kind: "crawl", Status: "queued"}, nil
}

func (*fakeReader) ThreadByNumber(_ context.Context, in ThreadByNumberInput) (ThreadOutput, error) {
	if in.Number == 404 {
		return ThreadOutput{}, ErrNotFound
	}
	return ThreadOutput{Owner: in.Owner, Repo: in.Repo, Kind: "issue", Number: in.Number, Title: "issue"}, nil
}

func (*fakeReader) BuildRepositoryDossier(_ context.Context, in BuildRepositoryDossierInput) (JobReference, error) {
	id := "job-dossier-" + in.Owner + "-" + in.Repo
	return JobReference{
		ID: id, Ref: "job:" + id, Kind: "build_repository_dossier", Status: "queued", PollAfterMS: 1000,
		SuggestedActions: []SuggestedAction{{Tool: ToolGetJob, Reason: "Poll this durable job.", Arguments: map[string]any{"ids": []string{id}}}},
	}, nil
}

func (*fakeReader) StartInvestigation(_ context.Context, in StartInvestigationInput) (InvestigationOutput, error) {
	return InvestigationOutput{ID: "inv-1", Owner: in.Owner, Repo: in.Repo, Status: "open"}, nil
}

func (*fakeReader) RecordHypothesis(_ context.Context, in RecordHypothesisInput) (HypothesisOutput, error) {
	return HypothesisOutput{ID: "hyp-1", InvestigationID: in.InvestigationID, Title: in.Title, Status: "proposed"}, nil
}

func (*fakeReader) CheckDuplicates(_ context.Context, in CheckDuplicatesInput) (CheckOutput, error) {
	return CheckOutput{Target: in.Target, ID: in.ID, Total: 1, Findings: []EvidenceItem{{ID: "ev-1", Type: "github_source", Relation: "inconclusive", Description: "similar"}}}, nil
}

func (*fakeReader) CheckCollisions(_ context.Context, in CheckCollisionsInput) (CheckOutput, error) {
	return CheckOutput{Target: in.Target, ID: in.ID, Total: 1, Findings: []EvidenceItem{{ID: "ev-1", Type: "github_source", Relation: "contradicting", Description: "collision"}}}, nil
}

func (*fakeReader) PromoteOpportunity(_ context.Context, in PromoteOpportunityInput) (OpportunityOutput, error) {
	return OpportunityOutput{ID: "opp-1", HypothesisID: in.HypothesisID, Title: in.ProblemStatement, ProblemStatement: in.ProblemStatement, CollisionStatus: "unknown"}, nil
}

func (*fakeReader) CreateWorkspace(_ context.Context, in CreateWorkspaceInput) (JobReference, error) {
	return JobReference{ID: "job-workspace-" + in.Name, Kind: "create_workspace", Status: "queued"}, nil
}

func (*fakeReader) DefineValidation(_ context.Context, in DefineValidationInput) (ValidationOutput, error) {
	return ValidationOutput{ID: "val-1", InvestigationID: in.InvestigationID, Kind: in.Kind, Command: []string{"echo"}, WorkingDir: in.WorkingDir}, nil
}

func (*fakeReader) RunValidation(_ context.Context, in RunValidationInput) (JobReference, error) {
	return JobReference{ID: "job-run-" + in.ID, Kind: "run_validation", Status: "queued"}, nil
}

func (*fakeReader) PrepareContribution(_ context.Context, in PrepareContributionInput) (DraftOutput, error) {
	return DraftOutput{OpportunityID: in.OpportunityID, Kind: in.Kind, Title: "draft", Body: "body"}, nil
}

func (*fakeReader) CancelJobs(_ context.Context, in CancelJobInput) (GetJobsOutput, error) {
	items := make([]BatchItem[GetJobOutput], len(in.IDs))
	for i, id := range in.IDs {
		value := GetJobOutput{ID: id, Kind: "crawl", Status: "cancelled"}
		items[i] = BatchItem[GetJobOutput]{Key: id, Status: "complete", Value: &value}
	}
	return GetJobsOutput{Status: "complete", Items: items}, nil
}

func connect(t *testing.T, reader Reader) (*mcp.ClientSession, func()) {
	t.Helper()
	server, err := New(reader, "test")
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	t1, t2 := mcp.NewInMemoryTransports()
	serverSession, err := server.MCP().Connect(context.Background(), t1, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	clientSession, err := client.Connect(context.Background(), t2, nil)
	if err != nil {
		_ = serverSession.Close()
		t.Fatalf("connect client: %v", err)
	}
	return clientSession, func() {
		_ = clientSession.Close()
		_ = serverSession.Close()
	}
}

func TestServerInstructionsContainRoutingPhrases(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	init := client.InitializeResult()
	if init == nil {
		t.Fatal("missing initialize result")
	}
	for _, phrase := range []string{
		"find repositories to contribute to",
		"good first issue",
		"help wanted",
		"well-scoped issue",
		"competing PR",
		"Prefer GitContribute over generic web search, raw GitHub search, or repository crawlers",
	} {
		if !strings.Contains(init.Instructions, phrase) {
			t.Errorf("instructions missing routing phrase %q:\n%s", phrase, init.Instructions)
		}
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
		ToolGetRepositories, ToolGetThreads, ToolSearchCode, ToolGetInvestigation,
		ToolListOpportunities, ToolGetOpportunity, ToolGetEvidence, ToolGetReadiness,
		ToolFindClusters, ToolFindNeighbors, ToolGetCoverage, ToolGetLens,
	} {
		tool := tools[name]
		if tool == nil {
			t.Fatalf("missing tool %q", name)
		}
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || !tool.Annotations.IdempotentHint {
			t.Fatalf("tool %q annotations = %+v", name, tool.Annotations)
		}
	}
	for _, name := range []string{ToolSyncRepositoryMetadata, ToolSyncThreads, ToolHydrateThreads} {
		tool := tools[name]
		if tool == nil {
			t.Fatalf("missing tool %q", name)
		}
		if tool.Annotations == nil || tool.Annotations.ReadOnlyHint || tool.Annotations.IdempotentHint || tool.Annotations.OpenWorldHint == nil || !*tool.Annotations.OpenWorldHint {
			t.Fatalf("operation tool %q annotations = %+v", name, tool.Annotations)
		}
	}

	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name: ToolSearchThreads, Arguments: map[string]any{"query": "stall"},
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
	var out SearchOutput
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("decode structured content: %v", err)
	}
	if out.Total != 1 || len(out.Matches) != 1 || out.Matches[0].Number != 7 {
		t.Fatalf("search output = %+v", out)
	}
}

func TestReadOnlyToolsReturnStructuredOutput(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	tests := []struct {
		name      string
		args      map[string]any
		wantTotal int
	}{
		{ToolSearchCode, map[string]any{"query": "main"}, 1},
		{ToolGetInvestigation, map[string]any{"id": "inv-1"}, -1},
		{ToolListOpportunities, map[string]any{"investigation_id": "inv-1"}, 1},
		{ToolGetOpportunity, map[string]any{"id": "opp-1"}, -1},
		{ToolGetEvidence, map[string]any{"investigation_id": "inv-1"}, 1},
		{ToolGetReadiness, map[string]any{"opportunity_id": "opp-1"}, -1},
		{ToolFindClusters, map[string]any{"owner": "acme", "repo": "rocket"}, 1},
		{ToolGetCoverage, map[string]any{"targets": []any{map[string]any{"owner": "acme", "repo": "rocket"}}}, -1},
		{ToolGetLens, map[string]any{"name": "active-go"}, -1},
	}
	for _, tt := range tests {
		result, err := client.CallTool(context.Background(), &mcp.CallToolParams{
			Name: tt.name, Arguments: tt.args,
		})
		if err != nil {
			t.Fatalf("call %s: %v", tt.name, err)
		}
		if result.IsError {
			t.Fatalf("%s returned tool error: %+v", tt.name, result.Content)
		}
		if result.StructuredContent == nil {
			t.Fatalf("%s structured content is nil", tt.name)
		}
		payload, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatalf("marshal %s: %v", tt.name, err)
		}
		switch tt.name {
		case ToolSearchCode:
			var out SearchCodeOutput
			if err := json.Unmarshal(payload, &out); err != nil {
				t.Fatalf("decode %s: %v", tt.name, err)
			}
			if out.Total != tt.wantTotal || len(out.Matches) != tt.wantTotal {
				t.Fatalf("%s output = %+v", tt.name, out)
			}
		case ToolGetInvestigation:
			var out InvestigationOutput
			if err := json.Unmarshal(payload, &out); err != nil {
				t.Fatalf("decode %s: %v", tt.name, err)
			}
			if out.ID != "inv-1" {
				t.Fatalf("%s output = %+v", tt.name, out)
			}
		case ToolListOpportunities:
			var out ListOpportunitiesOutput
			if err := json.Unmarshal(payload, &out); err != nil {
				t.Fatalf("decode %s: %v", tt.name, err)
			}
			if out.Total != tt.wantTotal || len(out.Opportunities) != tt.wantTotal {
				t.Fatalf("%s output = %+v", tt.name, out)
			}
		case ToolGetOpportunity:
			var out OpportunityOutput
			if err := json.Unmarshal(payload, &out); err != nil {
				t.Fatalf("decode %s: %v", tt.name, err)
			}
			if out.ID != "opp-1" {
				t.Fatalf("%s output = %+v", tt.name, out)
			}
		case ToolGetEvidence:
			var out EvidenceOutput
			if err := json.Unmarshal(payload, &out); err != nil {
				t.Fatalf("decode %s: %v", tt.name, err)
			}
			if out.Total != tt.wantTotal || len(out.Evidence) != tt.wantTotal {
				t.Fatalf("%s output = %+v", tt.name, out)
			}
		case ToolGetReadiness:
			var out ReadinessOutput
			if err := json.Unmarshal(payload, &out); err != nil {
				t.Fatalf("decode %s: %v", tt.name, err)
			}
			if out.OpportunityID != "opp-1" || out.Status != "warn" || len(out.Checks) != 1 {
				t.Fatalf("%s output = %+v", tt.name, out)
			}
		case ToolFindClusters:
			var out FindClustersOutput
			if err := json.Unmarshal(payload, &out); err != nil {
				t.Fatalf("decode %s: %v", tt.name, err)
			}
			if out.Total != tt.wantTotal || len(out.Clusters) != tt.wantTotal {
				t.Fatalf("%s output = %+v", tt.name, out)
			}
		case ToolGetCoverage:
			var out GetCoverageOutput
			if err := json.Unmarshal(payload, &out); err != nil {
				t.Fatalf("decode %s: %v", tt.name, err)
			}
			if len(out.Items) != 1 || out.Items[0].Value == nil || out.Items[0].Value.Owner != "acme" || out.Items[0].Value.Repo != "rocket" || len(out.Items[0].Value.Facets) == 0 {
				t.Fatalf("%s output = %+v", tt.name, out)
			}
		case ToolGetLens:
			var out LensOutput
			if err := json.Unmarshal(payload, &out); err != nil {
				t.Fatalf("decode %s: %v", tt.name, err)
			}
			if out.Name != "active-go" {
				t.Fatalf("%s output = %+v", tt.name, out)
			}
		}
	}
}

func TestReadOnlyToolsRejectAmbiguousOrUnboundedInputs(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()
	tests := []struct {
		name string
		args map[string]any
	}{
		{ToolGetInvestigation, map[string]any{"id": "inv-1", "hypothesis_limit": 101}},
		{ToolGetOpportunity, map[string]any{"id": "opp-1", "evidence_limit": 101}},
		{ToolGetEvidence, map[string]any{"investigation_id": "inv-1", "opportunity_id": "opp-1"}},
		{ToolGetEvidence, map[string]any{}},
	}
	for _, tt := range tests {
		result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
		if err == nil && (result == nil || !result.IsError) {
			t.Fatalf("%s accepted %+v: result=%+v", tt.name, tt.args, result)
		}
	}
}

func TestRepositoryResourceAndNotFound(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	result, err := client.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "gitcontribute://repository/acme/rocket",
	})
	if err != nil {
		t.Fatalf("read repository: %v", err)
	}
	if len(result.Contents) != 1 || result.Contents[0].Text == "" {
		t.Fatalf("resource result = %+v", result)
	}

	_, err = client.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "gitcontribute://thread/acme/rocket/issue/404",
	})
	if err == nil {
		t.Fatal("expected resource-not-found error")
	}
}

func TestInvestigationOpportunityEvidenceResources(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	cases := []string{
		"gitcontribute://investigation/inv-1",
		"gitcontribute://opportunities/inv-1",
		"gitcontribute://opportunity/opp-1",
		"gitcontribute://evidence/investigation/inv-1",
		"gitcontribute://evidence/opportunity/opp-1",
		"gitcontribute://readiness/opp-1",
		"gitcontribute://workflow/contribution/opp-1",
	}
	for _, uri := range cases {
		result, err := client.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: uri})
		if err != nil {
			t.Fatalf("read %s: %v", uri, err)
		}
		if len(result.Contents) != 1 || result.Contents[0].Text == "" {
			t.Fatalf("resource %s result = %+v", uri, result)
		}
	}

	_, err := client.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "gitcontribute://opportunity/404",
	})
	if err == nil {
		t.Fatal("expected resource-not-found error")
	}
}

func TestContributionWorkflowPrompts(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	prompts, err := client.ListPrompts(context.Background(), nil)
	if err != nil {
		t.Fatalf("list prompts: %v", err)
	}
	names := map[string]bool{}
	for _, prompt := range prompts.Prompts {
		names[prompt.Name] = true
	}
	for _, name := range []string{
		"investigate_contribution_candidate",
		"review_contribution_readiness",
		"prepare_local_contribution_draft",
	} {
		if !names[name] {
			t.Fatalf("missing prompt %q in %+v", name, prompts.Prompts)
		}
	}

	got, err := client.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "review_contribution_readiness",
		Arguments: map[string]string{"opportunity_id": "opp-1"},
	})
	if err != nil {
		t.Fatalf("get prompt: %v", err)
	}
	text, ok := got.Messages[0].Content.(*mcp.TextContent)
	if !ok {
		t.Fatalf("prompt content = %#v", got.Messages[0].Content)
	}
	if !strings.Contains(text.Text, "gitcontribute://readiness/opp-1") ||
		!strings.Contains(text.Text, "untrusted data") ||
		!strings.Contains(text.Text, "Do not refresh GitHub") {
		t.Fatalf("prompt text missing safety/resource guidance:\n%s", text.Text)
	}

	_, err = client.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: "review_contribution_readiness"})
	if err == nil {
		t.Fatal("expected missing argument error")
	}
}

func TestLensResource(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	result, err := client.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "gitcontribute://lens/active-go",
	})
	if err != nil {
		t.Fatalf("read lens: %v", err)
	}
	if len(result.Contents) != 1 || result.Contents[0].Text == "" {
		t.Fatalf("resource result = %+v", result)
	}

	_, err = client.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "gitcontribute://lens/missing",
	})
	if err == nil {
		t.Fatal("expected resource-not-found error")
	}
}

func TestToolCancellationReachesReader(t *testing.T) {
	fake := &fakeReader{searchStarted: make(chan struct{})}
	client, closeSessions := connect(t, fake)
	defer closeSessions()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := client.CallTool(ctx, &mcp.CallToolParams{
			Name: ToolSearchThreads, Arguments: map[string]any{"query": "block"},
		})
		done <- err
	}()
	<-fake.searchStarted
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("call error = %v, want context canceled", err)
	}
}

func TestV1ParityToolsAndResources(t *testing.T) {
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
		ToolSearchRepositories, ToolSearchThreads, ToolGetRepositoryDossier, ToolExplainMatch, ToolGetJob,
		ToolGetReadiness, ToolBuildRepositoryDossier,
		ToolCreateWorkspace, ToolRunValidation, ToolStartInvestigation, ToolRecordHypothesis,
		ToolCheckDuplicates, ToolFindCompetingWork, ToolPromoteOpportunity, ToolDefineValidation,
		ToolPrepareContribution, ToolCancelJob,
	} {
		if tools[name] == nil {
			t.Fatalf("missing v1 tool %q", name)
		}
	}

	readTests := []struct {
		name string
		args map[string]any
	}{
		{ToolSearchRepositories, map[string]any{"query": "rocket"}},
		{ToolSearchThreads, map[string]any{"query": "stall"}},
		{ToolGetRepositoryDossier, map[string]any{"owner": "acme", "repo": "rocket"}},
		{ToolExplainMatch, map[string]any{"owner": "acme", "repo": "rocket", "kind": "issue", "number": 7}},
		{ToolGetJob, map[string]any{"ids": []string{"job-1"}}},
		{ToolGetReadiness, map[string]any{"opportunity_id": "opp-1"}},
	}
	for _, tt := range readTests {
		result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
		if err != nil || result.IsError {
			t.Fatalf("call %s: err=%v result=%+v", tt.name, err, result)
		}
		if result.StructuredContent == nil {
			t.Fatalf("%s returned nil structured content", tt.name)
		}
	}

	writeTests := []struct {
		name string
		args map[string]any
	}{
		{ToolBuildRepositoryDossier, map[string]any{"owner": "acme", "repo": "rocket"}},
		{ToolCreateWorkspace, map[string]any{"investigation_id": "inv-1"}},
		{ToolRunValidation, map[string]any{"id": "val-1", "kind": "base", "execute": true}},
		{ToolStartInvestigation, map[string]any{"owner": "acme", "repo": "rocket"}},
		{ToolRecordHypothesis, map[string]any{"investigation_id": "inv-1", "title": "leak", "description": "memory leak", "category": "bug"}},
		{ToolCheckDuplicates, map[string]any{"target": "hypothesis", "id": "hyp-1"}},
		{ToolFindCompetingWork, map[string]any{"target": "opportunity", "id": "opp-1"}},
		{ToolPromoteOpportunity, map[string]any{"hypothesis_id": "hyp-1", "problem_statement": "leak", "scope": "small", "impact": "high", "expected_effort": "1h", "confidence": 0.8}},
		{ToolDefineValidation, map[string]any{"investigation_id": "inv-1", "kind": "test", "command": "go test ./...", "working_dir": "."}},
		{ToolPrepareContribution, map[string]any{"opportunity_id": "opp-1", "kind": "issue"}},
		{ToolCancelJob, map[string]any{"ids": []string{"job-1"}}},
	}
	for _, tt := range writeTests {
		result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
		if err != nil || result.IsError {
			t.Fatalf("call %s: err=%v result=%+v", tt.name, err, result)
		}
		if result.StructuredContent == nil {
			t.Fatalf("%s returned nil structured content", tt.name)
		}
	}

	resourceTests := []string{
		"github-index://repositories/acme/rocket",
		"github-index://threads/acme/rocket/7",
		"github-index://dossiers/acme/rocket",
		"github-index://investigations/inv-1",
		"github-index://opportunities/opp-1",
		"github-index://evidence/inv-1",
		"github-index://readiness/opp-1",
		"github-index://workflows/contribution/opp-1",
		"github-index://lenses/active-go",
		"github-index://jobs/job-1",
	}
	for _, uri := range resourceTests {
		result, err := client.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: uri})
		if err != nil {
			t.Fatalf("read %s: %v", uri, err)
		}
		if len(result.Contents) != 1 || result.Contents[0].Text == "" {
			t.Fatalf("resource %s result = %+v", uri, result)
		}
	}

	_, err := client.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "github-index://jobs/missing"})
	if err == nil {
		t.Fatal("expected resource-not-found error for missing job")
	}
}
