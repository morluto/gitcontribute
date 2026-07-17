package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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
	} {
		tool := tools[name]
		if tool == nil {
			t.Fatalf("missing tool %q", name)
		}
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || !tool.Annotations.IdempotentHint {
			t.Fatalf("tool %q annotations = %+v", name, tool.Annotations)
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
