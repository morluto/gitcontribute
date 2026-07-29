package mcpserver

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

const (
	// ToolListConcerns searches the offline local concern ledger.
	ToolListConcerns = "corpus.list_concerns"
	// ToolCreateConcern records one local concern.
	ToolCreateConcern = "workflow.create_concern"
	// ToolUpdateConcern updates concern content.
	ToolUpdateConcern = "workflow.update_concern"
	// ToolSetConcernState transitions concern status.
	ToolSetConcernState = "workflow.set_concern_status"
	// ToolLinkConcern stores an explicit concern relationship.
	ToolLinkConcern = "workflow.link_concern"
	// ToolPromoteConcern creates downstream workflow atomically.
	ToolPromoteConcern = "workflow.promote_concern"
)

// ConcernReader exposes bounded offline concern reads.
type ConcernReader interface {
	ListConcerns(context.Context, mcpcontract.ListConcernsInput) (mcpcontract.ConcernListOutput, error)
}

// ConcernOperator exposes local concern-ledger writes.
type ConcernOperator interface {
	Concern(context.Context, mcpcontract.ConcernInput) (mcpcontract.ConcernOutput, error)
	CreateConcern(context.Context, mcpcontract.CreateConcernInput) (mcpcontract.ConcernOutput, error)
	UpdateConcern(context.Context, mcpcontract.UpdateConcernInput) (mcpcontract.ConcernOutput, error)
	SetConcernStatus(context.Context, mcpcontract.SetConcernStatusInput) (mcpcontract.ConcernOutput, error)
	LinkConcern(context.Context, mcpcontract.LinkConcernInput) (mcpcontract.ConcernOutput, error)
	PromoteConcern(context.Context, mcpcontract.PromoteConcernInput) (mcpcontract.ConcernOutput, error)
}

// CreateConcernInput records one repository concern and its provenance.

// ListConcernsInput filters and bounds offline concern reads.

// UpdateConcernInput replaces explicitly supplied editable fields.

// SetConcernStatusInput requests one lifecycle transition.

// LinkConcernInput records one typed relationship.

// PromoteConcernInput configures atomic downstream workflow creation.

// ConcernLinkOutput is a transport-safe relationship.

// ConcernPromotionOutput preserves created downstream identities.

// ConcernOutput omits absolute paths and source-reference URLs.

// ConcernListOutput contains one bounded offline result set.

func (s *Server) registerConcernTools() {
	readOnly := readOnlyAnnotations()
	write := localWriteAnnotations(false)
	addCatalogTool(s, catalogTool[mcpcontract.ListConcernsInput, mcpcontract.ConcernListOutput]{
		name: ToolListConcerns, title: "List local concerns",
		description: "List or search compact triage summaries from the repo-local concern ledger. Read a returned resource URI for the complete concern. This never contacts GitHub, reads a worktree, or executes code.",
		annotations: readOnly, supportedBy: supports[ConcernReader], input: inputSchema[mcpcontract.ListConcernsInput](func(sc *schemaBuilder) {
			requireTogether(sc, "owner", "repo")
			setEnum(sc, "status", "untriaged", "accepted", "investigating", "deferred", "promoted", "resolved")
			setRange(sc, "limit", 1, 100)
			setDefault(sc, "limit", 20)
		}), output: outputSchema[mcpcontract.ConcernListOutput]("Bounded concern summaries with derived freshness and exact resource URIs."), handler: s.listConcerns,
	})
	addCatalogTool(s, catalogTool[mcpcontract.CreateConcernInput, mcpcontract.DurableArtifactReference]{
		name: ToolCreateConcern, title: "Create local concern",
		description: "Record a low-confidence repository concern in the local corpus. This does not create an investigation or GitHub issue.",
		annotations: write, supportedBy: supports[ConcernOperator], input: inputSchema[mcpcontract.CreateConcernInput](func(sc *schemaBuilder) {
			setRange(sc, "confidence", 0, 1)
			setArrayBounds(sc, "unknowns", 0, 100)
			setArrayBounds(sc, "evidence_ids", 0, 100)
			setArrayBounds(sc, "source_provenance", 0, 100)
			configureConcernSourceModes(sc)
		}), output: outputSchema[mcpcontract.DurableArtifactReference]("Compact reference to the persisted concern resource."), handler: s.createConcern,
	})
	addCatalogTool(s, catalogTool[mcpcontract.UpdateConcernInput, mcpcontract.DurableArtifactReference]{
		name: ToolUpdateConcern, title: "Update local concern", description: "Update editable fields on one local concern without changing lifecycle status. Use " + ToolSetConcernState + " for status changes.",
		annotations: write, supportedBy: supports[ConcernOperator], input: inputSchema[mcpcontract.UpdateConcernInput](func(sc *schemaBuilder) { setRange(sc, "confidence", 0, 1) }),
		output: outputSchema[mcpcontract.DurableArtifactReference]("Compact reference to the updated concern resource."), handler: s.updateConcern,
	})
	addCatalogTool(s, catalogTool[mcpcontract.SetConcernStatusInput, mcpcontract.DurableArtifactReference]{
		name: ToolSetConcernState, title: "Set local concern status", description: "Apply one validated concern lifecycle transition with a required rationale. Use " + ToolUpdateConcern + " for content changes.",
		annotations: write, supportedBy: supports[ConcernOperator], input: inputSchema[mcpcontract.SetConcernStatusInput](func(sc *schemaBuilder) {
			setEnum(sc, "status", "untriaged", "accepted", "investigating", "deferred", "resolved")
		}), output: outputSchema[mcpcontract.DurableArtifactReference]("Compact reference to the transitioned concern resource."), handler: s.setConcernStatus,
	})
	addCatalogTool(s, catalogTool[mcpcontract.LinkConcernInput, mcpcontract.DurableArtifactReference]{
		name: ToolLinkConcern, title: "Link local concern", description: "Attach an explicit related, duplicate-candidate, hotspot, investigation, or opportunity relationship. Similarity remains a candidate, not a root-cause claim.",
		annotations: localWriteAnnotations(true), supportedBy: supports[ConcernOperator], input: inputSchema[mcpcontract.LinkConcernInput](func(sc *schemaBuilder) {
			setEnum(sc, "kind", "related", "duplicate_candidate", "hotspot", "investigation", "opportunity")
		}), output: outputSchema[mcpcontract.DurableArtifactReference]("Compact reference to the linked concern resource."), handler: s.linkConcern,
	})
	addCatalogTool(s, catalogTool[mcpcontract.PromoteConcernInput, mcpcontract.DurableArtifactReference]{
		name: ToolPromoteConcern, title: "Promote local concern", description: "Atomically promote an accepted or investigating concern to an investigation, or to an investigation plus opportunity, preserving IDs, evidence links, and provenance.",
		annotations: write, supportedBy: supports[ConcernOperator], input: inputSchema[mcpcontract.PromoteConcernInput](func(sc *schemaBuilder) {
			setEnum(sc, "kind", "investigation", "opportunity")
			setEnum(sc, "category", "bug", "performance", "architecture", "testing", "documentation", "maintenance", "compatibility", "security", "other")
		}), output: outputSchema[mcpcontract.DurableArtifactReference]("Compact reference to the promoted concern resource."), handler: s.promoteConcern,
	})
}

func (s *Server) listConcerns(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.ListConcernsInput) (*mcp.CallToolResult, mcpcontract.ConcernListOutput, error) {
	reader, ok := s.reader.(ConcernReader)
	if !ok {
		return nil, mcpcontract.ConcernListOutput{}, errors.New("concern reads are not available")
	}
	out, err := reader.ListConcerns(ctx, in)
	return nil, out, err
}

func (s *Server) createConcern(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.CreateConcernInput) (*mcp.CallToolResult, mcpcontract.DurableArtifactReference, error) {
	if err := validateRepo(mcpcontract.RepoInput{Owner: in.Owner, Repo: in.Repo}); err != nil {
		return nil, mcpcontract.DurableArtifactReference{}, err
	}
	if strings.TrimSpace(in.CommitSHA) == "" && strings.TrimSpace(in.WorkspaceID) == "" {
		return nil, mcpcontract.DurableArtifactReference{}, mcpcontract.InvalidArgument("commit_sha", "commit_sha or workspace_id is required", map[string]any{"commit_sha": "<sha>"})
	}
	operator, ok := s.reader.(ConcernOperator)
	if !ok {
		return nil, mcpcontract.DurableArtifactReference{}, errors.New("concern writes are not available")
	}
	out, err := operator.CreateConcern(ctx, in)
	return concernReference(out, err)
}

func (s *Server) updateConcern(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.UpdateConcernInput) (*mcp.CallToolResult, mcpcontract.DurableArtifactReference, error) {
	return callConcernOperator(ctx, s.reader, func(operator ConcernOperator) (mcpcontract.ConcernOutput, error) {
		return operator.UpdateConcern(ctx, in)
	})
}

func (s *Server) setConcernStatus(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.SetConcernStatusInput) (*mcp.CallToolResult, mcpcontract.DurableArtifactReference, error) {
	return callConcernOperator(ctx, s.reader, func(operator ConcernOperator) (mcpcontract.ConcernOutput, error) {
		return operator.SetConcernStatus(ctx, in)
	})
}

func (s *Server) linkConcern(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.LinkConcernInput) (*mcp.CallToolResult, mcpcontract.DurableArtifactReference, error) {
	return callConcernOperator(ctx, s.reader, func(operator ConcernOperator) (mcpcontract.ConcernOutput, error) {
		return operator.LinkConcern(ctx, in)
	})
}

func (s *Server) promoteConcern(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.PromoteConcernInput) (*mcp.CallToolResult, mcpcontract.DurableArtifactReference, error) {
	return callConcernOperator(ctx, s.reader, func(operator ConcernOperator) (mcpcontract.ConcernOutput, error) {
		return operator.PromoteConcern(ctx, in)
	})
}

func callConcernOperator(ctx context.Context, reader mcpcontract.Reader, call func(ConcernOperator) (mcpcontract.ConcernOutput, error)) (*mcp.CallToolResult, mcpcontract.DurableArtifactReference, error) {
	if err := ctx.Err(); err != nil {
		return nil, mcpcontract.DurableArtifactReference{}, err
	}
	operator, ok := reader.(ConcernOperator)
	if !ok {
		return nil, mcpcontract.DurableArtifactReference{}, errors.New("concern writes are not available")
	}
	out, err := call(operator)
	return concernReference(out, err)
}

func concernReference(out mcpcontract.ConcernOutput, err error) (*mcp.CallToolResult, mcpcontract.DurableArtifactReference, error) {
	if err != nil {
		return nil, mcpcontract.DurableArtifactReference{}, err
	}
	uri := "gitcontribute://concern/" + out.ID
	ref := mcpcontract.DurableArtifactReference{Kind: "concern", ID: out.ID, URI: uri}
	return linkedResource(uri, "concern", "Concern", "Persisted local repository concern."), ref, nil
}
