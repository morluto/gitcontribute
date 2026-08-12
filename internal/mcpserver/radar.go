package mcpserver

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func (s *Server) rankContributionCandidates(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.RankContributionCandidatesInput) (*mcp.CallToolResult, mcpcontract.RankContributionCandidatesOutput, error) {
	ranker, ok := s.reader.(ContributionCandidateRanker)
	if !ok {
		return nil, mcpcontract.RankContributionCandidatesOutput{}, errors.New("contribution candidate ranking is not available")
	}
	out, err := ranker.RankContributionCandidates(ctx, in)
	return nil, out, err
}
