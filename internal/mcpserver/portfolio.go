package mcpserver

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func (s *Server) listPullRequestPortfolio(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.ListPullRequestPortfolioInput) (*mcp.CallToolResult, mcpcontract.ListPullRequestPortfolioOutput, error) {
	if len(in.PullRequests) == 0 && in.State == "" {
		in.State = "open"
	}
	if len(in.PullRequests) == 0 && in.Limit == 0 {
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
	for i := range in.Candidates {
		candidate := &in.Candidates[i]
		candidate.Kind = strings.TrimSpace(candidate.Kind)
		candidate.Ref = strings.TrimSpace(candidate.Ref)
		if candidate.Kind != "opportunity" && candidate.Kind != "workspace" && candidate.Kind != "pull_request" {
			return nil, mcpcontract.FindPortfolioOverlapsOutput{}, mcpcontract.InvalidArgument("candidates", "candidate kind must be opportunity, workspace, or pull_request", map[string]any{"candidates": []map[string]string{{"kind": "opportunity", "ref": "<id>"}}})
		}
		if candidate.Ref == "" {
			return nil, mcpcontract.FindPortfolioOverlapsOutput{}, mcpcontract.InvalidArgument("candidates", "candidate ref is required", nil)
		}
	}
	for i, pullRequest := range in.PullRequests {
		normalized, err := normalizeThreadRef(pullRequest, optionalThreadKind)
		if err != nil {
			return nil, mcpcontract.FindPortfolioOverlapsOutput{}, err
		}
		if normalized.Kind != "" && normalized.Kind != string(domain.PullRequestKind) {
			return nil, mcpcontract.FindPortfolioOverlapsOutput{}, mcpcontract.InvalidArgument("pull_requests", "kind must be pull_request when provided", map[string]any{"kind": "pull_request"})
		}
		in.PullRequests[i] = normalized
	}
	reader, ok := s.reader.(PortfolioReader)
	if !ok {
		return nil, mcpcontract.FindPortfolioOverlapsOutput{}, errors.New("portfolio reads are not available")
	}
	out, err := reader.FindPortfolioOverlaps(ctx, in)
	return nil, out, err
}

type threadKindRequirement uint8

const (
	optionalThreadKind threadKindRequirement = iota
	requiredThreadKind
)

func normalizeThreadRef(ref mcpcontract.ThreadRef, requirement threadKindRequirement) (mcpcontract.ThreadRef, error) {
	repository, err := domain.NewRepoRef(ref.Owner, ref.Repo)
	if err != nil {
		return mcpcontract.ThreadRef{}, mcpcontract.InvalidArgument("threads", "owner and repo are required", map[string]any{"owner": "acme", "repo": "rocket", "number": 1})
	}
	if ref.Number < 1 {
		return mcpcontract.ThreadRef{}, mcpcontract.InvalidArgument("threads", "number must be positive", map[string]any{"owner": repository.Owner(), "repo": repository.Repo(), "number": 1})
	}
	ref.Owner, ref.Repo, ref.Kind = repository.Owner(), repository.Repo(), strings.TrimSpace(ref.Kind)
	if ref.Kind == "" && requirement == optionalThreadKind {
		return ref, nil
	}
	kind, err := domain.ParseThreadKind(ref.Kind)
	if err != nil {
		return mcpcontract.ThreadRef{}, mcpcontract.InvalidArgument("threads", "kind must be issue or pull_request", map[string]any{"kind": "pull_request"})
	}
	ref.Kind = string(kind)
	return ref, nil
}

func (s *Server) linkPullRequest(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.LinkPullRequestInput) (*mcp.CallToolResult, mcpcontract.LinkPullRequestOutput, error) {
	operator, ok := s.reader.(PortfolioOperator)
	if !ok {
		return nil, mcpcontract.LinkPullRequestOutput{}, errors.New("portfolio linking is not available")
	}
	out, err := operator.LinkPullRequest(ctx, in)
	return nil, out, err
}

func (s *Server) compareFork(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.CompareForkInput) (*mcp.CallToolResult, mcpcontract.ForkFreshnessOutput, error) {
	upstreamOwner, upstreamRepo, err := normalizeRepository(in.Upstream.Owner, in.Upstream.Repo)
	if err != nil {
		return nil, mcpcontract.ForkFreshnessOutput{}, mcpcontract.InvalidArgument("upstream", "owner and repo are required", map[string]any{"owner": "acme", "repo": "rocket"})
	}
	forkOwner, forkRepo, err := normalizeRepository(in.Fork.Owner, in.Fork.Repo)
	if err != nil {
		return nil, mcpcontract.ForkFreshnessOutput{}, mcpcontract.InvalidArgument("fork", "owner and repo are required", map[string]any{"owner": "alice", "repo": "rocket"})
	}
	if strings.EqualFold(upstreamOwner, forkOwner) && strings.EqualFold(upstreamRepo, forkRepo) {
		return nil, mcpcontract.ForkFreshnessOutput{}, mcpcontract.InvalidArgument("fork", "fork must differ from the upstream repository", nil)
	}
	in.Upstream = mcpcontract.RepositoryRef{Owner: upstreamOwner, Repo: upstreamRepo}
	in.Fork = mcpcontract.RepositoryRef{Owner: forkOwner, Repo: forkRepo}
	reader, ok := s.reader.(ForkComparisonReader)
	if !ok {
		return nil, mcpcontract.ForkFreshnessOutput{}, errors.New("fork comparison is not available")
	}
	out, err := reader.CompareFork(ctx, in)
	return nil, out, err
}
