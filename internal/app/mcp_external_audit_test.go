package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
	"github.com/morluto/gitcontribute/internal/mcpserver"
)

func connectAuditMCP(t *testing.T, svc *Service) *mcp.ClientSession {
	t.Helper()
	server, err := mcpserver.New(svc.MCPReader(), "audit-test")
	if err != nil {
		t.Fatal(err)
	}
	a, b := mcp.NewInMemoryTransports()
	ss, err := server.MCP().Connect(context.Background(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "audit-test", Version: "test"}, nil)
	cs, err := client.Connect(context.Background(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func TestMCPIndexJobDetailedBatchReturnsUsableArtifactReference(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	job, err := svc.corpus.CreateJob(ctx, "index_repositories", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.StartJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	result := `{"status":"complete","items":[{"key":"acme/rocket","status":"complete","commit_sha":"commit-a","artifact_digest":"` + strings.Repeat("a", 64) + `","manifest_digest":"` + strings.Repeat("b", 64) + `"}]}`
	if err := svc.corpus.TransitionJob(ctx, job.ID, corpus.JobRunningToSucceeded, result, ""); err != nil {
		t.Fatal(err)
	}
	session := connectAuditMCP(t, svc)
	out := callMCPTool[mcpcontract.GetJobsOutput](ctx, t, session, mcpcontract.ToolGetJob, map[string]any{"ids": []string{job.ID, "missing"}, "response_format": "detailed"})
	if len(out.Items) != 2 || out.Items[0].Value == nil || len(out.Items[0].Value.Artifacts) != 1 || out.Items[1].Status != mcpcontract.BatchItemUnavailable {
		t.Fatalf("batch = %+v", out)
	}
	artifact := out.Items[0].Value.Artifacts[0]
	if artifact.URI != "gitcontribute://artifact/code-index/"+strings.Repeat("a", 64) || artifact.CodeIndex == nil || artifact.CodeIndex.CommitSHA != "commit-a" {
		t.Fatalf("artifact = %+v", artifact)
	}
	// A reference must not claim zero indexed files or missing provenance as if
	// it were the full indexed-commit manifest.
	raw, err := json.Marshal(artifact.CodeIndex)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["file_count"]; ok {
		t.Fatalf("reference contains invented manifest metadata: %s", raw)
	}
}

func TestMCPValidationJobHandsOffReadableAttemptIdentities(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	inv, err := svc.StartInvestigation(ctx, contracts.RepoRef{Owner: "owner", Repo: "repo"}, "commit-a", "")
	if err != nil {
		t.Fatal(err)
	}
	definition := &evidence.ValidationDefinition{ID: "definition", InvestigationID: inv.ID, Command: []string{"git", "diff", "--check"}, WorkingDir: "/private/worktree"}
	if err := svc.corpus.SaveValidationDefinition(ctx, definition); err != nil {
		t.Fatal(err)
	}
	group := &evidence.ValidationRunGroup{ID: "group", DefinitionID: definition.ID, InvestigationID: inv.ID, RequestedRuns: 2, CompletedRuns: 2, Classification: evidence.RunGroupStablePass, Attempts: []evidence.ValidationAttempt{
		{Index: 1, Kind: evidence.RunKindBase, RunID: "base-run", Classification: evidence.RunClassificationPassing},
		{Index: 1, Kind: evidence.RunKindCandidate, RunID: "candidate-run", Classification: evidence.RunClassificationPassing},
	}}
	if err := svc.corpus.SaveValidationRunGroup(ctx, group); err != nil {
		t.Fatal(err)
	}
	job, err := svc.corpus.CreateJob(ctx, "run_validation_group", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.StartJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.TransitionJob(ctx, job.ID, corpus.JobRunningToSucceeded, `{"id":"group"}`, ""); err != nil {
		t.Fatal(err)
	}
	session := connectAuditMCP(t, svc)
	out := callMCPTool[mcpcontract.GetJobsOutput](ctx, t, session, mcpcontract.ToolGetJob, map[string]any{"ids": []string{job.ID}, "response_format": "detailed"})
	if len(out.Items) != 1 || out.Items[0].Value == nil || len(out.Items[0].Value.Artifacts) != 1 {
		t.Fatalf("job = %+v", out)
	}
	artifact := out.Items[0].Value.Artifacts[0]
	if artifact.URI == "" || out.Items[0].Value.FollowUp == nil {
		t.Fatalf("validation job has no readable handoff: %+v", out.Items[0].Value)
	}
	resource, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: artifact.URI})
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		ID             string `json:"id"`
		Classification string `json:"classification"`
		Attempts       []struct {
			RunID string `json:"run_id"`
			Kind  string `json:"kind"`
		} `json:"attempts"`
	}
	if len(resource.Contents) != 1 {
		t.Fatalf("resource = %+v", resource)
	}
	if err := json.Unmarshal([]byte(resource.Contents[0].Text), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ID != "group" || payload.Classification != "stable_pass" || len(payload.Attempts) != 2 || payload.Attempts[0].RunID != "base-run" || payload.Attempts[1].RunID != "candidate-run" {
		t.Fatalf("group resource = %+v", payload)
	}
	if strings.Contains(resource.Contents[0].Text, "/private/worktree") {
		t.Fatal("resource exposed host path")
	}
}
