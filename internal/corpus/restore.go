package corpus

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
)

var removeRestoreSnapshot = removeFile

// PostCommitCleanupError indicates that restore committed successfully but a
// private staging artifact could not be cleaned up.
type PostCommitCleanupError struct {
	Err error
}

func (e *PostCommitCleanupError) Error() string {
	return fmt.Sprintf("restore committed but cleanup failed: %v", e.Err)
}

// Unwrap returns the cleanup failure.
func (e *PostCommitCleanupError) Unwrap() error { return e.Err }

// Restore atomically replaces a corpus from a verified backup.
func Restore(ctx context.Context, source, destination string, observer func(copied, total int)) (_ BackupResult, returnErr error) {
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
	defer releaseLeaseOnReturn(lease, &returnErr)
	return restoreWithLease(ctx, source, destination, observer)
}

// RestoreWithSafetyBackup holds one exclusive destination lease while taking
// the safety backup, replacing the corpus, and verifying the restored state.
// The returned safety backup remains available when restore fails.
func RestoreWithSafetyBackup(ctx context.Context, source, destination, safetyDestination string, observer func(copied, total int)) (_ *BackupResult, _ BackupResult, returnErr error) {
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
	defer releaseLeaseOnReturn(lease, &returnErr)
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

func restoreWithLease(ctx context.Context, source, destination string, observer func(copied, total int)) (_ BackupResult, returnErr error) {
	if err := validateRestorePaths(source, destination); err != nil {
		return BackupResult{}, err
	}
	manifest, err := readBackupManifest(source)
	if err != nil {
		return BackupResult{}, fmt.Errorf("verify restore manifest: %w", err)
	}
	requiredBytes, err := restoreDiskRequirement(manifest.SizeBytes)
	if err != nil {
		return BackupResult{}, err
	}
	if err := prepareRestoreDestination(destination, requiredBytes); err != nil {
		return BackupResult{}, err
	}
	snapshotPath, err := snapshotRestoreSource(ctx, source, destination)
	if err != nil {
		return BackupResult{}, err
	}
	snapshotCleanup := true
	defer removeFileOnReturn(snapshotPath, &snapshotCleanup, &returnErr)
	if err := validateRestoreArtifact(ctx, snapshotPath, manifest); err != nil {
		return BackupResult{}, err
	}
	tmpPath, err := createStagedRestore(ctx, snapshotPath, destination, observer)
	if err != nil {
		return BackupResult{}, err
	}
	cleanup := true
	defer removeRestoreFilesOnReturn(tmpPath, &cleanup, &returnErr)
	if err := checkpointDestinationForReplacement(ctx, destination); err != nil {
		return BackupResult{}, err
	}
	if err := replaceDatabaseFile(tmpPath, destination); err != nil {
		return BackupResult{}, fmt.Errorf("publish restored corpus: %w", err)
	}
	cleanup = false
	result, err := summarizeSQLiteFile(destination)
	if err != nil {
		return BackupResult{Path: destination}, &PostCommitCleanupError{Err: fmt.Errorf("summarize restored corpus: %w", err)}
	}
	if err := removeRestoreSnapshot(snapshotPath); err != nil {
		return result, &PostCommitCleanupError{Err: fmt.Errorf("remove restore source snapshot: %w", err)}
	}
	snapshotCleanup = false
	return result, nil
}

func validateRestorePaths(source, destination string) error {
	if strings.TrimSpace(source) == "" || strings.TrimSpace(destination) == "" {
		return errors.New("restore source and destination are required")
	}
	if filepath.Clean(source) == filepath.Clean(destination) {
		return errors.New("restore source and destination must differ")
	}
	return nil
}

func restoreDiskRequirement(size int64) (int64, error) {
	if size > math.MaxInt64/2 {
		return 0, errors.New("preflight restore disk space: backup size overflows int64")
	}
	return size * 2, nil
}

func validateRestoreArtifact(ctx context.Context, source string, manifest BackupManifest) error {
	actual, err := summarizeSQLiteFile(source)
	if err != nil {
		return err
	}
	if actual.SizeBytes != manifest.SizeBytes {
		return fmt.Errorf("backup size %d does not match manifest size %d", actual.SizeBytes, manifest.SizeBytes)
	}
	if !strings.EqualFold(actual.SHA256, manifest.SHA256) {
		return fmt.Errorf("backup checksum %s does not match manifest checksum %s", actual.SHA256, manifest.SHA256)
	}
	if err := verifySQLiteFile(ctx, source); err != nil {
		return fmt.Errorf("verify restore source: %w", err)
	}
	inspection, err := InspectSchema(ctx, source)
	if err != nil {
		return fmt.Errorf("inspect restore source: %w", err)
	}
	if !inspection.Exists {
		return errors.New("restore source does not contain a corpus")
	}
	if inspection.Current != manifest.SourceSchema {
		return fmt.Errorf("restore source schema %d does not match manifest schema %d", inspection.Current, manifest.SourceSchema)
	}
	if inspection.State == SchemaNewer {
		return &UnsupportedSchemaError{Current: inspection.Current, Target: inspection.Target}
	}
	return nil
}

func prepareRestoreDestination(destination string, requiredBytes int64) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return fmt.Errorf("create restore directory: %w", err)
	}
	if err := requireDiskSpace(filepath.Dir(destination), requiredBytes); err != nil {
		return fmt.Errorf("preflight restore disk space: %w", err)
	}
	return nil
}

func snapshotRestoreSource(ctx context.Context, source, destination string) (_ string, returnErr error) {
	sourceFile, err := openFileWithinParent(source)
	if err != nil {
		return "", fmt.Errorf("open restore source for snapshot: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".gitcontribute-restore-source-*.db")
	if err != nil {
		return "", errors.Join(fmt.Errorf("create restore source snapshot: %w", err), sourceFile.Close())
	}
	path := tmp.Name()
	cleanup := true
	defer removeFileOnReturn(path, &cleanup, &returnErr)
	copyErr := copyWithContext(ctx, tmp, sourceFile)
	syncErr := tmp.Sync()
	chmodErr := tmp.Chmod(0o600)
	closeErr := errors.Join(sourceFile.Close(), tmp.Close())
	if err := errors.Join(copyErr, syncErr, chmodErr, closeErr); err != nil {
		return "", fmt.Errorf("snapshot restore source: %w", err)
	}
	cleanup = false
	return path, nil
}

func copyWithContext(ctx context.Context, destination io.Writer, source io.Reader) error {
	buffer := make([]byte, 1024*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		count, readErr := source.Read(buffer)
		if count > 0 {
			written, err := destination.Write(buffer[:count])
			if err != nil {
				return err
			}
			if written != count {
				return io.ErrShortWrite
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func createStagedRestore(ctx context.Context, source, destination string, observer func(copied, total int)) (_ string, returnErr error) {
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".gitcontribute-restore-*.db")
	if err != nil {
		return "", fmt.Errorf("create restore staging file: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		return "", errors.Join(fmt.Errorf("close restore staging file: %w", err), removeFile(tmpPath))
	}
	if err := os.Remove(tmpPath); err != nil {
		return "", fmt.Errorf("prepare restore staging path: %w", err)
	}
	cleanup := true
	defer removeRestoreFilesOnReturn(tmpPath, &cleanup, &returnErr)
	db, err := sql.Open("sqlite", tmpPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(DELETE)&_pragma=busy_timeout(5000)")
	if err != nil {
		return "", fmt.Errorf("open restore staging database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	conn, err := db.Conn(ctx)
	if err != nil {
		return "", errors.Join(fmt.Errorf("acquire restore destination connection: %w", err), db.Close())
	}
	err = copySQLiteRestore(ctx, conn, source, observer)
	closeConnErr := conn.Close()
	closeDBErr := db.Close()
	if err != nil {
		return "", fmt.Errorf("restore SQLite backup: %w", err)
	}
	if err := errors.Join(closeConnErr, closeDBErr); err != nil {
		return "", fmt.Errorf("close restored corpus: %w", err)
	}
	if err := verifySQLiteFile(ctx, tmpPath); err != nil {
		return "", fmt.Errorf("verify restored corpus: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return "", fmt.Errorf("protect restored corpus: %w", err)
	}
	cleanup = false
	return tmpPath, nil
}

func copySQLiteRestore(ctx context.Context, conn *sql.Conn, source string, observer func(copied, total int)) error {
	return conn.Raw(func(driverConn any) error {
		starter, ok := driverConn.(sqliteRestorer)
		if !ok {
			return errors.New("sqlite driver does not support online restore")
		}
		restore, err := starter.NewRestore(source)
		if err != nil {
			return err
		}
		return runSQLitePageCopy(ctx, restore, observer)
	})
}

func removeRestoreFilesOnReturn(path string, enabled *bool, returnErr *error) {
	if !*enabled {
		return
	}
	*returnErr = errors.Join(*returnErr, removeFile(path), removeFile(path+"-wal"), removeFile(path+"-shm"))
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

func readBackupManifest(source string) (BackupManifest, error) {
	path := backupManifestPath(source)
	file, err := openFileWithinParent(path)
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
		return BackupManifest{}, errors.New("backup manifest is incomplete")
	}
	if want := classifySchemaCompatibility(manifest.SourceSchema, manifest.ExpectedSchema); manifest.Compatibility != want {
		return BackupManifest{}, fmt.Errorf("backup manifest compatibility %q does not match schema relationship %q", manifest.Compatibility, want)
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
		return errors.New("persistent SQLite path required")
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
	file, err := openFileWithinParent(path)
	if err != nil {
		return BackupResult{}, err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return BackupResult{}, err
	}
	return BackupResult{Path: path, SizeBytes: info.Size(), SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}
