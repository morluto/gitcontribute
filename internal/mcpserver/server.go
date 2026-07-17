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
)

// ErrNotFound lets readers distinguish absent corpus objects from failures.
var ErrNotFound = errors.New("not found")

// Reader is the local, read-only application boundary exposed through MCP.
// Implementations must not perform network access.
type Reader interface {
	Search(context.Context, SearchInput) (SearchOutput, error)
	Repository(context.Context, RepoInput) (RepositoryOutput, error)
	Thread(context.Context, ThreadInput) (ThreadOutput, error)
	Dossier(context.Context, RepoInput) (DossierOutput, error)
	SearchCode(context.Context, SearchCodeInput) (SearchCodeOutput, error)
	Investigation(context.Context, InvestigationInput) (InvestigationOutput, error)
	ListOpportunities(context.Context, ListOpportunitiesInput) (ListOpportunitiesOutput, error)
	Opportunity(context.Context, OpportunityInput) (OpportunityOutput, error)
	Evidence(context.Context, EvidenceInput) (EvidenceOutput, error)
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
	Query string `json:"query" jsonschema:"Full-text query"`
	Owner string `json:"owner,omitempty" jsonschema:"Optional repository owner"`
	Repo  string `json:"repo,omitempty" jsonschema:"Optional repository name"`
	Kind  string `json:"kind,omitempty" jsonschema:"Optional thread kind"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
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
	Query   string         `json:"query"`
	Matches []ThreadOutput `json:"matches"`
	Total   int            `json:"total"`
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
	Query string `json:"query" jsonschema:"Code search query"`
	Owner string `json:"owner,omitempty" jsonschema:"Optional repository owner"`
	Repo  string `json:"repo,omitempty" jsonschema:"Optional repository name"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
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
	Query   string            `json:"query"`
	Total   int               `json:"total"`
	Matches []CodeMatchOutput `json:"matches"`
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
	var value any
	switch u.Host {
	case "repository", "dossier":
		if len(parts) != 2 {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		input := RepoInput{Owner: parts[0], Repo: parts[1]}
		if u.Host == "repository" {
			value, err = s.reader.Repository(ctx, input)
		} else {
			value, err = s.reader.Dossier(ctx, input)
		}
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
	case "investigation":
		if len(parts) != 1 {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		value, err = s.reader.Investigation(ctx, InvestigationInput{ID: parts[0], HypothesisLimit: 100})
	case "opportunities":
		if len(parts) != 1 {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		value, err = s.reader.ListOpportunities(ctx, ListOpportunitiesInput{InvestigationID: parts[0], Limit: 100})
	case "opportunity":
		if len(parts) != 1 {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		value, err = s.reader.Opportunity(ctx, OpportunityInput{ID: parts[0], EvidenceLimit: 100})
	case "evidence":
		if len(parts) != 2 {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		var in EvidenceInput
		switch parts[0] {
		case "investigation":
			in.InvestigationID = parts[1]
		case "opportunity":
			in.OpportunityID = parts[1]
		default:
			return nil, mcp.ResourceNotFoundError(uri)
		}
		in.Limit = 100
		value, err = s.reader.Evidence(ctx, in)
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
