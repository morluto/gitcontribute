package corpus

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestPullRequestFeedbackProjectionRebuildAndSearch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	now := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	repo, err := c.UpsertRepository(ctx, Repository{Owner: "acme", Name: "rocket", SourceUpdatedAt: now}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	pr, err := c.UpsertThread(ctx, Thread{RepositoryID: repo.ID, Kind: ThreadKindPullRequest, Number: 7, State: "closed", Author: "submitter", Merged: true, MergedKnown: true, SourceUpdatedAt: now}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	completePayload := func(items string) string {
		return `{"head_sha":"head-7","coverage":{"complete":true},"items":` + items + `}`
	}
	if err := c.ApplyFacetObservationSet(ctx, repo.ID, &pr.ID, feedbackFacetIssueComments, now, []FacetObservationInput{{SourceUpdatedAt: now, Payload: completePayload(`[{"id":11,"author":"alice","body":"please fix latency","created_at":"2026-07-31T10:01:00Z","updated_at":"2026-07-31T10:02:00Z"}]`)}}, true, 0); err != nil {
		t.Fatal(err)
	}
	for _, facet := range []string{feedbackFacetReviews, feedbackFacetInlineComments, feedbackFacetReviewThreads} {
		if err := c.ApplyFacetObservationSet(ctx, repo.ID, &pr.ID, facet, now, []FacetObservationInput{{SourceUpdatedAt: now, Payload: completePayload(`[]`)}}, true, 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.UpsertFeedbackDiscovery(ctx, FeedbackDiscovery{RepositoryID: repo.ID, State: "all", NextPage: 1, Complete: true, DiscoveredPullRequests: 1, Channels: feedbackChannels, ThreadState: "all", SourceUpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RebuildPullRequestFeedbackProjection(ctx); err != nil {
		t.Fatal(err)
	}
	page, err := c.SearchPullRequestFeedback(ctx, FeedbackSearchFilter{RepositoryID: repo.ID, Text: "latency", FeedbackAuthor: "alice", Merged: "true", State: "closed", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].FeedbackID != "11" || page.Items[0].PullRequestNumber != 7 || page.Items[0].PullRequestMerged != true || page.Coverage.Status != "complete" {
		t.Fatalf("feedback page = %+v", page)
	}
}

func TestPullRequestFeedbackIncompleteRefreshPreservesCompleteProjection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	first := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	repo, err := c.UpsertRepository(ctx, Repository{Owner: "acme", Name: "rocket", SourceUpdatedAt: first}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	pr, err := c.UpsertThread(ctx, Thread{RepositoryID: repo.ID, Kind: ThreadKindPullRequest, Number: 8, State: "open", SourceUpdatedAt: first}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	payload := `{"head_sha":"old","coverage":{"complete":true},"items":[{"id":12,"author":"alice","body":"old review","created_at":"2026-07-31T10:01:00Z"}]}`
	if err := c.ApplyFacetObservationSet(ctx, repo.ID, &pr.ID, feedbackFacetIssueComments, first, []FacetObservationInput{{SourceUpdatedAt: first, Payload: payload}}, true, 0); err != nil {
		t.Fatal(err)
	}
	for _, facet := range []string{feedbackFacetReviews, feedbackFacetInlineComments, feedbackFacetReviewThreads} {
		if err := c.ApplyFacetObservationSet(ctx, repo.ID, &pr.ID, facet, first, []FacetObservationInput{{SourceUpdatedAt: first, Payload: `{"coverage":{"complete":true},"items":[]}`}}, true, 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.UpsertFeedbackDiscovery(ctx, FeedbackDiscovery{RepositoryID: repo.ID, State: "all", NextPage: 1, Complete: true, DiscoveredPullRequests: 1, Channels: feedbackChannels, ThreadState: "all", SourceUpdatedAt: first}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RebuildPullRequestFeedbackProjection(ctx); err != nil {
		t.Fatal(err)
	}
	newer := first.Add(time.Hour)
	if err := c.AdvanceFacet(ctx, repo.ID, &pr.ID, feedbackFacetIssueComments, newer, false, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RebuildPullRequestFeedbackProjection(ctx); err != nil {
		t.Fatal(err)
	}
	page, err := c.SearchPullRequestFeedback(ctx, FeedbackSearchFilter{RepositoryID: repo.ID, Text: "old", Channel: "issue_comments", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Body != "old review" || page.Coverage.Status != "partial" || page.Coverage.IncompletePRs != 1 {
		t.Fatalf("preserved feedback page = %+v", page)
	}
}

func TestPullRequestFeedbackFiltersSortingAndContinuation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	repo, err := c.UpsertRepository(ctx, Repository{Owner: "acme", Name: "rocket", SourceUpdatedAt: now}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	type pullRequestCase struct {
		number      int
		state       string
		mergedKnown bool
		merged      bool
		feedbackID  int
		author      string
		body        string
	}
	cases := []pullRequestCase{
		{number: 1, state: "open", feedbackID: 101, author: "alice", body: "latency discussion"},
		{number: 2, state: "closed", mergedKnown: true, merged: true, feedbackID: 102, author: "alice", body: "merged fix"},
		{number: 3, state: "closed", mergedKnown: true, feedbackID: 103, author: "bob", body: "closed discussion"},
	}
	for index, value := range cases {
		at := now.Add(time.Duration(index) * time.Minute)
		thread, err := c.UpsertThread(ctx, Thread{RepositoryID: repo.ID, Kind: ThreadKindPullRequest, Number: value.number, State: value.state, Author: fmt.Sprintf("pr-author-%d", value.number), MergedKnown: value.mergedKnown, Merged: value.merged, SourceUpdatedAt: at}, `{}`)
		if err != nil {
			t.Fatal(err)
		}
		issuePayload := fmt.Sprintf(`{"coverage":{"complete":true},"items":[{"id":%d,"author":%q,"body":%q,"created_at":"2026-07-31T12:00:00Z"}]}`, value.feedbackID, value.author, value.body)
		if err := c.ApplyFacetObservationSet(ctx, repo.ID, &thread.ID, feedbackFacetIssueComments, at, []FacetObservationInput{{SourceUpdatedAt: at, Payload: issuePayload}}, true, 0); err != nil {
			t.Fatal(err)
		}
		for _, facet := range []string{feedbackFacetReviews, feedbackFacetInlineComments} {
			if err := c.ApplyFacetObservationSet(ctx, repo.ID, &thread.ID, facet, at, []FacetObservationInput{{SourceUpdatedAt: at, Payload: `{"coverage":{"complete":true},"items":[]}`}}, true, 0); err != nil {
				t.Fatal(err)
			}
		}
		threadItems := `[]`
		if value.number == 3 {
			threadItems = `[{"id":"thread-3","resolved":true,"comments":[{"id":303,"author":"reviewer","body":"resolved body","created_at":"2026-07-31T12:03:00Z"}]}]`
		}
		threadPayload := `{"coverage":{"complete":true},"items":` + threadItems + `}`
		if err := c.ApplyFacetObservationSet(ctx, repo.ID, &thread.ID, feedbackFacetReviewThreads, at, []FacetObservationInput{{SourceUpdatedAt: at, Payload: threadPayload}}, true, 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.UpsertFeedbackDiscovery(ctx, FeedbackDiscovery{RepositoryID: repo.ID, State: "all", NextPage: 1, Complete: true, DiscoveredPullRequests: 3, Channels: feedbackChannels, ThreadState: "all", SourceUpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RebuildPullRequestFeedbackProjection(ctx); err != nil {
		t.Fatal(err)
	}

	first, err := c.SearchPullRequestFeedback(ctx, FeedbackSearchFilter{RepositoryID: repo.ID, Sort: "feedback_author", Order: "asc", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if first.Total != 4 || len(first.Items) != 1 || first.Items[0].Author != "alice" || first.Items[0].PullRequestNumber != 1 || first.NextCursor == "" {
		t.Fatalf("first sorted page = %+v", first)
	}
	second, err := c.SearchPullRequestFeedback(ctx, FeedbackSearchFilter{RepositoryID: repo.ID, Sort: "feedback_author", Order: "asc", Limit: 1, Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Items[0].Author != "alice" || second.Items[0].PullRequestNumber != 2 {
		t.Fatalf("second sorted page = %+v", second)
	}

	filtered, err := c.SearchPullRequestFeedback(ctx, FeedbackSearchFilter{RepositoryID: repo.ID, State: "closed", Merged: "false", Text: "resolved body", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Items) != 1 || filtered.Items[0].Channel != "review_threads" || !filtered.Items[0].ResolvedKnown || !filtered.Items[0].Resolved {
		t.Fatalf("filtered feedback = %+v", filtered)
	}
	resolved, err := c.SearchPullRequestFeedback(ctx, FeedbackSearchFilter{RepositoryID: repo.ID, Channel: "review_threads", ThreadState: "resolved", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Items) != 1 || resolved.Items[0].ThreadExternalID != "thread-3" {
		t.Fatalf("resolved feedback = %+v", resolved)
	}
}
