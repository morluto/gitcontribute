package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ErrNotFound lets readers distinguish absent corpus objects from failures.

// Reader is the local, read-only application boundary exposed through MCP.
// Implementations must not perform network access.

// NeighborReader is the optional local nearest-thread query capability.
type NeighborReader interface {
	FindNeighbors(context.Context, FindNeighborsInput) (FindNeighborsOutput, error)
}

// ScalableReader exposes bounded vectorized corpus reads. Implementations must
// remain offline and preserve input order for non-ranked results.
type ScalableReader interface {
	GetRepositories(context.Context, GetRepositoriesInput) (GetRepositoriesOutput, error)
	GetThreads(context.Context, GetThreadsInput) (GetThreadsOutput, error)
	RankOpportunities(context.Context, RankOpportunitiesInput) (RankOpportunitiesOutput, error)
	FindPrecedents(context.Context, FindPrecedentsInput) (FindPrecedentsOutput, error)
	GetJobs(context.Context, GetJobsInput) (GetJobsOutput, error)
}

// PortfolioReader exposes bounded offline pull-request portfolio reads.
type PortfolioReader interface {
	ListPullRequestPortfolio(context.Context, ListPullRequestPortfolioInput) (ListPullRequestPortfolioOutput, error)
	FindPortfolioOverlaps(context.Context, FindPortfolioOverlapsInput) (FindPortfolioOverlapsOutput, error)
}

// PortfolioOperator owns explicit local links between observed pull requests
// and contribution workflow state. It never mutates GitHub.
type PortfolioOperator interface {
	LinkPullRequest(context.Context, LinkPullRequestInput) (LinkPullRequestOutput, error)
}

// GitHubOperator exposes bounded GitHub reads that update only the local corpus.
type GitHubOperator interface {
	SearchGitHubRepositories(context.Context, SearchGitHubRepositoriesInput) (SearchGitHubRepositoriesOutput, error)
	SyncRepositoryMetadata(context.Context, SyncRepositoryMetadataInput) (JobReference, error)
	SyncThreads(context.Context, SyncThreadsInput) (JobReference, error)
	HydrateThreads(context.Context, HydrateThreadsInput) (JobReference, error)
	GetAuthenticatedIdentity(context.Context) (AuthenticatedIdentityOutput, error)
	SyncAuthoredPullRequests(context.Context, SyncAuthoredPullRequestsInput) (JobReference, error)
	SyncPullRequestStatus(context.Context, SyncPullRequestStatusInput) (JobReference, error)
}

// CodeIndexer safely acquires and indexes repository code.
type CodeIndexer interface {
	IndexRepositories(context.Context, IndexRepositoriesInput) (JobReference, error)
}

// MergeConflictReader performs local, non-mutating Git comparisons.
type MergeConflictReader interface {
	CheckMergeConflicts(context.Context, CheckMergeConflictsInput) (CheckMergeConflictsOutput, error)
}

// WorkspaceCreator exposes managed workspace creation separately from the
// broader contribution workflow capability.
type WorkspaceCreator interface {
	CreateWorkspace(context.Context, CreateWorkspaceInput) (JobReference, error)
}

// WorkspaceAdopter exposes non-owning external-worktree registration.
type WorkspaceAdopter interface {
	AdoptWorkspace(context.Context, AdoptWorkspaceInput) (AdoptWorkspaceOutput, error)
}

// ResearchReader exposes external derived repository context.
type ResearchReader interface {
	DeepWiki(context.Context, DeepWikiInput) (DeepWikiOutput, error)
}

// Operator is the optional explicit network-read/local-write capability.
type Operator interface {
	BuildRepositoryDossier(context.Context, BuildRepositoryDossierInput) (JobReference, error)
	StartInvestigation(context.Context, StartInvestigationInput) (InvestigationOutput, error)
	RecordHypothesis(context.Context, RecordHypothesisInput) (HypothesisOutput, error)
	CheckDuplicates(context.Context, CheckDuplicatesInput) (CheckOutput, error)
	CheckCollisions(context.Context, CheckCollisionsInput) (CheckOutput, error)
	PromoteOpportunity(context.Context, PromoteOpportunityInput) (OpportunityOutput, error)
	DefineValidation(context.Context, DefineValidationInput) (ValidationOutput, error)
	RunValidation(context.Context, RunValidationInput) (JobReference, error)
	RunRepeatedValidation(context.Context, RunRepeatedValidationInput) (JobReference, error)
	PrepareContribution(context.Context, PrepareContributionInput) (DraftOutput, error)
	ExportManifest(context.Context, ExportManifestInput) (ManifestOutput, error)
	CancelJobs(context.Context, CancelJobInput) (GetJobsOutput, error)
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

// FindClustersInput selects a repository and bounds duplicate clusters.

// FindNeighborsInput selects a thread and bounds similar-thread results.

// NeighborOutput describes one similar stored thread and its score.

// FindNeighborsOutput contains deterministic neighbors for a stored thread.

// ClusterMemberOutput describes one member of a duplicate cluster.

// ClusterOutput contains a stable duplicate cluster and its canonical member.

// FindClustersOutput contains duplicate clusters for a repository.

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
	reader          Reader
	server          *mcp.Server
	registrationErr error
	enabledTools    map[string]struct{}
	readOnly        bool
}

// Options selects MCP capability profiles. An empty Toolsets list is rejected.

// New constructs an MCP server over reader and registers all supported tools
// and resources. A blank version is reported as "dev".
func New(reader Reader, version string) (*Server, error) {
	return NewWithOptions(reader, version, Options{Toolsets: []string{"all"}})
}

// NewWithOptions constructs an MCP server with selected capability profiles.
func NewWithOptions(reader Reader, version string, opts Options) (*Server, error) {
	if version == "" {
		version = "dev"
	}
	if len(opts.Toolsets) == 0 {
		return nil, errors.New("at least one MCP toolset is required")
	}
	for i := range opts.Toolsets {
		opts.Toolsets[i] = strings.TrimSpace(opts.Toolsets[i])
		if opts.Toolsets[i] != "all" {
			if _, ok := toolsets[opts.Toolsets[i]]; !ok {
				return nil, fmt.Errorf("unknown MCP toolset %q", opts.Toolsets[i])
			}
		}
	}
	enabled := enabledToolNames(opts.Toolsets)
	s := &Server{
		reader:       reader,
		enabledTools: enabled,
		readOnly:     opts.ReadOnly,
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
	addCatalogTool(s, catalogTool[SearchCodeInput, SearchCodeOutput]{
		name: ToolSearchCode, title: "Search stored code",
		description: "Search indexed code and return bounded snippets plus selected-snapshot coverage, including for zero scoped matches. Optional owner/repo scope; offline.",
		annotations: readOnly, input: inputSchema[SearchCodeInput](func(schema *schemaBuilder) {
			setRange(schema, "limit", 1, 100)
			setDefault(schema, "limit", 20)
			requireTogether(schema, "owner", "repo")
		}), output: outputSchema[SearchCodeOutput]("One page of stored code matches."), handler: s.searchCode,
	})
	addCatalogTool(s, catalogTool[InvestigationInput, InvestigationOutput]{
		name: ToolGetInvestigation, title: "Get investigation",
		description: "Read one local investigation and a bounded set of its hypotheses. Use " + ToolListOpportunities + " separately for promoted contribution opportunities; this tool is offline.",
		annotations: readOnly, input: inputSchema[InvestigationInput](func(schema *schemaBuilder) {
			setRange(schema, "hypothesis_limit", 1, 100)
			setDefault(schema, "hypothesis_limit", 20)
		}), output: outputSchema[InvestigationOutput]("Local investigation with bounded hypothesis summaries."), handler: s.investigation,
	})
	addCatalogTool(s, catalogTool[ListOpportunitiesInput, ListOpportunitiesOutput]{
		name: ToolListOpportunities, title: "List investigation opportunities",
		description: "List a bounded set of promoted contribution opportunities for one local investigation. Use " + ToolGetOpportunity + " for full details and evidence identifiers; this tool is offline.",
		annotations: readOnly, input: inputSchema[ListOpportunitiesInput](func(schema *schemaBuilder) {
			setRange(schema, "limit", 1, 100)
			setDefault(schema, "limit", 20)
		}), output: outputSchema[ListOpportunitiesOutput]("Bounded contribution opportunity summaries."), handler: s.listOpportunities,
	})
	addCatalogTool(s, catalogTool[OpportunityInput, OpportunityOutput]{
		name: ToolGetOpportunity, title: "Get contribution opportunity",
		description: "Read one local contribution opportunity with a bounded set of evidence identifiers. Use " + ToolGetEvidence + " to inspect the evidence records themselves; this tool is offline.",
		annotations: readOnly, input: inputSchema[OpportunityInput](func(schema *schemaBuilder) {
			setRange(schema, "evidence_limit", 1, 100)
			setDefault(schema, "evidence_limit", 20)
		}), output: outputSchema[OpportunityOutput]("Local contribution opportunity and evidence references."), handler: s.opportunity,
	})
	addCatalogTool(s, catalogTool[EvidenceInput, EvidenceOutput]{
		name: ToolGetEvidence, title: "Get stored evidence",
		description: "Read bounded evidence for exactly one investigation or opportunity, optionally filtered by relation. Freshness is derived from local corpus revisions; this tool never refreshes GitHub.",
		annotations: readOnly, input: inputSchema[EvidenceInput](func(schema *schemaBuilder) {
			setEnum(schema, "relation", "supporting", "contradicting", "inconclusive", "stale", "invalid")
			setRange(schema, "limit", 1, 100)
			setDefault(schema, "limit", 20)
			requireExactlyOne(schema, "investigation_id", "opportunity_id")
		}), output: outputSchema[EvidenceOutput]("Bounded stored evidence with provenance and derived freshness."), handler: s.evidence,
	})
	addCatalogTool(s, catalogTool[ReadinessInput, ReadinessOutput]{
		name: ToolGetReadiness, title: "Get contribution readiness",
		description: "Evaluate deterministic local readiness rules for one opportunity and return pass, warn, block, or unknown checks with evidence and remediation. This is advisory, offline, and does not claim maintainer approval.",
		annotations: readOnly, input: inputSchema[ReadinessInput](noSchemaCustomization),
		output: outputSchema[ReadinessOutput]("Deterministic contribution readiness report."), handler: s.readiness,
	})
	addCatalogTool(s, catalogTool[FindClustersInput, FindClustersOutput]{
		name: ToolFindClusters, title: "Find duplicate clusters",
		description: "List stored duplicate clusters for a repository, or provide kind and number to read the current cluster containing one exact member. Use " + ToolFindNeighbors + " to compute similarity outside the stored projection.",
		annotations: readOnly, input: inputSchema[FindClustersInput](func(schema *schemaBuilder) {
			setEnum(schema, "kind", "issue", "pull_request")
			setMinimum(schema, "number", 1)
			requireTogether(schema, "kind", "number")
			setRange(schema, "limit", 1, 100)
			setDefault(schema, "limit", 20)
		}), output: outputSchema[FindClustersOutput]("Stored duplicate clusters."), handler: s.findClusters,
	})
	addCatalogTool(s, catalogTool[FindNeighborsInput, FindNeighborsOutput]{
		name: ToolFindNeighbors, title: "Find similar threads",
		description: "Rank stored threads similar to one issue or pull request using transparent deterministic scoring. Use this for a specific source thread; it never contacts GitHub.",
		annotations: readOnly, supportedBy: supports[NeighborReader], input: inputSchema[FindNeighborsInput](func(schema *schemaBuilder) {
			setEnum(schema, "kind", "issue", "pull_request")
			setMinimum(schema, "number", 1)
			setRange(schema, "limit", 1, 100)
			setDefault(schema, "limit", 10)
		}), output: outputSchema[FindNeighborsOutput]("Similar stored threads with transparent scores."), handler: s.findNeighbors,
	})
	addCatalogTool(s, catalogTool[GetCoverageInput, GetCoverageOutput]{
		name: ToolGetCoverage, title: "Get stored facet coverage in one batch",
		description: "Read offline facet coverage for up to 100 repository or exact-thread targets with ordered item-level outcomes.",
		annotations: readOnly, input: inputSchema[GetCoverageInput](func(sc *schemaBuilder) { setArrayBounds(sc, "targets", 1, 100) }),
		output: outputSchema[GetCoverageOutput]("Ordered local repository or thread facet coverage."), handler: s.getCoverage,
	})
	s.registerResourceTemplates()
	s.registerContributionPrompts()
	s.registerV1()
	s.registerScalable()
	s.registerCommitPlanning()
}

func boolPtr(v bool) *bool { return &v }

func (s *Server) searchCode(ctx context.Context, _ *mcp.CallToolRequest, in SearchCodeInput) (*mcp.CallToolResult, SearchCodeOutput, error) {
	if in.Query == "" {
		return nil, SearchCodeOutput{}, InvalidArgument("query", "is required", map[string]any{"query": "MIDI"})
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return nil, SearchCodeOutput{}, InvalidArgument("limit", "must be between 1 and 100", map[string]any{"limit": 20})
	}
	if (in.Owner == "") != (in.Repo == "") {
		return nil, SearchCodeOutput{}, InvalidArgument("owner", "owner and repo must be provided together", map[string]any{"owner": "acme", "repo": "synth"})
	}
	out, err := s.reader.SearchCode(ctx, in)
	return nil, out, err
}

func (s *Server) investigation(ctx context.Context, _ *mcp.CallToolRequest, in InvestigationInput) (*mcp.CallToolResult, InvestigationOutput, error) {
	id, err := normalizeID("id", in.ID)
	if err != nil {
		return nil, InvestigationOutput{}, err
	}
	in.ID = id
	if in.HypothesisLimit == 0 {
		in.HypothesisLimit = 20
	}
	if in.HypothesisLimit < 1 || in.HypothesisLimit > 100 {
		return nil, InvestigationOutput{}, errors.New("hypothesis_limit must be between 1 and 100")
	}
	out, err := s.reader.Investigation(ctx, in)
	return nil, out, err
}

func (s *Server) listOpportunities(ctx context.Context, _ *mcp.CallToolRequest, in ListOpportunitiesInput) (*mcp.CallToolResult, ListOpportunitiesOutput, error) {
	id, err := normalizeID("investigation_id", in.InvestigationID)
	if err != nil {
		return nil, ListOpportunitiesOutput{}, err
	}
	in.InvestigationID = id
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return nil, ListOpportunitiesOutput{}, errors.New("limit must be between 1 and 100")
	}
	out, err := s.reader.ListOpportunities(ctx, in)
	return nil, out, err
}

func (s *Server) opportunity(ctx context.Context, _ *mcp.CallToolRequest, in OpportunityInput) (*mcp.CallToolResult, OpportunityOutput, error) {
	id, err := normalizeID("id", in.ID)
	if err != nil {
		return nil, OpportunityOutput{}, err
	}
	in.ID = id
	if in.EvidenceLimit == 0 {
		in.EvidenceLimit = 20
	}
	if in.EvidenceLimit < 1 || in.EvidenceLimit > 100 {
		return nil, OpportunityOutput{}, errors.New("evidence_limit must be between 1 and 100")
	}
	out, err := s.reader.Opportunity(ctx, in)
	return nil, out, err
}

func (s *Server) evidence(ctx context.Context, _ *mcp.CallToolRequest, in EvidenceInput) (*mcp.CallToolResult, EvidenceOutput, error) {
	in.InvestigationID = strings.TrimSpace(in.InvestigationID)
	in.OpportunityID = strings.TrimSpace(in.OpportunityID)
	if (in.InvestigationID == "") == (in.OpportunityID == "") {
		return nil, EvidenceOutput{}, errors.New("exactly one of investigation_id or opportunity_id is required")
	}
	if in.InvestigationID != "" {
		if _, err := normalizeID("investigation_id", in.InvestigationID); err != nil {
			return nil, EvidenceOutput{}, err
		}
	} else if _, err := normalizeID("opportunity_id", in.OpportunityID); err != nil {
		return nil, EvidenceOutput{}, err
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return nil, EvidenceOutput{}, errors.New("limit must be between 1 and 100")
	}
	out, err := s.reader.Evidence(ctx, in)
	return nil, out, err
}

func (s *Server) readiness(ctx context.Context, _ *mcp.CallToolRequest, in ReadinessInput) (*mcp.CallToolResult, ReadinessOutput, error) {
	id, err := normalizeID("opportunity_id", in.OpportunityID)
	if err != nil {
		return nil, ReadinessOutput{}, err
	}
	in.OpportunityID = id
	out, err := s.reader.Readiness(ctx, in)
	return nil, out, err
}

func (s *Server) findClusters(ctx context.Context, _ *mcp.CallToolRequest, in FindClustersInput) (*mcp.CallToolResult, FindClustersOutput, error) {
	if err := validateRepo(RepoInput{Owner: in.Owner, Repo: in.Repo}); err != nil {
		return nil, FindClustersOutput{}, err
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return nil, FindClustersOutput{}, errors.New("limit must be between 1 and 100")
	}
	out, err := s.reader.FindClusters(ctx, in)
	return nil, out, err
}

func (s *Server) findNeighbors(ctx context.Context, _ *mcp.CallToolRequest, in FindNeighborsInput) (*mcp.CallToolResult, FindNeighborsOutput, error) {
	if err := validateRepo(RepoInput{Owner: in.Owner, Repo: in.Repo}); err != nil {
		return nil, FindNeighborsOutput{}, err
	}
	if in.Kind != "issue" && in.Kind != "pull_request" {
		return nil, FindNeighborsOutput{}, errors.New("kind must be issue or pull_request")
	}
	if in.Number <= 0 {
		return nil, FindNeighborsOutput{}, errors.New("number must be positive")
	}
	if in.Limit == 0 {
		in.Limit = 10
	}
	if in.Limit < 1 || in.Limit > 100 {
		return nil, FindNeighborsOutput{}, errors.New("limit must be between 1 and 100")
	}
	reader, ok := s.reader.(NeighborReader)
	if !ok {
		return nil, FindNeighborsOutput{}, errors.New("neighbor queries are not available")
	}
	out, err := reader.FindNeighbors(ctx, in)
	return nil, out, err
}

func (s *Server) getCoverage(ctx context.Context, _ *mcp.CallToolRequest, in GetCoverageInput) (*mcp.CallToolResult, GetCoverageOutput, error) {
	if len(in.Targets) < 1 || len(in.Targets) > 100 {
		return nil, GetCoverageOutput{}, errors.New("targets must contain 1 to 100 items")
	}
	out, err := s.reader.GetCoverage(ctx, in)
	return nil, out, err
}

func validateRepo(in RepoInput) error {
	if strings.TrimSpace(in.Owner) == "" || strings.TrimSpace(in.Repo) == "" {
		return InvalidArgument("owner", "owner and repo are required together", map[string]any{"owner": "acme", "repo": "rocket"})
	}
	return nil
}

func normalizeID(field, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", InvalidArgument(field, "is required", map[string]any{field: "<id>"})
	}
	if len(value) > 128 {
		return "", InvalidArgument(field, "must not exceed 128 bytes", map[string]any{field: "<id>"})
	}
	return value, nil
}
