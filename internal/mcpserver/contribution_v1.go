package mcpserver

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// PrepareContributionInput renders a local issue or pull-request draft.
type PrepareContributionInput struct {
	OpportunityID string `json:"opportunity_id" jsonschema:"Opportunity ID"`
	Kind          string `json:"kind" jsonschema:"Contribution kind: issue or pull_request"`
	WorkspaceID   string `json:"workspace_id,omitempty" jsonschema:"Workspace ID for pull_request drafts"`
	Approach      string `json:"approach,omitempty" jsonschema:"Approach summary for pull requests"`
	Changes       string `json:"changes,omitempty" jsonschema:"Changes summary for pull requests"`
	Compatibility string `json:"compatibility,omitempty" jsonschema:"Compatibility notes for pull requests"`
	Limitations   string `json:"limitations,omitempty" jsonschema:"Limitations for pull requests"`
	LinkedIssue   string `json:"linked_issue,omitempty" jsonschema:"Linked issue for pull requests"`
	Guidance      string `json:"guidance,omitempty" jsonschema:"Optional guidance to include"`
	Success       string `json:"success,omitempty" jsonschema:"Success criteria for issue drafts"`
	ManifestID    string `json:"manifest_id,omitempty" jsonschema:"Stored evidence manifest ID to reference without copying its claims"`
}

// DraftOutput contains a rendered contribution draft.
type DraftOutput struct {
	OpportunityID string `json:"opportunity_id"`
	Kind          string `json:"kind"`
	Title         string `json:"title"`
	Body          string `json:"body"`
	RenderedAt    string `json:"rendered_at"`
	ManifestID    string `json:"manifest_id,omitempty" jsonschema:"Referenced stored evidence manifest ID"`
}

// ExportManifestInput selects bounded local evidence for one contribution manifest.
type ExportManifestInput struct {
	OpportunityID string                    `json:"opportunity_id" jsonschema:"Opportunity ID"`
	WorkspaceID   string                    `json:"workspace_id,omitempty" jsonschema:"Managed workspace ID to bind"`
	PullRequest   *ManifestPullRequestInput `json:"pull_request,omitempty" jsonschema:"Exact stored pull request to include"`
}

// ManifestPullRequestInput identifies one exact stored pull request.
type ManifestPullRequestInput struct {
	Owner  string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo   string `json:"repo" jsonschema:"GitHub repository name"`
	Number int    `json:"number" jsonschema:"Positive pull request number"`
}

// ManifestOutput returns the stable identity and full in-toto-shaped statement.
type ManifestOutput struct {
	ManifestID    string         `json:"manifest_id" jsonschema:"Stable sha256-prefixed manifest ID"`
	ContentSHA256 string         `json:"content_sha256" jsonschema:"Hex SHA-256 of stable manifest content"`
	SchemaVersion string         `json:"schema_version" jsonschema:"Contribution manifest predicate schema version"`
	Status        string         `json:"status" jsonschema:"Overall completeness status"`
	Statement     map[string]any `json:"statement" jsonschema:"Full in-toto-shaped evidence statement"`
}

func (s *Server) prepareContribution(ctx context.Context, _ *mcp.CallToolRequest, in PrepareContributionInput) (*mcp.CallToolResult, DraftOutput, error) {
	if _, err := normalizeID("opportunity_id", in.OpportunityID); err != nil {
		return nil, DraftOutput{}, err
	}
	in.Kind = strings.ToLower(strings.TrimSpace(in.Kind))
	if in.Kind != "issue" && in.Kind != "pull_request" {
		return nil, DraftOutput{}, errors.New("kind must be issue or pull_request")
	}
	if in.Kind == "pull_request" && strings.TrimSpace(in.WorkspaceID) == "" {
		return nil, DraftOutput{}, errors.New("workspace_id is required for pull_request drafts")
	}
	if in.Kind == "pull_request" && strings.TrimSpace(in.Approach) == "" {
		return nil, DraftOutput{}, errors.New("approach is required for pull_request drafts")
	}
	if in.Kind == "issue" && (in.WorkspaceID != "" || in.Approach != "" || in.Changes != "" || in.Compatibility != "" || in.Limitations != "" || in.LinkedIssue != "") {
		return nil, DraftOutput{}, errors.New("pull-request-only fields are not accepted for issue drafts")
	}
	if in.Kind == "pull_request" && in.Success != "" {
		return nil, DraftOutput{}, errors.New("success is only accepted for issue drafts")
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, DraftOutput{}, errors.New("contribution preparation is not available")
	}
	out, err := operator.PrepareContribution(ctx, in)
	return nil, out, err
}

func (s *Server) exportManifest(ctx context.Context, _ *mcp.CallToolRequest, in ExportManifestInput) (*mcp.CallToolResult, ManifestOutput, error) {
	if _, err := normalizeID("opportunity_id", in.OpportunityID); err != nil {
		return nil, ManifestOutput{}, err
	}
	if in.PullRequest != nil && (strings.TrimSpace(in.PullRequest.Owner) == "" || strings.TrimSpace(in.PullRequest.Repo) == "" || in.PullRequest.Number <= 0) {
		return nil, ManifestOutput{}, InvalidArgument("pull_request", "owner, repo, and a positive number are required", map[string]any{"owner": "acme", "repo": "rocket", "number": 42})
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, ManifestOutput{}, errors.New("manifest export is not available")
	}
	out, err := operator.ExportManifest(ctx, in)
	return nil, out, err
}
