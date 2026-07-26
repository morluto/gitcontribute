package corpus

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestBatchReadsReturnRequestedRepositoriesThreadsAndCoverage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	repo, err := c.UpsertRepository(ctx, Repository{
		Owner: "owner", Name: "repo", Description: "batched",
		SourceUpdatedAt: now,
	}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := c.UpsertThread(ctx, Thread{
		RepositoryID: repo.ID, Kind: ThreadKindIssue, Number: 7,
		State: "open", Title: "batch me", Body: "body", SourceUpdatedAt: now,
	}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.AdvanceFacet(ctx, repo.ID, nil, "metadata", now, true, 0); err != nil {
		t.Fatal(err)
	}
	for i, asOf := range []time.Time{now.Add(-time.Hour), now} {
		if _, err := c.SaveDossier(ctx, repo.ID, repo.Owner, repo.Name, "sha", asOf, `{}`, `{}`, now.Add(time.Duration(i)*time.Minute), nil); err != nil {
			t.Fatal(err)
		}
	}

	repositories, err := c.GetRepositoriesBatch(ctx, []RepositoryKey{
		{Owner: "missing", Name: "repo"},
		{Owner: "owner", Name: "repo"},
		{Owner: "owner", Name: "repo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := repositories[RepositoryKey{Owner: "owner", Name: "repo"}]; got == nil || got.Description != "batched" {
		t.Fatalf("repository = %+v", got)
	}
	if len(repositories) != 1 {
		t.Fatalf("repository count = %d, want 1", len(repositories))
	}

	threads, err := c.GetThreadsBatch(ctx, []ThreadKey{
		{RepositoryID: repo.ID, Number: 7},
		{RepositoryID: repo.ID, Kind: ThreadKindIssue, Number: 7},
		{RepositoryID: repo.ID, Kind: ThreadKindPullRequest, Number: 7},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []ThreadKey{
		{RepositoryID: repo.ID, Number: 7},
		{RepositoryID: repo.ID, Kind: ThreadKindIssue, Number: 7},
	} {
		if got := threads[key]; got == nil || got.ID != thread.ID {
			t.Fatalf("thread %v = %+v", key, got)
		}
	}
	if threads[ThreadKey{RepositoryID: repo.ID, Kind: ThreadKindPullRequest, Number: 7}] != nil {
		t.Fatal("unexpected pull request result")
	}

	coverage, err := c.ListRepositoryCoverageBatch(ctx, []int64{repo.ID}, []string{"metadata"})
	if err != nil {
		t.Fatal(err)
	}
	if got := coverage[RepositoryFacetKey{RepositoryID: repo.ID, Facet: "metadata"}]; got == nil || !got.Complete {
		t.Fatalf("coverage = %+v", got)
	}
	dossiers, err := c.GetLatestDossierMetadataBatch(ctx, []int64{repo.ID, repo.ID + 100})
	if err != nil {
		t.Fatal(err)
	}
	if got := dossiers[repo.ID]; !got.AsOf.Equal(now) {
		t.Fatalf("dossier metadata = %+v", got)
	}
	if _, ok := dossiers[repo.ID+100]; ok {
		t.Fatal("unexpected dossier metadata for missing repository")
	}
}

func TestBatchReadsRejectOversizedInputs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	repositories := make([]RepositoryKey, maxBatchReadItems+1)
	if _, err := c.GetRepositoriesBatch(ctx, repositories); err == nil || !strings.Contains(err.Error(), "cannot exceed 100") {
		t.Fatalf("repository batch error = %v", err)
	}
	threads := make([]ThreadKey, maxBatchReadItems+1)
	if _, err := c.GetThreadsBatch(ctx, threads); err == nil || !strings.Contains(err.Error(), "cannot exceed 100") {
		t.Fatalf("thread batch error = %v", err)
	}
	if _, err := c.ListRepositoryCoverageBatch(ctx, make([]int64, maxBatchReadItems+1), []string{"metadata"}); err == nil || !strings.Contains(err.Error(), "cannot exceed 100") {
		t.Fatalf("coverage batch error = %v", err)
	}
	if _, err := c.GetLatestDossierMetadataBatch(ctx, make([]int64, maxBatchReadItems+1)); err == nil || !strings.Contains(err.Error(), "cannot exceed 100") {
		t.Fatalf("dossier metadata batch error = %v", err)
	}
}
