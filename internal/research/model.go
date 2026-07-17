// Package research builds deterministic, source-backed thread research
// briefs. It owns no network, persistence, process, or GitHub mutation
// capability.
package research

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

const (
	// SchemaVersion changes whenever the portable brief contract changes.
	SchemaVersion = "research-brief.v1"
	// MaximumBodyExcerpt bounds untrusted source text in one brief.
	MaximumBodyExcerpt = 2000
)

// SectionStatus makes missing or incomplete evidence explicit.
type SectionStatus string

const (
	// StatusAvailable means the section is backed by complete stored evidence.
	StatusAvailable SectionStatus = "available"
	// StatusPartial means some expected stored evidence is incomplete or capped.
	StatusPartial SectionStatus = "partial"
	// StatusUnknown means the corpus cannot support a claim for the section.
	StatusUnknown SectionStatus = "unknown"
)

// ThreadRef is a validated issue or pull-request reference. Kind may be empty
// when the input used OWNER/REPO#NUMBER and the corpus must resolve it.
type ThreadRef struct {
	Repo   domain.RepoRef
	Kind   domain.ThreadKind
	Number int
}

// ParseThreadRef accepts OWNER/REPO#NUMBER and the explicit issue:, pr:, or
// pull_request: forms.
func ParseThreadRef(raw string) (ThreadRef, error) {
	raw = strings.TrimSpace(raw)
	var kind domain.ThreadKind
	for _, prefix := range []struct {
		text string
		kind domain.ThreadKind
	}{
		{"issue:", domain.IssueKind},
		{"pr:", domain.PullRequestKind},
		{"pull_request:", domain.PullRequestKind},
	} {
		if strings.HasPrefix(strings.ToLower(raw), prefix.text) {
			kind = prefix.kind
			raw = raw[len(prefix.text):]
			break
		}
	}
	idx := strings.LastIndexByte(raw, '#')
	if idx <= 0 || idx == len(raw)-1 {
		return ThreadRef{}, fmt.Errorf("invalid thread reference %q: expected OWNER/REPO#NUMBER", raw)
	}
	parts := strings.Split(raw[:idx], "/")
	if len(parts) != 2 {
		return ThreadRef{}, fmt.Errorf("invalid thread reference %q: expected OWNER/REPO#NUMBER", raw)
	}
	repo := domain.RepoRef{Owner: parts[0], Repo: parts[1]}
	if err := repo.Validate(); err != nil {
		return ThreadRef{}, fmt.Errorf("invalid thread reference %q: %w", raw, err)
	}
	number, err := strconv.Atoi(raw[idx+1:])
	if err != nil || number <= 0 {
		return ThreadRef{}, fmt.Errorf("invalid thread reference %q: expected positive number", raw)
	}
	return ThreadRef{Repo: repo, Kind: kind, Number: number}, nil
}

// String returns the explicit, stable form when kind is known.
func (r ThreadRef) String() string {
	prefix := ""
	if r.Kind != "" {
		prefix = string(r.Kind) + ":"
	}
	return fmt.Sprintf("%s%s#%d", prefix, r.Repo, r.Number)
}

// Validate checks a programmatically constructed thread reference.
func (r ThreadRef) Validate() error {
	if err := r.Repo.Validate(); err != nil {
		return err
	}
	if r.Number <= 0 {
		return errors.New("thread number must be positive")
	}
	if r.Kind != "" && r.Kind != domain.IssueKind && r.Kind != domain.PullRequestKind {
		return fmt.Errorf("unsupported thread kind %q", r.Kind)
	}
	return nil
}

// SourceRef is the JSON-stable provenance representation used by briefs.
type SourceRef struct {
	Source     string    `json:"source"`
	URL        string    `json:"url,omitempty"`
	CommitSHA  string    `json:"commit_sha,omitempty"`
	ObservedAt time.Time `json:"observed_at,omitempty"`
	AsOf       time.Time `json:"as_of,omitempty"`
}

// SectionMeta is embedded in every brief section.
type SectionMeta struct {
	Status        SectionStatus `json:"status"`
	Sources       []SourceRef   `json:"sources"`
	UnknownReason string        `json:"unknown_reason,omitempty"`
}

// Target identifies the resolved source thread.
type Target struct {
	Ref        string `json:"ref"`
	Repository string `json:"repository"`
	Kind       string `json:"kind"`
	Number     int    `json:"number"`
	URL        string `json:"url"`
}

// CurrentStateSection records lifecycle facts without inference.
type CurrentStateSection struct {
	SectionMeta
	State       string    `json:"state"`
	StateReason string    `json:"state_reason,omitempty"`
	Draft       bool      `json:"draft"`
	Locked      bool      `json:"locked"`
	Merged      bool      `json:"merged"`
	Labels      []string  `json:"labels"`
	Milestone   string    `json:"milestone,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
	ClosedAt    time.Time `json:"closed_at,omitempty"`
	MergedAt    time.Time `json:"merged_at,omitempty"`
}

// ProblemSection exposes only stored fields and a bounded verbatim excerpt.
type ProblemSection struct {
	SectionMeta
	Title       string   `json:"title"`
	BodyExcerpt string   `json:"body_excerpt,omitempty"`
	Labels      []string `json:"labels"`
	Assignees   []string `json:"assignees"`
}

// ChecklistHint is one source checkbox, not a claim of complete acceptance.
type ChecklistHint struct {
	Text    string    `json:"text"`
	Checked bool      `json:"checked"`
	Source  SourceRef `json:"source"`
}

// TextHint is a source heading or maintainer statement.
type TextHint struct {
	Text   string    `json:"text"`
	Author string    `json:"author,omitempty"`
	Source SourceRef `json:"source"`
}

// AcceptanceSection contains extracted hints with an explicit caveat.
type AcceptanceSection struct {
	SectionMeta
	Checklist            []ChecklistHint `json:"checklist"`
	RelevantHeadings     []TextHint      `json:"relevant_headings"`
	MaintainerStatements []TextHint      `json:"maintainer_statements"`
	Caveat               string          `json:"caveat"`
}

// Participant records public association and observed roles.
type Participant struct {
	Login       string   `json:"login"`
	Association string   `json:"association,omitempty"`
	Roles       []string `json:"roles"`
}

// ParticipantsSection reports only identities present in stored evidence.
type ParticipantsSection struct {
	SectionMeta
	Participants []Participant `json:"participants"`
}

// TimelineEvent is one bounded source event.
type TimelineEvent struct {
	At      time.Time `json:"at"`
	Kind    string    `json:"kind"`
	Actor   string    `json:"actor,omitempty"`
	Summary string    `json:"summary"`
	Source  SourceRef `json:"source"`
}

// TimelineSection is deterministically ordered by time and stable identity.
type TimelineSection struct {
	SectionMeta
	Events    []TimelineEvent `json:"events"`
	Truncated bool            `json:"truncated"`
}

// RelatedThread is a source-backed explicit or clustered relationship.
type RelatedThread struct {
	Ref      string    `json:"ref"`
	Kind     string    `json:"kind,omitempty"`
	Number   int       `json:"number"`
	Title    string    `json:"title,omitempty"`
	State    string    `json:"state,omitempty"`
	Relation string    `json:"relation"`
	Basis    string    `json:"basis"`
	URL      string    `json:"url"`
	Source   SourceRef `json:"source"`
}

// DuplicateSection separates candidates from confirmed duplicate decisions.
type DuplicateSection struct {
	SectionMeta
	ClusterID  string          `json:"cluster_id,omitempty"`
	Canonical  string          `json:"canonical,omitempty"`
	Candidates []RelatedThread `json:"candidates"`
	Truncated  bool            `json:"truncated"`
	Caveat     string          `json:"caveat"`
}

// PullRequestSection reports open PRs that mention or claim to close a target.
type PullRequestSection struct {
	SectionMeta
	PullRequests []RelatedThread `json:"pull_requests"`
	Truncated    bool            `json:"truncated"`
}

// CodeHit is one path-level match at an immutable indexed commit.
type CodeHit struct {
	Path        string    `json:"path"`
	Language    string    `json:"language,omitempty"`
	CommitSHA   string    `json:"commit_sha"`
	MatchedTerm string    `json:"matched_term"`
	Source      SourceRef `json:"source"`
}

// CodeSection contains bounded local-index hits.
type CodeSection struct {
	SectionMeta
	CommitSHA string    `json:"commit_sha,omitempty"`
	Queries   []string  `json:"queries"`
	Hits      []CodeHit `json:"hits"`
	Truncated bool      `json:"truncated"`
}

// GuidanceSection never invents contribution or AI policy.
type GuidanceSection struct {
	SectionMeta
	Text string `json:"text,omitempty"`
}

// HealthSection is a compact projection of existing offline metrics.
type HealthSection struct {
	SectionMeta
	Archived                       bool    `json:"archived"`
	OpenIssues                     int     `json:"open_issues"`
	OpenPullRequests               int     `json:"open_pull_requests"`
	ExternalPRMergeRate            float64 `json:"external_pr_merge_rate"`
	ExternalPRSampleSize           int     `json:"external_pr_sample_size"`
	IssueResponseMedianHours       float64 `json:"issue_response_median_hours"`
	PullRequestResponseMedianHours float64 `json:"pull_request_response_median_hours"`
	IssueResponseSampleSize        int     `json:"issue_response_sample_size"`
	PullRequestResponseSampleSize  int     `json:"pull_request_response_sample_size"`
	ThreadSampleSize               int     `json:"thread_sample_size"`
	ThreadsTruncated               bool    `json:"threads_truncated"`
}

// CoverageFact records one repository, thread, or local-index coverage fact.
type CoverageFact struct {
	Scope     string    `json:"scope"`
	Facet     string    `json:"facet"`
	Present   bool      `json:"present"`
	Complete  bool      `json:"complete"`
	Truncated bool      `json:"truncated"`
	AsOf      time.Time `json:"as_of,omitempty"`
	Count     int       `json:"count"`
}

// CoverageSection makes missing and partial inputs inspectable.
type CoverageSection struct {
	SectionMeta
	Facets []CoverageFact `json:"facets"`
	Gaps   []string       `json:"gaps"`
}

// NextCommand is a copyable, explicit remediation or follow-on read.
type NextCommand struct {
	Reason  string `json:"reason"`
	Command string `json:"command"`
}

// NextSection provides deterministic follow-up commands only.
type NextSection struct {
	SectionMeta
	Commands []NextCommand `json:"commands"`
}

// Sections is the fixed v1 research brief contract.
type Sections struct {
	CurrentState CurrentStateSection `json:"current_state"`
	Problem      ProblemSection      `json:"problem_statement"`
	Acceptance   AcceptanceSection   `json:"acceptance_hints"`
	Participants ParticipantsSection `json:"participants"`
	Timeline     TimelineSection     `json:"timeline"`
	Duplicates   DuplicateSection    `json:"duplicate_candidates"`
	PullRequests PullRequestSection  `json:"linked_pull_requests"`
	Code         CodeSection         `json:"relevant_code"`
	Guidance     GuidanceSection     `json:"contribution_guidance"`
	Health       HealthSection       `json:"repository_health"`
	Coverage     CoverageSection     `json:"coverage_and_gaps"`
	Next         NextSection         `json:"next_commands"`
}

// Brief is a deterministic human/agent research package.
type Brief struct {
	SchemaVersion string    `json:"schema_version"`
	GeneratedAt   time.Time `json:"generated_at"`
	SourceAsOf    time.Time `json:"source_as_of"`
	Target        Target    `json:"target"`
	Sections      Sections  `json:"sections"`
}

// ValidateProvenance verifies the core contract for all fixed sections.
func (b *Brief) ValidateProvenance() error {
	if b == nil {
		return errors.New("brief is nil")
	}
	sections := []struct {
		name string
		meta SectionMeta
	}{
		{"current_state", b.Sections.CurrentState.SectionMeta},
		{"problem_statement", b.Sections.Problem.SectionMeta},
		{"acceptance_hints", b.Sections.Acceptance.SectionMeta},
		{"participants", b.Sections.Participants.SectionMeta},
		{"timeline", b.Sections.Timeline.SectionMeta},
		{"duplicate_candidates", b.Sections.Duplicates.SectionMeta},
		{"linked_pull_requests", b.Sections.PullRequests.SectionMeta},
		{"relevant_code", b.Sections.Code.SectionMeta},
		{"contribution_guidance", b.Sections.Guidance.SectionMeta},
		{"repository_health", b.Sections.Health.SectionMeta},
		{"coverage_and_gaps", b.Sections.Coverage.SectionMeta},
		{"next_commands", b.Sections.Next.SectionMeta},
	}
	for _, section := range sections {
		if section.meta.Status == "" {
			return fmt.Errorf("section %s has no status", section.name)
		}
		if len(section.meta.Sources) == 0 && section.meta.UnknownReason == "" {
			return fmt.Errorf("section %s has neither source nor unknown reason", section.name)
		}
	}
	return nil
}
