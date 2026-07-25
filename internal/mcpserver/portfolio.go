package mcpserver

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func (s *Server) listPullRequestPortfolio(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.ListPullRequestPortfolioInput) (*mcp.CallToolResult, mcpcontract.ListPullRequestPortfolioOutput, error) {
	if in.State == "" {
		in.State = "open"
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.ResponseFormat == "" {
		in.ResponseFormat = "concise"
	}
	reader, ok := s.reader.(PortfolioReader)
	if !ok {
		return nil, mcpcontract.ListPullRequestPortfolioOutput{}, errors.New("portfolio reads are not available")
	}
	out, err := reader.ListPullRequestPortfolio(ctx, in)
	return nil, out, err
}

func (s *Server) findPortfolioOverlaps(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.FindPortfolioOverlapsInput) (*mcp.CallToolResult, mcpcontract.FindPortfolioOverlapsOutput, error) {
	for _, candidate := range in.Candidates {
		if candidate.Kind != "opportunity" && candidate.Kind != "workspace" && candidate.Kind != "pull_request" {
			return nil, mcpcontract.FindPortfolioOverlapsOutput{}, mcpcontract.InvalidArgument("candidates", "candidate kind must be opportunity, workspace, or pull_request", map[string]any{"candidates": []map[string]string{{"kind": "opportunity", "ref": "<id>"}}})
		}
		if strings.TrimSpace(candidate.Ref) == "" {
			return nil, mcpcontract.FindPortfolioOverlapsOutput{}, mcpcontract.InvalidArgument("candidates", "candidate ref is required", nil)
		}
	}
	for _, pullRequest := range in.PullRequests {
		if err := validateThreadRef(pullRequest, true); err != nil {
			return nil, mcpcontract.FindPortfolioOverlapsOutput{}, err
		}
		if pullRequest.Kind != "" && pullRequest.Kind != "pull_request" {
			return nil, mcpcontract.FindPortfolioOverlapsOutput{}, mcpcontract.InvalidArgument("pull_requests", "kind must be pull_request when provided", map[string]any{"kind": "pull_request"})
		}
	}
	reader, ok := s.reader.(PortfolioReader)
	if !ok {
		return nil, mcpcontract.FindPortfolioOverlapsOutput{}, errors.New("portfolio reads are not available")
	}
	out, err := reader.FindPortfolioOverlaps(ctx, in)
	return nil, out, err
}

func validateThreadRef(ref mcpcontract.ThreadRef, kindOptional bool) error {
	if strings.TrimSpace(ref.Owner) == "" || strings.TrimSpace(ref.Repo) == "" {
		return mcpcontract.InvalidArgument("threads", "owner and repo are required", map[string]any{"owner": "acme", "repo": "rocket", "number": 1})
	}
	if ref.Number < 1 {
		return mcpcontract.InvalidArgument("threads", "number must be positive", map[string]any{"owner": ref.Owner, "repo": ref.Repo, "number": 1})
	}
	if ref.Kind == "" && kindOptional {
		return nil
	}
	if ref.Kind != "issue" && ref.Kind != "pull_request" {
		return mcpcontract.InvalidArgument("threads", "kind must be issue or pull_request", map[string]any{"kind": "pull_request"})
	}
	return nil
}

func (s *Server) linkPullRequest(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.LinkPullRequestInput) (*mcp.CallToolResult, mcpcontract.LinkPullRequestOutput, error) {
	operator, ok := s.reader.(PortfolioOperator)
	if !ok {
		return nil, mcpcontract.LinkPullRequestOutput{}, errors.New("portfolio linking is not available")
	}
	out, err := operator.LinkPullRequest(ctx, in)
	return nil, out, err
}
