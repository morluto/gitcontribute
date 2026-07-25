package clustering

import (
	"context"
	"errors"

	"github.com/morluto/gitcontribute/internal/similarity"
)

// Engine performs bounded, cancellable duplicate clustering with lossless
// candidate pruning and exact scoring.
type Engine struct {
	rule   similarity.DuplicateRule
	budget ComparisonBudget
}

// Computation describes one complete lossless clustering result.
type Computation struct {
	Clusters       []Cluster
	CandidateCount int
	PossiblePairs  uint64
	ScoredPairs    uint64
	RuleVersion    similarity.RuleVersion
}

// NewEngine constructs a clustering engine from a valid rule and nonzero budget.
func NewEngine(rule similarity.DuplicateRule, budget ComparisonBudget) (Engine, error) {
	if !rule.Valid() {
		return Engine{}, errors.New("duplicate rule is required")
	}
	if budget == 0 {
		return Engine{}, errors.New("comparison budget must be positive")
	}
	return Engine{rule: rule, budget: budget}, nil
}

// MaxCandidates returns the population bound derived from worst-case scoring.
func (e Engine) MaxCandidates() int { return e.budget.MaxCandidates() }

// RuleVersion identifies the exact scoring rule used by the engine.
func (e Engine) RuleVersion() similarity.RuleVersion { return e.rule.Version() }

// Cluster computes duplicate clusters without storage side effects. Candidate
// pruning is lossless for the configured scoring rule and threshold.
func (e Engine) Cluster(ctx context.Context, candidates []Candidate) (Computation, error) {
	return computeClusters(ctx, candidates, e.rule, e.budget)
}
