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
