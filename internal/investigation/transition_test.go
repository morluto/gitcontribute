package investigation

import (
	"testing"
)

func TestValidOpportunityTransitions(t *testing.T) {
	cases := []struct {
		from   OpportunityStatus
		to     OpportunityStatus
		wantOK bool
	}{
		{OpportunityHypothesis, OpportunityReproduced, true},
		{OpportunityHypothesis, OpportunityValidated, false},
		{OpportunityReproduced, OpportunityValidated, true},
		{OpportunityValidated, OpportunityMaintainerAligned, true},
		{OpportunityMaintainerAligned, OpportunityImplemented, true},
		{OpportunityImplemented, OpportunitySubmitted, true},
		{OpportunitySubmitted, OpportunityMerged, true},
		{OpportunityRejected, OpportunityValidated, false},
		{OpportunityMerged, OpportunitySubmitted, false},
	}

	for _, tc := range cases {
		t.Run(string(tc.from)+"_"+string(tc.to), func(t *testing.T) {
			got := ValidOpportunityTransition(tc.from, tc.to)
			if got != tc.wantOK {
				t.Fatalf("ValidOpportunityTransition(%q, %q) = %v, want %v", tc.from, tc.to, got, tc.wantOK)
			}
		})
	}
}

func TestOpportunityTransition(t *testing.T) {
	o := &Opportunity{Status: OpportunityHypothesis}
	if err := o.Transition(OpportunityReproduced, "base fails"); err != nil {
		t.Fatalf("transition: %v", err)
	}
	if len(o.AuditTrail) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(o.AuditTrail))
	}
	if err := o.Transition(OpportunityMerged, "jump"); err == nil {
		t.Fatal("expected transition error")
	}
}

func TestValidHypothesisTransitions(t *testing.T) {
	if !ValidHypothesisTransition(HypothesisProposed, HypothesisPromoted) {
		t.Fatal("proposed -> promoted should be allowed")
	}
	if ValidHypothesisTransition(HypothesisRejected, HypothesisPromoted) {
		t.Fatal("rejected -> promoted should be disallowed")
	}
}
