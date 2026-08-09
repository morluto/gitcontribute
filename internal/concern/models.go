// Package concern models low-confidence local findings before they become
// contribution hypotheses or public issues.
package concern

import (
	"errors"
	"fmt"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
)

// Status is the lifecycle of a local concern.
type Status string

const (
	// StatusUntriaged is newly recorded intake.
	StatusUntriaged Status = "untriaged"
	// StatusAccepted marks a concern as worth investigating.
	StatusAccepted Status = "accepted"
	// StatusInvestigating marks active evidence gathering.
	StatusInvestigating Status = "investigating"
	// StatusDeferred preserves a concern without active work.
	StatusDeferred Status = "deferred"
	// StatusPromoted marks an atomically created downstream workflow.
	StatusPromoted Status = "promoted"
	// StatusResolved closes a concern without active promotion.
	StatusResolved Status = "resolved"
)

// LinkKind describes an explicit, non-inferred relationship.
type LinkKind string

const (
	// LinkRelated is a non-causal association.
	LinkRelated LinkKind = "related"
	// LinkDuplicateCandidate records possible duplicate work.
	LinkDuplicateCandidate LinkKind = "duplicate_candidate"
	// LinkHotspot points at an affected repository area.
	LinkHotspot LinkKind = "hotspot"
	// LinkInvestigation points at a promoted or related investigation.
	LinkInvestigation LinkKind = "investigation"
	// LinkOpportunity points at a promoted or related opportunity.
	LinkOpportunity LinkKind = "opportunity"
)

// Link points to another local record or a credential-free repository ref.
type Link struct {
	Kind       LinkKind
	TargetType string
	TargetID   string
	Note       string
	CreatedAt  time.Time
}

// StatusChange records a deliberate lifecycle transition.
type StatusChange struct {
	From      Status
	To        Status
	Rationale string
	At        time.Time
}

// Promotion preserves the local concern's downstream workflow identity.
type Promotion struct {
	Kind            string
	InvestigationID string
	HypothesisID    string
	OpportunityID   string
	PromotedAt      time.Time
}

// Concern is a durable local intake record. WorkspaceID is an opaque corpus
// identity; absolute host paths are never stored here.
type Concern struct {
	ID               string
	Repo             domain.RepoRef
	CommitSHA        string
	WorkspaceID      string
	Title            string
	ProblemStatement string
	SuspectedOwner   string
	Confidence       float64
	Unknowns         []string
	SuccessCriterion string
	Notes            string
	EvidenceIDs      []string
	SourceRefs       []domain.SourceRef
	SourceProvenance []evidence.SourceRevision
	Links            []Link
	Status           Status
	AuditTrail       []StatusChange
	Promotion        *Promotion
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Filter bounds an offline list or search.
type Filter struct {
	Repo   domain.RepoRef
	Status Status
	Query  string
	Limit  int
	Offset int
}

// ParseStored rejects malformed durable records before they enter concern
// workflows. Creation and transition validation remain owned by Service.
func (c *Concern) ParseStored() error {
	if c == nil || c.ID == "" || !c.Repo.IsValid() {
		return errors.New("concern identity and repository are required")
	}
	if !validStatus(c.Status) {
		return fmt.Errorf("unsupported concern status %q", c.Status)
	}
	for i, change := range c.AuditTrail {
		if !validStatus(change.From) || !validStatus(change.To) {
			return fmt.Errorf("concern audit entry %d has an unsupported status", i)
		}
	}
	if c.Status == StatusPromoted && c.Promotion == nil {
		return errors.New("promoted concern is missing promotion identity")
	}
	if c.Promotion != nil {
		switch c.Promotion.Kind {
		case "investigation":
			if c.Promotion.InvestigationID == "" || c.Promotion.HypothesisID == "" || c.Promotion.OpportunityID != "" {
				return errors.New("invalid investigation promotion identity")
			}
		case "opportunity":
			if c.Promotion.InvestigationID == "" || c.Promotion.HypothesisID == "" || c.Promotion.OpportunityID == "" {
				return errors.New("invalid opportunity promotion identity")
			}
		default:
			return fmt.Errorf("unsupported concern promotion kind %q", c.Promotion.Kind)
		}
	}
	return nil
}
