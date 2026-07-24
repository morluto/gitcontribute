package mcpserver

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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
	ListConcerns(context.Context, ListConcernsInput) (ConcernListOutput, error)
}

// ConcernOperator exposes local concern-ledger writes.
type ConcernOperator interface {
	CreateConcern(context.Context, CreateConcernInput) (ConcernOutput, error)
	UpdateConcern(context.Context, UpdateConcernInput) (ConcernOutput, error)
	SetConcernStatus(context.Context, SetConcernStatusInput) (ConcernOutput, error)
	LinkConcern(context.Context, LinkConcernInput) (ConcernOutput, error)
	PromoteConcern(context.Context, PromoteConcernInput) (ConcernOutput, error)
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
	addCatalogTool(s, catalogTool[ListConcernsInput, ConcernListOutput]{
		name: ToolListConcerns, title: "List local concerns",
		description: "List or search the bounded repo-local concern ledger using stored SQLite data. This never contacts GitHub, reads a worktree, or executes code.",
		annotations: readOnly, supportedBy: supports[ConcernReader], input: inputSchema[ListConcernsInput](func(sc *schemaBuilder) {
			requireTogether(sc, "owner", "repo")
			setEnum(sc, "status", "untriaged", "accepted", "investigating", "deferred", "promoted", "resolved")
			setRange(sc, "limit", 1, 100)
			setDefault(sc, "limit", 20)
		}), output: outputSchema[ConcernListOutput]("Bounded local concern results with derived freshness."), handler: s.listConcerns,
	})
	addCatalogTool(s, catalogTool[CreateConcernInput, ConcernOutput]{
		name: ToolCreateConcern, title: "Create local concern",
		description: "Record a low-confidence repository concern in the local corpus. This does not create an investigation or GitHub issue.",
		annotations: write, supportedBy: supports[ConcernOperator], input: inputSchema[CreateConcernInput](func(sc *schemaBuilder) {
			setRange(sc, "confidence", 0, 1)
			setArrayBounds(sc, "unknowns", 0, 100)
			setArrayBounds(sc, "evidence_ids", 0, 100)
			setArrayBounds(sc, "source_provenance", 0, 100)
		}), output: outputSchema[ConcernOutput]("Persisted local concern without absolute paths or source URLs."), handler: s.createConcern,
	})
	addCatalogTool(s, catalogTool[UpdateConcernInput, ConcernOutput]{
		name: ToolUpdateConcern, title: "Update local concern", description: "Update editable fields on one local concern without changing lifecycle status. Use " + ToolSetConcernState + " for status changes.",
		annotations: write, supportedBy: supports[ConcernOperator], input: inputSchema[UpdateConcernInput](func(sc *schemaBuilder) { setRange(sc, "confidence", 0, 1) }),
		output: outputSchema[ConcernOutput]("Updated local concern."), handler: s.updateConcern,
	})
	addCatalogTool(s, catalogTool[SetConcernStatusInput, ConcernOutput]{
		name: ToolSetConcernState, title: "Set local concern status", description: "Apply one validated concern lifecycle transition with a required rationale. Use " + ToolUpdateConcern + " for content changes.",
		annotations: write, supportedBy: supports[ConcernOperator], input: inputSchema[SetConcernStatusInput](func(sc *schemaBuilder) {
			setEnum(sc, "status", "untriaged", "accepted", "investigating", "deferred", "resolved")
		}), output: outputSchema[ConcernOutput]("Concern after its lifecycle transition."), handler: s.setConcernStatus,
	})
	addCatalogTool(s, catalogTool[LinkConcernInput, ConcernOutput]{
		name: ToolLinkConcern, title: "Link local concern", description: "Attach an explicit related, duplicate-candidate, hotspot, investigation, or opportunity relationship. Similarity remains a candidate, not a root-cause claim.",
		annotations: localWriteAnnotations(true), supportedBy: supports[ConcernOperator], input: inputSchema[LinkConcernInput](func(sc *schemaBuilder) {
			setEnum(sc, "kind", "related", "duplicate_candidate", "hotspot", "investigation", "opportunity")
		}), output: outputSchema[ConcernOutput]("Concern with explicit relationships."), handler: s.linkConcern,
	})
	addCatalogTool(s, catalogTool[PromoteConcernInput, ConcernOutput]{
		name: ToolPromoteConcern, title: "Promote local concern", description: "Atomically promote an accepted or investigating concern to an investigation, or to an investigation plus opportunity, preserving IDs, evidence links, and provenance.",
		annotations: write, supportedBy: supports[ConcernOperator], input: inputSchema[PromoteConcernInput](func(sc *schemaBuilder) {
			setEnum(sc, "kind", "investigation", "opportunity")
			setEnum(sc, "category", "bug", "performance", "architecture", "testing", "documentation", "maintenance", "compatibility", "security", "other")
		}), output: outputSchema[ConcernOutput]("Promoted concern and downstream workflow identity."), handler: s.promoteConcern,
	})
}

func (s *Server) listConcerns(ctx context.Context, _ *mcp.CallToolRequest, in ListConcernsInput) (*mcp.CallToolResult, ConcernListOutput, error) {
	reader, ok := s.reader.(ConcernReader)
	if !ok {
		return nil, ConcernListOutput{}, errors.New("concern reads are not available")
	}
	out, err := reader.ListConcerns(ctx, in)
	return nil, out, err
}

func (s *Server) createConcern(ctx context.Context, _ *mcp.CallToolRequest, in CreateConcernInput) (*mcp.CallToolResult, ConcernOutput, error) {
	if err := validateRepo(RepoInput{Owner: in.Owner, Repo: in.Repo}); err != nil {
		return nil, ConcernOutput{}, err
	}
	if strings.TrimSpace(in.CommitSHA) == "" && strings.TrimSpace(in.WorkspaceID) == "" {
		return nil, ConcernOutput{}, errors.New("commit_sha or workspace_id is required")
	}
	operator, ok := s.reader.(ConcernOperator)
	if !ok {
		return nil, ConcernOutput{}, errors.New("concern writes are not available")
	}
	out, err := operator.CreateConcern(ctx, in)
	return nil, out, err
}

func (s *Server) updateConcern(ctx context.Context, _ *mcp.CallToolRequest, in UpdateConcernInput) (*mcp.CallToolResult, ConcernOutput, error) {
	return callConcernOperator(ctx, s.reader, func(operator ConcernOperator) (ConcernOutput, error) { return operator.UpdateConcern(ctx, in) })
}

func (s *Server) setConcernStatus(ctx context.Context, _ *mcp.CallToolRequest, in SetConcernStatusInput) (*mcp.CallToolResult, ConcernOutput, error) {
	return callConcernOperator(ctx, s.reader, func(operator ConcernOperator) (ConcernOutput, error) { return operator.SetConcernStatus(ctx, in) })
}

func (s *Server) linkConcern(ctx context.Context, _ *mcp.CallToolRequest, in LinkConcernInput) (*mcp.CallToolResult, ConcernOutput, error) {
	return callConcernOperator(ctx, s.reader, func(operator ConcernOperator) (ConcernOutput, error) { return operator.LinkConcern(ctx, in) })
}

func (s *Server) promoteConcern(ctx context.Context, _ *mcp.CallToolRequest, in PromoteConcernInput) (*mcp.CallToolResult, ConcernOutput, error) {
	return callConcernOperator(ctx, s.reader, func(operator ConcernOperator) (ConcernOutput, error) { return operator.PromoteConcern(ctx, in) })
}

func callConcernOperator(ctx context.Context, reader Reader, call func(ConcernOperator) (ConcernOutput, error)) (*mcp.CallToolResult, ConcernOutput, error) {
	if err := ctx.Err(); err != nil {
		return nil, ConcernOutput{}, err
	}
	operator, ok := reader.(ConcernOperator)
	if !ok {
		return nil, ConcernOutput{}, errors.New("concern writes are not available")
	}
	out, err := call(operator)
	return nil, out, err
}
