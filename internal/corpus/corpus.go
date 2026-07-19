package corpus

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
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
	if err := prepareDatabaseFile(path); err != nil {
		return nil, err
	}
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

func prepareDatabaseFile(path string) error {
	if path == ":memory:" || strings.HasPrefix(path, "file:") || strings.Contains(path, "?") {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("prepare corpus db: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close prepared corpus db: %w", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		return fmt.Errorf("protect corpus db: %w", err)
	}
	return nil
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
	provider, err := c.migrationProvider()
	if err != nil {
		return err
	}
	current, target, err := provider.GetVersions(ctx)
	if err != nil {
		return fmt.Errorf("read migration versions: %w", err)
	}
	if current > target {
		return fmt.Errorf("database schema version %d is newer than this binary supports (%d)", current, target)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	current, target, err = provider.GetVersions(ctx)
	if err != nil {
		return fmt.Errorf("verify migration versions: %w", err)
	}
	if current != target {
		return fmt.Errorf("database schema version %d does not match expected version %d", current, target)
	}
	return nil
}

func (c *Corpus) migrationProvider() (*goose.Provider, error) {
	migrations, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("open migration filesystem: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, c.db, migrations, goose.WithLogger(noopLogger{}))
	if err != nil {
		return nil, fmt.Errorf("create migration provider: %w", err)
	}
	return provider, nil
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

func encodeTime(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UTC().UnixNano()
}

// Status holds corpus health and count metadata.
type Status struct {
	Repositories int
	Threads      int
}

// Status returns the number of repositories and threads in the corpus.
func (c *Corpus) Status(ctx context.Context) (Status, error) {
	var s Status
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM repositories`).Scan(&s.Repositories); err != nil {
		return s, fmt.Errorf("count repositories: %w", err)
	}
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM threads`).Scan(&s.Threads); err != nil {
		return s, fmt.Errorf("count threads: %w", err)
	}
	return s, nil
}

func scanTime(nsec int64) time.Time {
	if nsec == 0 {
		return time.Time{}
	}
	return time.Unix(0, nsec).UTC()
}
