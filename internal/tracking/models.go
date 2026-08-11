package tracking

import (
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
)

const (
	// CurrentBundleSchemaVersion includes portable evidence provenance.
	CurrentBundleSchemaVersion = 2
)

// ParseOutcome converts durable or boundary text into a supported tracking
// decision.
func ParseOutcome(value string) (Outcome, error) {
	switch Outcome(strings.TrimSpace(value)) {
	case OutcomeViewed:
		return OutcomeViewed, nil
	case OutcomeIgnored:
		return OutcomeIgnored, nil
	case OutcomeSaved:
		return OutcomeSaved, nil
	case OutcomeInvestigated:
		return OutcomeInvestigated, nil
	case OutcomeImplemented:
		return OutcomeImplemented, nil
	case OutcomeSubmitted:
		return OutcomeSubmitted, nil
	case OutcomeMerged:
		return OutcomeMerged, nil
	case OutcomeRejected:
		return OutcomeRejected, nil
	case OutcomeAbandoned:
		return OutcomeAbandoned, nil
	default:
		return "", fmt.Errorf("unsupported tracking outcome %q", value)
	}
}

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

// ParseTargetKind converts durable or boundary text into a supported tracking
// target.
func ParseTargetKind(value string) (TargetKind, error) {
	switch TargetKind(strings.TrimSpace(value)) {
	case TargetRepository:
		return TargetRepository, nil
	case TargetIssue:
		return TargetIssue, nil
	case TargetPullRequest:
		return TargetPullRequest, nil
	case TargetThread:
		return TargetThread, nil
	case TargetOpportunity:
		return TargetOpportunity, nil
	case TargetInvestigation:
		return TargetInvestigation, nil
	default:
		return "", fmt.Errorf("unsupported triage target kind %q", value)
	}
}

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

// ContributionKindFilter is an optional parsed issue-or-pull-request filter.
// Its zero value includes both contribution kinds.
type ContributionKindFilter struct {
	kind domain.ThreadKind
}

// ParseContributionKind accepts the boundary aliases for a contribution and
// returns the repository-owned thread-kind value used by persisted records.
func ParseContributionKind(value string) (domain.ThreadKind, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "issue":
		return domain.IssueKind, nil
	case "pr", "pull_request", "pullrequest":
		return domain.PullRequestKind, nil
	default:
		return "", fmt.Errorf("unsupported contribution kind %q", value)
	}
}

// ParseContributionKindFilter parses an optional contribution-kind filter.
func ParseContributionKindFilter(value string) (ContributionKindFilter, error) {
	if strings.TrimSpace(value) == "" {
		return ContributionKindFilter{}, nil
	}
	kind, err := ParseContributionKind(value)
	if err != nil {
		return ContributionKindFilter{}, err
	}
	return ContributionKindFilter{kind: kind}, nil
}

// IsAny reports whether both contribution kinds are selected.
func (f ContributionKindFilter) IsAny() bool { return f.kind == "" }

// String returns the canonical persisted spelling, or empty for both kinds.
func (f ContributionKindFilter) String() string { return string(f.kind) }

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
	Kind          domain.ThreadKind
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
	Kind          ContributionKindFilter
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
