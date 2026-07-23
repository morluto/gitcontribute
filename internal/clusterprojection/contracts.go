// Package clusterprojection owns the dependency-neutral contracts for the
// durable duplicate-cluster projection.
package clusterprojection

import (
	"fmt"

	"github.com/morluto/gitcontribute/internal/clustering"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/similarity"
)

// Identity names the complete input and rule set that produced a projection.
type Identity struct {
	SourceRevision     string
	GovernanceRevision uint64
	RuleVersion        similarity.RuleVersion
	RunID              int64
}

// List is a bounded stored projection read. A nil identity means the
// repository has never completed a refresh.
type List struct {
	Repo       domain.RepoRef
	Projection *Identity
	Clusters   []clustering.Cluster
	Total      int
	Truncated  bool
}

// Matches reports whether the source, governance, and rule inputs are equal.
func (i Identity) Matches(source string, governance uint64, rule similarity.RuleVersion) bool {
	return i.SourceRevision == source && i.GovernanceRevision == governance && i.RuleVersion == rule
}

// RefreshSnapshot is every input needed for one refresh, read from a single
// repository snapshot. Callers must close the storage transaction before exact
// pair evaluation and pass the revisions back unchanged at commit time.
type RefreshSnapshot struct {
	Repo               domain.RepoRef
	Candidates         []clustering.Candidate
	ExistingClusters   []clustering.Cluster
	OverridesByCluster map[string][]clustering.MembershipOverride
	SourceRevision     string
	GovernanceRevision uint64
	CurrentProjection  *Identity
	ReadStatements     int
}

// RefreshStats records output cardinalities and work counts for one successful
// refresh. ClusterCount is the current non-retired projection count even when
// an unchanged refresh skips pair evaluation and commit work.
type RefreshStats struct {
	CandidateCount  int    `json:"candidate_count"`
	RequiredPairs   uint64 `json:"required_pairs"`
	ComparedPairs   uint64 `json:"compared_pairs"`
	ClusterCount    int    `json:"cluster_count"`
	SnapshotQueries int    `json:"snapshot_queries"`
	CommitQueries   int    `json:"commit_queries"`
}

// Commit contains a fully reconciled projection plus the exact source,
// governance, and rule inputs used to compute it. Corpus persistence rejects
// incomplete identities and rechecks source and governance before writing.
type Commit struct {
	Repo               domain.RepoRef
	ExpectedSource     string
	ExpectedGovernance uint64
	RuleVersion        similarity.RuleVersion
	Clusters           []clustering.Cluster
	Stats              RefreshStats
	MaxCandidates      int
}

// CommitDisposition describes whether this caller advanced the projection.
type CommitDisposition string

const (
	// Committed means this caller atomically advanced the current projection.
	Committed CommitDisposition = "committed"
	// AlreadyCurrent means an equivalent concurrent or earlier refresh won.
	AlreadyCurrent CommitDisposition = "already_current"
)

// CommitResult is the atomic persistence result.
type CommitResult struct {
	Disposition     CommitDisposition
	Projection      Identity
	WriteStatements int
}

// StaleInputError reports changed source or governance inputs at commit time.
type StaleInputError struct {
	ExpectedSource        string
	ActualSource          string
	ExpectedGovernance    uint64
	ActualGovernance      uint64
	CurrentCandidateCount int
}

// Error describes the source or governance revision that changed.
func (e *StaleInputError) Error() string {
	return fmt.Sprintf("cluster refresh inputs changed: source %q -> %q, governance %d -> %d", e.ExpectedSource, e.ActualSource, e.ExpectedGovernance, e.ActualGovernance)
}
