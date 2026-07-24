package mcpserver

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Canonical MCP tool names group operations by capability and side-effect boundary.

var canonicalToolNames = []string{
	ToolSearchRepositories,
	ToolSearchThreads,
	ToolSearchCode,
	ToolGetRepositories,
	ToolGetThreads,
	ToolRankThreads,
	ToolFindPrecedents,
	ToolGetRepositoryDossier,
	ToolExplainMatch,
	ToolGetInvestigation,
	ToolListOpportunities,
	ToolGetOpportunity,
	ToolGetEvidence,
	ToolGetReadiness,
	ToolListConcerns,
	ToolFindClusters,
	ToolFindNeighbors,
	ToolGetCoverage,
	ToolBuildRepositoryDossier,
	ToolGetJob,
	ToolCancelJob,
	ToolSearchGitHubRepositories,
	ToolSyncRepositoryMetadata,
	ToolSyncThreads,
	ToolHydrateThreads,
	ToolGetAuthenticatedIdentity,
	ToolSyncAuthoredPullRequests,
	ToolSyncPullRequestStatus,
	ToolListPullRequestPortfolio,
	ToolFindPortfolioOverlaps,
	ToolIndexRepositories,
	ToolCheckMergeConflicts,
	ToolInspectCommitChanges,
	ToolPlanSemanticCommits,
	ToolQueryDeepWiki,
	ToolCreateWorkspace,
	ToolAdoptWorkspace,
	ToolDefineValidation,
	ToolRunValidation,
	ToolRunRepeatedValidation,
	ToolStartInvestigation,
	ToolRecordHypothesis,
	ToolCreateConcern,
	ToolUpdateConcern,
	ToolSetConcernState,
	ToolLinkConcern,
	ToolPromoteConcern,
	ToolCheckDuplicates,
	ToolFindCompetingWork,
	ToolPromoteOpportunity,
	ToolPrepareContribution,
	ToolExportManifest,
	ToolLinkPullRequest,
}

type catalogTool[In, Out any] struct {
	name, title, description string
	annotations              *mcp.ToolAnnotations
	supportedBy              func(Reader) bool
	input                    schemaDefinition
	output                   schemaDefinition
	handler                  mcp.ToolHandlerFor[In, Out]
}

func addCatalogTool[In, Out any](server *Server, tool catalogTool[In, Out]) {
	if server.enabledTools != nil {
		if _, enabled := server.enabledTools[tool.name]; !enabled {
			return
		}
	}
	if tool.supportedBy != nil && !tool.supportedBy(server.reader) {
		return
	}
	if server.readOnly && (tool.annotations == nil || !tool.annotations.ReadOnlyHint) {
		return
	}
	if tool.input.err != nil {
		server.recordRegistrationError(tool.name, "input", tool.input.err)
		return
	}
	if tool.output.err != nil {
		server.recordRegistrationError(tool.name, "output", tool.output.err)
		return
	}
	mcp.AddTool(server.server, &mcp.Tool{
		Name:         tool.name,
		Title:        tool.title,
		Description:  tool.description,
		Annotations:  tool.annotations,
		InputSchema:  tool.input.schema,
		OutputSchema: tool.output.schema,
	}, structuredToolErrors(tool.handler))
}

func supports[T any](reader Reader) bool {
	_, ok := any(reader).(T)
	return ok
}

func structuredToolErrors[In, Out any](handler mcp.ToolHandlerFor[In, Out]) mcp.ToolHandlerFor[In, Out] {
	return func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		result, output, err := handler(ctx, request, input)
		if err == nil {
			return result, output, nil
		}
		var toolErr *ToolError
		if errors.As(err, &toolErr) {
			return result, output, toolErr
		}
		code := "operation_failed"
		switch {
		case isNotFound(err):
			code = "not_found"
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			code = "cancelled"
		}
		return result, output, &ToolError{Code: code, Message: err.Error(), Retryable: false}
	}
}

var toolsets = map[string][]string{
	"contribute": {
		ToolSearchRepositories, ToolSearchThreads, ToolGetRepositories, ToolGetThreads,
		ToolRankThreads, ToolFindPrecedents, ToolGetRepositoryDossier,
		ToolGetCoverage, ToolGetJob, ToolCancelJob,
		ToolSearchGitHubRepositories, ToolSyncRepositoryMetadata, ToolSyncThreads, ToolHydrateThreads,
		ToolStartInvestigation, ToolRecordHypothesis, ToolCheckDuplicates, ToolFindCompetingWork,
		ToolPromoteOpportunity, ToolGetInvestigation, ToolListOpportunities, ToolGetOpportunity,
		ToolGetEvidence, ToolGetReadiness, ToolPrepareContribution, ToolExportManifest,
		ToolListConcerns, ToolCreateConcern,
	},
	"code": {
		ToolSearchCode, ToolIndexRepositories, ToolCreateWorkspace, ToolAdoptWorkspace, ToolCheckMergeConflicts,
		ToolInspectCommitChanges, ToolPlanSemanticCommits,
		ToolDefineValidation, ToolRunValidation, ToolRunRepeatedValidation, ToolGetJob, ToolCancelJob,
	},
	"research":    {ToolQueryDeepWiki},
	"diagnostics": {ToolExplainMatch, ToolBuildRepositoryDossier, ToolGetJob},
	"portfolio": {
		ToolGetJob, ToolCancelJob, ToolGetAuthenticatedIdentity, ToolSyncAuthoredPullRequests,
		ToolSyncPullRequestStatus, ToolListPullRequestPortfolio, ToolFindPortfolioOverlaps, ToolLinkPullRequest,
	},
	"advanced": {ToolFindClusters, ToolFindNeighbors},
	"concerns": {ToolListConcerns, ToolCreateConcern, ToolUpdateConcern, ToolSetConcernState, ToolLinkConcern, ToolPromoteConcern},
}

func enabledToolNames(selected []string) map[string]struct{} {
	enabled := make(map[string]struct{})
	for _, name := range selected {
		if name == "all" {
			for _, tool := range canonicalToolNames {
				enabled[tool] = struct{}{}
			}
			return enabled
		}
		for _, tool := range toolsets[name] {
			enabled[tool] = struct{}{}
		}
	}
	return enabled
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

func externalReadAnnotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    true,
		IdempotentHint:  true,
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

func processReadAnnotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: boolPtr(false), DestructiveHint: boolPtr(false)}
}

func cancellationAnnotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    false,
		IdempotentHint:  true,
		OpenWorldHint:   boolPtr(false),
		DestructiveHint: boolPtr(true),
	}
}

func noSchemaCustomization(*schemaBuilder) {}
