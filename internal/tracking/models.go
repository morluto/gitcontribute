package tracking

import (
	"time"

	"github.com/morluto/gitcontribute/internal/evidence"
)

const (
	// CurrentBundleSchemaVersion includes portable evidence provenance.
	CurrentBundleSchemaVersion = 2
)

// Outcome records a local triage or lifecycle decision.
type Outcome string

const (
	OutcomeViewed       Outcome = "viewed"
	OutcomeIgnored      Outcome = "ignored"
	OutcomeSaved        Outcome = "saved"
	OutcomeInvestigated Outcome = "investigated"
	OutcomeImplemented  Outcome = "implemented"
	OutcomeSubmitted    Outcome = "submitted"
	OutcomeMerged       Outcome = "merged"
	OutcomeRejected     Outcome = "rejected"
	OutcomeAbandoned    Outcome = "abandoned"
)

// TargetKind names the kinds of local corpus references that can be tracked.
type TargetKind string

const (
	TargetRepository    TargetKind = "repository"
	TargetIssue         TargetKind = "issue"
	TargetPullRequest   TargetKind = "pull_request"
	TargetThread        TargetKind = "thread"
	TargetOpportunity   TargetKind = "opportunity"
	TargetInvestigation TargetKind = "investigation"
)

// TriageEvent records a single local triage decision for a typed target.
type TriageEvent struct {
	ID              string
	TargetKind      TargetKind
	TargetRef       string
	Outcome         Outcome
	Reason          string
	Lens            string
	SourceEventAt   time.Time
	RepositoryID    *int64
	ThreadID        *int64
	InvestigationID string
	OpportunityID   string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Contribution records prepared or submitted contribution material for an
// opportunity, kept separate from the live GitHub state.
type Contribution struct {
	ID            string
	OpportunityID string
	Kind          string
	Title         string
	Body          string
	Reference     string
	ReferenceURL  string
	PreparedAt    time.Time
	SubmittedAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Metadata      map[string]any
}

// ContributionOutcome records a lifecycle event for a contribution.
type ContributionOutcome struct {
	ID             string
	ContributionID string
	Outcome        Outcome
	Reason         string
	SourceEventAt  time.Time
	CreatedAt      time.Time
}

// TriageEventFilter bounds the triage events returned by a query.
type TriageEventFilter struct {
	TargetKind TargetKind
	TargetRef  string
	Outcome    Outcome
	Lens       string
	Limit      int
}

// ContributionFilter bounds the contributions returned by a query.
type ContributionFilter struct {
	OpportunityID string
	Kind          string
	Limit         int
}

// ExportOptions bounds a local metadata export.
type ExportOptions struct {
	Limit int
}

// Bundle is a portable, deterministic snapshot of local tracking metadata.
type Bundle struct {
	SchemaVersion        int                    `json:"schema_version"`
	TriageEvents         []*TriageEvent         `json:"triage_events"`
	Contributions        []*Contribution        `json:"contributions"`
	ContributionOutcomes []*ContributionOutcome `json:"contribution_outcomes"`
	Evidence             []*evidence.Evidence   `json:"evidence,omitempty"`
}
