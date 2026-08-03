package corpus

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestBaselineMigrationCreatesCurrentSchema(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "corpus.db")
	c, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	defer func() { _ = c.Close() }()

	for _, table := range []string{
		"repositories", "repository_observations", "threads", "thread_observations",
		"facet_coverage", "facet_observations", "code_snapshots", "code_documents",
		"threads_fts", "facet_observations_fts", "code_documents_fts", "projection_states",
		"investigations", "opportunities", "workspaces", "dossiers", "cluster_runs", "clusters",
		"contribution_manifests",
		"concerns", "concern_links", "concerns_fts",
		"validation_run_groups",
		"code_index_artifacts", "corpus_snapshot_tokens", "corpus_read_artifacts",
		"pull_request_feedback_discovery", "pull_request_feedback_projection", "pull_request_feedback_fts",
		"actors", "actor_aliases", "actor_observations", "actor_profiles", "actor_social_accounts",
		"actor_organization_memberships", "actor_pinned_items", "actor_repository_affiliations",
		"actor_contribution_periods", "actor_contribution_days", "actor_contribution_items",
		"actor_repository_contribution_totals", "actors_fts",
	} {
		if !migrationTableExists(ctx, t, c.db, table) {
			t.Fatalf("table %s missing after baseline migration", table)
		}
	}
	for _, table := range []string{
		"actors", "actor_aliases", "actor_observations", "actor_profiles", "actor_social_accounts",
		"actor_organization_memberships", "actor_pinned_items", "actor_repository_affiliations",
		"actor_contribution_periods", "actor_contribution_days", "actor_contribution_items",
		"actor_repository_contribution_totals",
	} {
		for _, suffix := range []string{"ai", "au", "ad"} {
			trigger := "corpus_revision_" + table + "_" + suffix
			if !migrationTriggerExists(ctx, t, c.db, trigger) {
				t.Fatalf("trigger %s missing after baseline migration", trigger)
			}
		}
	}

	for _, col := range []string{"merged_known", "author_association", "assignees", "draft", "locked", "state_reason", "milestone"} {
		if !migrationColumnExists(ctx, t, c.db, "threads", col) {
			t.Fatalf("column threads.%s missing after baseline migration", col)
		}
	}

	if !migrationColumnExists(ctx, t, c.db, "projection_states", "source_revision") {
		t.Fatal("projection_states.source_revision missing after baseline migration")
	}
	if !migrationColumnExists(ctx, t, c.db, "projection_states", "content_hash") {
		t.Fatal("projection_states.content_hash missing after baseline migration")
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

func migrationTableExists(ctx context.Context, t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var found int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&found); err != nil {
		t.Fatalf("query migration table %s: %v", table, err)
	}
	return found == 1
}

func migrationColumnExists(ctx context.Context, t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	var found int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info(?) WHERE name=?`, table, column).Scan(&found); err != nil {
		t.Fatalf("query migration column %s.%s: %v", table, column, err)
	}
	return found == 1
}

func migrationTriggerExists(ctx context.Context, t *testing.T, db *sql.DB, trigger string) bool {
	t.Helper()
	var found int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name=?`, trigger).Scan(&found); err != nil {
		t.Fatalf("query migration trigger %s: %v", trigger, err)
	}
	return found == 1
}
