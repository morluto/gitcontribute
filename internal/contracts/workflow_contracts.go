package contracts

import (
	"context"

	"github.com/morluto/gitcontribute/internal/radar"
)

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
