package corpus

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestMigration025BackfillsSearchableFacetEvidence(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "corpus.db")
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, sub, goose.WithLogger(noopLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 24); err != nil {
		t.Fatal(err)
	}

	mustExec := func(query string, args ...any) sql.Result {
		t.Helper()
		result, execErr := db.ExecContext(ctx, query, args...)
		if execErr != nil {
			t.Fatal(execErr)
		}
		return result
	}
	repoResult := mustExec(`INSERT INTO repositories (owner, name, source_updated_at, observation_sequence, created_at, updated_at) VALUES ('owner', 'repo', 1, 1, 1, 1)`)
	repoID, _ := repoResult.LastInsertId()
	threadResult := mustExec(`INSERT INTO threads (repository_id, kind, number, state, title, source_updated_at, observation_sequence, created_at, updated_at) VALUES (?, 'issue', 1, 'open', 'plain', 1, 1, 1, 1)`, repoID)
	threadID, _ := threadResult.LastInsertId()
	mustExec(`INSERT INTO facet_observations (repository_id, thread_id, facet, source_updated_at, observation_sequence, payload, observed_at) VALUES (?, ?, 'issue_comments', 2, 2, '[{"Author":"alice","Body":"transport boundary"}]', 2)`, repoID, threadID)
	mustExec(`INSERT INTO facet_observations (repository_id, thread_id, facet, source_updated_at, observation_sequence, payload, observed_at) VALUES (?, ?, 'issue_comments', 3, 3, '[{"Author":"bob","Body":"pagination invariant"}]', 3)`, repoID, threadID)
	mustExec(`INSERT INTO facet_observations (repository_id, thread_id, facet, source_updated_at, observation_sequence, payload, observed_at) VALUES (?, ?, 'pr_details', 4, 4, '{"Body":"adaptermetadata"}', 4)`, repoID, threadID)
	mustExec(`INSERT INTO facet_observations (repository_id, thread_id, facet, source_updated_at, observation_sequence, payload, observed_at) VALUES (?, ?, 'issue_comments', 4, 5, '{}', 4)`, repoID, threadID)

	if _, err := provider.UpTo(ctx, 25); err != nil {
		t.Fatalf("migrate up to 025: %v", err)
	}
	if !migrationColumnExists(ctx, t, db, "facet_observations", "search_text") || !migrationTableExists(ctx, t, db, "facet_observations_fts") {
		t.Fatal("searchable facet schema missing after migration 025")
	}
	var matches int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT fo.thread_id)
		FROM facet_observations_fts
		JOIN facet_observations fo ON fo.id = facet_observations_fts.rowid
		WHERE facet_observations_fts MATCH '"transport" "invariant"'`).Scan(&matches); err != nil {
		t.Fatal(err)
	}
	if matches != 1 {
		t.Fatalf("backfilled cross-page matches = %d, want 1", matches)
	}
	var documents int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM facet_observations WHERE facet='issue_comments' AND search_text<>''`).Scan(&documents); err != nil {
		t.Fatal(err)
	}
	if documents != 1 {
		t.Fatalf("search documents for paginated facet = %d, want 1", documents)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM facet_observations_fts WHERE facet_observations_fts MATCH 'adaptermetadata'`).Scan(&matches); err != nil {
		t.Fatal(err)
	}
	if matches != 0 {
		t.Fatalf("unsupported serialized payload became searchable: matches=%d", matches)
	}

	inserted := mustExec(`INSERT INTO facet_observations (repository_id, thread_id, facet, source_updated_at, observation_sequence, payload, search_text, observed_at) VALUES (?, ?, 'issue_comments', 5, 5, '[]', 'fresh searchable evidence', 5)`, repoID, threadID)
	insertedID, _ := inserted.LastInsertId()
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM facet_observations_fts WHERE facet_observations_fts MATCH 'fresh'`).Scan(&matches); err != nil || matches != 1 {
		t.Fatalf("insert trigger matches=%d err=%v", matches, err)
	}
	mustExec(`DELETE FROM facet_observations WHERE id=?`, insertedID)
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM facet_observations_fts WHERE facet_observations_fts MATCH 'fresh'`).Scan(&matches); err != nil || matches != 0 {
		t.Fatalf("delete trigger matches=%d err=%v", matches, err)
	}

	if _, err := provider.DownTo(ctx, 24); err != nil {
		t.Fatalf("migrate down from 025: %v", err)
	}
	if migrationColumnExists(ctx, t, db, "facet_observations", "search_text") || migrationTableExists(ctx, t, db, "facet_observations_fts") {
		t.Fatal("searchable facet schema remains after migration 025 down")
	}
	if _, err := provider.UpTo(ctx, 25); err != nil {
		t.Fatalf("migrate up to 025 again: %v", err)
	}
	if !migrationColumnExists(ctx, t, db, "facet_observations", "search_text") || !migrationTableExists(ctx, t, db, "facet_observations_fts") {
		t.Fatal("searchable facet schema missing after second migration 025 up")
	}
}
