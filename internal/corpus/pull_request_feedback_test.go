package corpus

import (
	"context"
	"errors"
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
	if err := c.ApplyFacetObservationSet(ctx, repo.ID, &pr.ID, feedbackFacetReviews, now, []FacetObservationInput{{SourceUpdatedAt: now, Payload: completePayload(`[{"id":12,"node_id":"PRR_node","author":"reviewer","body":"review body","state":"CHANGES_REQUESTED","submitted_at":"2026-07-31T10:03:00Z"}]`)}}, true, 0); err != nil {
		t.Fatal(err)
	}
	if err := c.ApplyFacetObservationSet(ctx, repo.ID, &pr.ID, feedbackFacetInlineComments, now, []FacetObservationInput{{SourceUpdatedAt: now, Payload: completePayload(`[{"id":13,"node_id":"PRC_node","in_reply_to_id":11,"author":"reviewer","body":"inline body","path":"main.go","line":9,"start_line":7,"side":"RIGHT","start_side":"RIGHT","created_at":"2026-07-31T10:04:00Z"}]`)}}, true, 0); err != nil {
		t.Fatal(err)
	}
	if err := c.ApplyFacetObservationSet(ctx, repo.ID, &pr.ID, feedbackFacetReviewThreads, now, []FacetObservationInput{{SourceUpdatedAt: now, Payload: completePayload(`[{"id":"thread-7","resolved":true,"resolved_by":"maintainer","path":"main.go","line":12,"comments":[{"id":14,"node_id":"PRT_node","in_reply_to_id":13,"author":"reviewer","body":"thread body","created_at":"2026-07-31T10:05:00Z"}]}]`)}}, true, 0); err != nil {
		t.Fatal(err)
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
	exact, err := c.SearchPullRequestFeedback(ctx, FeedbackSearchFilter{RepositoryID: repo.ID, FeedbackAuthor: "reviewer", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(exact.Items) != 3 {
		t.Fatalf("exact author feedback = %+v", exact.Items)
	}
	byChannel := make(map[string]PullRequestFeedbackProjection, len(exact.Items))
	for _, item := range exact.Items {
		byChannel[item.Channel] = item
	}
	if byChannel["submitted_reviews"].FeedbackNodeID != "PRR_node" || byChannel["submitted_reviews"].ReviewState != "CHANGES_REQUESTED" {
		t.Fatalf("review identity/state = %+v", byChannel["submitted_reviews"])
	}
	if byChannel["inline_comments"].FeedbackNodeID != "PRC_node" || byChannel["inline_comments"].InReplyToID != "11" || byChannel["inline_comments"].Side != "RIGHT" || byChannel["inline_comments"].StartLine == nil || *byChannel["inline_comments"].StartLine != 7 {
		t.Fatalf("inline identity/anchor = %+v", byChannel["inline_comments"])
	}
	if byChannel["review_threads"].ThreadExternalID != "thread-7" || byChannel["review_threads"].ResolvedBy != "maintainer" || byChannel["review_threads"].InReplyToID != "13" {
		t.Fatalf("thread identity/state = %+v", byChannel["review_threads"])
	}
	if err := c.ApplyFacetObservationSet(ctx, repo.ID, &pr.ID, feedbackFacetIssueComments, now.Add(time.Hour), []FacetObservationInput{{SourceUpdatedAt: now.Add(time.Hour), Payload: completePayload(`[{"id":12,"author":"alice","body":"new feedback"}]`)}}, true, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SearchPullRequestFeedback(ctx, FeedbackSearchFilter{RepositoryID: repo.ID, Limit: 10}); !errors.Is(err, ErrProjectionStale) {
		t.Fatalf("search after raw feedback replacement error = %v, want ErrProjectionStale", err)
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

func TestPullRequestFeedbackCoverageRespectsThreadSelection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	now := time.Date(2026, 7, 31, 11, 0, 0, 0, time.UTC)
	repo, err := c.UpsertRepository(ctx, Repository{Owner: "acme", Name: "rocket", SourceUpdatedAt: now}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	pr, err := c.UpsertThread(ctx, Thread{RepositoryID: repo.ID, Kind: ThreadKindPullRequest, Number: 9, State: "open", SourceUpdatedAt: now}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	threadPayload := `{"selection":"unresolved","coverage":{"complete":true},"items":[{"id":"thread-9","resolved":false,"comments":[{"id":909,"author":"reviewer","body":"unresolved feedback"}]}]}`
	if err := c.ApplyFacetObservationSet(ctx, repo.ID, &pr.ID, feedbackFacetReviewThreads, now, []FacetObservationInput{{SourceUpdatedAt: now, Payload: threadPayload}}, true, 0); err != nil {
		t.Fatal(err)
	}
	for _, facet := range []string{feedbackFacetIssueComments, feedbackFacetReviews, feedbackFacetInlineComments} {
		if err := c.ApplyFacetObservationSet(ctx, repo.ID, &pr.ID, facet, now, []FacetObservationInput{{SourceUpdatedAt: now, Payload: `{"coverage":{"complete":true},"items":[]}`}}, true, 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.UpsertFeedbackDiscovery(ctx, FeedbackDiscovery{RepositoryID: repo.ID, State: "all", NextPage: 1, Complete: true, DiscoveredPullRequests: 1, Channels: feedbackChannels, ThreadState: "unresolved", SourceUpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RebuildPullRequestFeedbackProjection(ctx); err != nil {
		t.Fatal(err)
	}

	unresolved, err := c.SearchPullRequestFeedback(ctx, FeedbackSearchFilter{RepositoryID: repo.ID, Channel: "review_threads", ThreadState: "unresolved", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(unresolved.Items) != 1 || unresolved.Coverage.Status != "complete" {
		t.Fatalf("unresolved search = %+v", unresolved)
	}

	all, err := c.SearchPullRequestFeedback(ctx, FeedbackSearchFilter{RepositoryID: repo.ID, Channel: "review_threads", ThreadState: "all", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if all.Coverage.Status != "partial" || all.Coverage.IncompletePRs != 1 {
		t.Fatalf("all-thread coverage = %+v", all.Coverage)
	}

	resolved, err := c.SearchPullRequestFeedback(ctx, FeedbackSearchFilter{RepositoryID: repo.ID, Channel: "review_threads", ThreadState: "resolved", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Items) != 0 || resolved.Coverage.Status != "partial" || resolved.Coverage.IncompletePRs != 1 {
		t.Fatalf("resolved-thread coverage = %+v", resolved)
	}
}

func TestFeedbackDiscoveryDoesNotRegressCheckpoint(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	first := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	repo, err := c.UpsertRepository(ctx, Repository{Owner: "acme", Name: "checkpoint", SourceUpdatedAt: first}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.UpsertFeedbackDiscovery(ctx, FeedbackDiscovery{RepositoryID: repo.ID, Generation: 1, State: "all", NextPage: 4, Channels: feedbackChannels, ThreadState: "all", SourceUpdatedAt: first}); err != nil {
		t.Fatal(err)
	}
	if err := c.UpsertFeedbackDiscovery(ctx, FeedbackDiscovery{RepositoryID: repo.ID, Generation: 1, State: "all", NextPage: 2, Complete: true, Channels: feedbackChannels, ThreadState: "all", SourceUpdatedAt: first.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	got, err := c.GetFeedbackDiscovery(ctx, repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Generation != 1 || got.NextPage != 4 || got.Complete {
		t.Fatalf("discovery checkpoint = %+v, want page 4 incomplete", got)
	}
	if err := c.UpsertFeedbackDiscovery(ctx, FeedbackDiscovery{RepositoryID: repo.ID, Generation: 2, State: "all", NextPage: 1, Complete: true, Channels: feedbackChannels, ThreadState: "all", SourceUpdatedAt: first.Add(2 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	got, err = c.GetFeedbackDiscovery(ctx, repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Generation != 2 || got.NextPage != 1 || !got.Complete {
		t.Fatalf("new discovery generation = %+v, want page 1 complete", got)
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
	unknown, err := c.SearchPullRequestFeedback(ctx, FeedbackSearchFilter{RepositoryID: repo.ID, Merged: "true", Text: "latency discussion", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(unknown.Items) != 0 || len(unknown.UnknownMergePullRequests) != 1 || unknown.UnknownMergePullRequests[0] != 1 {
		t.Fatalf("unknown merge candidates = %+v", unknown)
	}
	resolved, err := c.SearchPullRequestFeedback(ctx, FeedbackSearchFilter{RepositoryID: repo.ID, Channel: "review_threads", ThreadState: "resolved", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Items) != 1 || resolved.Items[0].ThreadExternalID != "thread-3" {
		t.Fatalf("resolved feedback = %+v", resolved)
	}
}
