package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/lens"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

type fakeReader struct {
	searchStarted chan struct{}
	repeatInput   mcpcontract.RunRepeatedValidationInput
	radarScore    int
	calls         map[string]int
}

var _ PublishedDraftVerifier = (*fakeReader)(nil)
var _ ValidationReceiptOperator = (*fakeReader)(nil)

func (*fakeReader) GetFixPatternReport(context.Context, string) (mcpcontract.FixPatternReport, error) {
	return mcpcontract.FixPatternReport{
		Status:     "complete",
		Repository: mcpcontract.RepositoryRef{Owner: "acme", Repo: "rocket"},
		TimeWindow: mcpcontract.FixPatternTimeWindow{UpdatedAfter: "2026-07-01T00:00:00Z"},
		Coverage:   mcpcontract.FixPatternCoverage{UniqueCandidates: 21},
	}, nil
}

func (f *fakeReader) recordCall(name string) {
	if f.calls == nil {
		f.calls = make(map[string]int)
	}
	f.calls[name]++
}

var _ WorkspaceCreator = (*fakeReader)(nil)
var _ WorkspaceAdopter = (*fakeReader)(nil)

func (f *fakeReader) Search(ctx context.Context, in mcpcontract.SearchInput) (mcpcontract.SearchOutput, error) {
	if in.Query == "block" {
		close(f.searchStarted)
		<-ctx.Done()
		return mcpcontract.SearchOutput{}, ctx.Err()
	}
	match := mcpcontract.ThreadOutput{Owner: "acme", Repo: "rocket", Kind: "issue", Number: 7, State: "open", Title: "engine stalls"}
	return mcpcontract.SearchOutput{Query: in.Query, Matches: []mcpcontract.ThreadOutput{match}, Total: 1}, nil
}

func (*fakeReader) Repository(context.Context, mcpcontract.RepoInput) (mcpcontract.RepositoryOutput, error) {
	return mcpcontract.RepositoryOutput{Owner: "acme", Repo: "rocket"}, nil
}

func (*fakeReader) Thread(_ context.Context, in mcpcontract.ThreadInput) (mcpcontract.ThreadOutput, error) {
	if in.Number == 404 {
		return mcpcontract.ThreadOutput{}, mcpcontract.ErrNotFound
	}
	return mcpcontract.ThreadOutput{Owner: in.Owner, Repo: in.Repo, Kind: in.Kind, Number: in.Number, Title: "engine stalls"}, nil
}

func (*fakeReader) Dossier(context.Context, mcpcontract.RepoInput) (mcpcontract.DossierOutput, error) {
	return mcpcontract.DossierOutput{Owner: "acme", Repo: "rocket", Sections: mcpcontract.DossierSections{OpenIssues: 1}}, nil
}

func (*fakeReader) SearchCode(_ context.Context, in mcpcontract.SearchCodeInput) (mcpcontract.SearchCodeOutput, error) {
	return mcpcontract.SearchCodeOutput{
		Query: in.Query,
		Total: 1,
		Matches: []mcpcontract.CodeMatchOutput{{
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

func (*fakeReader) Investigation(_ context.Context, in mcpcontract.InvestigationInput) (mcpcontract.InvestigationOutput, error) {
	if in.ID == "404" {
		return mcpcontract.InvestigationOutput{}, mcpcontract.ErrNotFound
	}
	return mcpcontract.InvestigationOutput{
		ID:              in.ID,
		Owner:           "acme",
		Repo:            "rocket",
		Status:          "open",
		HypothesisTotal: 1,
		Hypotheses: []mcpcontract.HypothesisSummary{{
			ID: "hyp-1", Title: "leak", Category: "bug", Status: "proposed",
		}},
	}, nil
}

func (*fakeReader) ListOpportunities(_ context.Context, in mcpcontract.ListOpportunitiesInput) (mcpcontract.ListOpportunitiesOutput, error) {
	return mcpcontract.ListOpportunitiesOutput{
		Opportunities: []mcpcontract.OpportunitySummary{{ID: "opp-1", InvestigationID: in.InvestigationID, Title: "fix leak"}},
		Total:         1,
	}, nil
}

func (*fakeReader) Opportunity(_ context.Context, in mcpcontract.OpportunityInput) (mcpcontract.OpportunityOutput, error) {
	if in.ID == "404" {
		return mcpcontract.OpportunityOutput{}, mcpcontract.ErrNotFound
	}
	return mcpcontract.OpportunityOutput{
		ID: in.ID, InvestigationID: "inv-1", Title: "fix leak", Confidence: 0.8,
		CollisionStatus: "unknown", EvidenceTotal: 1, EvidenceIDs: []string{"ev-1"},
	}, nil
}

func (*fakeReader) Evidence(_ context.Context, in mcpcontract.EvidenceInput) (mcpcontract.EvidenceOutput, error) {
	return mcpcontract.EvidenceOutput{
		InvestigationID: in.InvestigationID,
		OpportunityID:   in.OpportunityID,
		Total:           1,
		Evidence: []mcpcontract.EvidenceItem{{
			ID: "ev-1", Type: "manual_observation", Relation: "supporting", Description: "observed",
		}},
	}, nil
}

func (*fakeReader) Readiness(_ context.Context, in mcpcontract.ReadinessInput) (mcpcontract.ReadinessOutput, error) {
	if in.OpportunityID == "404" {
		return mcpcontract.ReadinessOutput{}, mcpcontract.ErrNotFound
	}
	return mcpcontract.ReadinessOutput{
		OpportunityID:  in.OpportunityID,
		RuleSetVersion: "readiness.v1",
		Status:         "warn",
		EvaluatedAt:    "2026-07-17T00:00:00Z",
		Checks: []mcpcontract.ReadinessCheck{{
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

func (*fakeReader) FindClusters(_ context.Context, in mcpcontract.FindClustersInput) (mcpcontract.FindClustersOutput, error) {
	items := make([]mcpcontract.BatchItem[mcpcontract.ClusterSetOutput], len(in.Targets))
	for i, target := range in.Targets {
		value := mcpcontract.ClusterSetOutput{
			Owner: target.Owner,
			Repo:  target.Repo,
			Total: 1,
			Clusters: []mcpcontract.ClusterOutput{{
				StableID: "abc12345",
				State:    "open",
				Canonical: mcpcontract.ClusterMemberOutput{
					Kind: "issue", Owner: target.Owner, Repo: target.Repo, Number: 1,
				},
				MemberCount: 2,
				Members: []mcpcontract.ClusterMemberOutput{
					{Kind: "issue", Owner: target.Owner, Repo: target.Repo, Number: 1, Title: "first", Score: 1.0, Reason: "canonical member", Included: true},
					{Kind: "issue", Owner: target.Owner, Repo: target.Repo, Number: 2, Title: "second", Score: 0.9, Reason: "similar title", Included: true},
				},
			}},
		}
		items[i] = mcpcontract.BatchItem[mcpcontract.ClusterSetOutput]{Key: target.Owner + "/" + target.Repo, Status: "complete", Value: &value}
	}
	return mcpcontract.FindClustersOutput{Status: "complete", Items: items}, nil
}

func (*fakeReader) GetCoverage(_ context.Context, in mcpcontract.GetCoverageInput) (mcpcontract.GetCoverageOutput, error) {
	items := make([]mcpcontract.BatchItem[mcpcontract.CoverageOutput], len(in.Targets))
	for i, target := range in.Targets {
		value := mcpcontract.CoverageOutput{Owner: target.Owner, Repo: target.Repo, Kind: target.Kind, Number: target.Number, AsOf: "2026-07-17T00:00:00Z", Facets: []mcpcontract.FacetCoverageOutput{{Facet: "metadata", Complete: true, Status: "fresh", UpdatedAt: "2026-07-17T00:00:00Z"}}}
		items[i] = mcpcontract.BatchItem[mcpcontract.CoverageOutput]{Key: target.Owner + "/" + target.Repo, Status: "complete", Value: &value}
	}
	return mcpcontract.GetCoverageOutput{Status: "complete", Items: items}, nil
}

func (*fakeReader) Lens(_ context.Context, in mcpcontract.LensInput) (mcpcontract.LensOutput, error) {
	if in.Name == "missing" {
		return mcpcontract.LensOutput{}, mcpcontract.ErrNotFound
	}
	return mcpcontract.LensOutput{
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

func (*fakeReader) SearchRepositories(_ context.Context, in mcpcontract.SearchRepositoriesInput) (mcpcontract.SearchRepositoriesOutput, error) {
	return mcpcontract.SearchRepositoriesOutput{Query: in.Query, Total: 1, Matches: []mcpcontract.RepositoryOutput{{Owner: in.Owner, Repo: in.Repo}}}, nil
}

func (f *fakeReader) SearchGitHubRepositories(_ context.Context, in mcpcontract.SearchGitHubRepositoriesInput) (mcpcontract.SearchGitHubRepositoriesOutput, error) {
	f.recordCall("search_github_repositories")
	stars := 42
	applied := in.RawQuery
	if applied == "" {
		applied = in.Text
	}
	return mcpcontract.SearchGitHubRepositoriesOutput{Status: "complete", Query: applied, Interpretation: "Search using structured repository filters.", ResponseFormat: "concise", Page: 1, Total: 1, Items: []mcpcontract.BatchItem[mcpcontract.RepositorySearchMatch]{{Key: "acme/rocket", Status: "complete", Value: &mcpcontract.RepositorySearchMatch{Ref: "repository:acme/rocket", Owner: "acme", Repo: "rocket", Stars: &stars}}}}, nil
}

func (*fakeReader) ExplainMatch(_ context.Context, in mcpcontract.ExplainMatchInput) (mcpcontract.ExplainMatchOutput, error) {
	return mcpcontract.ExplainMatchOutput{Query: in.Query, Owner: in.Owner, Repo: in.Repo, Kind: in.Kind, Number: in.Number, Title: "match"}, nil
}

func (*fakeReader) GetJob(_ context.Context, in mcpcontract.GetJobInput) (mcpcontract.GetJobOutput, error) {
	if in.ID == "missing" {
		return mcpcontract.GetJobOutput{}, mcpcontract.ErrNotFound
	}
	return mcpcontract.GetJobOutput{ID: in.ID, Kind: "crawl", Status: "queued"}, nil
}

func (*fakeReader) ThreadByNumber(_ context.Context, in mcpcontract.ThreadByNumberInput) (mcpcontract.ThreadOutput, error) {
	if in.Number == 404 {
		return mcpcontract.ThreadOutput{}, mcpcontract.ErrNotFound
	}
	return mcpcontract.ThreadOutput{Owner: in.Owner, Repo: in.Repo, Kind: "issue", Number: in.Number, Title: "issue"}, nil
}

func (*fakeReader) BuildRepositoryDossier(_ context.Context, in mcpcontract.BuildRepositoryDossierInput) (mcpcontract.JobReference, error) {
	id := "job-dossier-" + in.Owner + "-" + in.Repo
	return mcpcontract.JobReference{
		ID: id, Ref: "job:" + id, Kind: "build_repository_dossier", Status: "queued", PollAfterMS: 1000,
		FollowUp: &mcpcontract.JobFollowUp{Tool: mcpcontract.ToolGetJob, Reason: "Poll this durable job ID."},
	}, nil
}

func (f *fakeReader) StartInvestigation(_ context.Context, in mcpcontract.StartInvestigationInput) (mcpcontract.InvestigationOutput, error) {
	f.recordCall("start_investigation")
	return mcpcontract.InvestigationOutput{ID: "inv-1", Owner: in.Owner, Repo: in.Repo, Status: "open"}, nil
}

func (*fakeReader) RecordHypothesis(_ context.Context, in mcpcontract.RecordHypothesisInput) (mcpcontract.HypothesisOutput, error) {
	return mcpcontract.HypothesisOutput{ID: "hyp-1", InvestigationID: in.InvestigationID, Title: in.Title, Status: "proposed"}, nil
}

func (*fakeReader) CheckDuplicates(_ context.Context, in mcpcontract.CheckDuplicatesInput) (mcpcontract.CheckOutput, error) {
	return mcpcontract.CheckOutput{Target: in.Target, ID: in.ID, Total: 1, Findings: []mcpcontract.EvidenceItem{{ID: "ev-1", Type: "github_source", Relation: "inconclusive", Description: "similar"}}}, nil
}

func (*fakeReader) CheckCollisions(_ context.Context, in mcpcontract.CheckCollisionsInput) (mcpcontract.CheckOutput, error) {
	return mcpcontract.CheckOutput{Target: in.Target, ID: in.ID, Total: 1, Findings: []mcpcontract.EvidenceItem{{ID: "ev-1", Type: "github_source", Relation: "contradicting", Description: "collision"}}}, nil
}

func (*fakeReader) PromoteOpportunity(_ context.Context, in mcpcontract.PromoteOpportunityInput) (mcpcontract.OpportunityOutput, error) {
	return mcpcontract.OpportunityOutput{ID: "opp-1", HypothesisID: in.HypothesisID, Title: in.ProblemStatement, ProblemStatement: in.ProblemStatement, CollisionStatus: "unknown"}, nil
}

func (*fakeReader) CreateWorkspace(_ context.Context, in mcpcontract.CreateWorkspaceInput) (mcpcontract.JobReference, error) {
	return mcpcontract.JobReference{ID: "job-workspace-" + in.Name, Kind: "create_workspace", Status: "queued"}, nil
}

func (*fakeReader) InspectCommitChanges(_ context.Context, _ mcpcontract.InspectCommitChangesInput) (mcpcontract.CommitInventoryOutput, error) {
	return mcpcontract.CommitInventoryOutput{
		Units:             []mcpcontract.CommitUnitOutput{{ID: "hunk:one", Kind: "hunk", Path: "main.go", Operation: "modify", ContentSHA256: "one"}},
		SourcePatchSHA256: "patch",
		InventorySHA256:   "inventory",
	}, nil
}

func (*fakeReader) PlanSemanticCommits(_ context.Context, _ mcpcontract.PlanSemanticCommitsInput) (mcpcontract.SemanticCommitPlanOutput, error) {
	return mcpcontract.SemanticCommitPlanOutput{Reconstruction: mcpcontract.CommitReconstructionOutput{UnitCount: 1, AssignedCount: 1, Verified: true}}, nil
}

func (*fakeReader) ListConcerns(_ context.Context, _ mcpcontract.ListConcernsInput) (mcpcontract.ConcernListOutput, error) {
	return mcpcontract.ConcernListOutput{Concerns: []mcpcontract.ConcernOutput{{ID: "concern-1", Owner: "owner", Repo: "repo", Title: "flaky", ProblemStatement: "intermittent", Status: "untriaged", Freshness: "unknown"}}, Total: 1}, nil
}

func (f *fakeReader) CreateConcern(_ context.Context, in mcpcontract.CreateConcernInput) (mcpcontract.ConcernOutput, error) {
	f.recordCall("create_concern")
	return mcpcontract.ConcernOutput{ID: "concern-1", Owner: in.Owner, Repo: in.Repo, Title: in.Title, ProblemStatement: in.ProblemStatement, Status: "untriaged", Freshness: "unknown"}, nil
}

func (*fakeReader) UpdateConcern(_ context.Context, in mcpcontract.UpdateConcernInput) (mcpcontract.ConcernOutput, error) {
	var title string
	if in.Title != nil {
		title = *in.Title
	}
	return mcpcontract.ConcernOutput{ID: in.ID, Title: title, Status: "untriaged", Freshness: "unknown"}, nil
}

func (*fakeReader) SetConcernStatus(_ context.Context, in mcpcontract.SetConcernStatusInput) (mcpcontract.ConcernOutput, error) {
	return mcpcontract.ConcernOutput{ID: in.ID, Status: in.Status, Freshness: "unknown"}, nil
}

func (*fakeReader) LinkConcern(_ context.Context, in mcpcontract.LinkConcernInput) (mcpcontract.ConcernOutput, error) {
	return mcpcontract.ConcernOutput{ID: in.ID, Status: "untriaged", Freshness: "unknown", Links: []mcpcontract.ConcernLinkOutput{{Kind: in.Kind, TargetType: in.TargetType, TargetID: in.TargetID}}}, nil
}

func (*fakeReader) PromoteConcern(_ context.Context, in mcpcontract.PromoteConcernInput) (mcpcontract.ConcernOutput, error) {
	return mcpcontract.ConcernOutput{ID: in.ID, Status: "promoted", Freshness: "unknown", Promotion: &mcpcontract.ConcernPromotionOutput{Kind: in.Kind, InvestigationID: "inv-1", HypothesisID: "hyp-1"}}, nil
}

func (*fakeReader) AdoptWorkspace(_ context.Context, in mcpcontract.AdoptWorkspaceInput) (mcpcontract.AdoptWorkspaceOutput, error) {
	return mcpcontract.AdoptWorkspaceOutput{ID: in.Name, InvestigationID: in.InvestigationID, Ownership: "external"}, nil
}

func (f *fakeReader) DefineValidation(_ context.Context, in mcpcontract.DefineValidationInput) (mcpcontract.ValidationOutput, error) {
	f.recordCall("define_validation")
	return mcpcontract.ValidationOutput{ID: "val-1", InvestigationID: in.InvestigationID, Kind: in.Kind, Command: []string{"echo"}}, nil
}

func (*fakeReader) RunValidation(_ context.Context, in mcpcontract.RunValidationInput) (mcpcontract.JobReference, error) {
	return mcpcontract.JobReference{ID: "job-run-" + in.ID, Kind: "run_validation", Status: "queued"}, nil
}

func (f *fakeReader) RunRepeatedValidation(_ context.Context, in mcpcontract.RunRepeatedValidationInput) (mcpcontract.JobReference, error) {
	f.repeatInput = in
	return mcpcontract.JobReference{ID: "job-repeat-" + in.ID, Kind: "run_validation_group", Status: "queued"}, nil
}

func (f *fakeReader) PrepareContribution(_ context.Context, in mcpcontract.PrepareContributionInput) (mcpcontract.DraftOutput, error) {
	f.recordCall("prepare_contribution")
	return mcpcontract.DraftOutput{OpportunityID: in.OpportunityID, Kind: in.Kind, Title: "draft", Body: "body"}, nil
}

func (f *fakeReader) AttachValidationReceipt(_ context.Context, in mcpcontract.AttachValidationReceiptInput) (mcpcontract.ExternalValidationReceiptOutput, error) {
	f.recordCall("attach_validation_receipt")
	return mcpcontract.ExternalValidationReceiptOutput{RunID: "external-run", ReceiptSHA256: "digest"}, nil
}

func (f *fakeReader) VerifyPublishedDraft(_ context.Context, in mcpcontract.VerifyPublishedDraftInput) (mcpcontract.PublishedDraftVerificationOutput, error) {
	f.recordCall("verify_published_draft")
	return mcpcontract.PublishedDraftVerificationOutput{Status: "exact_match", DraftID: in.DraftID, Revision: in.Revision}, nil
}

func (*fakeReader) ExportManifest(_ context.Context, in mcpcontract.ExportManifestInput) (mcpcontract.ManifestOutput, error) {
	return mcpcontract.ManifestOutput{ManifestID: "sha256:test", ContentSHA256: "test", SchemaVersion: "contribution-evidence.v1", Status: "incomplete"}, nil
}

func (*fakeReader) CancelJobs(_ context.Context, in mcpcontract.CancelJobInput) (mcpcontract.GetJobsOutput, error) {
	items := make([]mcpcontract.BatchItem[mcpcontract.GetJobOutput], len(in.IDs))
	for i, id := range in.IDs {
		value := mcpcontract.GetJobOutput{ID: id, Kind: "crawl", Status: "cancelled"}
		items[i] = mcpcontract.BatchItem[mcpcontract.GetJobOutput]{Key: id, Status: "complete", Value: &value}
	}
	return mcpcontract.GetJobsOutput{Status: "complete", Items: items}, nil
}

func connect(t *testing.T, reader mcpcontract.Reader) (*mcp.ClientSession, func()) {
	t.Helper()
	return connectWithOptions(t, reader, mcpcontract.Options{Toolsets: []string{"all"}})
}

func connectWithOptions(t *testing.T, reader mcpcontract.Reader, options mcpcontract.Options) (*mcp.ClientSession, func()) {
	t.Helper()
	if base, ok := reader.(*fakeReader); ok {
		reader = completeFakeReader(base)
	}
	server, err := NewWithOptions(reader, "test", options)
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

func TestReadOnlyToolsReturnStructuredOutput(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	tests := []struct {
		name      string
		args      map[string]any
		wantTotal int
	}{
		{mcpcontract.ToolSearchCode, map[string]any{"query": "main"}, 1},
		{mcpcontract.ToolGetInvestigation, map[string]any{"id": "inv-1"}, -1},
		{mcpcontract.ToolListOpportunities, map[string]any{"investigation_id": "inv-1"}, 1},
		{mcpcontract.ToolGetOpportunity, map[string]any{"id": "opp-1"}, -1},
		{mcpcontract.ToolGetEvidence, map[string]any{"investigation_id": "inv-1"}, 1},
		{mcpcontract.ToolGetReadiness, map[string]any{"opportunity_id": "opp-1"}, -1},
		{mcpcontract.ToolFindClusters, map[string]any{"targets": []any{map[string]any{"owner": "acme", "repo": "rocket"}}}, 1},
		{mcpcontract.ToolGetCoverage, map[string]any{"targets": []any{map[string]any{"owner": "acme", "repo": "rocket"}}}, -1},
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
		case mcpcontract.ToolSearchCode:
			var out mcpcontract.SearchCodeOutput
			if err := json.Unmarshal(payload, &out); err != nil {
				t.Fatalf("decode %s: %v", tt.name, err)
			}
			if out.Total != tt.wantTotal || len(out.Matches) != tt.wantTotal {
				t.Fatalf("%s output = %+v", tt.name, out)
			}
		case mcpcontract.ToolGetInvestigation:
			var out mcpcontract.InvestigationOutput
			if err := json.Unmarshal(payload, &out); err != nil {
				t.Fatalf("decode %s: %v", tt.name, err)
			}
			if out.ID != "inv-1" {
				t.Fatalf("%s output = %+v", tt.name, out)
			}
		case mcpcontract.ToolListOpportunities:
			var out mcpcontract.ListOpportunitiesOutput
			if err := json.Unmarshal(payload, &out); err != nil {
				t.Fatalf("decode %s: %v", tt.name, err)
			}
			if out.Total != tt.wantTotal || len(out.Opportunities) != tt.wantTotal {
				t.Fatalf("%s output = %+v", tt.name, out)
			}
		case mcpcontract.ToolGetOpportunity:
			var out mcpcontract.OpportunityOutput
			if err := json.Unmarshal(payload, &out); err != nil {
				t.Fatalf("decode %s: %v", tt.name, err)
			}
			if out.ID != "opp-1" {
				t.Fatalf("%s output = %+v", tt.name, out)
			}
		case mcpcontract.ToolGetEvidence:
			var out mcpcontract.EvidenceOutput
			if err := json.Unmarshal(payload, &out); err != nil {
				t.Fatalf("decode %s: %v", tt.name, err)
			}
			if out.Total != tt.wantTotal || len(out.Evidence) != tt.wantTotal {
				t.Fatalf("%s output = %+v", tt.name, out)
			}
		case mcpcontract.ToolGetReadiness:
			var out mcpcontract.ReadinessOutput
			if err := json.Unmarshal(payload, &out); err != nil {
				t.Fatalf("decode %s: %v", tt.name, err)
			}
			if out.OpportunityID != "opp-1" || out.Status != "warn" || len(out.Checks) != 1 {
				t.Fatalf("%s output = %+v", tt.name, out)
			}
		case mcpcontract.ToolFindClusters:
			var out mcpcontract.FindClustersOutput
			if err := json.Unmarshal(payload, &out); err != nil {
				t.Fatalf("decode %s: %v", tt.name, err)
			}
			if len(out.Items) != 1 || out.Items[0].Value == nil || out.Items[0].Value.Total != tt.wantTotal || len(out.Items[0].Value.Clusters) != tt.wantTotal {
				t.Fatalf("%s output = %+v", tt.name, out)
			}
		case mcpcontract.ToolGetCoverage:
			var out mcpcontract.GetCoverageOutput
			if err := json.Unmarshal(payload, &out); err != nil {
				t.Fatalf("decode %s: %v", tt.name, err)
			}
			if len(out.Items) != 1 || out.Items[0].Value == nil || out.Items[0].Value.Owner != "acme" || out.Items[0].Value.Repo != "rocket" || len(out.Items[0].Value.Facets) == 0 {
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
		{mcpcontract.ToolGetInvestigation, map[string]any{"id": "inv-1", "hypothesis_limit": 101}},
		{mcpcontract.ToolGetOpportunity, map[string]any{"id": "opp-1", "evidence_limit": 101}},
		{mcpcontract.ToolGetEvidence, map[string]any{"investigation_id": "inv-1", "opportunity_id": "opp-1"}},
		{mcpcontract.ToolGetEvidence, map[string]any{}},
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
		"gitcontribute://fix-pattern-report/job-fix-patterns",
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

func TestFixPatternResourceTemplateTracksReaderCapability(t *testing.T) {
	base := &fakeReader{searchStarted: make(chan struct{})}
	for _, test := range []struct {
		name   string
		reader mcpcontract.Reader
		want   bool
	}{
		{name: "supported", reader: base, want: true},
		{name: "unsupported", reader: struct{ mcpcontract.Reader }{Reader: base}},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, closeSessions := connect(t, test.reader)
			defer closeSessions()
			found := false
			for template, err := range client.ResourceTemplates(context.Background(), nil) {
				if err != nil {
					t.Fatal(err)
				}
				if template.URITemplate == "gitcontribute://fix-pattern-report/{job_id}" {
					found = true
				}
			}
			if found != test.want {
				t.Fatalf("fix-pattern resource template advertised = %t, want %t", found, test.want)
			}
		})
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

	investigate, err := client.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "investigate_contribution_candidate",
		Arguments: map[string]string{"owner": "acme", "repo": "rocket", "number": "17"},
	})
	if err != nil {
		t.Fatalf("get investigate prompt: %v", err)
	}
	investigateText, ok := investigate.Messages[0].Content.(*mcp.TextContent)
	if !ok {
		t.Fatalf("investigate prompt content = %#v", investigate.Messages[0].Content)
	}
	if strings.Contains(investigateText.Text, "/issue/17") ||
		!strings.Contains(investigateText.Text, "gitcontribute://threads/acme/rocket/17") ||
		!strings.Contains(investigateText.Text, "returned kind") {
		t.Fatalf("investigate prompt hardcodes or fails to resolve thread kind:\n%s", investigateText.Text)
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
			Name: mcpcontract.ToolSearchThreads, Arguments: map[string]any{"query": "block"},
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
		mcpcontract.ToolSearchRepositories, mcpcontract.ToolSearchThreads, mcpcontract.ToolGetRepositoryDossier, mcpcontract.ToolExplainMatch, mcpcontract.ToolGetJob,
		mcpcontract.ToolGetReadiness, mcpcontract.ToolBuildRepositoryDossier,
		mcpcontract.ToolCreateWorkspace, mcpcontract.ToolAdoptWorkspace, mcpcontract.ToolRunValidation, mcpcontract.ToolRunRepeatedValidation,
		mcpcontract.ToolStartInvestigation, mcpcontract.ToolRecordHypothesis,
		mcpcontract.ToolCheckDuplicates, mcpcontract.ToolFindCompetingWork, mcpcontract.ToolPromoteOpportunity, mcpcontract.ToolDefineValidation,
		mcpcontract.ToolPrepareContribution, mcpcontract.ToolCancelJob,
	} {
		if tools[name] == nil {
			t.Fatalf("missing v1 tool %q", name)
		}
	}

	readTests := []struct {
		name string
		args map[string]any
	}{
		{mcpcontract.ToolSearchRepositories, map[string]any{"query": "rocket"}},
		{mcpcontract.ToolSearchThreads, map[string]any{"query": "stall"}},
		{mcpcontract.ToolGetRepositoryDossier, map[string]any{"owner": "acme", "repo": "rocket"}},
		{mcpcontract.ToolExplainMatch, map[string]any{"owner": "acme", "repo": "rocket", "kind": "issue", "number": 7}},
		{mcpcontract.ToolGetJob, map[string]any{"ids": []string{"job-1"}}},
		{mcpcontract.ToolGetReadiness, map[string]any{"opportunity_id": "opp-1"}},
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
		{mcpcontract.ToolBuildRepositoryDossier, map[string]any{"owner": "acme", "repo": "rocket"}},
		{mcpcontract.ToolCreateWorkspace, map[string]any{"investigation_id": "inv-1"}},
		{mcpcontract.ToolAdoptWorkspace, map[string]any{"investigation_id": "inv-1", "path": "/tmp/worktree", "base_ref": "main", "name": "external"}},
		{mcpcontract.ToolRunValidation, map[string]any{"id": "val-1", "kind": "base", "execute": true}},
		{mcpcontract.ToolRunRepeatedValidation, map[string]any{"id": "val-1", "target": "both", "execute": true}},
		{mcpcontract.ToolStartInvestigation, map[string]any{"owner": "acme", "repo": "rocket", "commit_sha": "abc123"}},
		{mcpcontract.ToolRecordHypothesis, map[string]any{"investigation_id": "inv-1", "title": "leak", "description": "memory leak", "category": "bug"}},
		{mcpcontract.ToolCheckDuplicates, map[string]any{"target": "hypothesis", "id": "hyp-1"}},
		{mcpcontract.ToolFindCompetingWork, map[string]any{"target": "opportunity", "id": "opp-1"}},
		{mcpcontract.ToolPromoteOpportunity, map[string]any{"hypothesis_id": "hyp-1", "problem_statement": "leak", "scope": "small", "impact": "high", "expected_effort": "1h", "confidence": 0.8}},
		{mcpcontract.ToolDefineValidation, map[string]any{"investigation_id": "inv-1", "kind": "test", "command": "go test ./...", "workspace_id": "ws-1"}},
		{mcpcontract.ToolPrepareContribution, map[string]any{"opportunity_id": "opp-1", "kind": "issue"}},
		{mcpcontract.ToolCancelJob, map[string]any{"ids": []string{"job-1"}}},
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

	_, err := client.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "github-index://repositories/acme/rocket"})
	if err == nil {
		t.Fatal("legacy github-index resource should not be routed")
	}
	for _, uri := range []string{
		"gitcontribute://repositories/acme/rocket",
		"gitcontribute://dossiers/acme/rocket",
		"gitcontribute://investigations/inv-1",
		"gitcontribute://workflows/contribution/opp-1",
		"gitcontribute://lenses/default",
		"gitcontribute://job/job-1",
		"gitcontribute://jobs/job-1",
	} {
		if _, err := client.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: uri}); err == nil {
			t.Errorf("unadvertised alias %q was routed", uri)
		}
	}
}
