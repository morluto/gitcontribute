package contracts

import (
	"context"

	"time"
)

// ThreadBaselineResult is the immutable observation revision saved at start.
type ThreadBaselineResult struct {
	Ref                  string                  `json:"ref"`
	Repository           string                  `json:"repository"`
	Kind                 string                  `json:"kind"`
	Number               int                     `json:"number"`
	ObservationID        int64                   `json:"observation_id"`
	SourceUpdatedAt      string                  `json:"source_updated_at,omitempty"`
	ObservationSequence  int64                   `json:"observation_sequence"`
	ObservedAt           string                  `json:"observed_at,omitempty"`
	Source               WorkflowSourceRefResult `json:"source"`
	DescriptionTruncated bool                    `json:"description_truncated"`
}

// ThreadInvestigationResult contains the atomically created or reused pair.
type ThreadInvestigationResult struct {
	Created       bool                 `json:"created"`
	Investigation *InvestigationResult `json:"investigation"`
	Hypothesis    *HypothesisResult    `json:"hypothesis"`
}

// WorkflowSourceRefResult is a transport-stable workflow provenance record.
type WorkflowSourceRefResult struct {
	Source     string `json:"source"`
	URL        string `json:"url,omitempty"`
	CommitSHA  string `json:"commit_sha,omitempty"`
	ObservedAt string `json:"observed_at,omitempty"`
	AsOf       string `json:"as_of,omitempty"`
}

// WorkflowLinkResult is an explicit hypothesis source link.
type WorkflowLinkResult struct {
	Kind   string                  `json:"kind"`
	Ref    string                  `json:"ref"`
	Source WorkflowSourceRefResult `json:"source"`
}

// WorkflowAuditResult records why a local workflow object changed state.
type WorkflowAuditResult struct {
	From      string `json:"from,omitempty"`
	To        string `json:"to"`
	Rationale string `json:"rationale"`
	At        string `json:"at"`
}

// ArchiveService exposes explicit network-reading archive operations.
type ArchiveService interface {
	SyncPlanningService
	ArchiveSync(ctx context.Context, repo RepoRef, opts ArchiveSyncOptions) (*SyncResult, error)
	Hydrate(ctx context.Context, repo RepoRef, number int, opts HydrateOptions) (*HydrateResult, error)
}

// SyncPlanningService computes a bounded request plan without network or
// corpus access.
type SyncPlanningService interface {
	PlanArchiveSync(ctx context.Context, repo RepoRef, opts ArchiveSyncOptions) (*SyncPlanResult, error)
}

// ArchiveSyncOptions bounds and filters one explicit archive synchronization.
type ArchiveSyncOptions struct {
	State       string
	Since       time.Duration
	Numbers     []int
	MaxPages    int
	MaxRequests int
}

// HydrateOptions selects bounded child facets for one stored thread.
type HydrateOptions struct {
	Facets   []string
	MaxPages int
}

// HydrateResult reports the facets retrieved for one issue or pull request.
type HydrateResult struct {
	Repo     RepoRef         `json:"repo"`
	Number   int             `json:"number"`
	Kind     string          `json:"kind"`
	Facets   []HydratedFacet `json:"facets"`
	Pages    int             `json:"pages"`
	Requests int             `json:"requests"`
	Message  string          `json:"message"`
}

// HydratedFacet reports one retrieved facet's item and coverage counts.
type HydratedFacet struct {
	Facet    string `json:"facet"`
	Count    int    `json:"count"`
	Pages    int    `json:"pages"`
	Complete bool   `json:"complete"`
}

// ClusteringService is the optional duplicate-candidate clustering capability
// used by the CLI.
type ClusteringService interface {
	ListClusters(ctx context.Context, repo RepoRef, limit int) (*ClusterListResult, error)
	RefreshClusters(ctx context.Context, repo RepoRef) (*ClusterRefreshResult, error)
	Cluster(ctx context.Context, id string, limit int) (*ClusterResult, error)
}

// ClusterRefreshResult attributes an explicit projection refresh.
type ClusterRefreshResult struct {
	Repo        RepoRef                   `json:"repo"`
	Disposition string                    `json:"disposition"`
	Projection  ClusterProjectionIdentity `json:"projection"`
	Stats       ClusterRefreshStats       `json:"stats"`
}

// ClusterProjectionIdentity identifies the inputs and durable run behind a projection.
type ClusterProjectionIdentity struct {
	SourceRevision     string `json:"source_revision"`
	GovernanceRevision uint64 `json:"governance_revision"`
	RuleVersion        string `json:"rule_version"`
	RunID              int64  `json:"run_id"`
}

// ClusterRefreshStats describes current projection cardinalities and bounded
// work performed by an explicit refresh.
type ClusterRefreshStats struct {
	CandidateCount  int    `json:"candidate_count"`
	RequiredPairs   uint64 `json:"required_pairs"`
	ComparedPairs   uint64 `json:"compared_pairs"`
	ClusterCount    int    `json:"cluster_count"`
	SnapshotQueries int    `json:"snapshot_queries"`
	CommitQueries   int    `json:"commit_queries"`
}

// ClusterResult is a single duplicate-candidate cluster.
type ClusterResult struct {
	StableID    string          `json:"stable_id"`
	State       string          `json:"state"`
	Canonical   ClusterMember   `json:"canonical"`
	MemberCount int             `json:"member_count"`
	Members     []ClusterMember `json:"members,omitempty"`
}

// ClusterMember is one thread inside a cluster.
type ClusterMember struct {
	Kind     string  `json:"kind"`
	Owner    string  `json:"owner"`
	Repo     string  `json:"repo"`
	Number   int     `json:"number"`
	Title    string  `json:"title,omitempty"`
	State    string  `json:"state,omitempty"`
	Score    float64 `json:"score"`
	Reason   string  `json:"reason"`
	Included bool    `json:"included"`
}

// ClusterListResult is the result of listing clusters for a repository.
type ClusterListResult struct {
	Repo       RepoRef                    `json:"repo"`
	Projection *ClusterProjectionIdentity `json:"projection,omitempty"`
	Total      int                        `json:"total"`
	Truncated  bool                       `json:"truncated"`
	Clusters   []ClusterResult            `json:"clusters"`
}

// ConcernService manages repo-local concern intake without GitHub access.
type ConcernService interface {
	CreateConcern(context.Context, ConcernCreateOptions) (*ConcernResult, error)
	ListConcerns(context.Context, ConcernListOptions) (*ConcernListResult, error)
	ShowConcern(context.Context, string) (*ConcernResult, error)
	UpdateConcern(context.Context, string, ConcernUpdateOptions) (*ConcernResult, error)
	SetConcernStatus(context.Context, string, string, string) (*ConcernResult, error)
	LinkConcern(context.Context, string, ConcernLinkOptions) (*ConcernResult, error)
	PromoteConcern(context.Context, string, ConcernPromoteOptions) (*ConcernResult, error)
}

// ConcernCreateOptions carries local concern intake fields.
type ConcernCreateOptions struct {
	Repo             RepoRef
	CommitSHA        string
	WorkspaceID      string
	Title            string
	ProblemStatement string
	SuspectedOwner   string
	Confidence       float64
	Unknowns         []string
	SuccessCriterion string
	Notes            string
	EvidenceIDs      []string
}

// ConcernUpdateOptions carries optional replacement fields.
type ConcernUpdateOptions struct {
	Title            *string
	ProblemStatement *string
	SuspectedOwner   *string
	Confidence       *float64
	Unknowns         []string
	SuccessCriterion *string
	Notes            *string
	EvidenceIDs      []string
}

// ConcernListOptions bounds an offline concern list or search.
type ConcernListOptions struct {
	Repo   RepoRef
	Status string
	Query  string
	Limit  int
}

// ConcernLinkOptions identifies one explicit relationship.
type ConcernLinkOptions struct {
	Kind       string
	TargetType string
	TargetID   string
	Note       string
}

// ConcernPromoteOptions configures atomic workflow promotion.
type ConcernPromoteOptions struct {
	Kind           string
	Category       string
	Scope          string
	Impact         string
	ExpectedEffort string
}

// ConcernLinkResult is a transport-safe relationship view.
type ConcernLinkResult struct {
	Kind       string `json:"kind"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	Note       string `json:"note,omitempty"`
}

// ConcernPromotionResult preserves downstream workflow IDs.
type ConcernPromotionResult struct {
	Kind            string `json:"kind"`
	InvestigationID string `json:"investigation_id"`
	HypothesisID    string `json:"hypothesis_id"`
	OpportunityID   string `json:"opportunity_id,omitempty"`
}

// ConcernResult omits source URLs and host paths. Source/evidence details stay
// available through their dedicated local records.
type ConcernResult struct {
	ID               string                  `json:"id"`
	Repo             RepoRef                 `json:"repo"`
	CommitSHA        string                  `json:"commit_sha,omitempty"`
	WorkspaceID      string                  `json:"workspace_id,omitempty"`
	Title            string                  `json:"title"`
	ProblemStatement string                  `json:"problem_statement"`
	SuspectedOwner   string                  `json:"suspected_owner,omitempty"`
	Confidence       float64                 `json:"confidence"`
	Unknowns         []string                `json:"unknowns,omitempty"`
	SuccessCriterion string                  `json:"success_criterion,omitempty"`
	Notes            string                  `json:"notes,omitempty"`
	EvidenceIDs      []string                `json:"evidence_ids,omitempty"`
	SourceRefCount   int                     `json:"source_ref_count"`
	Freshness        string                  `json:"freshness"`
	FreshnessReason  string                  `json:"freshness_reason"`
	Links            []ConcernLinkResult     `json:"links,omitempty"`
	Status           string                  `json:"status"`
	Promotion        *ConcernPromotionResult `json:"promotion,omitempty"`
	CreatedAt        string                  `json:"created_at"`
	UpdatedAt        string                  `json:"updated_at"`
}

// ConcernListResult contains one bounded result set.
type ConcernListResult struct {
	Concerns  []ConcernResult `json:"concerns"`
	Limit     int             `json:"limit"`
	Total     int             `json:"total"`
	Truncated bool            `json:"truncated"`
}

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

// EvidenceResult is the evidence packet for an investigation.
type EvidenceResult struct {
	InvestigationID string         `json:"investigation_id"`
	Evidence        []EvidenceItem `json:"evidence"`
}

// EvidenceItem is a single piece of evidence with derived corpus freshness.
type EvidenceItem struct {
	ID               string                         `json:"id"`
	Type             string                         `json:"type"`
	Relation         string                         `json:"relation"`
	Description      string                         `json:"description"`
	ValidationRunID  string                         `json:"validation_run_id,omitempty"`
	OpportunityID    string                         `json:"opportunity_id,omitempty"`
	SourceRefs       []WorkflowSourceRefResult      `json:"source_refs,omitempty"`
	SourceProvenance []EvidenceSourceRevisionResult `json:"source_provenance,omitempty"`
	Freshness        string                         `json:"freshness"`
	FreshnessReason  string                         `json:"freshness_reason,omitempty"`
	CreatedAt        string                         `json:"created_at"`
}

// EvidenceSourceSubjectResult identifies the independently refreshed corpus
// projection used by an evidence item.
type EvidenceSourceSubjectResult struct {
	Kind       string `json:"kind"`
	Owner      string `json:"owner"`
	Repo       string `json:"repo"`
	ThreadKind string `json:"thread_kind,omitempty"`
	Number     int    `json:"number,omitempty"`
	Facet      string `json:"facet,omitempty"`
}

// EvidenceSourceRevisionResult is the portable recorded source order.
type EvidenceSourceRevisionResult struct {
	Subject             EvidenceSourceSubjectResult `json:"subject"`
	SourceUpdatedAt     string                      `json:"source_updated_at,omitempty"`
	ObservationSequence int64                       `json:"observation_sequence"`
	ObservedAt          string                      `json:"observed_at"`
}

// ExportService renders redacted, deterministic local bundles.
type ExportService interface {
	ExportDossier(ctx context.Context, repo RepoRef, format string) (*ExportResult, error)
	ExportEvidence(ctx context.Context, investigationID, format string) (*ExportResult, error)
	ExportManifest(ctx context.Context, opportunityID string, opts ManifestExportOptions) (*ExportResult, error)
}

// ManifestExportOptions selects optional local identities for a manifest export.
type ManifestExportOptions struct {
	WorkspaceID string
	PullRequest *ManifestPullRequestRef
}

// ManifestPullRequestRef identifies one exact stored pull request.
type ManifestPullRequestRef struct {
	Owner  string
	Repo   string
	Number int
}

// ExportResult contains one rendered local export.
type ExportResult struct {
	Kind    string `json:"kind"`
	Format  string `json:"format"`
	Content string `json:"content"`
}
