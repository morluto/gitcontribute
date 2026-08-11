// Package concern models low-confidence local findings before they become
// contribution hypotheses or public issues.
package concern

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

func ParseStatus(value string) (Status, error) {
	status := Status(strings.TrimSpace(value))
	if !validStatus(status) {
		return "", ErrInvalidStatus
	}
	return status, nil
}

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

func ParseLinkKind(value string) (LinkKind, error) {
	kind := LinkKind(strings.TrimSpace(value))
	if !validLinkKind(kind) {
		return "", ErrInvalidLink
	}
	return kind, nil
}

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
	kind            promotionKind
	investigationID string
	hypothesisID    string
	opportunityID   string
	promotedAt      time.Time
}

type promotionKind uint8

const (
	investigationPromotion promotionKind = iota + 1
	opportunityPromotion
)

func NewInvestigationPromotion(investigationID, hypothesisID string, promotedAt time.Time) (*Promotion, error) {
	return parsePromotion("investigation", investigationID, hypothesisID, "", promotedAt)
}

func NewOpportunityPromotion(investigationID, hypothesisID, opportunityID string, promotedAt time.Time) (*Promotion, error) {
	return parsePromotion("opportunity", investigationID, hypothesisID, opportunityID, promotedAt)
}

func parsePromotion(kind, investigationID, hypothesisID, opportunityID string, promotedAt time.Time) (*Promotion, error) {
	investigationID = strings.TrimSpace(investigationID)
	hypothesisID = strings.TrimSpace(hypothesisID)
	opportunityID = strings.TrimSpace(opportunityID)
	if investigationID == "" || hypothesisID == "" {
		return nil, errors.New("promotion investigation and hypothesis identities are required")
	}
	switch strings.TrimSpace(kind) {
	case "investigation":
		if opportunityID != "" {
			return nil, errors.New("investigation promotion cannot carry an opportunity identity")
		}
		return &Promotion{kind: investigationPromotion, investigationID: investigationID, hypothesisID: hypothesisID, promotedAt: promotedAt}, nil
	case "opportunity":
		if opportunityID == "" {
			return nil, errors.New("opportunity promotion identity is required")
		}
		return &Promotion{kind: opportunityPromotion, investigationID: investigationID, hypothesisID: hypothesisID, opportunityID: opportunityID, promotedAt: promotedAt}, nil
	default:
		return nil, fmt.Errorf("unsupported concern promotion kind %q", kind)
	}
}

func (p Promotion) Kind() string {
	if p.kind == opportunityPromotion {
		return "opportunity"
	}
	if p.kind == investigationPromotion {
		return "investigation"
	}
	return ""
}

func (p Promotion) InvestigationID() string { return p.investigationID }
func (p Promotion) HypothesisID() string    { return p.hypothesisID }
func (p Promotion) OpportunityID() string   { return p.opportunityID }
func (p Promotion) PromotedAt() time.Time   { return p.promotedAt }

func (p Promotion) valid() bool {
	return p.Kind() != "" && p.investigationID != "" && p.hypothesisID != "" &&
		(p.kind != opportunityPromotion || p.opportunityID != "") &&
		(p.kind != investigationPromotion || p.opportunityID == "")
}

// MarshalJSON retains the pre-sealing durable payload field names.
func (p Promotion) MarshalJSON() ([]byte, error) {
	if !p.valid() {
		return nil, errors.New("invalid concern promotion")
	}
	return json.Marshal(struct {
		Kind            string
		InvestigationID string
		HypothesisID    string
		OpportunityID   string
		PromotedAt      time.Time
	}{p.Kind(), p.InvestigationID(), p.HypothesisID(), p.OpportunityID(), p.PromotedAt()})
}

func (p *Promotion) UnmarshalJSON(data []byte) error {
	var input struct {
		Kind            string
		InvestigationID string
		HypothesisID    string
		OpportunityID   string
		PromotedAt      time.Time
	}
	if err := json.Unmarshal(data, &input); err != nil {
		return err
	}
	parsed, err := parsePromotion(input.Kind, input.InvestigationID, input.HypothesisID, input.OpportunityID, input.PromotedAt)
	if err != nil {
		return err
	}
	*p = *parsed
	return nil
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
	for i, link := range c.Links {
		if !validLinkKind(link.Kind) || strings.TrimSpace(link.TargetType) == "" || strings.TrimSpace(link.TargetID) == "" {
			return fmt.Errorf("concern link %d is invalid", i)
		}
	}
	if c.Status == StatusPromoted && c.Promotion == nil {
		return errors.New("promoted concern is missing promotion identity")
	}
	if c.Promotion != nil && !c.Promotion.valid() {
		return errors.New("invalid concern promotion identity")
	}
	return nil
}
