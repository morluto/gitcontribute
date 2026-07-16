package investigation

import (
	"fmt"
	"time"
)

// opportunityTransitions defines the allowed OpportunityStatus transitions.
var opportunityTransitions = map[OpportunityStatus][]OpportunityStatus{
	OpportunityHypothesis:        {OpportunityReproduced, OpportunityRejected, OpportunityDeferred, OpportunitySuperseded},
	OpportunityReproduced:        {OpportunityValidated, OpportunityRejected, OpportunityDeferred, OpportunitySuperseded},
	OpportunityValidated:         {OpportunityMaintainerAligned, OpportunityRejected, OpportunityDeferred, OpportunitySuperseded},
	OpportunityMaintainerAligned: {OpportunityImplemented, OpportunityRejected, OpportunityDeferred, OpportunitySuperseded},
	OpportunityImplemented:       {OpportunitySubmitted, OpportunityRejected, OpportunityDeferred, OpportunitySuperseded},
	OpportunitySubmitted:         {OpportunityMerged, OpportunityRejected, OpportunityDeferred, OpportunitySuperseded},
}

// hypothesisTransitions defines the allowed HypothesisStatus transitions.
var hypothesisTransitions = map[HypothesisStatus][]HypothesisStatus{
	HypothesisProposed:   {HypothesisPromoted, HypothesisRejected, HypothesisDeferred, HypothesisSuperseded},
	HypothesisPromoted:   {HypothesisRejected, HypothesisDeferred, HypothesisSuperseded},
	HypothesisRejected:   {},
	HypothesisDeferred:   {},
	HypothesisSuperseded: {},
}

// ValidOpportunityTransition reports whether moving from -> to is allowed.
func ValidOpportunityTransition(from, to OpportunityStatus) bool {
	if from == to {
		return true
	}
	for _, allowed := range opportunityTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// ValidHypothesisTransition reports whether moving from -> to is allowed.
func ValidHypothesisTransition(from, to HypothesisStatus) bool {
	if from == to {
		return true
	}
	for _, allowed := range hypothesisTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// Transition advances the opportunity status if allowed and records a status change.
func (o *Opportunity) Transition(to OpportunityStatus, rationale string) error {
	if !ValidOpportunityTransition(o.Status, to) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, o.Status, to)
	}
	o.AuditTrail = append(o.AuditTrail, StatusChange{
		From:      string(o.Status),
		To:        string(to),
		Rationale: rationale,
		At:        time.Now().UTC(),
	})
	o.Status = to
	o.UpdatedAt = time.Now().UTC()
	return nil
}

// Transition advances the hypothesis status if allowed and records a status change.
func (h *Hypothesis) Transition(to HypothesisStatus, rationale string) error {
	if !ValidHypothesisTransition(h.Status, to) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, h.Status, to)
	}
	h.AuditTrail = append(h.AuditTrail, StatusChange{
		From:      string(h.Status),
		To:        string(to),
		Rationale: rationale,
		At:        time.Now().UTC(),
	})
	h.Status = to
	h.UpdatedAt = time.Now().UTC()
	return nil
}
