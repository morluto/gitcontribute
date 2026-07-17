package corpus

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestListThreadsFilteredAppliesStateBeforeLimit(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("apply repository: %v", err)
	}

	base := time.Unix(1000, 0).UTC()
	threads := []struct {
		number int
		state  string
		when   time.Time
	}{
		{1, "closed", base.Add(3 * time.Second)},
		{2, "closed", base.Add(2 * time.Second)},
		{3, "open", base.Add(1 * time.Second)},
	}
	for _, th := range threads {
		if _, err := c.UpsertThread(ctx, Thread{
			RepositoryID:    repo.ID,
			Kind:            ThreadKindIssue,
			Number:          th.number,
			State:           th.state,
			Title:           "title",
			Body:            "body",
			Author:          "alice",
			SourceCreatedAt: th.when,
			SourceUpdatedAt: th.when,
		}, `{}`); err != nil {
			t.Fatalf("upsert thread %d: %v", th.number, err)
		}
	}

	// A limit of 1 applied before the state filter would return nothing,
	// because the most recently updated row is closed. Filtering first
	// should return the most recently updated open thread (#3).
	listed, err := c.ListThreadsFiltered(ctx, repo.ID, ThreadKindIssue, "open", 1)
	if err != nil {
		t.Fatalf("list threads filtered: %v", err)
	}
	if len(listed) != 1 || listed[0].Number != 3 {
		t.Fatalf("got %+v, want one open thread with number 3", listed)
	}

	// Limit 2 should still return only the open threads and respect the bound.
	listed, err = c.ListThreadsFiltered(ctx, repo.ID, ThreadKindIssue, "open", 2)
	if err != nil {
		t.Fatalf("list threads filtered: %v", err)
	}
	if len(listed) != 1 || listed[0].Number != 3 {
		t.Fatalf("got %+v, want one open thread with number 3", listed)
	}
	total, err := c.CountThreadsFiltered(ctx, repo.ID, ThreadKindIssue, "open")
	if err != nil {
		t.Fatalf("count threads filtered: %v", err)
	}
	if total != 1 {
		t.Fatalf("open thread count = %d, want 1", total)
	}
}

func TestUpsertThreadPersistsMetadataAndDeterministicAssignees(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("apply repository: %v", err)
	}

	want := Thread{
		RepositoryID:      repo.ID,
		Kind:              ThreadKindIssue,
		Number:            1,
		State:             "open",
		StateReason:       "completed",
		Title:             "bug",
		Body:              "body",
		Author:            "alice",
		AuthorAssociation: "OWNER",
		Labels:            []string{"bug"},
		Assignees:         []string{"alice", "bob", "charlie"},
		Draft:             false,
		Locked:            true,
		Milestone:         "v1.0",
		SourceCreatedAt:   time.Unix(100, 0).UTC(),
		SourceUpdatedAt:   time.Unix(200, 0).UTC(),
	}

	// Pass assignees out of order; they should be stored deterministically.
	input := want
	input.Assignees = []string{"charlie", "bob", "alice"}
	got, err := c.UpsertThread(ctx, input, `{}`)
	if err != nil {
		t.Fatalf("upsert thread: %v", err)
	}
	if diff := cmp.Diff(want, *got, cmpopts.IgnoreFields(Thread{}, "ID", "ObservationSequence", "CreatedAt", "UpdatedAt", "Rank")); diff != "" {
		t.Fatalf("upsert mismatch (-want +got):\n%s", diff)
	}

	fetched, err := c.GetThread(ctx, repo.ID, ThreadKindIssue, 1)
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if diff := cmp.Diff(*got, *fetched); diff != "" {
		t.Fatalf("get thread mismatch (-want +got):\n%s", diff)
	}

	listed, err := c.ListThreads(ctx, repo.ID, "", 10)
	if err != nil {
		t.Fatalf("list threads: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("listed = %d, want 1", len(listed))
	}
	if diff := cmp.Diff(*got, listed[0]); diff != "" {
		t.Fatalf("list threads mismatch (-want +got):\n%s", diff)
	}

	byNumber, err := c.GetThreadByNumber(ctx, repo.ID, 1)
	if err != nil {
		t.Fatalf("get thread by number: %v", err)
	}
	if diff := cmp.Diff(*got, *byNumber); diff != "" {
		t.Fatalf("get thread by number mismatch (-want +got):\n%s", diff)
	}
}
