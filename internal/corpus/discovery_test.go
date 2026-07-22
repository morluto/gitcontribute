package corpus

import (
	"context"
	"testing"
	"time"
)

func TestDiscoveryCheckpointDoesNotMoveBackwards(t *testing.T) {
	t.Parallel()
	c, _ := openTestCorpus(t)
	ctx := context.Background()
	newer := time.Unix(200, 222).UTC()
	older := time.Unix(100, 111).UTC()
	if err := c.SetTime(ctx, "source", newer); err != nil {
		t.Fatal(err)
	}
	if err := c.SetTime(ctx, "source", older); err != nil {
		t.Fatal(err)
	}
	got, ok, err := c.GetTime(ctx, "source")
	if err != nil || !ok || !got.Equal(newer) {
		t.Fatalf("GetTime = (%v, %v, %v), want %v", got, ok, err, newer)
	}
}

func TestArchiveImportIsIdempotent(t *testing.T) {
	t.Parallel()
	c, _ := openTestCorpus(t)
	ctx := context.Background()
	for range 2 {
		if err := c.MarkImported(ctx, "2026010101"); err != nil {
			t.Fatal(err)
		}
	}
	imported, err := c.IsImported(ctx, "2026010101")
	if err != nil || !imported {
		t.Fatalf("IsImported = (%v, %v)", imported, err)
	}
	var count int
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM archive_imports`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("archive import rows = %d, want 1", count)
	}
}

func TestDiscoverySourcesAndPartitionsPersist(t *testing.T) {
	t.Parallel()
	c, _ := openTestCorpus(t)
	ctx := context.Background()
	source, err := c.SaveDiscoverySource(ctx, DiscoverySource{Name: "go", Kind: "search", Definition: `{"query":"language:go"}`, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.RecordSourcePartition(ctx, SourcePartition{
		SourceID: source.ID, Key: "created:1:2", Query: "language:go created:1..2",
		Qualifier: "created", Start: time.Unix(1, 0), End: time.Unix(2, 0), Total: 210, Pages: 3, ObservedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	sources, err := c.ListDiscoverySources(ctx)
	if err != nil || len(sources) != 1 || sources[0].Name != "go" {
		t.Fatalf("ListDiscoverySources = (%+v, %v)", sources, err)
	}
	var count int
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM source_partitions WHERE source_id=?`, source.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("source partitions = %d", count)
	}
	var pages int
	if err := c.db.QueryRowContext(ctx, `SELECT pages FROM source_partitions WHERE source_id=?`, source.ID).Scan(&pages); err != nil {
		t.Fatal(err)
	}
	if pages != 3 {
		t.Fatalf("source partition pages = %d, want 3", pages)
	}
}
