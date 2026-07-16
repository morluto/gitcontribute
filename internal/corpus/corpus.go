package corpus

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Corpus is a durable, product-owned SQLite archive for GitHub repositories
// and threads. It stores immutable observations and separately maintained
// current projections, runs, coverage facts, and an FTS5 thread index.
type Corpus struct {
	db *sql.DB
}

// Open opens or creates a corpus at path, applies pending migrations, and
// enables WAL, foreign keys, and a busy timeout. The returned Corpus is safe
// for concurrent use by a single writer with multiple readers.
func Open(ctx context.Context, path string) (*Corpus, error) {
	dsn := buildDSN(path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open corpus db: %w", err)
	}

	// SQLite gives the best durability guarantees with a single open connection.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	c := &Corpus{db: db}
	if err := c.applyMigrations(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := c.verifyPragmas(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return c, nil
}

// Close closes the underlying database connection.
func (c *Corpus) Close() error {
	return c.db.Close()
}

func buildDSN(path string) string {
	params := []string{
		"_pragma=foreign_keys(1)",
		"_pragma=busy_timeout(5000)",
	}
	if path != ":memory:" && !strings.Contains(path, "?") {
		// WAL is meaningful only for file-backed databases.
		params = append(params, "_pragma=journal_mode(WAL)")
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + strings.Join(params, "&")
}

func (c *Corpus) applyMigrations(ctx context.Context) error {
	goose.SetBaseFS(migrationsFS)
	goose.SetDialect("sqlite3")
	goose.SetLogger(noopLogger{})
	if err := goose.UpContext(ctx, c.db, "migrations"); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

type noopLogger struct{}

func (noopLogger) Printf(format string, v ...interface{}) {}
func (noopLogger) Fatalf(format string, v ...interface{}) { panic(fmt.Sprintf(format, v...)) }

func (c *Corpus) verifyPragmas(ctx context.Context) error {
	var fk int
	if err := c.db.QueryRowContext(ctx, "PRAGMA foreign_keys;").Scan(&fk); err != nil {
		return fmt.Errorf("read foreign_keys pragma: %w", err)
	}
	if fk != 1 {
		return fmt.Errorf("foreign_keys pragma is %d, want 1", fk)
	}

	var busy int
	if err := c.db.QueryRowContext(ctx, "PRAGMA busy_timeout;").Scan(&busy); err != nil {
		return fmt.Errorf("read busy_timeout pragma: %w", err)
	}
	if busy == 0 {
		return fmt.Errorf("busy_timeout pragma is 0")
	}

	var journal string
	if err := c.db.QueryRowContext(ctx, "PRAGMA journal_mode;").Scan(&journal); err != nil {
		return fmt.Errorf("read journal_mode pragma: %w", err)
	}
	// in-memory databases cannot use WAL and will report "memory" or "delete".
	if journal != "wal" && journal != "memory" && journal != "delete" {
		return fmt.Errorf("journal_mode pragma is %q, want wal", journal)
	}
	return nil
}

func (c *Corpus) nextSequence(ctx context.Context, tx *sql.Tx) (int64, error) {
	var seq int64
	err := tx.QueryRowContext(ctx, `
		INSERT INTO observation_sequences (name, next_value)
		VALUES ('observation', 1)
		ON CONFLICT (name) DO UPDATE
		SET next_value = observation_sequences.next_value + 1
		RETURNING next_value
	`).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("next observation sequence: %w", err)
	}
	return seq, nil
}

func scanTime(sec int64) time.Time {
	if sec == 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0).UTC()
}
