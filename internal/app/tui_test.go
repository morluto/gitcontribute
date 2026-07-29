package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/tuicontract"
)

func TestTUILoadReadsBoundedLocalData(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer svc.Close()
	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	repo := seedRepoForNeighbors(t, c)
	seedIssueForNeighbors(t, c, repo.ID, 7, "local issue", "local body", "alice", []string{"bug"})
	if err := c.AdvanceFacet(ctx, repo.ID, nil, "metadata", repo.SourceUpdatedAt, true, 0); err != nil {
		t.Fatal(err)
	}

	data, err := svc.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Repositories) != 1 || data.Repositories[0].Ref != "owner/repo" {
		t.Fatalf("repositories=%+v", data.Repositories)
	}
	if len(data.Threads) != 1 || data.Threads[0].Detail != "local body" || data.Threads[0].Kind != corpus.ThreadKindIssue {
		t.Fatalf("threads=%+v", data.Threads)
	}
	if len(data.Candidates) != 1 || data.Candidates[0].Ref != "issue:owner/repo#7" || data.Candidates[0].Assessment == nil {
		t.Fatalf("candidates=%+v", data.Candidates)
	}
	if len(data.Repositories[0].Coverage) != 1 || !data.Repositories[0].Coverage[0].Complete {
		t.Fatalf("coverage=%+v", data.Repositories[0].Coverage)
	}
}

func TestTUILoadProjectsOfflineSyncStatusAndRecovery(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer svc.Close()
	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	repo := seedRepoForNeighbors(t, c)
	run, err := c.StartRun(ctx, "archive sync")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.AdvanceFacet(ctx, repo.ID, nil, "metadata", repo.SourceUpdatedAt, true, run.ID); err != nil {
		t.Fatal(err)
	}
	if err := c.AdvanceFacet(ctx, repo.ID, nil, "threads", repo.SourceUpdatedAt.Add(-time.Second), false, run.ID); err != nil {
		t.Fatal(err)
	}
	if err := c.FinishRunPartial(ctx, run.ID, "threads=1", "rate limit interrupted thread retrieval"); err != nil {
		t.Fatal(err)
	}

	data, err := svc.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.SyncStatuses) != 1 {
		t.Fatalf("sync statuses=%+v", data.SyncStatuses)
	}
	status := data.SyncStatuses[0]
	if status.Ref != "owner/repo" || status.Status != "partial" {
		t.Fatalf("status=%+v", status)
	}
	if !strings.Contains(status.Detail, "incomplete") {
		t.Fatalf("detail=%q", status.Detail)
	}
	if len(status.Commands) != 1 || status.Commands[0] != "gitcontribute archive sync owner/repo" {
		t.Fatalf("commands=%v", status.Commands)
	}
	if len(status.Coverage) != 3 || status.Coverage[2].Name != "contribution_guidance" || status.Coverage[2].Present {
		t.Fatalf("coverage=%+v", status.Coverage)
	}
	if status.Assessment == nil || len(status.Assessment.Risks) == 0 ||
		!strings.Contains(status.Assessment.Risks[0].Summary, "partial") {
		t.Fatalf("assessment=%+v", status.Assessment)
	}
	foundLastSuccessful := false
	for _, fact := range status.Assessment.Positive {
		foundLastSuccessful = foundLastSuccessful || strings.Contains(fact.Summary, "Last successful sync")
	}
	if !foundLastSuccessful {
		t.Fatalf("last successful sync missing from assessment=%+v", status.Assessment)
	}
}

func TestTUILoadSyncStatusUsesOnlyStoredEvidence(t *testing.T) {
	t.Parallel()
	fixture := newResearchFixture(t)

	data, err := fixture.svc.Load(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.SyncStatuses) != 1 || data.SyncStatuses[0].Ref != "owner/repo" {
		t.Fatalf("sync statuses=%+v", data.SyncStatuses)
	}
}

func TestTUISyncStatusClassifiesCompleteStaleMissingAndFailedEvidence(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name       string
		seed       func(t *testing.T, ctx context.Context, c *corpus.Corpus, repo *corpus.Repository)
		wantStatus string
		wantFact   string
	}{
		{
			name: "complete",
			seed: func(t *testing.T, ctx context.Context, c *corpus.Corpus, repo *corpus.Repository) {
				t.Helper()
				for _, facet := range []string{"metadata", "threads", FacetContributionGuidance} {
					if err := c.AdvanceFacet(ctx, repo.ID, nil, facet, repo.SourceUpdatedAt, true, 0); err != nil {
						t.Fatal(err)
					}
				}
			},
			wantStatus: "complete",
			wantFact:   "current",
		},
		{
			name: "stale ranking evidence",
			seed: func(t *testing.T, ctx context.Context, c *corpus.Corpus, repo *corpus.Repository) {
				t.Helper()
				for _, facet := range []string{"metadata", "threads", FacetContributionGuidance} {
					if err := c.AdvanceFacet(ctx, repo.ID, nil, facet, repo.SourceUpdatedAt.Add(-time.Second), true, 0); err != nil {
						t.Fatal(err)
					}
				}
			},
			wantStatus: "stale",
			wantFact:   "stale evidence",
		},
		{
			name:       "missing facets",
			seed:       func(*testing.T, context.Context, *corpus.Corpus, *corpus.Repository) {},
			wantStatus: "partial",
			wantFact:   "required repository facets are missing",
		},
		{
			name: "failed run",
			seed: func(t *testing.T, ctx context.Context, c *corpus.Corpus, repo *corpus.Repository) {
				t.Helper()
				run, err := c.StartRun(ctx, "archive sync")
				if err != nil {
					t.Fatal(err)
				}
				if err := c.AdvanceFacet(ctx, repo.ID, nil, "metadata", repo.SourceUpdatedAt, true, run.ID); err != nil {
					t.Fatal(err)
				}
				if err := c.FailRun(ctx, run.ID, "authentication expired"); err != nil {
					t.Fatal(err)
				}
			},
			wantStatus: "partial",
			wantFact:   "authentication expired",
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			svc := newTestServiceNoNetwork(t)
			defer svc.Close()
			c, err := svc.openCorpus(ctx)
			if err != nil {
				t.Fatal(err)
			}
			repo := seedRepoForNeighbors(t, c)
			tc.seed(t, ctx, c, repo)

			data, err := svc.Load(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(data.SyncStatuses) != 1 {
				t.Fatalf("sync statuses=%+v", data.SyncStatuses)
			}
			status := data.SyncStatuses[0]
			if status.Status != tc.wantStatus {
				t.Fatalf("status=%q want %q: %+v", status.Status, tc.wantStatus, status)
			}
			rendered := status.Detail
			if status.Assessment != nil {
				for _, facts := range [][]tuicontract.Fact{
					status.Assessment.Positive, status.Assessment.Risks, status.Assessment.Unknowns,
				} {
					for _, fact := range facts {
						rendered += "\n" + fact.Summary
					}
				}
			}
			if !strings.Contains(rendered, tc.wantFact) {
				t.Fatalf("status evidence omitted %q:\n%s", tc.wantFact, rendered)
			}
		})
	}
}
