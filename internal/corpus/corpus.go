package corpus

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

var openLeaseHandoff = func(string) error { return nil }

// Corpus is a durable, product-owned SQLite archive for GitHub repositories
// and threads. It stores immutable observations and separately maintained
// current projections, runs, coverage facts, and FTS5 search indexes.
type Corpus struct {
	db    *sql.DB
	lease *corpusLease
}

// MigrationRequiredError reports that a corpus must be migrated before the
// requested operation can use the current schema. Read-only callers can
// inspect this error without granting migration authority.
type MigrationRequiredError struct {
	Current int64
	Target  int64
}

func (e *MigrationRequiredError) Error() string {
	return fmt.Sprintf("database schema migration required: current version %d, target version %d; inspect with `gitcontribute corpus inspect`, then run `gitcontribute corpus migrate --yes`", e.Current, e.Target)
}

// UnsupportedSchemaError reports that the corpus was written by a newer
// binary. Retaining an older executable is not sufficient for rollback once
// the durable schema has advanced.
type UnsupportedSchemaError struct {
	Current int64
	Target  int64
}

func (e *UnsupportedSchemaError) Error() string {
	return fmt.Sprintf("database schema version %d is newer than this binary supports (%d)", e.Current, e.Target)
}

// Open opens or creates a corpus at path, applies pending migrations, and
// enables WAL, foreign keys, and a busy timeout. The returned Corpus is safe
// for concurrent use by a single writer with multiple readers.
func Open(ctx context.Context, path string) (_ *Corpus, returnErr error) {
	needsInitialization, target, err := inspectOpenRequest(ctx, path)
	if err != nil {
		return nil, err
	}
	lease, err := acquireCorpusLease(path, needsInitialization, map[bool]string{true: "initialize corpus", false: "open corpus"}[needsInitialization])
	if err != nil {
		return nil, err
	}
	defer func() {
		returnErr = errors.Join(returnErr, lease.release())
	}()
	// Recheck compatibility while holding the process lease. A restore may have
	// replaced the file between the initial side-effect-free inspection and
	// lease acquisition; ordinary writable open must never gain migration
	// authority through that race.
	needsInitialization, err = inspectOpenRequestUnderLease(ctx, path, target, needsInitialization)
	if err != nil {
		return nil, err
	}
	c, err := openWritableCorpus(ctx, path, lease, needsInitialization)
	if err != nil {
		return nil, err
	}
	if needsInitialization && lease.lock != nil {
		c, lease, err = handoffInitializedCorpus(ctx, path, c, lease)
		if err != nil {
			return nil, err
		}
	}
	lease = nil
	return c, nil
}

func inspectOpenRequest(ctx context.Context, path string) (bool, int64, error) {
	version, exists, err := InspectSchemaVersion(ctx, path)
	if err != nil {
		return false, 0, err
	}
	target, err := latestSchemaVersion()
	if err != nil {
		return false, 0, err
	}
	if err := checkSchemaCompatibility(version, target, exists); err != nil {
		return false, 0, err
	}
	return !exists, target, nil
}

func inspectOpenRequestUnderLease(ctx context.Context, path string, target int64, needsInitialization bool) (bool, error) {
	version, exists, err := InspectSchemaVersion(ctx, path)
	if err != nil {
		return false, err
	}
	if !exists {
		return needsInitialization, nil
	}
	return false, checkSchemaCompatibility(version, target, true)
}

func checkSchemaCompatibility(current, target int64, exists bool) error {
	if !exists {
		return nil
	}
	if current < target {
		return &MigrationRequiredError{Current: current, Target: target}
	}
	if current > target {
		return &UnsupportedSchemaError{Current: current, Target: target}
	}
	return nil
}

func openWritableCorpus(ctx context.Context, path string, lease *corpusLease, needsInitialization bool) (*Corpus, error) {
	if err := prepareDatabaseFile(path); err != nil {
		return nil, err
	}
	db, err := openDatabase(path)
	if err != nil {
		return nil, err
	}

	c := &Corpus{db: db, lease: lease}
	if needsInitialization {
		if err := c.applyMigrations(ctx); err != nil {
			return nil, errors.Join(err, db.Close())
		}
	} else if err := validateOpenCorpusSchema(ctx, c); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	if err := c.verifyPragmas(ctx); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return c, nil
}

func validateOpenCorpusSchema(ctx context.Context, c *Corpus) error {
	current, target, err := c.inspectSchemaVersions(ctx)
	if err != nil {
		return err
	}
	return checkSchemaCompatibility(current, target, true)
}

func handoffInitializedCorpus(ctx context.Context, path string, c *Corpus, lease *corpusLease) (*Corpus, *corpusLease, error) {
	// Do not retain a handle to the inode initialized under the exclusive
	// lease. Restore may replace the path while this process waits to reacquire
	// a shared lease, so close first and reopen only after that lease is held.
	if err := c.db.Close(); err != nil {
		return nil, lease, fmt.Errorf("close initialized corpus before lease handoff: %w", err)
	}
	c.db = nil
	if err := lease.release(); err != nil {
		return nil, lease, fmt.Errorf("release migration lease: %w", err)
	}
	lease = nil
	if err := openLeaseHandoff(path); err != nil {
		return nil, nil, fmt.Errorf("complete corpus lease handoff: %w", err)
	}
	lease, err := acquireCorpusLease(path, false, "open corpus")
	if err != nil {
		return nil, nil, err
	}
	db, err := openDatabase(path)
	if err != nil {
		return nil, lease, err
	}
	c = &Corpus{db: db, lease: lease}
	if err := validateOpenCorpusSchema(ctx, c); err != nil {
		return nil, lease, errors.Join(err, db.Close())
	}
	return c, lease, nil
}

func openDatabase(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", buildDSN(path))
	if err != nil {
		return nil, fmt.Errorf("open corpus db: %w", err)
	}
	// SQLite gives the best durability guarantees with a single open connection.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	return db, nil
}

// OpenReadOnly opens an existing, current corpus without creating files or
// applying migrations. It returns a typed compatibility error when migration
// or a newer binary is required.
func OpenReadOnly(ctx context.Context, path string) (_ *Corpus, returnErr error) {
	filePath, dsn, inspectable, err := schemaInspectionTarget(path)
	if err != nil {
		return nil, err
	}
	if !inspectable {
		return nil, errors.New("read-only corpus requires a persistent database path")
	}
	if _, err := os.Stat(filePath); err != nil {
		return nil, fmt.Errorf("inspect read-only corpus: %w", err)
	}
	current, exists, err := InspectSchemaVersion(ctx, path)
	if err != nil {
		return nil, err
	}
	target, err := latestSchemaVersion()
	if err != nil {
		return nil, err
	}
	if !exists || current < target {
		return nil, &MigrationRequiredError{Current: current, Target: target}
	}
	if current > target {
		return nil, &UnsupportedSchemaError{Current: current, Target: target}
	}
	lease, err := acquireCorpusLease(path, false, "open corpus read-only")
	if err != nil {
		return nil, err
	}
	release := true
	defer func() {
		if release {
			returnErr = errors.Join(returnErr, lease.release())
		}
	}()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open corpus read-only: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	c := &Corpus{db: db, lease: lease}
	current, target, err = c.inspectSchemaVersions(ctx)
	if err != nil {
		return nil, errors.Join(err, db.Close())
	}
	switch {
	case current < target:
		return nil, errors.Join(&MigrationRequiredError{Current: current, Target: target}, db.Close())
	case current > target:
		return nil, errors.Join(&UnsupportedSchemaError{Current: current, Target: target}, db.Close())
	}
	release = false
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
	return errors.Join(c.db.Close(), c.lease.release())
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
	return c.ApplyMigrations(ctx, nil)
}

func (c *Corpus) migrationProvider() (*goose.Provider, *migrationLogger, error) {
	migrations, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return nil, nil, fmt.Errorf("open migration filesystem: %w", err)
	}
	logger := &migrationLogger{}
	provider, err := goose.NewProvider(goose.DialectSQLite3, c.db, migrations, goose.WithLogger(logger))
	if err != nil {
		return nil, nil, fmt.Errorf("create migration provider: %w", err)
	}
	if fatalErr := logger.Err(); fatalErr != nil {
		return nil, nil, fmt.Errorf("create migration provider: %w", fatalErr)
	}
	return provider, logger, nil
}

type migrationLogger struct {
	mu  sync.Mutex
	err error
}

func (*migrationLogger) Printf(_ string, _ ...interface{}) {}

func (l *migrationLogger) Fatalf(format string, v ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err == nil {
		l.err = fmt.Errorf(format, v...)
	}
}

func (l *migrationLogger) Err() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.err
}

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
