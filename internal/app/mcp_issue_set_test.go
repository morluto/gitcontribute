package app

import (
	"context"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
	"github.com/morluto/gitcontribute/internal/radar"
)

func TestPrepareIssueSetComposesStoredEvidenceWithoutClaimingClosure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)
	svc.SetGitHubReader(panicRadarReader{})
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{
		Owner: "acme", Name: "rocket", SourceUpdatedAt: now,
	}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	issue, err := svc.corpus.UpsertThread(ctx, corpus.Thread{
		RepositoryID: repo.ID, Kind: corpus.ThreadKindIssue, Number: 7, State: "open",
		Title: "Avoid duplicate cache work", Body: "Cache identical requests once.", Labels: []string{"performance"},
		SourceUpdatedAt: now.Add(-2 * time.Hour),
	}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	merged := true
	if _, err := svc.corpus.UpsertThread(ctx, corpus.Thread{
		RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 21, State: "closed",
		Title: "Avoid duplicate cache work in readers", Body: "This advances #7 by caching repository reads.",
		Merged: merged, MergedKnown: true, MergedAt: now.Add(-time.Hour), SourceUpdatedAt: now.Add(-time.Hour),
	}, `{}`); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.corpus.UpsertThread(ctx, corpus.Thread{
		RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 22, State: "closed",
		Title: "Avoid duplicate cache work", Body: "Cache identical requests once.",
		MergedKnown: true, SourceUpdatedAt: now.Add(-30 * time.Minute),
	}, `{}`); err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.AdvanceFacet(ctx, repo.ID, &issue.ID, FacetIssueComments, now, true, 0); err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.AdvanceFacet(ctx, repo.ID, nil, "threads", now, true, 0); err != nil {
		t.Fatal(err)
	}

	out, err := (&MCPReader{svc}).PrepareIssueSet(ctx, mcpcontract.PrepareIssueSetInput{
		Owner: "acme", Repo: "rocket", IssueNumbers: []int{7}, PrecedentLimit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "partial" || out.ResponseFormat != "concise" || len(out.Items) != 1 {
		t.Fatalf("result = %+v", out)
	}
	item := out.Items[0]
	if item.Status != "complete" || item.Value == nil {
		t.Fatalf("item = %+v", item)
	}
	value := item.Value
	if value.Title != "Avoid duplicate cache work" || value.BodyStatus != "available" || value.Body != "" {
		t.Fatalf("issue facts = %+v", value)
	}
	if len(value.RelatedWork) != 1 || value.RelatedWork[0].Number != 21 || value.RelatedWork[0].Merged == nil || !*value.RelatedWork[0].Merged {
		t.Fatalf("related work = %+v", value.RelatedWork)
	}
	if !value.RelatedWorkTruncated {
		t.Fatal("concise response did not report omitted relationship evidence")
	}
	if value.RelatedWorkTotalKnown {
		t.Fatal("related-work total reported known with missing timeline coverage")
	}
	if len(value.AcceptedExamples) != 1 || value.AcceptedExamples[0].Ref != "acme/rocket#21" {
		t.Fatalf("accepted examples = %+v", value.AcceptedExamples)
	}
	if value.Linkage.Relation != "related" || !value.Linkage.RequiresConfirmation {
		t.Fatalf("linkage = %+v", value.Linkage)
	}
	if len(value.Gaps) != 1 || value.Gaps[0].Facet != FacetIssueTimeline || value.Gaps[0].NextAction.Tool != mcpcontract.ToolHydrateThreads {
		t.Fatalf("gaps = %+v", value.Gaps)
	}
	detailed, err := (&MCPReader{svc}).PrepareIssueSet(ctx, mcpcontract.PrepareIssueSetInput{
		Owner: "acme", Repo: "rocket", IssueNumbers: []int{7}, ResponseFormat: "detailed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := detailed.Items[0].Value; got == nil || got.Body != "Cache identical requests once." || len(got.RelatedWork[0].EvidenceKinds) == 0 || got.RelatedWorkTruncated {
		t.Fatalf("detailed issue = %+v", got)
	}
}

func TestPrepareIssueSetPreservesUnknownAndExactRecovery(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)
	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: "acme", Name: "rocket"}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.corpus.UpsertThread(ctx, corpus.Thread{
		RepositoryID: repo.ID, Kind: corpus.ThreadKindIssue, Number: 1, State: "open", Title: "Body not captured",
	}, `{}`); err != nil {
		t.Fatal(err)
	}

	out, err := (&MCPReader{svc}).PrepareIssueSet(ctx, mcpcontract.PrepareIssueSetInput{
		Owner: "acme", Repo: "rocket", IssueNumbers: []int{1, 2}, ResponseFormat: "detailed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "partial" || len(out.Items) != 2 {
		t.Fatalf("result = %+v", out)
	}
	if len(out.Gaps) != 1 || out.Gaps[0].Code != "relationship_population_unknown" || out.Gaps[0].NextAction.Tool != mcpcontract.ToolSyncThreads {
		t.Fatalf("relationship gaps = %+v", out.Gaps)
	}
	if got := out.Items[0].Value; got == nil || got.BodyStatus != "unknown" || len(got.Gaps) != 3 {
		t.Fatalf("known issue = %+v", got)
	}
	missing := out.Items[1]
	if missing.Status != "unavailable" || missing.Reason != "issue_not_indexed" || missing.NextAction != "Call github.sync_threads for this exact issue." {
		t.Fatalf("missing issue = %+v", missing)
	}
	if len(out.SuggestedActions) == 0 || out.SuggestedActions[0].Tool != mcpcontract.ToolSyncThreads {
		t.Fatalf("suggested actions = %+v", out.SuggestedActions)
	}
}

func TestPrepareIssueSetRejectsDuplicateIssueNumbers(t *testing.T) {
	t.Parallel()
	svc := newSearchTestService(t)
	_, err := (&MCPReader{svc}).PrepareIssueSet(context.Background(), mcpcontract.PrepareIssueSetInput{
		Owner: "acme", Repo: "rocket", IssueNumbers: []int{7, 7},
	})
	if err == nil {
		t.Fatal("expected duplicate issue number error")
	}
}

func TestPrepareIssueSetReturnsArrayCoverageForUnknownRepository(t *testing.T) {
	t.Parallel()
	svc := newSearchTestService(t)
	out, err := (&MCPReader{svc}).PrepareIssueSet(context.Background(), mcpcontract.PrepareIssueSetInput{
		Owner: "acme", Repo: "missing", IssueNumbers: []int{7},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Coverage == nil || len(out.Coverage) != 0 {
		t.Fatalf("coverage = %#v, want empty array", out.Coverage)
	}
}

func TestPrepareIssueSetKeepsRelatedTotalUnknownForAmbiguousPullRequestBody(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: "acme", Name: "rocket"}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	issue, err := svc.corpus.UpsertThread(ctx, corpus.Thread{
		RepositoryID: repo.ID, Kind: corpus.ThreadKindIssue, Number: 7, State: "open",
		Title: "Exact issue", Body: "Known issue body.", SourceUpdatedAt: now,
	}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.corpus.UpsertThread(ctx, corpus.Thread{
		RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 21, State: "open",
		Title: "Body may not have been captured", SourceUpdatedAt: now,
	}, `{}`); err != nil {
		t.Fatal(err)
	}
	for _, target := range []struct {
		threadID *int64
		facet    string
	}{
		{facet: "threads"},
		{threadID: &issue.ID, facet: FacetIssueComments},
		{threadID: &issue.ID, facet: FacetIssueTimeline},
	} {
		if err := svc.corpus.AdvanceFacet(ctx, repo.ID, target.threadID, target.facet, now, true, 0); err != nil {
			t.Fatal(err)
		}
	}

	out, err := (&MCPReader{svc}).PrepareIssueSet(ctx, mcpcontract.PrepareIssueSetInput{
		Owner: "acme", Repo: "rocket", IssueNumbers: []int{7},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := out.Items[0].Value; got == nil || got.RelatedWorkTotalKnown {
		t.Fatalf("prepared issue = %+v", got)
	}
}

func TestIssueSetRelatedWorkDoesNotBorrowMergeStateAcrossRepositories(t *testing.T) {
	t.Parallel()
	out := issueSetRelatedWork(
		radar.RelatedWork{Ref: "pull_request:other/repo#21", Kind: corpus.ThreadKindPullRequest, Number: 21},
		domain.RepoRef{Owner: "acme", Repo: "rocket"},
		map[int]corpus.Thread{21: {Number: 21, Merged: true, MergedKnown: true}},
		"concise",
	)
	if out.Merged != nil || out.MergedAt != "" {
		t.Fatalf("external related work borrowed local merge state: %+v", out)
	}
}
