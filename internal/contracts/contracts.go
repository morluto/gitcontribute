package contracts

import (
	"context"
	"encoding/json"
	"time"

	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/health"
	"github.com/morluto/gitcontribute/internal/lens"
	"github.com/morluto/gitcontribute/internal/radar"
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

// Service is the product-owned application interface used by the CLI and MCP
// adapters. Implementations live outside the CLI package and must not leak
// CLI or transport concerns.
type Service interface {
	Init(ctx context.Context) (*InitResult, error)
	Status(ctx context.Context) (*StatusResult, error)
	Sync(ctx context.Context, repo RepoRef) (*SyncResult, error)
	Search(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error)
	Dossier(ctx context.Context, repo RepoRef) (*DossierResult, error)
	Index(ctx context.Context, repo RepoRef, path string) (*IndexResult, error)
}

// MCPRunner is the product-owned boundary for running an MCP server. The CLI
// adapter dispatches to it and does not own MCP protocol details.
type MCPRunner interface {
	Run(ctx context.Context, opts MCPOptions) error
}

// TUIRunner is the terminal UI adapter boundary.
type TUIRunner interface {
	Run(ctx context.Context, opts TUIOptions) error
}

// ControlService exposes local configuration and diagnostic capabilities.
// Implementations must not perform network access for Metadata or ControlStatus.
type ControlService interface {
	Metadata(ctx context.Context) (*MetadataResult, error)
	Configure(ctx context.Context, opts ConfigureOptions) (*ConfigureResult, error)
	ControlStatus(ctx context.Context) (*ControlStatusResult, error)
	Doctor(ctx context.Context) (*DoctorResult, error)
}

type UpgradeService interface {
	Upgrade(ctx context.Context, opts UpgradeOptions) (*UpgradeReport, error)
}

type UpgradeOptions struct {
	Check bool
	Yes   bool
}

type MetadataResult struct {
	Name                   string          `json:"name"`
	Version                string          `json:"version"`
	GoVersion              string          `json:"go_version"`
	OS                     string          `json:"os"`
	Architecture           string          `json:"architecture"`
	SchemaVersion          int64           `json:"schema_version"`
	SupportedSchemaVersion int64           `json:"supported_schema_version"`
	ConfigPath             string          `json:"config_path"`
	CorpusPath             string          `json:"corpus_path"`
	Capabilities           []string        `json:"capabilities"`
	Features               map[string]bool `json:"features"`
}

// ConfigureOptions uses pointers so callers can distinguish an omitted value
// from a deliberate zero value. Tokens themselves are never accepted here.
type ConfigureOptions struct {
	Database         *string
	TokenSource      *string
	TokenSourceKey   *string
	CrawlBudget      *int
	CrawlConcurrency *int
	CrawlRetryLimit  *int
	CrawlTimeout     *string
	OutputFormat     *string
	OutputMaxResults *int
	DryRun           bool
}

type ConfigResult struct {
	Database         string `json:"database"`
	TokenSource      string `json:"token_source"`
	TokenSourceKey   string `json:"token_source_key,omitempty"`
	CrawlBudget      int    `json:"crawl_budget"`
	CrawlConcurrency int    `json:"crawl_concurrency"`
	CrawlRetryLimit  int    `json:"crawl_retry_limit"`
	CrawlTimeout     string `json:"crawl_timeout"`
	OutputFormat     string `json:"output_format"`
	OutputMaxResults int    `json:"output_max_results"`
}

type ConfigureResult struct {
	Path    string       `json:"path"`
	DryRun  bool         `json:"dry_run"`
	Changed bool         `json:"changed"`
	Config  ConfigResult `json:"config"`
}

type ControlCounts struct {
	Repositories  int `json:"repositories"`
	Threads       int `json:"threads"`
	Sources       int `json:"sources"`
	FrontierReady int `json:"frontier_ready"`
	ActiveRuns    int `json:"active_runs"`
	ActiveJobs    int `json:"active_jobs"`
}

type ControlStatusResult struct {
	Healthy        bool             `json:"healthy"`
	Corpus         string           `json:"corpus"`
	Version        string           `json:"version"`
	SchemaVersion  int64            `json:"schema_version"`
	Counts         ControlCounts    `json:"counts"`
	FreshestSource string           `json:"freshest_source,omitempty"`
	RateLimits     []RateLimitState `json:"rate_limits,omitempty"`
	Warnings       []string         `json:"warnings"`
}

type RateLimitState struct {
	Resource   string `json:"resource"`
	Limit      int    `json:"limit"`
	Remaining  int    `json:"remaining"`
	Used       int    `json:"used"`
	ResetAt    string `json:"reset_at,omitempty"`
	StatusCode int    `json:"status_code"`
	ObservedAt string `json:"observed_at"`
}

type DoctorCheck struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Required bool   `json:"required"`
	Message  string `json:"message"`
}

type DoctorResult struct {
	Healthy bool          `json:"healthy"`
	Checks  []DoctorCheck `json:"checks"`
}

// DiscoveryService is the optional source and crawl capability used by the
// CLI without enlarging the core local archive contract.
type DiscoveryService interface {
	AddSearchSource(ctx context.Context, name, query string) (*SourceResult, error)
	AddRepoSource(ctx context.Context, name string, refs []RepoRef) (*SourceResult, error)
	AddGHArchiveSource(ctx context.Context, name string, events []string) (*SourceResult, error)
	ShowSource(ctx context.Context, name string) (*SourceResult, error)
	ListSources(ctx context.Context) (*SourceListResult, error)
	Crawl(ctx context.Context, name string, opts CrawlOptions) (*CrawlResult, error)
}

type SourceResult struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Definition string `json:"definition"`
	Enabled    bool   `json:"enabled"`
}

type SourceListResult struct {
	Sources   []SourceResult `json:"sources"`
	Total     int            `json:"total"`
	Truncated bool           `json:"truncated"`
}

type CrawlOptions struct {
	Since  time.Duration
	Budget int
}

type CrawlResult struct {
	Source       string `json:"source"`
	Windows      int    `json:"windows"`
	Repositories int    `json:"repositories"`
	Threads      int    `json:"threads,omitempty"`
	Events       int    `json:"events,omitempty"`
	Requests     int    `json:"requests"`
	Imported     int    `json:"imported,omitempty"`
	Skipped      int    `json:"skipped,omitempty"`
	Failures     int    `json:"failures,omitempty"`
	Checkpoint   string `json:"checkpoint"`
}

// TailService exposes continuous source execution separately from the stable
// discovery interface so lightweight clients can opt in explicitly.
type TailService interface {
	TailSource(ctx context.Context, name string, opts TailOptions) (*TailResult, error)
}

type TailOptions struct {
	Since    time.Duration
	Budget   int
	Interval time.Duration
	Once     bool
}

type TailResult struct {
	Source     string       `json:"source"`
	Iterations int          `json:"iterations"`
	Last       *CrawlResult `json:"last,omitempty"`
}

// RepoRef identifies a GitHub repository.
type RepoRef struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
}

// MCPOptions carries MCP server startup options.
type MCPOptions struct {
	Transport string
	Toolsets  []string
	ReadOnly  bool
}

type TUIOptions struct {
	Repo RepoRef
	JSON bool
}

// JobService exposes durable background job state and cancellation.
type JobService interface {
	ListJobs(ctx context.Context, status string, limit int) (*JobListResult, error)
	GetJob(ctx context.Context, id string) (*JobResult, error)
	CancelJob(ctx context.Context, id string) (*JobResult, error)
}

type JobResult struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Status       string `json:"status"`
	Request      string `json:"request,omitempty"`
	Result       string `json:"result,omitempty"`
	Error        string `json:"error,omitempty"`
	Progress     string `json:"progress,omitempty"`
	Statistics   string `json:"statistics,omitempty"`
	CreatedAt    string `json:"created_at"`
	StartedAt    string `json:"started_at,omitempty"`
	CompletedAt  string `json:"completed_at,omitempty"`
	CancelledAt  string `json:"cancelled_at,omitempty"`
	Cancellation bool   `json:"cancellation_requested"`
}

type JobListResult struct {
	Jobs []JobResult `json:"jobs"`
}

// WorkflowExtensionService exposes the evidence-first workflow capabilities
// that sit beyond the original compact CLI service contract.
type WorkflowExtensionService interface {
	UpdateHypothesisForCLI(ctx context.Context, id string, opts HypothesisUpdateOptions) (any, error)
	TransitionHypothesisForCLI(ctx context.Context, id, status, rationale string) (any, error)
	CheckDuplicatesForCLI(ctx context.Context, target, id string, limit int) (any, error)
	CheckCollisionsForCLI(ctx context.Context, target, id string, limit int) (any, error)
	SetCollisionForCLI(ctx context.Context, id, status, rationale string) (any, error)
	RecordEvidenceForCLI(ctx context.Context, opts RecordEvidenceOptions) (any, error)
	WorkspaceDiffForCLI(ctx context.Context, id string) (any, error)
	PrepareReviewForCLI(ctx context.Context, opportunityID, workspaceID string) (any, error)
}

type HypothesisUpdateOptions struct {
	Title              *string
	Description        *string
	Category           *string
	ExpectedBehavior   *string
	ObservedBehavior   *string
	PotentialImpact    *string
	OpenQuestions      []string
	AffectedComponents []string
	Rationale          string
}

type RecordEvidenceOptions struct {
	InvestigationID string
	HypothesisID    string
	OpportunityID   string
	Type            string
	Relation        string
	Description     string
}

type DossierExtensionService interface {
	BuildDossierForCLI(ctx context.Context, repo RepoRef) (any, error)
	GetDossierForCLI(ctx context.Context, repo RepoRef) (any, error)
	ExtractSeedsForCLI(ctx context.Context, repo RepoRef, classes, polarities []string, limit int) (any, error)
}

// InvestigationService is the optional investigation and opportunity
// management capability used by the CLI.
type InvestigationService interface {
	StartInvestigation(ctx context.Context, repo RepoRef, commit, lens string) (*InvestigationResult, error)
	ShowInvestigation(ctx context.Context, id string) (*InvestigationResult, error)
	ListInvestigations(ctx context.Context) (*InvestigationListResult, error)
	AddHypothesis(ctx context.Context, investigationID, title, description, category string) (*HypothesisResult, error)
	ListHypotheses(ctx context.Context, investigationID string) (*HypothesisListResult, error)
	PromoteOpportunity(ctx context.Context, hypothesisID, problem, scope, impact, effort string, confidence float64) (*OpportunityResult, error)
	ShowOpportunity(ctx context.Context, id string) (*OpportunityResult, error)
	ListOpportunities(ctx context.Context, investigationID string) (*OpportunityListResult, error)
	SetOpportunityStatus(ctx context.Context, id, status, rationale string) (*OpportunityResult, error)
}

// InvestigationListResult is a collection of investigations.
type InvestigationListResult struct {
	Investigations []InvestigationResult `json:"investigations"`
}

// HypothesisListResult is a collection of hypotheses.
type HypothesisListResult struct {
	Hypotheses []HypothesisResult `json:"hypotheses"`
}

// OpportunityResult is a single opportunity view.
type OpportunityResult struct {
	ID               string  `json:"id"`
	InvestigationID  string  `json:"investigation_id"`
	HypothesisID     string  `json:"hypothesis_id"`
	Title            string  `json:"title"`
	ProblemStatement string  `json:"problem_statement"`
	Category         string  `json:"category"`
	Scope            string  `json:"scope"`
	Impact           string  `json:"impact"`
	ExpectedEffort   string  `json:"expected_effort"`
	Confidence       float64 `json:"confidence"`
	CollisionStatus  string  `json:"collision_status"`
	Status           string  `json:"status"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

// OpportunityListResult is a collection of opportunities.
type OpportunityListResult struct {
	Opportunities []OpportunityResult `json:"opportunities"`
	Filter        string              `json:"filter,omitempty"`
}

// SearchOptions carries parameters for a local corpus search.
type SearchOptions struct {
	Kind         string
	Repo         string
	State        string
	StateReason  string
	Merged       *bool
	Author       string
	Association  string
	Assignee     string
	Labels       []string
	UpdatedAfter time.Time
	Limit        int
	Cursor       string
	Lens         string
	Sort         string
	MatchMode    string
}

// InitResult is the result of initializing a local corpus.
type InitResult struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// StatusResult reports the health and identity of the local corpus.
type StatusResult struct {
	Healthy bool   `json:"healthy"`
	Corpus  string `json:"corpus"`
	Version string `json:"version"`
	Message string `json:"message"`
}

// DossierResult is a summary view of a repository.
type DossierResult struct {
	Repo       RepoRef  `json:"repo"`
	Summary    string   `json:"summary"`
	Language   string   `json:"language"`
	Stars      int      `json:"stars"`
	OpenIssues int      `json:"open_issues"`
	Coverage   []string `json:"coverage"`
	Freshness  string   `json:"freshness"`
}

// IndexResult reports one immutable local code snapshot.
type IndexResult struct {
	Repo     RepoRef `json:"repo"`
	Path     string  `json:"path"`
	Commit   string  `json:"commit"`
	Files    int     `json:"files"`
	Bytes    int     `json:"bytes"`
	Inserted bool    `json:"inserted"`
	Message  string  `json:"message"`
}

// AcquisitionService exposes explicit managed clone/fetch and indexing.
type AcquisitionService interface {
	Acquire(ctx context.Context, repo RepoRef, remote string) (*AcquisitionResult, error)
}

type AcquisitionResult struct {
	Repo          RepoRef            `json:"repo"`
	Remote        string             `json:"remote"`
	DefaultBranch string             `json:"default_branch"`
	CommitSHA     string             `json:"commit_sha"`
	Files         int                `json:"files"`
	Bytes         int                `json:"bytes"`
	Indexed       bool               `json:"indexed"`
	Inserted      bool               `json:"inserted"`
	AcquiredAt    string             `json:"acquired_at"`
	Message       string             `json:"message"`
	IndexManifest codeindex.Manifest `json:"index_manifest"`
}

// HealthService exposes deterministic offline repository health metrics.
type HealthService interface {
	RepositoryHealthWithOptions(ctx context.Context, repo RepoRef, opts health.Options) (*health.Report, error)
}

// WorkspaceService is the optional workspace management capability used by the CLI.
type WorkspaceService interface {
	CreateWorkspace(ctx context.Context, investigationID string, opts WorkspaceCreateOptions) (*WorkspaceResult, error)
	AdoptWorkspace(ctx context.Context, investigationID string, opts WorkspaceAdoptOptions) (*WorkspaceResult, error)
	ShowWorkspace(ctx context.Context, id string) (*WorkspaceResult, error)
}

// WorkspaceAdoptOptions identifies an existing local Git worktree.
type WorkspaceAdoptOptions struct {
	Path    string
	BaseRef string
	Name    string
}

// WorkspaceCreateOptions carries explicit local-write intent for workspace creation.
type WorkspaceCreateOptions struct {
	Remote       string
	BaseRef      string
	CandidateRef string
	Name         string
}

// WorkspaceResult is a durable view of a managed Git worktree.
type WorkspaceResult struct {
	ID              string  `json:"id"`
	InvestigationID string  `json:"investigation_id"`
	Repo            RepoRef `json:"repo"`
	Path            string  `json:"path"`
	Remote          string  `json:"remote"`
	BaseSHA         string  `json:"base_sha"`
	CandidateSHA    string  `json:"candidate_sha"`
	MergeBase       string  `json:"merge_base"`
	Dirty           bool    `json:"dirty"`
	HasUntracked    bool    `json:"has_untracked"`
	Ownership       string  `json:"ownership"`
	CreatedAt       string  `json:"created_at"`
}

// EvidenceService is the optional evidence reading capability used by the CLI.
type EvidenceService interface {
	ShowEvidence(ctx context.Context, investigationID string) (*EvidenceResult, error)
}

// ContributionService is the optional contribution drafting capability used by the CLI.
type ContributionService interface {
	PrepareIssue(ctx context.Context, opportunityID string, opts PrepareIssueOptions) (*DraftResult, error)
	PreparePullRequest(ctx context.Context, opportunityID string, opts PreparePROptions) (*DraftResult, error)
}

// PrepareIssueOptions carries optional fields for issue preparation.
type PrepareIssueOptions struct {
	Guidance   string
	Success    string
	ManifestID string
}

// PreparePROptions carries explicit and optional fields for PR preparation.
type PreparePROptions struct {
	WorkspaceID   string
	Approach      string
	Changes       string
	Compatibility string
	Limitations   string
	LinkedIssue   string
	Guidance      string
	ManifestID    string
}

// DraftResult is a rendered, locally-stored contribution draft.
type DraftResult struct {
	OpportunityID string `json:"opportunity_id"`
	Kind          string `json:"kind"`
	Title         string `json:"title"`
	Body          string `json:"body"`
	RenderedAt    string `json:"rendered_at"`
	ManifestID    string `json:"manifest_id,omitempty"`
}

// LensService is the optional saved-lens management capability used by the CLI.
type LensService interface {
	AddLens(ctx context.Context, name string, def lens.Definition) (*LensResult, error)
	ListLenses(ctx context.Context) (*LensListResult, error)
	ShowLens(ctx context.Context, name string) (*LensResult, error)
	ExplainLens(ctx context.Context, name, ref string, opts LensExplainOptions) (*LensExplainResult, error)
}

type LensExplainOptions struct {
	Query        string
	Repo         string
	Kind         string
	State        string
	Author       string
	Association  string
	Assignee     string
	Labels       []string
	UpdatedAfter time.Time
}

// LensResult is a saved lens definition.
type LensResult struct {
	Name       string          `json:"name"`
	Definition lens.Definition `json:"definition"`
	CreatedAt  string          `json:"created_at"`
	UpdatedAt  string          `json:"updated_at"`
}

// LensListResult is a list of saved lenses.
type LensListResult struct {
	Lenses    []LensResult `json:"lenses"`
	Total     int          `json:"total"`
	Truncated bool         `json:"truncated"`
}

// LensExplainResult explains a saved lens score for one candidate.
type LensExplainResult struct {
	Lens            LensResult           `json:"lens"`
	Candidate       LensExplainCandidate `json:"candidate"`
	Query           string               `json:"query,omitempty"`
	PopulationSize  int                  `json:"population_size"`
	PopulationScope string               `json:"population_scope"`
	EvaluatedAt     string               `json:"evaluated_at"`
	Score           float64              `json:"score"`
	Signals         []LensExplainSignal  `json:"signals"`
	MissingSignals  []string             `json:"missing_signals,omitempty"`
}

// LensExplainCandidate identifies the explained result.
type LensExplainCandidate struct {
	Kind      string  `json:"kind"`
	Repo      RepoRef `json:"repo"`
	Number    int     `json:"number,omitempty"`
	Title     string  `json:"title"`
	State     string  `json:"state,omitempty"`
	URL       string  `json:"url,omitempty"`
	UpdatedAt string  `json:"updated_at,omitempty"`
}

// LensExplainSignal exposes one signal value, normalization, and contribution.
type LensExplainSignal struct {
	Name         string  `json:"name"`
	Value        float64 `json:"value,omitempty"`
	Normalized   float64 `json:"normalized,omitempty"`
	Weight       float64 `json:"weight"`
	Contribution float64 `json:"contribution"`
	Missing      bool    `json:"missing"`
}

// CollectionService is the optional collection management capability used by
// the CLI.
type CollectionService interface {
	CreateCollection(ctx context.Context, name string) (*CollectionResult, error)
	AddCollectionMembers(ctx context.Context, name string, members []CollectionMember) (*CollectionResult, error)
	ListCollections(ctx context.Context) (*CollectionListResult, error)
}

// CollectionMember is one typed reference added to a collection.
type CollectionMember struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
}

// CollectionResult is a single named collection.
type CollectionResult struct {
	Name        string `json:"name"`
	MemberCount int    `json:"member_count"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// CollectionListResult is a list of collections.
type CollectionListResult struct {
	Collections []CollectionResult `json:"collections"`
	Total       int                `json:"total"`
	Truncated   bool               `json:"truncated"`
}

// LocalQueryService exposes bounded offline corpus queries.
type LocalQueryService interface {
	Coverage(ctx context.Context, repo RepoRef) (*CoverageResult, error)
	RunHistory(ctx context.Context, limit int) (*RunListResult, error)
	NeighborQuery(ctx context.Context, repo RepoRef, kind string, number, limit int) (*NeighborListResult, error)
}

// ArchiveThreadService exposes the bounded offline archive listing separately
// from the stable local-query interface.
type ArchiveThreadService interface {
	ArchiveThreads(ctx context.Context, repo RepoRef, kind, state string, limit int) (*ThreadListResult, error)
}

type ThreadListResult struct {
	Repo      RepoRef          `json:"repo"`
	Threads   []ThreadListItem `json:"threads"`
	Freshness string           `json:"freshness,omitempty"`
	Coverage  []CoverageFacet  `json:"coverage,omitempty"`
}

type ThreadListItem struct {
	Kind      string   `json:"kind"`
	Number    int      `json:"number"`
	State     string   `json:"state"`
	Title     string   `json:"title"`
	Author    string   `json:"author,omitempty"`
	Labels    []string `json:"labels,omitempty"`
	UpdatedAt string   `json:"updated_at"`
}

type CoverageResult struct {
	Repo   RepoRef         `json:"repo"`
	Facets []CoverageFacet `json:"facets"`
}

type CoverageFacet struct {
	Facet     string `json:"facet"`
	Present   bool   `json:"present"`
	Complete  bool   `json:"complete"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type RunListResult struct {
	Runs []RunResult `json:"runs"`
}

type RunResult struct {
	ID          int64  `json:"id"`
	Kind        string `json:"kind"`
	Status      string `json:"status"`
	StartedAt   string `json:"started_at"`
	CompletedAt string `json:"completed_at,omitempty"`
	Stats       string `json:"stats,omitempty"`
	Error       string `json:"error,omitempty"`
}

type NeighborListResult struct {
	Repo           RepoRef          `json:"repo"`
	Kind           string           `json:"kind"`
	Number         int              `json:"number"`
	SourceRevision string           `json:"source_revision"`
	Neighbors      []NeighborResult `json:"neighbors"`
}

type NeighborResult struct {
	Kind   string  `json:"kind"`
	Repo   RepoRef `json:"repo"`
	Number int     `json:"number"`
	Title  string  `json:"title"`
	State  string  `json:"state"`
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}

// TrackingService exposes local triage, contribution, and metadata portability
// operations. Implementations must keep local state separate from GitHub state
// and must not perform network access.
type TrackingService interface {
	RecordTriageEvent(ctx context.Context, opts RecordTriageEventOptions) (*TriageEventResult, error)
	ListTriageEvents(ctx context.Context, opts ListTriageEventsOptions) (*TriageEventListResult, error)
	RecordContribution(ctx context.Context, opts RecordContributionOptions) (*ContributionResult, error)
	GetContribution(ctx context.Context, id string) (*ContributionResult, error)
	ListContributions(ctx context.Context, opts ListContributionsOptions) (*ContributionListResult, error)
	RecordContributionOutcome(ctx context.Context, opts RecordContributionOutcomeOptions) (*ContributionOutcomeResult, error)
	ListContributionOutcomes(ctx context.Context, contributionID string) (*ContributionOutcomeListResult, error)
	ExportLocalMetadata(ctx context.Context, opts MetadataExportOptions) (*MetadataExportResult, error)
	ImportLocalMetadata(ctx context.Context, opts MetadataImportOptions) (*MetadataImportResult, error)
}

type RecordTriageEventOptions struct {
	Target  string
	Outcome string
	Reason  string
	Lens    string
}

type TriageEventResult struct {
	ID            string `json:"id"`
	TargetKind    string `json:"target_kind"`
	TargetRef     string `json:"target_ref"`
	Outcome       string `json:"outcome"`
	Reason        string `json:"reason,omitempty"`
	Lens          string `json:"lens,omitempty"`
	SourceEventAt string `json:"source_event_at,omitempty"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type ListTriageEventsOptions struct {
	TargetKind string
	TargetRef  string
	Outcome    string
	Lens       string
	Limit      int
}

type TriageEventListResult struct {
	Events []TriageEventResult `json:"events"`
	Limit  int                 `json:"limit"`
	Total  int                 `json:"total"`
}

// InvestigationResult is a single investigation view.
type InvestigationResult struct {
	ID               string                `json:"id"`
	Repo             RepoRef               `json:"repo"`
	CommitSHA        string                `json:"commit_sha,omitempty"`
	Lens             string                `json:"lens,omitempty"`
	Status           string                `json:"status"`
	ThreadBaseline   *ThreadBaselineResult `json:"thread_baseline,omitempty"`
	SeedHypothesisID string                `json:"seed_hypothesis_id,omitempty"`
	AuditTrail       []WorkflowAuditResult `json:"audit_trail,omitempty"`
	CreatedAt        string                `json:"created_at"`
	UpdatedAt        string                `json:"updated_at"`
}

// HypothesisResult is a single hypothesis view.
type HypothesisResult struct {
	ID              string                    `json:"id"`
	InvestigationID string                    `json:"investigation_id"`
	Title           string                    `json:"title"`
	Description     string                    `json:"description"`
	Category        string                    `json:"category"`
	Status          string                    `json:"status"`
	SourceRefs      []WorkflowSourceRefResult `json:"source_refs,omitempty"`
	Links           []WorkflowLinkResult      `json:"links,omitempty"`
	AuditTrail      []WorkflowAuditResult     `json:"audit_trail,omitempty"`
	CreatedAt       string                    `json:"created_at"`
	UpdatedAt       string                    `json:"updated_at"`
}

// RadarService exposes explainable contribution ranking as a separate,
// optional offline-read capability.
type RadarService interface {
	ContributionRadar(ctx context.Context, opts RadarOptions) (*radar.Report, error)
}

// RadarOptions scopes one bounded, offline contribution ranking.
type RadarOptions struct {
	Repo  RepoRef
	Limit int
}

// ReadinessService is the optional contribution readiness capability used by the CLI.
type ReadinessService interface {
	OpportunityReadiness(ctx context.Context, opportunityID string) (*ReadinessResult, error)
	ExplainReadiness(ctx context.Context, checkID string) (*ReadinessCheck, error)
}

// ReadinessResult is the deterministic readiness report for one opportunity.
type ReadinessResult struct {
	OpportunityID  string           `json:"opportunity_id"`
	RuleSetVersion string           `json:"rule_set_version"`
	Status         string           `json:"status"`
	EvaluatedAt    string           `json:"evaluated_at"`
	Checks         []ReadinessCheck `json:"checks"`
}

// ReadinessCheck is one explainable readiness rule result.
type ReadinessCheck struct {
	CheckID      string   `json:"check_id"`
	RuleID       string   `json:"rule_id"`
	RuleVersion  string   `json:"rule_version"`
	Status       string   `json:"status"`
	Summary      string   `json:"summary"`
	EvidenceRefs []string `json:"evidence_refs,omitempty"`
	Remediation  string   `json:"remediation,omitempty"`
	EvaluatedAt  string   `json:"evaluated_at"`
}

// RuntimeContractService reports only immutable executable compatibility
// metadata. Implementations must not inspect configuration or the corpus.
type RuntimeContractService interface {
	RuntimeContract(ctx context.Context) (*RuntimeContractResult, error)
}

// RuntimeContractResult is immutable executable compatibility metadata.
type RuntimeContractResult struct {
	Name                   string `json:"name"`
	Version                string `json:"version"`
	SupportedSchemaVersion int64  `json:"supported_schema_version"`
}

// SearchMatch is one local search result.
type SearchMatch struct {
	Kind           string   `json:"kind"`
	Repo           RepoRef  `json:"repo"`
	Title          string   `json:"title"`
	Number         int      `json:"number,omitempty"`
	State          string   `json:"state,omitempty"`
	Author         string   `json:"author,omitempty"`
	Labels         []string `json:"labels,omitempty"`
	URL            string   `json:"url,omitempty"`
	Score          float64  `json:"score"`
	Body           string   `json:"-"`
	Freshness      string   `json:"freshness,omitempty"`
	Coverage       []string `json:"coverage,omitempty"`
	MatchSource    string   `json:"match_source,omitempty"`
	MatchExcerpt   string   `json:"match_excerpt,omitempty"`
	MatchTruncated bool     `json:"match_truncated,omitempty"`
}

// SearchResult is the result of a local corpus search.
type SearchResult struct {
	Query      string        `json:"query"`
	Kind       string        `json:"kind"`
	Repo       string        `json:"repo,omitempty"`
	Limit      int           `json:"limit"`
	Total      int           `json:"total"`
	Matches    []SearchMatch `json:"matches"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

// SetupService exposes local onboarding and client-registration operations.
// Setup may install a private MCP runtime, invoke npm for the global CLI, write
// local configuration, and initialize the corpus. It must not perform GitHub
// network access or execute repository-controlled code.
type SetupService interface {
	DiscoverSetup(ctx context.Context) (*SetupDiscovery, error)
	Setup(ctx context.Context, opts SetupOptions) (*SetupReport, error)
	SetupWithProgress(ctx context.Context, opts SetupOptions, observer SetupObserver) (*SetupReport, error)
}

// SetupObserver receives repository-owned progress events. Implementations must
// return promptly and must not alter setup behavior.
type SetupObserver interface {
	SetupStarted(phase SetupPhase)
	SetupCompleted(step SetupStep)
}

// SetupPhase identifies a long-running application operation.
type SetupPhase string

const (
	// SetupPhaseCLI installs and verifies the persistent terminal command.
	SetupPhaseCLI SetupPhase = "cli"
	// SetupPhaseMCPRuntime installs the private native runtime used by MCP-only setup.
	SetupPhaseMCPRuntime SetupPhase = "mcp-runtime"
	// SetupPhaseConfiguration writes shared local configuration.
	SetupPhaseConfiguration SetupPhase = "configuration"
	// SetupPhaseCorpus initializes the local corpus.
	SetupPhaseCorpus SetupPhase = "corpus"
	// SetupPhaseClients registers the MCP server with selected clients.
	SetupPhaseClients SetupPhase = "clients"
	// SetupPhaseRepository adds the optional initial repository source.
	SetupPhaseRepository SetupPhase = "repository"
	// SetupPhaseVerification checks the completed local installation.
	SetupPhaseVerification SetupPhase = "verification"
)

// SetupDiscovery is a read-only snapshot used to choose sensible onboarding
// defaults. Discovery never authenticates, performs network access, or writes
// configuration.
type SetupDiscovery struct {
	Version               string
	Clients               []SetupClientDiscovery
	ConfiguredTokenSource string
	ConfiguredTokenKey    string
	GitHubCLIAvailable    bool
	EnvironmentKeyPresent bool
}

// SetupClientDiscovery describes one supported coding client and the exact
// configuration file GitContribute would update.
type SetupClientDiscovery struct {
	Name       string
	Path       string
	Detected   bool
	Registered bool
	Error      string
}

// SetupMode selects one complete onboarding strategy.
type SetupMode string

const (
	// SetupModeMCP installs private MCP access without a global CLI command.
	SetupModeMCP SetupMode = "mcp"
	// SetupModeCLI installs the global CLI without coding-agent configuration.
	SetupModeCLI SetupMode = "cli"
	// SetupModeBoth installs the global CLI and configures coding-agent MCP access.
	SetupModeBoth SetupMode = "both"
)

// SetupOptions selects one access mode and its explicit targets. DryRun plans
// the selected mode without invoking npm or writing local state.
type SetupOptions struct {
	Remove     bool
	Mode       SetupMode
	Clients    []string
	AllClients bool

	TokenSource    string
	TokenSourceKey string
	Repository     string
	DryRun         bool
	// Version is the release used for persistent CLI or private MCP runtime
	// installation. Empty values inherit the running service version.
	Version string
	// Executable is the packaged native program copied for MCP-only setup. It is
	// injectable so installation behavior can be tested without copying the test
	// process itself.
	Executable string
}

// SetupStep describes one independently observable setup effect. Status is a
// stable human-readable state such as "would install", "installed",
// "configured", "not installed", or "failed".
type SetupStep struct {
	Name    string `json:"name"`
	Path    string `json:"path,omitempty"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// SetupAuthentication describes the credential source recorded by setup. It
// never contains a credential value and does not imply that credentials were
// read or validated.
type SetupAuthentication struct {
	Method string `json:"method"`
	Key    string `json:"key,omitempty"`
}

// SetupMCPCommand preserves the executable and argument boundaries registered
// with coding clients.
type SetupMCPCommand struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// SetupReport records the effects attempted by setup. MCPCommand is populated
// only when MCP was selected. A report may contain both successful and failed
// independent steps.
type SetupReport struct {
	Operation         string                  `json:"operation"`
	DryRun            bool                    `json:"dry_run"`
	MCPCommand        *SetupMCPCommand        `json:"mcp_command,omitempty"`
	MCPCommandPending bool                    `json:"mcp_command_pending,omitempty"`
	RestartClients    []string                `json:"restart_clients,omitempty"`
	Authentication    *SetupAuthentication    `json:"authentication,omitempty"`
	Corpus            *CorpusInspectionResult `json:"corpus,omitempty"`
	Steps             []SetupStep             `json:"steps"`
}

// SyncResult reports the outcome of syncing a repository.
type SyncResult struct {
	Repo            RepoRef `json:"repo"`
	Updated         int     `json:"updated"`
	Requests        int     `json:"requests"`
	PlannedRequests int     `json:"planned_requests"`
	RequestBudget   int     `json:"request_budget"`
	Capped          bool    `json:"request_capped"`
	Message         string  `json:"message"`
}

// SyncPlanResult is the conservative request ceiling computed before a sync
// obtains a GitHub reader or writes the corpus.
type SyncPlanResult struct {
	Repo                 RepoRef `json:"repo"`
	FixedRequests        int     `json:"fixed_requests"`
	ThreadRequestCeiling int     `json:"thread_request_ceiling"`
	PlannedRequests      int     `json:"planned_requests"`
	RequestBudget        int     `json:"request_budget"`
	MaxPages             int     `json:"max_pages"`
	ExactThreads         int     `json:"exact_threads"`
}

// MetadataExportOptions bounds a local tracking metadata export.
type MetadataExportOptions struct {
	Limit int
}

// MetadataExportResult contains the exported tracking bundle and record counts.
type MetadataExportResult struct {
	SchemaVersion        int             `json:"schema_version"`
	Data                 json.RawMessage `json:"data"`
	TriageEvents         int             `json:"triage_events"`
	Contributions        int             `json:"contributions"`
	ContributionOutcomes int             `json:"contribution_outcomes"`
	Evidence             int             `json:"evidence"`
}

// MetadataImportOptions carries a serialized local tracking bundle.
type MetadataImportOptions struct {
	Data []byte
}

// MetadataImportResult reports the imported bundle version and record counts.
type MetadataImportResult struct {
	SchemaVersion        int `json:"schema_version"`
	TriageEvents         int `json:"triage_events"`
	Contributions        int `json:"contributions"`
	ContributionOutcomes int `json:"contribution_outcomes"`
	Evidence             int `json:"evidence"`
}

// RecordContributionOptions describes a prepared contribution to persist.
type RecordContributionOptions struct {
	OpportunityID string
	Kind          string
	Title         string
	Body          string
	Reference     string
	ReferenceURL  string
}

// ContributionResult is the stored representation of a prepared contribution.
type ContributionResult struct {
	ID            string         `json:"id"`
	OpportunityID string         `json:"opportunity_id"`
	Kind          string         `json:"kind"`
	Title         string         `json:"title"`
	Body          string         `json:"body,omitempty"`
	Reference     string         `json:"reference,omitempty"`
	ReferenceURL  string         `json:"reference_url,omitempty"`
	PreparedAt    string         `json:"prepared_at"`
	SubmittedAt   string         `json:"submitted_at,omitempty"`
	CreatedAt     string         `json:"created_at"`
	UpdatedAt     string         `json:"updated_at"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// ListContributionsOptions filters and bounds contribution history.
type ListContributionsOptions struct {
	OpportunityID string
	Kind          string
	Limit         int
}

// ContributionListResult contains a bounded contribution history page.
type ContributionListResult struct {
	Contributions []ContributionResult `json:"contributions"`
	Limit         int                  `json:"limit"`
	Total         int                  `json:"total"`
}

// RecordContributionOutcomeOptions describes an outcome to attach to a contribution.
type RecordContributionOutcomeOptions struct {
	ContributionID string
	Outcome        string
	Reason         string
}

// ContributionOutcomeResult is a stored contribution outcome.
type ContributionOutcomeResult struct {
	ID             string `json:"id"`
	ContributionID string `json:"contribution_id"`
	Outcome        string `json:"outcome"`
	Reason         string `json:"reason,omitempty"`
	SourceEventAt  string `json:"source_event_at,omitempty"`
	CreatedAt      string `json:"created_at"`
}

// ContributionOutcomeListResult contains all stored outcomes for one contribution.
type ContributionOutcomeListResult struct {
	ContributionID string                      `json:"contribution_id"`
	Outcomes       []ContributionOutcomeResult `json:"outcomes"`
}

// UpgradeStage reports one inspectable upgrade stage.
type UpgradeStage struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
	Target  string `json:"target,omitempty"`
	Message string `json:"message,omitempty"`
}

// UpgradeConfiguredClient reports one coding client's runtime registration.
type UpgradeConfiguredClient struct {
	Name    string `json:"name"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// UpgradeReport describes installation, compatibility, activation, and rollback.
type UpgradeReport struct {
	Context           string                    `json:"context"`
	Current           string                    `json:"current"`
	Latest            string                    `json:"latest,omitempty"`
	Status            string                    `json:"status"`
	Command           string                    `json:"command,omitempty"`
	Action            string                    `json:"action,omitempty"`
	Rollback          string                    `json:"rollback,omitempty"`
	RestartClients    []string                  `json:"restart_clients,omitempty"`
	Stages            []UpgradeStage            `json:"stages"`
	ConfiguredClients []UpgradeConfiguredClient `json:"configured_clients,omitempty"`
}

// ValidationService is the optional validation management capability used by the CLI.
type ValidationService interface {
	DefineValidation(ctx context.Context, investigationID string, opts DefineValidationOptions) (*ValidationResult, error)
	ShowValidation(ctx context.Context, id string) (*ValidationResult, error)
	RunValidation(ctx context.Context, id string, opts RunValidationOptions) (*ValidationRunResult, error)
	RunValidationGroup(ctx context.Context, id string, opts RepeatValidationOptions) (*ValidationRunGroupResult, error)
	CompareValidation(ctx context.Context, baseRunID, candidateRunID string) (*ValidationComparisonResult, error)
}

// RunValidationOptions carries the run target and explicit host-execution authorization.
type RunValidationOptions struct {
	Kind    string
	Execute bool
}

// RepeatValidationOptions bounds an explicitly authorized run group.
type RepeatValidationOptions struct {
	Kinds          []string
	RunCount       int
	Concurrency    int
	PerRunTimeout  time.Duration
	OverallTimeout time.Duration
	SampleInterval time.Duration
	Execute        bool
}

// DefineValidationOptions carries an explicit validation definition.
type DefineValidationOptions struct {
	Kind                 string
	Command              string
	WorkingDir           string
	BaseWorkingDir       string
	CandidateDir         string
	WorkspaceID          string
	BaseWorkspaceID      string
	CandidateWorkspaceID string
	Env                  []string
	Timeout              time.Duration
	MaxOutputBytes       int64
	Observation          *ValidationObservationContract
	Protocol             string
	ReadinessTimeout     time.Duration
}

// ValidationResult is a stored validation definition view.
type ValidationResult struct {
	ID                   string                         `json:"id"`
	InvestigationID      string                         `json:"investigation_id"`
	Kind                 string                         `json:"kind"`
	Command              []string                       `json:"command"`
	WorkingDir           string                         `json:"working_dir"`
	BaseWorkingDir       string                         `json:"base_working_dir,omitempty"`
	CandidateDir         string                         `json:"candidate_dir,omitempty"`
	WorkspaceID          string                         `json:"workspace_id,omitempty"`
	BaseWorkspaceID      string                         `json:"base_workspace_id,omitempty"`
	CandidateWorkspaceID string                         `json:"candidate_workspace_id,omitempty"`
	Env                  []string                       `json:"environment_allowlist,omitempty"`
	Timeout              string                         `json:"timeout,omitempty"`
	MaxOutputBytes       int64                          `json:"max_output_bytes,omitempty"`
	Observation          *ValidationObservationContract `json:"observation,omitempty"`
	Protocol             string                         `json:"protocol,omitempty"`
	ReadinessTimeout     string                         `json:"readiness_timeout,omitempty"`
	CreatedAt            string                         `json:"created_at"`
}

// ValidationRunResult is the captured outcome of one validation run.
type ValidationRunResult struct {
	ID                      string                        `json:"id"`
	DefinitionID            string                        `json:"definition_id"`
	InvestigationID         string                        `json:"investigation_id"`
	Kind                    string                        `json:"kind"`
	ExitCode                int                           `json:"exit_code"`
	Stdout                  string                        `json:"stdout"`
	Stderr                  string                        `json:"stderr"`
	Truncated               bool                          `json:"truncated"`
	Error                   string                        `json:"error,omitempty"`
	Classification          string                        `json:"classification"`
	ObservationStatus       string                        `json:"observation_status"`
	Observations            []ValidationObservationResult `json:"observations,omitempty"`
	StartedAt               string                        `json:"started_at"`
	CompletedAt             string                        `json:"completed_at"`
	WorkspaceSnapshotBefore string                        `json:"workspace_snapshot_before,omitempty"`
	WorkspaceSnapshotAfter  string                        `json:"workspace_snapshot_after,omitempty"`
	WorkspaceBindingStatus  string                        `json:"workspace_binding_status,omitempty"`
	WorkspaceBindingReason  string                        `json:"workspace_binding_reason,omitempty"`
	Process                 ValidationProcessIdentity     `json:"process"`
	Phases                  ValidationRunPhases           `json:"phases"`
	TimeoutPhase            string                        `json:"timeout_phase,omitempty"`
	FailurePhase            string                        `json:"failure_phase,omitempty"`
	Resources               ValidationResourceTelemetry   `json:"resources"`
	Cleanup                 ValidationCleanupResult       `json:"cleanup"`
}

// ValidationProcessIdentity identifies a sampled process without conflating PID reuse.
type ValidationProcessIdentity struct {
	PID                 int32 `json:"pid,omitempty"`
	CreateTimeUnixMilli int64 `json:"create_time_unix_milli,omitempty"`
}

// ValidationRunPhases exposes process and declared protocol milestones.
type ValidationRunPhases struct {
	SpawnStartedAt    string `json:"spawn_started_at,omitempty"`
	ProcessStartedAt  string `json:"process_started_at,omitempty"`
	InitializedAt     string `json:"initialized_at,omitempty"`
	ToolsListedAt     string `json:"tools_listed_at,omitempty"`
	FirstResponseAt   string `json:"first_response_at,omitempty"`
	ExecutionEndedAt  string `json:"execution_ended_at,omitempty"`
	ShutdownStartedAt string `json:"shutdown_started_at,omitempty"`
	ShutdownCheckedAt string `json:"shutdown_checked_at,omitempty"`
}

// ValidationInt64Metric distinguishes an observed zero from unavailable data.
type ValidationInt64Metric struct {
	Value             *int64 `json:"value,omitempty"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
}

// ValidationUint64Metric distinguishes an observed zero from unavailable data.
type ValidationUint64Metric struct {
	Value             *uint64 `json:"value,omitempty"`
	UnavailableReason string  `json:"unavailable_reason,omitempty"`
}

// ValidationResourceTelemetry reports bounded process-tree high-water marks.
type ValidationResourceTelemetry struct {
	Provider                   string                 `json:"provider"`
	Platform                   string                 `json:"platform"`
	SampleInterval             string                 `json:"sample_interval"`
	SampleCount                int                    `json:"sample_count"`
	CPUTimeMillis              ValidationInt64Metric  `json:"cpu_time_millis"`
	PeakRSSBytes               ValidationUint64Metric `json:"peak_rss_bytes"`
	PeakChildCount             ValidationInt64Metric  `json:"peak_child_count"`
	SamplerOverheadNanoseconds int64                  `json:"sampler_overhead_nanoseconds"`
}

// ValidationCleanupResult reports sampled descendants after shutdown.
type ValidationCleanupResult struct {
	Status    string                      `json:"status"`
	Reason    string                      `json:"reason,omitempty"`
	Survivors []ValidationProcessIdentity `json:"survivors,omitempty"`
	CheckedAt string                      `json:"checked_at,omitempty"`
}

// ValidationAttemptResult summarizes one independently timed attempt.
type ValidationAttemptResult struct {
	Index             int                         `json:"index"`
	Kind              string                      `json:"kind"`
	RunID             string                      `json:"run_id,omitempty"`
	StartedAt         string                      `json:"started_at"`
	CompletedAt       string                      `json:"completed_at"`
	ExitCode          int                         `json:"exit_code"`
	Classification    string                      `json:"classification"`
	ObservationStatus string                      `json:"observation_status"`
	TimeoutPhase      string                      `json:"timeout_phase,omitempty"`
	FailurePhase      string                      `json:"failure_phase,omitempty"`
	Error             string                      `json:"error,omitempty"`
	Process           ValidationProcessIdentity   `json:"process"`
	Phases            ValidationRunPhases         `json:"phases"`
	Resources         ValidationResourceTelemetry `json:"resources"`
	Cleanup           ValidationCleanupResult     `json:"cleanup"`
}

// ValidationAggregateResult classifies comparable attempts for one run kind.
type ValidationAggregateResult struct {
	Kind                   string `json:"kind"`
	Requested              int    `json:"requested"`
	Completed              int    `json:"completed"`
	Passing                int    `json:"passing"`
	Failing                int    `json:"failing"`
	Inconclusive           int    `json:"inconclusive"`
	Cancelled              int    `json:"cancelled"`
	Classification         string `json:"classification"`
	ResourceClassification string `json:"resource_classification"`
}

// ValidationGroupComparisonResult compares stable base and candidate aggregates.
type ValidationGroupComparisonResult struct {
	Classification string `json:"classification"`
	Explanation    string `json:"explanation"`
}

// ValidationRunGroupResult is the bounded repeat/stress result returned to clients.
type ValidationRunGroupResult struct {
	ID                  string                           `json:"id"`
	DefinitionID        string                           `json:"definition_id"`
	InvestigationID     string                           `json:"investigation_id"`
	ConfigurationSHA256 string                           `json:"configuration_sha256"`
	RequestedRuns       int                              `json:"requested_runs"`
	CompletedRuns       int                              `json:"completed_runs"`
	Concurrency         int                              `json:"concurrency"`
	PerRunTimeout       string                           `json:"per_run_timeout"`
	OverallTimeout      string                           `json:"overall_timeout"`
	SampleInterval      string                           `json:"sample_interval"`
	Attempts            []ValidationAttemptResult        `json:"attempts"`
	Aggregates          []ValidationAggregateResult      `json:"aggregates"`
	Classification      string                           `json:"classification"`
	Comparison          *ValidationGroupComparisonResult `json:"comparison,omitempty"`
	StartedAt           string                           `json:"started_at"`
	CompletedAt         string                           `json:"completed_at"`
}

// ValidationComparisonResult classifies a base run against a candidate run.
type ValidationComparisonResult struct {
	Base           *ValidationRunResult `json:"base"`
	Candidate      *ValidationRunResult `json:"candidate"`
	Classification string               `json:"classification"`
	Explanation    string               `json:"explanation"`
}

// ValidationExpectedObservation is one assertion over captured output or a declared artifact.
type ValidationExpectedObservation struct {
	Name       string `json:"name"`
	Source     string `json:"source"`
	Matcher    string `json:"matcher"`
	Pattern    string `json:"pattern"`
	Occurrence string `json:"occurrence"`
	Path       string `json:"path,omitempty"`
}

// ValidationObservationContract ties output assertions to a proof intent.
type ValidationObservationContract struct {
	Intent    string                          `json:"intent"`
	Base      []ValidationExpectedObservation `json:"base,omitempty"`
	Candidate []ValidationExpectedObservation `json:"candidate,omitempty"`
}

// ValidationObservationResult records one evaluated output assertion.
type ValidationObservationResult struct {
	ValidationExpectedObservation
	Status  string `json:"status"`
	Excerpt string `json:"excerpt,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (r RepoRef) String() string { return r.Owner + "/" + r.Repo }

// InstallsCLI reports whether setup should install the global command.
func (m SetupMode) InstallsCLI() bool { return m == SetupModeCLI || m == SetupModeBoth }

// ConfiguresMCP reports whether setup should register coding-agent access.
func (m SetupMode) ConfiguresMCP() bool { return m == SetupModeMCP || m == SetupModeBoth }

// HasFailures reports whether setup could not produce a usable result. A nil
// report is a failure because callers cannot verify any planned or applied step.
func (r *SetupReport) HasFailures() bool {
	if r == nil {
		return true
	}
	for _, step := range r.Steps {
		if step.Status == "failed" {
			return true
		}
	}
	return false
}
