package corpus

import (
	"context"
	"testing"
	"time"
)

func TestResolutionRecordsAreAppendOnlyAndStaleSafe(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	threadID := insertPortfolioFixture(t, ctx, c)
	threadObservation, err := c.LatestThreadObservation(ctx, threadID)
	if err != nil {
		t.Fatal(err)
	}
	var repositoryID int64
	if err := c.db.QueryRowContext(ctx, `SELECT repository_id FROM threads WHERE id=?`, threadID).Scan(&repositoryID); err != nil {
		t.Fatal(err)
	}
	if err := c.ApplyFacetObservationSet(ctx, repositoryID, &threadID, "pr_files", time.Unix(500, 0).UTC(), []FacetObservationInput{{SourceUpdatedAt: time.Unix(500, 0).UTC(), Payload: `[]`}}, true, 0); err != nil {
		t.Fatal(err)
	}
	facetObservations, err := c.ListFacetObservations(ctx, repositoryID, &threadID, "pr_files")
	if err != nil {
		t.Fatal(err)
	}
	newer := ResolutionRecord{
		ThreadID: threadID, Kind: "fixed", Summary: "observed closing PR",
		RuleVersion: "resolution-v2", SourceUpdatedAt: time.Unix(500, 0).UTC(),
		SourceObservationRefs: []ObservationRef{{Kind: "thread", ID: threadObservation.ID}, {Kind: "facet", ID: facetObservations[0].ID}},
	}
	if _, err := c.SaveResolutionRecord(ctx, newer); err != nil {
		t.Fatalf("save newer resolution: %v", err)
	}
	stale := newer
	stale.Kind = "lexically_similar"
	stale.Summary = "stale lexical inference"
	stale.RuleVersion = "resolution-v1"
	stale.SourceUpdatedAt = newer.SourceUpdatedAt.Add(-time.Hour)
	if _, err := c.SaveResolutionRecord(ctx, stale); err != nil {
		t.Fatalf("save stale resolution history: %v", err)
	}

	got, err := c.GetResolutionRecord(ctx, threadID)
	if err != nil {
		t.Fatalf("get resolution: %v", err)
	}
	if got == nil || got.Kind != "fixed" || got.RuleVersion != "resolution-v2" || len(got.SourceObservationRefs) != 2 {
		t.Fatalf("current resolution = %#v", got)
	}
	var count int
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM resolution_records WHERE thread_id=?`, threadID).Scan(&count); err != nil {
		t.Fatalf("count resolution history: %v", err)
	}
	if count != 2 {
		t.Fatalf("resolution record count = %d, want 2", count)
	}
}
