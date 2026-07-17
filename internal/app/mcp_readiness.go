package app

import (
	"context"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/mcpserver"
)

// Readiness reads a local contribution readiness report through MCP.
func (r *MCPReader) Readiness(ctx context.Context, in mcpserver.ReadinessInput) (mcpserver.ReadinessOutput, error) {
	report, err := r.OpportunityReadiness(ctx, in.OpportunityID)
	if err != nil {
		return mcpserver.ReadinessOutput{}, err
	}
	return readinessToMCP(report), nil
}

func readinessToMCP(report *cli.ReadinessResult) mcpserver.ReadinessOutput {
	if report == nil {
		return mcpserver.ReadinessOutput{}
	}
	checks := make([]mcpserver.ReadinessCheck, len(report.Checks))
	for i, check := range report.Checks {
		checks[i] = mcpserver.ReadinessCheck{
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
	return mcpserver.ReadinessOutput{
		OpportunityID:  report.OpportunityID,
		RuleSetVersion: report.RuleSetVersion,
		Status:         report.Status,
		EvaluatedAt:    report.EvaluatedAt,
		Checks:         checks,
	}
}
