package mcpserver

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

type catalogTool[In, Out any] struct {
	name, title, description string
	annotations              *mcp.ToolAnnotations
	supportedBy              func(mcpcontract.Reader) bool
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

func supports[T any](reader mcpcontract.Reader) bool {
	_, ok := any(reader).(T)
	return ok
}

// allToolNames is a test and selection projection of the named toolsets. The
// runtime "all" profile does not use this list: it leaves filtering disabled
// and therefore follows the tools actually registered by the server.
func allToolNames() map[string]struct{} {
	all := make(map[string]struct{})
	for _, names := range toolsets {
		for _, name := range names {
			all[name] = struct{}{}
		}
	}
	return all
}

func structuredToolErrors[In, Out any](handler mcp.ToolHandlerFor[In, Out]) mcp.ToolHandlerFor[In, Out] {
	return func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		result, output, err := handler(ctx, request, input)
		if err == nil {
			return result, output, nil
		}
		var toolErr *mcpcontract.ToolError
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
		return result, output, &mcpcontract.ToolError{Code: code, Message: err.Error(), Retryable: false}
	}
}

var toolsets = map[string][]string{
	"contribute": {
		mcpcontract.ToolSearchRepositories, mcpcontract.ToolSearchThreads, mcpcontract.ToolGetRepositories, mcpcontract.ToolGetThreads,
		mcpcontract.ToolRankThreads, mcpcontract.ToolFindPrecedents, mcpcontract.ToolPrepareIssueSet, mcpcontract.ToolGetRepositoryDossier,
		mcpcontract.ToolGetCoverage, mcpcontract.ToolGetJob, mcpcontract.ToolCancelJob,
		mcpcontract.ToolSearchGitHubRepositories, mcpcontract.ToolSyncRepositoryContext, mcpcontract.ToolSyncThreads, mcpcontract.ToolHydrateThreads,
		mcpcontract.ToolStartInvestigation, mcpcontract.ToolRecordHypothesis, mcpcontract.ToolCheckDuplicates, mcpcontract.ToolFindCompetingWork,
		mcpcontract.ToolPromoteOpportunity, mcpcontract.ToolGetInvestigation, mcpcontract.ToolListOpportunities, mcpcontract.ToolGetOpportunity,
		mcpcontract.ToolGetEvidence, mcpcontract.ToolGetReadiness, mcpcontract.ToolPrepareContribution, mcpcontract.ToolVerifyPublishedDraft, mcpcontract.ToolExportManifest,
	},
	"code": {
		mcpcontract.ToolSearchCode, mcpcontract.ToolIndexRepositories, mcpcontract.ToolCreateWorkspace, mcpcontract.ToolAdoptWorkspace, mcpcontract.ToolCheckMergeConflicts,
		mcpcontract.ToolInspectCommitChanges, mcpcontract.ToolPlanSemanticCommits,
		mcpcontract.ToolDefineValidation, mcpcontract.ToolRunValidation, mcpcontract.ToolRunRepeatedValidation, mcpcontract.ToolAttachValidationReceipt,
		mcpcontract.ToolGetJob, mcpcontract.ToolCancelJob,
	},
	"research":    {mcpcontract.ToolQueryDeepWiki},
	"diagnostics": {mcpcontract.ToolExplainMatch, mcpcontract.ToolBuildRepositoryDossier, mcpcontract.ToolGetJob},
	"portfolio": {
		mcpcontract.ToolGetJob, mcpcontract.ToolCancelJob, mcpcontract.ToolGetAuthenticatedIdentity, mcpcontract.ToolSyncAuthoredPullRequests,
		mcpcontract.ToolSyncPullRequestStatus, mcpcontract.ToolSyncPortfolio, mcpcontract.ToolListPullRequestPortfolio, mcpcontract.ToolFindPortfolioOverlaps, mcpcontract.ToolLinkPullRequest,
	},
	"advanced": {mcpcontract.ToolFindClusters, mcpcontract.ToolFindNeighbors},
	"patterns": {mcpcontract.ToolMineRepositoryFixPatterns, mcpcontract.ToolGetJob, mcpcontract.ToolCancelJob},
	"concerns": {ToolListConcerns, ToolCreateConcern, ToolUpdateConcern, ToolSetConcernState, ToolLinkConcern, ToolPromoteConcern},
}

func enabledToolNames(selected []string) map[string]struct{} {
	enabled := make(map[string]struct{})
	for _, name := range selected {
		if name == "all" {
			return allToolNames()
		}
		for _, tool := range toolsets[name] {
			enabled[tool] = struct{}{}
		}
	}
	return enabled
}

func toolFilter(selected []string) map[string]struct{} {
	for _, name := range selected {
		if name == "all" {
			return nil
		}
	}
	return enabledToolNames(selected)
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
