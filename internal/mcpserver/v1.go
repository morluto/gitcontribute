package mcpserver

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// V1 read-only tool inputs and outputs.

// SearchRepositoriesInput describes an offline repository search page.

// SearchRepositoriesOutput contains one page of repository matches.

// SearchThreadsInput describes an offline issue and pull-request search page.
type SearchThreadsInput struct {
	Query         string   `json:"query" jsonschema:"Thread full-text query"`
	Owner         string   `json:"owner,omitempty" jsonschema:"Optional repository owner"`
	Repo          string   `json:"repo,omitempty" jsonschema:"Optional repository name"`
	Kind          string   `json:"kind,omitempty" jsonschema:"Optional thread kind: issue or pull_request"`
	State         string   `json:"state,omitempty" jsonschema:"Optional open or closed state"`
	StateReason   string   `json:"state_reason,omitempty" jsonschema:"Optional GitHub completed or not_planned state reason"`
	Merged        *bool    `json:"merged,omitempty" jsonschema:"Optional pull request merged state"`
	Author        string   `json:"author,omitempty" jsonschema:"Optional author login"`
	Association   string   `json:"author_association,omitempty" jsonschema:"Optional GitHub author association"`
	Assignee      string   `json:"assignee,omitempty" jsonschema:"Optional assignee login"`
	Labels        []string `json:"labels,omitempty" jsonschema:"Labels that must all be present"`
	UpdatedAfter  string   `json:"updated_after,omitempty" jsonschema:"Optional RFC 3339 lower bound"`
	UpdatedBefore string   `json:"updated_before,omitempty" jsonschema:"Optional RFC 3339 upper bound"`
	Limit         int      `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
	Cursor        string   `json:"cursor,omitempty" jsonschema:"Opaque cursor returned by the previous page"`
	Sort          string   `json:"sort,omitempty" jsonschema:"Order: relevance or updated"`
	MatchMode     string   `json:"match_mode,omitempty" jsonschema:"Term matching: all requires every term; any requires at least one term"`
	View          string   `json:"view,omitempty" jsonschema:"compact omits full bodies and returns bounded excerpts; full includes stored bodies"`
	SnapshotToken string   `json:"snapshot_token,omitempty" jsonschema:"Optional immutable corpus snapshot token"`
}

// ExplainMatchInput identifies an exact stored result and its original query.

// ExplainMatchOutput reports the stored facts that contributed to a match score.

// GetJobInput selects a durable job by opaque ID.

// GetJobOutput reports durable state and structured progress for a job.

// ThreadByNumberInput identifies a stored issue or pull request by number.

// JobReference is returned by long-running tools that submit durable jobs.

// V1 operation inputs and outputs.

// BuildRepositoryDossierInput selects a repository for durable dossier generation.

// StartInvestigationInput creates a local investigation for a repository revision.

// RecordHypothesisInput records a structured hypothesis and its provenance.

// HypothesisOutput is the stable MCP representation of a hypothesis.

// CheckDuplicatesInput selects a hypothesis or opportunity for duplicate analysis.

// CheckCollisionsInput selects a hypothesis or opportunity for collision analysis.

// CheckOutput contains evidence-backed duplicate or collision findings.

// PromoteOpportunityInput converts a hypothesis into a scoped opportunity.

// CancelJobInput selects durable jobs for bounded, persisted cancellation.

func (s *Server) registerV1() {
	readOnly := readOnlyAnnotations()
	localWrite := localWriteAnnotations(false)
	addCatalogTool(s, catalogTool[mcpcontract.SearchRepositoriesInput, mcpcontract.SearchRepositoriesOutput]{
		name: mcpcontract.ToolSearchRepositories, title: "Search stored repositories",
		description: "Search stored repository names, topics, and descriptions. Results expose incomplete local coverage and an exact typed recovery action; use it before inferring absence. Supports relevance or updated order; never contacts GitHub.",
		annotations: readOnly, input: inputSchema[mcpcontract.SearchRepositoriesInput](func(schema *schemaBuilder) {
			setRange(schema, "limit", 1, 100)
			setDefault(schema, "limit", 20)
			setEnum(schema, "sort", "relevance", "updated")
		}), output: outputSchema[mcpcontract.SearchRepositoriesOutput]("One page of stored repository matches."), handler: s.searchRepositories,
	})
	addCatalogTool(s, catalogTool[SearchThreadsInput, mcpcontract.SearchOutput]{
		name: mcpcontract.ToolSearchThreads, title: "Search stored issues and pull requests",
		description: "Search stored issue and PR titles, labels, bodies, and hydrated text. All terms are required by default; use match_mode=any for broader recall. Compact output is bounded, and local coverage is never proof of absence without the returned typed recovery action. Read finalists with corpus.get_threads and hydrate only missing facets. Offline.",
		annotations: readOnly, input: inputSchema[SearchThreadsInput](func(schema *schemaBuilder) {
			setEnum(schema, "kind", "issue", "pull_request")
			setEnum(schema, "state", "open", "closed")
			setEnum(schema, "state_reason", "completed", "not_planned")
			setEnum(schema, "sort", "relevance", "updated")
			setEnum(schema, "match_mode", "all", "any")
			setDefault(schema, "match_mode", "all")
			setEnum(schema, "view", "compact", "full")
			setDefault(schema, "view", "compact")
			setRange(schema, "limit", 1, 100)
			setDefault(schema, "limit", 20)
		}), output: outputSchema[mcpcontract.SearchOutput]("One page of stored issue and pull-request matches."), handler: s.searchThreads,
	})
	addCatalogTool(s, catalogTool[mcpcontract.ExplainMatchInput, mcpcontract.ExplainMatchOutput]{
		name: mcpcontract.ToolExplainMatch, title: "Explain a stored search match",
		description: "Read the FTS5 rank, stored match source, source revision, and coverage for one prior repository, thread, or code result. It does not reimplement token matching; this tool is offline.",
		annotations: readOnly, input: inputSchema[mcpcontract.ExplainMatchInput](func(schema *schemaBuilder) {
			setEnum(schema, "kind", "repo", "issue", "pull_request", "code")
			setMinimum(schema, "number", 1)
			setRange(schema, "limit", 1, 100)
			setDefault(schema, "limit", 20)
		}), output: outputSchema[mcpcontract.ExplainMatchOutput]("Stored facts and score signals explaining one search match."), handler: s.explainMatch,
	})
	addCatalogTool(s, catalogTool[mcpcontract.BuildRepositoryDossierInput, mcpcontract.JobReference]{
		name: mcpcontract.ToolBuildRepositoryDossier, title: "Build repository dossier",
		description: "Start an asynchronous local job that rebuilds and persists a source-backed dossier from the existing corpus. It performs no network access; read the returned dossier resource after the job succeeds.",
		annotations: localWriteAnnotations(true), supportedBy: supports[Operator], input: inputSchema[mcpcontract.BuildRepositoryDossierInput](noSchemaCustomization),
		output: outputSchema[mcpcontract.JobReference]("Reference to a newly queued dossier build job."), handler: s.buildRepositoryDossier,
	})
	addCatalogTool(s, catalogTool[mcpcontract.CreateWorkspaceInput, mcpcontract.JobReference]{
		name: mcpcontract.ToolCreateWorkspace, title: "Create managed Git workspace",
		description: "Start an asynchronous job that clones the specified remote and creates a managed worktree for an investigation. This performs network reads, Git process execution, filesystem writes, and local metadata writes, but never mutates GitHub.",
		annotations: networkReadAnnotations(), supportedBy: supports[WorkspaceCreator], input: inputSchema[mcpcontract.CreateWorkspaceInput](noSchemaCustomization),
		output: outputSchema[mcpcontract.JobReference]("Reference to a newly queued workspace creation job."), handler: s.createWorkspace,
	})
	addCatalogTool(s, catalogTool[mcpcontract.AdoptWorkspaceInput, mcpcontract.AdoptWorkspaceOutput]{
		name: mcpcontract.ToolAdoptWorkspace, title: "Adopt existing Git worktree",
		description: "Inspect and record an existing local Git worktree for an investigation. This runs read-only Git commands and writes local metadata; it never fetches, changes refs or files, takes deletion ownership, or contacts GitHub.",
		annotations: localWriteAnnotations(true), supportedBy: supports[WorkspaceAdopter], input: inputSchema[mcpcontract.AdoptWorkspaceInput](noSchemaCustomization),
		output: outputSchema[mcpcontract.AdoptWorkspaceOutput]("Persisted external-worktree identity without host paths or remote URLs."), handler: s.adoptWorkspace,
	})
	addCatalogTool(s, catalogTool[mcpcontract.RunValidationInput, mcpcontract.JobReference]{
		name: mcpcontract.ToolRunValidation, title: "Run stored validation command",
		description: "Execute a stored shell-free validation against base, candidate, or both workspaces. Set run_count above 1 for repeated or stress validation with bounded concurrency and telemetry. This can modify the workspace or host through the authorized command and requires execute=true.",
		annotations: executionAnnotations(), supportedBy: supports[Operator], input: inputSchema[mcpcontract.RunValidationInput](func(schema *schemaBuilder) {
			setEnum(schema, "target", "base", "candidate", "both")
			setRange(schema, "run_count", 1, 100)
			setDefault(schema, "run_count", 1)
			setRange(schema, "concurrency", 1, 16)
			setDefault(schema, "concurrency", 1)
			setDefault(schema, "sample_interval", "100ms")
			setConst(schema, "execute", true)
		}), output: outputSchema[mcpcontract.JobReference]("Reference to a newly queued validation execution job."), handler: s.runValidation,
	})
	addCatalogTool(s, catalogTool[mcpcontract.StartInvestigationInput, mcpcontract.DurableArtifactReference]{
		name: mcpcontract.ToolStartInvestigation, title: "Start local investigation",
		description: "Create a durable local investigation from a commit SHA, or atomically create its initial baseline hypothesis from a stored issue or pull-request number. Returns a compact resource reference; read its exact URI for the canonical investigation. This does not create a Git worktree or contact GitHub; use " + mcpcontract.ToolCreateWorkspace + " separately when filesystem work is authorized.",
		annotations: localWrite, supportedBy: supports[Operator], input: inputSchema[mcpcontract.StartInvestigationInput](configureInvestigationSourceModes),
		output: outputSchema[mcpcontract.DurableArtifactReference]("Compact reference to the newly created investigation resource."), handler: s.startInvestigation,
	})
	addCatalogTool(s, catalogTool[mcpcontract.RecordHypothesisInput, mcpcontract.DurableArtifactReference]{
		name: mcpcontract.ToolRecordHypothesis, title: "Record investigation hypothesis",
		description: "Persist a structured hypothesis and source references in an existing local investigation. Use this only after the problem is concrete enough to state expected or observed behavior; it performs no network access.",
		annotations: localWrite, supportedBy: supports[Operator], input: inputSchema[mcpcontract.RecordHypothesisInput](func(schema *schemaBuilder) {
			setEnum(schema, "category", "bug", "performance", "architecture", "testing", "documentation", "maintenance", "compatibility", "security", "other")
		}), output: outputSchema[mcpcontract.DurableArtifactReference]("Compact reference to the updated investigation resource."), handler: s.recordHypothesis,
	})
	addCatalogTool(s, catalogTool[mcpcontract.FindRelatedWorkInput, mcpcontract.FindRelatedWorkOutput]{
		name: mcpcontract.ToolFindRelatedWork, title: "Find related issues and open pull requests",
		description: "Search the local corpus for duplicate issues and pull requests, open pull requests that may compete or conflict, or both for one hypothesis or opportunity. Returns status, coverage, truncation, and exact typed recovery actions when the repository is absent or the bounded result is not exhaustive; this records no evidence, tests no Git merge conflicts, and performs no network access.",
		annotations: readOnly, supportedBy: supports[Operator], input: inputSchema[mcpcontract.FindRelatedWorkInput](func(schema *schemaBuilder) {
			setEnum(schema, "target", "hypothesis", "opportunity")
			setArrayBounds(schema, "kinds", 1, 2)
			setArrayEnum(schema, "kinds", "duplicates", "competing_pull_requests")
			setRange(schema, "limit", 1, 100)
			setDefault(schema, "limit", 20)
		}), output: outputSchema[mcpcontract.FindRelatedWorkOutput]("Requested related-work populations grouped by kind."), handler: s.findRelatedWork,
	})
	addCatalogTool(s, catalogTool[mcpcontract.PromoteOpportunityInput, mcpcontract.DurableArtifactReference]{
		name: mcpcontract.ToolPromoteOpportunity, title: "Promote hypothesis to opportunity",
		description: "Persist a scoped contribution opportunity from an existing hypothesis. Returns a compact resource reference; read its exact URI for impact, effort, confidence, dependencies, and source references. This changes local workflow state but never contacts or mutates GitHub.",
		annotations: localWrite, supportedBy: supports[Operator], input: inputSchema[mcpcontract.PromoteOpportunityInput](func(schema *schemaBuilder) {
			setRange(schema, "confidence", 0, 1)
		}), output: outputSchema[mcpcontract.DurableArtifactReference]("Compact reference to the newly promoted opportunity resource."), handler: s.promoteOpportunity,
	})
	addCatalogTool(s, catalogTool[mcpcontract.DefineValidationInput, mcpcontract.ValidationOutput]{
		name: mcpcontract.ToolDefineValidation, title: "Define validation command",
		description: "Parse and persist a shell-free validation command for managed workspace IDs belonging to the investigation, with an environment allowlist, timeout, output bound, and optional declared MCP stdio adapter. This does not execute the command; use " + mcpcontract.ToolRunValidation + " separately with explicit authorization.",
		annotations: localWrite, supportedBy: supports[Operator], input: inputSchema[mcpcontract.DefineValidationInput](func(schema *schemaBuilder) {
			setDefault(schema, "timeout", "30m")
			setRange(schema, "max_output_bytes", 1, 64*1024*1024)
			setDefault(schema, "max_output_bytes", 64*1024)
			setEnum(schema, "protocol", "mcp_stdio")
			configureValidationObservationSchema(schema)
			configureValidationWorkspaceModes(schema)
		}), output: outputSchema[mcpcontract.ValidationOutput]("Persisted validation definition."), handler: s.defineValidation,
	})
	addCatalogTool(s, catalogTool[mcpcontract.AttachValidationReceiptInput, mcpcontract.ExternalValidationReceiptOutput]{
		name: mcpcontract.ToolAttachValidationReceipt, title: "Attach external validation receipt",
		description: "Validate and persist a bounded, versioned external execution receipt without running its command or contacting its producer.",
		annotations: localWrite, supportedBy: supports[ValidationReceiptOperator],
		input:  inputSchema[mcpcontract.AttachValidationReceiptInput](noSchemaCustomization),
		output: outputSchema[mcpcontract.ExternalValidationReceiptOutput]("Stored external receipt identity and result."), handler: s.attachValidationReceipt,
	})
	addCatalogTool(s, catalogTool[mcpcontract.PrepareContributionInput, mcpcontract.DurableArtifactReference]{
		name: mcpcontract.ToolPrepareContribution, title: "Prepare pull request or issue draft",
		description: "Render and persist a pull request or issue draft from stored evidence, supplied changes, or a verified workspace diff; it inspects the managed workspace with non-mutating Git when changes are omitted. Never posts or mutates GitHub.",
		annotations: localWrite, supportedBy: supports[Operator], input: inputSchema[mcpcontract.PrepareContributionInput](func(schema *schemaBuilder) {
			setEnum(schema, "kind", "issue", "pull_request")
			configureContributionDraftModes(schema)
		}), output: outputSchema[mcpcontract.DurableArtifactReference]("Compact reference to the immutable persisted draft resource."), handler: s.prepareContribution,
	})
	addCatalogTool(s, catalogTool[mcpcontract.VerifyPublishedDraftInput, mcpcontract.PublishedDraftVerificationOutput]{
		name: mcpcontract.ToolVerifyPublishedDraft, title: "Verify published draft bytes",
		description: "Compare one immutable local draft revision with one explicitly synchronized GitHub issue or pull request. Read-only and never refreshes or edits GitHub.",
		annotations: readOnly, supportedBy: supports[PublishedDraftVerifier],
		input: inputSchema[mcpcontract.VerifyPublishedDraftInput](func(schema *schemaBuilder) {
			setEnum(schema, "kind", "issue", "pull_request")
			setRange(schema, "revision", 1, 1<<31-1)
			setRange(schema, "number", 1, 1<<31-1)
		}), output: outputSchema[mcpcontract.PublishedDraftVerificationOutput]("Exact, normalized-only, mismatch, or unknown comparison."), handler: s.verifyPublishedDraft,
	})
	addCatalogTool(s, catalogTool[mcpcontract.ExportManifestInput, mcpcontract.DurableArtifactReference]{
		name: mcpcontract.ToolExportManifest, title: "Export contribution evidence manifest",
		description: "Generate and persist a deterministic local evidence manifest from SQLite and an optional managed workspace snapshot. It may run non-mutating Git commands but never contacts GitHub; sync exact GitHub facets separately before export.",
		annotations: localWrite, supportedBy: supports[Operator], input: inputSchema[mcpcontract.ExportManifestInput](nil),
		output: outputSchema[mcpcontract.DurableArtifactReference]("Compact reference to the persisted contribution evidence manifest."), handler: s.exportManifest,
	})
	addCatalogTool(s, catalogTool[mcpcontract.CancelJobInput, mcpcontract.GetJobsOutput]{
		name: mcpcontract.ToolCancelJob, title: "Cancel durable jobs in one batch",
		description: "Cancel up to 100 durable jobs in order with isolated item outcomes; repeated cancellation is safe.",
		annotations: cancellationAnnotations(), supportedBy: supports[Operator], input: inputSchema[mcpcontract.CancelJobInput](func(sc *schemaBuilder) { setArrayBounds(sc, "ids", 1, 100) }),
		output: outputSchema[mcpcontract.GetJobsOutput]("Ordered durable job states after cancellation requests."), handler: s.cancelJob,
	})
	s.registerConcernTools()
}

func (s *Server) searchRepositories(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.SearchRepositoriesInput) (*mcp.CallToolResult, mcpcontract.SearchRepositoriesOutput, error) {
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return nil, mcpcontract.SearchRepositoriesOutput{}, mcpcontract.InvalidArgument("limit", "must be between 1 and 100", map[string]any{"limit": 20})
	}
	if (in.Owner == "") != (in.Repo == "") {
		return nil, mcpcontract.SearchRepositoriesOutput{}, mcpcontract.InvalidArgument("owner", "owner and repo must be provided together", map[string]any{"owner": "acme", "repo": "rocket"})
	}
	if in.Sort != "" && in.Sort != "relevance" && in.Sort != "updated" {
		return nil, mcpcontract.SearchRepositoriesOutput{}, mcpcontract.InvalidArgument("sort", "must be relevance or updated", map[string]any{"sort": "updated"})
	}
	out, err := s.reader.SearchRepositories(ctx, in)
	return nil, out, err
}

func (s *Server) searchThreads(ctx context.Context, _ *mcp.CallToolRequest, in SearchThreadsInput) (*mcp.CallToolResult, mcpcontract.SearchOutput, error) {
	if in.Query == "" {
		return nil, mcpcontract.SearchOutput{}, mcpcontract.InvalidArgument("query", "is required", map[string]any{"query": "music"})
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return nil, mcpcontract.SearchOutput{}, mcpcontract.InvalidArgument("limit", "must be between 1 and 100", map[string]any{"limit": 20})
	}
	if (in.Owner == "") != (in.Repo == "") {
		return nil, mcpcontract.SearchOutput{}, mcpcontract.InvalidArgument("owner", "owner and repo must be provided together", map[string]any{"owner": "acme", "repo": "rocket"})
	}
	if in.Kind != "" && in.Kind != "issue" && in.Kind != "pull_request" {
		return nil, mcpcontract.SearchOutput{}, mcpcontract.InvalidArgument("kind", "must be issue or pull_request", map[string]any{"kind": "issue"})
	}
	if in.State != "" && in.State != "open" && in.State != "closed" {
		return nil, mcpcontract.SearchOutput{}, mcpcontract.InvalidArgument("state", "must be open or closed", map[string]any{"state": "open"})
	}
	if in.Sort != "" && in.Sort != "relevance" && in.Sort != "updated" {
		return nil, mcpcontract.SearchOutput{}, mcpcontract.InvalidArgument("sort", "must be relevance or updated", map[string]any{"sort": "updated"})
	}
	if in.StateReason != "" && in.StateReason != "completed" && in.StateReason != "not_planned" {
		return nil, mcpcontract.SearchOutput{}, mcpcontract.InvalidArgument("state_reason", "must be completed or not_planned", map[string]any{"state_reason": "completed"})
	}
	if in.MatchMode == "" {
		in.MatchMode = "all"
	}
	if in.MatchMode != "all" && in.MatchMode != "any" {
		return nil, mcpcontract.SearchOutput{}, mcpcontract.InvalidArgument("match_mode", "must be all or any", map[string]any{"match_mode": "all"})
	}
	if in.View == "" {
		in.View = "compact"
	}
	if in.View != "compact" && in.View != "full" {
		return nil, mcpcontract.SearchOutput{}, mcpcontract.InvalidArgument("view", "must be compact or full", map[string]any{"view": "compact"})
	}
	searchIn := mcpcontract.SearchInput(in)
	out, err := s.reader.Search(ctx, searchIn)
	return nil, out, err
}

func (s *Server) explainMatch(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.ExplainMatchInput) (*mcp.CallToolResult, mcpcontract.ExplainMatchOutput, error) {
	if err := validateRepo(mcpcontract.RepoInput{Owner: in.Owner, Repo: in.Repo}); err != nil {
		return nil, mcpcontract.ExplainMatchOutput{}, err
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return nil, mcpcontract.ExplainMatchOutput{}, mcpcontract.InvalidArgument("limit", "must be between 1 and 100", map[string]any{"limit": 20})
	}
	if in.Kind != "" && in.Kind != "repo" && in.Kind != "issue" && in.Kind != "pull_request" && in.Kind != "code" {
		return nil, mcpcontract.ExplainMatchOutput{}, mcpcontract.InvalidArgument("kind", "must be repo, issue, pull_request, or code", map[string]any{"kind": "issue"})
	}
	if (in.Kind == "issue" || in.Kind == "pull_request") && in.Number < 1 {
		return nil, mcpcontract.ExplainMatchOutput{}, mcpcontract.InvalidArgument("number", "is required for issue and pull_request matches", map[string]any{"number": 1})
	}
	if in.Kind == "code" && (strings.TrimSpace(in.Path) == "" || strings.TrimSpace(in.Commit) == "") {
		return nil, mcpcontract.ExplainMatchOutput{}, mcpcontract.InvalidArgument("path", "path and commit are required for code matches", map[string]any{"path": "main.go", "commit": "<sha>"})
	}
	if in.Kind == "repo" && (in.Number != 0 || in.Path != "" || in.Commit != "") || (in.Kind == "issue" || in.Kind == "pull_request") && (in.Path != "" || in.Commit != "") || in.Kind == "code" && in.Number != 0 {
		return nil, mcpcontract.ExplainMatchOutput{}, mcpcontract.InvalidArgument("kind", "identity fields do not match kind; use number for threads, path and commit for code, and neither for repositories", map[string]any{"kind": "issue", "number": 1})
	}
	out, err := s.reader.ExplainMatch(ctx, in)
	return nil, out, err
}

func (s *Server) buildRepositoryDossier(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.BuildRepositoryDossierInput) (*mcp.CallToolResult, mcpcontract.JobReference, error) {
	if err := validateRepo(mcpcontract.RepoInput(in)); err != nil {
		return nil, mcpcontract.JobReference{}, err
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, mcpcontract.JobReference{}, errors.New("dossier build is not available")
	}
	out, err := operator.BuildRepositoryDossier(ctx, in)
	return nil, out, err
}

func (s *Server) startInvestigation(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.StartInvestigationInput) (*mcp.CallToolResult, mcpcontract.DurableArtifactReference, error) {
	if err := validateRepo(mcpcontract.RepoInput{Owner: in.Owner, Repo: in.Repo}); err != nil {
		return nil, mcpcontract.DurableArtifactReference{}, err
	}
	if in.Number <= 0 && strings.TrimSpace(in.CommitSHA) == "" {
		return nil, mcpcontract.DurableArtifactReference{}, mcpcontract.InvalidArgument("commit_sha", "provide commit_sha or a positive stored thread number", map[string]any{"commit_sha": "<sha>"})
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, mcpcontract.DurableArtifactReference{}, errors.New("investigations are not available")
	}
	out, err := operator.StartInvestigation(ctx, in)
	if err != nil {
		return nil, mcpcontract.DurableArtifactReference{}, err
	}
	uri := "gitcontribute://investigation/" + out.ID
	ref := mcpcontract.DurableArtifactReference{Kind: "investigation", ID: out.ID, URI: uri}
	return linkedResource(uri, "investigation", "Investigation", "Persisted local investigation."), ref, nil
}

func (s *Server) recordHypothesis(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.RecordHypothesisInput) (*mcp.CallToolResult, mcpcontract.DurableArtifactReference, error) {
	if _, err := normalizeID("investigation_id", in.InvestigationID); err != nil {
		return nil, mcpcontract.DurableArtifactReference{}, err
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	in.Category = strings.TrimSpace(in.Category)
	if in.Title == "" || in.Description == "" || in.Category == "" {
		return nil, mcpcontract.DurableArtifactReference{}, mcpcontract.InvalidArgument("title", "title, description, and category are required", map[string]any{"title": "Observed behavior", "description": "Describe the evidence-backed hypothesis.", "category": "bug"})
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, mcpcontract.DurableArtifactReference{}, errors.New("hypothesis recording is not available")
	}
	if _, err := operator.RecordHypothesis(ctx, in); err != nil {
		return nil, mcpcontract.DurableArtifactReference{}, err
	}
	uri := "gitcontribute://investigation/" + in.InvestigationID
	ref := mcpcontract.DurableArtifactReference{Kind: "investigation", ID: in.InvestigationID, URI: uri}
	return linkedResource(uri, "investigation", "Investigation", "Investigation containing the persisted hypothesis."), ref, nil
}

func (s *Server) findRelatedWork(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.FindRelatedWorkInput) (*mcp.CallToolResult, mcpcontract.FindRelatedWorkOutput, error) {
	check := mcpcontract.CheckDuplicatesInput{Target: in.Target, ID: in.ID, Limit: in.Limit}
	if err := validateCheckInput(&check); err != nil {
		return nil, mcpcontract.FindRelatedWorkOutput{}, err
	}
	if in.Limit == 0 {
		in.Limit = 20
		check.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return nil, mcpcontract.FindRelatedWorkOutput{}, mcpcontract.InvalidArgument("limit", "must be between 1 and 100", map[string]any{"limit": 20})
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, mcpcontract.FindRelatedWorkOutput{}, errors.New("related-work checks are not available")
	}
	kinds := in.Kinds
	if len(kinds) == 0 {
		kinds = []string{"duplicates", "competing_pull_requests"}
	}
	var out mcpcontract.FindRelatedWorkOutput
	seen := make(map[string]bool, len(kinds))
	for _, kind := range kinds {
		if seen[kind] {
			return nil, mcpcontract.FindRelatedWorkOutput{}, mcpcontract.InvalidArgument("kinds", "must not contain duplicates", map[string]any{"kinds": []string{"duplicates", "competing_pull_requests"}})
		}
		seen[kind] = true
		switch kind {
		case "duplicates":
			value, err := operator.CheckDuplicates(ctx, check)
			if err != nil {
				return nil, mcpcontract.FindRelatedWorkOutput{}, err
			}
			out.Duplicates = &value
		case "competing_pull_requests":
			value, err := operator.CheckCollisions(ctx, mcpcontract.CheckCollisionsInput(check))
			if err != nil {
				return nil, mcpcontract.FindRelatedWorkOutput{}, err
			}
			out.CompetingPullRequests = &value
		default:
			return nil, mcpcontract.FindRelatedWorkOutput{}, mcpcontract.InvalidArgument("kinds", "must contain duplicates or competing_pull_requests", map[string]any{"kinds": []string{"duplicates", "competing_pull_requests"}})
		}
	}
	return nil, out, nil
}

func validateCheckInput(in *mcpcontract.CheckDuplicatesInput) error {
	if _, err := normalizeID("id", in.ID); err != nil {
		return err
	}
	in.Target = strings.ToLower(strings.TrimSpace(in.Target))
	if in.Target != "hypothesis" && in.Target != "opportunity" {
		return mcpcontract.InvalidArgument("target", "must be hypothesis or opportunity", map[string]any{"target": "hypothesis"})
	}
	return nil
}

func (s *Server) promoteOpportunity(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.PromoteOpportunityInput) (*mcp.CallToolResult, mcpcontract.DurableArtifactReference, error) {
	if _, err := normalizeID("hypothesis_id", in.HypothesisID); err != nil {
		return nil, mcpcontract.DurableArtifactReference{}, err
	}
	if strings.TrimSpace(in.ProblemStatement) == "" || strings.TrimSpace(in.Scope) == "" || strings.TrimSpace(in.Impact) == "" || strings.TrimSpace(in.ExpectedEffort) == "" {
		return nil, mcpcontract.DurableArtifactReference{}, mcpcontract.InvalidArgument("problem_statement", "problem_statement, scope, impact, and expected_effort are required", map[string]any{"problem_statement": "Concrete problem", "scope": "Bounded scope", "impact": "Observed impact", "expected_effort": "small"})
	}
	if in.Confidence < 0 || in.Confidence > 1 {
		return nil, mcpcontract.DurableArtifactReference{}, mcpcontract.InvalidArgument("confidence", "must be between 0.0 and 1.0", map[string]any{"confidence": 0.8})
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, mcpcontract.DurableArtifactReference{}, errors.New("opportunity promotion is not available")
	}
	out, err := operator.PromoteOpportunity(ctx, in)
	if err != nil {
		return nil, mcpcontract.DurableArtifactReference{}, err
	}
	uri := "gitcontribute://opportunity/" + out.ID
	ref := mcpcontract.DurableArtifactReference{Kind: "opportunity", ID: out.ID, URI: uri}
	return linkedResource(uri, "opportunity", "Opportunity", "Persisted local contribution opportunity."), ref, nil
}

func (s *Server) cancelJob(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.CancelJobInput) (*mcp.CallToolResult, mcpcontract.GetJobsOutput, error) {
	if len(in.IDs) < 1 || len(in.IDs) > 100 {
		return nil, mcpcontract.GetJobsOutput{}, mcpcontract.InvalidArgument("ids", "must contain 1 to 100 items", map[string]any{"ids": []string{"<job-id>"}})
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, mcpcontract.GetJobsOutput{}, errors.New("job cancellation is not available")
	}
	out, err := operator.CancelJobs(ctx, in)
	for i := range out.Items {
		if out.Items[i].Value != nil {
			normalizeJobExecution(out.Items[i].Value)
		}
	}
	return linkedJobResources(out), out, err
}
