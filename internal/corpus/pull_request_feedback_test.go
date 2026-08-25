package corpus

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

func mustFeedbackSearch(t *testing.T, repositoryID int64, input FeedbackSearchInput) FeedbackSearchRequest {
	t.Helper()
	query, err := ParseFeedbackSearchQuery(input)
	if err != nil {
		t.Fatalf("parse feedback search: %v", err)
	}
	request, err := query.InRepository(repositoryID)
	if err != nil {
		t.Fatalf("bind feedback search: %v", err)
	}
	return request
}

func mustFeedbackSelection(t *testing.T, channels []string, threadState string) FeedbackSelection {
	t.Helper()
	selection, err := ParseFeedbackSelection(channels, threadState)
	if err != nil {
		t.Fatalf("parse feedback selection: %v", err)
	}
	return selection
}

func TestNormalizeFeedbackPayloadPreservesBigIntCommentIDs(t *testing.T) {
	items, complete, err := normalizeFeedbackPayload(feedbackFacetReviewThreads, `{
		"head_sha":"abc",
		"coverage":{"complete":true},
		"items":[{"id":"thread-1","comments":[{
			"id":"9223372036854775808",
			"node_id":"PRRC_1",
			"in_reply_to_id":"9223372036854775807"
		}]}]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !complete || len(items) != 1 || items[0].FeedbackID != "9223372036854775808" || items[0].InReplyToID != "9223372036854775807" {
		t.Fatalf("normalized items = %+v, complete=%t", items, complete)
	}
}

func TestPullRequestFeedbackProjectionRebuildAndSearch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	now := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	repo, err := c.UpsertRepository(ctx, Repository{Owner: "acme", Name: "rocket", SourceUpdatedAt: now}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	pr, err := c.UpsertThread(ctx, Thread{RepositoryID: repo.ID, Kind: domain.PullRequestKind, Number: 7, State: "closed", Author: "submitter", Merge: domain.MergedStatus(time.Time{}), SourceUpdatedAt: now}, `{}`)
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
	if err := c.UpsertFeedbackDiscovery(ctx, FeedbackDiscovery{RepositoryID: repo.ID, NextPage: 1, State: FeedbackDiscoveryComplete, DiscoveredPullRequests: 1, Selection: AllFeedbackSelection(), SourceUpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RebuildPullRequestFeedbackProjection(ctx); err != nil {
		t.Fatal(err)
	}
	page, err := c.SearchPullRequestFeedback(ctx, mustFeedbackSearch(t, repo.ID, FeedbackSearchInput{Text: "latency", FeedbackAuthor: "alice", Merged: "true", State: "closed", Limit: 10}))
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].FeedbackID != "11" || page.Items[0].PullRequestNumber != 7 || !page.Items[0].PullRequestMerge.IsMerged() || !page.Coverage.Complete() {
		t.Fatalf("feedback page = %+v", page)
	}
	exact, err := c.SearchPullRequestFeedback(ctx, mustFeedbackSearch(t, repo.ID, FeedbackSearchInput{FeedbackAuthor: "reviewer", Limit: 10}))
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
	if _, err := c.SearchPullRequestFeedback(ctx, mustFeedbackSearch(t, repo.ID, FeedbackSearchInput{Limit: 10})); !errors.Is(err, ErrProjectionStale) {
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
	pr, err := c.UpsertThread(ctx, Thread{RepositoryID: repo.ID, Kind: domain.PullRequestKind, Number: 8, State: "open", SourceUpdatedAt: first}, `{}`)
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
	if err := c.UpsertFeedbackDiscovery(ctx, FeedbackDiscovery{RepositoryID: repo.ID, NextPage: 1, State: FeedbackDiscoveryComplete, DiscoveredPullRequests: 1, Selection: AllFeedbackSelection(), SourceUpdatedAt: first}); err != nil {
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
	page, err := c.SearchPullRequestFeedback(ctx, mustFeedbackSearch(t, repo.ID, FeedbackSearchInput{Text: "old", Channel: "issue_comments", Limit: 10}))
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Body != "old review" || page.Coverage.Status() != "partial" || page.Coverage.IncompletePRs != 1 {
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
	pr, err := c.UpsertThread(ctx, Thread{RepositoryID: repo.ID, Kind: domain.PullRequestKind, Number: 9, State: "open", SourceUpdatedAt: now}, `{}`)
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
	if err := c.UpsertFeedbackDiscovery(ctx, FeedbackDiscovery{RepositoryID: repo.ID, NextPage: 1, State: FeedbackDiscoveryComplete, DiscoveredPullRequests: 1, Selection: mustFeedbackSelection(t, AllFeedbackSelection().Channels(), "unresolved"), SourceUpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RebuildPullRequestFeedbackProjection(ctx); err != nil {
		t.Fatal(err)
	}

	unresolved, err := c.SearchPullRequestFeedback(ctx, mustFeedbackSearch(t, repo.ID, FeedbackSearchInput{Channel: "review_threads", ThreadState: "unresolved", Limit: 10}))
	if err != nil {
		t.Fatal(err)
	}
	if len(unresolved.Items) != 1 || !unresolved.Coverage.Complete() {
		t.Fatalf("unresolved search = %+v", unresolved)
	}

	all, err := c.SearchPullRequestFeedback(ctx, mustFeedbackSearch(t, repo.ID, FeedbackSearchInput{Channel: "review_threads", ThreadState: "all", Limit: 10}))
	if err != nil {
		t.Fatal(err)
	}
	if all.Coverage.Status() != "partial" || all.Coverage.IncompletePRs != 1 {
		t.Fatalf("all-thread coverage = %+v", all.Coverage)
	}

	resolved, err := c.SearchPullRequestFeedback(ctx, mustFeedbackSearch(t, repo.ID, FeedbackSearchInput{Channel: "review_threads", ThreadState: "resolved", Limit: 10}))
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Items) != 0 || resolved.Coverage.Status() != "partial" || resolved.Coverage.IncompletePRs != 1 {
		t.Fatalf("resolved-thread coverage = %+v", resolved)
	}
}

func TestPullRequestFeedbackCoverageDoesNotOverclaimUnselectedEmptyChannel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	now := time.Date(2026, 7, 31, 11, 30, 0, 0, time.UTC)
	repo, err := c.UpsertRepository(ctx, Repository{Owner: "acme", Name: "empty", SourceUpdatedAt: now}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	selection := mustFeedbackSelection(t, []string{"issue_comments"}, "all")
	if err := c.UpsertFeedbackDiscovery(ctx, FeedbackDiscovery{RepositoryID: repo.ID, NextPage: 1, State: FeedbackDiscoveryComplete, Selection: selection, SourceUpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RebuildPullRequestFeedbackProjection(ctx); err != nil {
		t.Fatal(err)
	}
	selected, err := c.SearchPullRequestFeedback(ctx, mustFeedbackSearch(t, repo.ID, FeedbackSearchInput{Channel: "issue_comments"}))
	if err != nil {
		t.Fatal(err)
	}
	if !selected.Coverage.Complete() || !selected.Coverage.DiscoveryComplete() {
		t.Fatalf("selected channel coverage = %+v", selected.Coverage)
	}
	unselected, err := c.SearchPullRequestFeedback(ctx, mustFeedbackSearch(t, repo.ID, FeedbackSearchInput{Channel: "submitted_reviews"}))
	if err != nil {
		t.Fatal(err)
	}
	if unselected.Coverage.Status() != "partial" || unselected.Coverage.DiscoveryComplete() {
		t.Fatalf("unselected channel coverage = %+v", unselected.Coverage)
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
	if err := c.UpsertFeedbackDiscovery(ctx, FeedbackDiscovery{RepositoryID: repo.ID, Generation: 1, NextPage: 4, Selection: AllFeedbackSelection(), SourceUpdatedAt: first}); err != nil {
		t.Fatal(err)
	}
	if err := c.UpsertFeedbackDiscovery(ctx, FeedbackDiscovery{RepositoryID: repo.ID, Generation: 1, NextPage: 2, State: FeedbackDiscoveryComplete, Selection: AllFeedbackSelection(), SourceUpdatedAt: first.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	got, err := c.GetFeedbackDiscovery(ctx, repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Generation != 1 || got.NextPage != 4 || got.IsComplete() {
		t.Fatalf("discovery checkpoint = %+v, want page 4 incomplete", got)
	}
	if err := c.UpsertFeedbackDiscovery(ctx, FeedbackDiscovery{RepositoryID: repo.ID, Generation: 2, NextPage: 1, State: FeedbackDiscoveryComplete, Selection: AllFeedbackSelection(), SourceUpdatedAt: first.Add(2 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	got, err = c.GetFeedbackDiscovery(ctx, repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Generation != 2 || got.NextPage != 1 || !got.IsComplete() {
		t.Fatalf("new discovery generation = %+v, want page 1 complete", got)
	}
}

func TestFeedbackDiscoveryRejectsImpossibleState(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	repo, err := c.UpsertRepository(ctx, Repository{Owner: "acme", Name: "state", SourceUpdatedAt: now}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	invalid := FeedbackDiscovery{
		RepositoryID: repo.ID, State: "impossible", Selection: AllFeedbackSelection(), SourceUpdatedAt: now,
	}
	if err := c.UpsertFeedbackDiscovery(ctx, invalid); err == nil {
		t.Fatal("unsupported in-memory discovery state was accepted")
	}
	valid := FeedbackDiscovery{
		RepositoryID: repo.ID, State: FeedbackDiscoveryComplete, Selection: AllFeedbackSelection(), SourceUpdatedAt: now,
	}
	if err := c.UpsertFeedbackDiscovery(ctx, valid); err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.ExecContext(ctx, `UPDATE pull_request_feedback_discovery SET complete=1, truncated=1 WHERE repository_id=?`, repo.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetFeedbackDiscovery(ctx, repo.ID); err == nil {
		t.Fatal("contradictory stored discovery flags were accepted")
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
		state       domain.ThreadState
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
		merge := domain.UnknownMergeStatus()
		if value.mergedKnown {
			merge = domain.UnmergedStatus()
			if value.merged {
				merge = domain.MergedStatus(time.Time{})
			}
		}
		thread, err := c.UpsertThread(ctx, Thread{RepositoryID: repo.ID, Kind: domain.PullRequestKind, Number: value.number, State: value.state, Author: fmt.Sprintf("pr-author-%d", value.number), Merge: merge, SourceUpdatedAt: at}, `{}`)
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
	if err := c.UpsertFeedbackDiscovery(ctx, FeedbackDiscovery{RepositoryID: repo.ID, NextPage: 1, State: FeedbackDiscoveryComplete, DiscoveredPullRequests: 3, Selection: AllFeedbackSelection(), SourceUpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RebuildPullRequestFeedbackProjection(ctx); err != nil {
		t.Fatal(err)
	}

	first, err := c.SearchPullRequestFeedback(ctx, mustFeedbackSearch(t, repo.ID, FeedbackSearchInput{Sort: "feedback_author", Order: "asc", Limit: 1}))
	if err != nil {
		t.Fatal(err)
	}
	if first.Total != 4 || len(first.Items) != 1 || first.Items[0].Author != "alice" || first.Items[0].PullRequestNumber != 1 || first.NextCursor == "" {
		t.Fatalf("first sorted page = %+v", first)
	}
	second, err := c.SearchPullRequestFeedback(ctx, mustFeedbackSearch(t, repo.ID, FeedbackSearchInput{Sort: "feedback_author", Order: "asc", Limit: 1, Cursor: first.NextCursor}))
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Items[0].Author != "alice" || second.Items[0].PullRequestNumber != 2 {
		t.Fatalf("second sorted page = %+v", second)
	}

	filtered, err := c.SearchPullRequestFeedback(ctx, mustFeedbackSearch(t, repo.ID, FeedbackSearchInput{State: "closed", Merged: "false", Text: "resolved body", Limit: 10}))
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Items) != 1 {
		t.Fatalf("filtered feedback = %+v", filtered)
	}
	resolved, known := filtered.Items[0].Resolution.Value()
	if filtered.Items[0].Channel != "review_threads" || !known || !resolved {
		t.Fatalf("filtered feedback = %+v", filtered)
	}
	unknown, err := c.SearchPullRequestFeedback(ctx, mustFeedbackSearch(t, repo.ID, FeedbackSearchInput{Merged: "true", Text: "latency discussion", Limit: 10}))
	if err != nil {
		t.Fatal(err)
	}
	if len(unknown.Items) != 0 || len(unknown.UnknownMergePullRequests) != 1 || unknown.UnknownMergePullRequests[0] != 1 {
		t.Fatalf("unknown merge candidates = %+v", unknown)
	}
	resolvedPage, err := c.SearchPullRequestFeedback(ctx, mustFeedbackSearch(t, repo.ID, FeedbackSearchInput{Channel: "review_threads", ThreadState: "resolved", Limit: 10}))
	if err != nil {
		t.Fatal(err)
	}
	if len(resolvedPage.Items) != 1 || resolvedPage.Items[0].ThreadExternalID != "thread-3" {
		t.Fatalf("resolved feedback = %+v", resolvedPage)
	}
}
