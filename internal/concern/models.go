// Package concern models low-confidence local findings before they become
// contribution hypotheses or public issues.
package concern

import (
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
}
