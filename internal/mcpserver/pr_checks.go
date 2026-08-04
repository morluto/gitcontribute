package mcpserver

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func (s *Server) waitPullRequestChecks(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.WaitPullRequestChecksInput) (*mcp.CallToolResult, mcpcontract.JobReference, error) {
	in.Owner = strings.TrimSpace(in.Owner)
	in.Repo = strings.TrimSpace(in.Repo)
	in.ExpectedHeadSHA = strings.TrimSpace(in.ExpectedHeadSHA)
	if in.Owner == "" || in.Repo == "" || in.Number < 1 || in.ExpectedHeadSHA == "" {
		return nil, mcpcontract.JobReference{}, mcpcontract.InvalidArgument("expected_head_sha", "owner, repo, number, and expected_head_sha are required", map[string]any{"owner": "octo", "repo": "project", "number": 1, "expected_head_sha": "abcdef1234567"})
	}
	operator, ok := s.reader.(PullRequestCheckWaiter)
	if !ok {
		return nil, mcpcontract.JobReference{}, errors.New("pull-request check waiting is unavailable")
	}
	out, err := operator.WaitPullRequestChecks(ctx, in)
	return nil, out, err
}
