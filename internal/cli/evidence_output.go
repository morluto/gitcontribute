package cli

import (
	"fmt"
	"strings"
)

func evidenceHuman(r *EvidenceResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Evidence for investigation %s:\n", r.InvestigationID)
	if len(r.Evidence) == 0 {
		b.WriteString("No evidence recorded.")
		return b.String()
	}
	for _, item := range r.Evidence {
		fmt.Fprintf(&b, "- %s [%s / %s; freshness: %s] %s", item.ID, item.Type, item.Relation, item.Freshness, item.Description)
		if item.ValidationRunID != "" {
			fmt.Fprintf(&b, " [run: %s]", item.ValidationRunID)
		}
		if item.OpportunityID != "" {
			fmt.Fprintf(&b, " [opportunity: %s]", item.OpportunityID)
		}
		if item.FreshnessReason != "" {
			fmt.Fprintf(&b, " [freshness reason: %s]", item.FreshnessReason)
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}
