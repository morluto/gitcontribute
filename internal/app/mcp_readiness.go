package app

import (
	"context"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// Readiness reads a local contribution readiness report through MCP.
func (r *MCPReader) Readiness(ctx context.Context, in mcpcontract.ReadinessInput) (mcpcontract.ReadinessOutput, error) {
	report, err := r.OpportunityReadiness(ctx, in.OpportunityID)
	if err != nil {
		return mcpcontract.ReadinessOutput{}, err
	}
	return readinessToMCP(report), nil
}

func readinessToMCP(report *contracts.ReadinessResult) mcpcontract.ReadinessOutput {
	if report == nil {
		return mcpcontract.ReadinessOutput{}
	}
	checks := make([]mcpcontract.ReadinessCheck, len(report.Checks))
	for i, check := range report.Checks {
		checks[i] = mcpcontract.ReadinessCheck{
			CheckID:      check.CheckID,
			RuleID:       check.RuleID,
			RuleVersion:  check.RuleVersion,
			Status:       check.Status,
			Summary:      check.Summary,
			EvidenceRefs: append([]string(nil), check.EvidenceRefs...),
			Remediation:  check.Remediation,
			EvaluatedAt:  check.EvaluatedAt,
		}
	}
	return mcpcontract.ReadinessOutput{
		OpportunityID:  report.OpportunityID,
		RuleSetVersion: report.RuleSetVersion,
		Status:         report.Status,
		EvaluatedAt:    report.EvaluatedAt,
		Checks:         checks,
	}
}
