package cli

import "context"

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
	Clusters   []ClusterResult            `json:"clusters"`
}
