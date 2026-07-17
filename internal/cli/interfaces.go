package cli

import (
	"context"
	"time"

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

// InvestigationResult is a single investigation view.
type InvestigationResult struct {
	ID        string  `json:"id"`
	Repo      RepoRef `json:"repo"`
	CommitSHA string  `json:"commit_sha,omitempty"`
	Lens      string  `json:"lens,omitempty"`
	Status    string  `json:"status"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

// InvestigationListResult is a collection of investigations.
type InvestigationListResult struct {
	Investigations []InvestigationResult `json:"investigations"`
}

// HypothesisResult is a single hypothesis view.
type HypothesisResult struct {
	ID              string `json:"id"`
	InvestigationID string `json:"investigation_id"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	Category        string `json:"category"`
	Status          string `json:"status"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
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
	Kind  string
	Repo  string
	Limit int
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

// SyncResult reports the outcome of syncing a repository.
type SyncResult struct {
	Repo    RepoRef `json:"repo"`
	Updated int     `json:"updated"`
	Message string  `json:"message"`
}

// SearchMatch is one local search result.
type SearchMatch struct {
	Kind   string  `json:"kind"`
	Repo   RepoRef `json:"repo"`
	Title  string  `json:"title"`
	Number int     `json:"number,omitempty"`
	URL    string  `json:"url,omitempty"`
	Score  float64 `json:"score"`
}

// SearchResult is the result of a local corpus search.
type SearchResult struct {
	Query   string        `json:"query"`
	Kind    string        `json:"kind"`
	Repo    string        `json:"repo,omitempty"`
	Limit   int           `json:"limit"`
	Total   int           `json:"total"`
	Matches []SearchMatch `json:"matches"`
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
}

// ValidationResult is a stored validation definition view.
type ValidationResult struct {
	ID              string   `json:"id"`
	InvestigationID string   `json:"investigation_id"`
	Kind            string   `json:"kind"`
	Command         []string `json:"command"`
	WorkingDir      string   `json:"working_dir"`
	BaseWorkingDir  string   `json:"base_working_dir,omitempty"`
	CandidateDir    string   `json:"candidate_dir,omitempty"`
	Env             []string `json:"environment_allowlist,omitempty"`
	Timeout         string   `json:"timeout,omitempty"`
	MaxOutputBytes  int64    `json:"max_output_bytes,omitempty"`
	CreatedAt       string   `json:"created_at"`
}

// ValidationRunResult is the captured outcome of one validation run.
type ValidationRunResult struct {
	ID              string `json:"id"`
	DefinitionID    string `json:"definition_id"`
	InvestigationID string `json:"investigation_id"`
	Kind            string `json:"kind"`
	ExitCode        int    `json:"exit_code"`
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	Truncated       bool   `json:"truncated"`
	Error           string `json:"error,omitempty"`
	Classification  string `json:"classification"`
	StartedAt       string `json:"started_at"`
	CompletedAt     string `json:"completed_at"`
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

// EvidenceResult is the evidence packet for an investigation.
type EvidenceResult struct {
	InvestigationID string         `json:"investigation_id"`
	Evidence        []EvidenceItem `json:"evidence"`
}

// EvidenceItem is a single piece of evidence.
type EvidenceItem struct {
	ID              string `json:"id"`
	Type            string `json:"type"`
	Relation        string `json:"relation"`
	Description     string `json:"description"`
	ValidationRunID string `json:"validation_run_id,omitempty"`
	OpportunityID   string `json:"opportunity_id,omitempty"`
	CreatedAt       string `json:"created_at"`
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

// ClusteringService is the optional duplicate-candidate clustering capability
// used by the CLI.
type ClusteringService interface {
	Clusters(ctx context.Context, repo RepoRef, limit int) (*ClusterListResult, error)
	Cluster(ctx context.Context, id string, limit int) (*ClusterResult, error)
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
	Repo     RepoRef         `json:"repo"`
	Total    int             `json:"total"`
	Clusters []ClusterResult `json:"clusters"`
}

// LensService is the optional saved-lens management capability used by the CLI.
type LensService interface {
	AddLens(ctx context.Context, name string, def lens.Definition) (*LensResult, error)
	ListLenses(ctx context.Context) (*LensListResult, error)
	ShowLens(ctx context.Context, name string) (*LensResult, error)
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

// ArchiveService exposes explicit network-reading archive operations.
type ArchiveService interface {
	ArchiveSync(ctx context.Context, repo RepoRef, opts ArchiveSyncOptions) (*SyncResult, error)
	Hydrate(ctx context.Context, repo RepoRef, number int, opts HydrateOptions) (*HydrateResult, error)
}

type ArchiveSyncOptions struct {
	State    string
	Since    time.Duration
	Numbers  []int
	MaxPages int
}

type HydrateOptions struct {
	Facets   []string
	MaxPages int
}

type HydrateResult struct {
	Repo     RepoRef         `json:"repo"`
	Number   int             `json:"number"`
	Kind     string          `json:"kind"`
	Facets   []HydratedFacet `json:"facets"`
	Pages    int             `json:"pages"`
	Requests int             `json:"requests"`
	Message  string          `json:"message"`
}

type HydratedFacet struct {
	Facet    string `json:"facet"`
	Count    int    `json:"count"`
	Pages    int    `json:"pages"`
	Complete bool   `json:"complete"`
}

// LocalQueryService exposes bounded offline corpus queries.
type LocalQueryService interface {
	Coverage(ctx context.Context, repo RepoRef) (*CoverageResult, error)
	RunHistory(ctx context.Context, limit int) (*RunListResult, error)
	NeighborQuery(ctx context.Context, repo RepoRef, kind string, number, limit int) (*NeighborListResult, error)
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
