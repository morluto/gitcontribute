package health

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/github"
)

func openTestCorpus(t *testing.T) *corpus.Corpus {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "corpus.db")
	c, err := corpus.Open(ctx, path)
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func upsertThread(t *testing.T, ctx context.Context, c *corpus.Corpus, repoID int64, thread corpus.Thread, authorAssoc string) *corpus.Thread {
	t.Helper()
	payload, err := json.Marshal(github.Issue{Author: thread.Author, AuthorAssociation: authorAssoc})
	if err != nil {
		t.Fatalf("marshal thread payload: %v", err)
	}
	thread.RepositoryID = repoID
	thread.AuthorAssociation = authorAssoc
	out, err := c.UpsertThread(ctx, thread, string(payload))
	if err != nil {
		t.Fatalf("upsert thread: %v", err)
	}
	return out
}

func applyFacet(t *testing.T, ctx context.Context, c *corpus.Corpus, repoID, threadID int64, facet string, sourceUpdated time.Time, payload any, complete bool) {
	t.Helper()
	var pages []corpus.FacetObservationInput
	if payload != nil {
		payloadJSON, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal facet payload: %v", err)
		}
		pages = []corpus.FacetObservationInput{{SourceUpdatedAt: sourceUpdated, Payload: string(payloadJSON)}}
	}
	if err := c.ApplyFacetObservationSet(ctx, repoID, &threadID, facet, sourceUpdated, pages, complete, 0); err != nil {
		t.Fatalf("apply facet %s: %v", facet, err)
	}
}

func TestComputeHealthMetrics(t *testing.T) {
	ctx := context.Background()
	c := openTestCorpus(t)

	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	start := now.Add(-30 * 24 * time.Hour)

	repo, err := c.UpsertRepository(ctx, corpus.Repository{
		Owner:           "owner",
		Name:            "repo",
		SourceCreatedAt: now.Add(-365 * 24 * time.Hour),
		SourceUpdatedAt: now,
	}, "{}")
	if err != nil {
		t.Fatalf("upsert repository: %v", err)
	}

	openIssue := upsertThread(t, ctx, c, repo.ID, corpus.Thread{
		Kind:            corpus.ThreadKindIssue,
		Number:          1,
		State:           "open",
		Title:           "open issue",
		Author:          "carol",
		SourceCreatedAt: now.Add(-6 * 24 * time.Hour),
		SourceUpdatedAt: now.Add(-1 * time.Hour),
	}, "NONE")

	closedIssue := upsertThread(t, ctx, c, repo.ID, corpus.Thread{
		Kind:            corpus.ThreadKindIssue,
		Number:          2,
		State:           "closed",
		Title:           "closed issue",
		Author:          "dave",
		SourceCreatedAt: now.Add(-20 * 24 * time.Hour),
		SourceUpdatedAt: now.Add(-5 * 24 * time.Hour),
		ClosedAt:        now.Add(-5 * 24 * time.Hour),
	}, "CONTRIBUTOR")

	_ = upsertThread(t, ctx, c, repo.ID, corpus.Thread{
		Kind: corpus.ThreadKindIssue, Number: 3, State: "closed", Title: "outside window",
		Author: "old", SourceCreatedAt: now.Add(-60 * 24 * time.Hour), SourceUpdatedAt: now.Add(-50 * 24 * time.Hour),
	}, "CONTRIBUTOR")

	openPR := upsertThread(t, ctx, c, repo.ID, corpus.Thread{
		Kind:            corpus.ThreadKindPullRequest,
		Number:          10,
		State:           "open",
		Title:           "open pr",
		Author:          "alice",
		SourceCreatedAt: now.Add(-2 * 24 * time.Hour),
		SourceUpdatedAt: now.Add(-24 * time.Hour),
	}, "NONE")

	_ = upsertThread(t, ctx, c, repo.ID, corpus.Thread{
		Kind:            corpus.ThreadKindPullRequest,
		Number:          11,
		State:           "open",
		Title:           "stale pr",
		Author:          "bob",
		SourceCreatedAt: now.Add(-20 * 24 * time.Hour),
		SourceUpdatedAt: now.Add(-15 * 24 * time.Hour),
	}, "FIRST_TIMER")

	_ = upsertThread(t, ctx, c, repo.ID, corpus.Thread{
		Kind:            corpus.ThreadKindPullRequest,
		Number:          12,
		State:           "closed",
		Title:           "merged pr",
		Author:          "owner1",
		SourceCreatedAt: now.Add(-12 * 24 * time.Hour),
		SourceUpdatedAt: now.Add(-3 * 24 * time.Hour),
		MergedAt:        now.Add(-3 * 24 * time.Hour),
		Merged:          true,
	}, "OWNER")

	_ = upsertThread(t, ctx, c, repo.ID, corpus.Thread{
		Kind:            corpus.ThreadKindPullRequest,
		Number:          13,
		State:           "closed",
		Title:           "closed pr",
		Author:          "frank",
		SourceCreatedAt: now.Add(-8 * 24 * time.Hour),
		SourceUpdatedAt: now.Add(-2 * 24 * time.Hour),
		ClosedAt:        now.Add(-2 * 24 * time.Hour),
		MergedKnown:     true,
	}, "NONE")

	_ = upsertThread(t, ctx, c, repo.ID, corpus.Thread{
		Kind:            corpus.ThreadKindPullRequest,
		Number:          14,
		State:           "closed",
		Title:           "header-only closed pr",
		Author:          "grace",
		SourceCreatedAt: now.Add(-7 * 24 * time.Hour),
		SourceUpdatedAt: now.Add(-time.Hour),
		ClosedAt:        now.Add(-time.Hour),
	}, "NONE")

	// Facets for response times and staleness.
	applyFacet(t, ctx, c, repo.ID, openIssue.ID, facetIssueComments, now.Add(-6*24*time.Hour).Add(time.Hour), []github.IssueComment{
		{ID: 1, Author: "maintainer", AuthorAssociation: "MEMBER", CreatedAt: now.Add(-6 * 24 * time.Hour).Add(time.Hour), UpdatedAt: now.Add(-6 * 24 * time.Hour).Add(time.Hour)},
	}, true)
	applyFacet(t, ctx, c, repo.ID, closedIssue.ID, facetIssueComments, now.Add(-20*24*time.Hour).Add(2*time.Hour), []github.IssueComment{
		{ID: 2, Author: "reviewer", AuthorAssociation: "COLLABORATOR", CreatedAt: now.Add(-20 * 24 * time.Hour).Add(2 * time.Hour), UpdatedAt: now.Add(-20 * 24 * time.Hour).Add(2 * time.Hour)},
	}, true)
	applyFacet(t, ctx, c, repo.ID, openPR.ID, facetPRReviews, now.Add(-2*24*time.Hour).Add(30*time.Minute), []github.Review{
		{ID: 3, Author: "reviewer", AuthorAssociation: "COLLABORATOR", SubmittedAt: now.Add(-2 * 24 * time.Hour).Add(30 * time.Minute), State: "COMMENTED"},
	}, true)
	// Empty complete response facets let the open PR count as fully covered.
	applyFacet(t, ctx, c, repo.ID, openPR.ID, facetIssueComments, now.Add(-2*24*time.Hour).Add(time.Hour), nil, true)
	applyFacet(t, ctx, c, repo.ID, openPR.ID, facetPRReviewComments, now.Add(-2*24*time.Hour).Add(time.Hour), nil, true)

	opts := Options{
		Now:            now,
		Start:          start,
		End:            now,
		StaleThreshold: 14 * 24 * time.Hour,
	}

	report, err := Compute(ctx, c, repo.ID, opts)
	if err != nil {
		t.Fatalf("compute health: %v", err)
	}

	if report.Issues.Open != 1 || report.Issues.Closed != 1 || report.Issues.SampleSize != 2 {
		t.Fatalf("issue metrics = %+v, want open=1 closed=1", report.Issues)
	}
	if report.PullRequests.Open != 2 || report.PullRequests.Merged != 1 || report.PullRequests.ClosedUnmerged != 1 || report.PullRequests.ClosedUnknownMerge != 1 || report.PullRequests.SampleSize != 5 {
		t.Fatalf("pr metrics = %+v", report.PullRequests)
	}

	if report.External.External != 4 || report.External.Known != 5 || report.External.Open != 2 || report.External.ClosedUnmerged != 1 || report.External.ClosedUnknownMerge != 1 || report.External.Merged != 0 {
		t.Fatalf("external metrics = %+v", report.External)
	}
	if report.External.MergeRate != 0 {
		t.Fatalf("external merge rate = %v, want 0", report.External.MergeRate)
	}
	if report.PullRequests.Coverage != "partial (some closed pull requests lack an observed merge state)" || report.External.Coverage != "partial (some closed PRs lack an observed merge state)" {
		t.Fatalf("merge-state coverage not surfaced: pull_requests=%q external=%q", report.PullRequests.Coverage, report.External.Coverage)
	}
	if report.Issues.Coverage != "complete" {
		t.Fatalf("PR merge-state gap leaked into issue coverage: %q", report.Issues.Coverage)
	}

	if report.Congestion.OpenPRs != 2 || report.Congestion.SampleSize != 2 {
		t.Fatalf("congestion = %+v", report.Congestion)
	}
	if len(report.Congestion.AgeBuckets) != 3 {
		t.Fatalf("expected 3 age buckets, got %d", len(report.Congestion.AgeBuckets))
	}

	if report.Stale.SampleSize != 2 || report.Stale.StaleCount != 1 || report.Stale.ActiveCount != 1 {
		t.Fatalf("stale signals = %+v", report.Stale)
	}

	if report.Response.Issues.SampleSize != 2 {
		t.Fatalf("issue response samples = %d, want 2", report.Response.Issues.SampleSize)
	}
	if report.Response.PullRequests.SampleSize != 1 {
		t.Fatalf("pr response samples = %d, want 1", report.Response.PullRequests.SampleSize)
	}
	if report.Response.Issues.Median != 1.5 { // average of 1h and 2h response times: median = 1.5h
		t.Fatalf("issue response median = %v, want 1.5", report.Response.Issues.Median)
	}
	if report.Response.PullRequests.Median != 0.5 {
		t.Fatalf("pr response median = %v, want 0.5", report.Response.PullRequests.Median)
	}
}

func TestResponseRequiredFacetCoverage(t *testing.T) {
	ctx := context.Background()
	c := openTestCorpus(t)

	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	start := now.Add(-30 * 24 * time.Hour)

	repo, err := c.UpsertRepository(ctx, corpus.Repository{
		Owner:           "owner",
		Name:            "repo",
		SourceCreatedAt: now.Add(-365 * 24 * time.Hour),
		SourceUpdatedAt: now,
	}, "{}")
	if err != nil {
		t.Fatalf("upsert repository: %v", err)
	}

	issue := upsertThread(t, ctx, c, repo.ID, corpus.Thread{
		Kind:            corpus.ThreadKindIssue,
		Number:          1,
		State:           "open",
		Title:           "issue",
		Author:          "alice",
		SourceCreatedAt: now.Add(-7 * 24 * time.Hour),
		SourceUpdatedAt: now,
	}, "NONE")

	opts := Options{
		Now:            now,
		Start:          start,
		End:            now,
		StaleThreshold: 14 * 24 * time.Hour,
	}

	// A complete empty facet counts as covered and contributes no response.
	applyFacet(t, ctx, c, repo.ID, issue.ID, facetIssueComments, now.Add(-6*24*time.Hour), nil, true)
	report, err := Compute(ctx, c, repo.ID, opts)
	if err != nil {
		t.Fatalf("compute health: %v", err)
	}
	if report.Response.Issues.SampleSize != 0 {
		t.Fatalf("empty complete: sample size = %d, want 0", report.Response.Issues.SampleSize)
	}
	if report.Response.Issues.Coverage != "complete" {
		t.Fatalf("empty complete: coverage = %q, want complete", report.Response.Issues.Coverage)
	}

	// An incomplete facet with observations does not count; observations are ignored.
	created := issue.SourceCreatedAt
	applyFacet(t, ctx, c, repo.ID, issue.ID, facetIssueComments, now.Add(-5*24*time.Hour), []github.IssueComment{
		{ID: 1, Author: "bob", CreatedAt: created.Add(time.Hour), UpdatedAt: created.Add(time.Hour)},
	}, false)
	report, err = Compute(ctx, c, repo.ID, opts)
	if err != nil {
		t.Fatalf("compute health: %v", err)
	}
	if report.Response.Issues.SampleSize != 0 {
		t.Fatalf("nonempty incomplete: sample size = %d, want 0", report.Response.Issues.SampleSize)
	}
	if report.Response.Issues.Coverage != "partial" {
		t.Fatalf("nonempty incomplete: coverage = %q, want partial", report.Response.Issues.Coverage)
	}

	// A complete nonempty facet yields a response sample and complete coverage.
	applyFacet(t, ctx, c, repo.ID, issue.ID, facetIssueComments, now.Add(-4*24*time.Hour), []github.IssueComment{
		{ID: 2, Author: "bob", CreatedAt: created.Add(2 * time.Hour), UpdatedAt: created.Add(2 * time.Hour)},
	}, true)
	report, err = Compute(ctx, c, repo.ID, opts)
	if err != nil {
		t.Fatalf("compute health: %v", err)
	}
	if report.Response.Issues.SampleSize != 1 {
		t.Fatalf("complete nonempty: sample size = %d, want 1", report.Response.Issues.SampleSize)
	}
	if report.Response.Issues.Median != 2.0 {
		t.Fatalf("complete nonempty: median = %v, want 2.0", report.Response.Issues.Median)
	}
	if report.Response.Issues.Coverage != "complete" {
		t.Fatalf("complete nonempty: coverage = %q, want complete", report.Response.Issues.Coverage)
	}
}

func TestResponsePullRequestPartialFacetCoverage(t *testing.T) {
	ctx := context.Background()
	c := openTestCorpus(t)

	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	start := now.Add(-30 * 24 * time.Hour)

	repo, err := c.UpsertRepository(ctx, corpus.Repository{
		Owner:           "owner",
		Name:            "repo",
		SourceCreatedAt: now.Add(-365 * 24 * time.Hour),
		SourceUpdatedAt: now,
	}, "{}")
	if err != nil {
		t.Fatalf("upsert repository: %v", err)
	}

	pr := upsertThread(t, ctx, c, repo.ID, corpus.Thread{
		Kind:            corpus.ThreadKindPullRequest,
		Number:          1,
		State:           "open",
		Title:           "pr",
		Author:          "alice",
		SourceCreatedAt: now.Add(-7 * 24 * time.Hour),
		SourceUpdatedAt: now,
	}, "NONE")

	opts := Options{
		Now:            now,
		Start:          start,
		End:            now,
		StaleThreshold: 14 * 24 * time.Hour,
	}

	prCreated := pr.SourceCreatedAt
	applyFacet(t, ctx, c, repo.ID, pr.ID, facetPRReviews, now.Add(-6*24*time.Hour), []github.Review{
		{ID: 1, Author: "reviewer", SubmittedAt: prCreated.Add(time.Hour), State: "COMMENTED"},
	}, true)

	// Only pr_reviews is complete; the PR should not contribute a response sample.
	report, err := Compute(ctx, c, repo.ID, opts)
	if err != nil {
		t.Fatalf("compute health: %v", err)
	}
	if report.Response.PullRequests.SampleSize != 0 {
		t.Fatalf("partial coverage: sample size = %d, want 0", report.Response.PullRequests.SampleSize)
	}
	if report.Response.PullRequests.Coverage != "partial" {
		t.Fatalf("partial coverage: coverage = %q, want partial", report.Response.PullRequests.Coverage)
	}

	// Completing the remaining empty facets makes the PR fully covered.
	applyFacet(t, ctx, c, repo.ID, pr.ID, facetIssueComments, now.Add(-5*24*time.Hour), nil, true)
	applyFacet(t, ctx, c, repo.ID, pr.ID, facetPRReviewComments, now.Add(-5*24*time.Hour), nil, true)

	report, err = Compute(ctx, c, repo.ID, opts)
	if err != nil {
		t.Fatalf("compute health: %v", err)
	}
	if report.Response.PullRequests.SampleSize != 1 {
		t.Fatalf("complete coverage: sample size = %d, want 1", report.Response.PullRequests.SampleSize)
	}
	if report.Response.PullRequests.Median != 1.0 {
		t.Fatalf("complete coverage: median = %v, want 1.0", report.Response.PullRequests.Median)
	}
	if report.Response.PullRequests.Coverage != "complete" {
		t.Fatalf("complete coverage: coverage = %q, want complete", report.Response.PullRequests.Coverage)
	}
}

func TestResponseSelfCommentCaseInsensitive(t *testing.T) {
	ctx := context.Background()
	c := openTestCorpus(t)

	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	start := now.Add(-30 * 24 * time.Hour)

	repo, err := c.UpsertRepository(ctx, corpus.Repository{
		Owner:           "owner",
		Name:            "repo",
		SourceCreatedAt: now.Add(-365 * 24 * time.Hour),
		SourceUpdatedAt: now,
	}, "{}")
	if err != nil {
		t.Fatalf("upsert repository: %v", err)
	}

	issue := upsertThread(t, ctx, c, repo.ID, corpus.Thread{
		Kind:            corpus.ThreadKindIssue,
		Number:          1,
		State:           "open",
		Title:           "issue",
		Author:          "alice",
		SourceCreatedAt: now.Add(-7 * 24 * time.Hour),
		SourceUpdatedAt: now,
	}, "NONE")

	created := issue.SourceCreatedAt
	applyFacet(t, ctx, c, repo.ID, issue.ID, facetIssueComments, now.Add(-6*24*time.Hour), []github.IssueComment{
		{ID: 1, Author: "ALICE", CreatedAt: created.Add(30 * time.Minute), UpdatedAt: created.Add(30 * time.Minute)},
		{ID: 2, Author: "Bob", CreatedAt: created.Add(time.Hour), UpdatedAt: created.Add(time.Hour)},
		{ID: 3, Author: "alice", CreatedAt: created.Add(2 * time.Hour), UpdatedAt: created.Add(2 * time.Hour)},
	}, true)

	opts := Options{
		Now:            now,
		Start:          start,
		End:            now,
		StaleThreshold: 14 * 24 * time.Hour,
	}

	report, err := Compute(ctx, c, repo.ID, opts)
	if err != nil {
		t.Fatalf("compute health: %v", err)
	}
	if report.Response.Issues.SampleSize != 1 {
		t.Fatalf("issue response samples = %d, want 1", report.Response.Issues.SampleSize)
	}
	if report.Response.Issues.Median != 1.0 {
		t.Fatalf("issue response median = %v, want 1.0", report.Response.Issues.Median)
	}

	// PR reviews should also ignore same-login self-comments with different case.
	pr := upsertThread(t, ctx, c, repo.ID, corpus.Thread{
		Kind:            corpus.ThreadKindPullRequest,
		Number:          2,
		State:           "open",
		Title:           "pr",
		Author:          "bob",
		SourceCreatedAt: now.Add(-7 * 24 * time.Hour),
		SourceUpdatedAt: now,
	}, "NONE")

	prCreated := pr.SourceCreatedAt
	applyFacet(t, ctx, c, repo.ID, pr.ID, facetPRReviews, now.Add(-6*24*time.Hour), []github.Review{
		{ID: 10, Author: "BOB", SubmittedAt: prCreated.Add(30 * time.Minute), State: "COMMENTED"},
		{ID: 11, Author: "charlie", SubmittedAt: prCreated.Add(90 * time.Minute), State: "COMMENTED"},
		{ID: 12, Author: "Bob", SubmittedAt: prCreated.Add(2 * time.Hour), State: "COMMENTED"},
	}, true)
	applyFacet(t, ctx, c, repo.ID, pr.ID, facetIssueComments, now.Add(-6*24*time.Hour), nil, true)
	applyFacet(t, ctx, c, repo.ID, pr.ID, facetPRReviewComments, now.Add(-6*24*time.Hour), nil, true)

	report, err = Compute(ctx, c, repo.ID, opts)
	if err != nil {
		t.Fatalf("compute health: %v", err)
	}
	if report.Response.PullRequests.SampleSize != 1 {
		t.Fatalf("pr response samples = %d, want 1", report.Response.PullRequests.SampleSize)
	}
	if report.Response.PullRequests.Median != 1.5 {
		t.Fatalf("pr response median = %v, want 1.5", report.Response.PullRequests.Median)
	}
}
