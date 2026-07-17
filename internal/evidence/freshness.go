package evidence

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

// SourceSubjectKind identifies the independent corpus projection whose
// revision an evidence record used.
type SourceSubjectKind string

const (
	SourceSubjectRepository SourceSubjectKind = "repository"
	SourceSubjectThread     SourceSubjectKind = "thread"
	SourceSubjectFacet      SourceSubjectKind = "facet"
	SourceSubjectGuidance   SourceSubjectKind = "guidance"
)

// GuidanceFacet is the repository-level facet used by contribution guidance.
const GuidanceFacet = "contribution_guidance"

// FreshnessStatus describes whether recorded source revisions still match the
// winning local corpus projections.
type FreshnessStatus string

const (
	FreshnessFresh         FreshnessStatus = "fresh"
	FreshnessStale         FreshnessStatus = "stale"
	FreshnessUnknown       FreshnessStatus = "unknown"
	FreshnessNotApplicable FreshnessStatus = "not_applicable"
)

// SourceSubject is a vendor-neutral identity for a repository, thread, or
// independently refreshed facet.
type SourceSubject struct {
	Kind       SourceSubjectKind `json:"kind"`
	Owner      string            `json:"owner"`
	Repo       string            `json:"repo"`
	ThreadKind string            `json:"thread_kind,omitempty"`
	Number     int               `json:"number,omitempty"`
	Facet      string            `json:"facet,omitempty"`
}

// SourceRevision records the exact winning source order used by evidence.
type SourceRevision struct {
	Subject             SourceSubject `json:"subject"`
	SourceUpdatedAt     time.Time     `json:"source_updated_at,omitempty"`
	ObservationSequence int64         `json:"observation_sequence"`
	ObservedAt          time.Time     `json:"observed_at"`
}

// Freshness is a derived read-time assessment. It is never persisted over the
// evidence relation and does not imply that stale evidence is invalid.
type Freshness struct {
	Status FreshnessStatus
	Reason string
}

// RevisionReader returns the current winning revision for one stored subject.
// A nil revision means the subject or its current projection is unavailable.
type RevisionReader interface {
	CurrentSourceRevision(ctx context.Context, subject SourceSubject) (*SourceRevision, error)
}

// FreshnessEvaluator compares evidence provenance with current local corpus
// projections. It has no network, process, or write capability.
type FreshnessEvaluator struct {
	reader RevisionReader
}

// NewFreshnessEvaluator returns a pure read-side freshness evaluator.
func NewFreshnessEvaluator(reader RevisionReader) *FreshnessEvaluator {
	return &FreshnessEvaluator{reader: reader}
}

// Evaluate derives freshness without modifying the evidence record.
func (e *FreshnessEvaluator) Evaluate(ctx context.Context, item *Evidence) (Freshness, error) {
	if item == nil {
		return Freshness{}, errors.New("evidence is required")
	}
	if err := ctx.Err(); err != nil {
		return Freshness{}, err
	}
	revisions, err := NormalizeSourceRevisions(item.SourceProvenance)
	if err != nil {
		return Freshness{}, err
	}
	if len(revisions) == 0 {
		if item.Type == EvidenceTypeGitHubSource {
			return Freshness{Status: FreshnessUnknown, Reason: "GitHub evidence has no recorded corpus source revision"}, nil
		}
		return Freshness{Status: FreshnessNotApplicable, Reason: "local evidence has no corpus source revision"}, nil
	}
	if e == nil || e.reader == nil {
		return Freshness{}, errors.New("source revision reader is required")
	}

	var staleReasons, unknownReasons []string
	for _, recorded := range revisions {
		current, err := e.reader.CurrentSourceRevision(ctx, recorded.Subject)
		if err != nil {
			return Freshness{}, fmt.Errorf("read current %s revision: %w", recorded.Subject, err)
		}
		if current == nil {
			unknownReasons = append(unknownReasons, fmt.Sprintf("current revision for %s is unavailable", recorded.Subject))
			continue
		}
		ordering := compareSourceOrder(*current, recorded)
		switch {
		case ordering > 0:
			staleReasons = append(staleReasons, fmt.Sprintf("%s advanced from %s to %s", recorded.Subject, sourceOrder(recorded), sourceOrder(*current)))
		case ordering < 0:
			unknownReasons = append(unknownReasons, fmt.Sprintf("current revision for %s predates recorded %s", recorded.Subject, sourceOrder(recorded)))
		}
	}
	if len(staleReasons) > 0 {
		return Freshness{Status: FreshnessStale, Reason: strings.Join(staleReasons, "; ")}, nil
	}
	if len(unknownReasons) > 0 {
		return Freshness{Status: FreshnessUnknown, Reason: strings.Join(unknownReasons, "; ")}, nil
	}
	return Freshness{Status: FreshnessFresh, Reason: "all recorded corpus source revisions match current projections"}, nil
}

// NormalizeSourceRevisions validates, de-duplicates, and deterministically
// orders source provenance without changing the caller's slice.
func NormalizeSourceRevisions(revisions []SourceRevision) ([]SourceRevision, error) {
	if len(revisions) == 0 {
		return nil, nil
	}
	out := make([]SourceRevision, len(revisions))
	seen := make(map[string]struct{}, len(revisions))
	for i, revision := range revisions {
		revision.Subject.Owner = strings.TrimSpace(revision.Subject.Owner)
		revision.Subject.Repo = strings.TrimSpace(revision.Subject.Repo)
		revision.Subject.ThreadKind = strings.TrimSpace(revision.Subject.ThreadKind)
		revision.Subject.Facet = strings.TrimSpace(revision.Subject.Facet)
		if !revision.SourceUpdatedAt.IsZero() {
			revision.SourceUpdatedAt = revision.SourceUpdatedAt.UTC()
		}
		if !revision.ObservedAt.IsZero() {
			revision.ObservedAt = revision.ObservedAt.UTC()
		}
		if err := revision.Validate(); err != nil {
			return nil, fmt.Errorf("source revision %d: %w", i, err)
		}
		key := revision.Subject.Key()
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("duplicate source subject %s", revision.Subject)
		}
		seen[key] = struct{}{}
		out[i] = revision
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Subject.Key() < out[j].Subject.Key() })
	return out, nil
}

// Validate checks that a source revision is traceable and ordered.
func (r SourceRevision) Validate() error {
	if err := r.Subject.Validate(); err != nil {
		return err
	}
	if r.ObservationSequence <= 0 {
		return errors.New("observation sequence must be positive")
	}
	if r.ObservedAt.IsZero() {
		return errors.New("observed_at is required")
	}
	return nil
}

// Validate checks the shape required by each subject kind.
func (s SourceSubject) Validate() error {
	if err := (domain.RepoRef{Owner: s.Owner, Repo: s.Repo}).Validate(); err != nil {
		return fmt.Errorf("invalid source repository: %w", err)
	}
	threadScoped := s.ThreadKind != "" || s.Number != 0
	if threadScoped && (s.ThreadKind == "" || s.Number <= 0) {
		return errors.New("thread kind and positive number must be provided together")
	}
	if s.ThreadKind != "" && s.ThreadKind != string(domain.IssueKind) && s.ThreadKind != string(domain.PullRequestKind) {
		return fmt.Errorf("unsupported thread kind %q", s.ThreadKind)
	}
	switch s.Kind {
	case SourceSubjectRepository:
		if threadScoped || s.Facet != "" {
			return errors.New("repository subject cannot include thread or facet fields")
		}
	case SourceSubjectThread:
		if !threadScoped || s.Facet != "" {
			return errors.New("thread subject requires a thread and no facet")
		}
	case SourceSubjectFacet:
		if s.Facet == "" {
			return errors.New("facet subject requires a facet name")
		}
	case SourceSubjectGuidance:
		if threadScoped || (s.Facet != "" && s.Facet != GuidanceFacet) {
			return errors.New("guidance subject cannot include thread fields or another facet")
		}
	default:
		return fmt.Errorf("unsupported source subject kind %q", s.Kind)
	}
	return nil
}

// Key returns a stable case-insensitive subject identity.
func (s SourceSubject) Key() string {
	return strings.ToLower(fmt.Sprintf("%s:%s/%s:%s:%d:%s", s.Kind, s.Owner, s.Repo, s.ThreadKind, s.Number, s.Facet))
}

func (s SourceSubject) String() string {
	repo := s.Owner + "/" + s.Repo
	thread := fmt.Sprintf("%s:%s#%d", s.ThreadKind, repo, s.Number)
	switch s.Kind {
	case SourceSubjectRepository:
		return "repository " + repo
	case SourceSubjectThread:
		return "thread " + thread
	case SourceSubjectFacet:
		if s.ThreadKind != "" {
			return fmt.Sprintf("facet %s on %s", s.Facet, thread)
		}
		return fmt.Sprintf("facet %s on %s", s.Facet, repo)
	case SourceSubjectGuidance:
		return "guidance " + repo
	default:
		return string(s.Kind) + " " + repo
	}
}

func compareSourceOrder(a, b SourceRevision) int {
	if a.SourceUpdatedAt.After(b.SourceUpdatedAt) {
		return 1
	}
	if a.SourceUpdatedAt.Before(b.SourceUpdatedAt) {
		return -1
	}
	switch {
	case a.ObservationSequence > b.ObservationSequence:
		return 1
	case a.ObservationSequence < b.ObservationSequence:
		return -1
	default:
		return 0
	}
}

func sourceOrder(r SourceRevision) string {
	timestamp := "unknown source_updated_at"
	if !r.SourceUpdatedAt.IsZero() {
		timestamp = "source_updated_at=" + r.SourceUpdatedAt.UTC().Format(time.RFC3339)
	}
	return fmt.Sprintf("(%s, sequence=%d)", timestamp, r.ObservationSequence)
}
