package app

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

type feedbackIndexTestReader struct {
	panicRadarReader
	pages      map[int]github.ListResult[github.Issue]
	perPage    []int
	states     []string
	withThread bool
}

type cancelledPartialFeedbackReader struct {
	panicRadarReader
	cancel context.CancelFunc
}

func (r *cancelledPartialFeedbackReader) GetPullRequestFeedback(_ context.Context, _, _ string, _ int, _ github.PullRequestFeedbackOptions, _ *github.RequestBudget) (github.PullRequestFeedback, error) {
	r.cancel()
	return github.PullRequestFeedback{Coverage: map[string]github.FeedbackCoverage{
		"issue_comments": {Complete: true, Fetched: 1, Total: 1},
	}}, errors.New("provider interrupted")
}

func (r *feedbackIndexTestReader) ListPullRequests(_ context.Context, _, _ string, opts github.PullRequestListOptions) (github.ListResult[github.Issue], error) {
	r.perPage = append(r.perPage, opts.PerPage)
	r.states = append(r.states, opts.State)
	return r.pages[opts.Page], nil
}

func (r *feedbackIndexTestReader) GetPullRequestFeedback(_ context.Context, _, _ string, number int, opts github.PullRequestFeedbackOptions, _ *github.RequestBudget) (github.PullRequestFeedback, error) {
	now := time.Date(2026, 7, 31, 10, number, 0, 0, time.UTC)
	coverage := make(map[string]github.FeedbackCoverage, len(opts.Channels))
	for _, channel := range opts.Channels {
		coverage[channel] = github.FeedbackCoverage{Complete: true, Fetched: 1, Total: 1}
	}
	head := fmt.Sprintf("head-%d", number)
	threads := []github.FeedbackThread(nil)
	if r.withThread {
		threads = []github.FeedbackThread{{ID: "thread-2", Resolved: false, Comments: []github.FeedbackComment{{ID: "202", Author: "reviewer", Body: "thread feedback", CreatedAt: now, UpdatedAt: now}}}}
	}
	return github.PullRequestFeedback{
		Header:          github.PullRequestDetails{Number: number, State: map[int]string{1: "open", 2: "closed"}[number], Author: map[int]string{1: "alice", 2: "bob"}[number], CreatedAt: now.Add(-time.Hour), UpdatedAt: now, Merged: number == 2, HeadSHA: head},
		HeadSHA:         head,
		SourceUpdatedAt: now,
		ThreadState:     opts.ThreadState,
		IssueComments:   []github.FeedbackComment{{ID: fmt.Sprintf("%d", number), Author: "reviewer", Body: "please address latency", CreatedAt: now, UpdatedAt: now}},
		ReviewThreads:   threads,
		Coverage:        coverage,
	}, nil
}

func TestPullRequestFeedbackIndexResumesDiscoveryAndBuildsOfflineProjection(t *testing.T) {
	ctx := context.Background()
	svc := newLocalService(t)
	t.Cleanup(func() { _ = svc.Close() })
	githubReader := &feedbackIndexTestReader{pages: map[int]github.ListResult[github.Issue]{
		1: {Items: []github.Issue{{Number: 1, Kind: domain.PullRequestKind}}, Page: github.PageInfo{Page: 1, NextPage: 2, HasNext: true}},
		2: {Items: []github.Issue{{Number: 2, Kind: domain.PullRequestKind}}, Page: github.PageInfo{Page: 2, HasNext: false}},
	}}
	svc.SetGitHubReader(githubReader)
	reader := &MCPReader{Service: svc}
	in := mcpcontract.IndexPullRequestFeedbackInput{
		Repository:         mcpcontract.RepositoryRef{Owner: "acme", Repo: "rocket"},
		Channels:           []string{"issue_comments", "submitted_reviews", "inline_comments", "review_threads"},
		ThreadState:        "all",
		MaxPullRequests:    10,
		MaxItemsPerChannel: 10,
		MaxPages:           1,
		MaxRequests:        20,
	}
	report := func(string, string) error { return nil }
	selection := mustFeedbackSelection(t, in.Channels, in.ThreadState)
	first, err := reader.indexPullRequestFeedback(ctx, in, selection, report)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != "partial" || first.NextPage != 2 || first.DiscoveryStatus != "partial" {
		t.Fatalf("bounded index result = %+v", first)
	}
	discovery, err := svc.corpus.GetFeedbackDiscovery(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if discovery == nil || discovery.IsComplete() || discovery.NextPage != 2 || discovery.DiscoveredPullRequests != 1 {
		t.Fatalf("bounded discovery = %+v", discovery)
	}

	in.MaxPages = 1
	second, err := reader.indexPullRequestFeedback(ctx, in, selection, report)
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != "complete" || second.PullRequests != 1 {
		t.Fatalf("resumed index result = %+v", second)
	}
	if len(githubReader.perPage) != 2 || githubReader.perPage[0] != feedbackDiscoveryPageSize || githubReader.perPage[1] != feedbackDiscoveryPageSize {
		t.Fatalf("discovery page sizes = %v, want stable size %d", githubReader.perPage, feedbackDiscoveryPageSize)
	}
	if len(githubReader.states) != 2 || githubReader.states[0] != "all" || githubReader.states[1] != "all" {
		t.Fatalf("discovery states = %v, want all", githubReader.states)
	}
	discovery, err = svc.corpus.GetFeedbackDiscovery(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if discovery == nil || !discovery.IsComplete() || discovery.DiscoveredPullRequests != 2 {
		t.Fatalf("completed discovery = %+v", discovery)
	}

	result, err := reader.SearchPullRequestFeedback(ctx, mcpcontract.SearchPullRequestFeedbackInput{
		Repository: mcpcontract.RepositoryRef{Owner: "acme", Repo: "rocket"}, Merged: "true", FeedbackAuthor: "reviewer", Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "complete" || result.Coverage != "complete" || result.Total != 1 || len(result.Matches) != 1 || result.Matches[0].PullRequest.Number != 2 {
		t.Fatalf("offline feedback search = %+v", result)
	}
}

func TestPullRequestFeedbackIndexPreservesOpenDiscoveryScope(t *testing.T) {
	ctx := context.Background()
	svc := newLocalService(t)
	t.Cleanup(func() { _ = svc.Close() })
	githubReader := &feedbackIndexTestReader{pages: map[int]github.ListResult[github.Issue]{
		1: {Items: []github.Issue{{Number: 1, Kind: domain.PullRequestKind}}, Page: github.PageInfo{Page: 1, HasNext: false}},
	}}
	svc.SetGitHubReader(githubReader)
	reader := &MCPReader{Service: svc}
	in := mcpcontract.IndexPullRequestFeedbackInput{
		Repository: mcpcontract.RepositoryRef{Owner: "acme", Repo: "rocket"}, State: "open",
		Channels: []string{"issue_comments"}, ThreadState: "all", MaxPullRequests: 10, MaxItemsPerChannel: 10, MaxPages: 10, MaxRequests: 20,
	}
	result, err := reader.indexPullRequestFeedback(ctx, in, mustFeedbackSelection(t, in.Channels, in.ThreadState), func(string, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "open" || len(githubReader.states) != 1 || githubReader.states[0] != "open" {
		t.Fatalf("state-scoped provider request/result = states %v result %+v", githubReader.states, result)
	}
	discovery, err := svc.corpus.GetFeedbackDiscovery(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if discovery == nil || !discovery.IsComplete() || !discovery.PullRequestState.IsOpen() {
		t.Fatalf("open discovery checkpoint = %+v", discovery)
	}
	openResult, err := reader.SearchPullRequestFeedback(ctx, mcpcontract.SearchPullRequestFeedbackInput{Repository: in.Repository, State: "open", Channel: "issue_comments", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if openResult.Coverage != "complete" {
		t.Fatalf("open search coverage = %+v", openResult)
	}
	allResult, err := reader.SearchPullRequestFeedback(ctx, mcpcontract.SearchPullRequestFeedbackInput{Repository: in.Repository, State: "all", Channel: "issue_comments", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if allResult.Coverage == "complete" || allResult.Recovery == nil {
		t.Fatalf("open-only discovery was presented as historical coverage: %+v", allResult)
	}
}

func TestExactFeedbackSyncAcceptsSixtyTwoPullRequestsAsOneJob(t *testing.T) {
	ctx := context.Background()
	svc := newLocalService(t)
	t.Cleanup(func() { _ = svc.Close() })
	svc.SetGitHubReader(&feedbackIndexTestReader{})
	refs := make([]mcpcontract.ThreadRef, 62)
	for i := range refs {
		refs[i] = mcpcontract.ThreadRef{Owner: "acme", Repo: "rocket", Kind: "pull_request", Number: i + 1}
	}
	job, err := (&MCPReader{Service: svc}).SyncPullRequestFeedback(ctx, mcpcontract.SyncPullRequestFeedbackInput{
		PullRequests: refs, Channels: []string{"issue_comments"}, ThreadState: "all", MaxItemsPerChannel: 10, MaxRequests: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if job.ID == "" || job.Kind != "sync_pull_request_feedback" {
		t.Fatalf("62-PR exact feedback job = %+v", job)
	}
}

func TestFeedbackWorkflowSlotWaitIsCancellable(t *testing.T) {
	svc := &Service{}
	release, err := svc.acquireFeedbackWorkflow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := svc.acquireFeedbackWorkflow(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting feedback workflow error = %v, want cancellation", err)
	}
	release()
	secondRelease, err := svc.acquireFeedbackWorkflow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	secondRelease()
}

func TestPullRequestFeedbackSearchKeepsThreadResourceReadable(t *testing.T) {
	ctx := context.Background()
	svc := newLocalService(t)
	t.Cleanup(func() { _ = svc.Close() })
	reader := &feedbackIndexTestReader{withThread: true, pages: map[int]github.ListResult[github.Issue]{
		1: {Items: []github.Issue{{Number: 2, Kind: domain.PullRequestKind}}, Page: github.PageInfo{Page: 1, HasNext: false}},
	}}
	svc.SetGitHubReader(reader)
	appReader := &MCPReader{Service: svc}
	indexInput := mcpcontract.IndexPullRequestFeedbackInput{
		Repository: mcpcontract.RepositoryRef{Owner: "acme", Repo: "rocket"}, Channels: []string{"review_threads"}, ThreadState: "all",
		MaxPullRequests: 10, MaxItemsPerChannel: 10, MaxPages: 10, MaxRequests: 20,
	}
	if _, err := appReader.indexPullRequestFeedback(ctx, indexInput, mustFeedbackSelection(t, indexInput.Channels, indexInput.ThreadState), func(string, string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	result, err := appReader.SearchPullRequestFeedback(ctx, mcpcontract.SearchPullRequestFeedbackInput{
		Repository: mcpcontract.RepositoryRef{Owner: "acme", Repo: "rocket"}, Channel: "review_threads", Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 1 || result.Matches[0].ThreadID == "" || result.Matches[0].ThreadReference != "gitcontribute://pull-request-feedback/acme/rocket/2" || result.Matches[0].CommentReference != "gitcontribute://pull-request-feedback/acme/rocket/2/review_threads/202" {
		t.Fatalf("thread resource match = %+v", result.Matches)
	}
	item, err := appReader.PullRequestFeedbackItemResource(ctx, "acme", "rocket", 2, "review_threads", "202")
	if err != nil {
		t.Fatal(err)
	}
	if item.SchemaVersion != "gitcontribute.pull-request-feedback-item.v1" || item.FeedbackID != "202" || item.ThreadID != "thread-2" || item.Resolved == nil || *item.Resolved || item.ResolutionState != "unresolved" {
		t.Fatalf("exact feedback resource = %+v", item)
	}
}

func TestFeedbackIndexReportsPartialSnapshotPersistenceFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc := newLocalService(t)
	t.Cleanup(func() { _ = svc.Close() })
	reader := &MCPReader{Service: svc}
	selection := mustFeedbackSelection(t, []string{"issue_comments"}, "all")
	item := reader.indexOnePullRequestFeedback(ctx, &cancelledPartialFeedbackReader{cancel: cancel}, mcpcontract.ThreadRef{Owner: "acme", Repo: "rocket", Kind: "pull_request", Number: 7}, mcpcontract.IndexPullRequestFeedbackInput{Channels: []string{"issue_comments"}}, selection, github.NewRequestBudget(10))
	if item.Code != "feedback_persistence_failed" {
		t.Fatalf("partial snapshot persistence failure was hidden: %+v", item)
	}
}
