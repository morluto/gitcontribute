package research

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/repository"
)

var (
	// ErrThreadNotFound distinguishes a missing local projection from missing
	// child coverage.
	ErrThreadNotFound = errors.New("research thread not found")
	// ErrThreadKindMismatch reports an explicit issue:/pr: ref that disagrees
	// with the stored projection.
	ErrThreadKindMismatch = errors.New("research thread kind mismatch")
)

// KindMismatchError preserves the requested and stored kinds.
func KindMismatchError(requested, stored domain.ThreadKind) error {
	return fmt.Errorf("%w: requested %s, stored %s", ErrThreadKindMismatch, requested, stored)
}

type facetCoverageState uint8

const (
	facetCoverageMissing facetCoverageState = iota
	facetCoverageObserved
)

// FacetCoverage describes one expected thread child facet. Its private state
// prevents missing facets from carrying counts, timestamps, or source claims.
type FacetCoverage struct {
	facet     string
	state     facetCoverageState
	complete  bool
	truncated bool
	asOf      time.Time
	count     int
	source    SourceRef
}

// MissingFacetCoverage records an expected facet with no local observation.
func MissingFacetCoverage(facet string) (FacetCoverage, error) {
	facet = strings.TrimSpace(facet)
	if facet == "" {
		return FacetCoverage{}, errors.New("coverage facet is required")
	}
	return FacetCoverage{facet: facet}, nil
}

// ObservedFacetCoverage records a present facet observation.
func ObservedFacetCoverage(facet string, complete bool, asOf time.Time, count int, source SourceRef) (FacetCoverage, error) {
	facet = strings.TrimSpace(facet)
	if facet == "" {
		return FacetCoverage{}, errors.New("coverage facet is required")
	}
	if asOf.IsZero() {
		return FacetCoverage{}, errors.New("coverage as-of time is required")
	}
	if count < 0 {
		return FacetCoverage{}, errors.New("coverage count cannot be negative")
	}
	if strings.TrimSpace(source.Source) == "" || !source.AsOf.Equal(asOf) {
		return FacetCoverage{}, errors.New("coverage source and matching as-of time are required")
	}
	return FacetCoverage{facet: facet, state: facetCoverageObserved, complete: complete, asOf: asOf, count: count, source: source}, nil
}

func (c FacetCoverage) Facet() string                  { return c.facet }
func (c FacetCoverage) Present() bool                  { return c.state == facetCoverageObserved }
func (c FacetCoverage) Complete() bool                 { return c.Present() && c.complete && !c.truncated }
func (c FacetCoverage) Truncated() bool                { return c.Present() && c.truncated }
func (c FacetCoverage) AsOf() time.Time                { return c.asOf }
func (c FacetCoverage) Count() int                     { return c.count }
func (c FacetCoverage) Source() SourceRef              { return c.source }
func (c FacetCoverage) Equal(other FacetCoverage) bool { return c == other }

// WithTruncated records a read bound only for an observed facet. Missing
// facets cannot be truncated because no collection was read.
func (c FacetCoverage) WithTruncated(truncated bool) FacetCoverage {
	if c.Present() {
		c.truncated = truncated
		if truncated {
			c.complete = false
		}
	}
	return c
}

// ThreadSnapshot is a product-owned issue/PR projection.
type ThreadSnapshot struct {
	Ref               ThreadRef
	Title             string
	Body              string
	Author            string
	AuthorAssociation string
	State             string
	StateReason       string
	Labels            []string
	Assignees         []string
	Draft             bool
	Locked            bool
	Milestone         string
	Merge             domain.MergeStatus
	CreatedAt         time.Time
	UpdatedAt         time.Time
	ClosedAt          time.Time
	Source            SourceRef
}

// DiscussionItem is one stored comment, review, or review comment.
type DiscussionItem struct {
	ID                int64
	Kind              string
	Body              string
	Author            string
	AuthorAssociation string
	State             string
	Path              string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	Source            SourceRef
}

// ThreadEvidence includes bounded child data and explicit coverage facts.
type ThreadEvidence struct {
	Thread     ThreadSnapshot
	Discussion []DiscussionItem
	Coverage   []FacetCoverage
	Truncated  bool
}

// Reference is an explicit source-text reference before corpus resolution.
type Reference struct {
	Repo   domain.RepoRef
	Kind   domain.ThreadKind
	Number int
	Source SourceRef
}

// RelationshipEvidence contains locally resolved explicit, cluster, and PR
// relationships. Absence is meaningful only when Sources is non-empty and the
// corresponding scan is not truncated.
type RelationshipEvidence struct {
	ClusterID         string
	Canonical         string
	DuplicateThreads  []RelatedThread
	PullRequests      []RelatedThread
	Sources           []SourceRef
	DuplicateCapped   bool
	PullRequestCapped bool
}

type codeEvidenceState uint8

const (
	codeEvidenceMissing codeEvidenceState = iota
	codeEvidenceObserved
)

// CodeEvidence is either a query against a missing local snapshot or evidence
// bound to one immutable commit. Its private representation prevents callers
// from attaching hits or provenance to a missing snapshot.
type CodeEvidence struct {
	state     codeEvidenceState
	commitSHA string
	queries   []string
	hits      []CodeHit
	source    SourceRef
	truncated bool
}

// MissingCodeEvidence records the queries that could not be run because no
// local code snapshot exists.
func MissingCodeEvidence(queries []string) CodeEvidence {
	return CodeEvidence{queries: append([]string{}, queries...)}
}

// ObservedCodeEvidence binds all returned matches to one immutable snapshot.
func ObservedCodeEvidence(commitSHA string, queries []string, hits []CodeHit, source SourceRef, truncated bool) (CodeEvidence, error) {
	commitSHA = strings.TrimSpace(commitSHA)
	if commitSHA == "" {
		return CodeEvidence{}, errors.New("code evidence commit SHA is required")
	}
	if strings.TrimSpace(source.Source) == "" {
		return CodeEvidence{}, errors.New("code evidence source is required")
	}
	if source.CommitSHA != "" && source.CommitSHA != commitSHA {
		return CodeEvidence{}, errors.New("code evidence source commit disagrees with snapshot")
	}
	for i, hit := range hits {
		if strings.TrimSpace(hit.Path) == "" || strings.TrimSpace(hit.MatchedTerm) == "" {
			return CodeEvidence{}, fmt.Errorf("code hit %d requires a path and matched term", i)
		}
		if hit.CommitSHA != commitSHA {
			return CodeEvidence{}, fmt.Errorf("code hit %d commit disagrees with snapshot", i)
		}
		if strings.TrimSpace(hit.Source.Source) == "" {
			return CodeEvidence{}, fmt.Errorf("code hit %d source is required", i)
		}
		if hit.Source.CommitSHA != "" && hit.Source.CommitSHA != commitSHA {
			return CodeEvidence{}, fmt.Errorf("code hit %d source commit disagrees with snapshot", i)
		}
	}
	return CodeEvidence{
		state: codeEvidenceObserved, commitSHA: commitSHA,
		queries: append([]string{}, queries...), hits: append([]CodeHit{}, hits...),
		source: source, truncated: truncated,
	}, nil
}

func (e CodeEvidence) Present() bool     { return e.state == codeEvidenceObserved }
func (e CodeEvidence) CommitSHA() string { return e.commitSHA }
func (e CodeEvidence) Queries() []string { return append([]string{}, e.queries...) }
func (e CodeEvidence) Hits() []CodeHit   { return append([]CodeHit{}, e.hits...) }
func (e CodeEvidence) Source() SourceRef { return e.source }
func (e CodeEvidence) Truncated() bool   { return e.Present() && e.truncated }

// HealthMetrics is the measured portion of one offline health report.
type HealthMetrics struct {
	Archived                       bool
	OpenIssues                     int
	OpenPullRequests               int
	ExternalPRMergeRate            *float64
	ExternalPRSampleSize           int
	IssueResponseMedianHours       float64
	PullRequestResponseMedianHours float64
	IssueResponseSampleSize        int
	PullRequestResponseSampleSize  int
	ThreadSampleSize               int
	ThreadsTruncated               bool
}

// HealthEvidence is either unavailable with a reason or an observed metrics
// snapshot with provenance. The private representation prevents unavailable
// sections from carrying apparently authoritative metric values.
type HealthEvidence struct {
	metrics       *HealthMetrics
	sources       []SourceRef
	unknownReason string
}

func MissingHealthEvidence(reason string) HealthEvidence {
	return HealthEvidence{unknownReason: strings.TrimSpace(reason)}
}

func ObservedHealthEvidence(metrics HealthMetrics, sources []SourceRef, unknownReason string) (HealthEvidence, error) {
	if len(sources) == 0 {
		return HealthEvidence{}, errors.New("health evidence requires provenance")
	}
	for i, source := range sources {
		if strings.TrimSpace(source.Source) == "" {
			return HealthEvidence{}, fmt.Errorf("health source %d is required", i)
		}
	}
	if metrics.OpenIssues < 0 || metrics.OpenPullRequests < 0 || metrics.ExternalPRSampleSize < 0 ||
		metrics.IssueResponseSampleSize < 0 || metrics.PullRequestResponseSampleSize < 0 || metrics.ThreadSampleSize < 0 {
		return HealthEvidence{}, errors.New("health counts cannot be negative")
	}
	if metrics.ExternalPRMergeRate != nil && (*metrics.ExternalPRMergeRate < 0 || *metrics.ExternalPRMergeRate > 1) {
		return HealthEvidence{}, errors.New("external pull-request merge rate must be between zero and one")
	}
	copyMetrics := metrics
	if metrics.ExternalPRMergeRate != nil {
		value := *metrics.ExternalPRMergeRate
		copyMetrics.ExternalPRMergeRate = &value
	}
	return HealthEvidence{
		metrics: &copyMetrics, sources: append([]SourceRef{}, sources...),
		unknownReason: strings.TrimSpace(unknownReason),
	}, nil
}

func (e HealthEvidence) Metrics() (HealthMetrics, bool) {
	if e.metrics == nil {
		return HealthMetrics{}, false
	}
	metrics := *e.metrics
	if metrics.ExternalPRMergeRate != nil {
		value := *metrics.ExternalPRMergeRate
		metrics.ExternalPRMergeRate = &value
	}
	return metrics, true
}
func (e HealthEvidence) Sources() []SourceRef  { return append([]SourceRef{}, e.sources...) }
func (e HealthEvidence) UnknownReason() string { return e.unknownReason }

// ThreadReader reads one thread and its already stored child facets.
type ThreadReader interface {
	ReadResearchThread(ctx context.Context, ref ThreadRef) (ThreadEvidence, error)
}

// RelationshipReader performs bounded local relationship lookups.
type RelationshipReader interface {
	ReadResearchRelationships(ctx context.Context, ref ThreadRef, explicit []Reference) (RelationshipEvidence, error)
}

// CodeReader searches only an already indexed local snapshot.
type CodeReader interface {
	ReadResearchCode(ctx context.Context, repo domain.RepoRef, terms []string) (CodeEvidence, error)
}

// HealthReader returns existing offline health metrics.
type HealthReader interface {
	ReadResearchHealth(ctx context.Context, repo domain.RepoRef) (HealthEvidence, error)
}

// Reader is the composed, product-owned source contract for a brief. Each
// embedded capability remains independently testable and side-effect bounded.
type Reader interface {
	repository.Reader
	ThreadReader
	RelationshipReader
	CodeReader
	HealthReader
}
