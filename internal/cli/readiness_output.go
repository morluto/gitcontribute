package cli

import (
	"fmt"
	"strings"
)

func readinessHuman(r *ReadinessResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Readiness for opportunity %s: %s (%s)\n", r.OpportunityID, r.Status, r.RuleSetVersion)
	for _, check := range r.Checks {
		fmt.Fprintf(&b, "- %s [%s] %s", check.RuleID, check.Status, check.Summary)
		if check.Remediation != "" {
			fmt.Fprintf(&b, " [remediation: %s]", check.Remediation)
		}
		if len(check.EvidenceRefs) > 0 {
			fmt.Fprintf(&b, " [evidence: %s]", strings.Join(check.EvidenceRefs, ", "))
		}
		fmt.Fprintf(&b, " [id: %s]\n", check.CheckID)
	}
	return strings.TrimSpace(b.String())
}

func readinessCheckHuman(check *ReadinessCheck) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s\n", check.CheckID, check.Status)
	fmt.Fprintf(&b, "Rule: %s (%s)\n", check.RuleID, check.RuleVersion)
	fmt.Fprintf(&b, "Summary: %s\n", check.Summary)
	if check.Remediation != "" {
		fmt.Fprintf(&b, "Remediation: %s\n", check.Remediation)
	}
	if len(check.EvidenceRefs) > 0 {
		fmt.Fprintf(&b, "Evidence: %s\n", strings.Join(check.EvidenceRefs, ", "))
	}
	fmt.Fprintf(&b, "Evaluated: %s", check.EvaluatedAt)
	return b.String()
}
