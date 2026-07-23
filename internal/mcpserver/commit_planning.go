package mcpserver

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// CommitPlannerReader performs local, read-only Git inspection and planning.
type CommitPlannerReader interface {
	InspectCommitChanges(context.Context, InspectCommitChangesInput) (CommitInventoryOutput, error)
	PlanSemanticCommits(context.Context, PlanSemanticCommitsInput) (SemanticCommitPlanOutput, error)
}

// InspectCommitChangesInput selects one managed workspace.
type InspectCommitChangesInput struct {
	WorkspaceID string `json:"workspace_id" jsonschema:"Managed workspace ID"`
}

// CommitUnitOutput is one indivisible file or hunk assignment unit.
type CommitUnitOutput struct {
	ID             string `json:"id" jsonschema:"Stable content-derived unit ID"`
	Kind           string `json:"kind" jsonschema:"Unit kind: hunk, file, or untracked"`
	Path           string `json:"path" jsonschema:"Repository-relative path"`
	OldPath        string `json:"old_path,omitempty" jsonschema:"Previous path for rename or copy changes"`
	Operation      string `json:"operation" jsonschema:"Git change operation"`
	OldStart       int32  `json:"old_start,omitempty" jsonschema:"Original hunk start line"`
	OldLines       int32  `json:"old_lines,omitempty" jsonschema:"Original hunk line count"`
	NewStart       int32  `json:"new_start,omitempty" jsonschema:"New hunk start line"`
	NewLines       int32  `json:"new_lines,omitempty" jsonschema:"New hunk line count"`
	Patch          string `json:"patch,omitempty" jsonschema:"Canonical hunk preview for semantic classification"`
	ContentSHA256  string `json:"content_sha256" jsonschema:"Exact unit identity digest"`
	Generated      bool   `json:"generated" jsonschema:"Path appears generated or snapshot-owned"`
	WhitespaceOnly bool   `json:"whitespace_only" jsonschema:"Hunk changes only whitespace"`
}

// CommitPlanWarningOutput flags changes needing explicit judgment.
type CommitPlanWarningOutput struct {
	Code    string `json:"code" jsonschema:"Stable warning code"`
	Message string `json:"message" jsonschema:"Actionable warning explanation"`
	Path    string `json:"path,omitempty" jsonschema:"Affected repository-relative path"`
	UnitID  string `json:"unit_id,omitempty" jsonschema:"Affected assignable unit ID"`
}

// CommitInventoryOutput freezes assignable units and exact source digests.
type CommitInventoryOutput struct {
	Units             []CommitUnitOutput        `json:"units" jsonschema:"Ordered assignable changes"`
	Warnings          []CommitPlanWarningOutput `json:"warnings,omitempty" jsonschema:"Conditions requiring explicit judgment"`
	SourcePatchSHA256 string                    `json:"source_patch_sha256" jsonschema:"SHA-256 of the exact Git patch bytes"`
	InventorySHA256   string                    `json:"inventory_sha256" jsonschema:"SHA-256 binding ordered tracked and untracked unit identities"`
}

// SemanticCommitGroupInput supplies judgment that cannot be inferred safely.
type SemanticCommitGroupInput struct {
	Name               string   `json:"name" jsonschema:"Unique group name used by dependency references"`
	Intent             string   `json:"intent" jsonschema:"Concrete outcome of this commit"`
	Type               string   `json:"type" jsonschema:"Conventional commit type"`
	Scope              string   `json:"scope,omitempty" jsonschema:"Optional conventional commit scope"`
	UnitIDs            []string `json:"unit_ids" jsonschema:"One or more IDs from workspace.inspect_commit_changes"`
	DependsOn          []string `json:"depends_on,omitempty" jsonschema:"Group names that must precede this group"`
	ValidationCommands []string `json:"validation_commands,omitempty" jsonschema:"Focused validation commands for this group"`
	TestOwners         []string `json:"test_owners,omitempty" jsonschema:"Tests or owners responsible for this group"`
}

// UnresolvedCommitUnitInput preserves ambiguity instead of inventing ownership.
type UnresolvedCommitUnitInput struct {
	UnitID string `json:"unit_id" jsonschema:"Unassigned file or hunk ID"`
	Reason string `json:"reason" jsonschema:"Why ownership remains ambiguous"`
}

// PlanSemanticCommitsInput binds agent-authored groups to a frozen inventory.
type PlanSemanticCommitsInput struct {
	WorkspaceID             string                      `json:"workspace_id" jsonschema:"Managed workspace ID"`
	ExpectedInventorySHA256 string                      `json:"expected_inventory_sha256" jsonschema:"Inventory digest returned by the preceding inspection"`
	Groups                  []SemanticCommitGroupInput  `json:"groups" jsonschema:"One to 100 proposed semantic commit groups"`
	Unresolved              []UnresolvedCommitUnitInput `json:"unresolved,omitempty" jsonschema:"Ambiguous units with explicit reasons"`
}

// SemanticCommitGroupOutput is one validated proposed commit.
type SemanticCommitGroupOutput struct {
	Name               string   `json:"name" jsonschema:"Unique group name"`
	Intent             string   `json:"intent" jsonschema:"Concrete commit outcome"`
	SuggestedSubject   string   `json:"suggested_subject" jsonschema:"Conventional commit subject derived from type, scope, and intent"`
	UnitIDs            []string `json:"unit_ids" jsonschema:"Assigned file and hunk IDs"`
	Files              []string `json:"files" jsonschema:"Sorted repository-relative paths in the group"`
	DependsOn          []string `json:"depends_on,omitempty" jsonschema:"Groups that must precede this group"`
	ValidationCommands []string `json:"validation_commands,omitempty" jsonschema:"Focused validation commands"`
	TestOwners         []string `json:"test_owners,omitempty" jsonschema:"Tests or owners responsible for validation"`
}

// UnresolvedCommitUnitOutput reports an unassigned unit and reason.
type UnresolvedCommitUnitOutput struct {
	UnitID string `json:"unit_id" jsonschema:"Unassigned file or hunk ID"`
	Reason string `json:"reason" jsonschema:"Why ownership remains ambiguous"`
}

// CommitReconstructionOutput proves exact one-to-one unit coverage.
type CommitReconstructionOutput struct {
	SourcePatchSHA256 string `json:"source_patch_sha256" jsonschema:"SHA-256 of the exact Git patch bytes"`
	InventorySHA256   string `json:"inventory_sha256" jsonschema:"Digest of every ordered source unit"`
	AssignedSHA256    string `json:"assigned_sha256" jsonschema:"Digest of every uniquely assigned unit in source order"`
	UnitCount         int    `json:"unit_count" jsonschema:"Total source units"`
	AssignedCount     int    `json:"assigned_count" jsonschema:"Uniquely assigned units"`
	Verified          bool   `json:"verified" jsonschema:"True only when every source unit is assigned exactly once"`
}

// SemanticCommitPlanOutput is a read-only plan; it contains no patch apply.
type SemanticCommitPlanOutput struct {
	Groups         []SemanticCommitGroupOutput  `json:"groups" jsonschema:"Validated semantic commit groups"`
	Unresolved     []UnresolvedCommitUnitOutput `json:"unresolved,omitempty" jsonschema:"Units still requiring ownership judgment"`
	Warnings       []CommitPlanWarningOutput    `json:"warnings,omitempty" jsonschema:"Mixed, generated, binary, formatting, and subject warnings"`
	Reconstruction CommitReconstructionOutput   `json:"reconstruction" jsonschema:"Exact one-to-one source coverage proof"`
}

func (s *Server) registerCommitPlanning() {
	readOnly := readOnlyAnnotations()
	addCatalogTool(s, catalogTool[InspectCommitChangesInput, CommitInventoryOutput]{
		name: ToolInspectCommitChanges, title: "Inspect semantic commit units",
		description: "Parse a managed workspace's local Git diff into stable file and hunk IDs, including rename, binary, generated, and untracked warnings. Call this before planning; it never stages files or changes history.",
		annotations: readOnly, supportedBy: supports[CommitPlannerReader], input: inputSchema[InspectCommitChangesInput](noSchemaCustomization),
		output: outputSchema[CommitInventoryOutput]("Frozen hunk inventory and exact reconstruction digests."), handler: s.inspectCommitChanges,
	})
	addCatalogTool(s, catalogTool[PlanSemanticCommitsInput, SemanticCommitPlanOutput]{
		name: ToolPlanSemanticCommits, title: "Plan semantic commits",
		description: "Validate agent-authored semantic groups against the exact inventory from " + ToolInspectCommitChanges + ". Unassigned ambiguity stays explicit; duplicate assignment fails. This is read-only and never stages, commits, or rewrites history.",
		annotations: readOnly, supportedBy: supports[CommitPlannerReader], input: inputSchema[PlanSemanticCommitsInput](func(sc *schemaBuilder) {
			setArrayBounds(sc, "groups", 0, 100)
			setArrayBounds(sc, "unresolved", 0, 2000)
		}), output: outputSchema[SemanticCommitPlanOutput]("Semantic groups, warnings, unresolved units, and exact one-to-one coverage proof."), handler: s.planSemanticCommits,
	})
}

func (s *Server) inspectCommitChanges(ctx context.Context, _ *mcp.CallToolRequest, in InspectCommitChangesInput) (*mcp.CallToolResult, CommitInventoryOutput, error) {
	if strings.TrimSpace(in.WorkspaceID) == "" {
		return nil, CommitInventoryOutput{}, InvalidArgument("workspace_id", "is required", nil)
	}
	reader, ok := s.reader.(CommitPlannerReader)
	if !ok {
		return nil, CommitInventoryOutput{}, errors.New("semantic commit planning is not available")
	}
	out, err := reader.InspectCommitChanges(ctx, in)
	return nil, out, err
}

func (s *Server) planSemanticCommits(ctx context.Context, _ *mcp.CallToolRequest, in PlanSemanticCommitsInput) (*mcp.CallToolResult, SemanticCommitPlanOutput, error) {
	if strings.TrimSpace(in.WorkspaceID) == "" || strings.TrimSpace(in.ExpectedInventorySHA256) == "" {
		return nil, SemanticCommitPlanOutput{}, InvalidArgument("expected_inventory_sha256", "workspace_id and expected_inventory_sha256 are required", nil)
	}
	reader, ok := s.reader.(CommitPlannerReader)
	if !ok {
		return nil, SemanticCommitPlanOutput{}, errors.New("semantic commit planning is not available")
	}
	out, err := reader.PlanSemanticCommits(ctx, in)
	return nil, out, err
}
