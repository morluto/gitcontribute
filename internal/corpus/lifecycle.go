package corpus

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
	"modernc.org/sqlite"
)

// SchemaState is the non-mutating compatibility classification for a corpus.
type SchemaState string

const (
	SchemaMissing           SchemaState = "missing"
	SchemaCurrent           SchemaState = "current"
	SchemaMigrationRequired SchemaState = "migration_required"
	SchemaNewer             SchemaState = "newer"
	SchemaDamaged           SchemaState = "damaged"
)

// MigrationStep describes one embedded schema migration without applying it.
type MigrationStep struct {
	Version           int64
	Name              string
	AffectedRows      int64
	EstimateAvailable bool
	Transactional     bool
	Resumable         bool
	ResumeStrategy    string
	ProjectionRebuild bool
}

// SchemaInspection is a read-only migration plan input.
type SchemaInspection struct {
	Path                      string
	Exists                    bool
	SizeBytes                 int64
	WALBytes                  int64
	State                     SchemaState
	Current                   int64
	Target                    int64
	Pending                   []MigrationStep
	Repository                int
	Threads                   int
	Problem                   string
	BackupRequired            bool
	RequiredDiskBytes         uint64
	AvailableDiskBytes        uint64
	ProjectionRebuildRequired bool
}

// MigrationProgress reports stable step boundaries. SQL migration internals
// remain owned by Goose; data-sized migrations should expose their own bounded
// checkpoints rather than pretending statement-level progress is available.
type MigrationProgress struct {
	Phase   string
	Version int64
	Name    string
	Current int64
	Target  int64
}

type MigrationObserver func(MigrationProgress)

// BackupResult identifies a verified, consistent SQLite backup.
type BackupResult struct {
	Path           string
	ManifestPath   string
	SizeBytes      int64
	SHA256         string
	CreatedAt      time.Time
	SourceSchema   int64
	ExpectedSchema int64
	Compatibility  SchemaState
}

const backupManifestVersion = 1

// BackupManifest is stored next to each backup so restore can reject partial,
// corrupted, or incompatible artifacts before touching the live corpus.
type BackupManifest struct {
	FormatVersion  int         `json:"format_version"`
	CreatedAt      time.Time   `json:"created_at"`
	SizeBytes      int64       `json:"size_bytes"`
	SHA256         string      `json:"sha256"`
	SourceSchema   int64       `json:"source_schema"`
	ExpectedSchema int64       `json:"expected_schema"`
	Compatibility  SchemaState `json:"compatibility"`
}

func backupManifestPath(path string) string { return path + ".manifest.json" }

// Migrate opens or creates a persistent corpus, applies pending migrations
// with explicit progress, verifies connection pragmas, and closes it. Callers
// own consent, backup policy, and activation of any dependent runtime.
func Migrate(ctx context.Context, path string, observer MigrationObserver) error {
	lease, err := acquireCorpusLease(path, true, "migrate corpus")
	if err != nil {
		return err
	}
	defer lease.release()
	return migrateWithLease(ctx, path, observer)
}

// MigrateWithBackup holds one exclusive corpus lease from the start of the
// safety backup through migration verification. An empty backup destination
// explicitly opts out of backup creation.
func MigrateWithBackup(ctx context.Context, path, backupDestination string, observer MigrationObserver) (*BackupResult, error) {
	lease, err := acquireCorpusLease(path, true, "back up and migrate corpus")
	if err != nil {
		return nil, err
	}
	defer lease.release()
	var backup *BackupResult
	if strings.TrimSpace(backupDestination) != "" {
		result, err := backupWithLease(ctx, path, backupDestination, nil)
		if err != nil {
			return nil, err
		}
		backup = &result
	}
	if err := migrateWithLease(ctx, path, observer); err != nil {
		return backup, err
	}
	return backup, nil
}

func migrateWithLease(ctx context.Context, path string, observer MigrationObserver) error {
	if err := prepareDatabaseFile(path); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", buildDSN(path))
	if err != nil {
		return fmt.Errorf("open corpus for migration: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	c := &Corpus{db: db}
	if err := c.ApplyMigrations(ctx, observer); err != nil {
		_ = db.Close()
		return err
	}
	if err := c.verifyPragmas(ctx); err != nil {
		_ = db.Close()
		return err
	}
	if err := db.Close(); err != nil {
		return fmt.Errorf("close migrated corpus: %w", err)
	}
	return nil
}

// InspectSchema reports corpus identity, compatibility, and bounded inventory
// without creating or mutating the database.
func InspectSchema(ctx context.Context, path string) (SchemaInspection, error) {
	result := SchemaInspection{Path: path, State: SchemaMissing}
	filePath, dsn, inspectable, err := schemaInspectionTarget(path)
	if err != nil {
		return result, err
	}
	result.Target, err = latestSchemaVersion()
	if err != nil {
		return result, err
	}
	if !inspectable {
		return result, nil
	}
	result.Path = filePath
	result.AvailableDiskBytes, err = availableDiskBytes(filepath.Dir(filePath))
	if err != nil {
		return result, fmt.Errorf("inspect available disk space: %w", err)
	}
	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("inspect corpus file: %w", err)
	}
	result.Exists = true
	result.SizeBytes = info.Size()
	if walInfo, statErr := os.Stat(filePath + "-wal"); statErr == nil {
		result.WALBytes = walInfo.Size()
	} else if !os.IsNotExist(statErr) {
		return result, fmt.Errorf("inspect corpus WAL: %w", statErr)
	}
	lease, err := acquireSharedCorpusLeaseIfPresent(path, "inspect corpus")
	if err != nil {
		return result, err
	}
	defer lease.release()

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return result, fmt.Errorf("open corpus inspection: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	defer db.Close()
	c := &Corpus{db: db}
	result.Current, result.Target, err = c.inspectSchemaVersions(ctx)
	if err != nil {
		if isSQLiteDamage(err) {
			result.State = SchemaDamaged
			result.Problem = err.Error()
			return result, nil
		}
		return result, err
	}
	switch {
	case result.Current < result.Target:
		result.State = SchemaMigrationRequired
	case result.Current > result.Target:
		result.State = SchemaNewer
	default:
		result.State = SchemaCurrent
	}
	result.Pending, err = pendingMigrationSteps(result.Current)
	if err != nil {
		return result, err
	}
	result.Repository, err = countTableIfPresent(ctx, db, "repositories")
	if err != nil {
		if isSQLiteDamage(err) {
			result.State = SchemaDamaged
			result.Problem = err.Error()
			return result, nil
		}
		return result, err
	}
	result.Threads, err = countTableIfPresent(ctx, db, "threads")
	if err != nil {
		if isSQLiteDamage(err) {
			result.State = SchemaDamaged
			result.Problem = err.Error()
			return result, nil
		}
		return result, err
	}
	if err := annotateMigrationSteps(ctx, db, result.Pending); err != nil {
		if isSQLiteDamage(err) {
			result.State = SchemaDamaged
			result.Problem = err.Error()
			return result, nil
		}
		return result, err
	}
	for _, step := range result.Pending {
		result.ProjectionRebuildRequired = result.ProjectionRebuildRequired || step.ProjectionRebuild
	}
	if result.State == SchemaMigrationRequired {
		result.BackupRequired = true
		result.RequiredDiskBytes = migrationDiskRequirement(result.SizeBytes, result.WALBytes)
	}
	return result, nil
}

// migrationDiskRequirement reserves room for the default safety backup and a
// transaction-sized WAL while a migration is in progress. It is deliberately
// conservative: planning must not discover insufficient space after writes
// have started.
func migrationDiskRequirement(databaseBytes, walBytes int64) uint64 {
	var database, wal uint64
	if databaseBytes > 0 {
		database = uint64(databaseBytes)
	}
	if walBytes > 0 {
		wal = uint64(walBytes)
	}
	if database > (math.MaxUint64-wal)/2 {
		return math.MaxUint64
	}
	return database*2 + wal
}

func availableDiskBytes(path string) (uint64, error) {
	for {
		if _, err := os.Stat(path); err == nil {
			return freeDiskBytes(path)
		} else if !os.IsNotExist(err) {
			return 0, err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return 0, fmt.Errorf("no existing parent directory")
		}
		path = parent
	}
}

func annotateMigrationSteps(_ context.Context, _ *sql.DB, steps []MigrationStep) error {
	for i := range steps {
		// Embedded SQL migrations run in one Goose transaction. A cancelled run
		// rolls back the active step, while a later invocation continues after
		// every version that was already committed.
		steps[i].Transactional = true
		steps[i].Resumable = true
		steps[i].ResumeStrategy = "restart_step"
	}
	return nil
}

func isSQLiteDamage(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	// SQLite primary result codes: SQLITE_CORRUPT=11, SQLITE_NOTADB=26.
	switch sqliteErr.Code() & 0xff {
	case 11, 26:
		return true
	default:
		return false
	}
}

func pendingMigrationSteps(current int64) ([]MigrationStep, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("list embedded migrations: %w", err)
	}
	steps := make([]MigrationStep, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		prefix, name, ok := strings.Cut(entry.Name(), "_")
		if !ok {
			continue
		}
		var version int64
		if _, err := fmt.Sscanf(prefix, "%d", &version); err != nil {
			return nil, fmt.Errorf("parse migration version from %q: %w", entry.Name(), err)
		}
		if version > current {
			steps = append(steps, MigrationStep{Version: version, Name: strings.TrimSuffix(name, filepath.Ext(name))})
		}
	}
	sort.Slice(steps, func(i, j int) bool { return steps[i].Version < steps[j].Version })
	return steps, nil
}

func countTableIfPresent(ctx context.Context, db *sql.DB, table string) (int, error) {
	var exists int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&exists); err != nil {
		return 0, fmt.Errorf("inspect table %s: %w", table, err)
	}
	if exists == 0 {
		return 0, nil
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil {
		return 0, fmt.Errorf("count table %s: %w", table, err)
	}
	return count, nil
}

// ApplyMigrations applies pending migrations one at a time and reports stable
// step boundaries. The caller owns authorization, backup, and process leases.
func (c *Corpus) ApplyMigrations(ctx context.Context, observer MigrationObserver) error {
	provider, logger, err := c.migrationProvider()
	if err != nil {
		return err
	}
	current, target, err := provider.GetVersions(ctx)
	if fatalErr := logger.Err(); fatalErr != nil {
		return fmt.Errorf("read migration versions: %w", fatalErr)
	}
	if err != nil {
		return fmt.Errorf("read migration versions: %w", err)
	}
	if current > target {
		return &UnsupportedSchemaError{Current: current, Target: target}
	}
	statuses, err := provider.Status(ctx)
	if err != nil {
		return fmt.Errorf("read migration status: %w", err)
	}
	for _, status := range statuses {
		if status.State != goose.StatePending || status.Source == nil {
			continue
		}
		progress := MigrationProgress{
			Phase: "started", Version: status.Source.Version,
			Name: status.Source.Path, Current: current, Target: target,
		}
		if observer != nil {
			observer(progress)
		}
		if _, err := provider.ApplyVersion(ctx, status.Source.Version, true); err != nil {
			return fmt.Errorf("apply migration %d: %w", status.Source.Version, err)
		}
		current = status.Source.Version
		if observer != nil {
			progress.Phase = "completed"
			progress.Current = current
			observer(progress)
		}
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

type sqliteBackuper interface {
	NewBackup(string) (*sqlite.Backup, error)
}

type sqliteRestorer interface {
	NewRestore(string) (*sqlite.Backup, error)
}

// Backup creates and verifies an online SQLite backup, including committed WAL
// content visible to the source connection, before atomically publishing it.
func Backup(ctx context.Context, source, destination string, observer func(copied, total int)) (BackupResult, error) {
	sourcePath, _, inspectable, err := schemaInspectionTarget(source)
	if err != nil {
		return BackupResult{}, err
	}
	if !inspectable {
		return BackupResult{}, fmt.Errorf("backup requires a persistent source database")
	}
	if _, err := os.Stat(sourcePath); err != nil {
		return BackupResult{}, fmt.Errorf("inspect backup source: %w", err)
	}
	destination, err = persistentFilesystemPath(destination, "backup destination")
	if err != nil {
		return BackupResult{}, err
	}
	lease, err := acquireCorpusLease(source, false, "back up corpus")
	if err != nil {
		return BackupResult{}, err
	}
	defer lease.release()
	return backupWithLease(ctx, source, destination, observer)
}

func backupWithLease(ctx context.Context, source, destination string, observer func(copied, total int)) (BackupResult, error) {
	sourcePath, dsn, inspectable, err := schemaInspectionTarget(source)
	if err != nil {
		return BackupResult{}, err
	}
	if !inspectable {
		return BackupResult{}, fmt.Errorf("backup requires a persistent source database")
	}
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return BackupResult{}, fmt.Errorf("inspect backup source: %w", err)
	}
	if strings.TrimSpace(destination) == "" {
		return BackupResult{}, fmt.Errorf("backup destination is required")
	}
	destination, err = persistentFilesystemPath(destination, "backup destination")
	if err != nil {
		return BackupResult{}, err
	}
	if _, err := os.Stat(destination); err == nil {
		return BackupResult{}, fmt.Errorf("backup destination already exists: %s", destination)
	} else if !os.IsNotExist(err) {
		return BackupResult{}, fmt.Errorf("inspect backup destination: %w", err)
	}
	manifestPath := backupManifestPath(destination)
	if _, err := os.Stat(manifestPath); err == nil {
		return BackupResult{}, fmt.Errorf("backup manifest already exists: %s", manifestPath)
	} else if !os.IsNotExist(err) {
		return BackupResult{}, fmt.Errorf("inspect backup manifest destination: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return BackupResult{}, fmt.Errorf("create backup directory: %w", err)
	}
	requiredBytes := sourceInfo.Size()
	if walInfo, statErr := os.Stat(sourcePath + "-wal"); statErr == nil {
		if walInfo.Size() > math.MaxInt64-requiredBytes {
			return BackupResult{}, fmt.Errorf("preflight backup disk space: source size overflows int64")
		}
		requiredBytes += walInfo.Size()
	} else if !os.IsNotExist(statErr) {
		return BackupResult{}, fmt.Errorf("inspect backup WAL: %w", statErr)
	}
	if err := requireDiskSpace(filepath.Dir(destination), requiredBytes); err != nil {
		return BackupResult{}, fmt.Errorf("preflight backup disk space: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".gitcontribute-backup-*.db")
	if err != nil {
		return BackupResult{}, fmt.Errorf("create backup staging file: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return BackupResult{}, fmt.Errorf("close backup staging file: %w", err)
	}
	if err := os.Remove(tmpPath); err != nil {
		return BackupResult{}, fmt.Errorf("prepare backup staging path: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return BackupResult{}, fmt.Errorf("open backup source: %w", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	sourceSchema, expectedSchema, err := (&Corpus{db: db}).inspectSchemaVersions(ctx)
	if err != nil {
		return BackupResult{}, fmt.Errorf("inspect backup schema: %w", err)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return BackupResult{}, fmt.Errorf("acquire backup source connection: %w", err)
	}
	defer conn.Close()
	err = conn.Raw(func(driverConn any) error {
		starter, ok := driverConn.(sqliteBackuper)
		if !ok {
			return fmt.Errorf("sqlite driver does not support online backup")
		}
		backup, err := starter.NewBackup(tmpPath)
		if err != nil {
			return err
		}
		finished := false
		defer func() {
			if !finished {
				_ = backup.Finish()
			}
		}()
		for more := true; more; {
			if err := ctx.Err(); err != nil {
				return err
			}
			more, err = backup.Step(256)
			if err != nil {
				return err
			}
			if observer != nil {
				observer(backup.PageCount()-backup.Remaining(), backup.PageCount())
			}
		}
		finished = true
		return backup.Finish()
	})
	if err != nil {
		return BackupResult{}, fmt.Errorf("copy SQLite backup: %w", err)
	}
	check, err := sql.Open("sqlite", "file:"+filepath.ToSlash(tmpPath)+"?mode=ro&_pragma=query_only(1)")
	if err != nil {
		return BackupResult{}, fmt.Errorf("open staged backup: %w", err)
	}
	var integrity string
	checkErr := check.QueryRowContext(ctx, `PRAGMA quick_check(1)`).Scan(&integrity)
	closeErr := check.Close()
	if checkErr != nil {
		return BackupResult{}, fmt.Errorf("verify staged backup: %w", checkErr)
	}
	if closeErr != nil {
		return BackupResult{}, fmt.Errorf("close staged backup: %w", closeErr)
	}
	if integrity != "ok" {
		return BackupResult{}, fmt.Errorf("verify staged backup: %s", integrity)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return BackupResult{}, fmt.Errorf("protect staged backup: %w", err)
	}
	info, err := os.Stat(tmpPath)
	if err != nil {
		return BackupResult{}, fmt.Errorf("inspect staged backup: %w", err)
	}
	file, err := os.Open(tmpPath)
	if err != nil {
		return BackupResult{}, fmt.Errorf("hash staged backup: %w", err)
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, file)
	closeErr = file.Close()
	if copyErr != nil || closeErr != nil {
		return BackupResult{}, fmt.Errorf("hash staged backup: %w", errors.Join(copyErr, closeErr))
	}
	result := BackupResult{
		Path: destination, ManifestPath: manifestPath, SizeBytes: info.Size(),
		SHA256: fmt.Sprintf("%x", hash.Sum(nil)), CreatedAt: time.Now().UTC(),
		SourceSchema: sourceSchema, ExpectedSchema: expectedSchema,
		Compatibility: classifySchemaCompatibility(sourceSchema, expectedSchema),
	}
	manifest := BackupManifest{
		FormatVersion: backupManifestVersion, CreatedAt: result.CreatedAt,
		SizeBytes: result.SizeBytes, SHA256: result.SHA256,
		SourceSchema: result.SourceSchema, ExpectedSchema: result.ExpectedSchema,
		Compatibility: result.Compatibility,
	}
	manifestTmp, err := writeBackupManifestStaging(manifestPath, manifest)
	if err != nil {
		return BackupResult{}, err
	}
	manifestCleanup := true
	defer func() {
		if manifestCleanup {
			_ = os.Remove(manifestTmp)
		}
	}()
	if err := os.Rename(tmpPath, destination); err != nil {
		return BackupResult{}, fmt.Errorf("publish backup: %w", err)
	}
	cleanup = false
	if err := os.Rename(manifestTmp, manifestPath); err != nil {
		_ = os.Remove(destination)
		return BackupResult{}, fmt.Errorf("publish backup manifest: %w", err)
	}
	manifestCleanup = false
	return result, nil
}

func classifySchemaCompatibility(current, target int64) SchemaState {
	switch {
	case current < target:
		return SchemaMigrationRequired
	case current > target:
		return SchemaNewer
	default:
		return SchemaCurrent
	}
}

func requireDiskSpace(directory string, required int64) error {
	if required <= 0 {
		return nil
	}
	available, err := freeDiskBytes(directory)
	if err != nil {
		return err
	}
	if uint64(required) > available {
		return fmt.Errorf("need at least %d bytes, only %d bytes available", required, available)
	}
	return nil
}

func writeBackupManifestStaging(destination string, manifest BackupManifest) (string, error) {
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".gitcontribute-backup-manifest-*.json")
	if err != nil {
		return "", fmt.Errorf("create backup manifest staging file: %w", err)
	}
	path := tmp.Name()
	remove := true
	defer func() {
		if remove {
			_ = os.Remove(path)
		}
	}()
	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	writeErr := encoder.Encode(manifest)
	syncErr := tmp.Sync()
	closeErr := tmp.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return "", fmt.Errorf("write backup manifest: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return "", fmt.Errorf("protect backup manifest: %w", err)
	}
	remove = false
	return path, nil
}

// Restore replaces a persistent corpus from a verified SQLite backup while
// holding an exclusive process lease. Callers must create any safety backup
// and obtain destructive-operation consent before calling it.
func Restore(ctx context.Context, source, destination string, observer func(copied, total int)) (BackupResult, error) {
	var err error
	source, err = persistentFilesystemPath(source, "restore source")
	if err != nil {
		return BackupResult{}, err
	}
	destination, err = persistentFilesystemPath(destination, "restore destination")
	if err != nil {
		return BackupResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return BackupResult{}, fmt.Errorf("create restore directory: %w", err)
	}
	lease, err := acquireCorpusLease(destination, true, "restore corpus")
	if err != nil {
		return BackupResult{}, err
	}
	defer lease.release()
	return restoreWithLease(ctx, source, destination, observer)
}

// RestoreWithSafetyBackup holds one exclusive destination lease while taking
// the safety backup, replacing the corpus, and verifying the restored state.
// The returned safety backup remains available when restore fails.
func RestoreWithSafetyBackup(ctx context.Context, source, destination, safetyDestination string, observer func(copied, total int)) (*BackupResult, BackupResult, error) {
	var err error
	source, err = persistentFilesystemPath(source, "restore source")
	if err != nil {
		return nil, BackupResult{}, err
	}
	destination, err = persistentFilesystemPath(destination, "restore destination")
	if err != nil {
		return nil, BackupResult{}, err
	}
	if strings.TrimSpace(safetyDestination) != "" {
		safetyDestination, err = persistentFilesystemPath(safetyDestination, "safety backup destination")
		if err != nil {
			return nil, BackupResult{}, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return nil, BackupResult{}, fmt.Errorf("create restore directory: %w", err)
	}
	lease, err := acquireCorpusLease(destination, true, "back up and restore corpus")
	if err != nil {
		return nil, BackupResult{}, err
	}
	defer lease.release()
	var safety *BackupResult
	if strings.TrimSpace(safetyDestination) != "" {
		result, err := backupWithLease(ctx, destination, safetyDestination, nil)
		if err != nil {
			return nil, BackupResult{}, err
		}
		safety = &result
	}
	restored, err := restoreWithLease(ctx, source, destination, observer)
	return safety, restored, err
}

func persistentFilesystemPath(value, role string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s is required", role)
	}
	filePath, _, inspectable, err := schemaInspectionTarget(value)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", role, err)
	}
	if !inspectable || strings.TrimSpace(filePath) == "" {
		return "", fmt.Errorf("%s must be a persistent file path", role)
	}
	return filePath, nil
}

func restoreWithLease(ctx context.Context, source, destination string, observer func(copied, total int)) (BackupResult, error) {
	if strings.TrimSpace(source) == "" || strings.TrimSpace(destination) == "" {
		return BackupResult{}, fmt.Errorf("restore source and destination are required")
	}
	if filepath.Clean(source) == filepath.Clean(destination) {
		return BackupResult{}, fmt.Errorf("restore source and destination must differ")
	}
	manifest, err := readAndVerifyBackupManifest(source)
	if err != nil {
		return BackupResult{}, fmt.Errorf("verify restore manifest: %w", err)
	}
	if err := verifySQLiteFile(ctx, source); err != nil {
		return BackupResult{}, fmt.Errorf("verify restore source: %w", err)
	}
	inspection, err := InspectSchema(ctx, source)
	if err != nil {
		return BackupResult{}, fmt.Errorf("inspect restore source: %w", err)
	}
	if !inspection.Exists {
		return BackupResult{}, fmt.Errorf("restore source does not contain a corpus")
	}
	if inspection.Current != manifest.SourceSchema {
		return BackupResult{}, fmt.Errorf("restore source schema %d does not match manifest schema %d", inspection.Current, manifest.SourceSchema)
	}
	if inspection.State == SchemaNewer {
		return BackupResult{}, &UnsupportedSchemaError{Current: inspection.Current, Target: inspection.Target}
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return BackupResult{}, fmt.Errorf("create restore directory: %w", err)
	}
	if err := requireDiskSpace(filepath.Dir(destination), manifest.SizeBytes); err != nil {
		return BackupResult{}, fmt.Errorf("preflight restore disk space: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".gitcontribute-restore-*.db")
	if err != nil {
		return BackupResult{}, fmt.Errorf("create restore staging file: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return BackupResult{}, fmt.Errorf("close restore staging file: %w", err)
	}
	if err := os.Remove(tmpPath); err != nil {
		return BackupResult{}, fmt.Errorf("prepare restore staging path: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
			_ = os.Remove(tmpPath + "-wal")
			_ = os.Remove(tmpPath + "-shm")
		}
	}()
	db, err := sql.Open("sqlite", tmpPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(DELETE)&_pragma=busy_timeout(5000)")
	if err != nil {
		return BackupResult{}, fmt.Errorf("open restore staging database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	conn, err := db.Conn(ctx)
	if err != nil {
		_ = db.Close()
		return BackupResult{}, fmt.Errorf("acquire restore destination connection: %w", err)
	}
	err = conn.Raw(func(driverConn any) error {
		starter, ok := driverConn.(sqliteRestorer)
		if !ok {
			return fmt.Errorf("sqlite driver does not support online restore")
		}
		restore, err := starter.NewRestore(source)
		if err != nil {
			return err
		}
		finished := false
		defer func() {
			if !finished {
				_ = restore.Finish()
			}
		}()
		for more := true; more; {
			if err := ctx.Err(); err != nil {
				return err
			}
			more, err = restore.Step(256)
			if err != nil {
				return err
			}
			if observer != nil {
				observer(restore.PageCount()-restore.Remaining(), restore.PageCount())
			}
		}
		finished = true
		return restore.Finish()
	})
	closeConnErr := conn.Close()
	closeDBErr := db.Close()
	if err != nil {
		return BackupResult{}, fmt.Errorf("restore SQLite backup: %w", err)
	}
	if err := errors.Join(closeConnErr, closeDBErr); err != nil {
		return BackupResult{}, fmt.Errorf("close restored corpus: %w", err)
	}
	if err := verifySQLiteFile(ctx, tmpPath); err != nil {
		return BackupResult{}, fmt.Errorf("verify restored corpus: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return BackupResult{}, fmt.Errorf("protect restored corpus: %w", err)
	}
	if err := checkpointDestinationForReplacement(ctx, destination); err != nil {
		return BackupResult{}, err
	}
	if err := replaceDatabaseFile(tmpPath, destination); err != nil {
		return BackupResult{}, fmt.Errorf("publish restored corpus: %w", err)
	}
	cleanup = false
	return summarizeSQLiteFile(destination)
}

// checkpointDestinationForReplacement makes the current main database
// self-contained before its WAL and shared-memory sidecars are removed. Once
// the checkpoint succeeds, a crash before publication leaves the old main file
// usable; a crash after the atomic replacement cannot replay frames from the
// old database into the restored one.
func checkpointDestinationForReplacement(ctx context.Context, destination string) error {
	if _, err := os.Stat(destination); os.IsNotExist(err) {
		for _, suffix := range []string{"-wal", "-shm"} {
			if _, sidecarErr := os.Stat(destination + suffix); sidecarErr == nil {
				return fmt.Errorf("destination %s is missing while its %s sidecar exists", destination, suffix)
			} else if !os.IsNotExist(sidecarErr) {
				return fmt.Errorf("inspect destination %s sidecar: %w", suffix, sidecarErr)
			}
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect restore destination: %w", err)
	}

	hasSidecar := false
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(destination + suffix); err == nil {
			hasSidecar = true
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect destination %s sidecar: %w", suffix, err)
		}
	}
	if !hasSidecar {
		return nil
	}

	db, err := sql.Open("sqlite", destination+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return fmt.Errorf("open destination for WAL checkpoint: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	var busy, logFrames, checkpointedFrames int
	checkpointErr := db.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logFrames, &checkpointedFrames)
	closeErr := db.Close()
	if checkpointErr != nil {
		return fmt.Errorf("checkpoint destination WAL before restore: %w", checkpointErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close destination after WAL checkpoint: %w", closeErr)
	}
	if busy != 0 {
		return fmt.Errorf("checkpoint destination WAL before restore: %d busy connection(s)", busy)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(destination + suffix); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove checkpointed destination %s sidecar: %w", suffix, err)
		}
	}
	return nil
}

func readAndVerifyBackupManifest(source string) (BackupManifest, error) {
	path := backupManifestPath(source)
	file, err := os.Open(path)
	if err != nil {
		return BackupManifest{}, err
	}
	decoder := json.NewDecoder(file)
	var manifest BackupManifest
	decodeErr := decoder.Decode(&manifest)
	closeErr := file.Close()
	if err := errors.Join(decodeErr, closeErr); err != nil {
		return BackupManifest{}, fmt.Errorf("read %s: %w", path, err)
	}
	if manifest.FormatVersion != backupManifestVersion {
		return BackupManifest{}, fmt.Errorf("unsupported backup manifest version %d", manifest.FormatVersion)
	}
	if manifest.CreatedAt.IsZero() || manifest.SizeBytes <= 0 || strings.TrimSpace(manifest.SHA256) == "" || manifest.SourceSchema < 0 || manifest.ExpectedSchema <= 0 {
		return BackupManifest{}, fmt.Errorf("backup manifest is incomplete")
	}
	if want := classifySchemaCompatibility(manifest.SourceSchema, manifest.ExpectedSchema); manifest.Compatibility != want {
		return BackupManifest{}, fmt.Errorf("backup manifest compatibility %q does not match schema relationship %q", manifest.Compatibility, want)
	}
	actual, err := summarizeSQLiteFile(source)
	if err != nil {
		return BackupManifest{}, err
	}
	if actual.SizeBytes != manifest.SizeBytes {
		return BackupManifest{}, fmt.Errorf("backup size %d does not match manifest size %d", actual.SizeBytes, manifest.SizeBytes)
	}
	if !strings.EqualFold(actual.SHA256, manifest.SHA256) {
		return BackupManifest{}, fmt.Errorf("backup checksum %s does not match manifest checksum %s", actual.SHA256, manifest.SHA256)
	}
	return manifest, nil
}

func verifySQLiteFile(ctx context.Context, path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	_, dsn, inspectable, err := schemaInspectionTarget(path)
	if err != nil {
		return err
	}
	if !inspectable {
		return fmt.Errorf("persistent SQLite path required")
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	var result string
	checkErr := db.QueryRowContext(ctx, `PRAGMA quick_check(1)`).Scan(&result)
	closeErr := db.Close()
	if checkErr != nil {
		return checkErr
	}
	if closeErr != nil {
		return closeErr
	}
	if result != "ok" {
		return fmt.Errorf("database quick check: %s", result)
	}
	return nil
}

func summarizeSQLiteFile(path string) (BackupResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		return BackupResult{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return BackupResult{}, err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return BackupResult{}, err
	}
	return BackupResult{Path: path, SizeBytes: info.Size(), SHA256: fmt.Sprintf("%x", hash.Sum(nil))}, nil
}
