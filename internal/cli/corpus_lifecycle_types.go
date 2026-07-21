package cli

import "context"

// CorpusLifecycleService owns explicit inspection, backup, migration, and restore.
type CorpusLifecycleService interface {
	InspectCorpus(ctx context.Context) (*CorpusInspectionResult, error)
	MigrateCorpus(ctx context.Context, opts CorpusMigrateOptions) (*CorpusMigrationResult, error)
	BackupCorpus(ctx context.Context, destination string) (*CorpusBackupResult, error)
	RestoreCorpus(ctx context.Context, source, safetyBackup string) (*CorpusRestoreResult, error)
	InventoryCorpus(ctx context.Context, repo string) (*CorpusInventoryResult, error)
	ListCorpusInventory(ctx context.Context) (*CorpusInventoryListResult, error)
	PlanCodePrune(ctx context.Context, repo string, keepLatest int) (*CorpusPruneResult, error)
	ApplyCodePrune(ctx context.Context, repo string, keepLatest int, expectedDelete []string) (*CorpusPruneResult, error)
	PlanRepositoryRemoval(ctx context.Context, repo string) (*CorpusRepositoryRemovalResult, error)
	ApplyRepositoryRemoval(ctx context.Context, repo, expectedRevision string) (*CorpusRepositoryRemovalResult, error)
	ListCorpusProjections(ctx context.Context) (*CorpusProjectionListResult, error)
	RebuildCorpusProjection(ctx context.Context, name string) (*CorpusProjectionResult, error)
}

// CorpusRepositoryRemovalResult describes a repository-removal preview or result.
type CorpusRepositoryRemovalResult struct {
	Repo                         string `json:"repo"`
	DryRun                       bool   `json:"dry_run"`
	Revision                     string `json:"revision"`
	RepositoryObservations       int    `json:"repository_observations"`
	Threads                      int    `json:"threads"`
	ThreadObservations           int    `json:"thread_observations"`
	FacetObservations            int    `json:"facet_observations"`
	FacetCoverage                int    `json:"facet_coverage"`
	CodeSnapshots                int    `json:"code_snapshots"`
	CodeDocuments                int    `json:"code_documents"`
	Dossiers                     int    `json:"dossiers"`
	ClusterRuns                  int    `json:"cluster_runs"`
	Clusters                     int    `json:"clusters"`
	FrontierItems                int    `json:"frontier_items"`
	DetachedTriageEvents         int    `json:"detached_triage_events"`
	RemovedPortfolioLinks        int    `json:"removed_portfolio_links"`
	RemovedResolutionRecords     int    `json:"removed_resolution_records"`
	RemovedSignalSnapshots       int    `json:"removed_signal_snapshots"`
	DetachedClusterMembers       int    `json:"detached_cluster_members"`
	PreservedInvestigations      int    `json:"preserved_investigations"`
	PreservedCrossRepoReferences int    `json:"preserved_cross_repo_references"`
}

// CorpusProjectionResult describes one derived corpus projection.
type CorpusProjectionResult struct {
	Name              string `json:"name"`
	Version           string `json:"version"`
	Status            string `json:"status"`
	RowCount          int64  `json:"row_count"`
	RefreshedAt       string `json:"refreshed_at,omitempty"`
	SourceRevision    string `json:"source_revision,omitempty"`
	ContentHash       string `json:"content_hash,omitempty"`
	AttemptStatus     string `json:"attempt_status,omitempty"`
	AttemptStartedAt  string `json:"attempt_started_at,omitempty"`
	AttemptFinishedAt string `json:"attempt_finished_at,omitempty"`
	AttemptError      string `json:"attempt_error,omitempty"`
}

// CorpusProjectionListResult contains bounded projection status records.
type CorpusProjectionListResult struct {
	Projections []CorpusProjectionResult `json:"projections"`
}

// CorpusInventoryResult summarizes one repository's stored corpus data.
type CorpusInventoryResult struct {
	Repo                   string `json:"repo"`
	Issues                 int    `json:"issues"`
	PullRequests           int    `json:"pull_requests"`
	Threads                int    `json:"threads"`
	RepositoryObservations int    `json:"repository_observations"`
	ThreadObservations     int    `json:"thread_observations"`
	FacetObservations      int    `json:"facet_observations"`
	FacetCoverage          int    `json:"facet_coverage"`
	CodeSnapshots          int    `json:"code_snapshots"`
	CodeDocuments          int    `json:"code_documents"`
	CodeBytes              int64  `json:"code_bytes"`
	DatabaseBytes          int64  `json:"database_bytes"`
	WALBytes               int64  `json:"wal_bytes"`
}

// CorpusRepositoryInventoryResult summarizes one repository in a corpus listing.
type CorpusRepositoryInventoryResult struct {
	Repo                   string `json:"repo"`
	Issues                 int    `json:"issues"`
	PullRequests           int    `json:"pull_requests"`
	Threads                int    `json:"threads"`
	RepositoryObservations int    `json:"repository_observations"`
	ThreadObservations     int    `json:"thread_observations"`
	FacetObservations      int    `json:"facet_observations"`
	FacetCoverage          int    `json:"facet_coverage"`
	LatestObservationAt    string `json:"latest_observation_at,omitempty"`
	CodeSnapshots          int    `json:"code_snapshots"`
	CodeDocuments          int    `json:"code_documents"`
	CodeBytes              int64  `json:"code_bytes"`
}

// CorpusPendingWorkResult describes incomplete explicit corpus work.
type CorpusPendingWorkResult struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// CorpusInventoryListResult summarizes all bounded corpus scopes and storage.
type CorpusInventoryListResult struct {
	Schema                  *CorpusInspectionResult           `json:"schema"`
	Repositories            []CorpusRepositoryInventoryResult `json:"repositories"`
	Projections             []CorpusProjectionResult          `json:"projections"`
	PendingWork             []CorpusPendingWorkResult         `json:"pending_work"`
	ObservationPayloadBytes int64                             `json:"observation_payload_bytes"`
	CodeBytes               int64                             `json:"code_bytes"`
	DatabaseBytes           int64                             `json:"database_bytes"`
	WALBytes                int64                             `json:"wal_bytes"`
	SizeAttribution         string                            `json:"size_attribution"`
}

// CorpusPruneSnapshot identifies a derived code snapshot selected for deletion.
type CorpusPruneSnapshot struct {
	CommitSHA string `json:"commit_sha"`
	Bytes     int64  `json:"bytes"`
}

// CorpusPruneResult describes a code-pruning preview or result.
type CorpusPruneResult struct {
	Repo         string                `json:"repo"`
	DryRun       bool                  `json:"dry_run"`
	KeepLatest   int                   `json:"keep_latest"`
	Total        int                   `json:"total_snapshots"`
	Delete       []CorpusPruneSnapshot `json:"delete"`
	Deleted      int                   `json:"deleted"`
	ReclaimBytes int64                 `json:"reclaim_bytes"`
}

// CorpusRestoreResult reports a verified corpus replacement and its safety backup.
type CorpusRestoreResult struct {
	Source       string                  `json:"source"`
	Before       *CorpusInspectionResult `json:"before,omitempty"`
	After        *CorpusInspectionResult `json:"after"`
	SafetyBackup *CorpusBackupResult     `json:"safety_backup,omitempty"`
	Restored     *CorpusBackupResult     `json:"restored"`
}

// CorpusMigrateOptions controls explicit migration backup behavior.
type CorpusMigrateOptions struct {
	BackupPath string
	NoBackup   bool
}

// CorpusMigrationStep reports one planned or completed migration step.
type CorpusMigrationStep struct {
	Version           int64  `json:"version"`
	Name              string `json:"name"`
	Phase             string `json:"phase"`
	AffectedRows      int64  `json:"affected_rows_estimate,omitempty"`
	EstimateAvailable bool   `json:"affected_rows_estimate_available,omitempty"`
	Transactional     bool   `json:"transactional,omitempty"`
	Resumable         bool   `json:"resumable,omitempty"`
	ResumeStrategy    string `json:"resume_strategy,omitempty"`
	ProjectionRebuild bool   `json:"projection_rebuild,omitempty"`
}

// CorpusInspectionResult reports side-effect-free corpus compatibility and scope.
type CorpusInspectionResult struct {
	Path                      string                `json:"path"`
	Exists                    bool                  `json:"exists"`
	SizeBytes                 int64                 `json:"size_bytes"`
	WALBytes                  int64                 `json:"wal_bytes"`
	State                     string                `json:"state"`
	Current                   int64                 `json:"current_schema"`
	Target                    int64                 `json:"target_schema"`
	Repositories              int                   `json:"repositories"`
	Threads                   int                   `json:"threads"`
	Pending                   []CorpusMigrationStep `json:"pending_migrations"`
	Problem                   string                `json:"problem,omitempty"`
	BackupRequired            bool                  `json:"backup_required"`
	RequiredDiskBytes         uint64                `json:"required_disk_bytes"`
	AvailableDiskBytes        uint64                `json:"available_disk_bytes"`
	ProjectionRebuildRequired bool                  `json:"projection_rebuild_required"`
}

// CorpusBackupResult identifies a verified SQLite backup and manifest.
type CorpusBackupResult struct {
	Path           string `json:"path"`
	ManifestPath   string `json:"manifest_path,omitempty"`
	SizeBytes      int64  `json:"size_bytes"`
	SHA256         string `json:"sha256"`
	CreatedAt      string `json:"created_at,omitempty"`
	SourceSchema   int64  `json:"source_schema,omitempty"`
	ExpectedSchema int64  `json:"expected_schema,omitempty"`
	Compatibility  string `json:"compatibility,omitempty"`
}

// CorpusMigrationResult reports the before/after schema and optional backup.
type CorpusMigrationResult struct {
	Before *CorpusInspectionResult `json:"before"`
	After  *CorpusInspectionResult `json:"after"`
	Backup *CorpusBackupResult     `json:"backup,omitempty"`
	Steps  []CorpusMigrationStep   `json:"steps"`
}
