package corpus

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestMigrationFailsFastWhileCorpusIsOpen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "corpus.db")
	first, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()

	second, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatalf("second read lease: %v", err)
	}
	defer second.Close()

	err = Migrate(ctx, path, nil)
	var busy *BusyError
	if !errors.As(err, &busy) {
		t.Fatalf("Migrate error = %v, want BusyError", err)
	}
}

func TestInspectMissingCorpusHasNoFilesystemSideEffects(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "missing.db")
	inspection, err := InspectSchema(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Exists || inspection.State != SchemaMissing {
		t.Fatalf("inspection = %+v", inspection)
	}
	for _, candidate := range []string{path, path + ".lock"} {
		if _, err := os.Stat(candidate); !os.IsNotExist(err) {
			t.Fatalf("inspection created %s: %v", candidate, err)
		}
	}
}

func TestInspectSchemaClassifiesNonDatabaseWithoutMutation(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "damaged.db")
	want := []byte("this is not a sqlite database")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}

	inspection, err := InspectSchema(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.State != SchemaDamaged || inspection.Problem == "" {
		t.Fatalf("inspection = %+v", inspection)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("inspection changed damaged corpus: got %q", got)
	}
	if _, err := os.Stat(path + ".lock"); !os.IsNotExist(err) {
		t.Fatalf("inspection created process lease: %v", err)
	}
}

func TestBackupIncludesCommittedWALData(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	source := filepath.Join(dir, "source.db")
	destination := filepath.Join(dir, "backups", "source.db")
	c, err := Open(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	want := time.Unix(1700000000, 0).UTC()
	if _, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "external", want, `{}`); err != nil {
		t.Fatal(err)
	}

	var progressCalls int
	result, err := Backup(ctx, source, destination, func(copied, total int) {
		progressCalls++
		if copied < 0 || total < copied {
			t.Fatalf("invalid backup progress %d/%d", copied, total)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != destination || result.SizeBytes == 0 || result.SHA256 == "" || progressCalls == 0 {
		t.Fatalf("backup result = %+v progress=%d", result, progressCalls)
	}
	if result.ManifestPath != destination+".manifest.json" || result.CreatedAt.IsZero() || result.SourceSchema == 0 || result.ExpectedSchema == 0 || result.Compatibility != SchemaCurrent {
		t.Fatalf("backup metadata = %+v", result)
	}
	manifestBytes, err := os.ReadFile(result.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest BackupManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SHA256 != result.SHA256 || manifest.SizeBytes != result.SizeBytes || manifest.SourceSchema != result.SourceSchema {
		t.Fatalf("manifest = %+v, result = %+v", manifest, result)
	}
	backup, err := OpenReadOnly(ctx, destination)
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	repo, err := backup.GetRepository(ctx, "owner", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if repo == nil || repo.ExternalID != "external" {
		t.Fatalf("backed-up repository = %+v", repo)
	}
	if _, err := Backup(ctx, source, destination, nil); err == nil {
		t.Fatal("backup overwrote an existing destination")
	}
}

func TestBackupDoesNotOverwriteTargetsCreatedDuringCopy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	source := filepath.Join(dir, "source.db")
	c, err := Open(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "external", time.Unix(1, 0), `{}`); err != nil {
		t.Fatal(err)
	}

	for _, target := range []string{"backup", "manifest"} {
		t.Run(target, func(t *testing.T) {
			destination := filepath.Join(dir, target+".db")
			racePath := destination
			if target == "manifest" {
				racePath = backupManifestPath(destination)
			}
			sentinel := []byte("created by another process")
			created := false
			_, err := Backup(ctx, source, destination, func(_, _ int) {
				if created {
					return
				}
				created = true
				if writeErr := os.WriteFile(racePath, sentinel, 0o600); writeErr != nil {
					t.Errorf("create racing %s: %v", target, writeErr)
				}
			})
			if err == nil {
				t.Fatalf("Backup succeeded after %s appeared", target)
			}
			if !created {
				t.Fatalf("backup did not reach progress callback for %s race", target)
			}
			got, readErr := os.ReadFile(racePath)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if !bytes.Equal(got, sentinel) {
				t.Fatalf("racing %s was overwritten: %q", target, got)
			}
			if target == "manifest" {
				if _, statErr := os.Stat(destination); !os.IsNotExist(statErr) {
					t.Fatalf("failed manifest publication left backup destination: %v", statErr)
				}
			}
		})
	}
}

func TestCanceledBackupRemovesStagingFiles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	source := filepath.Join(dir, "source.db")
	destination := filepath.Join(dir, "backup.db")
	c, err := Open(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := Backup(canceled, source, destination, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Backup error = %v, want context.Canceled", err)
	}
	for _, pattern := range []string{".gitcontribute-backup-*.db", ".gitcontribute-backup-manifest-*.json"} {
		matches, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 0 {
			t.Fatalf("staging files after cancellation = %v", matches)
		}
	}
	for _, path := range []string{destination, backupManifestPath(destination)} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("canceled backup published %s: %v", path, err)
		}
	}
}

func TestRestoreRejectsCorruptedBackupWithoutChangingDestination(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	destination := filepath.Join(dir, "corpus.db")
	backupPath := filepath.Join(dir, "corpus.backup.db")
	c, err := Open(ctx, destination)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ApplyRepositoryObservation(ctx, "owner", "destination", "external", time.Unix(1, 0), `{}`); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Backup(ctx, destination, backupPath, nil); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(backupPath, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteAt([]byte("corrupt"), 100); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := Restore(ctx, backupPath, destination, nil); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("Restore error = %v, want checksum rejection", err)
	}
	after, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("failed restore changed destination")
	}
}

func TestCanceledRestorePreservesDestinationAndRemovesStaging(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	source := filepath.Join(dir, "source.db")
	destination := filepath.Join(dir, "destination.db")
	backupPath := filepath.Join(dir, "source.backup.db")

	sourceCorpus, err := Open(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sourceCorpus.db.ExecContext(ctx, `CREATE TABLE restore_payload (data BLOB); INSERT INTO restore_payload VALUES (zeroblob(4194304))`); err != nil {
		t.Fatal(err)
	}
	if err := sourceCorpus.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Backup(ctx, source, backupPath, nil); err != nil {
		t.Fatal(err)
	}

	destinationCorpus, err := Open(ctx, destination)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := destinationCorpus.ApplyRepositoryObservation(ctx, "owner", "preserved", "external", time.Unix(1, 0), `{}`); err != nil {
		t.Fatal(err)
	}
	if err := destinationCorpus.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}

	restoreCtx, cancel := context.WithCancel(ctx)
	progress := 0
	_, err = Restore(restoreCtx, backupPath, destination, func(_, _ int) {
		progress++
		cancel()
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Restore error = %v, want context.Canceled", err)
	}
	if progress == 0 {
		t.Fatal("restore did not report progress")
	}
	after, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("canceled restore changed destination")
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".gitcontribute-restore-*.db*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("restore staging files after cancellation = %v", matches)
	}
}

func TestRestoreReplacesCorpusFromVerifiedBackup(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	destination := filepath.Join(dir, "corpus.db")
	backupPath := filepath.Join(dir, "corpus.backup.db")
	c, err := Open(ctx, destination)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ApplyRepositoryObservation(ctx, "owner", "kept", "external", time.Unix(1, 0), `{}`); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Backup(ctx, destination, backupPath, nil); err != nil {
		t.Fatal(err)
	}
	c, err = Open(ctx, destination)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ApplyRepositoryObservation(ctx, "owner", "removed", "external", time.Unix(2, 0), `{}`); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := Restore(ctx, backupPath, destination, nil); err != nil {
		t.Fatal(err)
	}
	restored, err := OpenReadOnly(ctx, destination)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	repos, err := restored.ListRepositories(ctx, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].Name != "kept" {
		t.Fatalf("repositories after restore = %+v", repos)
	}
}

func TestRestoreUsesVerifiedSnapshotWhenSourceChangesDuringCopy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	source := filepath.Join(dir, "source.db")
	backupPath := filepath.Join(dir, "source.backup.db")
	destination := filepath.Join(dir, "destination.db")
	c, err := Open(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ApplyRepositoryObservation(ctx, "owner", "kept", "external", time.Unix(1, 0), `{}`); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Backup(ctx, source, backupPath, nil); err != nil {
		t.Fatal(err)
	}

	mutated := false
	var mutationErr error
	_, err = Restore(ctx, backupPath, destination, func(_, _ int) {
		if mutated {
			return
		}
		mutated = true
		db, openErr := sql.Open("sqlite", backupPath)
		if openErr != nil {
			mutationErr = openErr
			return
		}
		_, execErr := db.ExecContext(ctx, `UPDATE repositories SET name = 'tampered' WHERE name = 'kept'`)
		mutationErr = errors.Join(execErr, db.Close())
	})
	if err != nil {
		t.Fatal(err)
	}
	if mutationErr != nil {
		t.Fatalf("mutate original backup during restore: %v", mutationErr)
	}
	if !mutated {
		t.Fatal("restore did not report progress")
	}
	restored, err := OpenReadOnly(ctx, destination)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	kept, err := restored.GetRepository(ctx, "owner", "kept")
	if err != nil {
		t.Fatal(err)
	}
	if kept == nil {
		t.Fatal("restore copied post-manifest source content instead of verified snapshot")
	}
}

func TestRestoreReportsCommittedResultWhenSnapshotCleanupFails(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	source := filepath.Join(dir, "source.db")
	backupPath := filepath.Join(dir, "source.backup.db")
	destination := filepath.Join(dir, "destination.db")
	c, err := Open(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ApplyRepositoryObservation(ctx, "owner", "committed", "external", time.Unix(1, 0), `{}`); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Backup(ctx, source, backupPath, nil); err != nil {
		t.Fatal(err)
	}

	wantCleanupErr := errors.New("injected snapshot cleanup failure")
	originalRemove := removeRestoreSnapshot
	removeRestoreSnapshot = func(string) error { return wantCleanupErr }
	t.Cleanup(func() { removeRestoreSnapshot = originalRemove })
	result, err := Restore(ctx, backupPath, destination, nil)
	var committedErr *PostCommitCleanupError
	if !errors.As(err, &committedErr) || !errors.Is(err, wantCleanupErr) {
		t.Fatalf("Restore error = %v, want committed cleanup error", err)
	}
	if result.Path != destination || result.SizeBytes == 0 || result.SHA256 == "" {
		t.Fatalf("committed restore result = %+v", result)
	}
	restored, err := OpenReadOnly(ctx, destination)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	repo, err := restored.GetRepository(ctx, "owner", "committed")
	if err != nil {
		t.Fatal(err)
	}
	if repo == nil {
		t.Fatal("restore cleanup failure obscured an uncommitted destination")
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".gitcontribute-restore-source-*.db"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("deferred cleanup left source snapshots: %v", matches)
	}
}

func TestRestoreResolvesFileURIDestination(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	source := filepath.Join(dir, "source.db")
	destination := filepath.Join(dir, "destination.db")
	backupPath := filepath.Join(dir, "source.backup.db")

	sourceCorpus, err := Open(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sourceCorpus.ApplyRepositoryObservation(ctx, "owner", "restored", "external", time.Unix(1, 0), `{}`); err != nil {
		t.Fatal(err)
	}
	if err := sourceCorpus.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Backup(ctx, source, backupPath, nil); err != nil {
		t.Fatal(err)
	}

	destinationCorpus, err := Open(ctx, destination)
	if err != nil {
		t.Fatal(err)
	}
	if err := destinationCorpus.Close(); err != nil {
		t.Fatal(err)
	}
	uriPath := filepath.ToSlash(destination)
	if runtime.GOOS == "windows" && filepath.IsAbs(destination) && !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	u := &url.URL{Scheme: "file", Path: uriPath}
	query := u.Query()
	query.Set("cache", "shared")
	u.RawQuery = query.Encode()
	if _, err := Restore(ctx, backupPath, u.String(), nil); err != nil {
		t.Fatal(err)
	}

	restored, err := OpenReadOnly(ctx, destination)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	repo, err := restored.GetRepository(ctx, "owner", "restored")
	if err != nil {
		t.Fatal(err)
	}
	if repo == nil {
		t.Fatal("restore did not replace the file referenced by the destination URI")
	}
}

func TestRestoreRejectsCorruptedBackupAndPreservesWALSidecars(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	destination := filepath.Join(dir, "corpus.db")
	backupPath := filepath.Join(dir, "corpus.backup.db")

	destinationCorpus, err := Open(ctx, destination)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := destinationCorpus.ApplyRepositoryObservation(ctx, "owner", "preserved", "external", time.Unix(1, 0), `{}`); err != nil {
		t.Fatal(err)
	}
	if err := destinationCorpus.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := Backup(ctx, destination, backupPath, nil); err != nil {
		t.Fatal(err)
	}

	walContent := []byte("stale wal")
	shmContent := []byte("stale shm")
	if err := os.WriteFile(destination+"-wal", walContent, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination+"-shm", shmContent, 0o600); err != nil {
		t.Fatal(err)
	}

	beforeDB, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}

	file, err := os.OpenFile(backupPath, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteAt([]byte("corrupt"), 100); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := Restore(ctx, backupPath, destination, nil); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("Restore error = %v, want checksum rejection", err)
	}
	afterDB, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeDB, afterDB) {
		t.Fatal("failed restore changed destination database")
	}
	gotWAL, err := os.ReadFile(destination + "-wal")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotWAL, walContent) {
		t.Fatal("failed restore changed destination -wal sidecar")
	}
	gotSHM, err := os.ReadFile(destination + "-shm")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotSHM, shmContent) {
		t.Fatal("failed restore changed destination -shm sidecar")
	}
}

func TestRestoreRetiresRealUncheckpointedWAL(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	source := filepath.Join(dir, "source.db")
	destination := filepath.Join(dir, "destination.db")
	backupPath := filepath.Join(dir, "source.backup.db")

	sourceCorpus, err := Open(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sourceCorpus.ApplyRepositoryObservation(ctx, "owner", "from-backup", "external", time.Unix(1, 0), `{}`); err != nil {
		t.Fatal(err)
	}
	if err := sourceCorpus.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Backup(ctx, source, backupPath, nil); err != nil {
		t.Fatal(err)
	}

	destinationCorpus, err := Open(ctx, destination)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := destinationCorpus.ApplyRepositoryObservation(ctx, "owner", "stale-owner", "external", time.Unix(1, 0), `{}`); err != nil {
		t.Fatal(err)
	}
	if err := destinationCorpus.Close(); err != nil {
		t.Fatal(err)
	}

	raw, err := sql.Open("sqlite", destination+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	raw.SetMaxOpenConns(1)
	if _, err := raw.ExecContext(ctx, "INSERT INTO repositories (owner, name, created_at, updated_at) VALUES ('wal-owner', 'wal-repo', 1, 1)"); err != nil {
		t.Fatalf("insert into destination: %v", err)
	}
	wal, err := os.ReadFile(destination + "-wal")
	if err != nil {
		t.Fatal(err)
	}
	shm, err := os.ReadFile(destination + "-shm")
	if err != nil {
		t.Fatal(err)
	}
	if len(wal) == 0 {
		t.Fatal("destination had no WAL before close")
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	// Rewrite the real, uncheckpointed WAL/SHM files so they are present when restore begins.
	if err := os.WriteFile(destination+"-wal", wal, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination+"-shm", shm, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Restore(ctx, backupPath, destination, nil); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if _, err := os.Stat(destination + "-wal"); !os.IsNotExist(err) {
		t.Fatalf("restore left destination -wal sidecar: %v", err)
	}
	if _, err := os.Stat(destination + "-shm"); !os.IsNotExist(err) {
		t.Fatalf("restore left destination -shm sidecar: %v", err)
	}

	restored, err := OpenReadOnly(ctx, destination)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	repos, err := restored.ListRepositories(ctx, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].Name != "from-backup" {
		t.Fatalf("repositories after restore = %+v", repos)
	}
}

func TestCheckpointDestinationMakesWALDataSelfContained(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "destination.db")
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=wal_autocheckpoint(0)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "CREATE TABLE values_for_restore (value TEXT NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatal(err)
	}
	mainBeforeWAL, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO values_for_restore (value) VALUES ('committed-in-wal')"); err != nil {
		t.Fatal(err)
	}
	wal, err := os.ReadFile(path + "-wal")
	if err != nil {
		t.Fatal(err)
	}
	shm, err := os.ReadFile(path + "-shm")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, mainBeforeWAL, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+"-wal", wal, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+"-shm", shm, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := checkpointDestinationForReplacement(ctx, path); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(path + suffix); !os.IsNotExist(err) {
			t.Fatalf("checkpoint left %s sidecar: %v", suffix, err)
		}
	}
	verified, err := sql.Open("sqlite", path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer verified.Close()
	var value string
	if err := verified.QueryRowContext(ctx, "SELECT value FROM values_for_restore").Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "committed-in-wal" {
		t.Fatalf("checkpointed value = %q", value)
	}
}
