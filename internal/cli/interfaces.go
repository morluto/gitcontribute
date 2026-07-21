package cli

import (
	"context"
	"time"

	"github.com/morluto/gitcontribute/internal/health"
	"github.com/morluto/gitcontribute/internal/lens"
)

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

type UpgradeReport struct {
	Context string `json:"context"`
	Current string `json:"current"`
	Latest  string `json:"latest,omitempty"`
	Status  string `json:"status"`
	Command string `json:"command,omitempty"`
}

type MetadataResult struct {
	Name          string          `json:"name"`
	Version       string          `json:"version"`
	GoVersion     string          `json:"go_version"`
	OS            string          `json:"os"`
	Architecture  string          `json:"architecture"`
	SchemaVersion int64           `json:"schema_version"`
	ConfigPath    string          `json:"config_path"`
	CorpusPath    string          `json:"corpus_path"`
	Capabilities  []string        `json:"capabilities"`
	Features      map[string]bool `json:"features"`
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
	Sources []SourceResult `json:"sources"`
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

func (r RepoRef) String() string { return r.Owner + "/" + r.Repo }

// MCPOptions carries MCP server startup options.
type MCPOptions struct {
	Transport string
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
	Repo          RepoRef `json:"repo"`
	Remote        string  `json:"remote"`
	DefaultBranch string  `json:"default_branch"`
	CommitSHA     string  `json:"commit_sha"`
	Files         int     `json:"files"`
	Bytes         int     `json:"bytes"`
	Indexed       bool    `json:"indexed"`
	Inserted      bool    `json:"inserted"`
	AcquiredAt    string  `json:"acquired_at"`
	Message       string  `json:"message"`
}

// HealthService exposes deterministic offline repository health metrics.
type HealthService interface {
	RepositoryHealthWithOptions(ctx context.Context, repo RepoRef, opts health.Options) (*health.Report, error)
}

// WorkspaceService is the optional workspace management capability used by the CLI.
type WorkspaceService interface {
	CreateWorkspace(ctx context.Context, investigationID string, opts WorkspaceCreateOptions) (*WorkspaceResult, error)
	ShowWorkspace(ctx context.Context, id string) (*WorkspaceResult, error)
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
	CreatedAt       string  `json:"created_at"`
}

// ValidationService is the optional validation management capability used by the CLI.
type ValidationService interface {
	DefineValidation(ctx context.Context, investigationID string, opts DefineValidationOptions) (*ValidationResult, error)
	ShowValidation(ctx context.Context, id string) (*ValidationResult, error)
	RunValidation(ctx context.Context, id string, opts RunValidationOptions) (*ValidationRunResult, error)
	CompareValidation(ctx context.Context, baseRunID, candidateRunID string) (*ValidationComparisonResult, error)
}

// RunValidationOptions carries the run target and explicit host-execution authorization.
type RunValidationOptions struct {
	Kind    string
	Execute bool
}

// DefineValidationOptions carries an explicit validation definition.
type DefineValidationOptions struct {
	Kind           string
	Command        string
	WorkingDir     string
	BaseWorkingDir string
	CandidateDir   string
	Env            []string
	Timeout        time.Duration
	MaxOutputBytes int64
	Observation    *ValidationObservationContract
}

// ValidationResult is a stored validation definition view.
type ValidationResult struct {
	ID              string                         `json:"id"`
	InvestigationID string                         `json:"investigation_id"`
	Kind            string                         `json:"kind"`
	Command         []string                       `json:"command"`
	WorkingDir      string                         `json:"working_dir"`
	BaseWorkingDir  string                         `json:"base_working_dir,omitempty"`
	CandidateDir    string                         `json:"candidate_dir,omitempty"`
	Env             []string                       `json:"environment_allowlist,omitempty"`
	Timeout         string                         `json:"timeout,omitempty"`
	MaxOutputBytes  int64                          `json:"max_output_bytes,omitempty"`
	Observation     *ValidationObservationContract `json:"observation,omitempty"`
	CreatedAt       string                         `json:"created_at"`
}

// ValidationRunResult is the captured outcome of one validation run.
type ValidationRunResult struct {
	ID                string                        `json:"id"`
	DefinitionID      string                        `json:"definition_id"`
	InvestigationID   string                        `json:"investigation_id"`
	Kind              string                        `json:"kind"`
	ExitCode          int                           `json:"exit_code"`
	Stdout            string                        `json:"stdout"`
	Stderr            string                        `json:"stderr"`
	Truncated         bool                          `json:"truncated"`
	Error             string                        `json:"error,omitempty"`
	Classification    string                        `json:"classification"`
	ObservationStatus string                        `json:"observation_status"`
	Observations      []ValidationObservationResult `json:"observations,omitempty"`
	StartedAt         string                        `json:"started_at"`
	CompletedAt       string                        `json:"completed_at"`
}

// ValidationComparisonResult classifies a base run against a candidate run.
type ValidationComparisonResult struct {
	Base           *ValidationRunResult `json:"base"`
	Candidate      *ValidationRunResult `json:"candidate"`
	Classification string               `json:"classification"`
	Explanation    string               `json:"explanation"`
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
	Guidance string
	Success  string
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
}

// DraftResult is a rendered, locally-stored contribution draft.
type DraftResult struct {
	OpportunityID string `json:"opportunity_id"`
	Kind          string `json:"kind"`
	Title         string `json:"title"`
	Body          string `json:"body"`
	RenderedAt    string `json:"rendered_at"`
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
	Lenses []LensResult `json:"lenses"`
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

// ExportService renders redacted, deterministic local bundles.
type ExportService interface {
	ExportDossier(ctx context.Context, repo RepoRef, format string) (*ExportResult, error)
	ExportEvidence(ctx context.Context, investigationID, format string) (*ExportResult, error)
}

type ExportResult struct {
	Kind    string `json:"kind"`
	Format  string `json:"format"`
	Content string `json:"content"`
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
