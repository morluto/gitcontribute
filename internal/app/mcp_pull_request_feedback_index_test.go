package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

type feedbackIndexTestReader struct {
	panicRadarReader
	pages      map[int]github.ListResult[github.Issue]
	perPage    []int
	withThread bool
}

func (r *feedbackIndexTestReader) ListPullRequests(_ context.Context, _, _ string, opts github.PullRequestListOptions) (github.ListResult[github.Issue], error) {
	r.perPage = append(r.perPage, opts.PerPage)
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
		threads = []github.FeedbackThread{{ID: "thread-2", Resolved: false, Comments: []github.FeedbackComment{{ID: 202, Author: "reviewer", Body: "thread feedback", CreatedAt: now, UpdatedAt: now}}}}
	}
	return github.PullRequestFeedback{
		Header:          github.PullRequestDetails{Number: number, State: map[int]string{1: "open", 2: "closed"}[number], Author: map[int]string{1: "alice", 2: "bob"}[number], CreatedAt: now.Add(-time.Hour), UpdatedAt: now, Merged: number == 2, HeadSHA: head},
		HeadSHA:         head,
		SourceUpdatedAt: now,
		ThreadState:     opts.ThreadState,
		IssueComments:   []github.FeedbackComment{{ID: int64(number), Author: "reviewer", Body: "please address latency", CreatedAt: now, UpdatedAt: now}},
		ReviewThreads:   threads,
		Coverage:        coverage,
	}, nil
}

func TestPullRequestFeedbackIndexResumesDiscoveryAndBuildsOfflineProjection(t *testing.T) {
	ctx := context.Background()
	svc := newLocalService(t)
	t.Cleanup(func() { _ = svc.Close() })
	githubReader := &feedbackIndexTestReader{pages: map[int]github.ListResult[github.Issue]{
		1: {Items: []github.Issue{{Number: 1, Kind: github.ThreadKindPullRequest}}, Page: github.PageInfo{Page: 1, NextPage: 2, HasNext: true}},
		2: {Items: []github.Issue{{Number: 2, Kind: github.ThreadKindPullRequest}}, Page: github.PageInfo{Page: 2, HasNext: false}},
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
	first, err := reader.indexPullRequestFeedback(ctx, in, report)
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
	if discovery == nil || discovery.Complete || discovery.NextPage != 2 || discovery.DiscoveredPullRequests != 1 {
		t.Fatalf("bounded discovery = %+v", discovery)
	}

	in.MaxPages = 1
	second, err := reader.indexPullRequestFeedback(ctx, in, report)
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != "complete" || second.PullRequests != 1 {
		t.Fatalf("resumed index result = %+v", second)
	}
	if len(githubReader.perPage) != 2 || githubReader.perPage[0] != feedbackDiscoveryPageSize || githubReader.perPage[1] != feedbackDiscoveryPageSize {
		t.Fatalf("discovery page sizes = %v, want stable size %d", githubReader.perPage, feedbackDiscoveryPageSize)
	}
	discovery, err = svc.corpus.GetFeedbackDiscovery(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if discovery == nil || !discovery.Complete || discovery.DiscoveredPullRequests != 2 {
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

func TestPullRequestFeedbackSearchKeepsThreadResourceReadable(t *testing.T) {
	ctx := context.Background()
	svc := newLocalService(t)
	t.Cleanup(func() { _ = svc.Close() })
	reader := &feedbackIndexTestReader{withThread: true, pages: map[int]github.ListResult[github.Issue]{
		1: {Items: []github.Issue{{Number: 2, Kind: github.ThreadKindPullRequest}}, Page: github.PageInfo{Page: 1, HasNext: false}},
	}}
	svc.SetGitHubReader(reader)
	appReader := &MCPReader{Service: svc}
	if _, err := appReader.indexPullRequestFeedback(ctx, mcpcontract.IndexPullRequestFeedbackInput{
		Repository: mcpcontract.RepositoryRef{Owner: "acme", Repo: "rocket"}, Channels: []string{"review_threads"}, ThreadState: "all",
		MaxPullRequests: 10, MaxItemsPerChannel: 10, MaxPages: 10, MaxRequests: 20,
	}, func(string, string) error { return nil }); err != nil {
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
	if item["schema_version"] != "gitcontribute.pull-request-feedback-item.v1" || item["feedback_id"] != "202" || item["thread_id"] != "thread-2" || item["resolved"] != false || item["resolution_state"] != "unresolved" {
		t.Fatalf("exact feedback resource = %+v", item)
	}
}
