package corpus

import (
	"context"
	"testing"
	"time"
)

func TestActorMigrationDeduplicatesExistingLoginsCaseInsensitively(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	provider, logger, err := c.migrationProvider()
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := provider.Down(ctx); err != nil {
			t.Fatal(err)
		}
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
