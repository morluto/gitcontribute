package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// ErrNotFound lets readers distinguish absent corpus objects from failures.

// Reader is the local, read-only application boundary exposed through MCP.
// Implementations must not perform network access.

// NeighborReader is the optional local nearest-thread query capability.
type NeighborReader interface {
	FindNeighbors(context.Context, mcpcontract.FindNeighborsInput) (mcpcontract.FindNeighborsOutput, error)
}

// ScalableReader exposes bounded vectorized corpus reads. Implementations must
// remain offline and preserve input order for non-ranked results.
type ScalableReader interface {
	GetRepositories(context.Context, mcpcontract.GetRepositoriesInput) (mcpcontract.GetRepositoriesOutput, error)
	GetThreads(context.Context, mcpcontract.GetThreadsInput) (mcpcontract.GetThreadsOutput, error)
	RankOpportunities(context.Context, mcpcontract.RankOpportunitiesInput) (mcpcontract.RankOpportunitiesOutput, error)
	FindPrecedents(context.Context, mcpcontract.FindPrecedentsInput) (mcpcontract.FindPrecedentsOutput, error)
	GetJobs(context.Context, mcpcontract.GetJobsInput) (mcpcontract.GetJobsOutput, error)
}

// ThreadFacetReader exposes the bounded offline facet metadata surface.
type ThreadFacetReader interface {
	GetThreadFacets(context.Context, mcpcontract.GetThreadFacetsInput) (mcpcontract.GetThreadFacetsOutput, error)
}

// IssueSetReader prepares bounded contribution evidence from exact stored
// issues without requiring or creating durable workflow state.
type IssueSetReader interface {
	PrepareIssueSet(context.Context, mcpcontract.PrepareIssueSetInput) (mcpcontract.PrepareIssueSetOutput, error)
}

// PortfolioReader exposes bounded offline pull-request portfolio reads.
type PortfolioReader interface {
	ListPullRequestPortfolio(context.Context, mcpcontract.ListPullRequestPortfolioInput) (mcpcontract.ListPullRequestPortfolioOutput, error)
	FindPortfolioOverlaps(context.Context, mcpcontract.FindPortfolioOverlapsInput) (mcpcontract.FindPortfolioOverlapsOutput, error)
}

// PortfolioOperator owns explicit local links between observed pull requests
// and contribution workflow state. It never mutates GitHub.
type PortfolioOperator interface {
	LinkPullRequest(context.Context, mcpcontract.LinkPullRequestInput) (mcpcontract.LinkPullRequestOutput, error)
}

// ContributionPreflightReader performs a bounded live read plus local
// worktree inspection without creating or mutating workflow state.
type ContributionPreflightReader interface {
	PreflightContribution(context.Context, mcpcontract.ContributionPreflightInput) (mcpcontract.ContributionPreflightOutput, error)
}

// GitHubOperator exposes bounded GitHub reads that update only the local corpus.
type GitHubOperator interface {
	SearchGitHubRepositories(context.Context, mcpcontract.SearchGitHubRepositoriesInput) (mcpcontract.SearchGitHubRepositoriesOutput, error)
	SyncRepositoryContext(context.Context, mcpcontract.SyncRepositoryContextInput) (mcpcontract.JobReference, error)
	SyncThreads(context.Context, mcpcontract.SyncThreadsInput) (mcpcontract.JobReference, error)
	HydrateThreads(context.Context, mcpcontract.HydrateThreadsInput) (mcpcontract.JobReference, error)
	SyncPortfolio(context.Context, mcpcontract.SyncPortfolioInput) (mcpcontract.JobReference, error)
}

// GitHubAcquisitionOperator exposes synchronous, explicitly bounded live
// GitHub acquisition that writes only local corpus observations and artifacts.
// It is separate from GitHubOperator so older adapters can retain their
// existing capability set while the new acquisition tools are adopted.
type GitHubAcquisitionOperator interface {
	SearchGitHubThreads(context.Context, mcpcontract.SearchGitHubThreadsInput) (mcpcontract.SearchGitHubThreadsOutput, error)
	ReadSourceFiles(context.Context, mcpcontract.ReadSourceFilesInput) (mcpcontract.ReadSourceFilesOutput, error)
}

// CodeSearchBatchReader exposes one bounded offline batch over a shared code
// snapshot scope. It remains separate from Reader so existing local readers
// can retain the single-query compatibility tool.
type CodeSearchBatchReader interface {
	SearchCodeBatch(context.Context, mcpcontract.SearchCodeBatchInput) (mcpcontract.SearchCodeBatchOutput, error)
}

type CoverageOperator interface {
	EnsureCoverage(context.Context, mcpcontract.EnsureCoverageInput) (mcpcontract.JobReference, error)
}

type PullRequestFeedbackOperator interface {
	SyncPullRequestFeedback(context.Context, mcpcontract.SyncPullRequestFeedbackInput) (mcpcontract.JobReference, error)
}

type CIFailureOperator interface {
	SyncCIFailures(context.Context, mcpcontract.SyncCIFailuresInput) (mcpcontract.JobReference, error)
}

// FixPatternOperator owns the bounded repository-level search, finalist
// hydration, and typed report workflow. It performs GitHub reads and updates
// only the local corpus.
type FixPatternOperator interface {
	MineRepositoryFixPatterns(context.Context, mcpcontract.MineRepositoryFixPatternsInput) (mcpcontract.JobReference, error)
}

// FixPatternReader exposes only terminal persisted reports and remains offline.
type FixPatternReader interface {
	GetFixPatternReport(context.Context, string) (mcpcontract.FixPatternReport, error)
}

// FixPatternPreviewReader exposes bounded analytical fix-pattern reads without
// job creation, hydration, persistence, or network access.
type FixPatternPreviewReader interface {
	PreviewRepositoryFixPatterns(context.Context, mcpcontract.PreviewRepositoryFixPatternsInput) (mcpcontract.FixPatternReport, error)
}

// FixPatternWorkflow keeps submission and persisted report retrieval together
// so an advertised workflow never returns an unreadable resource link.
type FixPatternWorkflow interface {
	FixPatternOperator
	FixPatternReader
}

// CodeIndexer safely acquires and indexes repository code.
type CodeIndexer interface {
	IndexRepositories(context.Context, mcpcontract.IndexRepositoriesInput) (mcpcontract.JobReference, error)
}

// CodeIndexReader exposes one immutable indexed-commit handoff through the
// resource plane. It is intentionally separate from the acquisition writer.
type CodeIndexReader interface {
	CodeIndexArtifact(context.Context, string) (mcpcontract.CodeIndexArtifact, error)
}

type GitHubThreadSearchArtifactReader interface {
	ReadGitHubThreadSearchArtifact(context.Context, string) (mcpcontract.GitHubThreadSearchArtifact, error)
}

type SourceBundleArtifactReader interface {
	ReadSourceBundleArtifact(context.Context, string) (mcpcontract.SourceBundleArtifact, error)
}

type SnapshotReader interface {
	ReadSnapshot(context.Context, string) (mcpcontract.CorpusSnapshotArtifact, error)
}

// MergeConflictReader performs local, non-mutating Git comparisons.
type MergeConflictReader interface {
	CheckMergeConflicts(context.Context, mcpcontract.CheckMergeConflictsInput) (mcpcontract.CheckMergeConflictsOutput, error)
}

// WorkspaceCreator exposes managed workspace creation separately from the
// broader contribution workflow capability.
type WorkspaceCreator interface {
	CreateWorkspace(context.Context, mcpcontract.CreateWorkspaceInput) (mcpcontract.JobReference, error)
}

// WorkspaceAdopter exposes non-owning external-worktree registration.
type WorkspaceAdopter interface {
	AdoptWorkspace(context.Context, mcpcontract.AdoptWorkspaceInput) (mcpcontract.AdoptWorkspaceOutput, error)
}

// ResearchReader exposes external derived repository context.
type ResearchReader interface {
	DeepWiki(context.Context, mcpcontract.DeepWikiInput) (mcpcontract.DeepWikiOutput, error)
}

// Operator is the optional explicit network-read/local-write capability.
type Operator interface {
	BuildRepositoryDossier(context.Context, mcpcontract.BuildRepositoryDossierInput) (mcpcontract.JobReference, error)
	StartInvestigation(context.Context, mcpcontract.StartInvestigationInput) (mcpcontract.InvestigationOutput, error)
	RecordHypothesis(context.Context, mcpcontract.RecordHypothesisInput) (mcpcontract.HypothesisOutput, error)
	CheckDuplicates(context.Context, mcpcontract.CheckDuplicatesInput) (mcpcontract.CheckOutput, error)
	CheckCollisions(context.Context, mcpcontract.CheckCollisionsInput) (mcpcontract.CheckOutput, error)
	PromoteOpportunity(context.Context, mcpcontract.PromoteOpportunityInput) (mcpcontract.OpportunityOutput, error)
	DefineValidation(context.Context, mcpcontract.DefineValidationInput) (mcpcontract.ValidationOutput, error)
	RunValidation(context.Context, mcpcontract.RunValidationInput) (mcpcontract.JobReference, error)
	PrepareContribution(context.Context, mcpcontract.PrepareContributionInput) (mcpcontract.DraftOutput, error)
	Draft(context.Context, mcpcontract.DraftInput) (mcpcontract.DraftOutput, error)
	ExportManifest(context.Context, mcpcontract.ExportManifestInput) (mcpcontract.ManifestOutput, error)
	Manifest(context.Context, mcpcontract.ManifestInput) (mcpcontract.ManifestOutput, error)
	CancelJobs(context.Context, mcpcontract.CancelJobInput) (mcpcontract.GetJobsOutput, error)
}

type ValidationReceiptOperator interface {
	AttachValidationReceipt(context.Context, mcpcontract.AttachValidationReceiptInput) (mcpcontract.ExternalValidationReceiptOutput, error)
}

type PublishedDraftVerifier interface {
	VerifyPublishedDraft(context.Context, mcpcontract.VerifyPublishedDraftInput) (mcpcontract.PublishedDraftVerificationOutput, error)
}

// RepoInput identifies a repository for an MCP operation.

// ThreadInput identifies an issue or pull request for an MCP operation.

// SearchInput describes an offline thread search page.

// RepositoryOutput is the stable MCP representation of a repository.

// ThreadOutput is the stable MCP representation of an issue or pull request.

// SearchOutput contains one page of offline thread matches.

// DossierOutput contains a persisted repository dossier snapshot.

// SourceRef records provenance for an MCP result or workflow artifact.

// SearchCodeInput describes an offline code search page.

// CodeMatchOutput identifies one stored code match.

// CodeIndexCoverageOutput reports one selected snapshot's indexing coverage.

// SearchCodeOutput contains one page of offline code matches.

// InvestigationInput selects an investigation and bounds nested hypotheses.

// HypothesisSummary is the compact hypothesis representation nested in an investigation.

// InvestigationOutput is the stable MCP representation of an investigation.

// ListOpportunitiesInput selects and bounds opportunities for an investigation.

// OpportunitySummary is the compact opportunity representation used in lists.

// ListOpportunitiesOutput contains bounded opportunities for an investigation.

// OpportunityInput selects an opportunity and bounds nested evidence.

// OpportunityOutput is the stable MCP representation of a contribution opportunity.

// ClusterTarget selects one repository or exact cluster member.

// FindClustersInput selects bounded cluster targets.

// FindNeighborsInput selects bounded source threads.

// NeighborOutput describes one similar stored thread and its score.

// NeighborSetOutput contains deterministic neighbors for one stored thread.

// FindNeighborsOutput preserves source-thread order and item failures.

// ClusterMemberOutput describes one member of a duplicate cluster.

// ClusterOutput contains a stable duplicate cluster and its canonical member.

// ClusterSetOutput contains duplicate clusters for one repository target.

// FindClustersOutput preserves target order and item failures.

// CoverageTarget selects repository-level coverage or, when kind and number
// are both present, coverage for one exact stored thread.

// GetCoverageInput selects bounded repository or thread facet coverage reads.

// FacetCoverageOutput reports completeness and freshness for one facet.

// CoverageOutput reports all known coverage for one repository or thread.

// GetCoverageOutput preserves target order and isolates missing or invalid
// targets without failing unrelated coverage reads.

// LensInput selects a saved lens by name.

// LensOutput contains a saved lens definition and timestamps.

// Server owns the MCP protocol adapter around a local Reader.
type Server struct {
	reader          mcpcontract.Reader
	server          *mcp.Server
	registrationErr error
	readOnly        bool
}

// New constructs an MCP server with the unified catalog.
func New(reader mcpcontract.Reader, version string) (*Server, error) {
	return newServer(reader, version, false)
}

// NewReadOnly constructs an MCP server that advertises only read-only tools.
func NewReadOnly(reader mcpcontract.Reader, version string) (*Server, error) {
	return newServer(reader, version, true)
}

func newServer(reader mcpcontract.Reader, version string, readOnly bool) (*Server, error) {
	if version == "" {
		version = "dev"
	}
	s := &Server{
		reader:   reader,
		readOnly: readOnly,
		server: mcp.NewServer(&mcp.Implementation{
			Name:    "gitcontribute",
			Version: version,
		}, &mcp.ServerOptions{Instructions: serverInstructions}),
	}
	s.register()
	if s.registrationErr != nil {
		return nil, s.registrationErr
	}
	return s, nil
}

func (s *Server) recordRegistrationError(tool, direction string, err error) {
	if s.registrationErr == nil {
		s.registrationErr = fmt.Errorf("register MCP tool %q %s schema: %w", tool, direction, err)
	}
}

// MCP returns the underlying SDK server for embedding in another transport.
func (s *Server) MCP() *mcp.Server { return s.server }

// ServeStdio serves MCP messages over standard input and output until the
// context is cancelled or the transport stops.
func (s *Server) ServeStdio(ctx context.Context) error {
	return s.server.Run(ctx, &mcp.StdioTransport{})
}

func (s *Server) register() {
	readOnly := readOnlyAnnotations()
	addCatalogTool(s, catalogTool[mcpcontract.SearchCodeInput, mcpcontract.SearchCodeOutput]{
		name: mcpcontract.ToolSearchCode, title: "Search stored code",
		description: "Search indexed code and return bounded snippets plus selected-snapshot coverage, including for zero scoped matches. Missing, unknown, or truncated indexes include an exact typed code.index_repositories recovery action, while additional result pages include a typed cursor action. Optional owner/repo scope; offline.",
		annotations: readOnly, input: inputSchema[mcpcontract.SearchCodeInput](func(schema *schemaBuilder) {
			setRange(schema, "limit", 1, 100)
			setDefault(schema, "limit", 20)
			requireTogether(schema, "owner", "repo")
		}), output: outputSchema[mcpcontract.SearchCodeOutput]("One page of stored code matches."), handler: s.searchCode,
	})
	addCatalogTool(s, catalogTool[mcpcontract.SearchCodeBatchInput, mcpcontract.SearchCodeBatchOutput]{
		name:        mcpcontract.ToolSearchCodeBatch,
		title:       "Search stored code in one batch",
		description: "Run up to 20 ordered code searches against one shared immutable local corpus revision. This is offline, performs no GitHub fallback or mutation, and preserves each query's coverage and truncation semantics; corpus.search_code remains available for one query.",
		annotations: readOnly, supportedBy: supports[CodeSearchBatchReader],
		input: inputSchema[mcpcontract.SearchCodeBatchInput](func(sc *schemaBuilder) {
			requireTogether(sc, "owner", "repo")
			setArrayBounds(sc, "queries", 1, 20)
			setRange(sc, "limit", 1, 100)
			setDefault(sc, "limit", 20)
		}),
		output: outputSchema[mcpcontract.SearchCodeBatchOutput]("Ordered offline code-search results over one shared corpus revision."), handler: s.searchCodeBatch,
	})
	addCatalogTool(s, catalogTool[mcpcontract.FindClustersInput, mcpcontract.FindClustersOutput]{
		name: mcpcontract.ToolFindClusters, title: "Find duplicate clusters in one batch",
		description: "Read stored duplicate clusters for up to 20 repository or exact-member targets in input order. This does not recompute similarity; use " + mcpcontract.ToolFindNeighbors + " for transient nearest-thread scoring. Offline.",
		annotations: readOnly, input: inputSchema[mcpcontract.FindClustersInput](func(schema *schemaBuilder) {
			setArrayBounds(schema, "targets", 1, 20)
			setRange(schema, "limit", 1, 100)
			setDefault(schema, "limit", 20)
			if target := schema.schema.Defs["ClusterTarget"]; target != nil {
				targetSchema := &schemaBuilder{schema: target, err: schema.err}
				setEnum(targetSchema, "kind", "issue", "pull_request")
				setMinimum(targetSchema, "number", 1)
				requireTogether(targetSchema, "kind", "number")
			}
		}), output: outputSchema[mcpcontract.FindClustersOutput]("Ordered stored duplicate-cluster results with typed larger-limit recovery for truncated populations."), handler: s.findClusters,
	})
	addCatalogTool(s, catalogTool[mcpcontract.FindNeighborsInput, mcpcontract.FindNeighborsOutput]{
		name: mcpcontract.ToolFindNeighbors, title: "Find similar threads in one batch",
		description: "Rank stored threads similar to up to 20 exact source threads with transparent deterministic scoring and ordered item outcomes. This is transient offline similarity, not a stored duplicate-cluster read.",
		annotations: readOnly, supportedBy: supports[NeighborReader], input: inputSchema[mcpcontract.FindNeighborsInput](func(schema *schemaBuilder) {
			setArrayBounds(schema, "threads", 1, 20)
			setRange(schema, "limit", 1, 100)
			setDefault(schema, "limit", 10)
			if thread := schema.schema.Defs["ThreadRef"]; thread != nil {
				threadSchema := &schemaBuilder{schema: thread, err: schema.err}
				setEnum(threadSchema, "kind", "issue", "pull_request")
				setMinimum(threadSchema, "number", 1)
			}
		}), output: outputSchema[mcpcontract.FindNeighborsOutput]("Ordered similar-thread results with transparent scores."), handler: s.findNeighbors,
	})
	addCatalogTool(s, catalogTool[mcpcontract.GetCoverageInput, mcpcontract.GetCoverageOutput]{
		name: mcpcontract.ToolGetCoverage, title: "Get stored facet coverage in one batch",
		description: "Read offline facet coverage for up to 100 repository or exact-thread targets with ordered item-level outcomes. Missing or incomplete facets are unknown and return a typed recovery action; follow that exact or repository action, poll jobs.get, then reread.",
		annotations: readOnly, input: inputSchema[mcpcontract.GetCoverageInput](func(sc *schemaBuilder) {
			setArrayBounds(sc, "targets", 1, 100)
			configureCoverageTargetFields(sc)
		}),
		output: outputSchema[mcpcontract.GetCoverageOutput]("Ordered local repository or thread facet coverage."), handler: s.getCoverage,
	})
	s.registerResourceTemplates()
	s.registerContributionPrompts()
	s.registerV1()
	s.registerScalable()
	s.registerCommitPlanning()
}

func boolPtr(v bool) *bool { return &v }

func (s *Server) searchCode(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.SearchCodeInput) (*mcp.CallToolResult, mcpcontract.SearchCodeOutput, error) {
	if in.Query == "" {
		return nil, mcpcontract.SearchCodeOutput{}, mcpcontract.InvalidArgument("query", "is required", map[string]any{"query": "MIDI"})
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return nil, mcpcontract.SearchCodeOutput{}, mcpcontract.InvalidArgument("limit", "must be between 1 and 100", map[string]any{"limit": 20})
	}
	if (in.Owner == "") != (in.Repo == "") {
		return nil, mcpcontract.SearchCodeOutput{}, mcpcontract.InvalidArgument("owner", "owner and repo must be provided together", map[string]any{"owner": "acme", "repo": "synth"})
	}
	out, err := s.reader.SearchCode(ctx, in)
	return nil, out, err
}

func (s *Server) searchCodeBatch(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.SearchCodeBatchInput) (*mcp.CallToolResult, mcpcontract.SearchCodeBatchOutput, error) {
	if len(in.Queries) < 1 || len(in.Queries) > 20 {
		return nil, mcpcontract.SearchCodeBatchOutput{}, mcpcontract.InvalidArgument("queries", "must contain 1 to 20 items", map[string]any{"queries": []string{"MIDI", "latency"}})
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return nil, mcpcontract.SearchCodeBatchOutput{}, mcpcontract.InvalidArgument("limit", "must be between 1 and 100", map[string]any{"limit": 20})
	}
	if in.Owner == "" || in.Repo == "" {
		return nil, mcpcontract.SearchCodeBatchOutput{}, mcpcontract.InvalidArgument("owner", "owner and repo are required", map[string]any{"owner": "acme", "repo": "synth"})
	}
	reader, ok := s.reader.(CodeSearchBatchReader)
	if !ok {
		return nil, mcpcontract.SearchCodeBatchOutput{}, errors.New("batched offline code search is not available")
	}
	out, err := reader.SearchCodeBatch(ctx, in)
	return nil, out, err
}

func (s *Server) findClusters(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.FindClustersInput) (*mcp.CallToolResult, mcpcontract.FindClustersOutput, error) {
	if len(in.Targets) < 1 || len(in.Targets) > 20 {
		return nil, mcpcontract.FindClustersOutput{}, mcpcontract.InvalidArgument("targets", "must contain 1 to 20 items", map[string]any{
			"targets": []map[string]string{{"owner": "acme", "repo": "rocket"}},
		})
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return nil, mcpcontract.FindClustersOutput{}, mcpcontract.InvalidArgument("limit", "must be between 1 and 100", map[string]any{"limit": 20})
	}
	out, err := s.reader.FindClusters(ctx, in)
	return nil, out, err
}

func (s *Server) findNeighbors(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.FindNeighborsInput) (*mcp.CallToolResult, mcpcontract.FindNeighborsOutput, error) {
	if len(in.Threads) < 1 || len(in.Threads) > 20 {
		return nil, mcpcontract.FindNeighborsOutput{}, mcpcontract.InvalidArgument("threads", "must contain 1 to 20 items", map[string]any{
			"threads": []map[string]any{{"owner": "acme", "repo": "rocket", "kind": "issue", "number": 1}},
		})
	}
	if in.Limit == 0 {
		in.Limit = 10
	}
	if in.Limit < 1 || in.Limit > 100 {
		return nil, mcpcontract.FindNeighborsOutput{}, mcpcontract.InvalidArgument("limit", "must be between 1 and 100", map[string]any{"limit": 10})
	}
	reader, ok := s.reader.(NeighborReader)
	if !ok {
		return nil, mcpcontract.FindNeighborsOutput{}, errors.New("neighbor queries are not available")
	}
	out, err := reader.FindNeighbors(ctx, in)
	return nil, out, err
}

func (s *Server) getCoverage(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.GetCoverageInput) (*mcp.CallToolResult, mcpcontract.GetCoverageOutput, error) {
	if len(in.Targets) < 1 || len(in.Targets) > 100 {
		return nil, mcpcontract.GetCoverageOutput{}, mcpcontract.InvalidArgument("targets", "must contain 1 to 100 items", map[string]any{
			"targets": []any{map[string]any{"type": "repository", "repository": map[string]string{"owner": "acme", "repo": "rocket"}}},
		})
	}
	for i, target := range in.Targets {
		repositoryLevel := target.Type == mcpcontract.CoverageTargetRepository && target.Thread == nil
		exactThread := target.Type == mcpcontract.CoverageTargetExactThread && target.Thread != nil && (target.Thread.Kind == "issue" || target.Thread.Kind == "pull_request") && target.Thread.Number > 0
		if !repositoryLevel && !exactThread {
			return nil, mcpcontract.GetCoverageOutput{}, mcpcontract.InvalidArgument(fmt.Sprintf("targets[%d]", i), "must be a valid repository or exact_thread variant", map[string]any{"type": "exact_thread", "repository": map[string]string{"owner": "acme", "repo": "rocket"}, "thread": map[string]any{"kind": "issue", "number": 42}})
		}
	}
	out, err := s.reader.GetCoverage(ctx, in)
	return nil, out, err
}

func validateRepo(in mcpcontract.RepoInput) error {
	if strings.TrimSpace(in.Owner) == "" || strings.TrimSpace(in.Repo) == "" {
		return mcpcontract.InvalidArgument("owner", "owner and repo are required together", map[string]any{"owner": "acme", "repo": "rocket"})
	}
	return nil
}

func normalizeID(field, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", mcpcontract.InvalidArgument(field, "is required", map[string]any{field: "<id>"})
	}
	if len(value) > 128 {
		return "", mcpcontract.InvalidArgument(field, "must not exceed 128 bytes", map[string]any{field: "<id>"})
	}
	return value, nil
}
