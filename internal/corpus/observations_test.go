package corpus

import (
	"context"
	"fmt"
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

func TestListPullRequestPortfolioFiltersByAuthorAndState(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("apply repository: %v", err)
	}
	when := time.Unix(1000, 0).UTC()
	threads := []Thread{
		{RepositoryID: repo.ID, Kind: ThreadKindPullRequest, Number: 1, State: "open", Author: "Alice", Title: "alice open", SourceUpdatedAt: when},
		{RepositoryID: repo.ID, Kind: ThreadKindPullRequest, Number: 2, State: "closed", Author: "alice", Title: "alice closed", SourceUpdatedAt: when.Add(time.Second)},
		{RepositoryID: repo.ID, Kind: ThreadKindPullRequest, Number: 3, State: "open", Author: "bob", Title: "bob open", SourceUpdatedAt: when.Add(2 * time.Second)},
		{RepositoryID: repo.ID, Kind: ThreadKindIssue, Number: 4, State: "open", Author: "alice", Title: "not a pull request", SourceUpdatedAt: when.Add(3 * time.Second)},
	}
	for _, thread := range threads {
		if _, err := c.UpsertThread(ctx, thread, `{}`); err != nil {
			t.Fatalf("upsert thread %d: %v", thread.Number, err)
		}
	}

	got, err := c.ListPullRequestPortfolio(ctx, "ALICE", "OPEN", 10)
	if err != nil {
		t.Fatalf("list pull request portfolio: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("portfolio length = %d, want 1: %+v", len(got), got)
	}
	if got[0].Owner != "owner" || got[0].Repo != "repo" || got[0].Thread.Number != 1 {
		t.Fatalf("portfolio item = %+v, want owner/repo#1", got[0])
	}

	got, err = c.ListPullRequestPortfolio(ctx, "alice", "all", 10)
	if err != nil {
		t.Fatalf("list pull request portfolio for all states: %v", err)
	}
	if len(got) != 2 || got[0].Thread.Number != 2 || got[1].Thread.Number != 1 {
		t.Fatalf("all-state portfolio = %+v, want #2 then #1", got)
	}
}

func TestListPullRequestPortfolioUsesDeterministicGlobalOrder(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	type repoSpec struct {
		owner string
		name  string
	}
	repos := []repoSpec{{owner: "beta", name: "zeta"}, {owner: "alpha", name: "zeta"}, {owner: "alpha", name: "aardvark"}}
	when := time.Unix(2000, 0).UTC()
	for i, spec := range repos {
		repo, err := c.ApplyRepositoryObservation(ctx, spec.owner, spec.name, spec.owner+"/"+spec.name, time.Unix(1, 0).UTC(), `{}`)
		if err != nil {
			t.Fatalf("apply repository %s/%s: %v", spec.owner, spec.name, err)
		}
		for _, number := range []int{2, 1} {
			updated := when
			if i == 0 && number == 2 {
				updated = when.Add(time.Second)
			}
			if _, err := c.UpsertThread(ctx, Thread{
				RepositoryID:    repo.ID,
				Kind:            ThreadKindPullRequest,
				Number:          number,
				State:           "open",
				Author:          "alice",
				Title:           "pull request",
				SourceUpdatedAt: updated,
			}, `{}`); err != nil {
				t.Fatalf("upsert %s/%s#%d: %v", spec.owner, spec.name, number, err)
			}
		}
	}

	got, err := c.ListPullRequestPortfolio(ctx, "", "", 100)
	if err != nil {
		t.Fatalf("list pull request portfolio: %v", err)
	}
	keys := make([]string, 0, len(got))
	for _, item := range got {
		keys = append(keys, fmt.Sprintf("%s/%s#%d", item.Owner, item.Repo, item.Thread.Number))
	}
	want := []string{
		"beta/zeta#2",
		"alpha/aardvark#1",
		"alpha/aardvark#2",
		"alpha/zeta#1",
		"alpha/zeta#2",
		"beta/zeta#1",
	}
	if diff := cmp.Diff(want, keys); diff != "" {
		t.Fatalf("portfolio order mismatch (-want +got):\n%s", diff)
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

func TestUpsertThreadUnknownMergeStateDoesNotEraseKnownState(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("apply repository: %v", err)
	}

	at := time.Unix(100, 0).UTC()
	mergedAt := at.Add(-time.Hour)
	known, err := c.UpsertThread(ctx, Thread{
		RepositoryID: repo.ID, Kind: ThreadKindPullRequest, Number: 1,
		State: "closed", Title: "details", Merged: true, MergedKnown: true,
		MergedAt: mergedAt, SourceUpdatedAt: at,
	}, `{"Merged":true}`)
	if err != nil {
		t.Fatalf("upsert known details: %v", err)
	}
	if !known.MergedKnown || !known.Merged {
		t.Fatalf("known projection = %+v", known)
	}

	got, err := c.UpsertThread(ctx, Thread{
		RepositoryID: repo.ID, Kind: ThreadKindPullRequest, Number: 1,
		State: "closed", Title: "newer header", SourceUpdatedAt: at.Add(time.Second),
	}, `{"Kind":"pull_request"}`)
	if err != nil {
		t.Fatalf("upsert header-only observation: %v", err)
	}
	if got.Title != "newer header" || !got.MergedKnown || !got.Merged || !got.MergedAt.Equal(mergedAt) {
		t.Fatalf("header sync erased explicit merge state: %+v", got)
	}

	got, err = c.UpsertThread(ctx, Thread{
		RepositoryID: repo.ID, Kind: ThreadKindPullRequest, Number: 1,
		State: "closed", Title: "observed false", MergedKnown: true,
		SourceUpdatedAt: at.Add(2 * time.Second),
	}, `{"Merged":false}`)
	if err != nil {
		t.Fatalf("upsert observed false details: %v", err)
	}
	if !got.MergedKnown || got.Merged || !got.MergedAt.IsZero() {
		t.Fatalf("explicit false did not replace projection: %+v", got)
	}
}
