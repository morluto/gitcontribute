package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// PrepareContributionInput renders a local issue or pull-request draft.

// DraftOutput contains a rendered contribution draft.

// ExportManifestInput selects bounded local evidence for one contribution manifest.

// ManifestPullRequestInput identifies one exact stored pull request.

// ManifestOutput returns the stable identity and full in-toto-shaped statement.

func (s *Server) prepareContribution(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.PrepareContributionInput) (*mcp.CallToolResult, mcpcontract.DurableArtifactReference, error) {
	opportunityID, err := normalizeID("opportunity_id", in.OpportunityID)
	if err != nil {
		return nil, mcpcontract.DurableArtifactReference{}, err
	}
	in.OpportunityID = opportunityID
	if in.Kind == "pull_request" {
		workspaceID, err := normalizeID("workspace_id", in.WorkspaceID)
		if err != nil {
			return nil, mcpcontract.DurableArtifactReference{}, err
		}
		in.WorkspaceID = workspaceID
		in.Approach = strings.TrimSpace(in.Approach)
		if in.Approach == "" {
			return nil, mcpcontract.DurableArtifactReference{}, mcpcontract.InvalidArgument("approach", "is required for pull_request drafts", map[string]any{"approach": "Describe the implementation approach."})
		}
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, mcpcontract.DurableArtifactReference{}, errors.New("contribution preparation is not available")
	}
	out, err := operator.PrepareContribution(ctx, in)
	if err != nil {
		return nil, mcpcontract.DurableArtifactReference{}, err
	}
	uri := fmt.Sprintf("gitcontribute://draft/%s/%d", out.ID, out.Revision)
	ref := mcpcontract.DurableArtifactReference{Kind: "draft", ID: out.ID, URI: uri}
	return linkedResource(uri, "draft", "Contribution draft", "Immutable persisted contribution-draft revision."), ref, nil
}

func (s *Server) exportManifest(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.ExportManifestInput) (*mcp.CallToolResult, mcpcontract.DurableArtifactReference, error) {
	opportunityID, err := normalizeID("opportunity_id", in.OpportunityID)
	if err != nil {
		return nil, mcpcontract.DurableArtifactReference{}, err
	}
	in.OpportunityID = opportunityID
	if in.PullRequest != nil {
		owner, repo, err := normalizeRepository(in.PullRequest.Owner, in.PullRequest.Repo)
		if err != nil || in.PullRequest.Number <= 0 {
			return nil, mcpcontract.DurableArtifactReference{}, mcpcontract.InvalidArgument("pull_request", "owner, repo, and a positive number are required", map[string]any{"owner": "acme", "repo": "rocket", "number": 42})
		}
		in.PullRequest = &mcpcontract.ManifestPullRequestInput{Owner: owner, Repo: repo, Number: in.PullRequest.Number}
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, mcpcontract.DurableArtifactReference{}, errors.New("manifest export is not available")
	}
	out, err := operator.ExportManifest(ctx, in)
	if err != nil {
		return nil, mcpcontract.DurableArtifactReference{}, err
	}
	uri := "gitcontribute://manifest/" + url.QueryEscape(out.ManifestID)
	ref := mcpcontract.DurableArtifactReference{Kind: "manifest", ID: out.ManifestID, URI: uri}
	return linkedResource(uri, "manifest", "Contribution manifest", "Persisted contribution evidence manifest."), ref, nil
}
