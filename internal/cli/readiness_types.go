package cli

import "context"

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
