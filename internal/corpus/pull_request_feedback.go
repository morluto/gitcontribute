package corpus

import (
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

const (
	feedbackFacetIssueComments  = "pr_feedback_issue_comments"
	feedbackFacetReviews        = "pr_feedback_reviews"
	feedbackFacetInlineComments = "pr_feedback_inline_comments"
	feedbackFacetReviewThreads  = "pr_feedback_review_threads"
)

func feedbackFacet(channel FeedbackChannel) string {
	switch channel {
	case FeedbackIssueComments:
		return feedbackFacetIssueComments
	case FeedbackSubmittedReviews:
		return feedbackFacetReviews
	case FeedbackInlineComments:
		return feedbackFacetInlineComments
	case FeedbackReviewThreads:
		return feedbackFacetReviewThreads
	default:
		return ""
	}
}

func feedbackChannelForFacet(facet string) string {
	switch facet {
	case feedbackFacetIssueComments:
		return "issue_comments"
	case feedbackFacetReviews:
		return "submitted_reviews"
	case feedbackFacetInlineComments:
		return "inline_comments"
	case feedbackFacetReviewThreads:
		return "review_threads"
	default:
		return ""
	}
}

// FeedbackDiscovery is the durable repository-wide discovery checkpoint. A
// next page is intentionally retained when the provider or request/item bound
// stops a job so a retry can safely replay that page without losing items.
type FeedbackDiscovery struct {
	RepositoryID           int64
	Generation             int64
	NextPage               int
	State                  FeedbackDiscoveryState
	DiscoveredPullRequests int
	Requests               int
	PullRequestState       ThreadStateFilter
	Selection              FeedbackSelection
	LastError              string
	SourceUpdatedAt        time.Time
	UpdatedAt              time.Time
}

// FeedbackDiscoveryState is the mutually exclusive state of repository-wide
// pull-request discovery.
type FeedbackDiscoveryState string

const (
	FeedbackDiscoveryPending   FeedbackDiscoveryState = ""
	FeedbackDiscoveryComplete  FeedbackDiscoveryState = "complete"
	FeedbackDiscoveryTruncated FeedbackDiscoveryState = "truncated"
)

// IsComplete reports whether repository-wide discovery reached its last page.
func (d FeedbackDiscovery) IsComplete() bool { return d.State == FeedbackDiscoveryComplete }

// IsTruncated reports whether discovery stopped with a resumable next page.
func (d FeedbackDiscovery) IsTruncated() bool { return d.State == FeedbackDiscoveryTruncated }

func (s FeedbackDiscoveryState) valid() bool {
	return s == FeedbackDiscoveryPending || s == FeedbackDiscoveryComplete || s == FeedbackDiscoveryTruncated
}

// PullRequestFeedbackProjection is one normalized, queryable feedback item.
// The raw facet observation remains canonical; this row is rebuildable.
type PullRequestFeedbackProjection struct {
	ID                        int64
	RepositoryID              int64
	ThreadID                  int64
	PullRequestNumber         int
	PullRequestAuthor         string
	PullRequestState          string
	PullRequestMerge          domain.MergeStatus
	Channel                   string
	FeedbackID                string
	FeedbackNodeID            string
	ThreadExternalID          string
	InReplyToID               string
	Author                    string
	Body                      string
	Path                      string
	Line                      *int
	StartLine                 *int
	Side                      string
	StartSide                 string
	CommitOID                 string
	ReviewState               string
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
	Resolution                domain.ObservedBool
	ResolvedBy                string
	Outdated                  bool
	HeadSHA                   string
	SourceUpdatedAt           time.Time
	SourceObservationID       int64
	SourceObservationSequence int64
}

type FeedbackSearchPage struct {
	Items                    []PullRequestFeedbackProjection
	UnknownMergePullRequests []int
	Total                    int
	NextCursor               string
	Truncated                bool
	Coverage                 FeedbackCoverageSummary
}

// FeedbackCoverageState is the complete set of valid relationships between
// repository discovery and per-pull-request facet coverage.
type FeedbackCoverageState uint8

const (
	FeedbackCoverageUnknown FeedbackCoverageState = iota
	FeedbackCoveragePartialDiscovery
	FeedbackCoveragePartialFacets
	FeedbackCoverageComplete
)

type FeedbackCoverageSummary struct {
	State             FeedbackCoverageState
	IncompletePRs     int
	TotalPullRequests int
	Channels          []string
}

// Status returns the stable wire status derived from the coverage state.
func (c FeedbackCoverageSummary) Status() string {
	switch c.State {
	case FeedbackCoverageComplete:
		return "complete"
	case FeedbackCoveragePartialDiscovery, FeedbackCoveragePartialFacets:
		return "partial"
	default:
		return "unknown"
	}
}

// DiscoveryComplete reports whether repository discovery covers the requested
// channel and thread-state selection.
func (c FeedbackCoverageSummary) DiscoveryComplete() bool {
	return c.State == FeedbackCoveragePartialFacets || c.State == FeedbackCoverageComplete
}

// Complete reports whether both discovery and every requested facet are
// complete.
func (c FeedbackCoverageSummary) Complete() bool { return c.State == FeedbackCoverageComplete }
