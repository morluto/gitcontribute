package app

import "github.com/morluto/gitcontribute/internal/similarity"

const duplicateRuleVersion = similarity.DuplicateV1

// Neighbor is one ranked thread near a query.
type Neighbor struct {
	Kind   string  `json:"kind"`
	Owner  string  `json:"owner"`
	Repo   string  `json:"repo"`
	Number int     `json:"number"`
	Title  string  `json:"title"`
	State  string  `json:"state"`
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}

// NeighborsResult is the response for a local nearest-thread query.
type NeighborsResult struct {
	Repo           string                 `json:"repo"`
	Kind           string                 `json:"kind"`
	Number         int                    `json:"number"`
	Limit          int                    `json:"limit"`
	Total          int                    `json:"total"`
	SourceRevision string                 `json:"source_revision"`
	RuleVersion    similarity.RuleVersion `json:"rule_version"`
	Neighbors      []Neighbor             `json:"neighbors"`
}

// DuplicateCandidatesResult is the response for a duplicate-candidate query.
type DuplicateCandidatesResult struct {
	Repo           string     `json:"repo"`
	Kind           string     `json:"kind"`
	Number         int        `json:"number"`
	ClusterID      int64      `json:"cluster_id,omitempty"`
	StableID       string     `json:"stable_id,omitempty"`
	Canonical      ThreadRef  `json:"canonical,omitempty"`
	SourceRevision string     `json:"source_revision"`
	Limit          int        `json:"limit"`
	Total          int        `json:"total"`
	Candidates     []Neighbor `json:"candidates"`
}

// ThreadRef identifies a thread with minimal fields.
type ThreadRef struct {
	Kind   string `json:"kind"`
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Number int    `json:"number"`
}
