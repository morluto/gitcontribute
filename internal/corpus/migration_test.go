package corpus

import (
	"context"
	"testing"
	"time"
)

func TestRetireFrontierMigrationDropsObsoleteQueueAndRollsBackSchema(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	provider, logger, err := c.migrationProvider()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 15); err != nil {
		t.Fatal(err)
	}
	if err := logger.Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.ExecContext(ctx, `
		INSERT INTO frontier_items (work_key, subject_kind, created_at, updated_at)
		VALUES ('legacy-work', 'repository', 1, 1)
	`); err != nil {
		t.Fatalf("seed legacy frontier: %v", err)
	}

	if _, err := provider.UpTo(ctx, 16); err != nil {
		t.Fatal(err)
	}
	if err := logger.Err(); err != nil {
		t.Fatal(err)
	}
	exists, err := c.tableExists(ctx, "frontier_items")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("frontier_items still exists after migration")
	}

	if _, err := provider.Down(ctx); err != nil {
		t.Fatal(err)
	}
	if err := logger.Err(); err != nil {
		t.Fatal(err)
	}
	exists, err = c.tableExists(ctx, "frontier_items")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("frontier_items was not recreated by schema rollback")
	}
	var count int
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM frontier_items`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rolled-back frontier contains %d rows, want empty legacy schema", count)
	}
}

func TestActorMigrationDeduplicatesExistingLoginsCaseInsensitively(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	provider, logger, err := c.migrationProvider()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 13); err != nil {
		t.Fatal(err)
	}
	if err := logger.Err(); err != nil {
		t.Fatal(err)
	}
	for _, owner := range []string{"Mona", "mona"} {
		if _, err := c.ApplyRepositoryObservation(ctx, owner, "repo-"+owner, "", time.Unix(1, 0).UTC(), `{}`); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := provider.UpTo(ctx, 14); err != nil {
		t.Fatal(err)
	}
	if err := logger.Err(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM actors WHERE lower(current_login)='mona'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("case-insensitive actor count = %d, want 1", count)
	}
}
