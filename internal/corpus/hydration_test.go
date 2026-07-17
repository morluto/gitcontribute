package corpus

import (
	"context"
	"testing"
	"time"
)

func TestApplyFacetObservationSetReplacesWithEmptyCompleteSet(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("apply repository: %v", err)
	}
	thread, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 1, "open", "title", "body", "a", time.Unix(2, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("apply thread: %v", err)
	}

	tid := thread.ID
	first := time.Unix(100, 0).UTC()
	second := time.Unix(200, 0).UTC()

	if err := c.ApplyFacetObservationSet(ctx, repo.ID, &tid, "comments", first,
		[]FacetObservationInput{{SourceUpdatedAt: first, Payload: `{"comment":1}`}}, true, 0); err != nil {
		t.Fatalf("apply first set: %v", err)
	}

	obs, err := c.ListFacetObservations(ctx, repo.ID, &tid, "comments")
	if err != nil {
		t.Fatalf("list observations: %v", err)
	}
	if len(obs) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(obs))
	}

	if err := c.ApplyFacetObservationSet(ctx, repo.ID, &tid, "comments", second, nil, true, 0); err != nil {
		t.Fatalf("apply empty complete set: %v", err)
	}

	obs, err = c.ListFacetObservations(ctx, repo.ID, &tid, "comments")
	if err != nil {
		t.Fatalf("list observations after empty set: %v", err)
	}
	if len(obs) != 0 {
		t.Fatalf("expected 0 observations after empty set, got %d", len(obs))
	}

	cov, err := c.GetCoverage(ctx, repo.ID, &tid, "comments")
	if err != nil {
		t.Fatalf("get coverage: %v", err)
	}
	if cov == nil || !cov.Complete || !cov.SourceUpdatedAt.Equal(second) {
		t.Fatalf("expected complete coverage at %v, got %+v", second, cov)
	}
}

func TestApplyFacetObservationSetIgnoresStaleEmptySet(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("apply repository: %v", err)
	}
	thread, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 1, "open", "title", "body", "a", time.Unix(2, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("apply thread: %v", err)
	}

	tid := thread.ID
	newer := time.Unix(200, 0).UTC()
	older := time.Unix(100, 0).UTC()

	if err := c.ApplyFacetObservationSet(ctx, repo.ID, &tid, "comments", newer,
		[]FacetObservationInput{{SourceUpdatedAt: newer, Payload: `{"comment":1}`}}, true, 0); err != nil {
		t.Fatalf("apply newer set: %v", err)
	}

	if err := c.ApplyFacetObservationSet(ctx, repo.ID, &tid, "comments", older, nil, true, 0); err != nil {
		t.Fatalf("apply stale empty set: %v", err)
	}

	obs, err := c.ListFacetObservations(ctx, repo.ID, &tid, "comments")
	if err != nil {
		t.Fatalf("list observations: %v", err)
	}
	if len(obs) != 1 {
		t.Fatalf("stale empty set overwrote observations: got %d", len(obs))
	}

	cov, err := c.GetCoverage(ctx, repo.ID, &tid, "comments")
	if err != nil {
		t.Fatalf("get coverage: %v", err)
	}
	if cov == nil || !cov.Complete || !cov.SourceUpdatedAt.Equal(newer) {
		t.Fatalf("expected coverage to remain at newer %v, got %+v", newer, cov)
	}
}
