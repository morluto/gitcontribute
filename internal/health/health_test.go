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
	out, err := c.UpsertThread(ctx, thread, string(payload))
	if err != nil {
		t.Fatalf("upsert thread: %v", err)
	}
	return out
}

func applyFacet(t *testing.T, ctx context.Context, c *corpus.Corpus, repoID, threadID int64, facet string, sourceUpdated time.Time, payload any) {
	t.Helper()
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal facet payload: %v", err)
	}
	if err := c.ApplyFacetObservationSet(ctx, repoID, &threadID, facet, []corpus.FacetObservationInput{{SourceUpdatedAt: sourceUpdated, Payload: string(payloadJSON)}}, true, 0); err != nil {
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
	}, "NONE")

	// Facets for response times and staleness.
	applyFacet(t, ctx, c, repo.ID, openIssue.ID, facetIssueComments, now.Add(-6*24*time.Hour).Add(time.Hour), []github.IssueComment{
		{ID: 1, Author: "maintainer", AuthorAssociation: "MEMBER", CreatedAt: now.Add(-6 * 24 * time.Hour).Add(time.Hour), UpdatedAt: now.Add(-6 * 24 * time.Hour).Add(time.Hour)},
	})
	applyFacet(t, ctx, c, repo.ID, closedIssue.ID, facetIssueComments, now.Add(-20*24*time.Hour).Add(2*time.Hour), []github.IssueComment{
		{ID: 2, Author: "reviewer", AuthorAssociation: "COLLABORATOR", CreatedAt: now.Add(-20 * 24 * time.Hour).Add(2 * time.Hour), UpdatedAt: now.Add(-20 * 24 * time.Hour).Add(2 * time.Hour)},
	})
	applyFacet(t, ctx, c, repo.ID, openPR.ID, facetPRReviews, now.Add(-2*24*time.Hour).Add(30*time.Minute), []github.Review{
		{ID: 3, Author: "reviewer", AuthorAssociation: "COLLABORATOR", SubmittedAt: now.Add(-2 * 24 * time.Hour).Add(30 * time.Minute), State: "COMMENTED"},
	})

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
	if report.PullRequests.Open != 2 || report.PullRequests.Merged != 1 || report.PullRequests.ClosedUnmerged != 1 || report.PullRequests.SampleSize != 4 {
		t.Fatalf("pr metrics = %+v", report.PullRequests)
	}

	if report.External.External != 3 || report.External.Known != 4 || report.External.Open != 2 || report.External.ClosedUnmerged != 1 || report.External.Merged != 0 {
		t.Fatalf("external metrics = %+v", report.External)
	}
	if report.External.MergeRate != 0 {
		t.Fatalf("external merge rate = %v, want 0", report.External.MergeRate)
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
