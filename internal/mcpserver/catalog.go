package mcpserver

import (
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Canonical MCP tool names group operations by capability and side-effect boundary.
const (
	ToolSearchRepositories     = "gitcontribute.corpus.search_repositories"
	ToolSearchThreads          = "gitcontribute.corpus.search_threads"
	ToolSearchCode             = "gitcontribute.corpus.search_code"
	ToolGetRepository          = "gitcontribute.corpus.get_repository"
	ToolGetThread              = "gitcontribute.corpus.get_thread"
	ToolGetRepositoryDossier   = "gitcontribute.corpus.get_repository_dossier"
	ToolExplainMatch           = "gitcontribute.corpus.explain_match"
	ToolGetInvestigation       = "gitcontribute.corpus.get_investigation"
	ToolListOpportunities      = "gitcontribute.corpus.list_opportunities"
	ToolGetOpportunity         = "gitcontribute.corpus.get_opportunity"
	ToolGetEvidence            = "gitcontribute.corpus.get_evidence"
	ToolGetReadiness           = "gitcontribute.corpus.get_readiness"
	ToolFindClusters           = "gitcontribute.corpus.find_clusters"
	ToolFindNeighbors          = "gitcontribute.corpus.find_neighbors"
	ToolGetCoverage            = "gitcontribute.corpus.get_coverage"
	ToolGetLens                = "gitcontribute.corpus.get_lens"
	ToolBuildRepositoryDossier = "gitcontribute.corpus.build_repository_dossier"
	ToolGetJob                 = "gitcontribute.jobs.get"
	ToolCancelJob              = "gitcontribute.jobs.cancel"
	ToolStartCrawl             = "gitcontribute.github.start_crawl"
	ToolSyncRepository         = "gitcontribute.github.sync_repository"
	ToolHydrateThread          = "gitcontribute.github.hydrate_thread"
	ToolHydrateRepository      = "gitcontribute.github.hydrate_repository"
	ToolCreateWorkspace        = "gitcontribute.workspace.create"
	ToolDefineValidation       = "gitcontribute.validation.define"
	ToolRunValidation          = "gitcontribute.validation.run"
	ToolStartInvestigation     = "gitcontribute.workflow.start_investigation"
	ToolRecordHypothesis       = "gitcontribute.workflow.record_hypothesis"
	ToolCheckDuplicates        = "gitcontribute.workflow.check_duplicates"
	ToolCheckCollisions        = "gitcontribute.workflow.check_collisions"
	ToolPromoteOpportunity     = "gitcontribute.workflow.promote_opportunity"
	ToolPrepareContribution    = "gitcontribute.workflow.prepare_contribution"
)

var canonicalToolNames = []string{
	ToolSearchRepositories,
	ToolSearchThreads,
	ToolSearchCode,
	ToolGetRepository,
	ToolGetThread,
	ToolGetRepositoryDossier,
	ToolExplainMatch,
	ToolGetInvestigation,
	ToolListOpportunities,
	ToolGetOpportunity,
	ToolGetEvidence,
	ToolGetReadiness,
	ToolFindClusters,
	ToolFindNeighbors,
	ToolGetCoverage,
	ToolGetLens,
	ToolBuildRepositoryDossier,
	ToolGetJob,
	ToolCancelJob,
	ToolStartCrawl,
	ToolSyncRepository,
	ToolHydrateThread,
	ToolHydrateRepository,
	ToolCreateWorkspace,
	ToolDefineValidation,
	ToolRunValidation,
	ToolStartInvestigation,
	ToolRecordHypothesis,
	ToolCheckDuplicates,
	ToolCheckCollisions,
	ToolPromoteOpportunity,
	ToolPrepareContribution,
}

type catalogTool[In, Out any] struct {
	name, title, description string
	annotations              *mcp.ToolAnnotations
	input                    *jsonschema.Schema
	output                   *jsonschema.Schema
	handler                  mcp.ToolHandlerFor[In, Out]
}

func addCatalogTool[In, Out any](server *mcp.Server, tool catalogTool[In, Out]) {
	mcp.AddTool(server, &mcp.Tool{
		Name:         tool.name,
		Title:        tool.title,
		Description:  tool.description,
		Annotations:  tool.annotations,
		InputSchema:  tool.input,
		OutputSchema: tool.output,
	}, tool.handler)
}

func readOnlyAnnotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    true,
		IdempotentHint:  true,
		OpenWorldHint:   boolPtr(false),
		DestructiveHint: boolPtr(false),
	}
}

func localWriteAnnotations(idempotent bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    false,
		IdempotentHint:  idempotent,
		OpenWorldHint:   boolPtr(false),
		DestructiveHint: boolPtr(false),
	}
}

func networkReadAnnotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    false,
		IdempotentHint:  false,
		OpenWorldHint:   boolPtr(true),
		DestructiveHint: boolPtr(false),
	}
}

func executionAnnotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    false,
		IdempotentHint:  false,
		OpenWorldHint:   boolPtr(false),
		DestructiveHint: boolPtr(true),
	}
}

func cancellationAnnotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    false,
		IdempotentHint:  true,
		OpenWorldHint:   boolPtr(false),
		DestructiveHint: boolPtr(true),
	}
}

func noSchemaCustomization(*jsonschema.Schema) {}
