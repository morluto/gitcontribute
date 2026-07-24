package mcpserver

import (
	"context"
	"errors"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

const serverInstructions = "Use advertised GitContribute tools for durable, source-backed repository research and contribution tracking. " +
	"Prefer corpus tools for offline reads; they never refresh data implicitly. " +
	"GitHub tools perform explicit network reads and may update only the local corpus. " +
	"Research tools return derived external context, never live GitHub state. " +
	"When an operation returns a job, poll advertised job tools in batches. " +
	"Missing or truncated coverage is unknown, not negative evidence; retry only retryable batch items. " +
	"Only advertised tools are available. GitContribute never mutates GitHub."

// RepositoryRef identifies one GitHub repository without implying that it has
// been fetched or indexed locally.

// ThreadRef identifies an exact issue or pull request. Kind may be omitted only
// for tools that intentionally resolve a number without a preselected kind.

// BatchItem reports the outcome for one input-derived key while preserving
// input order. Value is present for complete items; recovery fields explain
// retryable, unavailable, or failed items without failing unrelated work.

// GetRepositoriesInput selects repositories for a bounded corpus read.

// RepositoryMetadataOutput describes the coverage of repository metadata.

// TypedRepositoryOutput contains repository facts with explicit metadata coverage.

// GetRepositoriesOutput preserves repository input order and represents
// unobserved metadata with nullable facts instead of false zero values.

// GetThreadsInput selects exact threads and the desired response view.

// GetThreadsOutput preserves exact-thread input order and item-level failures.

// FindPrecedentsInput selects source threads for offline analogue discovery.

// PrecedentOutput describes one stored thread analogous to a source thread.

// FindPrecedentsOutput returns stored closed or merged analogues for each
// source thread; it does not perform a network read.

// PrecedentSet reports both scored results and bounded candidate coverage.

// GetJobsInput selects durable jobs for a bounded status read.

// GetJobsOutput reports multiple durable jobs in requested order so callers can
// poll concurrent work with one MCP round trip.

// SyncRepositoryMetadataInput selects repositories for asynchronous metadata refresh.

// SyncThreadsInput selects either bounded repository-wide header discovery or
// exact thread refresh. It never requests child comments, reviews, or code.

// HydrateThreadsInput requests explicit child facets for already selected
// threads. Facets must be non-empty to prevent accidental broad hydration.

// AuthenticatedIdentityOutput identifies the account associated with active credentials.

// SyncAuthoredPullRequestsInput bounds authored pull-request discovery and refresh.

// SyncPullRequestStatusInput selects pull requests and bounds review hydration.

// ListPullRequestPortfolioInput filters and bounds the stored pull-request portfolio.

// PullRequestPortfolioItem contains source-backed PR facts and a deterministic
// portfolio.v1 attention classification. Missing status facets remain explicit
// in StatusCoverage and Reasons.

// ListPullRequestPortfolioOutput contains a deterministic portfolio projection.

// PortfolioSubjectInput identifies local candidate state for offline overlap analysis.

// FindPortfolioOverlapsInput compares candidates with exact stored authored PRs.

// PortfolioOverlapEvidenceOutput is one exact observed overlap reason.

// PortfolioOverlapMatchOutput associates overlap evidence with one authored PR.

// PortfolioOverlapOutput preserves explicit coverage and never infers no overlap.

// FindPortfolioOverlapsOutput preserves candidate input order.

// LinkPullRequestInput explicitly associates a stored PR with local workflow state.

// LinkPullRequestOutput reports the idempotently stored local relationship.

// IndexRepositoryInput identifies one repository to acquire and index.

// IndexRepositoriesInput selects repositories for bounded asynchronous indexing.

// MergeConflictInput names two already-fetched revisions in a managed workspace.

// CheckMergeConflictsInput selects local revision comparisons.

// MergeConflictOutput reports the result of one local revision comparison.

// CheckMergeConflictsOutput preserves comparison order and isolates local Git
// failures to the affected comparison.

// DeepWikiInput selects one bounded external derived-knowledge read. DeepWiki
// results are context, not authority for current GitHub state.

// DeepWikiOutput labels provider prose as derived external content and reports
// provider-level unavailability without persisting the response.

func (s *Server) registerScalable() {
	readOnly := readOnlyAnnotations()
	addCatalogTool(s, catalogTool[GetRepositoriesInput, GetRepositoriesOutput]{name: ToolGetRepositories, title: "Get stored repositories in one batch", description: "Read typed metadata and coverage for up to 100 stored repositories in input order. Missing metadata is returned as null with a sync next action; this offline tool never contacts GitHub.", annotations: readOnly, supportedBy: supports[ScalableReader], input: inputSchema[GetRepositoriesInput](func(sc *schemaBuilder) { setArrayBounds(sc, "repositories", 1, 100) }), output: outputSchema[GetRepositoriesOutput]("Ordered repository batch with item-level status."), handler: s.getRepositories})
	addCatalogTool(s, catalogTool[GetThreadsInput, GetThreadsOutput]{name: ToolGetThreads, title: "Get stored threads in one batch", description: "Read up to 100 exact stored issues or pull requests in input order. Choose compact for triage and full only for finalists; this tool is offline.", annotations: readOnly, supportedBy: supports[ScalableReader], input: inputSchema[GetThreadsInput](func(sc *schemaBuilder) {
		setArrayBounds(sc, "threads", 1, 100)
		setEnum(sc, "view", "compact", "full")
		setDefault(sc, "view", "compact")
	}), output: outputSchema[GetThreadsOutput]("Ordered stored-thread batch with item-level status."), handler: s.getThreads})
	addCatalogTool(s, catalogTool[RankOpportunitiesInput, RankOpportunitiesOutput]{name: ToolRankThreads, title: "Rank stored threads for contribution", description: "Rank open issues across 1-50 required stored repositories. This bounded offline result reports truncation and never persists opportunities.", annotations: readOnly, supportedBy: supports[ScalableReader], input: inputSchema[RankOpportunitiesInput](func(sc *schemaBuilder) {
		setArrayBounds(sc, "repositories", 1, 50)
		setRange(sc, "limit", 1, 100)
		setDefault(sc, "limit", 20)
		setRange(sc, "max_results_per_repository", 1, 100)
		setDefault(sc, "max_results_per_repository", 10)
	}), output: rankOpportunitiesOutputSchema(), handler: s.rankOpportunities})
	addCatalogTool(s, catalogTool[FindPrecedentsInput, FindPrecedentsOutput]{name: ToolFindPrecedents, title: "Find historical issue and pull-request precedents", description: "Find similar closed issues and pull requests for up to 20 source threads, including completed, not-planned, duplicate, and merged evidence. This is an offline historical read, not a current opportunity search.", annotations: readOnly, supportedBy: supports[ScalableReader], input: inputSchema[FindPrecedentsInput](func(sc *schemaBuilder) {
		setArrayBounds(sc, "threads", 1, 20)
		setRange(sc, "limit", 1, 100)
		setDefault(sc, "limit", 20)
	}), output: outputSchema[FindPrecedentsOutput]("Historical precedents grouped by source thread."), handler: s.findPrecedents})
	addCatalogTool(s, catalogTool[GetJobsInput, GetJobsOutput]{name: ToolGetJob, title: "Get durable jobs in one batch", description: "Poll up to 100 jobs. Use detailed only for a terminal finalist.", annotations: readOnly, input: inputSchema[GetJobsInput](func(sc *schemaBuilder) {
		setArrayBounds(sc, "ids", 1, 100)
		setEnum(sc, "response_format", "concise", "detailed")
		setDefault(sc, "response_format", "concise")
	}), output: outputSchema[GetJobsOutput]("Ordered durable-job states."), handler: s.getJobs})
	addCatalogTool(s, catalogTool[SearchGitHubRepositoriesInput, SearchGitHubRepositoriesOutput]{name: ToolSearchGitHubRepositories, title: "Search live GitHub repositories", description: "Find repositories with structured filters and persist metadata. Use raw_query for unsupported GitHub qualifiers. Does not fetch threads or code.", annotations: networkReadAnnotations(), supportedBy: supports[GitHubOperator], input: inputSchema[SearchGitHubRepositoriesInput](func(sc *schemaBuilder) {
		setArrayBounds(sc, "match_fields", 1, 3)
		setArrayEnum(sc, "match_fields", "name", "description", "readme")
		setArrayBounds(sc, "topics", 1, 10)
		setMinimum(sc, "stars_min", 0)
		setMinimum(sc, "stars_max", 0)
		setEnum(sc, "sort", "stars", "forks", "help-wanted-issues", "updated")
		setEnum(sc, "order", "asc", "desc")
		setRange(sc, "limit", 1, 100)
		setDefault(sc, "limit", 20)
		setRange(sc, "page", 1, 1000)
		setDefault(sc, "page", 1)
		setEnum(sc, "response_format", "concise", "detailed")
		setDefault(sc, "response_format", "concise")
	}), output: outputSchema[SearchGitHubRepositoriesOutput]("Live repository search with persisted metadata."), handler: s.searchGitHubRepositories})
	addCatalogTool(s, catalogTool[SyncRepositoryMetadataInput, JobReference]{name: ToolSyncRepositoryMetadata, title: "Sync repository metadata in one batch", description: "Fetch current stars and metadata for up to 100 explicit repositories; no threads or code.", annotations: networkReadAnnotations(), supportedBy: supports[GitHubOperator], input: inputSchema[SyncRepositoryMetadataInput](func(sc *schemaBuilder) { setArrayBounds(sc, "repositories", 1, 100) }), output: outputSchema[JobReference]("Reference to a metadata synchronization job."), handler: s.syncRepositoryMetadata})
	addCatalogTool(s, catalogTool[SyncThreadsInput, JobReference]{name: ToolSyncThreads, title: "Sync GitHub thread headers in one batch", description: "Sync GitHub issue and pull-request headers across selected repositories or exact threads, plus metadata and policy files; no discussions, reviews, checks, or code.", annotations: networkReadAnnotations(), supportedBy: supports[GitHubOperator], input: inputSchema[SyncThreadsInput](func(sc *schemaBuilder) {
		setEnum(sc, "selection", "repositories", "threads")
		property(sc, "repositories").MaxItems = jsonschema.Ptr(50)
		property(sc, "threads").MaxItems = jsonschema.Ptr(100)
		setEnum(sc, "kind", "issue", "pull_request", "both")
		setEnum(sc, "state", "open", "closed", "all")
		setRange(sc, "limit_per_repository", 1, 1000)
		setRange(sc, "max_requests", 9, 1000)
		setDefault(sc, "max_requests", 1000)
	}), output: outputSchema[JobReference]("Reference to a bounded thread-header synchronization job."), handler: s.syncThreads})
	addCatalogTool(s, catalogTool[HydrateThreadsInput, JobReference]{name: ToolHydrateThreads, title: "Fetch selected GitHub thread facets", description: "Fetch explicit comments, issue timelines, pull-request details, reviews, or review comments for up to 100 exact threads. Timeline history is opt-in; hydrate only finalists after ranking.", annotations: networkReadAnnotations(), supportedBy: supports[GitHubOperator], input: inputSchema[HydrateThreadsInput](func(sc *schemaBuilder) {
		setArrayBounds(sc, "threads", 1, 100)
		setArrayBounds(sc, "facets", 1, 5)
		setArrayEnum(sc, "facets", "issue_comments", "issue_timeline", "pr_details", "pr_reviews", "pr_review_comments")
		setRange(sc, "max_pages", 1, 100)
		setDefault(sc, "max_pages", 3)
	}), output: outputSchema[JobReference]("Reference to a bounded exact-thread hydration job."), handler: s.hydrateThreads})
	addCatalogTool(s, catalogTool[struct{}, AuthenticatedIdentityOutput]{name: ToolGetAuthenticatedIdentity, title: "Get authenticated GitHub identity", description: "Resolve the current read credential's GitHub login and stable ID before authored-PR discovery.", annotations: externalReadAnnotations(), supportedBy: supports[GitHubOperator], input: inputSchema[struct{}](noSchemaCustomization), output: outputSchema[AuthenticatedIdentityOutput]("Authenticated GitHub identity."), handler: s.getAuthenticatedIdentity})
	addCatalogTool(s, catalogTool[SyncAuthoredPullRequestsInput, JobReference]{name: ToolSyncAuthoredPullRequests, title: "Sync authored pull requests across GitHub", description: "Discover and persist up to 500 pull requests authored by the authenticated GitHub user across repositories. This reads only core thread state; use the dedicated exact-PR health tool afterward.", annotations: networkReadAnnotations(), supportedBy: supports[GitHubOperator], input: inputSchema[SyncAuthoredPullRequestsInput](func(sc *schemaBuilder) {
		setEnum(sc, "state", "open", "closed", "all")
		setRange(sc, "limit", 1, 500)
		setDefault(sc, "limit", 500)
		setRange(sc, "max_requests", 11, 1000)
		setDefault(sc, "max_requests", 1000)
	}), output: outputSchema[JobReference]("Reference to an authored pull-request synchronization job."), handler: s.syncAuthoredPullRequests})
	addCatalogTool(s, catalogTool[SyncPullRequestStatusInput, JobReference]{name: ToolSyncPullRequestStatus, title: "Sync exact PR health", description: "Refresh mergeability, reviews, checks, unresolved conversations, merge state, merge queue, closing issues, and changed files for up to 50 exact pull requests. Returns independent facet completeness; retry only incomplete items.", annotations: networkReadAnnotations(), supportedBy: supports[GitHubOperator], input: inputSchema[SyncPullRequestStatusInput](func(sc *schemaBuilder) {
		setArrayBounds(sc, "pull_requests", 1, 50)
		setRange(sc, "max_pages", 1, 20)
		setDefault(sc, "max_pages", 3)
	}), output: outputSchema[JobReference]("Reference to a pull-request status synchronization job."), handler: s.syncPullRequestStatus})
	addCatalogTool(s, catalogTool[ListPullRequestPortfolioInput, ListPullRequestPortfolioOutput]{name: ToolListPullRequestPortfolio, title: "List pull requests that need contributor attention", description: "List stored authored pull requests with deterministic attention from lifecycle, checks, review conversations, merge state, queue, and freshness. This offline read reports incomplete facets as unknown; sync authored PRs and health when stale.", annotations: readOnly, supportedBy: supports[PortfolioReader], input: inputSchema[ListPullRequestPortfolioInput](func(sc *schemaBuilder) {
		setEnum(sc, "state", "open", "closed", "all")
		setRange(sc, "limit", 1, 100)
		setDefault(sc, "limit", 100)
	}), output: outputSchema[ListPullRequestPortfolioOutput]("Offline pull-request portfolio with explainable attention states."), handler: s.listPullRequestPortfolio})
	addCatalogTool(s, catalogTool[FindPortfolioOverlapsInput, FindPortfolioOverlapsOutput]{name: ToolFindPortfolioOverlaps, title: "Find overlaps with authored pull requests", description: "Compare up to 50 local candidates with 100 stored authored pull requests using complete changed-path, linked-issue, and opportunity-similarity observations. This offline read returns unknown instead of claiming no overlap when coverage is missing.", annotations: readOnly, supportedBy: supports[PortfolioReader], input: inputSchema[FindPortfolioOverlapsInput](func(sc *schemaBuilder) {
		setArrayBounds(sc, "candidates", 1, 50)
		setArrayBounds(sc, "pull_requests", 1, 100)
		if candidate := sc.schema.Defs["PortfolioSubjectInput"]; candidate != nil {
			setEnum(&schemaBuilder{schema: candidate, err: sc.err}, "kind", "opportunity", "workspace", "pull_request")
		}
	}), output: outputSchema[FindPortfolioOverlapsOutput]("Ordered source-backed portfolio overlap results."), handler: s.findPortfolioOverlaps})
	addCatalogTool(s, catalogTool[LinkPullRequestInput, LinkPullRequestOutput]{name: ToolLinkPullRequest, title: "Link a pull request to local contribution work", description: "Idempotently link one stored authored pull request to an existing local opportunity, managed workspace, or both. This writes only local workflow state and never changes GitHub.", annotations: localWriteAnnotations(true), supportedBy: supports[PortfolioOperator], input: inputSchema[LinkPullRequestInput](func(sc *schemaBuilder) {
		sc.schema.AnyOf = []*jsonschema.Schema{{Required: []string{"opportunity_id"}}, {Required: []string{"workspace_id"}}}
		if p := property(sc, "opportunity_id"); p != nil {
			p.MinLength = jsonschema.Ptr(1)
		}
		if p := property(sc, "workspace_id"); p != nil {
			p.MinLength = jsonschema.Ptr(1)
		}
	}), output: outputSchema[LinkPullRequestOutput]("Stored local pull-request relationship."), handler: s.linkPullRequest})
	addCatalogTool(s, catalogTool[IndexRepositoriesInput, JobReference]{name: ToolIndexRepositories, title: "Acquire and index repository code in one batch", description: "Safely acquire and index up to 10 repositories. Runs Git and writes locally; never executes repository code.", annotations: networkReadAnnotations(), supportedBy: supports[CodeIndexer], input: inputSchema[IndexRepositoriesInput](func(sc *schemaBuilder) { setArrayBounds(sc, "repositories", 1, 10) }), output: outputSchema[JobReference]("Reference to a bounded repository acquisition and indexing job."), handler: s.indexRepositories})
	addCatalogTool(s, catalogTool[CheckMergeConflictsInput, CheckMergeConflictsOutput]{name: ToolCheckMergeConflicts, title: "Check local Git merge conflicts in one batch", description: "Compare up to 50 fetched OID pairs without fetching or changing repository state.", annotations: processReadAnnotations(), supportedBy: supports[MergeConflictReader], input: inputSchema[CheckMergeConflictsInput](func(sc *schemaBuilder) { setArrayBounds(sc, "comparisons", 1, 50) }), output: outputSchema[CheckMergeConflictsOutput]("Ordered local merge-conflict checks."), handler: s.checkMergeConflicts})
	addCatalogTool(s, catalogTool[DeepWikiInput, DeepWikiOutput]{name: ToolQueryDeepWiki, title: "Query derived repository knowledge from DeepWiki", description: "Query DeepWiki for public repository architecture, contribution rules, testing, and subsystem context. Actions map to its public structure, contents, and question reads. Do not use this for live stars, thread state, checks, reviews, or mergeability.", annotations: externalReadAnnotations(), supportedBy: supports[ResearchReader], input: inputSchema[DeepWikiInput](func(sc *schemaBuilder) {
		setEnum(sc, "action", "structure", "contents", "question")
		setArrayBounds(sc, "repositories", 1, 10)
		setRange(sc, "max_output_bytes", 1024, 1048576)
		setDefault(sc, "max_output_bytes", 131072)
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
	for _, thread := range in.Threads {
		if err := validateThreadRef(thread, true); err != nil {
			return nil, GetThreadsOutput{}, err
		}
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
	if in.ResponseFormat == "" {
		in.ResponseFormat = "concise"
	}
	if in.ResponseFormat != "concise" && in.ResponseFormat != "detailed" {
		return nil, GetJobsOutput{}, InvalidArgument("response_format", "must be concise or detailed", map[string]any{"response_format": "concise"})
	}
	if _, ok := s.reader.(ScalableReader); !ok {
		out := GetJobsOutput{Status: "complete", Items: make([]mcpcontract.BatchItem[GetJobOutput], len(in.IDs))}
		for i, id := range in.IDs {
			if err := ctx.Err(); err != nil {
				return nil, out, err
			}
			item := mcpcontract.BatchItem[GetJobOutput]{Key: id, Status: "complete"}
			job, err := s.reader.GetJob(ctx, GetJobInput{ID: id})
			if err != nil {
				if isNotFound(err) {
					item.Status, item.Reason = "unavailable", "not_found"
				} else {
					item.Status, item.Reason = "failed", "read_failed"
				}
				item.Message = err.Error()
				out.Status = "partial"
			} else {
				if in.ResponseFormat == "concise" {
					job.Request, job.Result = nil, nil
					if job.Status == "succeeded" || job.Status == "failed" || job.Status == "cancelled" {
						item.NextAction = "Call jobs.get with response_format=detailed to read the terminal payload."
					}
				}
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
	op, ok := s.reader.(GitHubOperator)
	if !ok {
		return nil, JobReference{}, errors.New("repository metadata sync is not available")
	}
	out, err := op.SyncRepositoryMetadata(ctx, in)
	return nil, out, err
}

func (s *Server) searchGitHubRepositories(ctx context.Context, _ *mcp.CallToolRequest, in SearchGitHubRepositoriesInput) (*mcp.CallToolResult, SearchGitHubRepositoriesOutput, error) {
	if err := validateRepositorySearchInput(in); err != nil {
		return nil, SearchGitHubRepositoriesOutput{}, err
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	op, ok := s.reader.(GitHubOperator)
	if !ok {
		return nil, SearchGitHubRepositoriesOutput{}, errors.New("live GitHub repository search is not available")
	}
	out, err := op.SearchGitHubRepositories(ctx, in)
	if s.readOnly {
		out.SuggestedActions = nil
	}
	return nil, out, err
}

func validateRepositorySearchInput(in SearchGitHubRepositoriesInput) error {
	raw := strings.TrimSpace(in.RawQuery)
	structured := strings.TrimSpace(in.Text) != "" || len(in.MatchFields) > 0 || len(in.Topics) > 0 || strings.TrimSpace(in.Language) != "" || in.StarsMin != 0 || in.StarsMax != 0 || in.CreatedAfter != "" || in.CreatedBefore != "" || in.PushedAfter != "" || in.PushedBefore != "" || in.Archived != nil || in.Fork != nil
	if raw != "" && structured {
		return InvalidArgument("raw_query", "cannot be combined with structured filters; choose one input mode", map[string]any{"raw_query": "is:public language:go stars:>=100"})
	}
	if raw == "" && !structured {
		return InvalidArgument("text", "provide raw_query or at least one structured filter", map[string]any{"text": "GitHub contribution research", "match_fields": []string{"name", "description"}})
	}
	if len(in.MatchFields) > 0 && strings.TrimSpace(in.Text) == "" {
		return InvalidArgument("match_fields", "requires text", map[string]any{"text": "GitHub contribution research", "match_fields": []string{"name", "description"}})
	}
	return nil
}
func (s *Server) syncThreads(ctx context.Context, _ *mcp.CallToolRequest, in SyncThreadsInput) (*mcp.CallToolResult, JobReference, error) {
	if in.Selection != "repositories" && in.Selection != "threads" {
		return nil, JobReference{}, InvalidArgument("selection", "must be repositories or threads", map[string]any{"selection": "repositories"})
	}
	if in.Selection == "repositories" && len(in.Repositories) == 0 {
		return nil, JobReference{}, InvalidArgument("repositories", "are required in repository selection mode", map[string]any{"selection": "repositories", "repositories": []map[string]string{{"owner": "acme", "repo": "rocket"}}})
	}
	if in.Selection == "threads" && len(in.Threads) == 0 {
		return nil, JobReference{}, InvalidArgument("threads", "are required in thread selection mode", map[string]any{"selection": "threads", "threads": []map[string]any{{"owner": "acme", "repo": "rocket", "kind": "issue", "number": 1}}})
	}
	if in.Selection == "repositories" && len(in.Threads) > 0 {
		return nil, JobReference{}, InvalidArgument("threads", "are not accepted in repository selection mode", nil)
	}
	if in.Selection == "threads" && (len(in.Repositories) > 0 || in.Kind != "" || in.State != "" || in.UpdatedAfter != "" || in.LimitPerRepository != 0) {
		return nil, JobReference{}, InvalidArgument("selection", "repository filters are not accepted in thread selection mode", nil)
	}
	for _, thread := range in.Threads {
		if err := validateThreadRef(thread, false); err != nil {
			return nil, JobReference{}, err
		}
	}
	if in.Selection == "repositories" && in.LimitPerRepository == 0 {
		in.LimitPerRepository = 100
	}
	op, ok := s.reader.(GitHubOperator)
	if !ok {
		return nil, JobReference{}, errors.New("batch thread sync is not available")
	}
	out, err := op.SyncThreads(ctx, in)
	return nil, out, err
}
func (s *Server) hydrateThreads(ctx context.Context, _ *mcp.CallToolRequest, in HydrateThreadsInput) (*mcp.CallToolResult, JobReference, error) {
	if len(in.Threads) == 0 || len(in.Facets) == 0 {
		return nil, JobReference{}, InvalidArgument("facets", "threads and at least one facet are required", map[string]any{"facets": []string{"issue_comments"}})
	}
	if in.MaxPages == 0 {
		in.MaxPages = 3
	}
	op, ok := s.reader.(GitHubOperator)
	if !ok {
		return nil, JobReference{}, errors.New("batch thread hydration is not available")
	}
	out, err := op.HydrateThreads(ctx, in)
	return nil, out, err
}
func (s *Server) getAuthenticatedIdentity(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, AuthenticatedIdentityOutput, error) {
	op, ok := s.reader.(GitHubOperator)
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
	op, ok := s.reader.(GitHubOperator)
	if !ok {
		return nil, JobReference{}, errors.New("authored pull-request sync is not available")
	}
	out, err := op.SyncAuthoredPullRequests(ctx, in)
	return nil, out, err
}
func (s *Server) syncPullRequestStatus(ctx context.Context, _ *mcp.CallToolRequest, in SyncPullRequestStatusInput) (*mcp.CallToolResult, JobReference, error) {
	if len(in.PullRequests) == 0 {
		return nil, JobReference{}, InvalidArgument("pull_requests", "are required", nil)
	}
	if in.MaxPages == 0 {
		in.MaxPages = 3
	}
	op, ok := s.reader.(GitHubOperator)
	if !ok {
		return nil, JobReference{}, errors.New("pull-request status sync is not available")
	}
	out, err := op.SyncPullRequestStatus(ctx, in)
	return nil, out, err
}
func (s *Server) indexRepositories(ctx context.Context, _ *mcp.CallToolRequest, in IndexRepositoriesInput) (*mcp.CallToolResult, JobReference, error) {
	if len(in.Repositories) == 0 {
		return nil, JobReference{}, InvalidArgument("repositories", "are required", nil)
	}
	op, ok := s.reader.(CodeIndexer)
	if !ok {
		return nil, JobReference{}, errors.New("batch code indexing is not available")
	}
	out, err := op.IndexRepositories(ctx, in)
	return nil, out, err
}
func (s *Server) checkMergeConflicts(ctx context.Context, _ *mcp.CallToolRequest, in CheckMergeConflictsInput) (*mcp.CallToolResult, CheckMergeConflictsOutput, error) {
	if len(in.Comparisons) == 0 {
		return nil, CheckMergeConflictsOutput{}, InvalidArgument("comparisons", "are required", nil)
	}
	op, ok := s.reader.(MergeConflictReader)
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
		return nil, DeepWikiOutput{}, InvalidArgument("max_output_bytes", "must be between 1024 and 1048576", map[string]any{"max_output_bytes": 131072})
	}
	if (in.Action == "structure" || in.Action == "contents") && strings.TrimSpace(in.Repository) == "" {
		return nil, DeepWikiOutput{}, InvalidArgument("repository", "is required for structure and contents", map[string]any{"repository": "owner/repo"})
	}
	if in.Action == "question" && (len(in.Repositories) == 0 || strings.TrimSpace(in.Question) == "") {
		return nil, DeepWikiOutput{}, InvalidArgument("question", "repositories and question are required for question", map[string]any{"repositories": []string{"owner/repo"}, "question": "Where is search ranking implemented?"})
	}
	op, ok := s.reader.(ResearchReader)
	if !ok {
		return nil, DeepWikiOutput{}, errors.New("DeepWiki is not available")
	}
	out, err := op.DeepWiki(ctx, in)
	return nil, out, err
}

func setArrayBounds(schema *schemaBuilder, name string, minimum, maximum int) {
	p := property(schema, name)
	if p == nil {
		return
	}
	p.MinItems = jsonschema.Ptr(minimum)
	p.MaxItems = jsonschema.Ptr(maximum)
}

func rankOpportunitiesOutputSchema() schemaDefinition {
	definition := outputSchema[RankOpportunitiesOutput]("Bounded cross-repository Radar ranking.")
	setOutputPropertyRange(definition.schema, "score", 0, 100)
	return definition
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
