package mcpserver

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// PrepareContributionInput renders a local issue or pull-request draft.

// DraftOutput contains a rendered contribution draft.

// ExportManifestInput selects bounded local evidence for one contribution manifest.

// ManifestPullRequestInput identifies one exact stored pull request.

// ManifestOutput returns the stable identity and full in-toto-shaped statement.

func (s *Server) prepareContribution(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.PrepareContributionInput) (*mcp.CallToolResult, mcpcontract.DraftOutput, error) {
	if _, err := normalizeID("opportunity_id", in.OpportunityID); err != nil {
		return nil, mcpcontract.DraftOutput{}, err
	}
	in.Kind = strings.ToLower(strings.TrimSpace(in.Kind))
	if in.Kind != "issue" && in.Kind != "pull_request" {
		return nil, mcpcontract.DraftOutput{}, mcpcontract.InvalidArgument("kind", "must be issue or pull_request", map[string]any{"kind": "pull_request"})
	}
	if in.Kind == "pull_request" && strings.TrimSpace(in.WorkspaceID) == "" {
		return nil, mcpcontract.DraftOutput{}, mcpcontract.InvalidArgument("workspace_id", "is required for pull_request drafts", map[string]any{"workspace_id": "<id>"})
	}
	if in.Kind == "pull_request" && strings.TrimSpace(in.Approach) == "" {
		return nil, mcpcontract.DraftOutput{}, mcpcontract.InvalidArgument("approach", "is required for pull_request drafts", map[string]any{"approach": "Describe the implementation approach."})
	}
	if in.Kind == "issue" && (in.WorkspaceID != "" || in.Approach != "" || in.Changes != "" || in.Compatibility != "" || in.Limitations != "" || in.LinkedIssue != "") {
		return nil, mcpcontract.DraftOutput{}, mcpcontract.InvalidArgument("kind", "pull-request-only fields are not accepted for issue drafts", map[string]any{"kind": "pull_request"})
	}
	if in.Kind == "pull_request" && in.Success != "" {
		return nil, mcpcontract.DraftOutput{}, mcpcontract.InvalidArgument("success", "is only accepted for issue drafts", map[string]any{"kind": "issue", "success": "Describe the desired outcome."})
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, mcpcontract.DraftOutput{}, errors.New("contribution preparation is not available")
	}
	out, err := operator.PrepareContribution(ctx, in)
	return nil, out, err
}

func (s *Server) exportManifest(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.ExportManifestInput) (*mcp.CallToolResult, mcpcontract.ManifestOutput, error) {
	if _, err := normalizeID("opportunity_id", in.OpportunityID); err != nil {
		return nil, mcpcontract.ManifestOutput{}, err
	}
	if in.PullRequest != nil && (strings.TrimSpace(in.PullRequest.Owner) == "" || strings.TrimSpace(in.PullRequest.Repo) == "" || in.PullRequest.Number <= 0) {
		return nil, mcpcontract.ManifestOutput{}, mcpcontract.InvalidArgument("pull_request", "owner, repo, and a positive number are required", map[string]any{"owner": "acme", "repo": "rocket", "number": 42})
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, mcpcontract.ManifestOutput{}, errors.New("manifest export is not available")
	}
	out, err := operator.ExportManifest(ctx, in)
	return nil, out, err
}
