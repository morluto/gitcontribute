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

// Issue is a domain-neutral view of an issue or pull-request marker from
// the issues list endpoint.
type Issue struct {
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
