package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func TestGetThreadFacetsIsOfflineAndReturnsCanonicalResources(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)
	svc.SetGitHubReader(panicRadarReader{})
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: "acme", Name: "rocket", SourceUpdatedAt: now}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	issue, err := svc.corpus.UpsertThread(ctx, corpus.Thread{RepositoryID: repo.ID, Kind: corpus.ThreadKindIssue, Number: 7, Title: "issue"}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	pullRequest, err := svc.corpus.UpsertThread(ctx, corpus.Thread{RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 7, Title: "pull request"}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.ApplyFacetObservationSet(ctx, repo.ID, &issue.ID, FacetIssueComments, now, []corpus.FacetObservationInput{{SourceUpdatedAt: now, Payload: `{"body":"hello"}`}}, true, 0); err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.ApplyFacetObservationSet(ctx, repo.ID, &pullRequest.ID, FacetPRDetails, now, []corpus.FacetObservationInput{{SourceUpdatedAt: now, Payload: `{"merged":true}`}}, true, 0); err != nil {
		t.Fatal(err)
	}

	reader := &MCPReader{svc}
	out, err := reader.GetThreadFacets(ctx, mcpcontract.GetThreadFacetsInput{
		Threads: []mcpcontract.ThreadRef{
			{Owner: "acme", Repo: "rocket", Kind: corpus.ThreadKindIssue, Number: 7},
			{Owner: "acme", Repo: "rocket", Kind: corpus.ThreadKindPullRequest, Number: 7},
		},
		Facets: []string{FacetIssueComments},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "complete" || len(out.Items) != 2 || out.Items[0].Value == nil || out.Items[1].Value == nil {
		t.Fatalf("facet batch = %+v", out)
	}
	if out.Items[0].Value.Kind != corpus.ThreadKindIssue || out.Items[1].Value.Kind != corpus.ThreadKindPullRequest {
		t.Fatalf("kind preservation = %+v", out.Items)
	}
	if out.Items[0].Value.Facets[0].ObservationCount != 1 || out.Items[0].Value.Facets[0].ResourceURI != "gitcontribute://thread/acme/rocket/issue/7/facet/issue_comments" {
		t.Fatalf("issue facets = %+v", out.Items[0].Value.Facets)
	}
	if out.Items[1].Value.Facets[0].ResourceURI != "gitcontribute://thread/acme/rocket/pull_request/7/facet/issue_comments" {
		t.Fatalf("pull-request facet = %+v", out.Items[1].Value.Facets)
	}

	missing, err := reader.GetThreadFacets(ctx, mcpcontract.GetThreadFacetsInput{
		Threads: []mcpcontract.ThreadRef{{Owner: "acme", Repo: "rocket", Kind: corpus.ThreadKindIssue, Number: 7}},
		Facets:  []string{FacetIssueTimeline},
	})
	if err != nil {
		t.Fatal(err)
	}
	if missing.Items[0].Value == nil || missing.Items[0].Value.Facets[0].Status != "not_observed" || missing.Items[0].Value.Facets[0].Recovery == nil || missing.Items[0].Value.Facets[0].Recovery.Reason != "facet_not_observed" || missing.Items[0].Value.Facets[0].Recovery.Then[0].Tool != mcpcontract.ToolHydrateThreads {
		t.Fatalf("missing facet recovery = %+v", missing.Items[0])
	}

	resource, err := reader.ThreadFacetResource(ctx, "acme", "rocket", corpus.ThreadKindPullRequest, 7, FacetPRDetails)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(resource)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" || resource["schema_version"] != "gitcontribute.thread-facet.v1" || len(resource["observations"].([]any)) != 1 {
		t.Fatalf("facet resource = %s", data)
	}
}

func TestFacetJobFollowUpReadsFacetSurface(t *testing.T) {
	t.Parallel()
	job := &contracts.JobResult{
		Kind:    jobKindSyncThreadFacets,
		Status:  "succeeded",
		Request: `{"threads":[{"owner":"acme","repo":"rocket","kind":"pull_request","number":7}],"facets":["pr_details"]}`,
		Result:  `{"status":"complete","items":[]}`,
	}
	artifacts, follow := jobArtifactsAndFollowUp(job, 1)
	if len(artifacts) != 1 || artifacts[0].Kind != "thread_facet_batch" || follow == nil || follow.Tool != mcpcontract.ToolGetThreadFacets || follow.Arguments == nil {
		t.Fatalf("facet job result = artifacts:%+v follow:%+v", artifacts, follow)
	}
	if len(follow.Arguments.Threads) != 1 || follow.Arguments.Threads[0].Kind != "pull_request" || len(follow.Arguments.Facets) != 1 || follow.Arguments.Facets[0] != "pr_details" {
		t.Fatalf("facet follow-up arguments = %+v", follow.Arguments)
	}
}
