package mcpserver

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *Server) listPullRequestPortfolio(ctx context.Context, _ *mcp.CallToolRequest, in ListPullRequestPortfolioInput) (*mcp.CallToolResult, ListPullRequestPortfolioOutput, error) {
	if in.State == "" {
		in.State = "open"
	}
	if in.Limit == 0 {
		in.Limit = 100
	}
	reader, ok := s.reader.(PortfolioReader)
	if !ok {
		return nil, ListPullRequestPortfolioOutput{}, errors.New("portfolio reads are not available")
	}
	out, err := reader.ListPullRequestPortfolio(ctx, in)
	return nil, out, err
}

func (s *Server) findPortfolioOverlaps(ctx context.Context, _ *mcp.CallToolRequest, in FindPortfolioOverlapsInput) (*mcp.CallToolResult, FindPortfolioOverlapsOutput, error) {
	for _, candidate := range in.Candidates {
		if candidate.Kind != "opportunity" && candidate.Kind != "workspace" && candidate.Kind != "pull_request" {
			return nil, FindPortfolioOverlapsOutput{}, InvalidArgument("candidates", "candidate kind must be opportunity, workspace, or pull_request", map[string]any{"candidates": []map[string]string{{"kind": "opportunity", "ref": "<id>"}}})
		}
		if strings.TrimSpace(candidate.Ref) == "" {
			return nil, FindPortfolioOverlapsOutput{}, InvalidArgument("candidates", "candidate ref is required", nil)
		}
	}
	for _, pullRequest := range in.PullRequests {
		if err := validateThreadRef(pullRequest, true); err != nil {
			return nil, FindPortfolioOverlapsOutput{}, err
		}
		if pullRequest.Kind != "" && pullRequest.Kind != "pull_request" {
			return nil, FindPortfolioOverlapsOutput{}, InvalidArgument("pull_requests", "kind must be pull_request when provided", map[string]any{"kind": "pull_request"})
		}
	}
	reader, ok := s.reader.(PortfolioReader)
	if !ok {
		return nil, FindPortfolioOverlapsOutput{}, errors.New("portfolio reads are not available")
	}
	out, err := reader.FindPortfolioOverlaps(ctx, in)
	return nil, out, err
}

func validateThreadRef(ref ThreadRef, kindOptional bool) error {
	if strings.TrimSpace(ref.Owner) == "" || strings.TrimSpace(ref.Repo) == "" {
		return InvalidArgument("threads", "owner and repo are required", map[string]any{"owner": "acme", "repo": "rocket", "number": 1})
	}
	if ref.Number < 1 {
		return InvalidArgument("threads", "number must be positive", map[string]any{"owner": ref.Owner, "repo": ref.Repo, "number": 1})
	}
	if ref.Kind == "" && kindOptional {
		return nil
	}
	if ref.Kind != "issue" && ref.Kind != "pull_request" {
		return InvalidArgument("threads", "kind must be issue or pull_request", map[string]any{"kind": "pull_request"})
	}
	return nil
}

func (s *Server) linkPullRequest(ctx context.Context, _ *mcp.CallToolRequest, in LinkPullRequestInput) (*mcp.CallToolResult, LinkPullRequestOutput, error) {
	operator, ok := s.reader.(PortfolioOperator)
	if !ok {
		return nil, LinkPullRequestOutput{}, errors.New("portfolio linking is not available")
	}
	out, err := operator.LinkPullRequest(ctx, in)
	return nil, out, err
}
