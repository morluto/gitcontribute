package github

import "time"

// ThreadKind classifies an issue-list entry.
type ThreadKind string

const (
	ThreadKindIssue       ThreadKind = "issue"
	ThreadKindPullRequest ThreadKind = "pull_request"
)

// Repository is a domain-neutral view of a GitHub repository.
type Repository struct {
	ID            int64
	NodeID        string
	Owner         string
	Name          string
	FullName      string
	Description   string
	DefaultBranch string
	HTMLURL       string
	Private       bool
	Fork          bool
	Archived      bool
	IsTemplate    bool
	Stars         int
	Watchers      int
	Forks         int
	OpenIssues    int
	Language      string
	License       string
	Topics        []string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	PushedAt      *time.Time
}

// RepositoryFile is a bounded text file read from a repository at its default
// branch. Content API types terminate in the GitHub adapter.
type RepositoryFile struct {
	Path    string
	SHA     string
	HTMLURL string
	Content string
}

// Issue is a domain-neutral view of an issue or pull-request marker from
// the issues list endpoint.
type Issue struct {
	RepositoryOwner   string
	RepositoryName    string
	ID                int64
	NodeID            string
	Number            int
	Kind              ThreadKind
	Title             string
	Body              string
	State             string
	StateReason       string
	Draft             bool
	Locked            bool
	Author            string
	AuthorAssociation string
	Labels            []string
	Assignees         []string
	Milestone         string
	CommentsCount     int
	CreatedAt         time.Time
	UpdatedAt         time.Time
	ClosedAt          *time.Time
	HTMLURL           string
	PullRequestURL    string
}

// Identity is the stable login and identifiers associated with the active
// GitHub read credential.
type Identity struct {
	Login  string
	ID     int64
	NodeID string
}

// AuthoredPullRequestSearchOptions selects one bounded authored-PR search page.
// UpdatedAfter is translated to GitHub Search's UTC date-granularity qualifier.
type AuthoredPullRequestSearchOptions struct {
	Login        string
	State        string
	UpdatedAfter time.Time
	PageOptions
}

// AuthoredPullRequestSearchResult preserves GitHub's pagination, rate, and
// incomplete-results signals alongside the converted pull-request markers.
type AuthoredPullRequestSearchResult struct {
	Total      int
	Incomplete bool
	Items      []Issue
	Page       PageInfo
	Rate       RateInfo
}

// IssueComment is a domain-neutral view of an issue comment.
type IssueComment struct {
	ID                int64
	NodeID            string
	Body              string
	Author            string
	AuthorAssociation string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	HTMLURL           string
	IssueURL          string
}

// IssueTimelineEvent is a product-owned view of one issue timeline event.
// Source fields are populated only for explicit cross-reference events; callers
// must not infer relationships from URLs or prose.
type IssueTimelineEvent struct {
	ID                  int64
	Event               string
	Actor               string
	CommitID            string
	CreatedAt           time.Time
	SourceOwner         string
	SourceRepository    string
	SourceNumber        int
	SourceIsPullRequest bool
}

// PullRequestDetails is the PR-specific metadata beyond the issue marker.
type PullRequestDetails struct {
	ID                int64
	NodeID            string
	Number            int
	State             string
	Title             string
	Body              string
	Draft             bool
	Locked            bool
	Author            string
	AuthorAssociation string
	Labels            []string
	Assignees         []string
	Milestone         string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	ClosedAt          *time.Time
	MergedAt          *time.Time
	Merged            bool
	Mergeable         *bool
	MergeCommitSHA    string
	HeadRef           string
	HeadSHA           string
	BaseRef           string
	BaseSHA           string
	CommentsCount     int
	Commits           int
	Additions         int
	Deletions         int
	ChangedFiles      int
	HTMLURL           string
}

// PullRequestStatusOptions bounds each collection returned by a single
// source-backed pull-request status read. GitHub currently caps these
// connections at 100 items per request.
type PullRequestStatusOptions struct {
	PageSize int
	MaxPages int
}

// FacetCoverage describes exactly how much of a GitHub collection was
// returned. Callers must not replace a complete child snapshot when Complete
// is false.
type FacetCoverage struct {
	Complete    bool
	Fetched     int
	Total       int
	HasNextPage bool
	EndCursor   string
}

// FacetResult pairs product-owned GitHub values with their source coverage.
type FacetResult[T any] struct {
	Items    []T
	Coverage FacetCoverage
}

// PullRequestStatus is a bounded, source-backed health snapshot. Scalar
// coverage is separate from values so an observed absence (for example, not
// queued) is distinguishable from unavailable data.
type PullRequestStatus struct {
	NodeID             string
	HeadSHA            string
	SourceUpdatedAt    time.Time
	MergeState         PullRequestMergeState
	MergeStateCoverage FacetCoverage
	MergeQueue         *PullRequestMergeQueueEntry
	MergeQueueCoverage FacetCoverage
	Checks             FacetResult[PullRequestCheck]
	ReviewThreads      FacetResult[PullRequestReviewThread]
	ClosingIssues      FacetResult[PullRequestClosingIssue]
	Files              FacetResult[PullRequestFile]
}

// PullRequestMergeState preserves GitHub's detailed merge state while making
// null and UNKNOWN mergeability explicitly unknown rather than negative.
type PullRequestMergeState struct {
	MergeStateStatus string
	Mergeable        string
	MergeableKnown   bool
}

// PullRequestMergeQueueEntry describes the PR's current queue entry.
type PullRequestMergeQueueEntry struct {
	NodeID                      string
	State                       string
	Position                    int
	EnqueuedAt                  time.Time
	EstimatedTimeToMergeSeconds *int
}

// PullRequestCheck is one check-run or commit status in the head commit's
// status-check rollup.
type PullRequestCheck struct {
	Kind        string
	Name        string
	Status      string
	Conclusion  string
	DetailsURL  string
	StartedAt   *time.Time
	CompletedAt *time.Time
}

// PullRequestReviewThread is one source review conversation. IsResolved is
// retained so callers can derive unresolved-thread counts without inference.
type PullRequestReviewThread struct {
	NodeID     string
	IsResolved bool
	IsOutdated bool
	Path       string
	Line       *int
	StartLine  *int
}

// PullRequestClosingIssue is an issue GitHub reports this PR will close.
type PullRequestClosingIssue struct {
	NodeID             string
	RepositoryFullName string
	Number             int
	HTMLURL            string
}

// PullRequestFile is a changed path in the PR snapshot.
type PullRequestFile struct {
	Path       string
	ChangeType string
	Additions  int
	Deletions  int
}

// Review is a domain-neutral view of a pull request review.
type Review struct {
	ID                int64
	NodeID            string
	State             string
	Body              string
	Author            string
	AuthorAssociation string
	CommitID          string
	SubmittedAt       time.Time
	HTMLURL           string
	PullRequestURL    string
}

// ReviewComment is a domain-neutral view of a pull request review comment.
type ReviewComment struct {
	ID                int64
	NodeID            string
	InReplyTo         int64
	Body              string
	Path              string
	DiffHunk          string
	Author            string
	AuthorAssociation string
	CommitID          string
	OriginalCommitID  string
	PullRequestURL    string
	HTMLURL           string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	Line              int
	OriginalLine      int
	StartLine         int
	OriginalStartLine int
	Side              string
	StartSide         string
	Position          int
	OriginalPosition  int
	SubjectType       string
}

// PageInfo carries response pagination metadata.
type PageInfo struct {
	Page      int
	PerPage   int
	NextPage  int
	PrevPage  int
	FirstPage int
	LastPage  int
	HasNext   bool
	HasPrev   bool
	HasFirst  bool
	HasLast   bool
}

// RateInfo carries rate-limit metadata from the response headers.
type RateInfo struct {
	Limit     int
	Remaining int
	Used      int
	Reset     time.Time
	Resource  string
}

// ListResult is the common wrapper for paginated list responses.
type ListResult[T any] struct {
	Items []T
	Page  PageInfo
	Rate  RateInfo
}

// PageOptions specifies pagination parameters.
type PageOptions struct {
	Page    int
	PerPage int
}

// ListIssueOptions specifies filters and pagination for listing repository issues.
type ListIssueOptions struct {
	State     string
	Sort      string
	Direction string
	Since     time.Time
	Labels    []string
	PageOptions
}

// RepositorySearchOptions controls one GitHub repository search page.
type RepositorySearchOptions struct {
	Query string
	Sort  string
	Order string
	PageOptions
}

// RepositorySearchResult preserves GitHub's truncation and pagination facts.
type RepositorySearchResult struct {
	Total      int
	Incomplete bool
	Items      []Repository
	Page       PageInfo
	Rate       RateInfo
}
