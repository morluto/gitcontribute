package corpus

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
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
	if len(records.Records) != 1 || records.Records[0].Definition.MaxResultsPerRepo != 2 || records.Total != 1 || records.Truncated {
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
		mustRepositoryCollectionMember(t, "octocat/hello-world"),
		mustThreadCollectionMember(t, domain.IssueKind, "octocat/hello-world", 12),
	}
	if err := c.AddCollectionMembers(ctx, "favorites", members); err != nil {
		t.Fatal(err)
	}
	if err := c.AddCollectionMembers(ctx, "favorites", []CollectionMember{
		mustThreadCollectionMember(t, domain.IssueKind, "octocat/hello-world", 12),
		mustThreadCollectionMember(t, domain.PullRequestKind, "octocat/hello-world", 12),
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
	collections, err := c.ListCollections(ctx)
	if err != nil || len(collections.Collections) != 1 || collections.Collections[0].MemberCount != 3 || collections.Total != 1 || collections.Truncated {
		t.Fatalf("collections = %+v, err = %v", collections, err)
	}
}

func TestAddCollectionMembersRequiresExistingBoundedCollection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	if err := c.AddCollectionMembers(ctx, "missing", []CollectionMember{mustRepositoryCollectionMember(t, "o/r")}); err == nil {
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

func TestOrganizeListsExposeHardCapTruncation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := encodeTime(time.Unix(100, 0))
	for i := 0; i <= lensListLimit; i++ {
		name := fmt.Sprintf("lens-%04d", i)
		definition := fmt.Sprintf(`{"name":%q,"weights":{"activity":1}}`, name)
		if _, err := tx.ExecContext(ctx, `INSERT INTO lenses (name, definition, created_at, updated_at) VALUES (?, ?, ?, ?)`, name, definition, now, now); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO collections (name, created_at, updated_at) VALUES (?, ?, ?)`, fmt.Sprintf("collection-%04d", i), now, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	lenses, err := c.ListLenses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(lenses.Records) != lensListLimit || lenses.Total != lensListLimit+1 || !lenses.Truncated {
		t.Fatalf("lenses = returned:%d total:%d truncated:%v", len(lenses.Records), lenses.Total, lenses.Truncated)
	}
	collections, err := c.ListCollections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(collections.Collections) != collectionListLimit || collections.Total != collectionListLimit+1 || !collections.Truncated {
		t.Fatalf("collections = returned:%d total:%d truncated:%v", len(collections.Collections), collections.Total, collections.Truncated)
	}
}

func mustRepositoryCollectionMember(t *testing.T, value string) CollectionMember {
	t.Helper()
	ref, err := domain.ParseRepoRef(value)
	if err != nil {
		t.Fatal(err)
	}
	member, err := NewRepositoryCollectionMember(ref)
	if err != nil {
		t.Fatal(err)
	}
	return member
}

func mustThreadCollectionMember(t *testing.T, kind domain.ThreadKind, repository string, number int) CollectionMember {
	t.Helper()
	ref, err := domain.ParseRepoRef(repository)
	if err != nil {
		t.Fatal(err)
	}
	member, err := NewThreadCollectionMember(kind, ref, number)
	if err != nil {
		t.Fatal(err)
	}
	return member
}
