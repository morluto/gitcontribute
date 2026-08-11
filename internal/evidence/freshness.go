package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

// SourceSubjectKind identifies the independent corpus projection whose
// revision an evidence record used.
type SourceSubjectKind uint8

// Source subject kinds supported by evidence provenance.
const (
	SourceSubjectRepository SourceSubjectKind = iota + 1
	SourceSubjectThread
	SourceSubjectFacet
	SourceSubjectGuidance
)

// String returns the stable boundary spelling of a source subject kind.
func (k SourceSubjectKind) String() string {
	switch k {
	case SourceSubjectRepository:
		return "repository"
	case SourceSubjectThread:
		return "thread"
	case SourceSubjectFacet:
		return "facet"
	case SourceSubjectGuidance:
		return "guidance"
	default:
		return ""
	}
}

// GuidanceFacet is the repository-level facet used by contribution guidance.
const GuidanceFacet = "contribution_guidance"

// FreshnessStatus describes whether recorded source revisions still match the
// winning local corpus projections.
type FreshnessStatus string

// Freshness statuses returned by read-time evidence evaluation.
const (
	FreshnessFresh         FreshnessStatus = "fresh"
	FreshnessStale         FreshnessStatus = "stale"
	FreshnessUnknown       FreshnessStatus = "unknown"
	FreshnessNotApplicable FreshnessStatus = "not_applicable"
)

// ParseFreshnessStatus parses a durable evaluated freshness outcome.
func ParseFreshnessStatus(value string) (FreshnessStatus, error) {
	switch FreshnessStatus(value) {
	case FreshnessFresh:
		return FreshnessFresh, nil
	case FreshnessStale:
		return FreshnessStale, nil
	case FreshnessUnknown:
		return FreshnessUnknown, nil
	case FreshnessNotApplicable:
		return FreshnessNotApplicable, nil
	default:
		return "", fmt.Errorf("unsupported evidence freshness status %q", value)
	}
}

// ErrSourceRevisionUnavailable means a reader cannot find the current local
// projection for a recorded source subject.
var ErrSourceRevisionUnavailable = errors.New("evidence: source revision unavailable")

// SourceSubject is a parsed vendor-neutral identity for exactly one repository,
// thread, or independently refreshed facet. Its private representation keeps
// fields belonging to other subject variants out of downstream code.
type SourceSubject struct {
	kind       SourceSubjectKind
	repository domain.RepoRef
	threadKind domain.ThreadKind
	number     int
	facet      string
}

type sourceSubjectJSON struct {
	Kind       string `json:"kind"`
	Owner      string `json:"owner"`
	Repo       string `json:"repo"`
	ThreadKind string `json:"thread_kind,omitempty"`
	Number     int    `json:"number,omitempty"`
	Facet      string `json:"facet,omitempty"`
}

// ParseSourceSubject converts one broad storage or API representation into a
// canonical source subject. Guidance accepts its historical explicit facet but
// stores it implicitly so it has only one representation.
func ParseSourceSubject(kind, owner, repo, threadKind string, number int, facet string) (SourceSubject, error) {
	repository, err := domain.NewRepoRef(owner, repo)
	if err != nil {
		return SourceSubject{}, fmt.Errorf("invalid source repository: %w", err)
	}
	threadKind = strings.TrimSpace(threadKind)
	facet = strings.TrimSpace(facet)
	threadScoped := threadKind != "" || number != 0
	var parsedThreadKind domain.ThreadKind
	if threadScoped {
		if threadKind == "" || number <= 0 {
			return SourceSubject{}, errors.New("thread kind and positive number must be provided together")
		}
		parsedThreadKind, err = domain.ParseThreadKind(threadKind)
		if err != nil {
			return SourceSubject{}, err
		}
	}

	var parsedKind SourceSubjectKind
	switch strings.TrimSpace(kind) {
	case "repository":
		parsedKind = SourceSubjectRepository
		if threadScoped || facet != "" {
			return SourceSubject{}, errors.New("repository subject cannot include thread or facet fields")
		}
	case "thread":
		parsedKind = SourceSubjectThread
		if !threadScoped || facet != "" {
			return SourceSubject{}, errors.New("thread subject requires a thread and no facet")
		}
	case "facet":
		parsedKind = SourceSubjectFacet
		if facet == "" {
			return SourceSubject{}, errors.New("facet subject requires a facet name")
		}
	case "guidance":
		parsedKind = SourceSubjectGuidance
		if threadScoped || (facet != "" && facet != GuidanceFacet) {
			return SourceSubject{}, errors.New("guidance subject cannot include thread fields or another facet")
		}
		facet = ""
	default:
		return SourceSubject{}, fmt.Errorf("unsupported source subject kind %q", kind)
	}
	return SourceSubject{
		kind: parsedKind, repository: repository, threadKind: parsedThreadKind,
		number: number, facet: facet,
	}, nil
}

// NewRepositorySourceSubject returns a repository-scoped source subject.
func NewRepositorySourceSubject(repository domain.RepoRef) (SourceSubject, error) {
	return ParseSourceSubject("repository", repository.Owner(), repository.Repo(), "", 0, "")
}

// NewThreadSourceSubject returns a source subject for one issue or pull request.
func NewThreadSourceSubject(repository domain.RepoRef, kind domain.ThreadKind, number int) (SourceSubject, error) {
	return ParseSourceSubject("thread", repository.Owner(), repository.Repo(), string(kind), number, "")
}

// Kind returns the sealed subject variant.
func (s SourceSubject) Kind() SourceSubjectKind { return s.kind }

// Repository returns the parsed repository identity shared by every variant.
func (s SourceSubject) Repository() domain.RepoRef { return s.repository }

// Thread returns the subject's thread identity when it is thread-scoped.
func (s SourceSubject) Thread() (domain.ThreadKind, int, bool) {
	return s.threadKind, s.number, s.threadKind != ""
}

// Facet returns the facet name for facet subjects. Guidance has an implied
// contribution-guidance facet and therefore returns an empty string here.
func (s SourceSubject) Facet() string { return s.facet }

// MarshalJSON preserves the durable source-provenance object representation.
func (s SourceSubject) MarshalJSON() ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(sourceSubjectJSON{
		Kind: s.kind.String(), Owner: s.repository.Owner(), Repo: s.repository.Repo(),
		ThreadKind: string(s.threadKind), Number: s.number, Facet: s.facet,
	})
}

// UnmarshalJSON parses durable source provenance before it enters the domain.
func (s *SourceSubject) UnmarshalJSON(data []byte) error {
	var raw sourceSubjectJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	parsed, err := ParseSourceSubject(raw.Kind, raw.Owner, raw.Repo, raw.ThreadKind, raw.Number, raw.Facet)
	if err != nil {
		return err
	}
	*s = parsed
	return nil
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

// RevisionReader returns the current winning revision for one stored subject,
// or ErrSourceRevisionUnavailable when the current projection is unavailable.
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
		if errors.Is(err, ErrSourceRevisionUnavailable) {
			unknownReasons = append(unknownReasons, fmt.Sprintf("current revision for %s is unavailable", recorded.Subject))
			continue
		}
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

// Validate rejects the invalid zero value. Variant-specific fields are parsed
// together by ParseSourceSubject and cannot be modified independently.
func (s SourceSubject) Validate() error {
	if !s.repository.IsValid() || s.kind.String() == "" {
		return errors.New("source subject is not parsed")
	}
	return nil
}

// Key returns a stable case-insensitive subject identity.
func (s SourceSubject) Key() string {
	return strings.ToLower(fmt.Sprintf("%s:%s:%s:%d:%s", s.kind, s.repository, s.threadKind, s.number, s.facet))
}

func (s SourceSubject) String() string {
	repo := s.repository.String()
	thread := fmt.Sprintf("%s:%s#%d", s.threadKind, repo, s.number)
	switch s.kind {
	case SourceSubjectRepository:
		return "repository " + repo
	case SourceSubjectThread:
		return "thread " + thread
	case SourceSubjectFacet:
		if s.threadKind != "" {
			return fmt.Sprintf("facet %s on %s", s.facet, thread)
		}
		return fmt.Sprintf("facet %s on %s", s.facet, repo)
	case SourceSubjectGuidance:
		return "guidance " + repo
	default:
		return "invalid source subject"
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
