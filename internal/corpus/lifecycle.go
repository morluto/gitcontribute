package corpus

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
	// SchemaMissing indicates that no persistent corpus exists.
	SchemaMissing SchemaState = "missing"
	// SchemaCurrent indicates that the corpus matches the supported schema.
	SchemaCurrent SchemaState = "current"
	// SchemaMigrationRequired indicates that the corpus must be migrated.
	SchemaMigrationRequired SchemaState = "migration_required"
	// SchemaNewer indicates that the corpus is newer than this runtime supports.
	SchemaNewer SchemaState = "newer"
	// SchemaDamaged indicates that SQLite could not read the corpus safely.
	SchemaDamaged SchemaState = "damaged"
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

// MigrationObserver receives explicit migration progress updates.
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
func Migrate(ctx context.Context, path string, observer MigrationObserver) (returnErr error) {
	lease, err := acquireCorpusLease(path, true, "migrate corpus")
	if err != nil {
		return err
	}
	defer releaseLeaseOnReturn(lease, &returnErr)
	return migrateWithLease(ctx, path, observer)
}

// MigrateWithBackup holds one exclusive corpus lease from the start of the
// safety backup through migration verification. An empty backup destination
// explicitly opts out of backup creation.
func MigrateWithBackup(ctx context.Context, path, backupDestination string, observer MigrationObserver) (_ *BackupResult, returnErr error) {
	lease, err := acquireCorpusLease(path, true, "back up and migrate corpus")
	if err != nil {
		return nil, err
	}
	defer releaseLeaseOnReturn(lease, &returnErr)
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

func migrateWithLease(ctx context.Context, path string, observer MigrationObserver) (returnErr error) {
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
	defer closeSQLOnReturn(db, &returnErr)
	c := &Corpus{db: db}
	if err := c.ApplyMigrations(ctx, observer); err != nil {
		return err
	}
	if err := c.verifyPragmas(ctx); err != nil {
		return err
	}
	return nil
}

// InspectSchema reports corpus identity, compatibility, and bounded inventory
// without creating or mutating the database.
func InspectSchema(ctx context.Context, path string) (result SchemaInspection, returnErr error) {
	result = SchemaInspection{Path: path, State: SchemaMissing}
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
	defer releaseLeaseOnReturn(lease, &returnErr)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return result, fmt.Errorf("open corpus inspection: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	defer closeSQLOnReturn(db, &returnErr)
	return inspectSchemaContents(ctx, db, result)
}

func inspectSchemaContents(ctx context.Context, db *sql.DB, result SchemaInspection) (SchemaInspection, error) {
	c := &Corpus{db: db}
	current, target, err := c.inspectSchemaVersions(ctx)
	if err != nil {
		return schemaInspectionError(result, err)
	}
	result.Current, result.Target = current, target
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
		return schemaInspectionError(result, err)
	}
	result.Threads, err = countTableIfPresent(ctx, db, "threads")
	if err != nil {
		return schemaInspectionError(result, err)
	}
	if err := annotateMigrationSteps(ctx, db, result.Pending); err != nil {
		return schemaInspectionError(result, err)
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

func schemaInspectionError(result SchemaInspection, err error) (SchemaInspection, error) {
	if isSQLiteDamage(err) {
		result.State = SchemaDamaged
		result.Problem = err.Error()
		return result, nil
	}
	return result, err
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
			return 0, errors.New("no existing parent directory")
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
func Backup(ctx context.Context, source, destination string, observer func(copied, total int)) (_ BackupResult, returnErr error) {
	sourcePath, _, inspectable, err := schemaInspectionTarget(source)
	if err != nil {
		return BackupResult{}, err
	}
	if !inspectable {
		return BackupResult{}, errors.New("backup requires a persistent source database")
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
	defer releaseLeaseOnReturn(lease, &returnErr)
	return backupWithLease(ctx, source, destination, observer)
}

func backupWithLease(ctx context.Context, source, destination string, observer func(copied, total int)) (_ BackupResult, returnErr error) {
	plan, err := prepareBackup(source, destination)
	if err != nil {
		return BackupResult{}, err
	}
	staged, err := createStagedBackup(ctx, plan, observer)
	if err != nil {
		return BackupResult{}, err
	}
	cleanup := true
	defer removeFileOnReturn(staged.path, &cleanup, &returnErr)
	result, err := publishBackup(plan, staged)
	if err != nil {
		return BackupResult{}, err
	}
	cleanup = false
	return result, nil
}

type backupPlan struct {
	dsn          string
	destination  string
	manifestPath string
}

func prepareBackup(source, destination string) (backupPlan, error) {
	sourcePath, dsn, inspectable, err := schemaInspectionTarget(source)
	if err != nil {
		return backupPlan{}, err
	}
	if !inspectable {
		return backupPlan{}, errors.New("backup requires a persistent source database")
	}
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return backupPlan{}, fmt.Errorf("inspect backup source: %w", err)
	}
	if strings.TrimSpace(destination) == "" {
		return backupPlan{}, errors.New("backup destination is required")
	}
	destination, err = persistentFilesystemPath(destination, "backup destination")
	if err != nil {
		return backupPlan{}, err
	}
	if _, err := os.Stat(destination); err == nil {
		return backupPlan{}, fmt.Errorf("backup destination already exists: %s", destination)
	} else if !os.IsNotExist(err) {
		return backupPlan{}, fmt.Errorf("inspect backup destination: %w", err)
	}
	manifestPath := backupManifestPath(destination)
	if _, err := os.Stat(manifestPath); err == nil {
		return backupPlan{}, fmt.Errorf("backup manifest already exists: %s", manifestPath)
	} else if !os.IsNotExist(err) {
		return backupPlan{}, fmt.Errorf("inspect backup manifest destination: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return backupPlan{}, fmt.Errorf("create backup directory: %w", err)
	}
	requiredBytes := sourceInfo.Size()
	if walInfo, statErr := os.Stat(sourcePath + "-wal"); statErr == nil {
		if walInfo.Size() > math.MaxInt64-requiredBytes {
			return backupPlan{}, errors.New("preflight backup disk space: source size overflows int64")
		}
		requiredBytes += walInfo.Size()
	} else if !os.IsNotExist(statErr) {
		return backupPlan{}, fmt.Errorf("inspect backup WAL: %w", statErr)
	}
	if err := requireDiskSpace(filepath.Dir(destination), requiredBytes); err != nil {
		return backupPlan{}, fmt.Errorf("preflight backup disk space: %w", err)
	}
	return backupPlan{dsn: dsn, destination: destination, manifestPath: manifestPath}, nil
}

type stagedBackup struct {
	path           string
	summary        BackupResult
	sourceSchema   int64
	expectedSchema int64
}

func createStagedBackup(ctx context.Context, plan backupPlan, observer func(copied, total int)) (_ stagedBackup, returnErr error) {
	tmp, err := os.CreateTemp(filepath.Dir(plan.destination), ".gitcontribute-backup-*.db")
	if err != nil {
		return stagedBackup{}, fmt.Errorf("create backup staging file: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		return stagedBackup{}, errors.Join(fmt.Errorf("close backup staging file: %w", err), removeFile(tmpPath))
	}
	if err := os.Remove(tmpPath); err != nil {
		return stagedBackup{}, fmt.Errorf("prepare backup staging path: %w", err)
	}
	cleanup := true
	defer removeFileOnReturn(tmpPath, &cleanup, &returnErr)

	db, err := sql.Open("sqlite", plan.dsn)
	if err != nil {
		return stagedBackup{}, fmt.Errorf("open backup source: %w", err)
	}
	db.SetMaxOpenConns(1)
	defer closeSQLOnReturn(db, &returnErr)
	sourceSchema, expectedSchema, err := (&Corpus{db: db}).inspectSchemaVersions(ctx)
	if err != nil {
		return stagedBackup{}, fmt.Errorf("inspect backup schema: %w", err)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return stagedBackup{}, fmt.Errorf("acquire backup source connection: %w", err)
	}
	defer closeSQLOnReturn(conn, &returnErr)
	if err := copySQLiteBackup(ctx, conn, tmpPath, observer); err != nil {
		return stagedBackup{}, fmt.Errorf("copy SQLite backup: %w", err)
	}
	if err := verifySQLiteFile(ctx, tmpPath); err != nil {
		return stagedBackup{}, fmt.Errorf("verify staged backup: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return stagedBackup{}, fmt.Errorf("protect staged backup: %w", err)
	}
	summary, err := summarizeSQLiteFile(tmpPath)
	if err != nil {
		return stagedBackup{}, fmt.Errorf("summarize staged backup: %w", err)
	}
	cleanup = false
	return stagedBackup{path: tmpPath, summary: summary, sourceSchema: sourceSchema, expectedSchema: expectedSchema}, nil
}

func copySQLiteBackup(ctx context.Context, conn *sql.Conn, destination string, observer func(copied, total int)) error {
	return conn.Raw(func(driverConn any) error {
		starter, ok := driverConn.(sqliteBackuper)
		if !ok {
			return errors.New("sqlite driver does not support online backup")
		}
		backup, err := starter.NewBackup(destination)
		if err != nil {
			return err
		}
		return runSQLitePageCopy(ctx, backup, observer)
	})
}

func runSQLitePageCopy(ctx context.Context, operation *sqlite.Backup, observer func(copied, total int)) (returnErr error) {
	finished := false
	defer func() {
		if !finished {
			returnErr = errors.Join(returnErr, operation.Finish())
		}
	}()
	for more := true; more; {
		if err := ctx.Err(); err != nil {
			return err
		}
		var err error
		more, err = operation.Step(256)
		if err != nil {
			return err
		}
		if observer != nil {
			observer(operation.PageCount()-operation.Remaining(), operation.PageCount())
		}
	}
	finished = true
	return operation.Finish()
}

func publishBackup(plan backupPlan, staged stagedBackup) (_ BackupResult, returnErr error) {
	result := BackupResult{
		Path: plan.destination, ManifestPath: plan.manifestPath, SizeBytes: staged.summary.SizeBytes,
		SHA256: staged.summary.SHA256, CreatedAt: time.Now().UTC(),
		SourceSchema: staged.sourceSchema, ExpectedSchema: staged.expectedSchema,
		Compatibility: classifySchemaCompatibility(staged.sourceSchema, staged.expectedSchema),
	}
	manifest := BackupManifest{
		FormatVersion: backupManifestVersion, CreatedAt: result.CreatedAt,
		SizeBytes: result.SizeBytes, SHA256: result.SHA256,
		SourceSchema: result.SourceSchema, ExpectedSchema: result.ExpectedSchema,
		Compatibility: result.Compatibility,
	}
	manifestTmp, err := writeBackupManifestStaging(plan.manifestPath, manifest)
	if err != nil {
		return BackupResult{}, err
	}
	manifestCleanup := true
	defer removeFileOnReturn(manifestTmp, &manifestCleanup, &returnErr)
	if err := publishFileNoReplace(staged.path, plan.destination); err != nil {
		return BackupResult{}, fmt.Errorf("publish backup: %w", err)
	}
	if err := publishFileNoReplace(manifestTmp, plan.manifestPath); err != nil {
		return BackupResult{}, errors.Join(fmt.Errorf("publish backup manifest: %w", err), removeFile(plan.destination))
	}
	manifestCleanup = false
	return result, nil
}

func publishFileNoReplace(staged, destination string) error {
	if err := os.Link(staged, destination); err != nil {
		return err
	}
	if err := os.Remove(staged); err != nil {
		return errors.Join(err, removeFile(destination))
	}
	return nil
}

func removeFile(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func openFileWithinParent(path string) (*os.File, error) {
	directory, base := filepath.Split(path)
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	file, openErr := root.Open(base)
	closeErr := root.Close()
	if err := errors.Join(openErr, closeErr); err != nil {
		if file != nil {
			err = errors.Join(err, file.Close())
		}
		return nil, err
	}
	return file, nil
}

func removeFileOnReturn(path string, enabled *bool, returnErr *error) {
	if *enabled {
		*returnErr = errors.Join(*returnErr, removeFile(path))
	}
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

func writeBackupManifestStaging(destination string, manifest BackupManifest) (_ string, returnErr error) {
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".gitcontribute-backup-manifest-*.json")
	if err != nil {
		return "", fmt.Errorf("create backup manifest staging file: %w", err)
	}
	path := tmp.Name()
	remove := true
	defer func() {
		if remove {
			returnErr = errors.Join(returnErr, removeFile(path))
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
