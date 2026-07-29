package app

import (
	"testing"

	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func TestIssueContributionDisposition(t *testing.T) {
	merged, unmerged := true, false
	complete := []mcpcontract.FacetCoverageOutput{
		{Facet: FacetIssueComments, Complete: true},
		{Facet: FacetIssueTimeline, Complete: true},
	}
	cases := []struct {
		name string
		in   mcpcontract.PreparedIssueEvidence
		want string
	}{
		{"merged closing work", mcpcontract.PreparedIssueEvidence{Number: 1, State: "open", RelatedWorkTotalKnown: true, Coverage: complete, RelatedWork: []mcpcontract.IssueSetRelatedWork{{Ref: "pr#2", Kind: "pull_request", State: "closed", Relation: "claims_to_close", Direction: "inbound", Merged: &merged}}}, "already_resolved_upstream"},
		{"active closing work", mcpcontract.PreparedIssueEvidence{Number: 1, State: "open", RelatedWorkTotalKnown: true, Coverage: complete, RelatedWork: []mcpcontract.IssueSetRelatedWork{{Ref: "pr#2", Kind: "pull_request", State: "open", Relation: "claims_to_close", Direction: "inbound", Merged: &unmerged}}}, "active_competing_work"},
		{"closed unmerged", mcpcontract.PreparedIssueEvidence{Number: 1, State: "open", RelatedWorkTotalKnown: true, Coverage: complete, RelatedWork: []mcpcontract.IssueSetRelatedWork{{Ref: "pr#2", Kind: "pull_request", State: "closed", Relation: "claims_to_close", Direction: "inbound", Merged: &unmerged}}}, "needs_maintainer_alignment"},
		{"not planned", mcpcontract.PreparedIssueEvidence{Number: 1, State: "closed", StateReason: "not_planned"}, "blocked_by_repository_policy"},
		{"complete no work", mcpcontract.PreparedIssueEvidence{Number: 1, State: "open", RelatedWorkTotalKnown: true, Coverage: complete}, "ready_to_investigate"},
		{"incomplete", mcpcontract.PreparedIssueEvidence{Number: 1, State: "open"}, "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := issueContributionDisposition(tc.in)
			if got.Status != tc.want {
				t.Fatalf("status = %q, want %q (%+v)", got.Status, tc.want, got)
			}
			if got.Status != "unknown" && len(got.EvidenceRefs) == 0 {
				t.Fatal("non-unknown disposition lacks evidence refs")
			}
		})
	}
}
