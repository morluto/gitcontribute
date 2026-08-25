package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func mustFeedbackSelection(t *testing.T, channels []string, threadState string) corpus.FeedbackSelection {
	t.Helper()
	selection, err := corpus.ParseFeedbackSelection(channels, threadState)
	if err != nil {
		t.Fatalf("parse feedback selection: %v", err)
	}
	return selection
}

func TestIncompletePullRequestFacetPreservesLastCompleteObservation(t *testing.T) {
	ctx := context.Background()
	svc := newLocalService(t)
	t.Cleanup(func() { _ = svc.Close() })
	stored := svc.corpus
	completeAt := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	incompleteAt := completeAt.Add(time.Hour)
	repo, err := stored.UpsertRepository(ctx, corpus.Repository{
		Owner: "acme", Name: "rocket", SourceUpdatedAt: completeAt,
	}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := stored.UpsertThread(ctx, corpus.Thread{
		RepositoryID: repo.ID, Kind: domain.PullRequestKind, Number: 7,
		State: "open", SourceUpdatedAt: completeAt,
	}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	reader := &MCPReader{Service: svc}
	ref := mcpcontract.ThreadRef{Owner: "acme", Repo: "rocket", Kind: "pull_request", Number: 7}
	completeUpdate, err := completeWorkflowFacetUpdate(map[string]string{"head_sha": "complete"})
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.persistPullRequestWorkflowFacet(ctx, ref, facetPRCIReport, completeAt, completeUpdate); err != nil {
		t.Fatal(err)
	}
	if err := reader.persistPullRequestWorkflowFacet(ctx, ref, facetPRCIReport, incompleteAt, incompleteWorkflowFacetUpdate()); err != nil {
		t.Fatal(err)
	}

	observations, _, err := stored.ListFacetObservationsBounded(ctx, repo.ID, &thread.ID, facetPRCIReport, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 1 || observations[0].Payload != `{"head_sha":"complete"}` {
		t.Fatalf("observations = %+v, want only last complete snapshot", observations)
	}
	coverage, err := stored.GetCoverage(ctx, repo.ID, &thread.ID, facetPRCIReport)
	if err != nil {
		t.Fatal(err)
	}
	if coverage == nil || coverage.Complete || !coverage.SourceUpdatedAt.Equal(incompleteAt) {
		t.Fatalf("coverage = %+v, want newer incomplete coverage", coverage)
	}
	resource, err := reader.CIFailureResource(ctx, "acme", "rocket", 7)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(resource)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		HeadSHA           string                        `json:"head_sha"`
		EffectiveCoverage *mcpcontract.ResourceCoverage `json:"effective_coverage"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.HeadSHA != "complete" || envelope.EffectiveCoverage == nil || envelope.EffectiveCoverage.Complete {
		t.Fatalf("resource = %s, want preserved payload with incomplete effective coverage", payload)
	}
}

func TestFeedbackSyncSeedsMissingRepositoryAndPullRequest(t *testing.T) {
	ctx := context.Background()
	svc := newLocalService(t)
	t.Cleanup(func() { _ = svc.Close() })
	now := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	svc.SetGitHubReader(&boundedWorkflowReader{
		feedback: github.PullRequestFeedback{
			Header: github.PullRequestDetails{
				Number: 7, State: "open", Title: "new title", Body: "body",
				Author: "alice", CreatedAt: now.Add(-time.Hour), UpdatedAt: now,
				HeadSHA: "head-7",
			},
			HeadSHA: "head-7", SourceUpdatedAt: now,
			IssueComments: []github.FeedbackComment{{ID: "10", Body: "comment"}},
			Coverage: map[string]github.FeedbackCoverage{
				"issue_comments": {Complete: true, Fetched: 1, Total: 1},
			},
		},
	})
	reader := &MCPReader{Service: svc}
	ref := mcpcontract.ThreadRef{Owner: "acme", Repo: "rocket", Kind: "pull_request", Number: 7}
	result, err := reader.syncPullRequestFeedback(ctx, mcpcontract.SyncPullRequestFeedbackInput{
		PullRequests: []mcpcontract.ThreadRef{ref}, Channels: []string{"issue_comments"}, MaxRequests: 10,
	}, mustFeedbackSelection(t, []string{"issue_comments"}, "unresolved"), func(string, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if result.BatchStatus != "complete" || len(result.Items) != 1 || result.Items[0].Status != "complete" {
		t.Fatalf("feedback result = %+v", result)
	}

	repo, err := svc.corpus.GetRepository(ctx, "acme", "rocket")
	if err != nil || repo == nil {
		t.Fatalf("stored repository = %v, %+v", err, repo)
	}
	thread, err := svc.corpus.GetThread(ctx, repo.ID, domain.PullRequestKind, 7)
	if err != nil || thread == nil {
		t.Fatalf("stored pull request = %v, %+v", err, thread)
	}
	if thread.Title != "new title" || thread.State != "open" || thread.StateReason != "" {
		t.Fatalf("stored pull-request header = %+v", thread)
	}
	resource, err := reader.PullRequestFeedbackResource(ctx, "acme", "rocket", 7)
	if err != nil {
		t.Fatal(err)
	}
	if resource.Number != 7 || result.Items[0].ResourceURI == "" {
		t.Fatalf("feedback resource/result = %+v / %+v", resource, result)
	}
}

func TestFeedbackSyncPreservesExistingRepositoryAndPullRequestFields(t *testing.T) {
	ctx := context.Background()
	svc := newLocalService(t)
	t.Cleanup(func() { _ = svc.Close() })
	now := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{
		Owner: "acme", Name: "rocket", Description: "existing metadata", SourceUpdatedAt: now,
	}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.corpus.UpsertThread(ctx, corpus.Thread{
		RepositoryID: repo.ID, Kind: domain.PullRequestKind, Number: 7,
		State: "open", StateReason: "completed", Title: "old title", SourceUpdatedAt: now,
	}, `{}`); err != nil {
		t.Fatal(err)
	}
	svc.SetGitHubReader(&boundedWorkflowReader{
		feedback: github.PullRequestFeedback{
			Header: github.PullRequestDetails{
				Number: 7, State: "open", Title: "new title", UpdatedAt: now.Add(time.Hour),
				HeadSHA: "head-7",
			},
			HeadSHA: "head-7", SourceUpdatedAt: now.Add(time.Hour),
			Coverage: map[string]github.FeedbackCoverage{
				"issue_comments": {Complete: true},
			},
		},
	})
	reader := &MCPReader{Service: svc}
	ref := mcpcontract.ThreadRef{Owner: "acme", Repo: "rocket", Kind: "pull_request", Number: 7}
	result, err := reader.syncPullRequestFeedback(ctx, mcpcontract.SyncPullRequestFeedbackInput{
		PullRequests: []mcpcontract.ThreadRef{ref}, Channels: []string{"issue_comments"}, MaxRequests: 10,
	}, mustFeedbackSelection(t, []string{"issue_comments"}, "unresolved"), func(string, string) error { return nil })
	if err != nil || result.BatchStatus != "complete" {
		t.Fatalf("feedback result = %v, %+v", err, result)
	}

	gotRepo, err := svc.corpus.GetRepository(ctx, "acme", "rocket")
	if err != nil {
		t.Fatal(err)
	}
	gotThread, err := svc.corpus.GetThread(ctx, gotRepo.ID, domain.PullRequestKind, 7)
	if err != nil {
		t.Fatal(err)
	}
	if gotRepo.Description != "existing metadata" || gotThread.Title != "new title" || gotThread.StateReason != "completed" {
		t.Fatalf("richer fields were not preserved: repo=%+v thread=%+v", gotRepo, gotThread)
	}
}

func TestFeedbackSyncReportsStructuredPersistenceFailure(t *testing.T) {
	ctx := context.Background()
	svc := newLocalService(t)
	t.Cleanup(func() { _ = svc.Close() })
	svc.SetGitHubReader(&boundedWorkflowReader{
		feedback: github.PullRequestFeedback{
			HeadSHA: "head-7", Coverage: map[string]github.FeedbackCoverage{
				"issue_comments": {Complete: true},
			},
		},
	})
	reader := &MCPReader{Service: svc}
	ref := mcpcontract.ThreadRef{Owner: "acme", Repo: "rocket", Kind: "pull_request", Number: 7}
	result, err := reader.syncPullRequestFeedback(ctx, mcpcontract.SyncPullRequestFeedbackInput{
		PullRequests: []mcpcontract.ThreadRef{ref}, Channels: []string{"issue_comments"}, MaxRequests: 10,
	}, mustFeedbackSelection(t, []string{"issue_comments"}, "unresolved"), func(string, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if result.BatchStatus != "failed" || result.Items[0].Status != "failed" || result.Items[0].Code != "repository_identity_unavailable" {
		t.Fatalf("feedback persistence failure = %+v", result)
	}
}

func TestBoundedWorkflowSnapshotsReturnRetryablePartialItems(t *testing.T) {
	ctx := context.Background()
	svc := newLocalService(t)
	t.Cleanup(func() { _ = svc.Close() })
	now := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: "acme", Name: "rocket", SourceUpdatedAt: now}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := svc.corpus.UpsertThread(ctx, corpus.Thread{
		RepositoryID: repo.ID, Kind: domain.PullRequestKind, Number: 7, State: "open", SourceUpdatedAt: now,
	}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	svc.SetGitHubReader(&boundedWorkflowReader{
		feedback: github.PullRequestFeedback{
			HeadSHA: "head", SourceUpdatedAt: now,
			Coverage: map[string]github.FeedbackCoverage{
				"issue_comments": {Complete: true},
				"review_threads": {Complete: false},
			},
		},
		ci: github.PullRequestCI{
			HeadSHA:  "head",
			Coverage: map[string]github.FeedbackCoverage{"workflow_runs": {Complete: true}},
			Runs:     []github.CIRun{{JobsTruncated: true}},
		},
	})
	reader := &MCPReader{Service: svc}
	ref := mcpcontract.ThreadRef{Owner: "acme", Repo: "rocket", Kind: "pull_request", Number: 7}
	report := func(string, string) error { return nil }

	feedback, err := reader.syncPullRequestFeedback(ctx, mcpcontract.SyncPullRequestFeedbackInput{
		PullRequests: []mcpcontract.ThreadRef{ref}, Channels: []string{"issue_comments", "review_threads"},
		MaxItemsPerChannel: 10, MaxRequests: 10,
	}, mustFeedbackSelection(t, []string{"issue_comments", "review_threads"}, "unresolved"), report)
	if err != nil {
		t.Fatal(err)
	}
	if feedback.BatchStatus != "partial" || feedback.Items[0].Status != "retryable" || feedback.Items[0].ResourceURI != "" {
		t.Fatalf("feedback result = %+v", feedback)
	}
	if feedback.Items[0].Recovery == nil || len(feedback.Items[0].Recovery.Then) != 1 {
		t.Fatalf("feedback recovery = %+v", feedback.Items[0].Recovery)
	}
	nextFeedback, ok := mcpcontract.RecoveryInput[mcpcontract.SyncPullRequestFeedbackInput](feedback.Items[0].Recovery.Then[0])
	if !ok || nextFeedback.MaxItemsPerChannel != 20 {
		t.Fatalf("feedback recovery = %+v", feedback.Items[0].Recovery)
	}

	ci, err := reader.syncCIFailures(ctx, mcpcontract.SyncCIFailuresInput{
		PullRequests: []mcpcontract.ThreadRef{ref}, MaxRunsPerPR: 1, MaxJobsPerRun: 1,
		MaxLogBytesPerJob: 1024, MaxRequests: 10,
	}, report)
	if err != nil {
		t.Fatal(err)
	}
	if ci.BatchStatus != "partial" || ci.Items[0].Status != "retryable" || ci.Items[0].ResourceURI != "" {
		t.Fatalf("CI result = %+v", ci)
	}

	// A budget can be exhausted immediately after the PR lookup, before any
	// child collection adds coverage. The refresh must still advance the
	// stored facet so the prior complete report is no longer treated as fresh.
	svc.SetGitHubReader(&boundedWorkflowReader{
		ci:    github.PullRequestCI{HeadSHA: "new-head", SourceUpdatedAt: now.Add(time.Hour)},
		ciErr: github.ErrRequestBudgetExhausted,
	})
	if _, err := reader.syncCIFailures(ctx, mcpcontract.SyncCIFailuresInput{
		PullRequests: []mcpcontract.ThreadRef{ref}, MaxRunsPerPR: 1, MaxJobsPerRun: 1,
		MaxLogBytesPerJob: 1024, MaxRequests: 1,
	}, report); err != nil {
		t.Fatal(err)
	}
	coverage, err := svc.corpus.GetCoverage(ctx, repo.ID, &thread.ID, facetPRCIReport)
	if err != nil {
		t.Fatal(err)
	}
	if coverage != nil && coverage.Complete {
		t.Fatalf("CI coverage remained complete after pre-collection budget exhaustion: %+v", coverage)
	}
}

func TestFeedbackCoverageRecoveryRespectsAdvertisedItemLimit(t *testing.T) {
	t.Parallel()
	ref := mcpcontract.ThreadRef{Owner: "acme", Repo: "rocket", Kind: "pull_request", Number: 7}
	if plan := feedbackCoverageRecovery(ref, mcpcontract.SyncPullRequestFeedbackInput{MaxItemsPerChannel: maxFeedbackItemsPerChannel}, "incomplete"); plan != nil {
		t.Fatalf("hard-limit recovery = %+v, want nil", plan)
	}
	plan := feedbackCoverageRecovery(ref, mcpcontract.SyncPullRequestFeedbackInput{MaxItemsPerChannel: maxFeedbackItemsPerChannel - 1}, "incomplete")
	if plan == nil || len(plan.Then) != 1 {
		t.Fatalf("capped recovery = %+v", plan)
	}
	nextFeedback, ok := mcpcontract.RecoveryInput[mcpcontract.SyncPullRequestFeedbackInput](plan.Then[0])
	if !ok || nextFeedback.MaxItemsPerChannel != maxFeedbackItemsPerChannel {
		t.Fatalf("capped recovery = %+v", plan)
	}
}

type boundedWorkflowReader struct {
	panicRadarReader
	feedback github.PullRequestFeedback
	ci       github.PullRequestCI
	ciErr    error
}

func (r *boundedWorkflowReader) GetPullRequestFeedback(context.Context, string, string, int, github.PullRequestFeedbackOptions, *github.RequestBudget) (github.PullRequestFeedback, error) {
	return r.feedback, nil
}

func (r *boundedWorkflowReader) GetPullRequestCI(context.Context, string, string, int, github.CIFailureOptions, *github.RequestBudget) (github.PullRequestCI, error) {
	return r.ci, r.ciErr
}

func TestFeedbackResourceUsesPublicChannelsAndPreservesThreadSelection(t *testing.T) {
	ctx := context.Background()
	svc := newLocalService(t)
	t.Cleanup(func() { _ = svc.Close() })
	now := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: "acme", Name: "rocket", SourceUpdatedAt: now}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.corpus.UpsertThread(ctx, corpus.Thread{
		RepositoryID: repo.ID, Kind: domain.PullRequestKind, Number: 7, State: "open", SourceUpdatedAt: now,
	}, `{}`); err != nil {
		t.Fatal(err)
	}
	reader := &MCPReader{Service: svc}
	ref := mcpcontract.ThreadRef{Owner: "acme", Repo: "rocket", Kind: "pull_request", Number: 7}
	snapshot := github.PullRequestFeedback{
		HeadSHA: "head", SourceUpdatedAt: now, ThreadState: "unresolved",
		Coverage: map[string]github.FeedbackCoverage{
			"issue_comments": {Complete: true},
			"review_threads": {Complete: true},
		},
	}
	selection := mustFeedbackSelection(t, []string{"issue_comments", "review_threads"}, "unresolved")
	if err := reader.persistPullRequestFeedback(ctx, ref, snapshot, selection.ChannelValues()); err != nil {
		t.Fatal(err)
	}
	resource, err := reader.PullRequestFeedbackResource(ctx, "acme", "rocket", 7)
	if err != nil {
		t.Fatal(err)
	}
	if resource.Channels.IssueComments == nil || resource.Channels.ReviewThreads == nil || resource.Channels.SubmittedReviews != nil {
		t.Fatalf("channels = %+v", resource.Channels)
	}
	payload, err := json.Marshal(resource.Channels.ReviewThreads)
	if err != nil {
		t.Fatal(err)
	}
	var reviewThreads struct {
		Selection string `json:"selection"`
	}
	if err := json.Unmarshal(payload, &reviewThreads); err != nil {
		t.Fatal(err)
	}
	if reviewThreads.Selection != "unresolved" {
		t.Fatalf("review-thread selection = %v", reviewThreads.Selection)
	}
}

func TestWorkflowFailurePreservesRetryableGitHubClassification(t *testing.T) {
	ref := mcpcontract.ThreadRef{Owner: "acme", Repo: "rocket", Number: 7}
	item := workflowFailure(ref, &github.TransientError{Cause: errors.New("head changed")}, mcpcontract.ToolSyncCIFailures)
	if item.Status != "retryable" || item.Code != "transient" || item.RetryAfterMS == 0 || item.Recovery == nil || len(item.Recovery.Then) != 1 {
		t.Fatalf("item = %+v", item)
	}
}

func TestFeedbackSearchRecoveryRefreshesAllThreadState(t *testing.T) {
	ctx := context.Background()
	svc := newLocalService(t)
	t.Cleanup(func() { _ = svc.Close() })
	now := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: "acme", Name: "rocket", SourceUpdatedAt: now}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.corpus.UpsertThread(ctx, corpus.Thread{RepositoryID: repo.ID, Kind: domain.PullRequestKind, Number: 7, State: "open", SourceUpdatedAt: now}, `{}`); err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.UpsertFeedbackDiscovery(ctx, corpus.FeedbackDiscovery{RepositoryID: repo.ID, Generation: 1, State: corpus.FeedbackDiscoveryComplete, Selection: mustFeedbackSelection(t, []string{"issue_comments"}, "all"), SourceUpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	plan := feedbackSearchRecovery(ctx, svc.corpus, repo.ID, domain.MustRepoRef("acme", "rocket"), mcpcontract.SearchPullRequestFeedbackInput{
		Repository: mcpcontract.RepositoryRef{Owner: "acme", Repo: "rocket"}, Channel: "issue_comments", ThreadState: "resolved",
	}, corpus.FeedbackSearchPage{Coverage: corpus.FeedbackCoverageSummary{State: corpus.FeedbackCoveragePartialFacets, IncompletePRs: 1}})
	if plan == nil || len(plan.Then) != 1 {
		t.Fatalf("feedback recovery plan = %+v", plan)
	}
	nextFeedback, ok := mcpcontract.RecoveryInput[mcpcontract.SyncPullRequestFeedbackInput](plan.Then[0])
	if !ok || nextFeedback.ThreadState != "all" {
		t.Fatalf("feedback recovery plan = %+v", plan)
	}
}

func TestFeedbackSearchRecoveryBoundsMergeStateHydration(t *testing.T) {
	unknown := make([]int, 0, 50)
	for number := 1; number <= 50; number++ {
		unknown = append(unknown, number)
	}
	items := make([]corpus.PullRequestFeedbackProjection, 0, 100)
	for number := 51; number <= 150; number++ {
		items = append(items, corpus.PullRequestFeedbackProjection{PullRequestNumber: number})
	}
	plan := feedbackSearchRecovery(context.Background(), nil, 0, domain.MustRepoRef("acme", "rocket"), mcpcontract.SearchPullRequestFeedbackInput{}, corpus.FeedbackSearchPage{
		Coverage:                 corpus.FeedbackCoverageSummary{State: corpus.FeedbackCoverageComplete},
		UnknownMergePullRequests: unknown,
		Items:                    items,
	})
	if plan == nil || len(plan.Then) != 1 {
		t.Fatalf("merge-state recovery plan = %+v", plan)
	}
	nextHydration, ok := mcpcontract.RecoveryInput[mcpcontract.HydrateThreadsInput](plan.Then[0])
	if !ok {
		t.Fatalf("merge-state recovery plan = %+v", plan)
	}
	threads := nextHydration.Threads
	if len(threads) != maxFeedbackMergeStateRecoveryThreads {
		t.Fatalf("recovery thread count = %d, want %d", len(threads), maxFeedbackMergeStateRecoveryThreads)
	}
	if threads[0].Number != 1 || threads[len(threads)-1].Number != 100 {
		t.Fatalf("recovery thread bounds = first %d, last %d; want 1 through 100", threads[0].Number, threads[len(threads)-1].Number)
	}
}
