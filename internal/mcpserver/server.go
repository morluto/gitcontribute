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

func validateRepo(in RepoInput) error {
	if strings.TrimSpace(in.Owner) == "" || strings.TrimSpace(in.Repo) == "" {
		return errors.New("owner and repo are required")
	}
	return nil
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
