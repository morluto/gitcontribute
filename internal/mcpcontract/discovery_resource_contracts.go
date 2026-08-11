package mcpcontract

import (
	"github.com/morluto/gitcontribute/internal/lens"
	"github.com/morluto/gitcontribute/internal/similarity"
)

// ClusterTarget selects one repository or one exact cluster member.
type ClusterTarget struct {
	Owner  string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo   string `json:"repo" jsonschema:"GitHub repository name"`
	Kind   string `json:"kind,omitempty" jsonschema:"Optional member kind: issue or pull_request"`
	Number int    `json:"number,omitempty" jsonschema:"Optional positive member number"`
}

// FindClustersInput selects up to 20 repositories or exact cluster members.
type FindClustersInput struct {
	Targets       []ClusterTarget `json:"targets" jsonschema:"One to 20 repository or exact-member targets"`
	Limit         int             `json:"limit,omitempty" jsonschema:"Maximum clusters per target from 1 to 100"`
	SnapshotToken string          `json:"snapshot_token,omitempty" jsonschema:"Optional immutable corpus snapshot token from a previous offline read"`
}

// FindNeighborsInput selects source threads and bounds similar-thread results.
type FindNeighborsInput struct {
	Threads       []ThreadRef `json:"threads" jsonschema:"One to 20 exact source threads"`
	Limit         int         `json:"limit,omitempty" jsonschema:"Maximum neighbors per source thread from 1 to 100"`
	SnapshotToken string      `json:"snapshot_token,omitempty" jsonschema:"Optional immutable corpus snapshot token from a previous offline read"`
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
	Status        string                         `json:"status"`
	Items         []BatchItem[NeighborSetOutput] `json:"items"`
	SnapshotToken string                         `json:"snapshot_token"`
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
	Recovery    *RecoveryPlan          `json:"recovery,omitempty"`
}

// FindClustersOutput preserves target order and isolates item failures.
type FindClustersOutput struct {
	Status        string                        `json:"status"`
	Items         []BatchItem[ClusterSetOutput] `json:"items"`
	SnapshotToken string                        `json:"snapshot_token"`
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
	Targets       []CoverageTarget `json:"targets" jsonschema:"One to 100 repository or exact-thread targets"`
	SnapshotToken string           `json:"snapshot_token,omitempty" jsonschema:"Optional immutable corpus snapshot token from a previous offline read"`
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
	Status        string                      `json:"status"`
	Items         []BatchItem[CoverageOutput] `json:"items"`
	SnapshotToken string                      `json:"snapshot_token"`
	Provenance    CorpusReadProvenance        `json:"provenance"`
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
