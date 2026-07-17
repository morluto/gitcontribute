package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

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
	FindClusters(context.Context, FindClustersInput) (FindClustersOutput, error)
	GetCoverage(context.Context, GetCoverageInput) (GetCoverageOutput, error)
	Lens(context.Context, LensInput) (LensOutput, error)
}

// NeighborReader is the optional local nearest-thread query capability.
type NeighborReader interface {
	FindNeighbors(context.Context, FindNeighborsInput) (FindNeighborsOutput, error)
}

// Operator is the optional explicit network-read/local-write capability.
type Operator interface {
	SyncRepository(context.Context, SyncRepositoryInput) (SyncRepositoryOutput, error)
	HydrateThread(context.Context, HydrateThreadInput) (HydrateThreadOutput, error)
	HydrateRepository(context.Context, HydrateRepositoryInput) (JobReference, error)
	BuildRepositoryDossier(context.Context, BuildRepositoryDossierInput) (JobReference, error)
	StartCrawl(context.Context, StartCrawlInput) (JobReference, error)
	StartInvestigation(context.Context, StartInvestigationInput) (InvestigationOutput, error)
	RecordHypothesis(context.Context, RecordHypothesisInput) (HypothesisOutput, error)
	CheckDuplicates(context.Context, CheckDuplicatesInput) (CheckOutput, error)
	CheckCollisions(context.Context, CheckCollisionsInput) (CheckOutput, error)
	PromoteOpportunity(context.Context, PromoteOpportunityInput) (OpportunityOutput, error)
	CreateWorkspace(context.Context, CreateWorkspaceInput) (JobReference, error)
	DefineValidation(context.Context, DefineValidationInput) (ValidationOutput, error)
	RunValidation(context.Context, RunValidationInput) (JobReference, error)
	PrepareContribution(context.Context, PrepareContributionInput) (DraftOutput, error)
	CancelJob(context.Context, CancelJobInput) (GetJobOutput, error)
}

type RepoInput struct {
	Owner string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo  string `json:"repo" jsonschema:"GitHub repository name"`
}

type ThreadInput struct {
	Owner  string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo   string `json:"repo" jsonschema:"GitHub repository name"`
	Kind   string `json:"kind" jsonschema:"Thread kind: issue or pull_request"`
	Number int    `json:"number" jsonschema:"GitHub issue or pull request number"`
}

type SearchInput struct {
	Query  string `json:"query" jsonschema:"Full-text query"`
	Owner  string `json:"owner,omitempty" jsonschema:"Optional repository owner"`
	Repo   string `json:"repo,omitempty" jsonschema:"Optional repository name"`
	Kind   string `json:"kind,omitempty" jsonschema:"Optional thread kind"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
	Cursor string `json:"cursor,omitempty" jsonschema:"Opaque cursor returned by the previous page"`
}

type RepositoryOutput struct {
	Owner     string         `json:"owner"`
	Repo      string         `json:"repo"`
	UpdatedAt string         `json:"updated_at,omitempty"`
	Fields    map[string]any `json:"fields,omitempty"`
}

type ThreadOutput struct {
	Owner     string   `json:"owner"`
	Repo      string   `json:"repo"`
	Kind      string   `json:"kind"`
	Number    int      `json:"number"`
	State     string   `json:"state"`
	Title     string   `json:"title"`
	Body      string   `json:"body,omitempty"`
	Author    string   `json:"author,omitempty"`
	Labels    []string `json:"labels,omitempty"`
	UpdatedAt string   `json:"updated_at,omitempty"`
}

type SearchOutput struct {
	Query      string         `json:"query"`
	Matches    []ThreadOutput `json:"matches"`
	Total      int            `json:"total"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

type DossierOutput struct {
	Owner    string         `json:"owner"`
	Repo     string         `json:"repo"`
	AsOf     string         `json:"as_of,omitempty"`
	Sections map[string]any `json:"sections"`
}

type SourceRef struct {
	Source     string `json:"source" jsonschema:"Source identifier"`
	URL        string `json:"url,omitempty" jsonschema:"Source URL"`
	CommitSHA  string `json:"commit_sha,omitempty" jsonschema:"Source commit SHA"`
	ObservedAt string `json:"observed_at,omitempty" jsonschema:"Observation timestamp"`
	AsOf       string `json:"as_of,omitempty" jsonschema:"As-of timestamp"`
}

type SearchCodeInput struct {
	Query  string `json:"query" jsonschema:"Code search query"`
	Owner  string `json:"owner,omitempty" jsonschema:"Optional repository owner"`
	Repo   string `json:"repo,omitempty" jsonschema:"Optional repository name"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
	Cursor string `json:"cursor,omitempty" jsonschema:"Opaque cursor returned by the previous page"`
}

type CodeMatchOutput struct {
	ID       string `json:"id"`
	Repo     string `json:"repo"`
	Commit   string `json:"commit"`
	Path     string `json:"path"`
	Language string `json:"language,omitempty"`
	Snippet  string `json:"snippet"`
	Bytes    int    `json:"bytes"`
}

type SearchCodeOutput struct {
	Query      string            `json:"query"`
	Total      int               `json:"total"`
	Matches    []CodeMatchOutput `json:"matches"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

type InvestigationInput struct {
	ID              string `json:"id" jsonschema:"Investigation ID"`
	HypothesisLimit int    `json:"hypothesis_limit,omitempty" jsonschema:"Maximum hypotheses from 1 to 100"`
}

type HypothesisSummary struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Category    string `json:"category"`
	Status      string `json:"status"`
	Description string `json:"description,omitempty"`
}

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

type ListOpportunitiesInput struct {
	InvestigationID string `json:"investigation_id" jsonschema:"Investigation ID"`
	Limit           int    `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
}

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

type ListOpportunitiesOutput struct {
	Opportunities []OpportunitySummary `json:"opportunities"`
	Total         int                  `json:"total"`
}

type OpportunityInput struct {
	ID            string `json:"id" jsonschema:"Opportunity ID"`
	EvidenceLimit int    `json:"evidence_limit,omitempty" jsonschema:"Maximum evidence IDs from 1 to 100"`
}

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

type EvidenceInput struct {
	InvestigationID string `json:"investigation_id,omitempty" jsonschema:"Filter by investigation ID"`
	OpportunityID   string `json:"opportunity_id,omitempty" jsonschema:"Filter by opportunity ID"`
	Relation        string `json:"relation,omitempty" jsonschema:"Optional relation filter: supporting, contradicting, inconclusive, stale, invalid"`
	Limit           int    `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
}

type EvidenceItem struct {
	ID          string      `json:"id"`
	Type        string      `json:"type"`
	Relation    string      `json:"relation"`
	Description string      `json:"description"`
	SourceRefs  []SourceRef `json:"source_refs,omitempty"`
	CreatedAt   string      `json:"created_at"`
}

type EvidenceOutput struct {
	InvestigationID string         `json:"investigation_id,omitempty"`
	OpportunityID   string         `json:"opportunity_id,omitempty"`
	Total           int            `json:"total"`
	Evidence        []EvidenceItem `json:"evidence"`
}

type FindClustersInput struct {
	Owner string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo  string `json:"repo" jsonschema:"GitHub repository name"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum clusters from 1 to 100"`
}

type FindNeighborsInput struct {
	Owner  string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo   string `json:"repo" jsonschema:"GitHub repository name"`
	Kind   string `json:"kind" jsonschema:"Thread kind: issue or pull_request"`
	Number int    `json:"number" jsonschema:"GitHub issue or pull request number"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum neighbors from 1 to 100"`
}

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

type FindNeighborsOutput struct {
	Owner          string           `json:"owner"`
	Repo           string           `json:"repo"`
	Kind           string           `json:"kind"`
	Number         int              `json:"number"`
	SourceRevision string           `json:"source_revision"`
	Neighbors      []NeighborOutput `json:"neighbors"`
}

type SyncRepositoryInput struct {
	Owner    string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo     string `json:"repo" jsonschema:"GitHub repository name"`
	State    string `json:"state,omitempty" jsonschema:"Thread state: open, closed, or all"`
	Since    string `json:"since,omitempty" jsonschema:"Optional Go duration such as 720h"`
	Numbers  []int  `json:"numbers,omitempty" jsonschema:"Optional exact issue or pull request numbers"`
	MaxPages int    `json:"max_pages,omitempty" jsonschema:"Maximum issue-list pages from 1 to 1000"`
}

type SyncRepositoryOutput struct {
	Owner   string `json:"owner"`
	Repo    string `json:"repo"`
	Updated int    `json:"updated"`
	Message string `json:"message"`
}

type HydrateThreadInput struct {
	Owner    string   `json:"owner" jsonschema:"GitHub repository owner"`
	Repo     string   `json:"repo" jsonschema:"GitHub repository name"`
	Number   int      `json:"number" jsonschema:"GitHub issue or pull request number"`
	Facets   []string `json:"facets,omitempty" jsonschema:"Facets to hydrate; empty selects all applicable facets"`
	MaxPages int      `json:"max_pages,omitempty" jsonschema:"Maximum pages per facet from 1 to 100"`
}

type HydratedFacetOutput struct {
	Facet    string `json:"facet"`
	Count    int    `json:"count"`
	Pages    int    `json:"pages"`
	Complete bool   `json:"complete"`
}

type HydrateThreadOutput struct {
	Owner    string                `json:"owner"`
	Repo     string                `json:"repo"`
	Number   int                   `json:"number"`
	Kind     string                `json:"kind"`
	Requests int                   `json:"requests"`
	Facets   []HydratedFacetOutput `json:"facets"`
	Message  string                `json:"message"`
}

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

type ClusterOutput struct {
	StableID    string                `json:"stable_id"`
	State       string                `json:"state"`
	Canonical   ClusterMemberOutput   `json:"canonical"`
	MemberCount int                   `json:"member_count"`
	Members     []ClusterMemberOutput `json:"members,omitempty"`
}

type FindClustersOutput struct {
	Owner    string          `json:"owner"`
	Repo     string          `json:"repo"`
	Total    int             `json:"total"`
	Clusters []ClusterOutput `json:"clusters"`
}

type GetCoverageInput struct {
	Owner string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo  string `json:"repo" jsonschema:"GitHub repository name"`
}

type FacetCoverageOutput struct {
	Facet     string `json:"facet"`
	Complete  bool   `json:"complete"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at"`
}

type GetCoverageOutput struct {
	Owner  string                `json:"owner"`
	Repo   string                `json:"repo"`
	AsOf   string                `json:"as_of"`
	Facets []FacetCoverageOutput `json:"facets"`
}

type LensInput struct {
	Name string `json:"name" jsonschema:"Lens name"`
}

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

func New(reader Reader, version string) *Server {
	if version == "" {
		version = "dev"
	}
	s := &Server{
		reader: reader,
		server: mcp.NewServer(&mcp.Implementation{
			Name:    "gitcontribute",
			Version: version,
		}, nil),
	}
	s.register()
	return s
}

func (s *Server) MCP() *mcp.Server { return s.server }

func (s *Server) ServeStdio(ctx context.Context) error {
	return s.server.Run(ctx, &mcp.StdioTransport{})
}

func (s *Server) register() {
	annotations := &mcp.ToolAnnotations{
		Title:           "Read local GitContribute corpus",
		ReadOnlyHint:    true,
		IdempotentHint:  true,
		OpenWorldHint:   boolPtr(false),
		DestructiveHint: boolPtr(false),
	}
	operationAnnotations := &mcp.ToolAnnotations{
		Title:           "Read GitHub and update the local GitContribute corpus",
		ReadOnlyHint:    false,
		IdempotentHint:  false,
		OpenWorldHint:   boolPtr(true),
		DestructiveHint: boolPtr(false),
	}
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "search",
		Description: "Search the local GitContribute corpus without network access",
		Annotations: annotations,
	}, s.search)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_repository",
		Description: "Read one repository from the local corpus",
		Annotations: annotations,
	}, s.repository)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_thread",
		Description: "Read one issue or pull request from the local corpus",
		Annotations: annotations,
	}, s.thread)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_dossier",
		Description: "Read a source-backed repository dossier from the local corpus",
		Annotations: annotations,
	}, s.dossier)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "search_code",
		Description: "Search indexed code snapshots in the local corpus without network access",
		Annotations: annotations,
	}, s.searchCode)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_investigation",
		Description: "Read a local investigation workspace from the corpus",
		Annotations: annotations,
	}, s.investigation)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "list_opportunities",
		Description: "List opportunities for a local investigation",
		Annotations: annotations,
	}, s.listOpportunities)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_opportunity",
		Description: "Read a local contribution opportunity",
		Annotations: annotations,
	}, s.opportunity)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_evidence",
		Description: "Read evidence for a local investigation or opportunity",
		Annotations: annotations,
	}, s.evidence)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "find_clusters",
		Description: "List duplicate-candidate clusters for a repository in the local corpus without network access",
		Annotations: annotations,
	}, s.findClusters)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "find_neighbors",
		Description: "Rank similar threads from the local corpus with transparent scoring and no network access",
		Annotations: annotations,
	}, s.findNeighbors)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_coverage",
		Description: "Read facet coverage and freshness for a repository in the local corpus without network access",
		Annotations: annotations,
	}, s.getCoverage)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_lens",
		Description: "Read a saved lens definition from the local corpus",
		Annotations: annotations,
	}, s.getLens)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "sync_repository",
		Description: "Explicitly read repository threads from GitHub and update the local corpus",
		Annotations: operationAnnotations,
	}, s.syncRepository)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "hydrate_thread",
		Description: "Explicitly read selected issue or pull request facets from GitHub and update the local corpus",
		Annotations: operationAnnotations,
	}, s.hydrateThread)

	s.server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitcontribute://repository/{owner}/{repo}",
		Name:        "Repository",
		Description: "Local repository record",
		MIMEType:    "application/json",
	}, s.readResource)
	s.server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitcontribute://thread/{owner}/{repo}/{kind}/{number}",
		Name:        "Thread",
		Description: "Local issue or pull request",
		MIMEType:    "application/json",
	}, s.readResource)
	s.server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitcontribute://dossier/{owner}/{repo}",
		Name:        "Dossier",
		Description: "Local source-backed repository dossier",
		MIMEType:    "application/json",
	}, s.readResource)
	s.server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitcontribute://investigation/{id}",
		Name:        "Investigation",
		Description: "Local investigation workspace",
		MIMEType:    "application/json",
	}, s.readResource)
	s.server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitcontribute://opportunities/{investigation_id}",
		Name:        "Opportunities",
		Description: "Local opportunities for an investigation",
		MIMEType:    "application/json",
	}, s.readResource)
	s.server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitcontribute://opportunity/{id}",
		Name:        "Opportunity",
		Description: "Local contribution opportunity",
		MIMEType:    "application/json",
	}, s.readResource)
	s.server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitcontribute://evidence/{scope}/{id}",
		Name:        "Evidence",
		Description: "Local evidence for an investigation or opportunity",
		MIMEType:    "application/json",
	}, s.readResource)
	s.server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitcontribute://lens/{name}",
		Name:        "Lens",
		Description: "Saved lens definition",
		MIMEType:    "application/json",
	}, s.readResource)

	s.registerV1()
}

func boolPtr(v bool) *bool { return &v }

func (s *Server) search(ctx context.Context, _ *mcp.CallToolRequest, in SearchInput) (*mcp.CallToolResult, SearchOutput, error) {
	if in.Query == "" {
		return nil, SearchOutput{}, errors.New("query is required")
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return nil, SearchOutput{}, errors.New("limit must be between 1 and 100")
	}
	out, err := s.reader.Search(ctx, in)
	return nil, out, err
}

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

func (s *Server) dossier(ctx context.Context, _ *mcp.CallToolRequest, in RepoInput) (*mcp.CallToolResult, DossierOutput, error) {
	if err := validateRepo(in); err != nil {
		return nil, DossierOutput{}, err
	}
	out, err := s.reader.Dossier(ctx, in)
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
	if err := validateRepo(RepoInput{Owner: in.Owner, Repo: in.Repo}); err != nil {
		return nil, GetCoverageOutput{}, err
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

func (s *Server) syncRepository(ctx context.Context, _ *mcp.CallToolRequest, in SyncRepositoryInput) (*mcp.CallToolResult, SyncRepositoryOutput, error) {
	if err := validateRepo(RepoInput{Owner: in.Owner, Repo: in.Repo}); err != nil {
		return nil, SyncRepositoryOutput{}, err
	}
	if in.State == "" {
		in.State = "all"
	}
	if in.State != "open" && in.State != "closed" && in.State != "all" {
		return nil, SyncRepositoryOutput{}, errors.New("state must be open, closed, or all")
	}
	if in.MaxPages == 0 {
		in.MaxPages = 1000
	}
	if in.MaxPages < 1 || in.MaxPages > 1000 {
		return nil, SyncRepositoryOutput{}, errors.New("max_pages must be between 1 and 1000")
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, SyncRepositoryOutput{}, errors.New("repository sync is not available")
	}
	out, err := operator.SyncRepository(ctx, in)
	return nil, out, err
}

func (s *Server) hydrateThread(ctx context.Context, _ *mcp.CallToolRequest, in HydrateThreadInput) (*mcp.CallToolResult, HydrateThreadOutput, error) {
	if err := validateRepo(RepoInput{Owner: in.Owner, Repo: in.Repo}); err != nil {
		return nil, HydrateThreadOutput{}, err
	}
	if in.Number <= 0 {
		return nil, HydrateThreadOutput{}, errors.New("number must be positive")
	}
	if in.MaxPages == 0 {
		in.MaxPages = 50
	}
	if in.MaxPages < 1 || in.MaxPages > 100 {
		return nil, HydrateThreadOutput{}, errors.New("max_pages must be between 1 and 100")
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, HydrateThreadOutput{}, errors.New("thread hydration is not available")
	}
	out, err := operator.HydrateThread(ctx, in)
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

func (s *Server) readResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	uri := req.Params.URI
	u, err := url.Parse(uri)
	if err != nil {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	scheme := u.Scheme
	var value any
	switch u.Host {
	case "repository", "repositories":
		if len(parts) != 2 {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		input := RepoInput{Owner: parts[0], Repo: parts[1]}
		value, err = s.reader.Repository(ctx, input)
	case "dossier", "dossiers":
		if len(parts) != 2 {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		input := RepoInput{Owner: parts[0], Repo: parts[1]}
		value, err = s.reader.Dossier(ctx, input)
	case "thread":
		if len(parts) != 4 {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		number, parseErr := strconv.Atoi(parts[3])
		if parseErr != nil || number < 1 {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		value, err = s.reader.Thread(ctx, ThreadInput{
			Owner: parts[0], Repo: parts[1], Kind: parts[2], Number: number,
		})
	case "threads":
		if len(parts) != 3 {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		number, parseErr := strconv.Atoi(parts[2])
		if parseErr != nil || number < 1 {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		value, err = s.reader.ThreadByNumber(ctx, ThreadByNumberInput{
			Owner: parts[0], Repo: parts[1], Number: number,
		})
	case "investigation", "investigations":
		if len(parts) != 1 {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		value, err = s.reader.Investigation(ctx, InvestigationInput{ID: parts[0], HypothesisLimit: 100})
	case "opportunities":
		if len(parts) != 1 {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		if scheme == "github-index" {
			value, err = s.reader.Opportunity(ctx, OpportunityInput{ID: parts[0], EvidenceLimit: 100})
		} else {
			value, err = s.reader.ListOpportunities(ctx, ListOpportunitiesInput{InvestigationID: parts[0], Limit: 100})
		}
	case "opportunity":
		if len(parts) != 1 {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		value, err = s.reader.Opportunity(ctx, OpportunityInput{ID: parts[0], EvidenceLimit: 100})
	case "evidence":
		var in EvidenceInput
		if scheme == "github-index" {
			if len(parts) != 1 {
				return nil, mcp.ResourceNotFoundError(uri)
			}
			in.InvestigationID = parts[0]
		} else {
			if len(parts) != 2 {
				return nil, mcp.ResourceNotFoundError(uri)
			}
			switch parts[0] {
			case "investigation":
				in.InvestigationID = parts[1]
			case "opportunity":
				in.OpportunityID = parts[1]
			default:
				return nil, mcp.ResourceNotFoundError(uri)
			}
		}
		in.Limit = 100
		value, err = s.reader.Evidence(ctx, in)
	case "lens", "lenses":
		if len(parts) != 1 {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		value, err = s.reader.Lens(ctx, LensInput{Name: parts[0]})
	case "job", "jobs":
		if len(parts) != 1 {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		value, err = s.reader.GetJob(ctx, GetJobInput{ID: parts[0]})
	default:
		return nil, mcp.ResourceNotFoundError(uri)
	}
	if errors.Is(err, ErrNotFound) {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", uri, err)
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", uri, err)
	}
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
		URI: uri, MIMEType: "application/json", Text: string(payload),
	}}}, nil
}
