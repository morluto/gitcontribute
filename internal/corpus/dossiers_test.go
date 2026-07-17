package corpus

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

func TestDossiersMigration(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	rows, err := c.db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name IN ('dossiers', 'dossier_sources') ORDER BY name`)
	if err != nil {
		t.Fatalf("query tables: %v", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "dossier_sources" || names[1] != "dossiers" {
		t.Fatalf("expected dossier tables, got %v", names)
	}

	for _, col := range []string{"id", "repository_id", "commit_sha", "as_of", "section_metadata", "snapshot", "generated_at", "created_at"} {
		var found int
		if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('dossiers') WHERE name=?`, col).Scan(&found); err != nil {
			t.Fatalf("pragma dossiers %s: %v", col, err)
		}
		if found != 1 {
			t.Fatalf("dossiers missing column %s", col)
		}
	}

	for _, col := range []string{"id", "dossier_id", "source", "url", "commit_sha", "observed_at", "as_of"} {
		var found int
		if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('dossier_sources') WHERE name=?`, col).Scan(&found); err != nil {
			t.Fatalf("pragma dossier_sources %s: %v", col, err)
		}
		if found != 1 {
			t.Fatalf("dossier_sources missing column %s", col)
		}
	}
}

func TestDossierSaveGetListAndRefresh(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	repo, err := c.UpsertRepository(ctx, Repository{
		Owner:           "owner",
		Name:            "repo",
		SourceUpdatedAt: time.Unix(1000, 0).UTC(),
	}, `{}`)
	if err != nil {
		t.Fatalf("upsert repository: %v", err)
	}

	sources := []domain.SourceRef{
		{Source: "github:rest", URL: "https://api.github.com/repos/owner/repo", ObservedAt: time.Unix(1000, 0).UTC(), AsOf: time.Unix(1000, 0).UTC()},
		{Source: "github:graphql", URL: "https://api.github.com/graphql", ObservedAt: time.Unix(1000, 0).UTC(), AsOf: time.Unix(1000, 0).UTC()},
	}

	asOf := time.Unix(2000, 0).UTC()
	snapshot := `{"commit_sha":"abc123"}`
	sectionMeta := `{"recent_limit":10}`
	generated := time.Unix(3000, 0).UTC()

	id, err := c.SaveDossier(ctx, repo.ID, repo.Owner, repo.Name, "abc123", asOf, sectionMeta, snapshot, generated, sources)
	if err != nil {
		t.Fatalf("save dossier: %v", err)
	}

	record, gotSources, err := c.GetDossier(ctx, repo.Owner, repo.Name)
	if err != nil {
		t.Fatalf("get dossier: %v", err)
	}
	if record == nil {
		t.Fatal("expected dossier record")
	}
	if record.ID != id {
		t.Fatalf("record id = %d, want %d", record.ID, id)
	}
	if record.CommitSHA != "abc123" || !record.AsOf.Equal(asOf) || record.Snapshot != snapshot || record.SectionMetadata != sectionMeta || !record.GeneratedAt.Equal(generated) {
		t.Fatalf("unexpected record: %+v", record)
	}
	if len(gotSources) != len(sources) {
		t.Fatalf("expected %d sources, got %d", len(sources), len(gotSources))
	}

	list, err := c.ListDossiers(ctx, 10)
	if err != nil {
		t.Fatalf("list dossiers: %v", err)
	}
	if len(list) != 1 || list[0].ID != id {
		t.Fatalf("expected 1 dossier in list, got %+v", list)
	}

	olderAsOf := asOf.Add(-time.Hour)
	olderSnapshot := `{"commit_sha":"old"}`
	refreshedID, inserted, err := c.RefreshDossier(ctx, repo.ID, repo.Owner, repo.Name, "old", olderAsOf, sectionMeta, olderSnapshot, generated, sources)
	if err != nil {
		t.Fatalf("refresh older: %v", err)
	}
	if inserted {
		t.Fatal("refresh with older as_of should not insert")
	}
	if refreshedID != id {
		t.Fatalf("refresh older id = %d, want %d", refreshedID, id)
	}

	newAsOf := asOf.Add(time.Hour)
	newSnapshot := `{"commit_sha":"new"}`
	newGen := generated.Add(time.Hour)
	newID, inserted, err := c.RefreshDossier(ctx, repo.ID, repo.Owner, repo.Name, "new", newAsOf, sectionMeta, newSnapshot, newGen, sources)
	if err != nil {
		t.Fatalf("refresh newer: %v", err)
	}
	if !inserted {
		t.Fatal("refresh with newer as_of should insert")
	}
	if newID == id {
		t.Fatal("expected new dossier id")
	}

	latest, _, err := c.GetDossier(ctx, repo.Owner, repo.Name)
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	if latest.ID != newID {
		t.Fatalf("latest id = %d, want %d", latest.ID, newID)
	}

	dupID, inserted, err := c.RefreshDossier(ctx, repo.ID, repo.Owner, repo.Name, "new", newAsOf, sectionMeta, newSnapshot, newGen, sources)
	if err != nil {
		t.Fatalf("refresh duplicate: %v", err)
	}
	if inserted {
		t.Fatal("duplicate as_of/snapshot should not insert")
	}
	if dupID != newID {
		t.Fatalf("duplicate refresh id = %d, want %d", dupID, newID)
	}
}

func TestDossierSaveAndRefreshAllowSameGeneratedAt(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	repo, err := c.UpsertRepository(ctx, Repository{
		Owner:           "owner",
		Name:            "repo",
		SourceUpdatedAt: time.Unix(1000, 0).UTC(),
	}, `{}`)
	if err != nil {
		t.Fatalf("upsert repository: %v", err)
	}

	generated := time.Unix(3000, 0).UTC()

	id1, err := c.SaveDossier(ctx, repo.ID, repo.Owner, repo.Name, "sha", time.Unix(2000, 0).UTC(), `{}`, `{"i":1}`, generated, nil)
	if err != nil {
		t.Fatalf("save first dossier: %v", err)
	}
	id2, err := c.SaveDossier(ctx, repo.ID, repo.Owner, repo.Name, "sha", time.Unix(3000, 0).UTC(), `{}`, `{"i":2}`, generated, nil)
	if err != nil {
		t.Fatalf("save second dossier with same generated_at: %v", err)
	}
	if id1 == id2 {
		t.Fatal("expected two distinct dossier rows")
	}

	latest, _, err := c.GetDossier(ctx, repo.Owner, repo.Name)
	if err != nil {
		t.Fatalf("get dossier: %v", err)
	}
	if latest == nil || latest.Snapshot != `{"i":2}` {
		t.Fatalf("expected latest dossier snapshot, got %+v", latest)
	}

	id3, inserted, err := c.RefreshDossier(ctx, repo.ID, repo.Owner, repo.Name, "sha", time.Unix(4000, 0).UTC(), `{}`, `{"i":3}`, generated, nil)
	if err != nil {
		t.Fatalf("refresh with same generated_at: %v", err)
	}
	if !inserted {
		t.Fatal("expected refresh with newer as_of to insert even when generated_at is unchanged")
	}
	if id3 == id2 {
		t.Fatal("expected a new dossier row on refresh")
	}
}

func TestDossierListReturnsLatestPerRepository(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	for _, name := range []string{"a", "b"} {
		repo, err := c.UpsertRepository(ctx, Repository{
			Owner:           "owner",
			Name:            name,
			SourceUpdatedAt: time.Unix(1000, 0).UTC(),
		}, `{}`)
		if err != nil {
			t.Fatalf("upsert %s: %v", name, err)
		}
		for i := 0; i < 2; i++ {
			asOf := time.Unix(int64(2000+i*1000), 0).UTC()
			gen := time.Unix(int64(3000+i*1000), 0).UTC()
			if _, err := c.SaveDossier(ctx, repo.ID, repo.Owner, name, "sha", asOf, `{}`, fmt.Sprintf(`{"i":%d}`, i), gen, nil); err != nil {
				t.Fatalf("save %s %d: %v", name, i, err)
			}
		}
	}

	list, err := c.ListDossiers(ctx, 10)
	if err != nil {
		t.Fatalf("list dossiers: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 dossiers, got %d", len(list))
	}
	for _, r := range list {
		if r.Snapshot != `{"i":1}` {
			t.Fatalf("expected latest snapshot for %s/%s, got %s", r.RepoOwner, r.RepoName, r.Snapshot)
		}
	}
}
