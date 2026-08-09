package investigation

import (
	"errors"
	"fmt"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
)

// Category classifies the kind of hypothesis or opportunity.
type Category string

const (
	CategoryBug           Category = "bug"
	CategoryPerformance   Category = "performance"
	CategoryArchitecture  Category = "architecture"
	CategoryTesting       Category = "testing"
	CategoryDocumentation Category = "documentation"
	CategoryMaintenance   Category = "maintenance"
	CategoryCompatibility Category = "compatibility"
	CategorySecurity      Category = "security"
	CategoryOther         Category = "other"
)

// HypothesisStatus is the lifecycle of an individual hypothesis.
type HypothesisStatus string

const (
	HypothesisProposed   HypothesisStatus = "proposed"
	HypothesisPromoted   HypothesisStatus = "promoted"
	HypothesisRejected   HypothesisStatus = "rejected"
	HypothesisDeferred   HypothesisStatus = "deferred"
	HypothesisSuperseded HypothesisStatus = "superseded"
)

// OpportunityStatus is the lifecycle of an opportunity.
type OpportunityStatus string

const (
	OpportunityHypothesis        OpportunityStatus = "hypothesis"
	OpportunityReproduced        OpportunityStatus = "reproduced"
	OpportunityValidated         OpportunityStatus = "validated"
	OpportunityMaintainerAligned OpportunityStatus = "maintainer_aligned"
	OpportunityImplemented       OpportunityStatus = "implemented"
	OpportunitySubmitted         OpportunityStatus = "submitted"
	OpportunityMerged            OpportunityStatus = "merged"
	OpportunityRejected          OpportunityStatus = "rejected"
	OpportunityDeferred          OpportunityStatus = "deferred"
	OpportunitySuperseded        OpportunityStatus = "superseded"
)

// CollisionStatus records whether known competing work exists.
type CollisionStatus string

const (
	CollisionUnknown   CollisionStatus = "unknown"
	CollisionNone      CollisionStatus = "none"
	CollisionPossible  CollisionStatus = "possible"
	CollisionConfirmed CollisionStatus = "confirmed"
	CollisionBlocked   CollisionStatus = "blocked"
)

// StatusChange records a deliberate lifecycle transition with rationale.
type StatusChange struct {
	From      string
	To        string
	Rationale string
	At        time.Time
}

// Investigation is a durable workspace scoped to a repository and commit.
type Investigation struct {
	ID               string
	Repo             domain.RepoRef
	CommitSHA        string
	Lens             string
	Status           InvestigationStatus
	ThreadBaseline   *ThreadBaseline
	SeedHypothesisID string
	AuditTrail       []StatusChange
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// InvestigationStatus is the high-level state of the workspace.
type InvestigationStatus string

const (
	InvestigationOpen   InvestigationStatus = "open"
	InvestigationClosed InvestigationStatus = "closed"
)

// Hypothesis is a possible bug or improvement that has not yet met an evidence threshold.
type Hypothesis struct {
	ID                 string
	InvestigationID    string
	Title              string
	Description        string
	Category           Category
	ExpectedBehavior   string
	ObservedBehavior   string
	PotentialImpact    string
	OpenQuestions      []string
	AffectedComponents []string
	SourceRefs         []domain.SourceRef
	Links              []Link
	Status             HypothesisStatus
	AuditTrail         []StatusChange
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Link is an explicit reference to an issue, PR, commit, file, test, or other hypothesis.
type Link struct {
	Kind   string
	Ref    string
	Source domain.SourceRef
}

// Opportunity is a scoped potential contribution with evidence, impact, and collision status.
type Opportunity struct {
	ID                  string
	InvestigationID     string
	HypothesisID        string
	Title               string
	ProblemStatement    string
	Category            Category
	Scope               string
	Impact              string
	Confidence          float64
	ExpectedEffort      string
	Dependencies        []string
	CollisionStatus     CollisionStatus
	MaintainerAlignment string
	SourceRefs          []domain.SourceRef
	EvidenceIDs         []string
	Status              OpportunityStatus
	AuditTrail          []StatusChange
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// ParseStored parses an investigation at the durable JSON boundary.
func (i *Investigation) ParseStored() error {
	if i == nil || i.ID == "" || !i.Repo.IsValid() {
		return errors.New("investigation identity and repository are required")
	}
	if i.Status != InvestigationOpen && i.Status != InvestigationClosed {
		return fmt.Errorf("unsupported investigation status %q", i.Status)
	}
	for index, change := range i.AuditTrail {
		fromValid := validInvestigationStatus(InvestigationStatus(change.From)) || (index == 0 && change.From == "")
		if !fromValid || !validInvestigationStatus(InvestigationStatus(change.To)) {
			return fmt.Errorf("investigation audit entry %d has an unsupported status", index)
		}
	}
	return nil
}

// ParseStored parses a hypothesis at the durable JSON boundary.
func (h *Hypothesis) ParseStored() error {
	if h == nil || h.ID == "" || h.InvestigationID == "" {
		return errors.New("hypothesis identity and investigation are required")
	}
	if !ValidCategory(h.Category) {
		return fmt.Errorf("unsupported hypothesis category %q", h.Category)
	}
	switch h.Status {
	case HypothesisProposed, HypothesisPromoted, HypothesisRejected, HypothesisDeferred, HypothesisSuperseded:
	default:
		return fmt.Errorf("unsupported hypothesis status %q", h.Status)
	}
	for index, change := range h.AuditTrail {
		fromValid := validHypothesisStatus(HypothesisStatus(change.From)) || (index == 0 && change.From == "")
		if !fromValid || !validHypothesisStatus(HypothesisStatus(change.To)) {
			return fmt.Errorf("hypothesis audit entry %d has an unsupported status", index)
		}
	}
	return nil
}

// ParseStored parses an opportunity at the durable JSON boundary, including
// the legacy empty representation of unknown collision state.
func (o *Opportunity) ParseStored() error {
	if o == nil || o.ID == "" || o.InvestigationID == "" || o.HypothesisID == "" {
		return errors.New("opportunity identity, investigation, and hypothesis are required")
	}
	if !ValidCategory(o.Category) {
		return fmt.Errorf("unsupported opportunity category %q", o.Category)
	}
	// Empty is the legacy JSON representation of the initial unknown state.
	if o.CollisionStatus == "" {
		o.CollisionStatus = CollisionUnknown
	}
	switch o.Status {
	case OpportunityHypothesis, OpportunityReproduced, OpportunityValidated, OpportunityMaintainerAligned,
		OpportunityImplemented, OpportunitySubmitted, OpportunityMerged, OpportunityRejected,
		OpportunityDeferred, OpportunitySuperseded:
	default:
		return fmt.Errorf("unsupported opportunity status %q", o.Status)
	}
	switch o.CollisionStatus {
	case CollisionUnknown, CollisionNone, CollisionPossible, CollisionConfirmed, CollisionBlocked:
	default:
		return fmt.Errorf("unsupported collision status %q", o.CollisionStatus)
	}
	for index, change := range o.AuditTrail {
		initialChange := index == 0 && change.From == "" && validOpportunityStatus(OpportunityStatus(change.To))
		initialCollisionChange := index == 0 && change.From == "" && validCollisionStatus(CollisionStatus(change.To))
		lifecycleChange := validOpportunityStatus(OpportunityStatus(change.From)) && validOpportunityStatus(OpportunityStatus(change.To))
		collisionChange := validCollisionStatus(CollisionStatus(change.From)) && validCollisionStatus(CollisionStatus(change.To))
		if !initialChange && !initialCollisionChange && !lifecycleChange && !collisionChange {
			return fmt.Errorf("opportunity audit entry %d has an unsupported status", index)
		}
	}
	return nil
}

func validInvestigationStatus(status InvestigationStatus) bool {
	return status == InvestigationOpen || status == InvestigationClosed
}

func validHypothesisStatus(status HypothesisStatus) bool {
	switch status {
	case HypothesisProposed, HypothesisPromoted, HypothesisRejected, HypothesisDeferred, HypothesisSuperseded:
		return true
	default:
		return false
	}
}

func validOpportunityStatus(status OpportunityStatus) bool {
	switch status {
	case OpportunityHypothesis, OpportunityReproduced, OpportunityValidated, OpportunityMaintainerAligned,
		OpportunityImplemented, OpportunitySubmitted, OpportunityMerged, OpportunityRejected,
		OpportunityDeferred, OpportunitySuperseded:
		return true
	default:
		return false
	}
}

func validCollisionStatus(status CollisionStatus) bool {
	switch status {
	case CollisionUnknown, CollisionNone, CollisionPossible, CollisionConfirmed, CollisionBlocked:
		return true
	default:
		return false
	}
}

// SupportingEvidence returns evidence items marked as supporting.
func (o *Opportunity) SupportingEvidence(all []*evidence.Evidence) []*evidence.Evidence {
	return filterEvidence(all, evidence.RelationSupporting)
}

// ContradictingEvidence returns evidence items marked as contradicting.
func (o *Opportunity) ContradictingEvidence(all []*evidence.Evidence) []*evidence.Evidence {
	return filterEvidence(all, evidence.RelationContradicting)
}

func filterEvidence(all []*evidence.Evidence, want evidence.Relation) []*evidence.Evidence {
	var out []*evidence.Evidence
	for _, e := range all {
		if e != nil && e.Relation == want {
			out = append(out, e)
		}
	}
	return out
}
