package corpus

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestMigration015DownAndUp(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "corpus.db")
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		t.Fatalf("open migration filesystem: %v", err)
	}

	provider, err := goose.NewProvider(goose.DialectSQLite3, db, sub, goose.WithLogger(noopLogger{}))
	if err != nil {
		t.Fatalf("create migration provider: %v", err)
	}

	if _, err := provider.UpTo(ctx, 15); err != nil {
		t.Fatalf("migrate up to 015: %v", err)
	}

	cols := []string{"author_association", "assignees", "draft", "locked", "state_reason", "milestone"}
	if !threadsHaveColumns(t, ctx, db, cols) {
		t.Fatal("expected 015 thread metadata columns after up")
	}

	if _, err := provider.DownTo(ctx, 14); err != nil {
		t.Fatalf("migrate down from 015: %v", err)
	}

	if threadsHaveColumns(t, ctx, db, cols) {
		t.Fatal("expected 015 thread metadata columns to be removed after down")
	}

	if _, err := provider.UpTo(ctx, 15); err != nil {
		t.Fatalf("migrate up to 015 again: %v", err)
	}

	if !threadsHaveColumns(t, ctx, db, cols) {
		t.Fatal("expected 015 thread metadata columns after second up")
	}
}

func TestMigration019DownAndUp(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "corpus.db")
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		t.Fatalf("open migration filesystem: %v", err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, sub, goose.WithLogger(noopLogger{}))
	if err != nil {
		t.Fatalf("create migration provider: %v", err)
	}
	if _, err := provider.UpTo(ctx, 18); err != nil {
		t.Fatalf("migrate up to 018: %v", err)
	}
	if investigationsHaveOriginKey(ctx, t, db) {
		t.Fatal("origin_key exists before migration 019")
	}
	if _, err := provider.UpTo(ctx, 19); err != nil {
		t.Fatalf("migrate up to 019: %v", err)
	}
	if !investigationsHaveOriginKey(ctx, t, db) {
		t.Fatal("origin_key missing after migration 019")
	}
	if _, err := provider.DownTo(ctx, 18); err != nil {
		t.Fatalf("migrate down from 019: %v", err)
	}
	if investigationsHaveOriginKey(ctx, t, db) {
		t.Fatal("origin_key remains after migration 019 down")
	}
	if _, err := provider.UpTo(ctx, 19); err != nil {
		t.Fatalf("migrate up to 019 again: %v", err)
	}
	if !investigationsHaveOriginKey(ctx, t, db) {
		t.Fatal("origin_key missing after second migration 019 up")
	}
}

func TestMigration020DownAndUp(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "corpus.db")
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		t.Fatalf("open migration filesystem: %v", err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, sub, goose.WithLogger(noopLogger{}))
	if err != nil {
		t.Fatalf("create migration provider: %v", err)
	}
	if _, err := provider.UpTo(ctx, 19); err != nil {
		t.Fatalf("migrate up to 019: %v", err)
	}
	if evidenceHasSourceProvenance(ctx, t, db) {
		t.Fatal("source_provenance exists before migration 020")
	}
	if _, err := provider.UpTo(ctx, 20); err != nil {
		t.Fatalf("migrate up to 020: %v", err)
	}
	if !evidenceHasSourceProvenance(ctx, t, db) {
		t.Fatal("source_provenance missing after migration 020")
	}
	if _, err := provider.DownTo(ctx, 19); err != nil {
		t.Fatalf("migrate down from 020: %v", err)
	}
	if evidenceHasSourceProvenance(ctx, t, db) {
		t.Fatal("source_provenance remains after migration 020 down")
	}
	if _, err := provider.UpTo(ctx, 20); err != nil {
		t.Fatalf("migrate up to 020 again: %v", err)
	}
	if !evidenceHasSourceProvenance(ctx, t, db) {
		t.Fatal("source_provenance missing after second migration 020 up")
	}
}

func TestMigration021DownAndUp(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "corpus.db")
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		t.Fatalf("open migration filesystem: %v", err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, sub, goose.WithLogger(noopLogger{}))
	if err != nil {
		t.Fatalf("create migration provider: %v", err)
	}
	if _, err := provider.UpTo(ctx, 20); err != nil {
		t.Fatalf("migrate up to 020: %v", err)
	}
	if migrationTableExists(ctx, t, db, "portfolio_links") {
		t.Fatal("portfolio tables exist before migration 021")
	}
	if _, err := provider.UpTo(ctx, 21); err != nil {
		t.Fatalf("migrate up to 021: %v", err)
	}
	for _, table := range []string{"portfolio_links", "portfolio_signal_snapshots", "portfolio_signals", "portfolio_signal_projections", "resolution_records", "resolution_projections"} {
		if !migrationTableExists(ctx, t, db, table) {
			t.Fatalf("%s missing after migration 021", table)
		}
	}
	if _, err := provider.DownTo(ctx, 20); err != nil {
		t.Fatalf("migrate down from 021: %v", err)
	}
	if migrationTableExists(ctx, t, db, "portfolio_links") || migrationTableExists(ctx, t, db, "resolution_records") {
		t.Fatal("migration 021 tables remain after down")
	}
	if _, err := provider.UpTo(ctx, 21); err != nil {
		t.Fatalf("migrate up to 021 again: %v", err)
	}
	if !migrationTableExists(ctx, t, db, "portfolio_links") || !migrationTableExists(ctx, t, db, "resolution_records") {
		t.Fatal("migration 021 tables missing after second up")
	}
}

func TestMigration022BackfillsCurrentClusterProjection(t *testing.T) {
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
	if _, err := provider.UpTo(ctx, 21); err != nil {
		t.Fatal(err)
	}
	result, err := db.ExecContext(ctx, `INSERT INTO cluster_runs
		(repo_owner, repo_name, source_revision, source_window_start, source_window_end, params_hash, status, started_at, completed_at)
		VALUES ('acme', 'rocket', 'source-a', 0, 0, 'params', 'completed', 1, 2)`)
	if err != nil {
		t.Fatal(err)
	}
	runID, _ := result.LastInsertId()
	if _, err := provider.UpTo(ctx, 22); err != nil {
		t.Fatalf("migrate up to 022: %v", err)
	}
	var gotRun int64
	var source, rule string
	if err := db.QueryRowContext(ctx, `SELECT current_run_id, source_revision, rule_version FROM cluster_projection_state WHERE repo_owner='acme' AND repo_name='rocket'`).Scan(&gotRun, &source, &rule); err != nil {
		t.Fatal(err)
	}
	if gotRun != runID || source != "source-a" || rule != "duplicate-v1" {
		t.Fatalf("projection = run %d source %q rule %q", gotRun, source, rule)
	}
	if _, err := provider.DownTo(ctx, 21); err != nil {
		t.Fatalf("migrate down from 022: %v", err)
	}
	if migrationTableExists(ctx, t, db, "cluster_projection_state") {
		t.Fatal("projection state remains after down")
	}
}

func TestMigration023RemovesLegacyClusterRunColumns(t *testing.T) {
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
	if _, err := provider.UpTo(ctx, 22); err != nil {
		t.Fatal(err)
	}
	if !migrationColumnExists(ctx, t, db, "cluster_runs", "params_hash") || !migrationColumnExists(ctx, t, db, "cluster_runs", "stats") {
		t.Fatal("legacy cluster-run columns missing before migration 023")
	}
	if _, err := provider.UpTo(ctx, 23); err != nil {
		t.Fatalf("migrate up to 023: %v", err)
	}
	if migrationColumnExists(ctx, t, db, "cluster_runs", "params_hash") || migrationColumnExists(ctx, t, db, "cluster_runs", "stats") {
		t.Fatal("legacy cluster-run columns remain after migration 023")
	}
	if _, err := provider.DownTo(ctx, 22); err != nil {
		t.Fatalf("migrate down from 023: %v", err)
	}
	if !migrationColumnExists(ctx, t, db, "cluster_runs", "params_hash") || !migrationColumnExists(ctx, t, db, "cluster_runs", "stats") {
		t.Fatal("legacy cluster-run columns were not restored by migration 023 down")
	}
}

func TestMigration024BackfillsOnlyObservedMergeState(t *testing.T) {
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
	if _, err := provider.UpTo(ctx, 23); err != nil {
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
	result := mustExec(`INSERT INTO repositories (owner, name, source_updated_at, observation_sequence, created_at, updated_at) VALUES ('owner', 'repo', 1, 1, 1, 1)`)
	repoID, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	threadIDs := make([]int64, 5)
	for index := range threadIDs {
		result = mustExec(`INSERT INTO threads (repository_id, kind, number, state, title, source_updated_at, observation_sequence, created_at, updated_at) VALUES (?, 'pull_request', ?, 'closed', 'pr', 1, ?, 1, 1)`, repoID, index+1, index+1)
		threadIDs[index], err = result.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
	}
	mustExec(`INSERT INTO thread_observations (thread_id, source_updated_at, observation_sequence, payload, observed_at) VALUES (?, 1, 4, '{"Merged":false}', 1)`, threadIDs[1])
	mustExec(`INSERT INTO facet_coverage (repository_id, thread_id, facet, source_updated_at, observation_sequence, complete, updated_at) VALUES (?, ?, 'pr_details', 1, 5, 1, 1)`, repoID, threadIDs[2])
	mustExec(`INSERT INTO facet_observations (repository_id, thread_id, facet, source_updated_at, observation_sequence, payload, observed_at) VALUES (?, ?, 'pr_details', 1, 5, '{"Merged":true}', 1)`, repoID, threadIDs[2])
	mustExec(`UPDATE threads SET merged = 1 WHERE id = ?`, threadIDs[3])
	mustExec(`INSERT INTO facet_coverage (repository_id, thread_id, facet, source_updated_at, observation_sequence, complete, updated_at) VALUES (?, ?, 'pr_details', 1, 6, 0, 1)`, repoID, threadIDs[4])
	mustExec(`INSERT INTO facet_observations (repository_id, thread_id, facet, source_updated_at, observation_sequence, payload, observed_at) VALUES (?, ?, 'pr_details', 1, 6, '{"Merged":false}', 1)`, repoID, threadIDs[4])

	if _, err := provider.UpTo(ctx, 24); err != nil {
		t.Fatalf("migrate up to 024: %v", err)
	}
	rows, err := db.QueryContext(ctx, `SELECT merged, merged_known FROM threads ORDER BY number`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	want := [][2]int{{0, 0}, {0, 1}, {1, 1}, {1, 1}, {0, 0}}
	index := 0
	for rows.Next() {
		var got [2]int
		if err := rows.Scan(&got[0], &got[1]); err != nil {
			t.Fatal(err)
		}
		if index >= len(want) || got != want[index] {
			t.Fatalf("row %d merge state = %v, want %v", index, got, want[index])
		}
		index++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if index != len(want) {
		t.Fatalf("merge-state rows = %d, want %d", index, len(want))
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 23); err != nil {
		t.Fatalf("migrate down from 024: %v", err)
	}
	if migrationColumnExists(ctx, t, db, "threads", "merged_known") {
		t.Fatal("merged_known remains after migration 024 down")
	}
}

func threadsHaveColumns(t *testing.T, ctx context.Context, db *sql.DB, cols []string) bool {
	t.Helper()
	for _, col := range cols {
		var found int
		err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('threads') WHERE name=?`, col).Scan(&found)
		if err != nil {
			t.Fatalf("query columns: %v", err)
		}
		if found != 1 {
			return false
		}
	}
	return true
}

func investigationsHaveOriginKey(ctx context.Context, t *testing.T, db *sql.DB) bool {
	t.Helper()
	var found int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('investigations') WHERE name='origin_key'`).Scan(&found); err != nil {
		t.Fatalf("query investigation columns: %v", err)
	}
	return found == 1
}

func evidenceHasSourceProvenance(ctx context.Context, t *testing.T, db *sql.DB) bool {
	t.Helper()
	var found int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('evidence') WHERE name='source_provenance'`).Scan(&found); err != nil {
		t.Fatalf("query evidence columns: %v", err)
	}
	return found == 1
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
