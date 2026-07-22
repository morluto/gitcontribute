package corpus

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/lens"
)

func TestLensesPersistAndUpdateByName(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "organize.db")
	c, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	first, err := c.SaveLens(ctx, lens.Definition{
		Name: " active-go ",
		Filter: lens.Filter{
			Languages: []string{"Go"}, ExcludeArchived: true, UpdatedWithin: 30 * 24 * time.Hour,
		},
		Weights: map[string]float64{"activity": 2, "collision_risk": -1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Definition.Name != "active-go" || first.CreatedAt.IsZero() {
		t.Fatalf("first lens = %+v", first)
	}
	updated, err := c.SaveLens(ctx, lens.Definition{
		Name: "active-go", Weights: map[string]float64{"maintainer_fit": 3}, MaxResultsPerRepo: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.CreatedAt.Equal(first.CreatedAt) || updated.Definition.Weights["maintainer_fit"] != 3 {
		t.Fatalf("updated lens = %+v, first = %+v", updated, first)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}

	c, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	records, err := c.ListLenses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Definition.MaxResultsPerRepo != 2 {
		t.Fatalf("lenses = %+v", records)
	}
}

func TestSaveLensRejectsInvalidDefinition(t *testing.T) {
	t.Parallel()
	c, _ := openTestCorpus(t)
	if _, err := c.SaveLens(context.Background(), lens.Definition{
		Name: "invalid", Weights: map[string]float64{"activity": 0},
	}); err == nil {
		t.Fatal("expected invalid lens error")
	}
}

func TestCollectionsDeduplicateTypedReferences(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	created, err := c.SaveCollection(ctx, "favorites")
	if err != nil {
		t.Fatal(err)
	}
	members := []CollectionMember{
		{Kind: "repository", Ref: "octocat/hello-world"},
		{Kind: "issue", Ref: "octocat/hello-world#12"},
	}
	if err := c.AddCollectionMembers(ctx, "favorites", members); err != nil {
		t.Fatal(err)
	}
	if err := c.AddCollectionMembers(ctx, "favorites", []CollectionMember{
		{Kind: "issue", Ref: "octocat/hello-world#12"},
		{Kind: "pull_request", Ref: "octocat/hello-world#12"},
	}); err != nil {
		t.Fatal(err)
	}

	stored, err := c.GetCollection(ctx, "favorites")
	if err != nil {
		t.Fatal(err)
	}
	if stored.ID != created.ID || stored.MemberCount != 3 {
		t.Fatalf("collection = %+v", stored)
	}
	got, err := c.ListCollectionMembers(ctx, "favorites")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Kind != "issue" || got[1].Kind != "pull_request" || got[2].Kind != "repository" {
		t.Fatalf("members = %+v", got)
	}
	collections, err := c.ListCollections(ctx)
	if err != nil || len(collections) != 1 || collections[0].MemberCount != 3 {
		t.Fatalf("collections = %+v, err = %v", collections, err)
	}
}

func TestAddCollectionMembersRequiresExistingBoundedCollection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	if err := c.AddCollectionMembers(ctx, "missing", []CollectionMember{{Kind: "repository", Ref: "o/r"}}); err == nil {
		t.Fatal("expected missing collection error")
	}
	if _, err := c.SaveCollection(ctx, "saved"); err != nil {
		t.Fatal(err)
	}
	oversized := make([]CollectionMember, maxCollectionBatchSize+1)
	if err := c.AddCollectionMembers(ctx, "saved", oversized); err == nil {
		t.Fatal("expected oversized batch error")
	}
}
