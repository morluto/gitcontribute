package mcpcontract

// SyncPullRequestFeedbackInput refreshes distinct human feedback channels for
// exact pull requests without conflating absent coverage with no feedback. The
// operation also persists the local repository and pull-request identities
// required by those exact facet writes; it does not require prior broad sync.
type SyncPullRequestFeedbackInput struct {
	PullRequests       []ThreadRef `json:"pull_requests" jsonschema:"One to 50 exact pull requests"`
	ThreadState        string      `json:"thread_state,omitempty" jsonschema:"Review threads to return: unresolved or all"`
	Channels           []string    `json:"channels" jsonschema:"One or more of issue_comments, submitted_reviews, inline_comments, review_threads"`
	MaxItemsPerChannel int         `json:"max_items_per_channel,omitempty" jsonschema:"Maximum items per requested channel from 1 to 1000"`
	MaxRequests        int         `json:"max_requests,omitempty" jsonschema:"Maximum total GitHub requests from 1 to 1000"`
}

// IndexPullRequestFeedbackInput requests one repository-scoped, resumable
// pull-request discovery and feedback synchronization job. A retry resumes
// the durable discovery checkpoint for the repository.
type IndexPullRequestFeedbackInput struct {
	Repository         RepositoryRef `json:"repository" jsonschema:"One GitHub repository"`
	Channels           []string      `json:"channels" jsonschema:"Feedback channels: issue_comments, submitted_reviews, inline_comments, review_threads"`
	ThreadState        string        `json:"thread_state,omitempty" jsonschema:"Review threads to index: unresolved or all"`
	MaxPullRequests    int           `json:"max_pull_requests,omitempty" jsonschema:"Maximum pull requests retained in one job from 1 to 1000"`
	MaxItemsPerChannel int           `json:"max_items_per_channel,omitempty" jsonschema:"Maximum feedback items per channel and pull request from 1 to 1000"`
	MaxPages           int           `json:"max_pages,omitempty" jsonschema:"Maximum provider discovery pages per job from 1 to 1000"`
	MaxRequests        int           `json:"max_requests,omitempty" jsonschema:"Maximum total GitHub requests from 1 to 1000"`
}

// SearchPullRequestFeedbackInput filters the stored repository feedback
// projection. It never performs a network read.
type SearchPullRequestFeedbackInput struct {
	Repository        RepositoryRef `json:"repository" jsonschema:"One indexed GitHub repository"`
	FeedbackAuthor    string        `json:"feedback_author,omitempty" jsonschema:"Exact feedback author login"`
	PullRequestAuthor string        `json:"pull_request_author,omitempty" jsonschema:"Exact pull-request author login"`
	State             string        `json:"state,omitempty" jsonschema:"Pull-request state: open, closed, or all"`
	Merged            string        `json:"merged,omitempty" jsonschema:"Merge state: true, false, unknown, or any"`
	ThreadState       string        `json:"thread_state,omitempty" jsonschema:"Review-thread resolution: resolved, unresolved, or all"`
	Channel           string        `json:"channel,omitempty" jsonschema:"Feedback channel"`
	Text              string        `json:"text,omitempty" jsonschema:"Full-text feedback search"`
	CreatedAfter      string        `json:"created_after,omitempty" jsonschema:"Feedback created at or after RFC3339 timestamp"`
	CreatedBefore     string        `json:"created_before,omitempty" jsonschema:"Feedback created at or before RFC3339 timestamp"`
	UpdatedAfter      string        `json:"updated_after,omitempty" jsonschema:"Feedback updated at or after RFC3339 timestamp"`
	UpdatedBefore     string        `json:"updated_before,omitempty" jsonschema:"Feedback updated at or before RFC3339 timestamp"`
	Sort              string        `json:"sort,omitempty" jsonschema:"Sort: feedback_author, pull_request_state, merge_state, created, updated, or pull_request_number"`
	Order             string        `json:"order,omitempty" jsonschema:"Sort order: asc or desc"`
	Limit             int           `json:"limit,omitempty" jsonschema:"Results per page from 1 to 100"`
	Cursor            string        `json:"cursor,omitempty" jsonschema:"Opaque cursor returned by the previous page"`
	SnapshotToken     string        `json:"snapshot_token,omitempty" jsonschema:"Optional immutable corpus snapshot token from a previous offline read"`
}

type PullRequestFeedbackMatch struct {
	Repository           RepositoryRef `json:"repository"`
	PullRequest          ThreadRef     `json:"pull_request"`
	PullRequestReference string        `json:"pull_request_reference"`
	PullRequestAuthor    string        `json:"pull_request_author,omitempty"`
	PullRequestState     string        `json:"pull_request_state"`
	Merged               *bool         `json:"merged,omitempty"`
	Channel              string        `json:"channel"`
	FeedbackID           string        `json:"feedback_id"`
	ThreadID             string        `json:"thread_id,omitempty"`
	ThreadReference      string        `json:"thread_reference,omitempty"`
	CommentReference     string        `json:"comment_reference,omitempty"`
	FeedbackAuthor       string        `json:"feedback_author,omitempty"`
	Body                 string        `json:"body,omitempty"`
	Path                 string        `json:"path,omitempty"`
	Line                 *int          `json:"line,omitempty"`
	StartLine            *int          `json:"start_line,omitempty"`
	Outdated             bool          `json:"outdated"`
	Resolved             *bool         `json:"resolved,omitempty"`
	CreatedAt            string        `json:"created_at,omitempty"`
	UpdatedAt            string        `json:"updated_at,omitempty"`
	HeadSHA              string        `json:"head_sha,omitempty"`
	SourceObservationID  int64         `json:"source_observation_id"`
}

type SearchPullRequestFeedbackOutput struct {
	Status        string                     `json:"status"`
	Coverage      string                     `json:"coverage"`
	Matches       []PullRequestFeedbackMatch `json:"matches"`
	Total         int                        `json:"total"`
	Truncated     bool                       `json:"truncated"`
	NextCursor    string                     `json:"next_cursor,omitempty"`
	SnapshotToken string                     `json:"snapshot_token"`
	Projection    string                     `json:"projection_version"`
	IncompletePRs int                        `json:"incomplete_pull_requests,omitempty"`
	Recovery      *RecoveryPlan              `json:"recovery,omitempty"`
}
