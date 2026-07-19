package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/lens"
)

// ErrNotFound lets readers distinguish absent corpus objects from failures.
var ErrNotFound = errors.New("not found")

// Reader is the local, read-only application boundary exposed through MCP.
// Implementations must not perform network access.
type Reader interface {
	Search(context.Context, SearchInput) (SearchOutput, error)
	SearchRepositories(context.Context, SearchRepositoriesInput) (SearchRepositoriesOutput, error)
	Repository(context.Context, RepoInput) (RepositoryOutput, error)
	Thread(context.Context, ThreadInput) (ThreadOutput, error)
	ThreadByNumber(context.Context, ThreadByNumberInput) (ThreadOutput, error)
	Dossier(context.Context, RepoInput) (DossierOutput, error)
	SearchCode(context.Context, SearchCodeInput) (SearchCodeOutput, error)
	ExplainMatch(context.Context, ExplainMatchInput) (ExplainMatchOutput, error)
	GetJob(context.Context, GetJobInput) (GetJobOutput, error)
	Investigation(context.Context, InvestigationInput) (InvestigationOutput, error)
	ListOpportunities(context.Context, ListOpportunitiesInput) (ListOpportunitiesOutput, error)
	Opportunity(context.Context, OpportunityInput) (OpportunityOutput, error)
	Evidence(context.Context, EvidenceInput) (EvidenceOutput, error)
	Readiness(context.Context, ReadinessInput) (ReadinessOutput, error)
	FindClusters(context.Context, FindClustersInput) (FindClustersOutput, error)
	GetCoverage(context.Context, GetCoverageInput) (GetCoverageOutput, error)
	Lens(context.Context, LensInput) (LensOutput, error)
}

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
	ListPullRequestPortfolio(context.Context, ListPullRequestPortfolioInput) (ListPullRequestPortfolioOutput, error)
	FindPortfolioOverlaps(context.Context, FindPortfolioOverlapsInput) (FindPortfolioOverlapsOutput, error)
}

// PortfolioOperator owns explicit local links between observed pull requests
// and contribution workflow state. It never mutates GitHub.
type PortfolioOperator interface {
	LinkPullRequest(context.Context, LinkPullRequestInput) (LinkPullRequestOutput, error)
}

// ScalableOperator exposes bounded external reads without combining unrelated
// facets or workflow mutations.
type ScalableOperator interface {
	SearchGitHubRepositories(context.Context, SearchGitHubRepositoriesInput) (SearchGitHubRepositoriesOutput, error)
	SyncRepositoryMetadata(context.Context, SyncRepositoryMetadataInput) (JobReference, error)
	SyncThreads(context.Context, SyncThreadsInput) (JobReference, error)
	HydrateThreads(context.Context, HydrateThreadsInput) (JobReference, error)
	GetAuthenticatedIdentity(context.Context) (AuthenticatedIdentityOutput, error)
	SyncAuthoredPullRequests(context.Context, SyncAuthoredPullRequestsInput) (JobReference, error)
	SyncPullRequestStatus(context.Context, SyncPullRequestStatusInput) (JobReference, error)
	IndexRepositories(context.Context, IndexRepositoriesInput) (JobReference, error)
	CheckMergeConflicts(context.Context, CheckMergeConflictsInput) (CheckMergeConflictsOutput, error)
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
	CreateWorkspace(context.Context, CreateWorkspaceInput) (JobReference, error)
	DefineValidation(context.Context, DefineValidationInput) (ValidationOutput, error)
	RunValidation(context.Context, RunValidationInput) (JobReference, error)
	PrepareContribution(context.Context, PrepareContributionInput) (DraftOutput, error)
	CancelJobs(context.Context, CancelJobInput) (GetJobsOutput, error)
}

// RepoInput identifies a repository for an MCP operation.
type RepoInput struct {
	Owner string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo  string `json:"repo" jsonschema:"GitHub repository name"`
}

// ThreadInput identifies an issue or pull request for an MCP operation.
type ThreadInput struct {
	Owner  string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo   string `json:"repo" jsonschema:"GitHub repository name"`
	Kind   string `json:"kind" jsonschema:"Thread kind: issue or pull_request"`
	Number int    `json:"number" jsonschema:"GitHub issue or pull request number"`
}

// SearchInput describes an offline thread search page.
type SearchInput struct {
	Query        string   `json:"query" jsonschema:"Full-text query"`
	Owner        string   `json:"owner,omitempty" jsonschema:"Optional repository owner"`
	Repo         string   `json:"repo,omitempty" jsonschema:"Optional repository name"`
	Kind         string   `json:"kind,omitempty" jsonschema:"Optional thread kind"`
	State        string   `json:"state,omitempty"`
	StateReason  string   `json:"state_reason,omitempty"`
	Merged       *bool    `json:"merged,omitempty"`
	Author       string   `json:"author,omitempty"`
	Association  string   `json:"author_association,omitempty"`
	Assignee     string   `json:"assignee,omitempty"`
	Labels       []string `json:"labels,omitempty"`
	UpdatedAfter string   `json:"updated_after,omitempty"`
	Limit        int      `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
	Cursor       string   `json:"cursor,omitempty" jsonschema:"Opaque cursor returned by the previous page"`
}

// RepositoryOutput is the stable MCP representation of a repository.
type RepositoryOutput struct {
	Owner     string         `json:"owner"`
	Repo      string         `json:"repo"`
	UpdatedAt string         `json:"updated_at,omitempty"`
	Fields    map[string]any `json:"fields,omitempty"`
}

// ThreadOutput is the stable MCP representation of an issue or pull request.
type ThreadOutput struct {
	Owner             string   `json:"owner"`
	Repo              string   `json:"repo"`
	Kind              string   `json:"kind"`
	Number            int      `json:"number"`
	State             string   `json:"state"`
	StateReason       string   `json:"state_reason,omitempty"`
	Title             string   `json:"title"`
	Body              string   `json:"body,omitempty"`
	Author            string   `json:"author,omitempty"`
	AuthorAssociation string   `json:"author_association,omitempty"`
	Labels            []string `json:"labels,omitempty"`
	Assignees         []string `json:"assignees,omitempty"`
	Draft             bool     `json:"draft,omitempty"`
	ClosedAt          string   `json:"closed_at,omitempty"`
	MergedAt          string   `json:"merged_at,omitempty"`
	Merged            bool     `json:"merged,omitempty"`
	UpdatedAt         string   `json:"updated_at,omitempty"`
}

// SearchOutput contains one page of offline thread matches.
type SearchOutput struct {
	Query      string         `json:"query"`
	Matches    []ThreadOutput `json:"matches"`
	Total      int            `json:"total"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

// DossierOutput contains a persisted repository dossier snapshot.
type DossierOutput struct {
	Owner    string         `json:"owner"`
	Repo     string         `json:"repo"`
	AsOf     string         `json:"as_of,omitempty"`
	Sections map[string]any `json:"sections"`
}

// SourceRef records provenance for an MCP result or workflow artifact.
type SourceRef struct {
	Source     string `json:"source" jsonschema:"Source identifier"`
	URL        string `json:"url,omitempty" jsonschema:"Source URL"`
	CommitSHA  string `json:"commit_sha,omitempty" jsonschema:"Source commit SHA"`
	ObservedAt string `json:"observed_at,omitempty" jsonschema:"Observation timestamp"`
	AsOf       string `json:"as_of,omitempty" jsonschema:"As-of timestamp"`
}

// SearchCodeInput describes an offline code search page.
type SearchCodeInput struct {
	Query  string `json:"query" jsonschema:"Code search query"`
	Owner  string `json:"owner,omitempty" jsonschema:"Optional repository owner"`
	Repo   string `json:"repo,omitempty" jsonschema:"Optional repository name"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
	Cursor string `json:"cursor,omitempty" jsonschema:"Opaque cursor returned by the previous page"`
}

// CodeMatchOutput identifies one stored code match.
type CodeMatchOutput struct {
	ID       string `json:"id"`
	Repo     string `json:"repo"`
	Commit   string `json:"commit"`
	Path     string `json:"path"`
	Language string `json:"language,omitempty"`
	Snippet  string `json:"snippet"`
	Bytes    int    `json:"bytes"`
}

// SearchCodeOutput contains one page of offline code matches.
type SearchCodeOutput struct {
	Query      string            `json:"query"`
	Total      int               `json:"total"`
	Matches    []CodeMatchOutput `json:"matches"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

// InvestigationInput selects an investigation and bounds nested hypotheses.
type InvestigationInput struct {
	ID              string `json:"id" jsonschema:"Investigation ID"`
	HypothesisLimit int    `json:"hypothesis_limit,omitempty" jsonschema:"Maximum hypotheses from 1 to 100"`
}

// HypothesisSummary is the compact hypothesis representation nested in an investigation.
type HypothesisSummary struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Category    string `json:"category"`
	Status      string `json:"status"`
	Description string `json:"description,omitempty"`
}

// InvestigationOutput is the stable MCP representation of an investigation.
type InvestigationOutput struct {
	ID              string              `json:"id"`
	Owner           string              `json:"owner"`
	Repo            string              `json:"repo"`
	CommitSHA       string              `json:"commit_sha,omitempty"`
	Lens            string              `json:"lens,omitempty"`
	Status          string              `json:"status"`
	CreatedAt       string              `json:"created_at"`
	UpdatedAt       string              `json:"updated_at"`
	HypothesisTotal int                 `json:"hypothesis_total"`
	Hypotheses      []HypothesisSummary `json:"hypotheses,omitempty"`
}

// ListOpportunitiesInput selects and bounds opportunities for an investigation.
type ListOpportunitiesInput struct {
	InvestigationID string `json:"investigation_id" jsonschema:"Investigation ID"`
	Limit           int    `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
}

// OpportunitySummary is the compact opportunity representation used in lists.
type OpportunitySummary struct {
	ID              string  `json:"id"`
	InvestigationID string  `json:"investigation_id"`
	Title           string  `json:"title"`
	Category        string  `json:"category"`
	Status          string  `json:"status"`
	Confidence      float64 `json:"confidence"`
	CollisionStatus string  `json:"collision_status"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

// ListOpportunitiesOutput contains bounded opportunities for an investigation.
type ListOpportunitiesOutput struct {
	Opportunities []OpportunitySummary `json:"opportunities"`
	Total         int                  `json:"total"`
}

// OpportunityInput selects an opportunity and bounds nested evidence.
type OpportunityInput struct {
	ID            string `json:"id" jsonschema:"Opportunity ID"`
	EvidenceLimit int    `json:"evidence_limit,omitempty" jsonschema:"Maximum evidence IDs from 1 to 100"`
}

// OpportunityOutput is the stable MCP representation of a contribution opportunity.
type OpportunityOutput struct {
	ID                  string      `json:"id"`
	InvestigationID     string      `json:"investigation_id"`
	HypothesisID        string      `json:"hypothesis_id,omitempty"`
	Title               string      `json:"title"`
	ProblemStatement    string      `json:"problem_statement"`
	Category            string      `json:"category"`
	Scope               string      `json:"scope"`
	Impact              string      `json:"impact"`
	Confidence          float64     `json:"confidence"`
	ExpectedEffort      string      `json:"expected_effort,omitempty"`
	Dependencies        []string    `json:"dependencies,omitempty"`
	CollisionStatus     string      `json:"collision_status"`
	MaintainerAlignment string      `json:"maintainer_alignment,omitempty"`
	SourceRefs          []SourceRef `json:"source_refs,omitempty"`
	EvidenceTotal       int         `json:"evidence_total"`
	EvidenceIDs         []string    `json:"evidence_ids,omitempty"`
	Status              string      `json:"status"`
	CreatedAt           string      `json:"created_at"`
	UpdatedAt           string      `json:"updated_at"`
}

// FindClustersInput selects a repository and bounds duplicate clusters.
type FindClustersInput struct {
	Owner string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo  string `json:"repo" jsonschema:"GitHub repository name"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum clusters from 1 to 100"`
}

// FindNeighborsInput selects a thread and bounds similar-thread results.
type FindNeighborsInput struct {
	Owner  string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo   string `json:"repo" jsonschema:"GitHub repository name"`
	Kind   string `json:"kind" jsonschema:"Thread kind: issue or pull_request"`
	Number int    `json:"number" jsonschema:"GitHub issue or pull request number"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum neighbors from 1 to 100"`
}

// NeighborOutput describes one similar stored thread and its score.
type NeighborOutput struct {
	Kind   string  `json:"kind"`
	Owner  string  `json:"owner"`
	Repo   string  `json:"repo"`
	Number int     `json:"number"`
	Title  string  `json:"title"`
	State  string  `json:"state"`
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}

// FindNeighborsOutput contains deterministic neighbors for a stored thread.
type FindNeighborsOutput struct {
	Owner          string           `json:"owner"`
	Repo           string           `json:"repo"`
	Kind           string           `json:"kind"`
	Number         int              `json:"number"`
	SourceRevision string           `json:"source_revision"`
	Neighbors      []NeighborOutput `json:"neighbors"`
}

// ClusterMemberOutput describes one member of a duplicate cluster.
type ClusterMemberOutput struct {
	Kind     string  `json:"kind"`
	Owner    string  `json:"owner"`
	Repo     string  `json:"repo"`
	Number   int     `json:"number"`
	Title    string  `json:"title,omitempty"`
	State    string  `json:"state,omitempty"`
	Score    float64 `json:"score"`
	Reason   string  `json:"reason"`
	Included bool    `json:"included"`
}

// ClusterOutput contains a stable duplicate cluster and its canonical member.
type ClusterOutput struct {
	StableID    string                `json:"stable_id"`
	State       string                `json:"state"`
	Canonical   ClusterMemberOutput   `json:"canonical"`
	MemberCount int                   `json:"member_count"`
	Members     []ClusterMemberOutput `json:"members,omitempty"`
}

// FindClustersOutput contains duplicate clusters for a repository.
type FindClustersOutput struct {
	Owner    string          `json:"owner"`
	Repo     string          `json:"repo"`
	Total    int             `json:"total"`
	Clusters []ClusterOutput `json:"clusters"`
}

// CoverageTarget selects repository-level coverage or, when kind and number
// are both present, coverage for one exact stored thread.
type CoverageTarget struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Kind   string `json:"kind,omitempty"`
	Number int    `json:"number,omitempty"`
}

// GetCoverageInput selects bounded repository or thread facet coverage reads.
type GetCoverageInput struct {
	Targets []CoverageTarget `json:"targets"`
}

// FacetCoverageOutput reports completeness and freshness for one facet.
type FacetCoverageOutput struct {
	Facet     string `json:"facet"`
	Complete  bool   `json:"complete"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at"`
}

// CoverageOutput reports all known coverage for one repository or thread.
type CoverageOutput struct {
	Owner  string                `json:"owner"`
	Repo   string                `json:"repo"`
	Kind   string                `json:"kind,omitempty"`
	Number int                   `json:"number,omitempty"`
	AsOf   string                `json:"as_of"`
	Facets []FacetCoverageOutput `json:"facets"`
}

// GetCoverageOutput preserves target order and isolates missing or invalid
// targets without failing unrelated coverage reads.
type GetCoverageOutput struct {
	Status string                      `json:"status"`
	Items  []BatchItem[CoverageOutput] `json:"items"`
}

// LensInput selects a saved lens by name.
type LensInput struct {
	Name string `json:"name" jsonschema:"Lens name"`
}

// LensOutput contains a saved lens definition and timestamps.
type LensOutput struct {
	Name       string          `json:"name"`
	Definition lens.Definition `json:"definition"`
	CreatedAt  string          `json:"created_at"`
	UpdatedAt  string          `json:"updated_at"`
}

// Server owns the MCP protocol adapter around a local Reader.
type Server struct {
	reader Reader
	server *mcp.Server
}

// New constructs an MCP server over reader and registers all supported tools
// and resources. A blank version is reported as "dev".
func New(reader Reader, version string) *Server {
	if version == "" {
		version = "dev"
	}
	s := &Server{
		reader: reader,
		server: mcp.NewServer(&mcp.Implementation{
			Name:    "gitcontribute",
			Version: version,
		}, &mcp.ServerOptions{Instructions: serverInstructions}),
	}
	s.register()
	return s
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
	addCatalogTool(s.server, catalogTool[SearchCodeInput, SearchCodeOutput]{
		name: ToolSearchCode, title: "Search stored code",
		description: "Search indexed code snapshots in the local corpus and return bounded snippets with repository, commit, and path context. Provide owner and repo together to restrict the search; this tool is offline.",
		annotations: readOnly, input: inputSchema[SearchCodeInput](func(schema *jsonschema.Schema) {
			setRange(schema, "limit", 1, 100)
			setDefault(schema, "limit", 20)
			requireTogether(schema, "owner", "repo")
		}), output: outputSchema[SearchCodeOutput]("One page of stored code matches."), handler: s.searchCode,
	})
	addCatalogTool(s.server, catalogTool[InvestigationInput, InvestigationOutput]{
		name: ToolGetInvestigation, title: "Get investigation",
		description: "Read one local investigation and a bounded set of its hypotheses. Use " + ToolListOpportunities + " separately for promoted contribution opportunities; this tool is offline.",
		annotations: readOnly, input: inputSchema[InvestigationInput](func(schema *jsonschema.Schema) {
			setRange(schema, "hypothesis_limit", 1, 100)
			setDefault(schema, "hypothesis_limit", 20)
		}), output: outputSchema[InvestigationOutput]("Local investigation with bounded hypothesis summaries."), handler: s.investigation,
	})
	addCatalogTool(s.server, catalogTool[ListOpportunitiesInput, ListOpportunitiesOutput]{
		name: ToolListOpportunities, title: "List investigation opportunities",
		description: "List a bounded set of promoted contribution opportunities for one local investigation. Use " + ToolGetOpportunity + " for full details and evidence identifiers; this tool is offline.",
		annotations: readOnly, input: inputSchema[ListOpportunitiesInput](func(schema *jsonschema.Schema) {
			setRange(schema, "limit", 1, 100)
			setDefault(schema, "limit", 20)
		}), output: outputSchema[ListOpportunitiesOutput]("Bounded contribution opportunity summaries."), handler: s.listOpportunities,
	})
	addCatalogTool(s.server, catalogTool[OpportunityInput, OpportunityOutput]{
		name: ToolGetOpportunity, title: "Get contribution opportunity",
		description: "Read one local contribution opportunity with a bounded set of evidence identifiers. Use " + ToolGetEvidence + " to inspect the evidence records themselves; this tool is offline.",
		annotations: readOnly, input: inputSchema[OpportunityInput](func(schema *jsonschema.Schema) {
			setRange(schema, "evidence_limit", 1, 100)
			setDefault(schema, "evidence_limit", 20)
		}), output: outputSchema[OpportunityOutput]("Local contribution opportunity and evidence references."), handler: s.opportunity,
	})
	addCatalogTool(s.server, catalogTool[EvidenceInput, EvidenceOutput]{
		name: ToolGetEvidence, title: "Get stored evidence",
		description: "Read bounded evidence for exactly one investigation or opportunity, optionally filtered by relation. Freshness is derived from local corpus revisions; this tool never refreshes GitHub.",
		annotations: readOnly, input: inputSchema[EvidenceInput](func(schema *jsonschema.Schema) {
			setEnum(schema, "relation", "supporting", "contradicting", "inconclusive", "stale", "invalid")
			setRange(schema, "limit", 1, 100)
			setDefault(schema, "limit", 20)
			requireExactlyOne(schema, "investigation_id", "opportunity_id")
		}), output: outputSchema[EvidenceOutput]("Bounded stored evidence with provenance and derived freshness."), handler: s.evidence,
	})
	addCatalogTool(s.server, catalogTool[ReadinessInput, ReadinessOutput]{
		name: ToolGetReadiness, title: "Get contribution readiness",
		description: "Evaluate deterministic local readiness rules for one opportunity and return pass, warn, block, or unknown checks with evidence and remediation. This is advisory, offline, and does not claim maintainer approval.",
		annotations: readOnly, input: inputSchema[ReadinessInput](noSchemaCustomization),
		output: outputSchema[ReadinessOutput]("Deterministic contribution readiness report."), handler: s.readiness,
	})
	addCatalogTool(s.server, catalogTool[FindClustersInput, FindClustersOutput]{
		name: ToolFindClusters, title: "Find duplicate clusters",
		description: "List bounded duplicate-candidate clusters already computed for one repository. Use this for repository-wide duplicate structure; use " + ToolFindNeighbors + " for one specific thread.",
		annotations: readOnly, input: inputSchema[FindClustersInput](func(schema *jsonschema.Schema) {
			setRange(schema, "limit", 1, 100)
			setDefault(schema, "limit", 20)
		}), output: outputSchema[FindClustersOutput]("Duplicate-candidate clusters for a stored repository."), handler: s.findClusters,
	})
	addCatalogTool(s.server, catalogTool[FindNeighborsInput, FindNeighborsOutput]{
		name: ToolFindNeighbors, title: "Find similar threads",
		description: "Rank stored threads similar to one issue or pull request using transparent deterministic scoring. Use this for a specific source thread; it never contacts GitHub.",
		annotations: readOnly, input: inputSchema[FindNeighborsInput](func(schema *jsonschema.Schema) {
			setEnum(schema, "kind", "issue", "pull_request")
			setMinimum(schema, "number", 1)
			setRange(schema, "limit", 1, 100)
			setDefault(schema, "limit", 10)
		}), output: outputSchema[FindNeighborsOutput]("Similar stored threads with transparent scores."), handler: s.findNeighbors,
	})
	addCatalogTool(s.server, catalogTool[GetCoverageInput, GetCoverageOutput]{
		name: ToolGetCoverage, title: "Get stored facet coverage in one batch",
		description: "Read offline facet coverage for up to 100 repository or exact-thread targets with ordered item-level outcomes.",
		annotations: readOnly, input: inputSchema[GetCoverageInput](func(sc *jsonschema.Schema) { setArrayBounds(sc, "targets", 1, 100) }),
		output: outputSchema[GetCoverageOutput]("Ordered local repository or thread facet coverage."), handler: s.getCoverage,
	})
	addCatalogTool(s.server, catalogTool[LensInput, LensOutput]{
		name: ToolGetLens, title: "Get saved lens",
		description: "Read one saved local ranking lens by name. Use it to explain or reproduce lens-based ranking; this tool is offline and does not modify the lens.",
		annotations: readOnly, input: inputSchema[LensInput](noSchemaCustomization),
		output: outputSchema[LensOutput]("Saved lens definition and timestamps."), handler: s.getLens,
	})
	s.registerResourceTemplates()
	s.registerContributionPrompts()
	s.registerV1()
	s.registerScalable()
}

func boolPtr(v bool) *bool { return &v }

func (s *Server) repository(ctx context.Context, _ *mcp.CallToolRequest, in RepoInput) (*mcp.CallToolResult, RepositoryOutput, error) {
	if err := validateRepo(in); err != nil {
		return nil, RepositoryOutput{}, err
	}
	out, err := s.reader.Repository(ctx, in)
	return nil, out, err
}

func (s *Server) thread(ctx context.Context, _ *mcp.CallToolRequest, in ThreadInput) (*mcp.CallToolResult, ThreadOutput, error) {
	if err := validateRepo(RepoInput{Owner: in.Owner, Repo: in.Repo}); err != nil {
		return nil, ThreadOutput{}, err
	}
	if in.Kind != "issue" && in.Kind != "pull_request" {
		return nil, ThreadOutput{}, errors.New("kind must be issue or pull_request")
	}
	if in.Number < 1 {
		return nil, ThreadOutput{}, errors.New("number must be positive")
	}
	out, err := s.reader.Thread(ctx, in)
	return nil, out, err
}

func (s *Server) searchCode(ctx context.Context, _ *mcp.CallToolRequest, in SearchCodeInput) (*mcp.CallToolResult, SearchCodeOutput, error) {
	if in.Query == "" {
		return nil, SearchCodeOutput{}, errors.New("query is required")
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return nil, SearchCodeOutput{}, errors.New("limit must be between 1 and 100")
	}
	if (in.Owner == "") != (in.Repo == "") {
		return nil, SearchCodeOutput{}, errors.New("owner and repo must be provided together")
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

func (s *Server) getLens(ctx context.Context, _ *mcp.CallToolRequest, in LensInput) (*mcp.CallToolResult, LensOutput, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return nil, LensOutput{}, errors.New("name is required")
	}
	out, err := s.reader.Lens(ctx, in)
	return nil, out, err
}

func validateRepo(in RepoInput) error {
	if strings.TrimSpace(in.Owner) == "" || strings.TrimSpace(in.Repo) == "" {
		return errors.New("owner and repo are required")
	}
	return nil
}

func normalizeID(field, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required", field)
	}
	if len(value) > 128 {
		return "", fmt.Errorf("%s exceeds 128 bytes", field)
	}
	return value, nil
}
