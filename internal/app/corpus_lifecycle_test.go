package app

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/corpus"
	_ "modernc.org/sqlite"
)

func TestApplicationWriteOpenDoesNotMigrateExistingCorpus(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	home := t.TempDir()
	paths := config.NewPaths(&config.Env{Home: home})
	first, err := New(paths, "1.2.3", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Init(ctx); err != nil {
		t.Fatal(err)
	}
	dbPath := first.databasePath()
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM goose_db_version WHERE version_id > 0`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := New(paths, "1.2.3", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	_, err = second.openCorpus(ctx)
	var migrationRequired *corpus.MigrationRequiredError
	if !errors.As(err, &migrationRequired) {
		t.Fatalf("openCorpus error = %v, want MigrationRequiredError", err)
	}
	version, exists, err := corpus.InspectSchemaVersion(ctx, dbPath)
	if err != nil || !exists || version != 0 {
		t.Fatalf("schema after rejected write open = %d, exists=%v, err=%v", version, exists, err)
	}
	report, err := second.Setup(ctx, cli.SetupOptions{
		Mode: cli.SetupModeMCP, Clients: []string{"codex"}, TokenSource: "none",
		Executable: filepath.Join(home, "missing-runtime"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.HasFailures() || report.Corpus == nil || report.Corpus.State != "migration_required" {
		t.Fatalf("setup corpus preflight = %+v", report)
	}
	dryRunReport, err := second.Setup(ctx, cli.SetupOptions{
		Mode: cli.SetupModeMCP, Clients: []string{"codex"}, TokenSource: "none", DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !dryRunReport.HasFailures() || dryRunReport.Corpus == nil || dryRunReport.Corpus.State != "migration_required" {
		t.Fatalf("dry-run corpus preflight = %+v", dryRunReport)
	}
	if report.Steps[0].Message != dryRunReport.Steps[0].Message || !strings.Contains(report.Steps[0].Message, "gitcontribute corpus migrate --yes") {
		t.Fatalf("setup diagnostics differ: real=%q dry-run=%q", report.Steps[0].Message, dryRunReport.Steps[0].Message)
	}
	for _, step := range report.Steps {
		if step.Name == "mcp-runtime" {
			t.Fatalf("setup attempted runtime installation before corpus preflight: %+v", report)
		}
	}
}

func TestSetupFailsFastForNewerCorpusInDryRunAndRealModes(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dbPath := filepath.Join(home, "newer.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE goose_db_version (id INTEGER PRIMARY KEY, version_id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO goose_db_version (id, version_id) VALUES (1, 9999)"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	svc := testService(t, home, "1.2.3", dbPath)
	configPath, err := svc.paths.ConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Database = dbPath
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	for _, dryRun := range []bool{false, true} {
		report, err := svc.Setup(context.Background(), cli.SetupOptions{
			Mode: cli.SetupModeMCP, Clients: []string{"codex"}, TokenSource: "none", DryRun: dryRun,
		})
		if err == nil || !strings.Contains(err.Error(), "database schema version 9999 is newer than this binary supports") || !strings.Contains(err.Error(), "gitcontribute corpus inspect") {
			t.Fatalf("dry_run=%v report=%+v error = %v", dryRun, report, err)
		}
		if report == nil || report.Corpus == nil || report.Corpus.State != "newer" || len(report.Steps) != 0 {
			t.Fatalf("dry_run=%v report = %+v", dryRun, report)
		}
	}
}

func TestRestoreCorpusCreatesSafetyBackupAndReplacesState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	home := t.TempDir()
	svc, err := New(config.NewPaths(&config.Env{Home: home}), "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	if _, err := svc.Init(ctx); err != nil {
		t.Fatal(err)
	}
	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ApplyRepositoryObservation(ctx, "owner", "kept", "external", time.Unix(1, 0), `{}`); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(home, "restore-source.db")
	if _, err := corpus.Backup(ctx, svc.databasePath(), source, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ApplyRepositoryObservation(ctx, "owner", "removed", "external", time.Unix(2, 0), `{}`); err != nil {
		t.Fatal(err)
	}
	safety := filepath.Join(home, "safety.db")
	report, err := svc.RestoreCorpus(ctx, source, safety)
	if err != nil {
		t.Fatal(err)
	}
	if report.SafetyBackup == nil || report.SafetyBackup.Path != safety {
		t.Fatalf("restore report = %+v", report)
	}
	if _, err := os.Stat(safety); err != nil {
		t.Fatalf("safety backup: %v", err)
	}
	read, err := svc.openReadOnlyCorpus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	repos, err := read.ListRepositories(ctx, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].Name != "kept" {
		t.Fatalf("repositories = %+v", repos)
	}
}

func TestProjectionResultExposesSourceAndAttemptState(t *testing.T) {
	t.Parallel()
	started := time.Unix(100, 0).UTC()
	finished := time.Unix(200, 0).UTC()
	result := projectionResult(corpus.ProjectionState{
		Name: "threads_fts", Version: "threads-fts-v1", Status: corpus.ProjectionStatusFailed,
		RowCount: 42, SourceRevision: "source-revision", ContentHash: "sha256",
		AttemptStatus: corpus.ProjectionAttemptFailed, AttemptStartedAt: started,
		AttemptFinishedAt: finished, AttemptError: "canceled",
	})
	if result.SourceRevision != "source-revision" || result.ContentHash != "sha256" ||
		result.AttemptStatus != "failed" || result.AttemptError != "canceled" ||
		result.AttemptStartedAt == "" || result.AttemptFinishedAt == "" {
		t.Fatalf("projection result = %+v", result)
	}
}

func TestListCorpusInventoryCombinesSchemaRepositoriesAndProjections(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	home := t.TempDir()
	svc, err := New(config.NewPaths(&config.Env{Home: home}), "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	if _, err := svc.Init(ctx); err != nil {
		t.Fatal(err)
	}
	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "1", time.Unix(1, 0), `{"stored":true}`); err != nil {
		t.Fatal(err)
	}

	result, err := svc.ListCorpusInventory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Schema == nil || result.Schema.State != "current" || len(result.Repositories) != 1 || result.Repositories[0].Repo != "owner/repo" {
		t.Fatalf("inventory = %+v", result)
	}
	if len(result.Projections) != 3 || result.DatabaseBytes == 0 || result.SizeAttribution == "" {
		t.Fatalf("inventory metadata = %+v", result)
	}
}
