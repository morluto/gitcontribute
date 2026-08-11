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
	searchStarted   chan struct{}
	validationInput mcpcontract.RunValidationInput
	radarScore      int
	calls           map[string]int
}

type canonicalIDReader struct {
	*fakeReader
	validation    mcpcontract.DefineValidationInput
	workspace     mcpcontract.CreateWorkspaceInput
	verification  mcpcontract.VerifyPublishedDraftInput
	hypothesis    mcpcontract.RecordHypothesisInput
	relatedWork   mcpcontract.CheckDuplicatesInput
	promotion     mcpcontract.PromoteOpportunityInput
	contribution  mcpcontract.PrepareContributionInput
	manifest      mcpcontract.ExportManifestInput
	junit         mcpcontract.AttachJUnitReportInput
	explanation   mcpcontract.ExplainMatchInput
	dossier       mcpcontract.BuildRepositoryDossierInput
	investigation mcpcontract.StartInvestigationInput
	concern       mcpcontract.CreateConcernInput
	commitInspect mcpcontract.InspectCommitChangesInput
	commitPlan    mcpcontract.PlanSemanticCommitsInput
}

func (r *canonicalIDReader) DefineValidation(_ context.Context, in mcpcontract.DefineValidationInput) (mcpcontract.ValidationOutput, error) {
	r.validation = in
	return mcpcontract.ValidationOutput{ID: "val-1", InvestigationID: in.InvestigationID}, nil
}

func (r *canonicalIDReader) CreateWorkspace(_ context.Context, in mcpcontract.CreateWorkspaceInput) (mcpcontract.JobReference, error) {
	r.workspace = in
	return mcpcontract.JobReference{ID: "job-workspace", Status: "queued"}, nil
}

func (r *canonicalIDReader) VerifyPublishedDraft(_ context.Context, in mcpcontract.VerifyPublishedDraftInput) (mcpcontract.PublishedDraftVerificationOutput, error) {
	r.verification = in
	return mcpcontract.PublishedDraftVerificationOutput{Status: "exact_match", DraftID: in.DraftID, Revision: in.Revision}, nil
}

func (r *canonicalIDReader) RecordHypothesis(_ context.Context, in mcpcontract.RecordHypothesisInput) (mcpcontract.HypothesisOutput, error) {
	r.hypothesis = in
	return mcpcontract.HypothesisOutput{ID: "hyp-1", InvestigationID: in.InvestigationID}, nil
}

func (r *canonicalIDReader) CheckDuplicates(_ context.Context, in mcpcontract.CheckDuplicatesInput) (mcpcontract.CheckOutput, error) {
	r.relatedWork = in
	return mcpcontract.CheckOutput{Target: in.Target, ID: in.ID}, nil
}

func (r *canonicalIDReader) PromoteOpportunity(_ context.Context, in mcpcontract.PromoteOpportunityInput) (mcpcontract.OpportunityOutput, error) {
	r.promotion = in
	return mcpcontract.OpportunityOutput{ID: "opp-1", HypothesisID: in.HypothesisID}, nil
}

func (r *canonicalIDReader) PrepareContribution(_ context.Context, in mcpcontract.PrepareContributionInput) (mcpcontract.DraftOutput, error) {
	r.contribution = in
	return mcpcontract.DraftOutput{ID: "draft-1", Revision: 1, OpportunityID: in.OpportunityID}, nil
}

func (r *canonicalIDReader) ExportManifest(_ context.Context, in mcpcontract.ExportManifestInput) (mcpcontract.ManifestOutput, error) {
	r.manifest = in
	return mcpcontract.ManifestOutput{ManifestID: "sha256:test"}, nil
}

func (r *canonicalIDReader) AttachJUnitReport(_ context.Context, in mcpcontract.AttachJUnitReportInput) (mcpcontract.AttachJUnitReportOutput, error) {
	r.junit = in
	return mcpcontract.AttachJUnitReportOutput{RunID: in.RunID}, nil
}

func (r *canonicalIDReader) ExplainMatch(_ context.Context, in mcpcontract.ExplainMatchInput) (mcpcontract.ExplainMatchOutput, error) {
	r.explanation = in
	return mcpcontract.ExplainMatchOutput{Owner: in.Owner, Repo: in.Repo, Kind: in.Kind}, nil
}

func (r *canonicalIDReader) BuildRepositoryDossier(_ context.Context, in mcpcontract.BuildRepositoryDossierInput) (mcpcontract.JobReference, error) {
	r.dossier = in
	return mcpcontract.JobReference{ID: "job-dossier", Status: "queued"}, nil
}

func (r *canonicalIDReader) StartInvestigation(_ context.Context, in mcpcontract.StartInvestigationInput) (mcpcontract.InvestigationOutput, error) {
	r.investigation = in
	return mcpcontract.InvestigationOutput{ID: "inv-1", Owner: in.Owner, Repo: in.Repo}, nil
}

func (r *canonicalIDReader) CreateConcern(_ context.Context, in mcpcontract.CreateConcernInput) (mcpcontract.ConcernOutput, error) {
	r.concern = in
	return mcpcontract.ConcernOutput{ID: "concern-1", Owner: in.Owner, Repo: in.Repo}, nil
}

func (r *canonicalIDReader) InspectCommitChanges(_ context.Context, in mcpcontract.InspectCommitChangesInput) (mcpcontract.CommitInventoryOutput, error) {
	r.commitInspect = in
	return mcpcontract.CommitInventoryOutput{}, nil
}

func (r *canonicalIDReader) PlanSemanticCommits(_ context.Context, in mcpcontract.PlanSemanticCommitsInput) (mcpcontract.SemanticCommitPlanOutput, error) {
	r.commitPlan = in
	return mcpcontract.SemanticCommitPlanOutput{}, nil
}

type canonicalRepositoryReader struct {
	*fakeReader
	threadSearch   mcpcontract.SearchGitHubThreadsInput
	sourceFiles    mcpcontract.ReadSourceFilesInput
	portfolio      mcpcontract.SyncPortfolioInput
	overlaps       mcpcontract.FindPortfolioOverlapsInput
	checkWait      mcpcontract.WaitPullRequestChecksInput
	feedbackIndex  mcpcontract.IndexPullRequestFeedbackInput
	feedbackSearch mcpcontract.SearchPullRequestFeedbackInput
}

func (r *canonicalRepositoryReader) SearchGitHubThreads(_ context.Context, in mcpcontract.SearchGitHubThreadsInput) (mcpcontract.SearchGitHubThreadsOutput, error) {
	r.threadSearch = in
	return mcpcontract.SearchGitHubThreadsOutput{}, nil
}

func (r *canonicalRepositoryReader) ReadSourceFiles(_ context.Context, in mcpcontract.ReadSourceFilesInput) (mcpcontract.ReadSourceFilesOutput, error) {
	r.sourceFiles = in
	return mcpcontract.ReadSourceFilesOutput{}, nil
}

func (*canonicalRepositoryReader) SyncRepositoryContext(context.Context, mcpcontract.SyncRepositoryContextInput) (mcpcontract.JobReference, error) {
	return mcpcontract.JobReference{ID: "job-context", Status: "queued"}, nil
}

func (*canonicalRepositoryReader) SyncThreads(context.Context, mcpcontract.SyncThreadsInput) (mcpcontract.JobReference, error) {
	return mcpcontract.JobReference{ID: "job-threads", Status: "queued"}, nil
}

func (*canonicalRepositoryReader) HydrateThreads(context.Context, mcpcontract.HydrateThreadsInput) (mcpcontract.JobReference, error) {
	return mcpcontract.JobReference{ID: "job-hydrate", Status: "queued"}, nil
}

func (r *canonicalRepositoryReader) SyncPortfolio(_ context.Context, in mcpcontract.SyncPortfolioInput) (mcpcontract.JobReference, error) {
	r.portfolio = in
	return mcpcontract.JobReference{ID: "job-portfolio", Status: "queued"}, nil
}

func (r *canonicalRepositoryReader) FindPortfolioOverlaps(_ context.Context, in mcpcontract.FindPortfolioOverlapsInput) (mcpcontract.FindPortfolioOverlapsOutput, error) {
	r.overlaps = in
	return mcpcontract.FindPortfolioOverlapsOutput{}, nil
}

func (*canonicalRepositoryReader) ListPullRequestPortfolio(context.Context, mcpcontract.ListPullRequestPortfolioInput) (mcpcontract.ListPullRequestPortfolioOutput, error) {
	return mcpcontract.ListPullRequestPortfolioOutput{}, nil
}

func (r *canonicalRepositoryReader) WaitPullRequestChecks(_ context.Context, in mcpcontract.WaitPullRequestChecksInput) (mcpcontract.JobReference, error) {
	r.checkWait = in
	return mcpcontract.JobReference{ID: "job-checks", Status: "queued"}, nil
}

func (r *canonicalRepositoryReader) IndexPullRequestFeedback(_ context.Context, in mcpcontract.IndexPullRequestFeedbackInput) (mcpcontract.JobReference, error) {
	r.feedbackIndex = in
	return mcpcontract.JobReference{ID: "job-feedback", Status: "queued"}, nil
}

func (r *canonicalRepositoryReader) SearchPullRequestFeedback(_ context.Context, in mcpcontract.SearchPullRequestFeedbackInput) (mcpcontract.SearchPullRequestFeedbackOutput, error) {
	r.feedbackSearch = in
	return mcpcontract.SearchPullRequestFeedbackOutput{}, nil
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

func (*fakeReader) PullRequestFeedbackResource(context.Context, string, string, int) (mcpcontract.PullRequestFeedbackResource, error) {
	return mcpcontract.PullRequestFeedbackResource{SchemaVersion: "gitcontribute.pull-request-feedback.v1"}, nil
}

func (*fakeReader) PullRequestFeedbackItemResource(context.Context, string, string, int, string, string) (mcpcontract.PullRequestFeedbackItemResource, error) {
	return mcpcontract.PullRequestFeedbackItemResource{SchemaVersion: "gitcontribute.pull-request-feedback-item.v1"}, nil
}

func (*fakeReader) CIFailureResource(context.Context, string, string, int) (mcpcontract.CIFailureResource, error) {
	return mcpcontract.NewCIFailureResource(json.RawMessage(`{}`), "acme", "project", 7, nil)
}

func (*fakeReader) CIJobLogResource(context.Context, string, string, int, int64) (mcpcontract.CIJobLogResource, error) {
	return mcpcontract.NewCIJobLogResource(31, "failure", false)
}

func (*fakeReader) IndexPullRequestFeedback(context.Context, mcpcontract.IndexPullRequestFeedbackInput) (mcpcontract.JobReference, error) {
	return mcpcontract.JobReference{ID: "job-feedback-index", Status: "queued"}, nil
}

func (*fakeReader) SearchPullRequestFeedback(context.Context, mcpcontract.SearchPullRequestFeedbackInput) (mcpcontract.SearchPullRequestFeedbackOutput, error) {
	return mcpcontract.SearchPullRequestFeedbackOutput{Status: "complete"}, nil
}

func (*fakeReader) GetThreadFacets(_ context.Context, _ mcpcontract.GetThreadFacetsInput) (mcpcontract.GetThreadFacetsOutput, error) {
	return mcpcontract.GetThreadFacetsOutput{Status: "complete"}, nil
}

func (*fakeReader) ThreadFacetResource(context.Context, string, string, string, int, string) (mcpcontract.ThreadFacetResource, error) {
	return mcpcontract.ThreadFacetResource{SchemaVersion: "gitcontribute.thread-facet.v1"}, nil
}

func TestPullRequestWorkflowResourcesAreReadable(t *testing.T) {
	server := &Server{reader: &fakeReader{}}
	tests := []struct {
		uri, version string
	}{
		{"gitcontribute://thread/acme/project/pull_request/7/facet/pr_details", "gitcontribute.thread-facet.v1"},
		{"gitcontribute://pull-request-feedback/acme/project/7", "gitcontribute.pull-request-feedback.v1"},
		{"gitcontribute://pull-request-feedback/acme/project/7/inline_comments/123", "gitcontribute.pull-request-feedback-item.v1"},
		{"gitcontribute://ci-failure-report/acme/project/7", "gitcontribute.ci-failure-report.v1"},
		{"gitcontribute://ci-job-log/acme/project/7/31", "gitcontribute.ci-job-log.v1"},
	}
	for _, test := range tests {
		u := strings.TrimPrefix(test.uri, "gitcontribute://")
		host, path, _ := strings.Cut(u, "/")
		value, err := server.readResourceValue(context.Background(), resourceRequest{
			uri: test.uri, scheme: "gitcontribute", host: host, parts: strings.Split(path, "/"),
		})
		if err != nil {
			t.Fatalf("%s: %v", test.uri, err)
		}
		payload, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var document struct {
			SchemaVersion string `json:"schema_version"`
		}
		if err := json.Unmarshal(payload, &document); err != nil {
			t.Fatal(err)
		}
		if document.SchemaVersion != test.version {
			t.Fatalf("%s schema_version=%v", test.uri, document.SchemaVersion)
		}
	}
}

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
		value := mcpcontract.CoverageOutput{Owner: target.Repository.Owner, Repo: target.Repository.Repo, AsOf: "2026-07-17T00:00:00Z", Facets: []mcpcontract.FacetCoverageOutput{{Facet: "metadata", Complete: true, Status: "fresh", UpdatedAt: "2026-07-17T00:00:00Z"}}}
		if target.Thread != nil {
			value.Kind, value.Number = target.Thread.Kind, target.Thread.Number
		}
		items[i] = mcpcontract.BatchItem[mcpcontract.CoverageOutput]{Key: target.Repository.Owner + "/" + target.Repository.Repo, Status: "complete", Value: &value}
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
		FollowUp: &mcpcontract.JobFollowUp{Action: mcpcontract.FollowUpActionFor(mcpcontract.GetJobsInput{IDs: []string{"job-1"}}), Reason: "Poll this durable job ID."},
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
	return mcpcontract.ConcernListOutput{Concerns: []mcpcontract.ConcernSummaryOutput{{
		ID: "concern-1", Owner: "owner", Repo: "repo", Title: "flaky", Status: "untriaged",
		Freshness: "unknown", URI: "gitcontribute://concern/concern-1",
	}}, Total: 1}, nil
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

func (f *fakeReader) RunValidation(_ context.Context, in mcpcontract.RunValidationInput) (mcpcontract.JobReference, error) {
	f.validationInput = in
	return mcpcontract.JobReference{ID: "job-repeat-" + in.ID, Kind: "run_validation_group", Status: "queued"}, nil
}

func (f *fakeReader) PrepareContribution(_ context.Context, in mcpcontract.PrepareContributionInput) (mcpcontract.DraftOutput, error) {
	f.recordCall("prepare_contribution")
	return mcpcontract.DraftOutput{ID: "draft-1", Revision: 1, OpportunityID: in.OpportunityID, Kind: in.Kind, Title: "draft", Body: "body"}, nil
}

func (*fakeReader) Draft(_ context.Context, in mcpcontract.DraftInput) (mcpcontract.DraftOutput, error) {
	return mcpcontract.DraftOutput{ID: in.ID, Revision: in.Revision, OpportunityID: "opp-1", Kind: "issue", Title: "draft", Body: "body"}, nil
}

func (f *fakeReader) AttachValidationReceipt(_ context.Context, _ mcpcontract.AttachValidationReceiptInput) (mcpcontract.ExternalValidationReceiptOutput, error) {
	f.recordCall("attach_validation_receipt")
	return mcpcontract.ExternalValidationReceiptOutput{RunID: "external-run", ReceiptSHA256: "digest"}, nil
}

func (f *fakeReader) VerifyPublishedDraft(_ context.Context, in mcpcontract.VerifyPublishedDraftInput) (mcpcontract.PublishedDraftVerificationOutput, error) {
	f.recordCall("verify_published_draft")
	return mcpcontract.PublishedDraftVerificationOutput{Status: "exact_match", DraftID: in.DraftID, Revision: in.Revision}, nil
}

func (*fakeReader) ExportManifest(_ context.Context, _ mcpcontract.ExportManifestInput) (mcpcontract.ManifestOutput, error) {
	return mcpcontract.ManifestOutput{ManifestID: "sha256:test", ContentSHA256: "test", SchemaVersion: "contribution-evidence.v1", Status: "incomplete"}, nil
}

func (*fakeReader) Manifest(_ context.Context, in mcpcontract.ManifestInput) (mcpcontract.ManifestOutput, error) {
	return mcpcontract.ManifestOutput{ManifestID: in.ID, ContentSHA256: "test", SchemaVersion: "contribution-evidence.v1", Status: "incomplete"}, nil
}

func (*fakeReader) Concern(_ context.Context, in mcpcontract.ConcernInput) (mcpcontract.ConcernOutput, error) {
	return mcpcontract.ConcernOutput{ID: in.ID, Title: "concern", Status: "untriaged", Freshness: "unknown"}, nil
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
	return connectServer(t, reader, false)
}

func connectServer(t *testing.T, reader mcpcontract.Reader, readOnly bool) (*mcp.ClientSession, func()) {
	t.Helper()
	if base, ok := reader.(*fakeReader); ok {
		reader = completeFakeReader(base)
	}
	newServer := New
	if readOnly {
		newServer = NewReadOnly
	}
	server, err := newServer(reader, "test")
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
		{mcpcontract.ToolFindClusters, map[string]any{"targets": []any{map[string]any{"owner": "acme", "repo": "rocket"}}}, 1},
		{mcpcontract.ToolGetCoverage, map[string]any{"targets": []any{map[string]any{"type": "repository", "repository": map[string]any{"owner": "acme", "repo": "rocket"}}}}, -1},
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

func TestInvestigationOpportunityEvidenceResources(t *testing.T) {
	client, closeSessions := connect(t, &fakeReader{searchStarted: make(chan struct{})})
	defer closeSessions()

	cases := []string{
		"gitcontribute://investigation/inv-1",
		"gitcontribute://opportunity/opp-1",
		"gitcontribute://evidence/investigation/inv-1",
		"gitcontribute://evidence/opportunity/opp-1",
		"gitcontribute://readiness/opp-1",
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
		{name: "supported", reader: base, want: false},
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
		!strings.Contains(investigateText.Text, "corpus.get_threads") ||
		!strings.Contains(investigateText.Text, "acme/rocket#17") {
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

func TestWriteBoundariesPassCanonicalIDsToOperators(t *testing.T) {
	reader := &canonicalIDReader{fakeReader: &fakeReader{}}
	server := &Server{reader: reader}
	ctx := context.Background()

	if _, _, err := server.defineValidation(ctx, nil, mcpcontract.DefineValidationInput{InvestigationID: " inv-1 ", Kind: "test", Command: "go test ./..."}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.createWorkspace(ctx, nil, mcpcontract.CreateWorkspaceInput{InvestigationID: " inv-1 "}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.verifyPublishedDraft(ctx, nil, mcpcontract.VerifyPublishedDraftInput{DraftID: " draft-1 ", Revision: 1, Kind: "issue", Number: 7}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.recordHypothesis(ctx, nil, mcpcontract.RecordHypothesisInput{InvestigationID: " inv-1 ", Title: "title", Description: "description", Category: "bug"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.findRelatedWork(ctx, nil, mcpcontract.FindRelatedWorkInput{Target: " HYPOTHESIS ", ID: " hyp-1 ", Kinds: []string{"duplicates"}, Limit: 1}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.promoteOpportunity(ctx, nil, mcpcontract.PromoteOpportunityInput{HypothesisID: " hyp-1 ", ProblemStatement: " problem ", Scope: " scope ", Impact: " impact ", ExpectedEffort: " small "}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.prepareContribution(ctx, nil, mcpcontract.PrepareContributionInput{OpportunityID: " opp-1 ", Kind: "issue"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.prepareContribution(ctx, nil, mcpcontract.PrepareContributionInput{OpportunityID: " opp-2 ", Kind: "pull_request", WorkspaceID: " ws-1 ", Approach: " approach "}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.exportManifest(ctx, nil, mcpcontract.ExportManifestInput{OpportunityID: " opp-1 ", PullRequest: &mcpcontract.ManifestPullRequestInput{Owner: " acme ", Repo: " rocket ", Number: 7}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.attachJUnitReport(ctx, nil, mcpcontract.AttachJUnitReportInput{RunID: " run-1 ", ReportXML: "<testsuite/>"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.explainMatch(ctx, nil, mcpcontract.ExplainMatchInput{Owner: " acme ", Repo: " rocket ", Kind: " code ", Path: " main.go ", Commit: " abc123 "}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.buildRepositoryDossier(ctx, nil, mcpcontract.BuildRepositoryDossierInput{Owner: " acme ", Repo: " rocket "}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.startInvestigation(ctx, nil, mcpcontract.StartInvestigationInput{Owner: " acme ", Repo: " rocket ", CommitSHA: " abc123 ", Lens: " reliability "}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.createConcern(ctx, nil, mcpcontract.CreateConcernInput{Owner: " acme ", Repo: " rocket ", CommitSHA: " abc123 ", Title: "title", ProblemStatement: "problem"}); err != nil {
		t.Fatal(err)
	}

	if reader.validation.InvestigationID != "inv-1" || reader.workspace.InvestigationID != "inv-1" || reader.hypothesis.InvestigationID != "inv-1" {
		t.Fatalf("investigation IDs were not canonical: validation=%q workspace=%q hypothesis=%q", reader.validation.InvestigationID, reader.workspace.InvestigationID, reader.hypothesis.InvestigationID)
	}
	if reader.verification.DraftID != "draft-1" || reader.relatedWork.ID != "hyp-1" || reader.relatedWork.Target != "hypothesis" || reader.promotion.HypothesisID != "hyp-1" {
		t.Fatalf("workflow IDs were not canonical: verification=%q related=%q/%q promotion=%q", reader.verification.DraftID, reader.relatedWork.Target, reader.relatedWork.ID, reader.promotion.HypothesisID)
	}
	if reader.promotion.ProblemStatement != "problem" || reader.promotion.Scope != "scope" || reader.promotion.Impact != "impact" || reader.promotion.ExpectedEffort != "small" {
		t.Fatalf("opportunity fields were not canonical: %+v", reader.promotion)
	}
	if reader.contribution.OpportunityID != "opp-2" || reader.contribution.WorkspaceID != "ws-1" || reader.contribution.Approach != "approach" || reader.manifest.OpportunityID != "opp-1" {
		t.Fatalf("opportunity IDs were not canonical: contribution=%q manifest=%q", reader.contribution.OpportunityID, reader.manifest.OpportunityID)
	}
	if reader.manifest.PullRequest == nil || reader.manifest.PullRequest.Owner != "acme" || reader.manifest.PullRequest.Repo != "rocket" || reader.junit.RunID != "run-1" {
		t.Fatalf("manifest and validation identities were not canonical: manifest=%+v junit=%+v", reader.manifest.PullRequest, reader.junit)
	}
	if reader.explanation.Owner != "acme" || reader.explanation.Repo != "rocket" || reader.explanation.Path != "main.go" || reader.explanation.Commit != "abc123" {
		t.Fatalf("explanation identity was not canonical: %+v", reader.explanation)
	}
	if reader.dossier.Owner != "acme" || reader.dossier.Repo != "rocket" || reader.investigation.Owner != "acme" || reader.investigation.Repo != "rocket" || reader.investigation.CommitSHA != "abc123" || reader.investigation.Lens != "reliability" {
		t.Fatalf("repository workflow identities were not canonical: dossier=%+v investigation=%+v", reader.dossier, reader.investigation)
	}
	if reader.concern.Owner != "acme" || reader.concern.Repo != "rocket" || reader.concern.CommitSHA != "abc123" {
		t.Fatalf("concern identity was not canonical: %+v", reader.concern)
	}
}

func TestCommitPlanningBoundariesPassCanonicalInventoryIdentity(t *testing.T) {
	reader := &canonicalIDReader{fakeReader: &fakeReader{}}
	server := &Server{reader: reader}
	ctx := context.Background()

	if _, _, err := server.inspectCommitChanges(ctx, nil, mcpcontract.InspectCommitChangesInput{WorkspaceID: " ws-1 "}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.planSemanticCommits(ctx, nil, mcpcontract.PlanSemanticCommitsInput{WorkspaceID: " ws-1 ", ExpectedInventorySHA256: " inventory-sha "}); err != nil {
		t.Fatal(err)
	}
	if reader.commitInspect.WorkspaceID != "ws-1" || reader.commitPlan.WorkspaceID != "ws-1" || reader.commitPlan.ExpectedInventorySHA256 != "inventory-sha" {
		t.Fatalf("commit inventory identity was not canonical: inspect=%+v plan=%+v", reader.commitInspect, reader.commitPlan)
	}
}

func TestLiveRepositoryBoundariesPassCanonicalReferencesToOperators(t *testing.T) {
	reader := &canonicalRepositoryReader{fakeReader: &fakeReader{}}
	server := &Server{reader: reader}
	ctx := context.Background()

	if _, _, err := server.searchGitHubThreads(ctx, nil, mcpcontract.SearchGitHubThreadsInput{Repository: mcpcontract.RepositoryRef{Owner: " acme ", Repo: " rocket "}, Query: " regression "}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.readSourceFiles(ctx, nil, mcpcontract.ReadSourceFilesInput{Repository: mcpcontract.RepositoryRef{Owner: " acme ", Repo: " rocket "}, Ref: " main ", Files: []mcpcontract.SourceFileRequest{{Path: "README.md"}}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.syncPortfolio(ctx, nil, mcpcontract.SyncPortfolioInput{Selection: "authored", Repository: &mcpcontract.RepositoryRef{Owner: " acme ", Repo: " rocket "}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.findPortfolioOverlaps(ctx, nil, mcpcontract.FindPortfolioOverlapsInput{
		Candidates:   []mcpcontract.PortfolioSubjectInput{{Kind: " opportunity ", Ref: " opp-1 "}},
		PullRequests: []mcpcontract.ThreadRef{{Owner: " acme ", Repo: " rocket ", Number: 7}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.waitPullRequestChecks(ctx, nil, mcpcontract.WaitPullRequestChecksInput{Owner: " acme ", Repo: " rocket ", Number: 7, ExpectedHeadSHA: " abc123 "}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.indexPullRequestFeedback(ctx, nil, mcpcontract.IndexPullRequestFeedbackInput{Repository: mcpcontract.RepositoryRef{Owner: " acme ", Repo: " rocket "}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.searchPullRequestFeedback(ctx, nil, mcpcontract.SearchPullRequestFeedbackInput{Repository: mcpcontract.RepositoryRef{Owner: " acme ", Repo: " rocket "}}); err != nil {
		t.Fatal(err)
	}

	if reader.threadSearch.Repository.Owner != "acme" || reader.threadSearch.Repository.Repo != "rocket" || reader.threadSearch.Query != "regression" {
		t.Fatalf("thread-search boundary = %+v", reader.threadSearch)
	}
	if reader.sourceFiles.Repository.Owner != "acme" || reader.sourceFiles.Repository.Repo != "rocket" || reader.sourceFiles.Ref != "main" {
		t.Fatalf("source-file boundary = %+v", reader.sourceFiles)
	}
	if reader.portfolio.Repository == nil || reader.portfolio.Repository.Owner != "acme" || reader.portfolio.Repository.Repo != "rocket" {
		t.Fatalf("portfolio boundary = %+v", reader.portfolio)
	}
	if len(reader.overlaps.Candidates) != 1 || reader.overlaps.Candidates[0].Kind != "opportunity" || reader.overlaps.Candidates[0].Ref != "opp-1" || reader.overlaps.PullRequests[0].Owner != "acme" || reader.overlaps.PullRequests[0].Repo != "rocket" {
		t.Fatalf("portfolio overlap boundary = %+v", reader.overlaps)
	}
	if reader.checkWait.Owner != "acme" || reader.checkWait.Repo != "rocket" || reader.checkWait.ExpectedHeadSHA != "abc123" {
		t.Fatalf("pull-request check boundary = %+v", reader.checkWait)
	}
	if reader.feedbackIndex.Repository.Owner != "acme" || reader.feedbackIndex.Repository.Repo != "rocket" || reader.feedbackSearch.Repository.Owner != "acme" || reader.feedbackSearch.Repository.Repo != "rocket" {
		t.Fatalf("pull-request feedback boundaries: index=%+v search=%+v", reader.feedbackIndex, reader.feedbackSearch)
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
		mcpcontract.ToolSearchRepositories, mcpcontract.ToolSearchThreads, mcpcontract.ToolExplainMatch, mcpcontract.ToolGetJob,
		mcpcontract.ToolCreateWorkspace, mcpcontract.ToolAdoptWorkspace, mcpcontract.ToolRunValidation,
		mcpcontract.ToolStartInvestigation, mcpcontract.ToolRecordHypothesis,
		mcpcontract.ToolPromoteOpportunity, mcpcontract.ToolDefineValidation,
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
		{mcpcontract.ToolExplainMatch, map[string]any{"owner": "acme", "repo": "rocket", "kind": "issue", "number": 7}},
		{mcpcontract.ToolGetJob, map[string]any{"ids": []string{"job-1"}}},
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
		{mcpcontract.ToolCreateWorkspace, map[string]any{"investigation_id": "inv-1"}},
		{mcpcontract.ToolAdoptWorkspace, map[string]any{"investigation_id": "inv-1", "path": "/tmp/worktree", "base_ref": "main", "name": "external"}},
		{mcpcontract.ToolRunValidation, map[string]any{"id": "val-1", "target": "both", "run_count": 3, "execute": true}},
		{mcpcontract.ToolStartInvestigation, map[string]any{"owner": "acme", "repo": "rocket", "commit_sha": "abc123"}},
		{mcpcontract.ToolRecordHypothesis, map[string]any{"investigation_id": "inv-1", "title": "leak", "description": "memory leak", "category": "bug"}},
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

	templates := map[string]bool{}
	for template, err := range client.ResourceTemplates(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		templates[template.URITemplate] = true
	}
	for _, uriTemplate := range []string{
		"gitcontribute://thread/{owner}/{repo}/{kind}/{number}/facet/{facet}",
		"gitcontribute://concern/{id}",
		"gitcontribute://draft/{id}/{revision}",
		"gitcontribute://manifest/{id}",
	} {
		if !templates[uriTemplate] {
			t.Errorf("missing resource template %q", uriTemplate)
		}
	}
}
