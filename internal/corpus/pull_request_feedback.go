package corpus

import "time"

const (
	feedbackFacetIssueComments  = "pr_feedback_issue_comments"
	feedbackFacetReviews        = "pr_feedback_reviews"
	feedbackFacetInlineComments = "pr_feedback_inline_comments"
	feedbackFacetReviewThreads  = "pr_feedback_review_threads"
)

var feedbackChannels = []string{"issue_comments", "submitted_reviews", "inline_comments", "review_threads"}

func feedbackFacetForChannel(channel string) string {
	switch channel {
	case "issue_comments":
		return feedbackFacetIssueComments
	case "submitted_reviews":
		return feedbackFacetReviews
	case "inline_comments":
		return feedbackFacetInlineComments
	case "review_threads":
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
	State                  string
	NextPage               int
	Complete               bool
	Truncated              bool
	DiscoveredPullRequests int
	Requests               int
	Channels               []string
	ThreadState            string
	LastError              string
	SourceUpdatedAt        time.Time
	UpdatedAt              time.Time
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
	PullRequestMergedKnown    bool
	PullRequestMerged         bool
	Channel                   string
	FeedbackID                string
	ThreadExternalID          string
	Author                    string
	Body                      string
	Path                      string
	Line                      *int
	StartLine                 *int
	CommitOID                 string
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
	ResolvedKnown             bool
	Resolved                  bool
	Outdated                  bool
	HeadSHA                   string
	SourceUpdatedAt           time.Time
	SourceObservationID       int64
	SourceObservationSequence int64
}

// FeedbackSearchFilter scopes an offline normalized feedback search.
type FeedbackSearchFilter struct {
	RepositoryID      int64
	FeedbackAuthor    string
	PullRequestAuthor string
	State             string
	Merged            string
	ThreadState       string
	Channel           string
	Text              string
	CreatedAfter      time.Time
	CreatedBefore     time.Time
	UpdatedAfter      time.Time
	UpdatedBefore     time.Time
	Sort              string
	Order             string
	Limit             int
	Cursor            string
}

type FeedbackSearchPage struct {
	Items      []PullRequestFeedbackProjection
	Total      int
	NextCursor string
	Truncated  bool
	Coverage   FeedbackCoverageSummary
}

type FeedbackCoverageSummary struct {
	Status            string
	DiscoveryComplete bool
	IncompletePRs     int
	TotalPullRequests int
	Channels          []string
}
