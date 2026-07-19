package clustering

import (
	"context"
	"errors"

	"github.com/morluto/gitcontribute/internal/similarity"
)

const defaultDuplicateThreshold = 0.30

// Engine performs bounded, cancellable exact duplicate clustering.
type Engine struct {
	rule   similarity.DuplicateRule
	budget ExactPairBudget
}

// Computation describes one complete exact clustering result.
type Computation struct {
	Clusters       []Cluster
	CandidateCount int
	RequiredPairs  uint64
	ComparedPairs  uint64
	RuleVersion    similarity.RuleVersion
}

// NewEngine constructs an exact clustering engine from a valid rule and nonzero budget.
func NewEngine(rule similarity.DuplicateRule, budget ExactPairBudget) (Engine, error) {
	if !rule.Valid() {
		return Engine{}, errors.New("duplicate rule is required")
	}
	if budget == 0 {
		return Engine{}, errors.New("exact pair budget must be positive")
	}
	return Engine{rule: rule, budget: budget}, nil
}

// MaxCandidates returns the population bound derived from the exact pair budget.
func (e Engine) MaxCandidates() int { return e.budget.MaxCandidates() }

// RuleVersion identifies the exact scoring rule used by the engine.
func (e Engine) RuleVersion() similarity.RuleVersion { return e.rule.Version() }

// Cluster computes exact duplicate clusters without storage side effects.
func (e Engine) Cluster(ctx context.Context, candidates []Candidate) (Computation, error) {
	return computeClusters(ctx, candidates, nil, e.rule, e.budget, defaultDuplicateThreshold)
}
