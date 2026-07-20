package research

import (
	"context"
	"errors"
	"fmt"
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

// FacetCoverage describes one thread child-facet snapshot.
type FacetCoverage struct {
	Facet     string
	Present   bool
	Complete  bool
	Truncated bool
	AsOf      time.Time
	Count     int
	Source    SourceRef
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
	Merged            bool
	MergedKnown       bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
	ClosedAt          time.Time
	MergedAt          time.Time
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

// CodeEvidence reports the latest immutable snapshot and bounded matches.
type CodeEvidence struct {
	Present   bool
	CommitSHA string
	Queries   []string
	Hits      []CodeHit
	Source    SourceRef
	Truncated bool
}

// HealthEvidence is a compact adapter view over offline health metrics.
type HealthEvidence struct {
	Available                      bool
	Archived                       bool
	OpenIssues                     int
	OpenPullRequests               int
	ExternalPRMergeRate            float64
	ExternalPRSampleSize           int
	IssueResponseMedianHours       float64
	PullRequestResponseMedianHours float64
	IssueResponseSampleSize        int
	PullRequestResponseSampleSize  int
	ThreadSampleSize               int
	ThreadsTruncated               bool
	Sources                        []SourceRef
	UnknownReason                  string
}

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
