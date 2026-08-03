package corpus

import (
	"context"
	"testing"
	"time"
)

func TestActorProfileObservationReconcilesLoginToNodeIDAndPreservesNewerProjection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	one := int64(1)
	newer := time.Unix(20, 0).UTC()
	name := "Mona"
	bio := "machine learning"
	followers := 10
	first, err := c.ApplyActorProfileObservation(ctx, ActorProfileObservation{
		Provider: "github", Login: "Mona", NodeID: "U_1", DatabaseID: &one, Kind: "user",
		SourceUpdatedAt: newer, ObservedAt: time.Unix(21, 0).UTC(),
		Profile: ActorProfile{Name: &name, Bio: &bio, Followers: &followers},
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.NodeID != "U_1" || first.Key != "github:node:U_1" || first.Profile == nil || *first.Profile.Followers != 10 {
		t.Fatalf("first actor = %+v", first)
	}
	olderFollowers := 2
	olderBio := "gardening"
	if _, err := c.ApplyActorProfileObservation(ctx, ActorProfileObservation{
		Provider: "github", Login: "mona", NodeID: "U_1", Kind: "user",
		SourceUpdatedAt: time.Unix(10, 0).UTC(), ObservedAt: time.Unix(22, 0).UTC(),
		Profile: ActorProfile{Bio: &olderBio, Followers: &olderFollowers},
	}); err != nil {
		t.Fatal(err)
	}
	stored, err := c.GetActor(ctx, "MONA")
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil || stored.ID != first.ID || stored.Profile == nil || *stored.Profile.Followers != 10 {
		t.Fatalf("stored actor = %+v", stored)
	}
	if _, err := c.ApplyActorIdentityObservation(ctx, "github", "mona", "U_1", &one, "user", "public", time.Unix(23, 0).UTC(), nil); err != nil {
		t.Fatal(err)
	}
	machine, err := c.SearchActors(ctx, ActorSearchOptions{Query: "machine", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	gardening, err := c.SearchActors(ctx, ActorSearchOptions{Query: "gardening", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(machine.Actors) != 1 || machine.Actors[0].ID != first.ID || len(gardening.Actors) != 0 {
		t.Fatalf("search projection after stale profile and identity observations: machine=%+v gardening=%+v", machine, gardening)
	}
}

func TestSearchActorsReturnsNullableProfilesAndBoundedCursor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	for index, login := range []string{"alice", "alicia"} {
		bio := "machine learning"
		followers := 20 - index
		if _, err := c.ApplyActorProfileObservation(ctx, ActorProfileObservation{
			Provider: "github", Login: login, NodeID: "U_" + login, Kind: "user",
			SourceUpdatedAt: time.Unix(int64(index+1), 0).UTC(), ObservedAt: time.Unix(10, 0).UTC(),
			Profile: ActorProfile{Bio: &bio, Followers: &followers},
		}); err != nil {
			t.Fatal(err)
		}
	}
	page, err := c.SearchActors(ctx, ActorSearchOptions{Query: "machine", Sort: "followers", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Actors) != 1 || page.Total != 2 || page.NextCursor == "" || page.Actors[0].Login != "alice" {
		t.Fatalf("first page = %+v", page)
	}
	next, err := c.SearchActors(ctx, ActorSearchOptions{Query: "machine", Sort: "followers", Limit: 1, Cursor: page.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Actors) != 1 || next.Actors[0].Login != "alicia" {
		t.Fatalf("next page = %+v", next)
	}
	if _, err := c.SearchActors(ctx, ActorSearchOptions{Query: "machine", Kinds: []string{"bot"}, Sort: "followers", Limit: 1, Cursor: page.NextCursor}); err == nil {
		t.Fatal("actor cursor was accepted with a different kind filter")
	}
}

func TestActorContributionSearchBindsCursorToFilters(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	actor, err := c.ApplyActorIdentityObservation(ctx, "github", "alice", "U_alice", nil, "user", "public", time.Unix(1, 0).UTC(), nil)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := c.UpsertRepository(ctx, Repository{Owner: "acme", Name: "ml"}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	items := []ActorContributionItem{
		{Kind: "commit", OccurredAt: from.Add(time.Hour), RepositoryID: &repository.ID, Count: 1},
		{Kind: "issue", OccurredAt: from.Add(2 * time.Hour), RepositoryID: &repository.ID, Count: 1},
	}
	if err := c.ApplyActorContributionPeriod(ctx, ActorContributionPeriodInput{ActorID: actor.ID, From: from, To: from.Add(24 * time.Hour), Complete: true, ObservedAt: from.Add(25 * time.Hour), SourceUpdatedAt: from.Add(24 * time.Hour), Items: items}); err != nil {
		t.Fatal(err)
	}
	if err := c.ApplyActorContributionPeriod(ctx, ActorContributionPeriodInput{ActorID: actor.ID, From: from, To: from.Add(24 * time.Hour), OrganizationNodeID: "O_acme", Complete: true, ObservedAt: from.Add(26 * time.Hour), SourceUpdatedAt: from.Add(24 * time.Hour), Items: items}); err != nil {
		t.Fatal(err)
	}
	page, err := c.SearchActorContributions(ctx, ContributionSearchOptions{ActorRefs: []string{"alice"}, RepositoryRefs: []string{"acme/ml"}, Sort: "occurred_at", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Total != 2 || page.NextCursor == "" || page.Items[0].Kind != "issue" {
		t.Fatalf("page = %+v", page)
	}
	organizationPage, err := c.SearchActorContributions(ctx, ContributionSearchOptions{ActorRefs: []string{"alice"}, OrganizationNodeID: "O_acme", Sort: "occurred_at", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if organizationPage.Total != 2 {
		t.Fatalf("organization-scoped page = %+v", organizationPage)
	}
	for _, ref := range []string{actor.Key, actor.NodeID} {
		exact, err := c.SearchActorContributions(ctx, ContributionSearchOptions{ActorRefs: []string{ref}, Sort: "occurred_at", Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if exact.Total != 2 {
			t.Fatalf("actor reference %q returned %+v", ref, exact)
		}
	}
	if _, err := c.SearchActorContributions(ctx, ContributionSearchOptions{ActorRefs: []string{"alice"}, RepositoryRefs: []string{"other/repo"}, Sort: "occurred_at", Limit: 1, Cursor: page.NextCursor}); err == nil {
		t.Fatal("cursor was accepted with different repository filters")
	}
	covered, err := c.GetActorContributionCoverage(ctx, actor.ID, "", from.Add(time.Hour), from.Add(12*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	uncovered, err := c.GetActorContributionCoverage(ctx, actor.ID, "", from.Add(-time.Hour), from.Add(12*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if covered == nil || uncovered != nil {
		t.Fatalf("period coverage: covered=%+v uncovered=%+v", covered, uncovered)
	}
	if err := c.ApplyActorContributionPeriod(ctx, ActorContributionPeriodInput{ActorID: actor.ID, From: from, To: from.Add(24 * time.Hour), Complete: false, ObservedAt: from.Add(27 * time.Hour), SourceUpdatedAt: from.Add(24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	partial, err := c.GetActorContributionCoverage(ctx, actor.ID, "", from.Add(time.Hour), from.Add(12*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if partial != nil {
		t.Fatalf("partial refresh retained complete coverage: %+v", partial)
	}
	organizationCovered, err := c.GetActorContributionCoverage(ctx, actor.ID, "O_acme", from.Add(time.Hour), from.Add(12*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if organizationCovered == nil {
		t.Fatal("organization-scoped period was not recognized as covered")
	}
}

func TestActorContributionSearchDeduplicatesOverlappingPeriods(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	actor, err := c.ApplyActorIdentityObservation(ctx, "github", "alice", "U_alice", nil, "user", "public", time.Unix(1, 0).UTC(), nil)
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	item := ActorContributionItem{Kind: "issue", OccurredAt: from.Add(2 * time.Hour), TargetNodeID: "I_1", TargetURL: "https://example/issues/1", Count: 1}
	for _, period := range []struct{ from, to time.Time }{
		{from: from, to: from.Add(24 * time.Hour)},
		{from: from.Add(time.Hour), to: from.Add(12 * time.Hour)},
	} {
		if err := c.ApplyActorContributionPeriod(ctx, ActorContributionPeriodInput{ActorID: actor.ID, From: period.from, To: period.to, Complete: true, ObservedAt: period.to.Add(time.Hour), SourceUpdatedAt: period.to, Items: []ActorContributionItem{item}}); err != nil {
			t.Fatal(err)
		}
	}
	page, err := c.SearchActorContributions(ctx, ContributionSearchOptions{ActorRefs: []string{actor.Key}, From: from.Add(time.Hour), To: from.Add(12 * time.Hour), Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].TargetNodeID != "I_1" {
		t.Fatalf("overlapping contribution periods = %+v", page)
	}
}
