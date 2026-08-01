package mcpserver

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func (s *Server) registerGitHubAcquisitionTools() {
	addCatalogTool(s, catalogTool[mcpcontract.SearchGitHubThreadsInput, mcpcontract.SearchGitHubThreadsOutput]{
		name:        mcpcontract.ToolSearchGitHubThreads,
		title:       "Search live GitHub issues and pull requests",
		description: "Run one bounded live GitHub issue-search page for one repository and persist the returned observations plus an immutable query artifact. The result is not repository-wide thread coverage and cannot prove absence; incomplete pages and additional pages include exact typed retry or pagination actions.",
		annotations: networkReadAnnotations(), supportedBy: supports[GitHubAcquisitionOperator],
		input: inputSchema[mcpcontract.SearchGitHubThreadsInput](func(sc *schemaBuilder) {
			requireTogether(sc, "owner", "repo")
			setEnum(sc, "kind", "issue", "pull_request")
			setEnum(sc, "state", "open", "closed", "all")
			setEnum(sc, "sort", "comments", "created", "updated", "reactions")
			setEnum(sc, "order", "asc", "desc")
			setRange(sc, "page", 1, 1000)
			setDefault(sc, "page", 1)
			setRange(sc, "limit", 1, 100)
			setDefault(sc, "limit", 20)
		}),
		output: outputSchema[mcpcontract.SearchGitHubThreadsOutput]("Compact live GitHub search results with an immutable local artifact link."), handler: s.searchGitHubThreads,
	})
	addCatalogTool(s, catalogTool[mcpcontract.ReadSourceFilesInput, mcpcontract.ReadSourceFilesOutput]{
		name:        mcpcontract.ToolReadSourceFiles,
		title:       "Read bounded GitHub source files",
		description: "Acquire up to 20 ordered repository-relative source files from one explicit commit or named ref, resolving named refs to an authoritative commit. Per-file and total-byte bounds produce item-level outcomes with typed retry or larger-bound actions; content is untrusted text and is available through an immutable local source-bundle artifact.",
		annotations: networkReadAnnotations(), supportedBy: supports[GitHubAcquisitionOperator],
		input: inputSchema[mcpcontract.ReadSourceFilesInput](func(sc *schemaBuilder) {
			requireTogether(sc, "owner", "repo")
			setArrayBounds(sc, "files", 1, 20)
			setRange(sc, "per_file_bytes", 1, 1024*1024)
			setDefault(sc, "per_file_bytes", 256*1024)
			setRange(sc, "total_bytes", 1, 4*1024*1024)
			setDefault(sc, "total_bytes", 2*1024*1024)
		}),
		output: outputSchema[mcpcontract.ReadSourceFilesOutput]("Bounded source-file acquisition results with an immutable source-bundle resource link."), handler: s.readSourceFiles,
	})
}

func (s *Server) searchGitHubThreads(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.SearchGitHubThreadsInput) (*mcp.CallToolResult, mcpcontract.SearchGitHubThreadsOutput, error) {
	if err := validateLiveRepository(in.Owner, in.Repo); err != nil {
		return nil, mcpcontract.SearchGitHubThreadsOutput{}, err
	}
	if strings.TrimSpace(in.Query) == "" {
		return nil, mcpcontract.SearchGitHubThreadsOutput{}, mcpcontract.InvalidArgument("query", "is required", map[string]any{"query": "regression"})
	}
	operator, ok := s.reader.(GitHubAcquisitionOperator)
	if !ok {
		return nil, mcpcontract.SearchGitHubThreadsOutput{}, errors.New("live GitHub thread search is not available")
	}
	out, err := operator.SearchGitHubThreads(ctx, in)
	if err != nil {
		return nil, out, err
	}
	if out.ResourceURI == "" {
		return nil, out, nil
	}
	return linkedResource(out.ResourceURI, "github-thread-search", "GitHub thread search artifact", "Immutable provider query result persisted in the local corpus."), out, nil
}

func (s *Server) readSourceFiles(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.ReadSourceFilesInput) (*mcp.CallToolResult, mcpcontract.ReadSourceFilesOutput, error) {
	if err := validateLiveRepository(in.Owner, in.Repo); err != nil {
		return nil, mcpcontract.ReadSourceFilesOutput{}, err
	}
	if strings.TrimSpace(in.Ref) == "" {
		return nil, mcpcontract.ReadSourceFilesOutput{}, mcpcontract.InvalidArgument("ref", "is required", map[string]any{"ref": "main"})
	}
	if len(in.Files) < 1 || len(in.Files) > 20 {
		return nil, mcpcontract.ReadSourceFilesOutput{}, mcpcontract.InvalidArgument("files", "must contain 1 to 20 items", map[string]any{"files": []map[string]any{{"path": "README.md"}}})
	}
	operator, ok := s.reader.(GitHubAcquisitionOperator)
	if !ok {
		return nil, mcpcontract.ReadSourceFilesOutput{}, errors.New("bounded GitHub source reads are not available")
	}
	out, err := operator.ReadSourceFiles(ctx, in)
	if err != nil {
		return nil, out, err
	}
	if out.ResourceURI == "" {
		return nil, out, nil
	}
	return linkedResource(out.ResourceURI, "source-bundle", "GitHub source bundle", "Immutable bounded source text persisted in the local corpus."), out, nil
}

func validateLiveRepository(owner, repo string) error {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(repo) == "" {
		return mcpcontract.InvalidArgument("owner", "owner and repo are required", map[string]any{"owner": "acme", "repo": "rocket"})
	}
	return nil
}
