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
	if in.View == "" {
		in.View = "compact"
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

func (s *Server) preflightContribution(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.ContributionPreflightInput) (*mcp.CallToolResult, mcpcontract.ContributionPreflightOutput, error) {
	if err := validateThreadRef(mcpcontract.ThreadRef{Owner: in.Repository.Owner, Repo: in.Repository.Repo, Number: 1}, true); err != nil {
		return nil, mcpcontract.ContributionPreflightOutput{}, mcpcontract.InvalidArgument("repository", "owner and repo are required", map[string]any{"owner": "acme", "repo": "rocket"})
	}
	if in.Fork != nil && (strings.TrimSpace(in.Fork.Owner) == "" || strings.TrimSpace(in.Fork.Repo) == "") {
		return nil, mcpcontract.ContributionPreflightOutput{}, mcpcontract.InvalidArgument("fork", "owner and repo are required when fork is provided", map[string]any{"owner": "alice", "repo": "rocket"})
	}
	if in.Fork != nil && strings.EqualFold(strings.TrimSpace(in.Fork.Owner), strings.TrimSpace(in.Repository.Owner)) && strings.EqualFold(strings.TrimSpace(in.Fork.Repo), strings.TrimSpace(in.Repository.Repo)) {
		return nil, mcpcontract.ContributionPreflightOutput{}, mcpcontract.InvalidArgument("fork", "fork must differ from the upstream repository", nil)
	}
	if strings.TrimSpace(in.Candidate.Title) == "" && strings.TrimSpace(in.Candidate.Query) == "" && strings.TrimSpace(in.Candidate.Body) == "" && in.Candidate.IssueNumber < 1 && strings.TrimSpace(in.Candidate.HeadRef) == "" && strings.TrimSpace(in.Candidate.HeadSHA) == "" && len(in.Candidate.ChangedFiles) == 0 && len(in.WorkspacePaths) == 0 {
		return nil, mcpcontract.ContributionPreflightOutput{}, mcpcontract.InvalidArgument("candidate", "candidate or workspace_paths must provide title, query, body, issue_number, head_ref, head_sha, or changed_files", nil)
	}
	if in.Candidate.IssueNumber < 0 {
		return nil, mcpcontract.ContributionPreflightOutput{}, mcpcontract.InvalidArgument("candidate.issue_number", "issue_number must be positive when provided", map[string]any{"issue_number": 1})
	}
	if in.Limit < 0 || in.Limit > 100 {
		return nil, mcpcontract.ContributionPreflightOutput{}, mcpcontract.InvalidArgument("limit", "limit must be between 1 and 100 when provided", map[string]any{"limit": 20})
	}
	if in.MaxRequests < 0 || in.MaxRequests > 1000 || (in.MaxRequests > 0 && in.MaxRequests < 2) {
		return nil, mcpcontract.ContributionPreflightOutput{}, mcpcontract.InvalidArgument("max_requests", "max_requests must be between 2 and 1000 when provided", map[string]any{"max_requests": 100})
	}
	reader, ok := s.reader.(ContributionPreflightReader)
	if !ok {
		return nil, mcpcontract.ContributionPreflightOutput{}, errors.New("contribution preflight is not available")
	}
	out, err := reader.PreflightContribution(ctx, in)
	return nil, out, err
}
