package investigation

import (
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
