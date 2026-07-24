package app

import (
	"context"

	"github.com/morluto/gitcontribute/internal/commitplan"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// InspectCommitChanges implements the MCP commit-inventory capability.
func (r *MCPReader) InspectCommitChanges(ctx context.Context, in mcpcontract.InspectCommitChangesInput) (mcpcontract.CommitInventoryOutput, error) {
	inventory, err := r.Service.InspectCommitChanges(ctx, in.WorkspaceID)
	if err != nil {
		return mcpcontract.CommitInventoryOutput{}, err
	}
	return commitInventoryToMCP(inventory), nil
}

// PlanSemanticCommits implements the MCP read-only semantic planning capability.
func (r *MCPReader) PlanSemanticCommits(ctx context.Context, in mcpcontract.PlanSemanticCommitsInput) (mcpcontract.SemanticCommitPlanOutput, error) {
	groups := make([]commitplan.GroupInput, len(in.Groups))
	for index, group := range in.Groups {
		groups[index] = commitplan.GroupInput{
			Name: group.Name, Intent: group.Intent, Type: group.Type, Scope: group.Scope,
			UnitIDs: group.UnitIDs, DependsOn: group.DependsOn,
			ValidationCommands: group.ValidationCommands, TestOwners: group.TestOwners,
		}
	}
	unresolved := make([]commitplan.UnresolvedInput, len(in.Unresolved))
	for index, item := range in.Unresolved {
		unresolved[index] = commitplan.UnresolvedInput{UnitID: item.UnitID, Reason: item.Reason}
	}
	plan, err := r.Service.PlanSemanticCommits(ctx, in.WorkspaceID, in.ExpectedInventorySHA256, commitplan.PlanInput{Groups: groups, Unresolved: unresolved})
	if err != nil {
		return mcpcontract.SemanticCommitPlanOutput{}, err
	}
	return semanticCommitPlanToMCP(plan), nil
}

func commitInventoryToMCP(inventory commitplan.Inventory) mcpcontract.CommitInventoryOutput {
	out := mcpcontract.CommitInventoryOutput{SourcePatchSHA256: inventory.SourcePatchSHA256, InventorySHA256: inventory.InventorySHA256}
	for _, unit := range inventory.Units {
		out.Units = append(out.Units, mcpcontract.CommitUnitOutput{
			ID: unit.ID, Kind: unit.Kind, Path: unit.Path, OldPath: unit.OldPath, Operation: unit.Operation,
			OldStart: unit.OldStart, OldLines: unit.OldLines, NewStart: unit.NewStart, NewLines: unit.NewLines,
			Patch: unit.Patch, ContentSHA256: unit.ContentHash, Generated: unit.Generated, WhitespaceOnly: unit.WhitespaceOnly,
		})
	}
	out.Warnings = commitWarningsToMCP(inventory.Warnings)
	return out
}

func semanticCommitPlanToMCP(plan commitplan.Plan) mcpcontract.SemanticCommitPlanOutput {
	out := mcpcontract.SemanticCommitPlanOutput{
		Warnings: commitWarningsToMCP(plan.Warnings),
		Reconstruction: mcpcontract.CommitReconstructionOutput{
			SourcePatchSHA256: plan.Reconstruction.SourcePatchSHA256, InventorySHA256: plan.Reconstruction.InventorySHA256,
			AssignedSHA256: plan.Reconstruction.AssignedSHA256, UnitCount: plan.Reconstruction.UnitCount,
			AssignedCount: plan.Reconstruction.AssignedCount, Verified: plan.Reconstruction.Verified,
		},
	}
	for _, group := range plan.Groups {
		out.Groups = append(out.Groups, mcpcontract.SemanticCommitGroupOutput{
			Name: group.Name, Intent: group.Intent, SuggestedSubject: group.SuggestedSubject,
			UnitIDs: group.UnitIDs, Files: group.Files, DependsOn: group.DependsOn,
			ValidationCommands: group.ValidationCommands, TestOwners: group.TestOwners,
		})
	}
	for _, item := range plan.Unresolved {
		out.Unresolved = append(out.Unresolved, mcpcontract.UnresolvedCommitUnitOutput{UnitID: item.UnitID, Reason: item.Reason})
	}
	return out
}

func commitWarningsToMCP(warnings []commitplan.Warning) []mcpcontract.CommitPlanWarningOutput {
	out := make([]mcpcontract.CommitPlanWarningOutput, len(warnings))
	for index, warning := range warnings {
		out[index] = mcpcontract.CommitPlanWarningOutput{Code: warning.Code, Message: warning.Message, Path: warning.Path, UnitID: warning.UnitID}
	}
	return out
}
