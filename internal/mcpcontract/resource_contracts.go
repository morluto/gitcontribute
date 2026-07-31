package mcpcontract

import (
	"context"

	"github.com/morluto/gitcontribute/internal/lens"
	"github.com/morluto/gitcontribute/internal/similarity"
)

// WorkspaceResource is the canonical host-path-free representation of a
// managed workspace.
type WorkspaceResource struct {
	SchemaVersion   string `json:"schema_version"`
	ID              string `json:"id"`
	InvestigationID string `json:"investigation_id,omitempty"`
	Owner           string `json:"owner"`
	Repo            string `json:"repo"`
	BaseSHA         string `json:"base_sha"`
	HeadSHA         string `json:"head_sha"`
	MergeBase       string `json:"merge_base,omitempty"`
	Ownership       string `json:"ownership"`
	Dirty           bool   `json:"dirty"`
	HasUntracked    bool   `json:"has_untracked"`
	CreatedAt       string `json:"created_at"`
}

// Reader is the local, read-only application boundary exposed through MCP.
// Implementations must not perform network access.
type Reader interface {
	Search(context.Context, SearchInput) (SearchOutput, error)
	SearchRepositories(context.Context, SearchRepositoriesInput) (SearchRepositoriesOutput, error)
	Repository(context.Context, RepoInput) (RepositoryOutput, error)
	Thread(context.Context, ThreadInput) (ThreadOutput, error)
	ThreadByNumber(context.Context, ThreadByNumberInput) (ThreadOutput, error)
	Dossier(context.Context, RepoInput) (DossierOutput, error)
	SearchCode(context.Context, SearchCodeInput) (SearchCodeOutput, error)
	ExplainMatch(context.Context, ExplainMatchInput) (ExplainMatchOutput, error)
	GetJob(context.Context, GetJobInput) (GetJobOutput, error)
	Investigation(context.Context, InvestigationInput) (InvestigationOutput, error)
	ListOpportunities(context.Context, ListOpportunitiesInput) (ListOpportunitiesOutput, error)
	Opportunity(context.Context, OpportunityInput) (OpportunityOutput, error)
	Evidence(context.Context, EvidenceInput) (EvidenceOutput, error)
	Readiness(context.Context, ReadinessInput) (ReadinessOutput, error)
	FindClusters(context.Context, FindClustersInput) (FindClustersOutput, error)
	GetCoverage(context.Context, GetCoverageInput) (GetCoverageOutput, error)
	Lens(context.Context, LensInput) (LensOutput, error)
}

// RepoInput identifies a repository for an MCP operation.
type RepoInput struct {
	Owner string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo  string `json:"repo" jsonschema:"GitHub repository name"`
}

// ThreadInput identifies an issue or pull request for an MCP operation.
type ThreadInput struct {
	Owner  string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo   string `json:"repo" jsonschema:"GitHub repository name"`
	Kind   string `json:"kind" jsonschema:"Thread kind: issue or pull_request"`
	Number int    `json:"number" jsonschema:"GitHub issue or pull request number"`
}

// ConcernInput identifies one persisted local concern.
type ConcernInput struct {
	ID string `json:"id" jsonschema:"Concern ID"`
}

// DraftInput identifies one immutable persisted contribution-draft revision.
type DraftInput struct {
	ID       string `json:"id" jsonschema:"Draft ID"`
	Revision int    `json:"revision" jsonschema:"Positive draft revision"`
}

// ManifestInput identifies one persisted contribution evidence manifest.
type ManifestInput struct {
	ID string `json:"id" jsonschema:"Manifest ID"`
}

// SearchInput describes an offline thread search page.
type SearchInput struct {
	Query          string   `json:"query" jsonschema:"Full-text query"`
	Owner          string   `json:"owner,omitempty" jsonschema:"Optional repository owner"`
	Repo           string   `json:"repo,omitempty" jsonschema:"Optional repository name"`
	Kind           string   `json:"kind,omitempty" jsonschema:"Optional thread kind"`
	State          string   `json:"state,omitempty"`
	StateReason    string   `json:"state_reason,omitempty"`
	Merged         *bool    `json:"merged,omitempty"`
	Author         string   `json:"author,omitempty"`
	Association    string   `json:"author_association,omitempty"`
	Assignee       string   `json:"assignee,omitempty"`
	Labels         []string `json:"labels,omitempty"`
	UpdatedAfter   string   `json:"updated_after,omitempty"`
	UpdatedBefore  string   `json:"updated_before,omitempty"`
	Limit          int      `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
	Cursor         string   `json:"cursor,omitempty" jsonschema:"Opaque cursor returned by the previous page"`
	Sort           string   `json:"sort,omitempty" jsonschema:"Order: relevance or updated"`
	MatchMode      string   `json:"match_mode,omitempty" jsonschema:"Term matching: all requires every term; any requires at least one term"`
	View           string   `json:"view,omitempty" jsonschema:"compact omits full bodies and keeps bounded excerpts; full includes stored bodies"`
	CorpusRevision *int64   `json:"corpus_revision,omitempty" jsonschema:"Optional corpus revision pin from a previous offline read"`
}

// RepositoryOutput is the stable MCP representation of a repository.
type RepositoryOutput = TypedRepositoryOutput

// ThreadOutput is the stable MCP representation of an issue or pull request.
type ThreadOutput struct {
	Owner             string   `json:"owner"`
	Repo              string   `json:"repo"`
	Kind              string   `json:"kind"`
	Number            int      `json:"number"`
	State             string   `json:"state"`
	StateReason       string   `json:"state_reason,omitempty"`
	Title             string   `json:"title"`
	Body              string   `json:"body,omitempty"`
	Author            string   `json:"author,omitempty"`
	AuthorAssociation string   `json:"author_association,omitempty"`
	Labels            []string `json:"labels,omitempty"`
	Assignees         []string `json:"assignees,omitempty"`
	Draft             bool     `json:"draft,omitempty"`
	ClosedAt          string   `json:"closed_at,omitempty"`
	MergedAt          string   `json:"merged_at,omitempty"`
	Merged            *bool    `json:"merged,omitempty"`
	UpdatedAt         string   `json:"updated_at,omitempty"`
	MatchSource       string   `json:"match_source,omitempty"`
	MatchExcerpt      string   `json:"match_excerpt,omitempty"`
	MatchTruncated    bool     `json:"match_truncated,omitempty" jsonschema:"Whether the per-thread hydrated search document was bounded"`
	MatchUpdatedAt    string   `json:"match_updated_at,omitempty"`
	CorpusRevision    int64    `json:"corpus_revision"`
}

// SearchOutput contains one page of offline thread matches.
type SearchOutput struct {
	Query               string               `json:"query"`
	QueryInterpretation string               `json:"query_interpretation"`
	MatchMode           string               `json:"match_mode"`
	View                string               `json:"view"`
	Matches             []ThreadOutput       `json:"matches"`
	Total               int                  `json:"total"`
	NextCursor          string               `json:"next_cursor,omitempty"`
	UnknownMergeCount   int                  `json:"unknown_merge_count,omitempty"`
	Suggestion          string               `json:"suggestion,omitempty"`
	CorpusRevision      int64                `json:"corpus_revision"`
	Provenance          CorpusReadProvenance `json:"provenance"`
}

// DossierOutput contains a persisted repository dossier snapshot.
type DossierOutput struct {
	Owner                string          `json:"owner"`
	Repo                 string          `json:"repo"`
	AsOf                 string          `json:"as_of,omitempty"`
	RecentItemsLimit     NonNegativeInt  `json:"recent_items_limit" jsonschema:"Maximum items retained in each recent-thread section"`
	RecentItemsTruncated bool            `json:"recent_items_truncated" jsonschema:"True when at least one recent-thread section is a bounded sample"`
	Sections             DossierSections `json:"sections"`
}

// DossierSections is the stable typed projection of persisted dossier data.
type DossierSections struct {
	Description                      string                `json:"description,omitempty"`
	Language                         string                `json:"language,omitempty"`
	Stars                            NonNegativeInt        `json:"stars"`
	OpenIssues                       NonNegativeInt        `json:"open_issues"`
	ClosedIssues                     NonNegativeInt        `json:"closed_issues"`
	OpenPullRequests                 NonNegativeInt        `json:"open_prs"`
	MergedPullRequests               NonNegativeInt        `json:"merged_prs"`
	ClosedUnmergedPullRequests       NonNegativeInt        `json:"closed_unmerged_prs"`
	ClosedUnknownMergePullRequests   NonNegativeInt        `json:"closed_unknown_merge_prs"`
	RecentMergedPullRequests         []DossierThreadOutput `json:"recent_merged_prs"`
	RecentOpenPullRequests           []DossierThreadOutput `json:"recent_open_prs"`
	RecentClosedUnmergedPullRequests []DossierThreadOutput `json:"recent_closed_unmerged_prs"`
	RecentClosedUnknownPullRequests  []DossierThreadOutput `json:"recent_closed_unknown_merge_prs"`
	RecentIssues                     []DossierThreadOutput `json:"recent_issues"`
	Guidance                         string                `json:"guidance,omitempty"`
	Coverage                         []string              `json:"coverage" jsonschema:"Observed dossier facets; omitted facets are unknown"`
}

// DossierThreadOutput is one bounded recent thread summary.
type DossierThreadOutput struct {
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	Author    string   `json:"author,omitempty"`
	State     string   `json:"state"`
	Draft     bool     `json:"draft,omitempty"`
	CreatedAt string   `json:"created_at,omitempty"`
	UpdatedAt string   `json:"updated_at,omitempty"`
	ClosedAt  string   `json:"closed_at,omitempty"`
	MergedAt  string   `json:"merged_at,omitempty"`
	Labels    []string `json:"labels,omitempty"`
}

// SourceRef records provenance for an MCP result or workflow artifact.
type SourceRef struct {
	Source     string `json:"source" jsonschema:"Source identifier"`
	URL        string `json:"url,omitempty" jsonschema:"Source URL"`
	CommitSHA  string `json:"commit_sha,omitempty" jsonschema:"Source commit SHA"`
	ObservedAt string `json:"observed_at,omitempty" jsonschema:"Observation timestamp"`
	AsOf       string `json:"as_of,omitempty" jsonschema:"As-of timestamp"`
}

// CorpusReadProvenance binds an offline result to the exact query and source
// observation it used. Transaction-bound identities are deliberately marked
// non-durable; callers that need cross-call reuse must request a persisted
// snapshot rather than treating a mutable projection as historical evidence.
type CorpusReadProvenance struct {
	SnapshotToken        string      `json:"snapshot_token"`
	Durable              bool        `json:"durable"`
	ObservationWatermark int64       `json:"observation_watermark"`
	QueryDigestSHA256    string      `json:"query_digest_sha256"`
	Complete             bool        `json:"complete"`
	Truncated            bool        `json:"truncated"`
	UnknownCoverage      bool        `json:"unknown_coverage"`
	Limitations          []string    `json:"limitations,omitempty"`
	ExternalContext      []SourceRef `json:"external_context,omitempty"`
}

// SearchCodeInput describes an offline code search page.
type SearchCodeInput struct {
	Query          string `json:"query" jsonschema:"Code search query"`
	Owner          string `json:"owner,omitempty" jsonschema:"Optional repository owner"`
	Repo           string `json:"repo,omitempty" jsonschema:"Optional repository name"`
	Limit          int    `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
	Cursor         string `json:"cursor,omitempty" jsonschema:"Opaque cursor returned by the previous page"`
	CorpusRevision *int64 `json:"corpus_revision,omitempty" jsonschema:"Optional corpus revision pin from a previous offline read"`
}

// CodeMatchOutput identifies one stored code match.
type CodeMatchOutput struct {
	ID       string `json:"id"`
	Repo     string `json:"repo"`
	Commit   string `json:"commit"`
	Path     string `json:"path"`
	Language string `json:"language,omitempty"`
	Snippet  string `json:"snippet"`
	Bytes    int    `json:"bytes"`
}

// CodeIndexCoverageOutput reports one selected snapshot's indexing coverage.
type CodeIndexCoverageOutput struct {
	Repo           string `json:"repo"`
	Status         string `json:"status" jsonschema:"Index coverage state"`
	Commit         string `json:"commit"`
	Truncated      bool   `json:"truncated" jsonschema:"Whether index limits omitted files"`
	IndexedFiles   int    `json:"indexed_files" jsonschema:"Files indexed in this snapshot"`
	TrackedEntries int    `json:"tracked_entries" jsonschema:"Tracked tree entries considered"`
	SkippedFiles   int    `json:"skipped_files" jsonschema:"Entries omitted by policy or limits"`
	SkippedPolicy  int    `json:"skipped_policy" jsonschema:"Invalid, excluded, or non-regular entries"`
	SkippedLimits  int    `json:"skipped_limits" jsonschema:"Entries omitted by file-size, total-size, or file-count bounds"`
	SkippedNonText int    `json:"skipped_non_text" jsonschema:"Entries omitted because content was binary or invalid UTF-8"`
}

// SearchCodeOutput contains one page of offline code matches.
type SearchCodeOutput struct {
	Query          string                    `json:"query"`
	Total          int                       `json:"total"`
	Matches        []CodeMatchOutput         `json:"matches"`
	Coverage       []CodeIndexCoverageOutput `json:"coverage,omitempty"`
	NextCursor     string                    `json:"next_cursor,omitempty"`
	CorpusRevision int64                     `json:"corpus_revision"`
	Provenance     CorpusReadProvenance      `json:"provenance"`
}

// InvestigationInput selects an investigation and bounds nested hypotheses.
type InvestigationInput struct {
	ID              string `json:"id" jsonschema:"Investigation ID"`
	HypothesisLimit int    `json:"hypothesis_limit,omitempty" jsonschema:"Maximum hypotheses from 1 to 100"`
}

// HypothesisSummary is the compact hypothesis representation nested in an investigation.
type HypothesisSummary struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Category    string `json:"category"`
	Status      string `json:"status"`
	Description string `json:"description,omitempty"`
}

// InvestigationOutput is the stable MCP representation of an investigation.
type InvestigationOutput struct {
	ID              string              `json:"id"`
	Owner           string              `json:"owner"`
	Repo            string              `json:"repo"`
	CommitSHA       string              `json:"commit_sha,omitempty"`
	Lens            string              `json:"lens,omitempty"`
	Status          string              `json:"status"`
	CreatedAt       string              `json:"created_at"`
	UpdatedAt       string              `json:"updated_at"`
	HypothesisTotal int                 `json:"hypothesis_total"`
	Hypotheses      []HypothesisSummary `json:"hypotheses,omitempty"`
}

// ListOpportunitiesInput selects and bounds opportunities for an investigation.
type ListOpportunitiesInput struct {
	InvestigationID string `json:"investigation_id" jsonschema:"Investigation ID"`
	Limit           int    `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
}

// OpportunitySummary is the compact opportunity representation used in lists.
type OpportunitySummary struct {
	ID              string      `json:"id"`
	InvestigationID string      `json:"investigation_id"`
	Title           string      `json:"title"`
	Category        string      `json:"category"`
	Status          string      `json:"status"`
	Confidence      Probability `json:"confidence"`
	CollisionStatus string      `json:"collision_status"`
	CreatedAt       string      `json:"created_at"`
	UpdatedAt       string      `json:"updated_at"`
}

// ListOpportunitiesOutput contains bounded opportunities for an investigation.
type ListOpportunitiesOutput struct {
	Opportunities []OpportunitySummary `json:"opportunities"`
	Total         int                  `json:"total"`
}

// OpportunityInput selects an opportunity and bounds nested evidence.
type OpportunityInput struct {
	ID            string `json:"id" jsonschema:"Opportunity ID"`
	EvidenceLimit int    `json:"evidence_limit,omitempty" jsonschema:"Maximum evidence IDs from 1 to 100"`
}

// OpportunityOutput is the stable MCP representation of a contribution opportunity.
type OpportunityOutput struct {
	ID                  string      `json:"id"`
	InvestigationID     string      `json:"investigation_id"`
	HypothesisID        string      `json:"hypothesis_id,omitempty"`
	Title               string      `json:"title"`
	ProblemStatement    string      `json:"problem_statement"`
	Category            string      `json:"category"`
	Scope               string      `json:"scope"`
	Impact              string      `json:"impact"`
	Confidence          Probability `json:"confidence"`
	ExpectedEffort      string      `json:"expected_effort,omitempty"`
	Dependencies        []string    `json:"dependencies,omitempty"`
	CollisionStatus     string      `json:"collision_status"`
	MaintainerAlignment string      `json:"maintainer_alignment,omitempty"`
	SourceRefs          []SourceRef `json:"source_refs,omitempty"`
	EvidenceTotal       int         `json:"evidence_total"`
	EvidenceIDs         []string    `json:"evidence_ids,omitempty"`
	Status              string      `json:"status"`
	CreatedAt           string      `json:"created_at"`
	UpdatedAt           string      `json:"updated_at"`
}

// ClusterTarget selects one repository or one exact cluster member.
type ClusterTarget struct {
	Owner  string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo   string `json:"repo" jsonschema:"GitHub repository name"`
	Kind   string `json:"kind,omitempty" jsonschema:"Optional member kind: issue or pull_request"`
	Number int    `json:"number,omitempty" jsonschema:"Optional positive member number"`
}

// FindClustersInput selects up to 20 repositories or exact cluster members.
type FindClustersInput struct {
	Targets        []ClusterTarget `json:"targets" jsonschema:"One to 20 repository or exact-member targets"`
	Limit          int             `json:"limit,omitempty" jsonschema:"Maximum clusters per target from 1 to 100"`
	CorpusRevision *int64          `json:"corpus_revision,omitempty" jsonschema:"Optional corpus revision pin from a previous offline read"`
}

// FindNeighborsInput selects source threads and bounds similar-thread results.
type FindNeighborsInput struct {
	Threads        []ThreadRef `json:"threads" jsonschema:"One to 20 exact source threads"`
	Limit          int         `json:"limit,omitempty" jsonschema:"Maximum neighbors per source thread from 1 to 100"`
	CorpusRevision *int64      `json:"corpus_revision,omitempty" jsonschema:"Optional corpus revision pin from a previous offline read"`
}

// NeighborOutput describes one similar stored thread and its score.
type NeighborOutput struct {
	Kind   string          `json:"kind"`
	Owner  string          `json:"owner"`
	Repo   string          `json:"repo"`
	Number int             `json:"number"`
	Title  string          `json:"title"`
	State  string          `json:"state"`
	Score  SimilarityScore `json:"score"`
	Reason string          `json:"reason"`
}

// NeighborSetOutput contains deterministic neighbors for one stored thread.
type NeighborSetOutput struct {
	Owner          string           `json:"owner"`
	Repo           string           `json:"repo"`
	Kind           string           `json:"kind"`
	Number         int              `json:"number"`
	SourceRevision string           `json:"source_revision"`
	Neighbors      []NeighborOutput `json:"neighbors"`
}

// FindNeighborsOutput preserves source-thread order and isolates item failures.
type FindNeighborsOutput struct {
	Status         string                         `json:"status"`
	Items          []BatchItem[NeighborSetOutput] `json:"items"`
	CorpusRevision int64                          `json:"corpus_revision"`
}

// ClusterMemberOutput describes one member of a duplicate cluster.
type ClusterMemberOutput struct {
	Kind     string          `json:"kind"`
	Owner    string          `json:"owner"`
	Repo     string          `json:"repo"`
	Number   int             `json:"number"`
	Title    string          `json:"title,omitempty"`
	State    string          `json:"state,omitempty"`
	Score    SimilarityScore `json:"score"`
	Reason   string          `json:"reason"`
	Included bool            `json:"included"`
}

// ClusterOutput contains a stable duplicate cluster and its canonical member.
type ClusterOutput struct {
	StableID    string                `json:"stable_id"`
	State       string                `json:"state"`
	Canonical   ClusterMemberOutput   `json:"canonical"`
	MemberCount int                   `json:"member_count"`
	Members     []ClusterMemberOutput `json:"members,omitempty"`
}

// ClusterSetOutput contains duplicate clusters for one repository target.
type ClusterSetOutput struct {
	Owner       string                 `json:"owner"`
	Repo        string                 `json:"repo"`
	RuleVersion similarity.RuleVersion `json:"rule_version,omitempty"`
	Total       int                    `json:"total"`
	Clusters    []ClusterOutput        `json:"clusters"`
	Truncated   bool                   `json:"truncated" jsonschema:"Whether more clusters matched"`
}

// FindClustersOutput preserves target order and isolates item failures.
type FindClustersOutput struct {
	Status         string                        `json:"status"`
	Items          []BatchItem[ClusterSetOutput] `json:"items"`
	CorpusRevision int64                         `json:"corpus_revision"`
}

type CoverageTargetKind string

const (
	CoverageTargetRepository  CoverageTargetKind = "repository"
	CoverageTargetExactThread CoverageTargetKind = "exact_thread"
)

type ExactCoverageThread struct {
	Kind   string `json:"kind" jsonschema:"Thread kind: issue or pull_request"`
	Number int    `json:"number" jsonschema:"Positive issue or pull request number"`
}

// CoverageTarget is an explicit discriminated target. Thread is required only
// for exact_thread and forbidden for repository.
type CoverageTarget struct {
	Type       CoverageTargetKind   `json:"type" jsonschema:"Target variant: repository or exact_thread"`
	Repository RepositoryRef        `json:"repository"`
	Thread     *ExactCoverageThread `json:"thread,omitempty"`
}

// GetCoverageInput selects bounded repository or thread facet coverage reads.
type GetCoverageInput struct {
	Targets        []CoverageTarget `json:"targets" jsonschema:"One to 100 repository or exact-thread targets"`
	CorpusRevision *int64           `json:"corpus_revision,omitempty" jsonschema:"Optional corpus revision pin from a previous offline read"`
}

// FacetCoverageOutput reports completeness and freshness for one facet.
type FacetCoverageOutput struct {
	Facet     string `json:"facet"`
	Complete  bool   `json:"complete"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at"`
}

// CoverageOutput reports all known coverage for one repository or thread.
type CoverageOutput struct {
	Owner  string                `json:"owner"`
	Repo   string                `json:"repo"`
	Kind   string                `json:"kind,omitempty"`
	Number int                   `json:"number,omitempty"`
	AsOf   string                `json:"as_of"`
	Facets []FacetCoverageOutput `json:"facets"`
}

// GetCoverageOutput preserves target order and isolates missing or invalid
// targets without failing unrelated coverage reads.
type GetCoverageOutput struct {
	Status         string                      `json:"status"`
	Items          []BatchItem[CoverageOutput] `json:"items"`
	CorpusRevision int64                       `json:"corpus_revision"`
	Provenance     CorpusReadProvenance        `json:"provenance"`
}

// LensInput selects a saved lens by name.
type LensInput struct {
	Name string `json:"name" jsonschema:"Lens name"`
}

// LensOutput contains a saved lens definition and timestamps.
type LensOutput struct {
	Name       string          `json:"name"`
	Definition lens.Definition `json:"definition"`
	CreatedAt  string          `json:"created_at"`
	UpdatedAt  string          `json:"updated_at"`
}
