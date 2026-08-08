package cli_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
)

func (f *fakeService) InspectCorpus(context.Context) (*contracts.CorpusInspectionResult, error) {
	return &contracts.CorpusInspectionResult{Path: "/tmp/corpus.db", Exists: true, State: "migration_required", Current: 24, Target: 27, Repositories: 8, Threads: 8599}, f.err
}

func (f *fakeService) MigrateCorpus(context.Context, contracts.CorpusMigrateOptions) (*contracts.CorpusMigrationResult, error) {
	return &contracts.CorpusMigrationResult{
		Before: &contracts.CorpusInspectionResult{Current: 24, Target: 27},
		After:  &contracts.CorpusInspectionResult{Current: 27, Target: 27},
		Backup: &contracts.CorpusBackupResult{Path: "/tmp/corpus.bak", SHA256: "abc"},
	}, f.err
}

func (f *fakeService) BackupCorpus(_ context.Context, destination string) (*contracts.CorpusBackupResult, error) {
	return &contracts.CorpusBackupResult{Path: destination, SizeBytes: 42, SHA256: "abc"}, f.err
}

func (f *fakeService) RestoreCorpus(_ context.Context, source, _ string) (*contracts.CorpusRestoreResult, error) {
	return &contracts.CorpusRestoreResult{
		Source:       source,
		After:        &contracts.CorpusInspectionResult{Current: 27, Repositories: 8, Threads: 8599},
		SafetyBackup: &contracts.CorpusBackupResult{Path: "/tmp/before-restore.bak", SHA256: "def"},
		Restored:     &contracts.CorpusBackupResult{Path: "/tmp/corpus.db", SHA256: "abc"},
	}, f.err
}

func (f *fakeService) InventoryCorpus(_ context.Context, repo string) (*contracts.CorpusInventoryResult, error) {
	return &contracts.CorpusInventoryResult{Repo: repo, Issues: 3, PullRequests: 2, CodeSnapshots: 4, CodeBytes: 100}, f.err
}

func (f *fakeService) ListCorpusInventory(context.Context) (*contracts.CorpusInventoryListResult, error) {
	return &contracts.CorpusInventoryListResult{
		Schema:        &contracts.CorpusInspectionResult{State: "current", Current: 28, Target: 28},
		Repositories:  []contracts.CorpusRepositoryInventoryResult{{Repo: "owner/repo", PullRequests: 2, ThreadObservations: 2, LatestObservationAt: "2026-07-21T00:00:00Z"}},
		Projections:   []contracts.CorpusProjectionResult{{Name: "threads_fts", Version: "threads-fts-v1", Status: "stale"}},
		PendingWork:   []contracts.CorpusPendingWorkResult{{Kind: "projection", Name: "threads_fts", Status: "stale"}},
		DatabaseBytes: 200, WALBytes: 20, ObservationPayloadBytes: 40,
	}, f.err
}

func (f *fakeService) PlanCodePrune(_ context.Context, repo string, keepLatest int) (*contracts.CorpusPruneResult, error) {
	return &contracts.CorpusPruneResult{Repo: repo, DryRun: true, KeepLatest: keepLatest, Total: 2, Delete: []contracts.CorpusPruneSnapshot{{CommitSHA: "old", Bytes: 40}}, ReclaimBytes: 40}, f.err
}

func (f *fakeService) ApplyCodePrune(_ context.Context, repo string, keepLatest int, _ []string) (*contracts.CorpusPruneResult, error) {
	return &contracts.CorpusPruneResult{Repo: repo, KeepLatest: keepLatest, Total: 2, Delete: []contracts.CorpusPruneSnapshot{{CommitSHA: "old", Bytes: 40}}, Deleted: 1, ReclaimBytes: 40}, f.err
}

func (f *fakeService) PlanRepositoryRemoval(_ context.Context, repo string) (*contracts.CorpusRepositoryRemovalResult, error) {
	return &contracts.CorpusRepositoryRemovalResult{Repo: repo, DryRun: true, Revision: "revision-1", Threads: 5, RepositoryObservations: 1, ThreadObservations: 5, PreservedInvestigations: 2}, f.err
}

func (f *fakeService) ApplyRepositoryRemoval(_ context.Context, repo, _ string) (*contracts.CorpusRepositoryRemovalResult, error) {
	return &contracts.CorpusRepositoryRemovalResult{Repo: repo, Revision: "revision-1", Threads: 5, RepositoryObservations: 1, ThreadObservations: 5, PreservedInvestigations: 2}, f.err
}

func (f *fakeService) ListCorpusProjections(context.Context) (*contracts.CorpusProjectionListResult, error) {
	return &contracts.CorpusProjectionListResult{Projections: []contracts.CorpusProjectionResult{{Name: "threads_fts", Version: "threads-fts-v1", Status: "current", RowCount: 12}}}, f.err
}

func (f *fakeService) RebuildCorpusProjection(_ context.Context, name string) (*contracts.CorpusProjectionResult, error) {
	return &contracts.CorpusProjectionResult{Name: name, Version: "threads-fts-v1", Status: "current", RowCount: 12}, f.err
}

func (f *fakeService) Setup(_ context.Context, opts contracts.SetupOptions) (*contracts.SetupReport, error) {
	f.lastSetup = opts
	f.setupCalls = append(f.setupCalls, opts)
	if f.setupResult != nil {
		return f.setupResult, nil
	}
	return &contracts.SetupReport{Operation: "setup", DryRun: opts.DryRun, Steps: []contracts.SetupStep{{Name: "codex", Status: "configured"}}}, nil
}

func TestCorpusLifecycleCommands(t *testing.T) {
	t.Parallel()
	svc := &fakeService{}
	c, stdout, _ := newTestCLI(svc, nil)

	requireNoErr(t, c.Run(context.Background(), []string{"corpus", "inspect", "--json"}))
	if !strings.Contains(stdout.String(), `"state": "migration_required"`) || !strings.Contains(stdout.String(), `"repositories": 8`) {
		t.Fatalf("inspect output = %q", stdout.String())
	}

	stdout.Reset()
	requireNoErr(t, c.Run(context.Background(), []string{"corpus", "migrate", "--yes"}))
	if !strings.Contains(stdout.String(), "Migrated corpus schema 24 -> 27") || !strings.Contains(stdout.String(), "/tmp/corpus.bak") {
		t.Fatalf("migrate output = %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Next: rerun setup to finish configuration.") {
		t.Fatalf("migration next action = %q", stdout.String())
	}

	stdout.Reset()
	requireNoErr(t, c.Run(context.Background(), []string{"corpus", "restore", "/tmp/source.bak", "--yes"}))
	if !strings.Contains(stdout.String(), "Restored corpus from /tmp/source.bak") || !strings.Contains(stdout.String(), "Safety backup") {
		t.Fatalf("restore output = %q", stdout.String())
	}

	stdout.Reset()
	requireNoErr(t, c.Run(context.Background(), []string{"corpus", "inventory", "owner/repo"}))
	if !strings.Contains(stdout.String(), "3 issues") || !strings.Contains(stdout.String(), "4 code snapshots") {
		t.Fatalf("inventory output = %q", stdout.String())
	}

	stdout.Reset()
	requireNoErr(t, c.Run(context.Background(), []string{"corpus", "list"}))
	if !strings.Contains(stdout.String(), "1 repository scopes") || !strings.Contains(stdout.String(), "owner/repo") || !strings.Contains(stdout.String(), "projection threads_fts: stale") {
		t.Fatalf("corpus list output = %q", stdout.String())
	}

	stdout.Reset()
	requireNoErr(t, c.Run(context.Background(), []string{"corpus", "prune-code", "owner/repo", "--keep-latest", "1"}))
	if !strings.Contains(stdout.String(), "Would delete 1 derived code snapshots") {
		t.Fatalf("prune plan output = %q", stdout.String())
	}
	stdout.Reset()
	requireNoErr(t, c.Run(context.Background(), []string{"corpus", "prune-code", "owner/repo", "--keep-latest", "1", "--yes"}))
	if !strings.Contains(stdout.String(), "Deleted 1 derived code snapshots") {
		t.Fatalf("prune output = %q", stdout.String())
	}

	stdout.Reset()
	requireNoErr(t, c.Run(context.Background(), []string{"corpus", "remove-repository", "owner/repo"}))
	if !strings.Contains(stdout.String(), "Would remove repository owner/repo") || !strings.Contains(stdout.String(), "preserved 2 investigations") {
		t.Fatalf("repository removal plan output = %q", stdout.String())
	}
	stdout.Reset()
	requireNoErr(t, c.Run(context.Background(), []string{"corpus", "remove-repository", "owner/repo", "--yes"}))
	if !strings.Contains(stdout.String(), "Removed repository owner/repo") {
		t.Fatalf("repository removal output = %q", stdout.String())
	}

	stdout.Reset()
	requireNoErr(t, c.Run(context.Background(), []string{"corpus", "projections"}))
	if !strings.Contains(stdout.String(), "threads_fts threads-fts-v1: current") {
		t.Fatalf("projections output = %q", stdout.String())
	}
	stdout.Reset()
	requireNoErr(t, c.Run(context.Background(), []string{"corpus", "rebuild-projection", "threads_fts", "--yes"}))
	if !strings.Contains(stdout.String(), "Rebuilt projection threads_fts") {
		t.Fatalf("rebuild projection output = %q", stdout.String())
	}
}

func TestCorpusMigrateExplainsBusyCorpusRecovery(t *testing.T) {
	service := &fakeService{err: &corpus.BusyError{Path: "/tmp/corpus.db", Operation: "back up and migrate corpus"}}
	c, _, _ := newTestCLI(service, nil)
	err := c.Run(context.Background(), []string{"corpus", "migrate", "--yes"})
	if err == nil {
		t.Fatal("expected migration to fail")
	}
	for _, want := range []string{"another GitContribute process is still using the local data", "Quit or restart any coding client using GitContribute", "npx --yes gitcontribute@latest corpus migrate --yes", "No changes were made"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("migration error %q does not contain %q", err, want)
		}
	}
}

func TestCorpusDestructiveCommandsRequireConsent(t *testing.T) {
	t.Parallel()
	c, _, _ := newTestCLI(&fakeService{}, nil)
	input, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = input.Close() }()
	c.SetInput(input)
	for _, args := range [][]string{
		{"corpus", "migrate"},
		{"corpus", "restore", "/tmp/source.bak"},
	} {
		runErr := c.Run(context.Background(), args)
		requireCLIError(t, runErr, cli.ExitUsage)
	}
}
