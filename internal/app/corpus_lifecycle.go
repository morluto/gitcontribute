package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
)

func (s *Service) InspectCorpus(ctx context.Context) (*cli.CorpusInspectionResult, error) {
	cfg, err := s.loadConfig(false)
	if err != nil {
		return nil, err
	}
	inspection, err := corpus.InspectSchema(ctx, cfg.Database)
	if err != nil {
		return nil, err
	}
	return corpusInspectionResult(inspection), nil
}

func (s *Service) InventoryCorpus(ctx context.Context, repo string) (*cli.CorpusInventoryResult, error) {
	ref, err := parseRepoRef(repo)
	if err != nil {
		return nil, err
	}
	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return nil, err
	}
	inv, err := c.Inventory(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return nil, err
	}
	if inv == nil {
		return nil, cli.NewCLIError(cli.ExitNotFound, fmt.Errorf("repository %s is not stored", repo))
	}
	return &cli.CorpusInventoryResult{
		Repo: ref.String(), Issues: inv.Issues, PullRequests: inv.PullRequests, Threads: inv.Threads,
		RepositoryObservations: inv.RepositoryObservations, ThreadObservations: inv.ThreadObservations,
		FacetObservations: inv.FacetObservations, FacetCoverage: inv.FacetCoverage,
		CodeSnapshots: inv.CodeSnapshots, CodeDocuments: inv.CodeDocuments, CodeBytes: inv.CodeBytes,
		DatabaseBytes: inv.DBSize, WALBytes: inv.WALSize,
	}, nil
}

func (s *Service) ListCorpusInventory(ctx context.Context) (*cli.CorpusInventoryListResult, error) {
	inspection, err := s.InspectCorpus(ctx)
	if err != nil {
		return nil, err
	}
	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return nil, err
	}
	inv, err := c.ListInventory(ctx)
	if err != nil {
		return nil, err
	}
	states, err := c.ListProjectionStates(ctx)
	if err != nil {
		return nil, err
	}
	out := &cli.CorpusInventoryListResult{
		Schema:                  inspection,
		Repositories:            make([]cli.CorpusRepositoryInventoryResult, len(inv.Repositories)),
		Projections:             make([]cli.CorpusProjectionResult, len(states)),
		ObservationPayloadBytes: inv.ObservationPayloadBytes,
		CodeBytes:               inv.CodeBytes,
		DatabaseBytes:           inv.DBSize,
		WALBytes:                inv.WALSize,
		SizeAttribution:         "SQLite database and WAL pages are shared; observation payload and code content bytes are logical measurements, not page allocation",
	}
	for i, item := range inv.Repositories {
		result := cli.CorpusRepositoryInventoryResult{
			Repo:   domain.RepoRef{Owner: item.RepoOwner, Repo: item.RepoName}.String(),
			Issues: item.Issues, PullRequests: item.PullRequests, Threads: item.Threads,
			RepositoryObservations: item.RepositoryObservations, ThreadObservations: item.ThreadObservations,
			FacetObservations: item.FacetObservations, FacetCoverage: item.FacetCoverage,
			CodeSnapshots: item.CodeSnapshots, CodeDocuments: item.CodeDocuments, CodeBytes: item.CodeBytes,
		}
		if !item.LatestObservationAt.IsZero() {
			result.LatestObservationAt = item.LatestObservationAt.UTC().Format("2006-01-02T15:04:05Z")
		}
		out.Repositories[i] = result
	}
	for i, state := range states {
		out.Projections[i] = projectionResult(state)
	}
	for _, step := range inspection.Pending {
		out.PendingWork = append(out.PendingWork, cli.CorpusPendingWorkResult{
			Kind: "migration", Name: fmt.Sprintf("%03d_%s", step.Version, step.Name), Status: "pending",
		})
	}
	expectedVersions := map[string]string{
		corpus.ProjectionNameThreadsFTS:           corpus.ProjectionVersionThreadsFTS,
		corpus.ProjectionNameFacetObservationsFTS: corpus.ProjectionVersionFacetObservationsFTS,
		corpus.ProjectionNameCodeDocumentsFTS:     corpus.ProjectionVersionCodeDocumentsFTS,
	}
	for _, state := range states {
		expected := expectedVersions[state.Name]
		if state.Status == corpus.ProjectionStatusCurrent && state.Version == expected && state.AttemptStatus != corpus.ProjectionAttemptFailed {
			continue
		}
		detail := state.AttemptError
		if expected != "" && state.Version != expected {
			detail = fmt.Sprintf("version %s; expected %s", state.Version, expected)
		}
		out.PendingWork = append(out.PendingWork, cli.CorpusPendingWorkResult{
			Kind: "projection", Name: state.Name, Status: string(state.Status), Detail: detail,
		})
	}
	return out, nil
}

func (s *Service) PlanCodePrune(ctx context.Context, repo string, keepLatest int) (*cli.CorpusPruneResult, error) {
	ref, err := parseRepoRef(repo)
	if err != nil {
		return nil, err
	}
	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return nil, err
	}
	plan, err := c.PlanCodeSnapshotPrune(ctx, domain.RepoRef{Owner: ref.Owner, Repo: ref.Repo}, keepLatest)
	if err != nil {
		return nil, err
	}
	return codePrunePlanResult(plan), nil
}

func (s *Service) ApplyCodePrune(ctx context.Context, repo string, keepLatest int, expectedDelete []string) (*cli.CorpusPruneResult, error) {
	ref, err := parseRepoRef(repo)
	if err != nil {
		return nil, err
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	domainRef := domain.RepoRef{Owner: ref.Owner, Repo: ref.Repo}
	plan, err := c.PlanCodeSnapshotPrune(ctx, domainRef, keepLatest)
	if err != nil {
		return nil, err
	}
	if len(plan.Delete) != len(expectedDelete) {
		return nil, corpus.ErrCodeSnapshotPrunePlanStale
	}
	for i := range plan.Delete {
		if plan.Delete[i].CommitSHA != expectedDelete[i] {
			return nil, corpus.ErrCodeSnapshotPrunePlanStale
		}
	}
	result, err := c.ApplyCodeSnapshotPrune(ctx, domainRef, plan)
	if err != nil {
		return nil, err
	}
	out := codePrunePlanResult(plan)
	out.DryRun = false
	out.Deleted = result.Deleted
	out.ReclaimBytes = result.ReclaimBytes
	return out, nil
}

func codePrunePlanResult(plan *corpus.CodeSnapshotPrunePlan) *cli.CorpusPruneResult {
	out := &cli.CorpusPruneResult{Repo: plan.Ref.String(), DryRun: true, KeepLatest: plan.KeepLatest, Total: plan.TotalSnapshots, ReclaimBytes: plan.ReclaimBytes}
	for _, snapshot := range plan.Delete {
		out.Delete = append(out.Delete, cli.CorpusPruneSnapshot{CommitSHA: snapshot.CommitSHA, Bytes: snapshot.TotalBytes})
	}
	return out
}

func (s *Service) PlanRepositoryRemoval(ctx context.Context, repo string) (*cli.CorpusRepositoryRemovalResult, error) {
	ref, err := parseRepoRef(repo)
	if err != nil {
		return nil, err
	}
	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return nil, err
	}
	plan, err := c.PlanRepositoryRemoval(ctx, ref)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, cli.NewCLIError(cli.ExitNotFound, fmt.Errorf("repository %s is not stored", repo))
	}
	return repositoryRemovalResult(plan, true), nil
}

func (s *Service) ApplyRepositoryRemoval(ctx context.Context, repo, expectedRevision string) (*cli.CorpusRepositoryRemovalResult, error) {
	ref, err := parseRepoRef(repo)
	if err != nil {
		return nil, err
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	plan, err := c.PlanRepositoryRemoval(ctx, ref)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, cli.NewCLIError(cli.ExitNotFound, fmt.Errorf("repository %s is not stored", repo))
	}
	if plan.Revision != expectedRevision {
		return nil, corpus.ErrRepositoryRemovalPlanStale
	}
	result, err := c.ApplyRepositoryRemoval(ctx, ref, plan)
	if err != nil {
		return nil, err
	}
	return repositoryRemovalResult(result.Plan, false), nil
}

func repositoryRemovalResult(plan *corpus.RepositoryRemovalPlan, dryRun bool) *cli.CorpusRepositoryRemovalResult {
	return &cli.CorpusRepositoryRemovalResult{
		Repo: plan.Ref.String(), DryRun: dryRun, Revision: plan.Revision,
		RepositoryObservations: plan.RepositoryObservations, Threads: plan.Threads,
		ThreadObservations: plan.ThreadObservations, FacetObservations: plan.FacetObservations,
		FacetCoverage: plan.FacetCoverage, CodeSnapshots: plan.CodeSnapshots,
		CodeDocuments: plan.CodeDocuments, Dossiers: plan.Dossiers, ClusterRuns: plan.ClusterRuns,
		Clusters: plan.Clusters, FrontierItems: plan.FrontierItems,
		DetachedTriageEvents: plan.DetachedTriageEvents, RemovedPortfolioLinks: plan.RemovedPortfolioLinks,
		RemovedResolutionRecords: plan.RemovedResolutionRecords, RemovedSignalSnapshots: plan.RemovedSignalSnapshots,
		DetachedClusterMembers:       plan.DetachedClusterMembers,
		PreservedInvestigations:      plan.PreservedInvestigations,
		PreservedCrossRepoReferences: plan.PreservedCrossRepoReferences,
	}
}

func (s *Service) ListCorpusProjections(ctx context.Context) (*cli.CorpusProjectionListResult, error) {
	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return nil, err
	}
	states, err := c.ListProjectionStates(ctx)
	if err != nil {
		return nil, err
	}
	out := &cli.CorpusProjectionListResult{Projections: make([]cli.CorpusProjectionResult, len(states))}
	for i := range states {
		out.Projections[i] = projectionResult(states[i])
	}
	return out, nil
}

func (s *Service) RebuildCorpusProjection(ctx context.Context, name string) (*cli.CorpusProjectionResult, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	var state corpus.ProjectionState
	switch name {
	case corpus.ProjectionNameThreadsFTS:
		state, err = c.RebuildThreadSearchProjection(ctx)
	case corpus.ProjectionNameFacetObservationsFTS:
		state, err = c.RebuildFacetSearchProjection(ctx)
	case corpus.ProjectionNameCodeDocumentsFTS:
		state, err = c.RebuildCodeSearchProjection(ctx)
	default:
		return nil, fmt.Errorf("unknown corpus projection %q", name)
	}
	if err != nil {
		return nil, err
	}
	result := projectionResult(state)
	return &result, nil
}

func projectionResult(state corpus.ProjectionState) cli.CorpusProjectionResult {
	result := cli.CorpusProjectionResult{
		Name: state.Name, Version: state.Version, Status: string(state.Status), RowCount: state.RowCount,
		SourceRevision: state.SourceRevision, ContentHash: state.ContentHash,
		AttemptStatus: string(state.AttemptStatus), AttemptError: state.AttemptError,
	}
	if !state.RefreshedAt.IsZero() {
		result.RefreshedAt = state.RefreshedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	if !state.AttemptStartedAt.IsZero() {
		result.AttemptStartedAt = state.AttemptStartedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	if !state.AttemptFinishedAt.IsZero() {
		result.AttemptFinishedAt = state.AttemptFinishedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	return result
}

func corpusInspectionResult(inspection corpus.SchemaInspection) *cli.CorpusInspectionResult {
	result := &cli.CorpusInspectionResult{
		Path: inspection.Path, Exists: inspection.Exists, SizeBytes: inspection.SizeBytes, WALBytes: inspection.WALBytes,
		State: string(inspection.State), Current: inspection.Current, Target: inspection.Target,
		Repositories: inspection.Repository, Threads: inspection.Threads,
		Problem:        inspection.Problem,
		BackupRequired: inspection.BackupRequired, RequiredDiskBytes: inspection.RequiredDiskBytes,
		AvailableDiskBytes: inspection.AvailableDiskBytes, ProjectionRebuildRequired: inspection.ProjectionRebuildRequired,
		Pending: make([]cli.CorpusMigrationStep, 0, len(inspection.Pending)),
	}
	for _, step := range inspection.Pending {
		result.Pending = append(result.Pending, cli.CorpusMigrationStep{
			Version: step.Version, Name: step.Name, Phase: "pending", AffectedRows: step.AffectedRows,
			EstimateAvailable: step.EstimateAvailable, Transactional: step.Transactional,
			Resumable: step.Resumable, ResumeStrategy: step.ResumeStrategy, ProjectionRebuild: step.ProjectionRebuild,
		})
	}
	return result
}

func (s *Service) BackupCorpus(ctx context.Context, destination string) (*cli.CorpusBackupResult, error) {
	cfg, err := s.loadConfig(false)
	if err != nil {
		return nil, err
	}
	if destination == "" {
		return nil, errors.New("backup destination is required")
	}
	result, err := corpus.Backup(ctx, cfg.Database, destination, nil)
	if err != nil {
		return nil, err
	}
	return corpusBackupResult(result), nil
}

func (s *Service) RestoreCorpus(ctx context.Context, source, safetyBackup string) (*cli.CorpusRestoreResult, error) {
	cfg, err := s.loadConfig(false)
	if err != nil {
		return nil, err
	}
	if source == "" {
		return nil, errors.New("restore source is required")
	}
	before, err := corpus.InspectSchema(ctx, cfg.Database)
	if err != nil {
		return nil, err
	}
	report := &cli.CorpusRestoreResult{Source: source, Before: corpusInspectionResult(before)}
	if err := s.releaseCorpusForMigration(); err != nil {
		return nil, err
	}
	if before.Exists {
		if safetyBackup == "" {
			stamp := s.now().UTC().Format("20060102T150405.000000000Z")
			safetyBackup = filepath.Join(filepath.Dir(before.Path), fmt.Sprintf("%s.before-restore.%s.bak", filepath.Base(before.Path), stamp))
		}
	}
	safety, restored, err := corpus.RestoreWithSafetyBackup(ctx, source, cfg.Database, safetyBackup, nil)
	if safety != nil {
		report.SafetyBackup = corpusBackupResult(*safety)
	}
	if err != nil {
		if report.SafetyBackup != nil {
			return report, fmt.Errorf("restore corpus (safety backup preserved at %s): %w", report.SafetyBackup.Path, err)
		}
		return report, err
	}
	report.Restored = corpusBackupResult(restored)
	after, err := corpus.InspectSchema(ctx, cfg.Database)
	if err != nil {
		return report, err
	}
	report.After = corpusInspectionResult(after)
	return report, nil
}

func (s *Service) MigrateCorpus(ctx context.Context, opts cli.CorpusMigrateOptions) (*cli.CorpusMigrationResult, error) {
	cfg, err := s.loadConfig(false)
	if err != nil {
		return nil, err
	}
	before, err := corpus.InspectSchema(ctx, cfg.Database)
	if err != nil {
		return nil, err
	}
	if before.State == corpus.SchemaNewer {
		return nil, &corpus.UnsupportedSchemaError{Current: before.Current, Target: before.Target}
	}
	if before.State == corpus.SchemaDamaged {
		return nil, fmt.Errorf("cannot migrate damaged corpus: %s", before.Problem)
	}
	report := &cli.CorpusMigrationResult{Before: corpusInspectionResult(before)}
	if before.State == corpus.SchemaCurrent {
		report.After = corpusInspectionResult(before)
		return report, nil
	}
	if err := s.releaseCorpusForMigration(); err != nil {
		return nil, err
	}
	var destination string
	if before.Exists && !opts.NoBackup {
		destination = opts.BackupPath
		if destination == "" {
			stamp := s.now().UTC().Format("20060102T150405.000000000Z")
			destination = filepath.Join(filepath.Dir(before.Path), fmt.Sprintf("%s.schema-%d.%s.bak", filepath.Base(before.Path), before.Current, stamp))
		}
	}
	backup, err := corpus.MigrateWithBackup(ctx, cfg.Database, destination, func(progress corpus.MigrationProgress) {
		report.Steps = append(report.Steps, cli.CorpusMigrationStep{Version: progress.Version, Name: progress.Name, Phase: progress.Phase})
	})
	if backup != nil {
		report.Backup = corpusBackupResult(*backup)
	}
	if err != nil {
		if report.Backup != nil {
			return report, fmt.Errorf("migrate corpus (backup preserved at %s): %w", report.Backup.Path, err)
		}
		return report, err
	}
	after, err := corpus.InspectSchema(ctx, cfg.Database)
	if err != nil {
		return report, err
	}
	report.After = corpusInspectionResult(after)
	return report, nil
}

func corpusBackupResult(result corpus.BackupResult) *cli.CorpusBackupResult {
	out := &cli.CorpusBackupResult{
		Path: result.Path, ManifestPath: result.ManifestPath, SizeBytes: result.SizeBytes,
		SHA256: result.SHA256, SourceSchema: result.SourceSchema,
		ExpectedSchema: result.ExpectedSchema, Compatibility: string(result.Compatibility),
	}
	if !result.CreatedAt.IsZero() {
		out.CreatedAt = result.CreatedAt.UTC().Format(time.RFC3339Nano)
	}
	return out
}

func (s *Service) releaseCorpusForMigration() error {
	s.mu.Lock()
	if s.jobs != nil {
		s.mu.Unlock()
		return errors.New("cannot migrate while the job executor is active; stop the current GitContribute process and retry")
	}
	writable := s.corpus
	readOnly := s.readCorpus
	s.corpus = nil
	s.readCorpus = nil
	s.mu.Unlock()
	var closeErr error
	if writable != nil {
		closeErr = errors.Join(closeErr, writable.Close())
	}
	if readOnly != nil {
		closeErr = errors.Join(closeErr, readOnly.Close())
	}
	return closeErr
}
