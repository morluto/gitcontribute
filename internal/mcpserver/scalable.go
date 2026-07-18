package mcpserver

import (
	"context"
	"errors"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const serverInstructions = "Use GitContribute for durable, source-backed repository research and contribution tracking. Prefer corpus tools for offline reads. Use github.sync_repository_metadata for stars and repository facts, then github.sync_threads for issue or pull-request headers. Rank stored open issues before hydrating finalists with github.hydrate_threads. Use research.deepwiki for derived architecture, contribution, testing, and subsystem context; never treat it as authoritative for live GitHub state. Missing facets are unknown, not negative evidence. Batch tools preserve item-level failures so retry only retryable items. GitContribute never mutates GitHub."

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

type GetRepositoriesInput struct {
	Repositories []RepositoryRef `json:"repositories" jsonschema:"One to 100 repository identities"`
}

type RepositoryMetadataOutput struct {
	Status          string `json:"status"`
	ObservedAt      string `json:"observed_at,omitempty"`
	SourceUpdatedAt string `json:"source_updated_at,omitempty"`
	NextAction      string `json:"next_action,omitempty"`
}

type TypedRepositoryOutput struct {
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

type GetThreadsInput struct {
	Threads []ThreadRef `json:"threads" jsonschema:"One to 100 exact thread identities"`
	View    string      `json:"view,omitempty" jsonschema:"compact or full; compact omits bodies"`
}

// GetThreadsOutput preserves exact-thread input order and item-level failures.
type GetThreadsOutput struct {
	Status string                    `json:"status"`
	Items  []BatchItem[ThreadOutput] `json:"items"`
}

type RankOpportunitiesInput struct {
	Repositories            []RepositoryRef `json:"repositories" jsonschema:"One to 50 stored repositories"`
	Limit                   int             `json:"limit,omitempty" jsonschema:"Maximum total candidates from 1 to 100"`
	MaxResultsPerRepository int             `json:"max_results_per_repository,omitempty" jsonschema:"Maximum candidates per repository from 1 to 100"`
}

type OpportunityCandidateOutput struct {
	Rank               int      `json:"rank"`
	Ref                string   `json:"ref"`
	Repo               string   `json:"repo"`
	Number             int      `json:"number"`
	Title              string   `json:"title"`
	URL                string   `json:"url"`
	Score              int      `json:"score"`
	Eligibility        string   `json:"eligibility"`
	Confidence         string   `json:"confidence"`
	PositiveSignals    []string `json:"positive_signals,omitempty"`
	Risks              []string `json:"risks,omitempty"`
	Blockers           []string `json:"blockers,omitempty"`
	Unknowns           []string `json:"unknowns,omitempty"`
	LinkedPullRequests []int    `json:"linked_pull_requests,omitempty"`
	SourceUpdatedAt    string   `json:"source_updated_at,omitempty"`
}

type RepositoryOpportunitySummaryOutput struct {
	Repo            string `json:"repo"`
	TotalOpenIssues int    `json:"total_open_issues"`
	Considered      int    `json:"considered"`
}

// RankOpportunitiesOutput combines deterministic cross-repository ranking with
// per-repository coverage or availability results.
type RankOpportunitiesOutput struct {
	Status       string                                          `json:"status"`
	Candidates   []OpportunityCandidateOutput                    `json:"candidates"`
	Repositories []BatchItem[RepositoryOpportunitySummaryOutput] `json:"repositories"`
	GeneratedAt  string                                          `json:"generated_at"`
}

type FindPrecedentsInput struct {
	Threads []ThreadRef `json:"threads" jsonschema:"One to 20 source threads"`
	Limit   int         `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
}

type PrecedentOutput struct {
	Source      string   `json:"source"`
	Ref         string   `json:"ref"`
	Kind        string   `json:"kind"`
	State       string   `json:"state"`
	StateReason string   `json:"state_reason,omitempty"`
	Title       string   `json:"title"`
	Score       float64  `json:"score"`
	Reasons     []string `json:"reasons"`
	ClosedAt    string   `json:"closed_at,omitempty"`
	MergedAt    string   `json:"merged_at,omitempty"`
}

// FindPrecedentsOutput returns stored closed or merged analogues for each
// source thread; it does not perform a network read.
type FindPrecedentsOutput struct {
	Status string                         `json:"status"`
	Items  []BatchItem[[]PrecedentOutput] `json:"items"`
	Total  int                            `json:"total"`
}

type GetJobsInput struct {
	IDs []string `json:"ids" jsonschema:"One to 100 durable job IDs"`
}

// GetJobsOutput reports multiple durable jobs in requested order so callers can
// poll concurrent work with one MCP round trip.
type GetJobsOutput struct {
	Status string                    `json:"status"`
	Items  []BatchItem[GetJobOutput] `json:"items"`
}

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
}

// HydrateThreadsInput requests explicit child facets for already selected
// threads. Facets must be non-empty to prevent accidental broad hydration.
type HydrateThreadsInput struct {
	Threads  []ThreadRef `json:"threads" jsonschema:"One to 100 exact threads"`
	Facets   []string    `json:"facets" jsonschema:"One or more explicit child facets"`
	MaxPages int         `json:"max_pages,omitempty" jsonschema:"Maximum pages per facet from 1 to 100"`
}

type AuthenticatedIdentityOutput struct {
	Login      string `json:"login"`
	ID         int64  `json:"id"`
	NodeID     string `json:"node_id,omitempty"`
	ObservedAt string `json:"observed_at"`
}

type SyncAuthoredPullRequestsInput struct {
	State        string `json:"state,omitempty" jsonschema:"open, closed, or all"`
	UpdatedAfter string `json:"updated_after,omitempty" jsonschema:"Optional RFC 3339 lower bound"`
	Limit        int    `json:"limit,omitempty" jsonschema:"Maximum authored pull requests from 1 to 500"`
}

type SyncPullRequestStatusInput struct {
	PullRequests []ThreadRef `json:"pull_requests" jsonschema:"One to 50 exact pull requests"`
	MaxPages     int         `json:"max_pages,omitempty" jsonschema:"Maximum review pages from 1 to 20"`
}

type ListPullRequestPortfolioInput struct {
	Author string `json:"author,omitempty" jsonschema:"Optional authored GitHub login"`
	State  string `json:"state,omitempty" jsonschema:"open, closed, or all"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum pull requests from 1 to 100"`
}

// PullRequestPortfolioItem contains source-backed PR facts and a deterministic
// portfolio.v1 attention classification. Missing status facets remain explicit
// in StatusCoverage and Reasons.
type PullRequestPortfolioItem struct {
	Ref              string   `json:"ref"`
	Owner            string   `json:"owner"`
	Repo             string   `json:"repo"`
	Number           int      `json:"number"`
	Title            string   `json:"title"`
	State            string   `json:"state"`
	Author           string   `json:"author"`
	Draft            bool     `json:"draft"`
	Attention        string   `json:"attention"`
	Reasons          []string `json:"reasons"`
	Mergeable        *bool    `json:"mergeable,omitempty"`
	HeadRef          string   `json:"head_ref,omitempty"`
	HeadSHA          string   `json:"head_sha,omitempty"`
	BaseRef          string   `json:"base_ref,omitempty"`
	BaseSHA          string   `json:"base_sha,omitempty"`
	ReviewDecision   string   `json:"review_decision,omitempty"`
	StatusCoverage   string   `json:"status_coverage"`
	SourceUpdatedAt  string   `json:"source_updated_at"`
	StatusObservedAt string   `json:"status_observed_at,omitempty"`
}

type ListPullRequestPortfolioOutput struct {
	Status       string                     `json:"status"`
	RuleVersion  string                     `json:"rule_version"`
	GeneratedAt  string                     `json:"generated_at"`
	PullRequests []PullRequestPortfolioItem `json:"pull_requests"`
	Total        int                        `json:"total"`
}

type IndexRepositoryInput struct {
	Owner  string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo   string `json:"repo" jsonschema:"GitHub repository name"`
	Remote string `json:"remote,omitempty" jsonschema:"Optional explicit credential-free Git remote"`
}

type IndexRepositoriesInput struct {
	Repositories []IndexRepositoryInput `json:"repositories" jsonschema:"One to 10 repositories to acquire and index"`
}

// MergeConflictInput names two already-fetched revisions in a managed workspace.
type MergeConflictInput struct {
	WorkspaceID string `json:"workspace_id"`
	BaseOID     string `json:"base_oid"`
	HeadOID     string `json:"head_oid"`
}
type CheckMergeConflictsInput struct {
	Comparisons []MergeConflictInput `json:"comparisons" jsonschema:"One to 50 already-fetched revision comparisons"`
}
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

func (s *Server) registerScalable() {
	readOnly := readOnlyAnnotations()
	addCatalogTool(s.server, catalogTool[GetRepositoriesInput, GetRepositoriesOutput]{name: ToolGetRepositories, title: "Get stored repositories in one batch", description: "Read typed metadata and coverage for up to 100 stored repositories in input order. Missing metadata is returned as null with a sync next action; this offline tool never contacts GitHub.", annotations: readOnly, input: inputSchema[GetRepositoriesInput](func(sc *jsonschema.Schema) { setArrayBounds(sc, "repositories", 1, 100) }), output: outputSchema[GetRepositoriesOutput]("Ordered repository batch with item-level status."), handler: s.getRepositories})
	addCatalogTool(s.server, catalogTool[GetThreadsInput, GetThreadsOutput]{name: ToolGetThreads, title: "Get stored threads in one batch", description: "Read up to 100 exact stored issues or pull requests in input order. Choose compact for triage and full only for finalists; this tool is offline.", annotations: readOnly, input: inputSchema[GetThreadsInput](func(sc *jsonschema.Schema) {
		setArrayBounds(sc, "threads", 1, 100)
		setEnum(sc, "view", "compact", "full")
		setDefault(sc, "view", "compact")
	}), output: outputSchema[GetThreadsOutput]("Ordered stored-thread batch with item-level status."), handler: s.getThreads})
	addCatalogTool(s.server, catalogTool[RankOpportunitiesInput, RankOpportunitiesOutput]{name: ToolRankOpportunities, title: "Rank contribution opportunities across repositories", description: "Run Contribution Radar over up to 50 stored repositories and return one compact cross-repository ranking. Use after syncing open issue headers; missing coverage remains explicit unknown evidence.", annotations: readOnly, input: inputSchema[RankOpportunitiesInput](func(sc *jsonschema.Schema) {
		setArrayBounds(sc, "repositories", 1, 50)
		setRange(sc, "limit", 1, 100)
		setDefault(sc, "limit", 20)
		setRange(sc, "max_results_per_repository", 1, 100)
		setDefault(sc, "max_results_per_repository", 10)
	}), output: rankOpportunitiesOutputSchema(), handler: s.rankOpportunities})
	addCatalogTool(s.server, catalogTool[FindPrecedentsInput, FindPrecedentsOutput]{name: ToolFindPrecedents, title: "Find historical issue and pull-request precedents", description: "Find similar closed issues and pull requests for up to 20 source threads, including completed, not-planned, duplicate, and merged evidence. This is an offline historical read, not a current opportunity search.", annotations: readOnly, input: inputSchema[FindPrecedentsInput](func(sc *jsonschema.Schema) {
		setArrayBounds(sc, "threads", 1, 20)
		setRange(sc, "limit", 1, 100)
		setDefault(sc, "limit", 20)
	}), output: outputSchema[FindPrecedentsOutput]("Historical precedents grouped by source thread."), handler: s.findPrecedents})
	addCatalogTool(s.server, catalogTool[GetJobsInput, GetJobsOutput]{name: ToolGetJob, title: "Get durable jobs in one batch", description: "Read status, progress, result, and errors for one to 100 durable jobs in input order. Poll all related jobs together and retry only item-level retryable results.", annotations: readOnly, input: inputSchema[GetJobsInput](func(sc *jsonschema.Schema) {
		setArrayBounds(sc, "ids", 1, 100)
	}), output: outputSchema[GetJobsOutput]("Ordered durable-job states."), handler: s.getJobs})
	addCatalogTool(s.server, catalogTool[SyncRepositoryMetadataInput, JobReference]{name: ToolSyncRepositoryMetadata, title: "Sync repository metadata in one batch", description: "Start one durable GitHub read for metadata only for up to 100 explicit repositories. Use it for stars, language, archive state, and issue counts; it does not fetch threads or code.", annotations: networkReadAnnotations(), input: inputSchema[SyncRepositoryMetadataInput](func(sc *jsonschema.Schema) { setArrayBounds(sc, "repositories", 1, 100) }), output: outputSchema[JobReference]("Reference to a metadata synchronization job."), handler: s.syncRepositoryMetadata})
	addCatalogTool(s.server, catalogTool[SyncThreadsInput, JobReference]{name: ToolSyncThreads, title: "Sync GitHub thread headers in one batch", description: "Start one durable bounded read of issue or pull-request headers across up to 50 repositories or 100 exact threads. Choose exactly one selection mode; this does not fetch comments, reviews, checks, or code.", annotations: networkReadAnnotations(), input: inputSchema[SyncThreadsInput](func(sc *jsonschema.Schema) {
		setEnum(sc, "selection", "repositories", "threads")
		property(sc, "repositories").MaxItems = jsonschema.Ptr(50)
		property(sc, "threads").MaxItems = jsonschema.Ptr(100)
		setEnum(sc, "kind", "issue", "pull_request", "both")
		setEnum(sc, "state", "open", "closed", "all")
		setRange(sc, "limit_per_repository", 1, 1000)
		setDefault(sc, "limit_per_repository", 100)
		requireWhen(sc, "selection", "repositories", "repositories")
		requireWhen(sc, "selection", "threads", "threads")
		forbidWhen(sc, "selection", "repositories", "threads")
		forbidWhen(sc, "selection", "threads", "repositories", "kind", "state", "updated_after", "limit_per_repository")
	}), output: outputSchema[JobReference]("Reference to a bounded thread-header synchronization job."), handler: s.syncThreads})
	addCatalogTool(s.server, catalogTool[HydrateThreadsInput, JobReference]{name: ToolHydrateThreads, title: "Fetch GitHub comments and reviews for selected threads", description: "Fetch explicit comments, pull-request details, reviews, or review comments for up to 100 exact threads across repositories. An empty facet list is rejected; hydrate only finalists after ranking.", annotations: networkReadAnnotations(), input: inputSchema[HydrateThreadsInput](func(sc *jsonschema.Schema) {
		setArrayBounds(sc, "threads", 1, 100)
		setArrayBounds(sc, "facets", 1, 8)
		setArrayEnum(sc, "facets", "issue_comments", "pr_details", "pr_reviews", "pr_review_comments")
		setRange(sc, "max_pages", 1, 100)
		setDefault(sc, "max_pages", 3)
	}), output: outputSchema[JobReference]("Reference to a bounded exact-thread hydration job."), handler: s.hydrateThreads})
	addCatalogTool(s.server, catalogTool[struct{}, AuthenticatedIdentityOutput]{name: ToolGetAuthenticatedIdentity, title: "Get authenticated GitHub identity", description: "Resolve the current read credential's GitHub login and stable ID. Use before authored pull-request discovery; this performs one external read and never mutates GitHub.", annotations: networkReadAnnotations(), input: inputSchema[struct{}](noSchemaCustomization), output: outputSchema[AuthenticatedIdentityOutput]("Authenticated GitHub identity."), handler: s.getAuthenticatedIdentity})
	addCatalogTool(s.server, catalogTool[SyncAuthoredPullRequestsInput, JobReference]{name: ToolSyncAuthoredPullRequests, title: "Sync authored pull requests across GitHub", description: "Discover and persist up to 500 pull requests authored by the authenticated GitHub user across repositories. This reads only core thread state; use the dedicated exact-PR health tool afterward.", annotations: networkReadAnnotations(), input: inputSchema[SyncAuthoredPullRequestsInput](func(sc *jsonschema.Schema) {
		setEnum(sc, "state", "open", "closed", "all")
		setRange(sc, "limit", 1, 500)
		setDefault(sc, "limit", 500)
	}), output: outputSchema[JobReference]("Reference to an authored pull-request synchronization job."), handler: s.syncAuthoredPullRequests})
	addCatalogTool(s.server, catalogTool[SyncPullRequestStatusInput, JobReference]{name: ToolSyncPullRequestStatus, title: "Sync exact PR status", description: "Refresh mergeability, head/base revisions, and reviews for up to 50 exact selected pull requests. Checks and unresolved review-thread coverage remain explicitly unknown until their adapters are available.", annotations: networkReadAnnotations(), input: inputSchema[SyncPullRequestStatusInput](func(sc *jsonschema.Schema) {
		setArrayBounds(sc, "pull_requests", 1, 50)
		setRange(sc, "max_pages", 1, 20)
		setDefault(sc, "max_pages", 3)
	}), output: outputSchema[JobReference]("Reference to a pull-request status synchronization job."), handler: s.syncPullRequestStatus})
	addCatalogTool(s.server, catalogTool[ListPullRequestPortfolioInput, ListPullRequestPortfolioOutput]{name: ToolListPullRequestPortfolio, title: "List pull requests that need contributor attention", description: "List stored authored pull requests with deterministic attention states from mergeability, reviews, lifecycle, and freshness. This offline read reports missing status facets as unknown; sync authored PRs and status explicitly when stale.", annotations: readOnly, input: inputSchema[ListPullRequestPortfolioInput](func(sc *jsonschema.Schema) {
		setEnum(sc, "state", "open", "closed", "all")
		setRange(sc, "limit", 1, 100)
		setDefault(sc, "limit", 100)
	}), output: outputSchema[ListPullRequestPortfolioOutput]("Offline pull-request portfolio with explainable attention states."), handler: s.listPullRequestPortfolio})
	addCatalogTool(s.server, catalogTool[IndexRepositoriesInput, JobReference]{name: ToolIndexRepositories, title: "Acquire and index repository code in one batch", description: "Start a durable low-concurrency Git acquisition and safe code indexing job for up to 10 repositories. This performs network reads, Git processes, and local writes, but disables hooks and never executes repository-controlled code.", annotations: networkReadAnnotations(), input: inputSchema[IndexRepositoriesInput](func(sc *jsonschema.Schema) { setArrayBounds(sc, "repositories", 1, 10) }), output: outputSchema[JobReference]("Reference to a bounded repository acquisition and indexing job."), handler: s.indexRepositories})
	addCatalogTool(s.server, catalogTool[CheckMergeConflictsInput, CheckMergeConflictsOutput]{name: ToolCheckMergeConflicts, title: "Check local Git merge conflicts in one batch", description: "Compare up to 50 already-fetched base/head OID pairs in managed workspaces using non-mutating Git reads. This never fetches remotes or changes refs, indexes, or worktrees; use it for actual merge conflicts, not competing upstream work.", annotations: processReadAnnotations(), input: inputSchema[CheckMergeConflictsInput](func(sc *jsonschema.Schema) { setArrayBounds(sc, "comparisons", 1, 50) }), output: outputSchema[CheckMergeConflictsOutput]("Ordered local merge-conflict checks."), handler: s.checkMergeConflicts})
	addCatalogTool(s.server, catalogTool[DeepWikiInput, DeepWikiOutput]{name: ToolDeepWiki, title: "Read derived repository knowledge from DeepWiki", description: "Use DeepWiki for public repository architecture, contribution rules, testing, and subsystem context. Actions map to its public structure, contents, and question tools. Do not use this for live stars, thread state, checks, reviews, or mergeability.", annotations: networkReadAnnotations(), input: inputSchema[DeepWikiInput](func(sc *jsonschema.Schema) {
		setEnum(sc, "action", "structure", "contents", "question")
		setArrayBounds(sc, "repositories", 1, 10)
		setRange(sc, "max_output_bytes", 1024, 1048576)
		setDefault(sc, "max_output_bytes", 131072)
		requireWhen(sc, "action", "structure", "repository")
		requireWhen(sc, "action", "contents", "repository")
		requireWhen(sc, "action", "question", "repositories", "question")
	}), output: outputSchema[DeepWikiOutput]("Derived DeepWiki response with provenance."), handler: s.deepWiki})
}

func (s *Server) scalableReader() (ScalableReader, error) {
	r, ok := s.reader.(ScalableReader)
	if !ok {
		return nil, errors.New("bounded batch reads are not available")
	}
	return r, nil
}
func (s *Server) getRepositories(ctx context.Context, _ *mcp.CallToolRequest, in GetRepositoriesInput) (*mcp.CallToolResult, GetRepositoriesOutput, error) {
	r, err := s.scalableReader()
	if err != nil {
		return nil, GetRepositoriesOutput{}, err
	}
	out, err := r.GetRepositories(ctx, in)
	return nil, out, err
}
func (s *Server) getThreads(ctx context.Context, _ *mcp.CallToolRequest, in GetThreadsInput) (*mcp.CallToolResult, GetThreadsOutput, error) {
	if in.View == "" {
		in.View = "compact"
	}
	r, err := s.scalableReader()
	if err != nil {
		return nil, GetThreadsOutput{}, err
	}
	out, err := r.GetThreads(ctx, in)
	return nil, out, err
}
func (s *Server) rankOpportunities(ctx context.Context, _ *mcp.CallToolRequest, in RankOpportunitiesInput) (*mcp.CallToolResult, RankOpportunitiesOutput, error) {
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.MaxResultsPerRepository == 0 {
		in.MaxResultsPerRepository = 10
	}
	r, err := s.scalableReader()
	if err != nil {
		return nil, RankOpportunitiesOutput{}, err
	}
	out, err := r.RankOpportunities(ctx, in)
	return nil, out, err
}
func (s *Server) findPrecedents(ctx context.Context, _ *mcp.CallToolRequest, in FindPrecedentsInput) (*mcp.CallToolResult, FindPrecedentsOutput, error) {
	if in.Limit == 0 {
		in.Limit = 20
	}
	r, err := s.scalableReader()
	if err != nil {
		return nil, FindPrecedentsOutput{}, err
	}
	out, err := r.FindPrecedents(ctx, in)
	return nil, out, err
}
func (s *Server) getJobs(ctx context.Context, _ *mcp.CallToolRequest, in GetJobsInput) (*mcp.CallToolResult, GetJobsOutput, error) {
	if _, ok := s.reader.(ScalableReader); !ok {
		out := GetJobsOutput{Status: "complete", Items: make([]BatchItem[GetJobOutput], len(in.IDs))}
		for i, id := range in.IDs {
			item := BatchItem[GetJobOutput]{Key: id, Status: "complete"}
			job, err := s.reader.GetJob(ctx, GetJobInput{ID: id})
			if err != nil {
				item.Status, item.Reason, item.Message = "unavailable", "not_found", err.Error()
				out.Status = "partial"
			} else {
				item.Value = &job
			}
			out.Items[i] = item
		}
		return nil, out, nil
	}
	r, err := s.scalableReader()
	if err != nil {
		return nil, GetJobsOutput{}, err
	}
	out, err := r.GetJobs(ctx, in)
	return nil, out, err
}
func (s *Server) syncRepositoryMetadata(ctx context.Context, _ *mcp.CallToolRequest, in SyncRepositoryMetadataInput) (*mcp.CallToolResult, JobReference, error) {
	op, ok := s.reader.(ScalableOperator)
	if !ok {
		return nil, JobReference{}, errors.New("repository metadata sync is not available")
	}
	out, err := op.SyncRepositoryMetadata(ctx, in)
	return nil, out, err
}
func (s *Server) syncThreads(ctx context.Context, _ *mcp.CallToolRequest, in SyncThreadsInput) (*mcp.CallToolResult, JobReference, error) {
	if in.Selection == "repositories" && len(in.Repositories) == 0 {
		return nil, JobReference{}, errors.New("repositories are required in repository selection mode")
	}
	if in.Selection == "threads" && len(in.Threads) == 0 {
		return nil, JobReference{}, errors.New("threads are required in thread selection mode")
	}
	op, ok := s.reader.(ScalableOperator)
	if !ok {
		return nil, JobReference{}, errors.New("batch thread sync is not available")
	}
	out, err := op.SyncThreads(ctx, in)
	return nil, out, err
}
func (s *Server) hydrateThreads(ctx context.Context, _ *mcp.CallToolRequest, in HydrateThreadsInput) (*mcp.CallToolResult, JobReference, error) {
	if len(in.Threads) == 0 || len(in.Facets) == 0 {
		return nil, JobReference{}, errors.New("threads and at least one facet are required")
	}
	if in.MaxPages == 0 {
		in.MaxPages = 3
	}
	op, ok := s.reader.(ScalableOperator)
	if !ok {
		return nil, JobReference{}, errors.New("batch thread hydration is not available")
	}
	out, err := op.HydrateThreads(ctx, in)
	return nil, out, err
}
func (s *Server) getAuthenticatedIdentity(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, AuthenticatedIdentityOutput, error) {
	op, ok := s.reader.(ScalableOperator)
	if !ok {
		return nil, AuthenticatedIdentityOutput{}, errors.New("GitHub identity lookup is not available")
	}
	out, err := op.GetAuthenticatedIdentity(ctx)
	return nil, out, err
}
func (s *Server) syncAuthoredPullRequests(ctx context.Context, _ *mcp.CallToolRequest, in SyncAuthoredPullRequestsInput) (*mcp.CallToolResult, JobReference, error) {
	if in.State == "" {
		in.State = "open"
	}
	if in.Limit == 0 {
		in.Limit = 500
	}
	op, ok := s.reader.(ScalableOperator)
	if !ok {
		return nil, JobReference{}, errors.New("authored pull-request sync is not available")
	}
	out, err := op.SyncAuthoredPullRequests(ctx, in)
	return nil, out, err
}
func (s *Server) syncPullRequestStatus(ctx context.Context, _ *mcp.CallToolRequest, in SyncPullRequestStatusInput) (*mcp.CallToolResult, JobReference, error) {
	if len(in.PullRequests) == 0 {
		return nil, JobReference{}, errors.New("pull_requests are required")
	}
	if in.MaxPages == 0 {
		in.MaxPages = 3
	}
	op, ok := s.reader.(ScalableOperator)
	if !ok {
		return nil, JobReference{}, errors.New("pull-request status sync is not available")
	}
	out, err := op.SyncPullRequestStatus(ctx, in)
	return nil, out, err
}
func (s *Server) listPullRequestPortfolio(ctx context.Context, _ *mcp.CallToolRequest, in ListPullRequestPortfolioInput) (*mcp.CallToolResult, ListPullRequestPortfolioOutput, error) {
	if in.State == "" {
		in.State = "open"
	}
	if in.Limit == 0 {
		in.Limit = 100
	}
	r, err := s.scalableReader()
	if err != nil {
		return nil, ListPullRequestPortfolioOutput{}, err
	}
	out, err := r.ListPullRequestPortfolio(ctx, in)
	return nil, out, err
}
func (s *Server) indexRepositories(ctx context.Context, _ *mcp.CallToolRequest, in IndexRepositoriesInput) (*mcp.CallToolResult, JobReference, error) {
	if len(in.Repositories) == 0 {
		return nil, JobReference{}, errors.New("repositories are required")
	}
	op, ok := s.reader.(ScalableOperator)
	if !ok {
		return nil, JobReference{}, errors.New("batch code indexing is not available")
	}
	out, err := op.IndexRepositories(ctx, in)
	return nil, out, err
}
func (s *Server) checkMergeConflicts(ctx context.Context, _ *mcp.CallToolRequest, in CheckMergeConflictsInput) (*mcp.CallToolResult, CheckMergeConflictsOutput, error) {
	if len(in.Comparisons) == 0 {
		return nil, CheckMergeConflictsOutput{}, errors.New("comparisons are required")
	}
	op, ok := s.reader.(ScalableOperator)
	if !ok {
		return nil, CheckMergeConflictsOutput{}, errors.New("local merge-conflict checks are not available")
	}
	out, err := op.CheckMergeConflicts(ctx, in)
	return nil, out, err
}
func (s *Server) deepWiki(ctx context.Context, _ *mcp.CallToolRequest, in DeepWikiInput) (*mcp.CallToolResult, DeepWikiOutput, error) {
	in.Action = strings.TrimSpace(in.Action)
	if in.MaxOutputBytes == 0 {
		in.MaxOutputBytes = 131072
	}
	if in.MaxOutputBytes < 1024 || in.MaxOutputBytes > 1048576 {
		return nil, DeepWikiOutput{}, errors.New("max_output_bytes must be between 1024 and 1048576")
	}
	if (in.Action == "structure" || in.Action == "contents") && strings.TrimSpace(in.Repository) == "" {
		return nil, DeepWikiOutput{}, errors.New("repository is required for structure and contents")
	}
	if in.Action == "question" && (len(in.Repositories) == 0 || strings.TrimSpace(in.Question) == "") {
		return nil, DeepWikiOutput{}, errors.New("repositories and question are required for question")
	}
	op, ok := s.reader.(ScalableOperator)
	if !ok {
		return nil, DeepWikiOutput{}, errors.New("DeepWiki is not available")
	}
	out, err := op.DeepWiki(ctx, in)
	return nil, out, err
}

func setArrayBounds(schema *jsonschema.Schema, name string, min, max int) {
	p := property(schema, name)
	p.MinItems = jsonschema.Ptr(min)
	p.MaxItems = jsonschema.Ptr(max)
}

func rankOpportunitiesOutputSchema() *jsonschema.Schema {
	schema := outputSchema[RankOpportunitiesOutput]("Cross-repository opportunity ranking.")
	setOutputPropertyRange(schema, "score", 0, 100)
	return schema
}

func setOutputPropertyRange(schema *jsonschema.Schema, name string, minimum, maximum float64) {
	if schema == nil {
		return
	}
	if field := schema.Properties[name]; field != nil {
		field.Minimum = jsonschema.Ptr(minimum)
		field.Maximum = jsonschema.Ptr(maximum)
	}
	for _, field := range schema.Properties {
		setOutputPropertyRange(field, name, minimum, maximum)
	}
	setOutputPropertyRange(schema.Items, name, minimum, maximum)
	setOutputPropertyRange(schema.AdditionalProperties, name, minimum, maximum)
	for _, definition := range schema.Defs {
		setOutputPropertyRange(definition, name, minimum, maximum)
	}
}
