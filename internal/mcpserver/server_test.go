package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
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
	return GetCoverageOutput{
		Owner:  in.Owner,
		Repo:   in.Repo,
		AsOf:   "2026-07-17T00:00:00Z",
		Facets: []FacetCoverageOutput{{Facet: "metadata", Complete: true, Status: "fresh", UpdatedAt: "2026-07-17T00:00:00Z"}},
	}, nil
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

func (*fakeReader) SyncRepository(_ context.Context, in SyncRepositoryInput) (SyncRepositoryOutput, error) {
	return SyncRepositoryOutput{Owner: in.Owner, Repo: in.Repo, Updated: len(in.Numbers), Message: "synced"}, nil
}

func (*fakeReader) HydrateThread(_ context.Context, in HydrateThreadInput) (HydrateThreadOutput, error) {
	return HydrateThreadOutput{
		Owner: in.Owner, Repo: in.Repo, Number: in.Number, Kind: "issue", Requests: 1,
		Facets: []HydratedFacetOutput{{Facet: "issue_comments", Count: 2, Pages: 1, Complete: true}},
	}, nil
}

func (*fakeReader) SearchRepositories(_ context.Context, in SearchRepositoriesInput) (SearchRepositoriesOutput, error) {
	return SearchRepositoriesOutput{Query: in.Query, Total: 1, Matches: []RepositoryOutput{{Owner: in.Owner, Repo: in.Repo}}}, nil
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

func (*fakeReader) HydrateRepository(_ context.Context, in HydrateRepositoryInput) (JobReference, error) {
	return JobReference{ID: "job-" + in.Owner + "-" + in.Repo, Kind: "hydrate_repository", Status: "queued"}, nil
}

func (*fakeReader) BuildRepositoryDossier(_ context.Context, in BuildRepositoryDossierInput) (JobReference, error) {
	return JobReference{ID: "job-dossier-" + in.Owner + "-" + in.Repo, Kind: "build_repository_dossier", Status: "queued"}, nil
}

func (*fakeReader) StartCrawl(_ context.Context, in StartCrawlInput) (JobReference, error) {
	return JobReference{ID: "job-crawl-" + in.Source, Kind: "start_crawl", Status: "queued"}, nil
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

func (*fakeReader) CancelJob(_ context.Context, in CancelJobInput) (GetJobOutput, error) {
	return GetJobOutput{ID: in.ID, Kind: "crawl", Status: "cancelled"}, nil
}

func connect(t *testing.T, reader Reader) (*mcp.ClientSession, func()) {
	t.Helper()
	server := New(reader, "test")
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
		"search", "get_repository", "get_thread", "get_dossier",
		"search_code", "get_investigation", "list_opportunities", "get_opportunity", "get_evidence",
		"find_clusters", "get_coverage", "get_lens",
	} {
		tool := tools[name]
		if tool == nil {
			t.Fatalf("missing tool %q", name)
		}
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || !tool.Annotations.IdempotentHint {
			t.Fatalf("tool %q annotations = %+v", name, tool.Annotations)
		}
	}
	for _, name := range []string{"sync_repository", "hydrate_thread"} {
		tool := tools[name]
		if tool == nil {
			t.Fatalf("missing tool %q", name)
		}
		if tool.Annotations == nil || tool.Annotations.ReadOnlyHint || tool.Annotations.IdempotentHint || tool.Annotations.OpenWorldHint == nil || !*tool.Annotations.OpenWorldHint {
			t.Fatalf("operation tool %q annotations = %+v", name, tool.Annotations)
		}
	}

	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "search", Arguments: map[string]any{"query": "stall"},
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
		{"search_code", map[string]any{"query": "main"}, 1},
		{"get_investigation", map[string]any{"id": "inv-1"}, -1},
		{"list_opportunities", map[string]any{"investigation_id": "inv-1"}, 1},
		{"get_opportunity", map[string]any{"id": "opp-1"}, -1},
		{"get_evidence", map[string]any{"investigation_id": "inv-1"}, 1},
		{"find_clusters", map[string]any{"owner": "acme", "repo": "rocket"}, 1},
		{"get_coverage", map[string]any{"owner": "acme", "repo": "rocket"}, -1},
		{"get_lens", map[string]any{"name": "active-go"}, -1},
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
		case "search_code":
			var out SearchCodeOutput
			if err := json.Unmarshal(payload, &out); err != nil {
				t.Fatalf("decode %s: %v", tt.name, err)
			}
			if out.Total != tt.wantTotal || len(out.Matches) != tt.wantTotal {
				t.Fatalf("%s output = %+v", tt.name, out)
			}
		case "get_investigation":
			var out InvestigationOutput
			if err := json.Unmarshal(payload, &out); err != nil {
				t.Fatalf("decode %s: %v", tt.name, err)
			}
			if out.ID != "inv-1" {
				t.Fatalf("%s output = %+v", tt.name, out)
			}
		case "list_opportunities":
			var out ListOpportunitiesOutput
			if err := json.Unmarshal(payload, &out); err != nil {
				t.Fatalf("decode %s: %v", tt.name, err)
			}
			if out.Total != tt.wantTotal || len(out.Opportunities) != tt.wantTotal {
				t.Fatalf("%s output = %+v", tt.name, out)
			}
		case "get_opportunity":
			var out OpportunityOutput
			if err := json.Unmarshal(payload, &out); err != nil {
				t.Fatalf("decode %s: %v", tt.name, err)
			}
			if out.ID != "opp-1" {
				t.Fatalf("%s output = %+v", tt.name, out)
			}
		case "get_evidence":
			var out EvidenceOutput
			if err := json.Unmarshal(payload, &out); err != nil {
				t.Fatalf("decode %s: %v", tt.name, err)
			}
			if out.Total != tt.wantTotal || len(out.Evidence) != tt.wantTotal {
				t.Fatalf("%s output = %+v", tt.name, out)
			}
		case "find_clusters":
			var out FindClustersOutput
			if err := json.Unmarshal(payload, &out); err != nil {
				t.Fatalf("decode %s: %v", tt.name, err)
			}
			if out.Total != tt.wantTotal || len(out.Clusters) != tt.wantTotal {
				t.Fatalf("%s output = %+v", tt.name, out)
			}
		case "get_coverage":
			var out GetCoverageOutput
			if err := json.Unmarshal(payload, &out); err != nil {
				t.Fatalf("decode %s: %v", tt.name, err)
			}
			if out.Owner != "acme" || out.Repo != "rocket" || len(out.Facets) == 0 {
				t.Fatalf("%s output = %+v", tt.name, out)
			}
		case "get_lens":
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

func TestExplicitOperationToolsReturnStructuredOutput(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	syncResult, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "sync_repository", Arguments: map[string]any{"owner": "acme", "repo": "rocket", "numbers": []int{1, 2}},
	})
	if err != nil || syncResult.IsError {
		t.Fatalf("sync_repository: result=%+v err=%v", syncResult, err)
	}
	hydrateResult, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "hydrate_thread", Arguments: map[string]any{"owner": "acme", "repo": "rocket", "number": 7, "facets": []string{"issue_comments"}},
	})
	if err != nil || hydrateResult.IsError {
		t.Fatalf("hydrate_thread: result=%+v err=%v", hydrateResult, err)
	}
}

func TestReadOnlyToolsRejectAmbiguousOrUnboundedInputs(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()
	tests := []struct {
		name string
		args map[string]any
	}{
		{"get_investigation", map[string]any{"id": "inv-1", "hypothesis_limit": 101}},
		{"get_opportunity", map[string]any{"id": "opp-1", "evidence_limit": 101}},
		{"get_evidence", map[string]any{"investigation_id": "inv-1", "opportunity_id": "opp-1"}},
		{"get_evidence", map[string]any{}},
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
			Name: "search", Arguments: map[string]any{"query": "block"},
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
		"search_repositories", "search_threads", "get_repository_dossier", "explain_match", "get_job",
		"start_crawl", "hydrate_repository", "build_repository_dossier", "create_workspace", "run_validation",
		"start_investigation", "record_hypothesis", "check_duplicates", "check_collisions",
		"promote_opportunity", "define_validation", "prepare_contribution", "cancel_job",
	} {
		if tools[name] == nil {
			t.Fatalf("missing v1 tool %q", name)
		}
	}

	readTests := []struct {
		name string
		args map[string]any
	}{
		{"search_repositories", map[string]any{"query": "rocket"}},
		{"search_threads", map[string]any{"query": "stall"}},
		{"get_repository_dossier", map[string]any{"owner": "acme", "repo": "rocket"}},
		{"explain_match", map[string]any{"owner": "acme", "repo": "rocket", "kind": "issue", "number": 7}},
		{"get_job", map[string]any{"id": "job-1"}},
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
		{"start_crawl", map[string]any{"source": "go", "since": "720h", "budget": 10}},
		{"hydrate_repository", map[string]any{"owner": "acme", "repo": "rocket"}},
		{"build_repository_dossier", map[string]any{"owner": "acme", "repo": "rocket"}},
		{"create_workspace", map[string]any{"investigation_id": "inv-1", "remote": "https://github.com/acme/rocket.git", "base_ref": "main", "candidate_ref": "feature", "name": "ws-1"}},
		{"run_validation", map[string]any{"id": "val-1", "kind": "base", "execute": true}},
		{"start_investigation", map[string]any{"owner": "acme", "repo": "rocket"}},
		{"record_hypothesis", map[string]any{"investigation_id": "inv-1", "title": "leak", "description": "memory leak", "category": "bug"}},
		{"check_duplicates", map[string]any{"target": "hypothesis", "id": "hyp-1"}},
		{"check_collisions", map[string]any{"target": "opportunity", "id": "opp-1"}},
		{"promote_opportunity", map[string]any{"hypothesis_id": "hyp-1", "problem_statement": "leak", "scope": "small", "impact": "high", "expected_effort": "1h", "confidence": 0.8}},
		{"define_validation", map[string]any{"investigation_id": "inv-1", "kind": "test", "command": "go test ./...", "working_dir": "."}},
		{"prepare_contribution", map[string]any{"opportunity_id": "opp-1", "kind": "issue"}},
		{"cancel_job", map[string]any{"id": "job-1"}},
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
