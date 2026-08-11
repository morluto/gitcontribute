package mcpserver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

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
