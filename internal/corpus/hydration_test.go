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

func TestListFacetObservationsBoundedPreservesOrderAndReportsMore(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 1, "open", "title", "body", "a", time.Unix(2, 0).UTC(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	pages := []FacetObservationInput{
		{SourceUpdatedAt: time.Unix(3, 0).UTC(), Payload: `{"page":1}`},
		{SourceUpdatedAt: time.Unix(4, 0).UTC(), Payload: `{"page":2}`},
		{SourceUpdatedAt: time.Unix(5, 0).UTC(), Payload: `{"page":3}`},
	}
	if err := c.ApplyFacetObservationSet(ctx, repo.ID, &thread.ID, "comments", time.Unix(5, 0).UTC(), pages, true, 0); err != nil {
		t.Fatal(err)
	}

	observations, hasMore, err := c.ListFacetObservationsBounded(ctx, repo.ID, &thread.ID, "comments", 2)
	if err != nil {
		t.Fatal(err)
	}
	if !hasMore || len(observations) != 2 || observations[0].Payload != `{"page":1}` || observations[1].Payload != `{"page":2}` {
		t.Fatalf("bounded observations = %+v, has_more=%t", observations, hasMore)
	}
	observations, hasMore, err = c.ListFacetObservationsBounded(ctx, repo.ID, &thread.ID, "comments", 3)
	if err != nil {
		t.Fatal(err)
	}
	if hasMore || len(observations) != 3 {
		t.Fatalf("exact-bound observations = %d, has_more=%t", len(observations), hasMore)
	}
	if _, _, err := c.ListFacetObservationsBounded(ctx, repo.ID, &thread.ID, "comments", 0); err == nil {
		t.Fatal("expected invalid limit error")
	}
}

func TestApplyFacetObservationSetCASRejectsConcurrentEqualClockReplacement(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "1", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindPullRequest, 1, "open", "title", "body", "a", time.Unix(2, 0).UTC(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Unix(3, 0).UTC()
	if err := c.ApplyFacetObservationSet(ctx, repo.ID, &thread.ID, "checks", at, []FacetObservationInput{{SourceUpdatedAt: at, Payload: `[{"name":"initial"}]`}}, true, 0); err != nil {
		t.Fatal(err)
	}
	baseline, err := c.GetCoverage(ctx, repo.ID, &thread.ID, "checks")
	if err != nil {
		t.Fatal(err)
	}
	applied, err := c.ApplyFacetObservationSetCAS(ctx, repo.ID, &thread.ID, "checks", at, []FacetObservationInput{{SourceUpdatedAt: at, Payload: `[{"name":"newer"}]`}}, true, 0, baseline.ObservationSequence)
	if err != nil || !applied {
		t.Fatalf("first CAS applied=%v err=%v", applied, err)
	}
	applied, err = c.ApplyFacetObservationSetCAS(ctx, repo.ID, &thread.ID, "checks", at, []FacetObservationInput{{SourceUpdatedAt: at, Payload: `[{"name":"late-stale"}]`}}, true, 0, baseline.ObservationSequence)
	if err != nil || applied {
		t.Fatalf("stale CAS applied=%v err=%v", applied, err)
	}
	observations, err := c.ListFacetObservations(ctx, repo.ID, &thread.ID, "checks")
	if err != nil || len(observations) != 1 || observations[0].Payload != `[{"name":"newer"}]` {
		t.Fatalf("observations=%+v err=%v", observations, err)
	}
}
