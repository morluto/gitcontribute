package mcpserver

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// CreateWorkspaceInput configures a durable managed-workspace creation job.
type CreateWorkspaceInput struct {
	InvestigationID string `json:"investigation_id" jsonschema:"Investigation ID"`
	Remote          string `json:"remote,omitempty" jsonschema:"Git remote URL to clone; defaults to the investigation repository"`
	BaseRef         string `json:"base_ref,omitempty" jsonschema:"Base ref to resolve; defaults to the remote HEAD"`
	CandidateRef    string `json:"candidate_ref,omitempty" jsonschema:"Candidate ref to resolve; defaults to the investigation commit"`
	Name            string `json:"name,omitempty" jsonschema:"Workspace name; defaults to a generated ID"`
}

// AdoptWorkspaceInput identifies an existing local worktree and an already
// available base revision. Adoption never fetches or changes the worktree.
type AdoptWorkspaceInput struct {
	InvestigationID string `json:"investigation_id" jsonschema:"Investigation ID"`
	Path            string `json:"path" jsonschema:"Existing local worktree root"`
	BaseRef         string `json:"base_ref" jsonschema:"Base ref already available in the repository"`
	Name            string `json:"name,omitempty" jsonschema:"Workspace name; defaults to a generated ID"`
}

// AdoptWorkspaceOutput deliberately omits host paths and remote URLs.
type AdoptWorkspaceOutput struct {
	ID              string `json:"id" jsonschema:"Workspace ID"`
	InvestigationID string `json:"investigation_id" jsonschema:"Investigation ID"`
	Owner           string `json:"owner" jsonschema:"Repository owner"`
	Repo            string `json:"repo" jsonschema:"Repository name"`
	BaseSHA         string `json:"base_sha" jsonschema:"Resolved base commit"`
	CandidateSHA    string `json:"candidate_sha" jsonschema:"Worktree HEAD observed during adoption"`
	MergeBase       string `json:"merge_base" jsonschema:"Merge base of base and candidate commits"`
	Dirty           bool   `json:"dirty" jsonschema:"Whether tracked or untracked changes were observed"`
	HasUntracked    bool   `json:"has_untracked" jsonschema:"Whether untracked non-ignored files were observed"`
	Ownership       string `json:"ownership" jsonschema:"Workspace ownership classification"`
}

func (s *Server) adoptWorkspace(ctx context.Context, _ *mcp.CallToolRequest, in AdoptWorkspaceInput) (*mcp.CallToolResult, AdoptWorkspaceOutput, error) {
	var err error
	if in.InvestigationID, err = normalizeID("investigation_id", in.InvestigationID); err != nil {
		return nil, AdoptWorkspaceOutput{}, err
	}
	in.Path, in.BaseRef, in.Name = strings.TrimSpace(in.Path), strings.TrimSpace(in.BaseRef), strings.TrimSpace(in.Name)
	if in.Path == "" || in.BaseRef == "" {
		return nil, AdoptWorkspaceOutput{}, errors.New("path and base_ref are required")
	}
	operator, ok := s.reader.(WorkspaceAdopter)
	if !ok {
		return nil, AdoptWorkspaceOutput{}, errors.New("workspace adoption is not available")
	}
	out, err := operator.AdoptWorkspace(ctx, in)
	return nil, out, err
}

func (s *Server) createWorkspace(ctx context.Context, _ *mcp.CallToolRequest, in CreateWorkspaceInput) (*mcp.CallToolResult, JobReference, error) {
	if _, err := normalizeID("investigation_id", in.InvestigationID); err != nil {
		return nil, JobReference{}, err
	}
	in.Remote = strings.TrimSpace(in.Remote)
	in.BaseRef = strings.TrimSpace(in.BaseRef)
	in.CandidateRef = strings.TrimSpace(in.CandidateRef)
	in.Name = strings.TrimSpace(in.Name)
	operator, ok := s.reader.(WorkspaceCreator)
	if !ok {
		return nil, JobReference{}, errors.New("workspace creation is not available")
	}
	out, err := operator.CreateWorkspace(ctx, in)
	return nil, out, err
}
