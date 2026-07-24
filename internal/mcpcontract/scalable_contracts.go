package mcpcontract

import (
	"github.com/morluto/gitcontribute/internal/failure"

	"github.com/morluto/gitcontribute/internal/similarity"
)

// SearchGitHubRepositoriesInput defines one bounded live GitHub search.
type SearchGitHubRepositoriesInput struct {
	RawQuery       string   `json:"raw_query,omitempty" jsonschema:"Advanced raw GitHub query; exclusive with filters"`
	Text           string   `json:"text,omitempty" jsonschema:"Text to match"`
	MatchFields    []string `json:"match_fields,omitempty" jsonschema:"Text fields: name, description, or readme"`
	Topics         []string `json:"topics,omitempty" jsonschema:"Topics that must all match"`
	Language       string   `json:"language,omitempty" jsonschema:"Primary language"`
	StarsMin       int      `json:"stars_min,omitempty" jsonschema:"Minimum stargazer count"`
	StarsMax       int      `json:"stars_max,omitempty" jsonschema:"Maximum stargazer count"`
	CreatedAfter   string   `json:"created_after,omitempty" jsonschema:"Created on or after YYYY-MM-DD"`
	CreatedBefore  string   `json:"created_before,omitempty" jsonschema:"Created on or before YYYY-MM-DD"`
	PushedAfter    string   `json:"pushed_after,omitempty" jsonschema:"Pushed on or after YYYY-MM-DD"`
	PushedBefore   string   `json:"pushed_before,omitempty" jsonschema:"Pushed on or before YYYY-MM-DD"`
	Archived       *bool    `json:"archived,omitempty" jsonschema:"Archived state"`
	Fork           *bool    `json:"fork,omitempty" jsonschema:"Fork state"`
	Sort           string   `json:"sort,omitempty" jsonschema:"Optional GitHub sort: stars, forks, help-wanted-issues, or updated"`
	Order          string   `json:"order,omitempty" jsonschema:"Sort order: asc or desc"`
	Limit          int      `json:"limit,omitempty" jsonschema:"Results per page from 1 to 100"`
	Page           int      `json:"page,omitempty" jsonschema:"Result page within GitHub's 1,000-result cap"`
	ResponseFormat string   `json:"response_format,omitempty" jsonschema:"concise or detailed"`
}

// SuggestedAction describes a non-mandatory follow-up with reusable arguments.
type SuggestedAction struct {
	Tool      string         `json:"tool"`
	Reason    string         `json:"reason"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// SearchWarning explains a request-specific limitation and how to improve it.
type SearchWarning struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

// RepositorySearchMatch is a token-bounded live repository search result.
type RepositorySearchMatch struct {
	Ref           string                   `json:"ref"`
	Owner         string                   `json:"owner"`
	Repo          string                   `json:"repo"`
	Description   *string                  `json:"description,omitempty"`
	Language      *string                  `json:"language,omitempty"`
	Stars         *int                     `json:"stars,omitempty"`
	PushedAt      string                   `json:"pushed_at,omitempty"`
	Metadata      RepositoryMetadataOutput `json:"metadata"`
	DefaultBranch *string                  `json:"default_branch,omitempty"`
	License       *string                  `json:"license,omitempty"`
	Topics        []string                 `json:"topics,omitempty"`
	Watchers      *int                     `json:"watchers,omitempty"`
	Forks         *int                     `json:"forks,omitempty"`
	OpenIssues    *int                     `json:"open_issues,omitempty"`
	Archived      *bool                    `json:"archived,omitempty"`
	Fork          *bool                    `json:"fork,omitempty"`
}

// SearchGitHubRepositoriesOutput contains search results and completeness metadata.
type SearchGitHubRepositoriesOutput struct {
	Status           string                             `json:"status"`
	Query            string                             `json:"query"`
	Interpretation   string                             `json:"interpretation"`
	ResponseFormat   string                             `json:"response_format"`
	Page             int                                `json:"page"`
	NextPage         int                                `json:"next_page,omitempty"`
	Total            int                                `json:"total"`
	Incomplete       bool                               `json:"incomplete"`
	Items            []BatchItem[RepositorySearchMatch] `json:"items"`
	Warnings         []SearchWarning                    `json:"warnings,omitempty"`
	SuggestedActions []SuggestedAction                  `json:"suggested_actions,omitempty"`
}

// RepositoryRef identifies one GitHub repository without implying that it has
// been fetched or indexed locally.
type RepositoryRef struct {
	Owner string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo  string `json:"repo" jsonschema:"GitHub repository name"`
}

// ThreadRef identifies an exact issue or pull request. Kind may be omitted only
// for tools that intentionally resolve a number without a preselected kind.
type ThreadRef struct {
	Owner  string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo   string `json:"repo" jsonschema:"GitHub repository name"`
	Kind   string `json:"kind,omitempty" jsonschema:"Optional thread kind: issue or pull_request"`
	Number int    `json:"number" jsonschema:"Positive issue or pull request number"`
}

// BatchItem reports the outcome for one input-derived key while preserving
// input order. Value is present for complete items; recovery fields explain
// retryable, unavailable, or failed items without failing unrelated work.
type BatchItem[T any] struct {
	Key          string `json:"key"`
	Status       string `json:"status"`
	Value        *T     `json:"value,omitempty"`
	Reason       string `json:"reason,omitempty"`
	Message      string `json:"message,omitempty"`
	RetryAfterMS int    `json:"retry_after_ms,omitempty"`
	NextAction   string `json:"next_action,omitempty"`
}

// GetRepositoriesInput selects repositories for a bounded corpus read.
type GetRepositoriesInput struct {
	Repositories []RepositoryRef `json:"repositories" jsonschema:"One to 100 repository identities"`
}

// RepositoryMetadataOutput describes the coverage of repository metadata.
type RepositoryMetadataOutput struct {
	Status          string `json:"status"`
	ObservedAt      string `json:"observed_at,omitempty"`
	SourceUpdatedAt string `json:"source_updated_at,omitempty"`
	NextAction      string `json:"next_action,omitempty"`
}

// TypedRepositoryOutput contains repository facts with explicit metadata coverage.
type TypedRepositoryOutput struct {
	Ref           string                   `json:"ref"`
	Owner         string                   `json:"owner"`
	Repo          string                   `json:"repo"`
	Metadata      RepositoryMetadataOutput `json:"metadata"`
	Description   *string                  `json:"description"`
	DefaultBranch *string                  `json:"default_branch"`
	Language      *string                  `json:"language"`
	License       *string                  `json:"license"`
	Topics        []string                 `json:"topics,omitempty"`
	Stars         *int                     `json:"stars"`
	Watchers      *int                     `json:"watchers"`
	Forks         *int                     `json:"forks"`
	OpenIssues    *int                     `json:"open_issues"`
	Archived      *bool                    `json:"archived"`
	Fork          *bool                    `json:"fork"`
}

// GetRepositoriesOutput preserves repository input order and represents
// unobserved metadata with nullable facts instead of false zero values.
type GetRepositoriesOutput struct {
	Status string                             `json:"status"`
	Items  []BatchItem[TypedRepositoryOutput] `json:"items"`
}

// GetThreadsInput selects exact threads and the desired response view.
type GetThreadsInput struct {
	Threads []ThreadRef `json:"threads" jsonschema:"One to 100 exact thread identities"`
	View    string      `json:"view,omitempty" jsonschema:"compact or full; compact omits bodies"`
}

// GetThreadsOutput preserves exact-thread input order and item-level failures.
type GetThreadsOutput struct {
	Status string                    `json:"status"`
	Items  []BatchItem[ThreadOutput] `json:"items"`
}

// FindPrecedentsInput selects source threads for offline analogue discovery.
type FindPrecedentsInput struct {
	Threads []ThreadRef `json:"threads" jsonschema:"One to 20 source threads"`
	Limit   int         `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
}

// PrecedentOutput describes one stored thread analogous to a source thread.
type PrecedentOutput struct {
	Source      string                 `json:"source"`
	Ref         string                 `json:"ref"`
	Kind        string                 `json:"kind"`
	State       string                 `json:"state"`
	StateReason string                 `json:"state_reason,omitempty"`
	Title       string                 `json:"title"`
	Score       float64                `json:"score"`
	RuleVersion similarity.RuleVersion `json:"rule_version"`
	Reasons     []string               `json:"reasons"`
	ClosedAt    string                 `json:"closed_at,omitempty"`
	MergedAt    string                 `json:"merged_at,omitempty"`
}

// FindPrecedentsOutput returns stored closed or merged analogues for each
// source thread; it does not perform a network read.
type FindPrecedentsOutput struct {
	Status string                    `json:"status"`
	Items  []BatchItem[PrecedentSet] `json:"items"`
	Total  int                       `json:"total"`
}

// PrecedentSet reports both scored results and bounded candidate coverage.
type PrecedentSet struct {
	Matches    []PrecedentOutput `json:"matches" jsonschema:"Ranked precedent matches"`
	Population int               `json:"population" jsonschema:"All stored closed candidates"`
	Considered int               `json:"considered" jsonschema:"Candidates scored under the bound"`
	Truncated  bool              `json:"truncated" jsonschema:"Whether candidates or matches were omitted"`
}

// GetJobsInput selects durable jobs for a bounded status read.
type GetJobsInput struct {
	IDs            []string `json:"ids" jsonschema:"One to 100 durable job IDs"`
	ResponseFormat string   `json:"response_format,omitempty" jsonschema:"concise omits request and result payloads; detailed includes them"`
}

// GetJobsOutput reports multiple durable jobs in requested order so callers can
// poll concurrent work with one MCP round trip.
type GetJobsOutput struct {
	Status string                    `json:"status"`
	Items  []BatchItem[GetJobOutput] `json:"items"`
}

// SyncRepositoryMetadataInput selects repositories for asynchronous metadata refresh.
type SyncRepositoryMetadataInput struct {
	Repositories []RepositoryRef `json:"repositories" jsonschema:"One to 100 explicit repositories"`
}

// SyncThreadsInput selects either bounded repository-wide header discovery or
// exact thread refresh. It never requests child comments, reviews, or code.
type SyncThreadsInput struct {
	Selection          string          `json:"selection" jsonschema:"repositories or threads"`
	Repositories       []RepositoryRef `json:"repositories,omitempty" jsonschema:"One to 50 repositories in repository mode"`
	Threads            []ThreadRef     `json:"threads,omitempty" jsonschema:"One to 100 exact threads in thread mode"`
	Kind               string          `json:"kind,omitempty" jsonschema:"issue, pull_request, or both in repository mode"`
	State              string          `json:"state,omitempty" jsonschema:"open, closed, or all in repository mode"`
	UpdatedAfter       string          `json:"updated_after,omitempty" jsonschema:"Optional RFC 3339 lower bound in repository mode"`
	LimitPerRepository int             `json:"limit_per_repository,omitempty" jsonschema:"Maximum headers per repository from 1 to 1000"`
	MaxRequests        int             `json:"max_requests,omitempty" jsonschema:"Maximum total GitHub requests from 9 to 1000"`
}

// HydrateThreadsInput requests explicit child facets for already selected
// threads. Facets must be non-empty to prevent accidental broad hydration.
type HydrateThreadsInput struct {
	Threads  []ThreadRef `json:"threads" jsonschema:"One to 100 exact threads"`
	Facets   []string    `json:"facets" jsonschema:"One or more explicit child facets"`
	MaxPages int         `json:"max_pages,omitempty" jsonschema:"Maximum pages per facet from 1 to 100"`
}

// AuthenticatedIdentityOutput identifies the account associated with active credentials.
type AuthenticatedIdentityOutput struct {
	Login      string `json:"login"`
	ID         int64  `json:"id"`
	NodeID     string `json:"node_id,omitempty"`
	ObservedAt string `json:"observed_at"`
}

// SyncAuthoredPullRequestsInput bounds authored pull-request discovery and refresh.
type SyncAuthoredPullRequestsInput struct {
	State        string `json:"state,omitempty" jsonschema:"open, closed, or all"`
	UpdatedAfter string `json:"updated_after,omitempty" jsonschema:"Optional RFC 3339 lower bound"`
	Limit        int    `json:"limit,omitempty" jsonschema:"Maximum authored pull requests from 1 to 500"`
	MaxRequests  int    `json:"max_requests,omitempty" jsonschema:"Maximum total GitHub requests from 11 to 1000"`
}

// SyncPullRequestStatusInput selects pull requests and bounds review hydration.
type SyncPullRequestStatusInput struct {
	PullRequests []ThreadRef `json:"pull_requests" jsonschema:"One to 50 exact pull requests"`
	MaxPages     int         `json:"max_pages,omitempty" jsonschema:"Maximum review pages from 1 to 20"`
}

// ListPullRequestPortfolioInput filters and bounds the stored pull-request portfolio.
type ListPullRequestPortfolioInput struct {
	Author string `json:"author,omitempty" jsonschema:"Optional authored GitHub login"`
	State  string `json:"state,omitempty" jsonschema:"open, closed, or all"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum pull requests from 1 to 100"`
}

// PullRequestPortfolioItem contains source-backed PR facts and a deterministic
// portfolio.v1 attention classification. Missing status facets remain explicit
// in StatusCoverage and Reasons.
type PullRequestPortfolioItem struct {
	Ref                     string                `json:"ref"`
	Owner                   string                `json:"owner"`
	Repo                    string                `json:"repo"`
	Number                  int                   `json:"number"`
	Title                   string                `json:"title"`
	State                   string                `json:"state"`
	Author                  string                `json:"author"`
	Draft                   bool                  `json:"draft"`
	Attention               string                `json:"attention"`
	Reasons                 []string              `json:"reasons"`
	Mergeable               *bool                 `json:"mergeable,omitempty"`
	MergeStateStatus        string                `json:"merge_state_status,omitempty"`
	HeadRef                 string                `json:"head_ref,omitempty"`
	HeadSHA                 string                `json:"head_sha,omitempty"`
	BaseRef                 string                `json:"base_ref,omitempty"`
	BaseSHA                 string                `json:"base_sha,omitempty"`
	ReviewDecision          string                `json:"review_decision,omitempty"`
	ChecksStatus            string                `json:"checks_status,omitempty"`
	ChecksTotal             int                   `json:"checks_total,omitempty"`
	UnresolvedReviewThreads *int                  `json:"unresolved_review_threads,omitempty"`
	MergeQueueState         string                `json:"merge_queue_state,omitempty"`
	MergeQueuePosition      int                   `json:"merge_queue_position,omitempty"`
	ClosingIssues           []string              `json:"closing_issues,omitempty"`
	ChangedFiles            []string              `json:"changed_files,omitempty"`
	StatusCoverage          string                `json:"status_coverage"`
	Facets                  []FacetCoverageOutput `json:"facets"`
	SourceUpdatedAt         string                `json:"source_updated_at"`
	StatusObservedAt        string                `json:"status_observed_at,omitempty"`
}

// ListPullRequestPortfolioOutput contains a deterministic portfolio projection.
type ListPullRequestPortfolioOutput struct {
	Status       string                     `json:"status"`
	RuleVersion  string                     `json:"rule_version"`
	GeneratedAt  string                     `json:"generated_at"`
	PullRequests []PullRequestPortfolioItem `json:"pull_requests"`
	Total        int                        `json:"total"`
	Truncated    bool                       `json:"truncated"`
}

// PortfolioSubjectInput identifies local candidate state for offline overlap analysis.
type PortfolioSubjectInput struct {
	Kind string `json:"kind" jsonschema:"Candidate kind: opportunity, workspace, or pull_request"`
	Ref  string `json:"ref" jsonschema:"Local candidate ID or corpus pull-request thread ID"`
}

// FindPortfolioOverlapsInput compares candidates with exact stored authored PRs.
type FindPortfolioOverlapsInput struct {
	Candidates   []PortfolioSubjectInput `json:"candidates" jsonschema:"One to 50 local candidate subjects"`
	PullRequests []ThreadRef             `json:"pull_requests" jsonschema:"One to 100 exact authored pull requests"`
}

// PortfolioOverlapEvidenceOutput is one exact observed overlap reason.
type PortfolioOverlapEvidenceOutput struct {
	Kind       string   `json:"kind"`
	Value      string   `json:"value"`
	Score      float64  `json:"score,omitempty"`
	SourceRefs []string `json:"source_refs"`
}

// PortfolioOverlapMatchOutput associates overlap evidence with one authored PR.
type PortfolioOverlapMatchOutput struct {
	PullRequestThreadID int64                            `json:"pull_request_thread_id"`
	Evidence            []PortfolioOverlapEvidenceOutput `json:"evidence"`
}

// PortfolioOverlapOutput preserves explicit coverage and never infers no overlap.
type PortfolioOverlapOutput struct {
	Candidate PortfolioSubjectInput         `json:"candidate"`
	Status    string                        `json:"status"`
	Coverage  map[string]string             `json:"coverage"`
	Matches   []PortfolioOverlapMatchOutput `json:"matches"`
}

// FindPortfolioOverlapsOutput preserves candidate input order.
type FindPortfolioOverlapsOutput struct {
	Status string                              `json:"status"`
	Items  []BatchItem[PortfolioOverlapOutput] `json:"items"`
}

// LinkPullRequestInput explicitly associates a stored PR with local workflow state.
type LinkPullRequestInput struct {
	PullRequest   ThreadRef `json:"pull_request" jsonschema:"Exact stored pull request to link"`
	OpportunityID string    `json:"opportunity_id,omitempty" jsonschema:"Optional local opportunity ID"`
	WorkspaceID   string    `json:"workspace_id,omitempty" jsonschema:"Optional managed workspace ID"`
}

// LinkPullRequestOutput reports the idempotently stored local relationship.
type LinkPullRequestOutput struct {
	ID                  int64  `json:"id"`
	PullRequestThreadID int64  `json:"pull_request_thread_id"`
	OpportunityID       string `json:"opportunity_id,omitempty"`
	WorkspaceID         string `json:"workspace_id,omitempty"`
	CreatedAt           string `json:"created_at"`
}

// IndexRepositoryInput identifies one repository to acquire and index.
type IndexRepositoryInput struct {
	Owner  string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo   string `json:"repo" jsonschema:"GitHub repository name"`
	Remote string `json:"remote,omitempty" jsonschema:"Optional explicit credential-free Git remote"`
}

// IndexRepositoriesInput selects repositories for bounded asynchronous indexing.
type IndexRepositoriesInput struct {
	Repositories []IndexRepositoryInput `json:"repositories" jsonschema:"One to 10 repositories to acquire and index"`
}

// MergeConflictInput names two already-fetched revisions in a managed workspace.
type MergeConflictInput struct {
	WorkspaceID string `json:"workspace_id"`
	BaseOID     string `json:"base_oid"`
	HeadOID     string `json:"head_oid"`
}

// CheckMergeConflictsInput selects local revision comparisons.
type CheckMergeConflictsInput struct {
	Comparisons []MergeConflictInput `json:"comparisons" jsonschema:"One to 50 already-fetched revision comparisons"`
}

// MergeConflictOutput reports the result of one local revision comparison.
type MergeConflictOutput struct {
	WorkspaceID string `json:"workspace_id"`
	BaseOID     string `json:"base_oid"`
	HeadOID     string `json:"head_oid"`
	MergeBase   string `json:"merge_base,omitempty"`
	Conflicted  bool   `json:"conflicted"`
	Summary     string `json:"summary"`
}

// CheckMergeConflictsOutput preserves comparison order and isolates local Git
// failures to the affected comparison.
type CheckMergeConflictsOutput struct {
	Status string                           `json:"status"`
	Items  []BatchItem[MergeConflictOutput] `json:"items"`
}

// DeepWikiInput selects one bounded external derived-knowledge read. DeepWiki
// results are context, not authority for current GitHub state.
type DeepWikiInput struct {
	Action         string   `json:"action" jsonschema:"structure, contents, or question"`
	Repository     string   `json:"repository,omitempty" jsonschema:"OWNER/REPO for structure or contents"`
	Repositories   []string `json:"repositories,omitempty" jsonschema:"One to 10 OWNER/REPO values for question"`
	Question       string   `json:"question,omitempty" jsonschema:"Focused cross-repository question"`
	MaxOutputBytes int      `json:"max_output_bytes,omitempty" jsonschema:"Maximum returned bytes from 1024 to 1048576"`
}

// DeepWikiOutput labels provider prose as derived external content and reports
// provider-level unavailability without persisting the response.
type DeepWikiOutput struct {
	Status       string   `json:"status"`
	Provider     string   `json:"provider"`
	Action       string   `json:"action"`
	Repositories []string `json:"repositories"`
	Question     string   `json:"question,omitempty"`
	Result       string   `json:"result,omitempty"`
	SourceURL    string   `json:"source_url,omitempty"`
	RetrievedAt  string   `json:"retrieved_at"`
	Provenance   string   `json:"provenance"`
	Truncated    bool     `json:"truncated"`
	Reason       string   `json:"reason,omitempty"`
	NextAction   string   `json:"next_action,omitempty"`
}

// ErrNotFound lets readers distinguish absent corpus objects from failures.
var ErrNotFound = failure.NotFound(nil)
