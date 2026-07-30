package mcpserver

import (
	"context"
	"errors"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/facets"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
	"github.com/morluto/gitcontribute/internal/repositorycontext"
)

const serverInstructions = "Use advertised GitContribute tools for durable, source-backed repository research and contribution tracking. " +
	"Prefer corpus tools for offline reads; they never refresh data implicitly. " +
	"GitHub tools perform explicit network reads and may update only the local corpus. " +
	"Research tools return derived external context, never live GitHub state. " +
	"The durable workflow is concern to investigation to hypothesis to opportunity to workspace to draft; use only advertised stages. " +
	"Use workflow.prepare_issue_set when exact issue numbers already define the contribution scope. " +
	"When an operation returns a job, poll advertised job tools in batches. " +
	"To inspect a returned resource, ask the host to perform MCP resources/read with this server and the exact URI; in Codex, call read_mcp_resource. Treat resource URIs as opaque identifiers and never shorten, pluralize, or reconstruct them. " +
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

// SyncRepositoryContextInput selects repositories for asynchronous context refresh.

// SyncThreadsInput selects either bounded repository-wide header discovery or
// exact thread refresh. It never requests child comments, reviews, or code.

// HydrateThreadsInput requests explicit child facets for already selected
// threads. Facets must be non-empty to prevent accidental broad hydration.

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
	addCatalogTool(s, catalogTool[mcpcontract.GetRepositoriesInput, mcpcontract.GetRepositoriesOutput]{name: mcpcontract.ToolGetRepositories, title: "Get stored repositories in one batch", description: "Read metadata, coverage, and dossier availability for up to 100 stored repositories. Use for comparison before reading dossier resources. Missing metadata includes a sync action. Offline.", annotations: readOnly, supportedBy: supports[ScalableReader], input: inputSchema[mcpcontract.GetRepositoriesInput](func(sc *schemaBuilder) { setArrayBounds(sc, "repositories", 1, 100) }), output: outputSchema[mcpcontract.GetRepositoriesOutput]("Ordered repository batch with item-level status and dossier availability."), handler: s.getRepositories})
	addCatalogTool(s, catalogTool[mcpcontract.GetThreadsInput, mcpcontract.GetThreadsOutput]{name: mcpcontract.ToolGetThreads, title: "Get stored threads in one batch", description: "Read up to 100 exact stored issues or pull requests in input order. Choose compact for triage and full only for finalists; this tool is offline.", annotations: readOnly, supportedBy: supports[ScalableReader], input: inputSchema[mcpcontract.GetThreadsInput](func(sc *schemaBuilder) {
		setArrayBounds(sc, "threads", 1, 100)
		setEnum(sc, "view", "compact", "full")
		setDefault(sc, "view", "compact")
	}), output: outputSchema[mcpcontract.GetThreadsOutput]("Ordered stored-thread batch with item-level status."), handler: s.getThreads})
	addCatalogTool(s, catalogTool[mcpcontract.RankOpportunitiesInput, mcpcontract.RankOpportunitiesOutput]{name: mcpcontract.ToolRankThreads, title: "Rank stored threads for contribution", description: "Rank open issues across 1-50 required stored repositories. This bounded offline result reports truncation and never persists opportunities.", annotations: readOnly, supportedBy: supports[ScalableReader], input: inputSchema[mcpcontract.RankOpportunitiesInput](func(sc *schemaBuilder) {
		setArrayBounds(sc, "repositories", 1, 50)
		setRange(sc, "limit", 1, 100)
		setDefault(sc, "limit", 20)
		setRange(sc, "max_results_per_repository", 1, 100)
		setDefault(sc, "max_results_per_repository", 10)
	}), output: outputSchema[mcpcontract.RankOpportunitiesOutput]("Bounded cross-repository Radar ranking."), handler: s.rankOpportunities})
	addCatalogTool(s, catalogTool[mcpcontract.FindPrecedentsInput, mcpcontract.FindPrecedentsOutput]{name: mcpcontract.ToolFindPrecedents, title: "Find historical issue and pull-request precedents", description: "Find similar closed issues and pull requests for up to 20 source threads, including completed, not-planned, duplicate, and merged evidence. This is an offline historical read, not a current opportunity search.", annotations: readOnly, supportedBy: supports[ScalableReader], input: inputSchema[mcpcontract.FindPrecedentsInput](func(sc *schemaBuilder) {
		setArrayBounds(sc, "threads", 1, 20)
		setRange(sc, "limit", 1, 100)
		setDefault(sc, "limit", 20)
	}), output: outputSchema[mcpcontract.FindPrecedentsOutput]("Historical precedents grouped by source thread."), handler: s.findPrecedents})
	addCatalogTool(s, catalogTool[mcpcontract.PrepareIssueSetInput, mcpcontract.PrepareIssueSetOutput]{name: mcpcontract.ToolPrepareIssueSet, title: "Prepare contribution evidence from exact issues", description: "Compose stored facts, coverage gaps, related work, merged precedents, and linkage candidates for 1-20 exact issues. Prefer this to manual reads. Offline; creates no opportunity or draft.", annotations: readOnly, supportedBy: supports[IssueSetReader], input: inputSchema[mcpcontract.PrepareIssueSetInput](func(sc *schemaBuilder) {
		setArrayBounds(sc, "issue_numbers", 1, 20)
		if numbers := property(sc, "issue_numbers"); numbers != nil {
			numbers.UniqueItems = true
			if numbers.Items != nil {
				numbers.Items.Minimum = jsonschema.Ptr(1.0)
			}
		}
		setRange(sc, "precedent_limit", 1, 10)
		setDefault(sc, "precedent_limit", 3)
		setEnum(sc, "response_format", "concise", "detailed")
		setDefault(sc, "response_format", "concise")
	}), output: outputSchema[mcpcontract.PrepareIssueSetOutput]("Contribution-facing evidence for an exact stored issue set."), handler: s.prepareIssueSet})
	addCatalogTool(s, catalogTool[mcpcontract.GetJobsInput, mcpcontract.GetJobsOutput]{name: mcpcontract.ToolGetJob, title: "Get durable jobs in one batch", description: "Poll up to 100 jobs with execution state, terminal outcome, progress, and artifact links. Use concise while polling and detailed after completion. Offline; executor blobs stay hidden.", annotations: readOnly, input: inputSchema[mcpcontract.GetJobsInput](func(sc *schemaBuilder) {
		setArrayBounds(sc, "ids", 1, 100)
		setEnum(sc, "response_format", "concise", "detailed")
		setDefault(sc, "response_format", "concise")
	}), output: outputSchema[mcpcontract.GetJobsOutput]("Ordered durable-job states."), handler: s.getJobs})
	addCatalogTool(s, catalogTool[mcpcontract.SearchGitHubRepositoriesInput, mcpcontract.SearchGitHubRepositoriesOutput]{name: mcpcontract.ToolSearchGitHubRepositories, title: "Search live GitHub repositories", description: "Find repositories with structured filters and persist metadata. Use raw_query for unsupported GitHub qualifiers. Does not fetch threads or code.", annotations: networkReadAnnotations(), supportedBy: supports[GitHubOperator], input: inputSchema[mcpcontract.SearchGitHubRepositoriesInput](func(sc *schemaBuilder) {
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
		configureRepositorySearchModes(sc)
	}), output: outputSchema[mcpcontract.SearchGitHubRepositoriesOutput]("Live repository search with persisted metadata."), handler: s.searchGitHubRepositories})
	addCatalogTool(s, catalogTool[mcpcontract.SyncRepositoryContextInput, mcpcontract.JobReference]{name: mcpcontract.ToolSyncRepositoryContext, title: "Sync repository context in one batch", description: "Fetch current GitHub stars, metadata, and fixed contribution-guidance files for up to 100 explicit repositories; no threads or code.", annotations: networkReadAnnotations(), supportedBy: supports[GitHubOperator], input: inputSchema[mcpcontract.SyncRepositoryContextInput](func(sc *schemaBuilder) {
		setArrayBounds(sc, "repositories", 1, 100)
		setRange(sc, "max_requests", float64(repositorycontext.RequestCost()), 1000)
		setDefault(sc, "max_requests", 1000)
	}), output: outputSchema[mcpcontract.JobReference]("Reference to a repository-context synchronization job."), handler: s.syncRepositoryContext})
	addCatalogTool(s, catalogTool[mcpcontract.SyncThreadsInput, mcpcontract.JobReference]{name: mcpcontract.ToolSyncThreads, title: "Sync GitHub thread headers in one batch", description: "Fetch and persist GitHub issue or pull-request headers for up to 50 repositories or 100 exact threads. Use exact mode for known numbers and repository mode for discovery. Fetches no metadata, policy files, comments, reviews, checks, or code.", annotations: networkReadAnnotations(), supportedBy: supports[GitHubOperator], input: inputSchema[mcpcontract.SyncThreadsInput](func(sc *schemaBuilder) {
		setEnum(sc, "selection", "repositories", "threads")
		property(sc, "repositories").MaxItems = jsonschema.Ptr(50)
		property(sc, "threads").MaxItems = jsonschema.Ptr(100)
		setEnum(sc, "kind", "issue", "pull_request", "both")
		setEnum(sc, "state", "open", "closed", "all")
		setRange(sc, "limit_per_repository", 1, 1000)
		setRange(sc, "max_requests", 1, 1000)
		setDefault(sc, "max_requests", 1000)
		configureSyncThreadModes(sc)
	}), output: outputSchema[mcpcontract.JobReference]("Reference to a bounded thread-header synchronization job."), handler: s.syncThreads})
	addCatalogTool(s, catalogTool[mcpcontract.HydrateThreadsInput, mcpcontract.JobReference]{name: mcpcontract.ToolHydrateThreads, title: "Synchronize selected GitHub thread details", description: "Fetch selected GitHub child data for up to 100 known issues or pull requests. Use after ranking finalists. Do not use to inspect existing corpus coverage; corpus.get_coverage is the offline coverage read.", annotations: networkReadAnnotations(), supportedBy: supports[GitHubOperator], input: inputSchema[mcpcontract.HydrateThreadsInput](func(sc *schemaBuilder) {
		setArrayBounds(sc, "threads", 1, 100)
		setArrayBounds(sc, "facets", 1, 5)
		setArrayEnum(sc, "facets", facets.SelectableNames()...)
		setRange(sc, "max_pages", 1, 100)
		setDefault(sc, "max_pages", 3)
	}), output: outputSchema[mcpcontract.JobReference]("Reference to a bounded exact-thread hydration job."), handler: s.hydrateThreads})
	addCatalogTool(s, catalogTool[mcpcontract.MineRepositoryFixPatternsInput, mcpcontract.JobReference]{
		name: mcpcontract.ToolMineRepositoryFixPatterns, title: "Mine repository fix patterns",
		description: "Mine repository-level evidence about how similar problems were accepted, rejected, or superseded. Searches stored pull requests, refreshes only bounded unknown-state finalists, and persists a report separating explicit links from similarity. Prefer this over repeated search and hydration loops; not for live competing-work checks. Performs GitHub reads and local writes, never GitHub mutation.",
		annotations: networkReadAnnotations(), supportedBy: supports[FixPatternWorkflow],
		input: inputSchema[mcpcontract.MineRepositoryFixPatternsInput](func(sc *schemaBuilder) {
			setArrayBounds(sc, "symptom_taxonomy", 1, 12)
			setArrayBounds(sc, "merge_outcomes", 1, 5)
			property(sc, "merge_outcomes").UniqueItems = true
			setRange(sc, "candidate_limit", 1, 100)
			setDefault(sc, "candidate_limit", mcpcontract.DefaultFixPatternCandidateLimit)
			setRange(sc, "hydration_limit", 0, 100)
			setDefault(sc, "hydration_limit", mcpcontract.DefaultFixPatternHydrationLimit)
			setRange(sc, "representative_limit", 1, 20)
			setDefault(sc, "representative_limit", mcpcontract.DefaultFixPatternRepresentativeLimit)
			symptoms := property(sc, "symptom_taxonomy")
			if symptoms != nil && symptoms.Items != nil {
				name := symptoms.Items.Properties["name"]
				if name != nil {
					name.MinLength = jsonschema.Ptr(1)
					name.Pattern = nonWhitespacePattern
				}
				terms := symptoms.Items.Properties["terms"]
				if terms != nil {
					terms.MinItems = jsonschema.Ptr(1)
					terms.MaxItems = jsonschema.Ptr(12)
					terms.UniqueItems = true
					if terms.Items != nil {
						terms.Items.MinLength = jsonschema.Ptr(1)
						terms.Items.Pattern = nonWhitespacePattern
					}
				}
			}
			window := property(sc, "time_window")
			if window != nil {
				for _, field := range []string{"updated_after", "updated_before"} {
					if value := window.Properties[field]; value != nil {
						value.Format = "date-time"
					}
				}
			}
		}),
		output:  outputSchema[mcpcontract.JobReference]("Reference to a bounded repository fix-pattern mining job."),
		handler: s.mineRepositoryFixPatterns,
	})
	addCatalogTool(s, catalogTool[mcpcontract.SyncPortfolioInput, mcpcontract.JobReference]{name: mcpcontract.ToolSyncPortfolio, title: "Synchronize a pull-request portfolio", description: "Use for an authored portfolio or 1-100 exact pull requests. Refreshes PR details, merge state, checks, review state, unresolved threads, merge queue, files, and closing issues in one durable job.", annotations: networkReadAnnotations(), supportedBy: supports[GitHubOperator], input: inputSchema[mcpcontract.SyncPortfolioInput](func(sc *schemaBuilder) {
		setEnum(sc, "selection", "authored", "explicit")
		setArrayBounds(sc, "pull_requests", 1, 100)
		setEnum(sc, "state", "open", "closed", "all")
		setRange(sc, "limit", 1, 100)
		setRange(sc, "discovery_max_requests", 2, 1000)
		setRange(sc, "status_max_pages", 1, 20)
		setDefault(sc, "status_max_pages", 3)
		configureSyncPortfolioModes(sc)
	}), output: outputSchema[mcpcontract.JobReference]("Reference to a pull-request portfolio synchronization job."), handler: s.syncPortfolio})
	addCatalogTool(s, catalogTool[mcpcontract.SyncPullRequestFeedbackInput, mcpcontract.JobReference]{name: mcpcontract.ToolSyncPullRequestFeedback, title: "Synchronize pull-request feedback", description: "Fetch independently covered issue comments, submitted reviews, inline comments, and review-thread topology for 1-50 exact pull requests under one total request budget.", annotations: networkReadAnnotations(), supportedBy: supports[PullRequestFeedbackOperator], input: inputSchema[mcpcontract.SyncPullRequestFeedbackInput](func(sc *schemaBuilder) {
		setArrayBounds(sc, "pull_requests", 1, 50)
		setArrayBounds(sc, "channels", 1, 4)
		setArrayEnum(sc, "channels", "issue_comments", "submitted_reviews", "inline_comments", "review_threads")
		property(sc, "channels").UniqueItems = true
		setEnum(sc, "thread_state", "unresolved", "all")
		setRange(sc, "max_items_per_channel", 1, 1000)
		setRange(sc, "max_requests", 1, 1000)
	}), output: outputSchema[mcpcontract.JobReference]("Reference to a bounded pull-request feedback job."), handler: s.syncPullRequestFeedback})
	addCatalogTool(s, catalogTool[mcpcontract.SyncCIFailuresInput, mcpcontract.JobReference]{name: mcpcontract.ToolSyncCIFailures, title: "Synchronize pull-request CI failures", description: "Resolve each current head SHA and normalize legacy statuses, check runs, Actions runs, jobs, and optionally bounded failed-job logs for 1-20 exact pull requests.", annotations: networkReadAnnotations(), supportedBy: supports[CIFailureOperator], input: inputSchema[mcpcontract.SyncCIFailuresInput](func(sc *schemaBuilder) {
		setArrayBounds(sc, "pull_requests", 1, 20)
		setEnum(sc, "logs", "none", "failures_only")
		setRange(sc, "max_runs_per_pr", 1, 100)
		setRange(sc, "max_jobs_per_run", 1, 100)
		setRange(sc, "max_log_bytes_per_job", 1024, 1048576)
		setRange(sc, "max_requests", 1, 1000)
	}), output: outputSchema[mcpcontract.JobReference]("Reference to a bounded CI diagnostics job."), handler: s.syncCIFailures})
	addCatalogTool(s, catalogTool[mcpcontract.ListPullRequestPortfolioInput, mcpcontract.ListPullRequestPortfolioOutput]{name: mcpcontract.ToolListPullRequestPortfolio, title: "List pull requests that need contributor attention", description: "List stored authored pull requests with deterministic attention from lifecycle, checks, review conversations, merge state, queue, and freshness. This offline read reports incomplete facets as unknown; sync authored PRs and health when stale.", annotations: readOnly, supportedBy: supports[PortfolioReader], input: inputSchema[mcpcontract.ListPullRequestPortfolioInput](func(sc *schemaBuilder) {
		setArrayBounds(sc, "authors", 0, 1)
		setEnum(sc, "state", "open", "closed", "all")
		setRange(sc, "limit", 1, 100)
		setDefault(sc, "limit", 20)
		setEnum(sc, "view", "compact", "full")
		setDefault(sc, "view", "compact")
	}), output: outputSchema[mcpcontract.ListPullRequestPortfolioOutput]("Offline pull-request portfolio with explainable attention states."), handler: s.listPullRequestPortfolio})
	addCatalogTool(s, catalogTool[mcpcontract.FindPortfolioOverlapsInput, mcpcontract.FindPortfolioOverlapsOutput]{name: mcpcontract.ToolFindPortfolioOverlaps, title: "Find overlaps with authored pull requests", description: "Compare up to 50 local candidates with 100 stored authored pull requests using complete changed-path, linked-issue, and opportunity-similarity observations. This offline read returns unknown instead of claiming no overlap when coverage is missing.", annotations: readOnly, supportedBy: supports[PortfolioReader], input: inputSchema[mcpcontract.FindPortfolioOverlapsInput](func(sc *schemaBuilder) {
		setArrayBounds(sc, "candidates", 1, 50)
		setArrayBounds(sc, "pull_requests", 1, 100)
		if candidate := sc.schema.Defs["PortfolioSubjectInput"]; candidate != nil {
			setEnum(&schemaBuilder{schema: candidate, err: sc.err}, "kind", "opportunity", "workspace", "pull_request")
		}
	}), output: outputSchema[mcpcontract.FindPortfolioOverlapsOutput]("Ordered source-backed portfolio overlap results."), handler: s.findPortfolioOverlaps})
	addCatalogTool(s, catalogTool[mcpcontract.LinkPullRequestInput, mcpcontract.LinkPullRequestOutput]{name: mcpcontract.ToolLinkPullRequest, title: "Link a pull request to local contribution work", description: "Idempotently link one stored authored pull request to an existing local opportunity, managed workspace, or both. This writes only local workflow state and never changes GitHub.", annotations: localWriteAnnotations(true), supportedBy: supports[PortfolioOperator], input: inputSchema[mcpcontract.LinkPullRequestInput](func(sc *schemaBuilder) {
		sc.schema.AnyOf = []*jsonschema.Schema{{Required: []string{"opportunity_id"}}, {Required: []string{"workspace_id"}}}
		if p := property(sc, "opportunity_id"); p != nil {
			p.MinLength = jsonschema.Ptr(1)
		}
		if p := property(sc, "workspace_id"); p != nil {
			p.MinLength = jsonschema.Ptr(1)
		}
	}), output: outputSchema[mcpcontract.LinkPullRequestOutput]("Stored local pull-request relationship."), handler: s.linkPullRequest})
	addCatalogTool(s, catalogTool[mcpcontract.IndexRepositoriesInput, mcpcontract.JobReference]{name: mcpcontract.ToolIndexRepositories, title: "Acquire and index repository code in one batch", description: "Safely acquire and index up to 10 repositories. Runs Git and writes locally; never executes repository code.", annotations: networkReadAnnotations(), supportedBy: supports[CodeIndexer], input: inputSchema[mcpcontract.IndexRepositoriesInput](func(sc *schemaBuilder) { setArrayBounds(sc, "repositories", 1, 10) }), output: outputSchema[mcpcontract.JobReference]("Reference to a bounded repository acquisition and indexing job."), handler: s.indexRepositories})
	addCatalogTool(s, catalogTool[mcpcontract.CheckMergeConflictsInput, mcpcontract.CheckMergeConflictsOutput]{name: mcpcontract.ToolCheckMergeConflicts, title: "Check local Git merge conflicts in one batch", description: "Compare up to 50 fetched OID pairs without fetching or changing repository state.", annotations: processReadAnnotations(), supportedBy: supports[MergeConflictReader], input: inputSchema[mcpcontract.CheckMergeConflictsInput](func(sc *schemaBuilder) { setArrayBounds(sc, "comparisons", 1, 50) }), output: outputSchema[mcpcontract.CheckMergeConflictsOutput]("Ordered local merge-conflict checks."), handler: s.checkMergeConflicts})
	addCatalogTool(s, catalogTool[mcpcontract.DeepWikiInput, mcpcontract.DeepWikiOutput]{name: mcpcontract.ToolQueryDeepWiki, title: "Query derived repository knowledge from DeepWiki", description: "Query DeepWiki for public repository architecture, contribution rules, testing, and subsystem context. Actions map to its public structure, contents, and question reads. Do not use this for live stars, thread state, checks, reviews, or mergeability.", annotations: externalReadAnnotations(), supportedBy: supports[ResearchReader], input: inputSchema[mcpcontract.DeepWikiInput](func(sc *schemaBuilder) {
		setEnum(sc, "action", "structure", "contents", "question")
		setArrayBounds(sc, "repositories", 1, 10)
		setRange(sc, "max_output_bytes", mcpcontract.DeepWikiMinOutputBytes, mcpcontract.DeepWikiMaxOutputBytes)
		setDefault(sc, "max_output_bytes", mcpcontract.DeepWikiDefaultOutputBytes)
		configureDeepWikiModes(sc)
	}), output: outputSchema[mcpcontract.DeepWikiOutput]("Derived DeepWiki response with provenance."), handler: s.deepWiki})
}

func (s *Server) scalableReader() (ScalableReader, error) {
	r, ok := s.reader.(ScalableReader)
	if !ok {
		return nil, errors.New("bounded batch reads are not available")
	}
	return r, nil
}
func (s *Server) getRepositories(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.GetRepositoriesInput) (*mcp.CallToolResult, mcpcontract.GetRepositoriesOutput, error) {
	r, err := s.scalableReader()
	if err != nil {
		return nil, mcpcontract.GetRepositoriesOutput{}, err
	}
	out, err := r.GetRepositories(ctx, in)
	return nil, out, err
}
func (s *Server) getThreads(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.GetThreadsInput) (*mcp.CallToolResult, mcpcontract.GetThreadsOutput, error) {
	if in.View == "" {
		in.View = "compact"
	}
	for _, thread := range in.Threads {
		if err := validateThreadRef(thread, true); err != nil {
			return nil, mcpcontract.GetThreadsOutput{}, err
		}
	}
	r, err := s.scalableReader()
	if err != nil {
		return nil, mcpcontract.GetThreadsOutput{}, err
	}
	out, err := r.GetThreads(ctx, in)
	return nil, out, err
}
func (s *Server) rankOpportunities(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.RankOpportunitiesInput) (*mcp.CallToolResult, mcpcontract.RankOpportunitiesOutput, error) {
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.MaxResultsPerRepository == 0 {
		in.MaxResultsPerRepository = 10
	}
	r, err := s.scalableReader()
	if err != nil {
		return nil, mcpcontract.RankOpportunitiesOutput{}, err
	}
	out, err := r.RankOpportunities(ctx, in)
	return nil, out, err
}
func (s *Server) findPrecedents(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.FindPrecedentsInput) (*mcp.CallToolResult, mcpcontract.FindPrecedentsOutput, error) {
	if in.Limit == 0 {
		in.Limit = 20
	}
	r, err := s.scalableReader()
	if err != nil {
		return nil, mcpcontract.FindPrecedentsOutput{}, err
	}
	out, err := r.FindPrecedents(ctx, in)
	return nil, out, err
}
func (s *Server) getJobs(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.GetJobsInput) (*mcp.CallToolResult, mcpcontract.GetJobsOutput, error) {
	if in.ResponseFormat == "" {
		in.ResponseFormat = "concise"
	}
	if in.ResponseFormat != "concise" && in.ResponseFormat != "detailed" {
		return nil, mcpcontract.GetJobsOutput{}, mcpcontract.InvalidArgument("response_format", "must be concise or detailed", map[string]any{"response_format": "concise"})
	}
	if _, ok := s.reader.(ScalableReader); !ok {
		out := mcpcontract.GetJobsOutput{Status: "complete", Items: make([]mcpcontract.BatchItem[mcpcontract.GetJobOutput], len(in.IDs))}
		for i, id := range in.IDs {
			if err := ctx.Err(); err != nil {
				return nil, out, err
			}
			item := mcpcontract.BatchItem[mcpcontract.GetJobOutput]{Key: id, Status: "complete"}
			job, err := s.reader.GetJob(ctx, mcpcontract.GetJobInput{ID: id})
			if err != nil {
				if isNotFound(err) {
					item.Status, item.Reason = "unavailable", "not_found"
				} else {
					item.Status, item.Reason = "failed", "read_failed"
				}
				item.Message = err.Error()
				out.Status = "partial"
			} else {
				normalizeJobExecution(&job)
				if in.ResponseFormat == "concise" {
					if job.Status == "succeeded" || job.Status == "failed" || job.Status == "cancelled" {
						item.NextAction = "Call jobs.get with response_format=detailed to read typed artifact and follow-up references."
					}
				}
				item.Value = &job
			}
			out.Items[i] = item
		}
		return linkedJobResources(out), out, nil
	}
	r, err := s.scalableReader()
	if err != nil {
		return nil, mcpcontract.GetJobsOutput{}, err
	}
	out, err := r.GetJobs(ctx, in)
	if err != nil {
		return nil, out, err
	}
	for i := range out.Items {
		if out.Items[i].Value != nil {
			normalizeJobExecution(out.Items[i].Value)
		}
	}
	return linkedJobResources(out), out, nil
}

func normalizeJobExecution(job *mcpcontract.GetJobOutput) {
	switch job.Status {
	case "queued":
		job.ExecutionState, job.Outcome = "queued", ""
	case "running":
		job.ExecutionState, job.Outcome = "running", ""
	case "succeeded":
		job.ExecutionState = "terminal"
		if job.Outcome == "" {
			job.Outcome = "succeeded"
		}
	case "failed":
		job.ExecutionState, job.Outcome = "terminal", "failed"
	case "cancelled":
		job.ExecutionState, job.Outcome = "terminal", "cancelled"
	}
}
func (s *Server) syncRepositoryContext(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.SyncRepositoryContextInput) (*mcp.CallToolResult, mcpcontract.JobReference, error) {
	op, ok := s.reader.(GitHubOperator)
	if !ok {
		return nil, mcpcontract.JobReference{}, errors.New("repository context sync is not available")
	}
	out, err := op.SyncRepositoryContext(ctx, in)
	return nil, out, err
}

func (s *Server) prepareIssueSet(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.PrepareIssueSetInput) (*mcp.CallToolResult, mcpcontract.PrepareIssueSetOutput, error) {
	r, ok := s.reader.(IssueSetReader)
	if !ok {
		return nil, mcpcontract.PrepareIssueSetOutput{}, errors.New("issue-set preparation is not available")
	}
	out, err := r.PrepareIssueSet(ctx, in)
	return nil, out, err
}

func (s *Server) searchGitHubRepositories(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.SearchGitHubRepositoriesInput) (*mcp.CallToolResult, mcpcontract.SearchGitHubRepositoriesOutput, error) {
	if err := validateRepositorySearchInput(in); err != nil {
		return nil, mcpcontract.SearchGitHubRepositoriesOutput{}, err
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	op, ok := s.reader.(GitHubOperator)
	if !ok {
		return nil, mcpcontract.SearchGitHubRepositoriesOutput{}, errors.New("live GitHub repository search is not available")
	}
	out, err := op.SearchGitHubRepositories(ctx, in)
	if s.readOnly {
		out.SuggestedActions = nil
	}
	return nil, out, err
}

func validateRepositorySearchInput(in mcpcontract.SearchGitHubRepositoriesInput) error {
	raw := strings.TrimSpace(in.RawQuery)
	structured := strings.TrimSpace(in.Text) != "" || len(in.MatchFields) > 0 || len(in.Topics) > 0 || strings.TrimSpace(in.Language) != "" || in.StarsMin != nil || in.StarsMax != nil || in.CreatedAfter != "" || in.CreatedBefore != "" || in.PushedAfter != "" || in.PushedBefore != "" || in.Archived != nil || in.Fork != nil
	// The SDK owns the raw-versus-structured shape. These checks retain only
	// whitespace semantics that JSON Schema cannot express.
	if raw == "" && !structured {
		return mcpcontract.InvalidArgument("text", "provide raw_query or at least one structured filter", map[string]any{"text": "GitHub contribution research", "match_fields": []string{"name", "description"}})
	}
	if len(in.MatchFields) > 0 && strings.TrimSpace(in.Text) == "" {
		return mcpcontract.InvalidArgument("match_fields", "requires text", map[string]any{"text": "GitHub contribution research", "match_fields": []string{"name", "description"}})
	}
	return nil
}
func (s *Server) syncThreads(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.SyncThreadsInput) (*mcp.CallToolResult, mcpcontract.JobReference, error) {
	for _, thread := range in.Threads {
		if err := validateThreadRef(thread, false); err != nil {
			return nil, mcpcontract.JobReference{}, err
		}
	}
	if in.Selection == "repositories" && in.LimitPerRepository == 0 {
		in.LimitPerRepository = 100
	}
	op, ok := s.reader.(GitHubOperator)
	if !ok {
		return nil, mcpcontract.JobReference{}, errors.New("batch thread sync is not available")
	}
	out, err := op.SyncThreads(ctx, in)
	return nil, out, err
}
func (s *Server) hydrateThreads(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.HydrateThreadsInput) (*mcp.CallToolResult, mcpcontract.JobReference, error) {
	if len(in.Threads) == 0 || len(in.Facets) == 0 {
		return nil, mcpcontract.JobReference{}, mcpcontract.InvalidArgument("facets", "threads and at least one facet are required", map[string]any{"facets": []string{facets.IssueComments}})
	}
	if in.MaxPages == 0 {
		in.MaxPages = 3
	}
	op, ok := s.reader.(GitHubOperator)
	if !ok {
		return nil, mcpcontract.JobReference{}, errors.New("batch thread hydration is not available")
	}
	out, err := op.HydrateThreads(ctx, in)
	return nil, out, err
}

func (s *Server) mineRepositoryFixPatterns(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.MineRepositoryFixPatternsInput) (*mcp.CallToolResult, mcpcontract.JobReference, error) {
	operator, ok := s.reader.(FixPatternOperator)
	if !ok {
		return nil, mcpcontract.JobReference{}, errors.New("repository fix-pattern mining is not available")
	}
	out, err := operator.MineRepositoryFixPatterns(ctx, in)
	return nil, out, err
}
func (s *Server) syncPortfolio(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.SyncPortfolioInput) (*mcp.CallToolResult, mcpcontract.JobReference, error) {
	if in.Selection == "" {
		in.Selection = "authored"
	}
	if in.Selection == "authored" && in.State == "" {
		in.State = "open"
	}
	if in.Selection == "authored" && in.Limit == 0 {
		in.Limit = 100
	}
	if in.Selection == "authored" && in.DiscoveryMaxRequests == 0 {
		in.DiscoveryMaxRequests = 1000
	}
	if in.StatusMaxPages == 0 {
		in.StatusMaxPages = 3
	}
	op, ok := s.reader.(GitHubOperator)
	if !ok {
		return nil, mcpcontract.JobReference{}, errors.New("portfolio synchronization is not available")
	}
	out, err := op.SyncPortfolio(ctx, in)
	return nil, out, err
}
func (s *Server) syncPullRequestFeedback(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.SyncPullRequestFeedbackInput) (*mcp.CallToolResult, mcpcontract.JobReference, error) {
	if len(in.PullRequests) == 0 {
		return nil, mcpcontract.JobReference{}, mcpcontract.InvalidArgument("pull_requests", "are required", nil)
	}
	op, ok := s.reader.(PullRequestFeedbackOperator)
	if !ok {
		return nil, mcpcontract.JobReference{}, errors.New("pull-request feedback synchronization is not available")
	}
	out, err := op.SyncPullRequestFeedback(ctx, in)
	return nil, out, err
}
func (s *Server) syncCIFailures(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.SyncCIFailuresInput) (*mcp.CallToolResult, mcpcontract.JobReference, error) {
	if len(in.PullRequests) == 0 {
		return nil, mcpcontract.JobReference{}, mcpcontract.InvalidArgument("pull_requests", "are required", nil)
	}
	op, ok := s.reader.(CIFailureOperator)
	if !ok {
		return nil, mcpcontract.JobReference{}, errors.New("CI failure synchronization is not available")
	}
	out, err := op.SyncCIFailures(ctx, in)
	return nil, out, err
}
func (s *Server) indexRepositories(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.IndexRepositoriesInput) (*mcp.CallToolResult, mcpcontract.JobReference, error) {
	if len(in.Repositories) == 0 {
		return nil, mcpcontract.JobReference{}, mcpcontract.InvalidArgument("repositories", "are required", nil)
	}
	op, ok := s.reader.(CodeIndexer)
	if !ok {
		return nil, mcpcontract.JobReference{}, errors.New("batch code indexing is not available")
	}
	out, err := op.IndexRepositories(ctx, in)
	return nil, out, err
}
func (s *Server) checkMergeConflicts(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.CheckMergeConflictsInput) (*mcp.CallToolResult, mcpcontract.CheckMergeConflictsOutput, error) {
	if len(in.Comparisons) == 0 {
		return nil, mcpcontract.CheckMergeConflictsOutput{}, mcpcontract.InvalidArgument("comparisons", "are required", nil)
	}
	op, ok := s.reader.(MergeConflictReader)
	if !ok {
		return nil, mcpcontract.CheckMergeConflictsOutput{}, errors.New("local merge-conflict checks are not available")
	}
	out, err := op.CheckMergeConflicts(ctx, in)
	return nil, out, err
}
func (s *Server) deepWiki(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.DeepWikiInput) (*mcp.CallToolResult, mcpcontract.DeepWikiOutput, error) {
	in.Action = strings.TrimSpace(in.Action)
	if in.MaxOutputBytes == 0 {
		in.MaxOutputBytes = mcpcontract.DeepWikiDefaultOutputBytes
	}
	if in.MaxOutputBytes < mcpcontract.DeepWikiMinOutputBytes || in.MaxOutputBytes > mcpcontract.DeepWikiMaxOutputBytes {
		return nil, mcpcontract.DeepWikiOutput{}, mcpcontract.InvalidArgument("max_output_bytes", "must be between 1024 and 1048576", map[string]any{"max_output_bytes": mcpcontract.DeepWikiDefaultOutputBytes})
	}
	op, ok := s.reader.(ResearchReader)
	if !ok {
		return nil, mcpcontract.DeepWikiOutput{}, errors.New("DeepWiki is not available")
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
