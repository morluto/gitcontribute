package mcpserver

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// CommitPlannerReader performs local, read-only Git inspection and planning.
type CommitPlannerReader interface {
	InspectCommitChanges(context.Context, mcpcontract.InspectCommitChangesInput) (mcpcontract.CommitInventoryOutput, error)
	PlanSemanticCommits(context.Context, mcpcontract.PlanSemanticCommitsInput) (mcpcontract.SemanticCommitPlanOutput, error)
}

// InspectCommitChangesInput selects one managed workspace.

// CommitUnitOutput is one indivisible file or hunk assignment unit.

// CommitPlanWarningOutput flags changes needing explicit judgment.

// CommitInventoryOutput freezes assignable units and exact source digests.

// SemanticCommitGroupInput supplies judgment that cannot be inferred safely.

// UnresolvedCommitUnitInput preserves ambiguity instead of inventing ownership.

// PlanSemanticCommitsInput binds agent-authored groups to a frozen inventory.

// SemanticCommitGroupOutput is one validated proposed commit.

// UnresolvedCommitUnitOutput reports an unassigned unit and reason.

// CommitReconstructionOutput proves exact one-to-one unit coverage.

// SemanticCommitPlanOutput is a read-only plan; it contains no patch apply.

func (s *Server) registerCommitPlanning() {
	readOnly := readOnlyAnnotations()
	addCatalogTool(s, catalogTool[mcpcontract.InspectCommitChangesInput, mcpcontract.CommitInventoryOutput]{
		name: mcpcontract.ToolInspectCommitChanges, title: "Inspect semantic commit units",
		description: "Parse a managed workspace's local Git diff into stable file and hunk IDs, including rename, binary, generated, and untracked warnings. Call this before planning; it never stages files or changes history.",
		annotations: readOnly, supportedBy: supports[CommitPlannerReader], input: inputSchema[mcpcontract.InspectCommitChangesInput](noSchemaCustomization),
		output: outputSchema[mcpcontract.CommitInventoryOutput]("Frozen hunk inventory and exact reconstruction digests."), handler: s.inspectCommitChanges,
	})
	addCatalogTool(s, catalogTool[mcpcontract.PlanSemanticCommitsInput, mcpcontract.SemanticCommitPlanOutput]{
		name: mcpcontract.ToolPlanSemanticCommits, title: "Plan semantic commits",
		description: "Validate agent-authored semantic groups against the exact inventory from " + mcpcontract.ToolInspectCommitChanges + ". Unassigned ambiguity stays explicit; duplicate assignment fails. This is read-only and never stages, commits, or rewrites history.",
		annotations: readOnly, supportedBy: supports[CommitPlannerReader], input: inputSchema[mcpcontract.PlanSemanticCommitsInput](func(sc *schemaBuilder) {
			setArrayBounds(sc, "groups", 0, 100)
			setArrayBounds(sc, "unresolved", 0, 2000)
		}), output: outputSchema[mcpcontract.SemanticCommitPlanOutput]("Semantic groups, warnings, unresolved units, and exact one-to-one coverage proof."), handler: s.planSemanticCommits,
	})
}

func (s *Server) inspectCommitChanges(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.InspectCommitChangesInput) (*mcp.CallToolResult, mcpcontract.CommitInventoryOutput, error) {
	if strings.TrimSpace(in.WorkspaceID) == "" {
		return nil, mcpcontract.CommitInventoryOutput{}, mcpcontract.InvalidArgument("workspace_id", "is required", nil)
	}
	reader, ok := s.reader.(CommitPlannerReader)
	if !ok {
		return nil, mcpcontract.CommitInventoryOutput{}, errors.New("semantic commit planning is not available")
	}
	out, err := reader.InspectCommitChanges(ctx, in)
	return nil, out, err
}

func (s *Server) planSemanticCommits(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.PlanSemanticCommitsInput) (*mcp.CallToolResult, mcpcontract.SemanticCommitPlanOutput, error) {
	if strings.TrimSpace(in.WorkspaceID) == "" || strings.TrimSpace(in.ExpectedInventorySHA256) == "" {
		return nil, mcpcontract.SemanticCommitPlanOutput{}, mcpcontract.InvalidArgument("expected_inventory_sha256", "workspace_id and expected_inventory_sha256 are required", nil)
	}
	reader, ok := s.reader.(CommitPlannerReader)
	if !ok {
		return nil, mcpcontract.SemanticCommitPlanOutput{}, errors.New("semantic commit planning is not available")
	}
	out, err := reader.PlanSemanticCommits(ctx, in)
	return nil, out, err
}
