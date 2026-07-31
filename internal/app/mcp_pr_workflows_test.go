package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

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
		RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 7,
		SourceUpdatedAt: completeAt,
	}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	reader := &MCPReader{Service: svc}
	ref := mcpcontract.ThreadRef{Owner: "acme", Repo: "rocket", Kind: "pull_request", Number: 7}
	if err := reader.persistPullRequestWorkflowFacet(ctx, ref, facetPRCIReport, completeAt, map[string]string{"head_sha": "complete"}, true); err != nil {
		t.Fatal(err)
	}
	if err := reader.persistPullRequestWorkflowFacet(ctx, ref, facetPRCIReport, incompleteAt, map[string]string{"head_sha": "partial"}, false); err != nil {
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
	effective, _ := resource["effective_coverage"].(map[string]any)
	if complete, _ := effective["complete"].(bool); complete {
		t.Fatalf("resource effective coverage = %+v, want incomplete", effective)
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
		RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 7, SourceUpdatedAt: now,
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
	}, report)
	if err != nil {
		t.Fatal(err)
	}
	if feedback.BatchStatus != "partial" || feedback.Items[0].Status != "retryable" || feedback.Items[0].ResourceURI != "" {
		t.Fatalf("feedback result = %+v", feedback)
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
		RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 7, SourceUpdatedAt: now,
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
	if err := reader.persistPullRequestFeedback(ctx, ref, snapshot, []string{"issue_comments", "review_threads"}); err != nil {
		t.Fatal(err)
	}
	resource, err := reader.PullRequestFeedbackResource(ctx, "acme", "rocket", 7)
	if err != nil {
		t.Fatal(err)
	}
	channels := resource["channels"].(map[string]any)
	if channels["issue_comments"] == nil || channels["review_threads"] == nil || channels[facetPRFeedbackIssueComments] != nil {
		t.Fatalf("channels = %+v", channels)
	}
	reviewThreads := channels["review_threads"].(map[string]any)
	if reviewThreads["selection"] != "unresolved" {
		t.Fatalf("review-thread selection = %v", reviewThreads["selection"])
	}
}

func TestWorkflowFailurePreservesRetryableGitHubClassification(t *testing.T) {
	ref := mcpcontract.ThreadRef{Owner: "acme", Repo: "rocket", Number: 7}
	item := workflowFailure(ref, &github.TransientError{Cause: errors.New("head changed")}, mcpcontract.ToolSyncCIFailures)
	if item.Status != "retryable" || item.Code != "transient" || item.RetryAfterMS == 0 || item.Recovery == nil || len(item.Recovery.Then) != 1 {
		t.Fatalf("item = %+v", item)
	}
}
