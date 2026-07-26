package mcpserver

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// CreateWorkspaceInput configures a durable managed-workspace creation job.

// AdoptWorkspaceInput identifies an existing local worktree and an already
// available base revision. Adoption never fetches or changes the worktree.

// AdoptWorkspaceOutput deliberately omits host paths and remote URLs.

func (s *Server) adoptWorkspace(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.AdoptWorkspaceInput) (*mcp.CallToolResult, mcpcontract.AdoptWorkspaceOutput, error) {
	var err error
	if in.InvestigationID, err = normalizeID("investigation_id", in.InvestigationID); err != nil {
		return nil, mcpcontract.AdoptWorkspaceOutput{}, err
	}
	in.Path, in.BaseRef, in.Name = strings.TrimSpace(in.Path), strings.TrimSpace(in.BaseRef), strings.TrimSpace(in.Name)
	if in.Path == "" || in.BaseRef == "" {
		return nil, mcpcontract.AdoptWorkspaceOutput{}, mcpcontract.InvalidArgument("path", "path and base_ref are required", map[string]any{"path": "/absolute/worktree", "base_ref": "origin/main"})
	}
	operator, ok := s.reader.(WorkspaceAdopter)
	if !ok {
		return nil, mcpcontract.AdoptWorkspaceOutput{}, errors.New("workspace adoption is not available")
	}
	out, err := operator.AdoptWorkspace(ctx, in)
	return nil, out, err
}

func (s *Server) createWorkspace(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.CreateWorkspaceInput) (*mcp.CallToolResult, mcpcontract.JobReference, error) {
	if _, err := normalizeID("investigation_id", in.InvestigationID); err != nil {
		return nil, mcpcontract.JobReference{}, err
	}
	in.Remote = strings.TrimSpace(in.Remote)
	in.BaseRef = strings.TrimSpace(in.BaseRef)
	in.CandidateRef = strings.TrimSpace(in.CandidateRef)
	in.Name = strings.TrimSpace(in.Name)
	operator, ok := s.reader.(WorkspaceCreator)
	if !ok {
		return nil, mcpcontract.JobReference{}, errors.New("workspace creation is not available")
	}
	out, err := operator.CreateWorkspace(ctx, in)
	return nil, out, err
}
