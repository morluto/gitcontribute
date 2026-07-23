package corpus

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
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
	} {
		if !migrationTableExists(ctx, t, c.db, table) {
			t.Fatalf("table %s missing after baseline migration", table)
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
