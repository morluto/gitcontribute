package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/health"
)

func TestRepositoryHealth(t *testing.T) {
	ctx := context.Background()
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	if _, err := svc.Init(ctx); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })

	ref := cli.RepoRef{Owner: "owner", Repo: "repo"}
	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{
		Owner:           ref.Owner,
		Name:            ref.Repo,
		SourceCreatedAt: now.Add(-365 * 24 * time.Hour),
		SourceUpdatedAt: now,
	}, "{}")
	if err != nil {
		t.Fatalf("upsert repository: %v", err)
	}

	// External open PR with a recent review.
	openPR, err := upsertThread(ctx, svc.corpus, repo.ID, corpus.Thread{
		Kind:            corpus.ThreadKindPullRequest,
		Number:          1,
		State:           "open",
		Title:           "open pr",
		Author:          "alice",
		SourceCreatedAt: now.Add(-2 * 24 * time.Hour),
		SourceUpdatedAt: now.Add(-24 * time.Hour),
	}, map[string]string{"Author": "alice", "AuthorAssociation": "NONE"})
	if err != nil {
		t.Fatalf("upsert open pr: %v", err)
	}
	payload, _ := json.Marshal([]github.Review{
		{ID: 1, Author: "maintainer", AuthorAssociation: "MEMBER", SubmittedAt: now.Add(-24 * time.Hour), State: "COMMENTED"},
	})
	if err := svc.corpus.ApplyFacetObservationSet(ctx, repo.ID, &openPR.ID, "pr_reviews", []corpus.FacetObservationInput{{SourceUpdatedAt: now.Add(-24 * time.Hour), Payload: string(payload)}}, true, 0); err != nil {
		t.Fatalf("apply pr reviews: %v", err)
	}

	// Owner merged PR.
	_, err = upsertThread(ctx, svc.corpus, repo.ID, corpus.Thread{
		Kind:            corpus.ThreadKindPullRequest,
		Number:          2,
		State:           "closed",
		Title:           "merged pr",
		Author:          "owner1",
		SourceCreatedAt: now.Add(-12 * 24 * time.Hour),
		SourceUpdatedAt: now.Add(-3 * 24 * time.Hour),
		MergedAt:        now.Add(-3 * 24 * time.Hour),
		Merged:          true,
	}, map[string]string{"Author": "owner1", "AuthorAssociation": "OWNER"})
	if err != nil {
		t.Fatalf("upsert merged pr: %v", err)
	}

	// PR with no author-association metadata should not be counted as external.
	_, err = upsertThread(ctx, svc.corpus, repo.ID, corpus.Thread{
		Kind:            corpus.ThreadKindPullRequest,
		Number:          3,
		State:           "open",
		Title:           "unknown assoc pr",
		Author:          "ghost",
		SourceCreatedAt: now.Add(-5 * 24 * time.Hour),
		SourceUpdatedAt: now.Add(-1 * 24 * time.Hour),
	}, map[string]string{})
	if err != nil {
		t.Fatalf("upsert unknown assoc pr: %v", err)
	}

	opts := health.Options{
		Start:          now.Add(-30 * 24 * time.Hour),
		End:            now,
		StaleThreshold: 14 * 24 * time.Hour,
	}
	report, err := svc.RepositoryHealthWithOptions(ctx, ref, opts)
	if err != nil {
		t.Fatalf("repository health: %v", err)
	}

	if report.PullRequests.Open != 2 || report.PullRequests.Merged != 1 || report.PullRequests.SampleSize != 3 {
		t.Fatalf("pr metrics = %+v", report.PullRequests)
	}
	if report.External.External != 1 || report.External.Known != 2 || report.External.Open != 1 {
		t.Fatalf("external metrics = %+v", report.External)
	}
	if report.Congestion.OpenPRs != 2 || report.Congestion.SampleSize != 2 {
		t.Fatalf("congestion = %+v", report.Congestion)
	}
	if report.Stale.SampleSize != 2 || report.Stale.ActiveCount != 2 || report.Stale.NoReviewOrCommentCount != 1 {
		t.Fatalf("stale signals = %+v", report.Stale)
	}
	if report.Response.PullRequests.SampleSize != 1 || report.Response.PullRequests.Median != 24 {
		t.Fatalf("pr response = %+v", report.Response.PullRequests)
	}
}
