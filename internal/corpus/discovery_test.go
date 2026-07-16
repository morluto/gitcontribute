package corpus

import (
	"context"
	"testing"
	"time"
)

func TestDiscoveryCheckpointDoesNotMoveBackwards(t *testing.T) {
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
