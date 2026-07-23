package mcpserver

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// V1 read-only tool inputs and outputs.

// SearchRepositoriesInput describes an offline repository search page.
type SearchRepositoriesInput struct {
	Query  string `json:"query,omitempty" jsonschema:"Repository full-text query"`
	Owner  string `json:"owner,omitempty" jsonschema:"Optional repository owner"`
	Repo   string `json:"repo,omitempty" jsonschema:"Optional repository name"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
	Cursor string `json:"cursor,omitempty" jsonschema:"Opaque cursor returned by the previous page"`
	Sort   string `json:"sort,omitempty" jsonschema:"Order: relevance or updated"`
}

// SearchRepositoriesOutput contains one page of repository matches.
type SearchRepositoriesOutput struct {
	Query      string             `json:"query"`
	Total      int                `json:"total"`
	Matches    []RepositoryOutput `json:"matches"`
	NextCursor string             `json:"next_cursor,omitempty"`
}

// SearchThreadsInput describes an offline issue and pull-request search page.
type SearchThreadsInput struct {
	Query        string   `json:"query" jsonschema:"Thread full-text query"`
	Owner        string   `json:"owner,omitempty" jsonschema:"Optional repository owner"`
	Repo         string   `json:"repo,omitempty" jsonschema:"Optional repository name"`
	Kind         string   `json:"kind,omitempty" jsonschema:"Optional thread kind: issue or pull_request"`
	State        string   `json:"state,omitempty" jsonschema:"Optional open or closed state"`
	StateReason  string   `json:"state_reason,omitempty" jsonschema:"Optional GitHub completed or not_planned state reason"`
	Merged       *bool    `json:"merged,omitempty" jsonschema:"Optional pull request merged state"`
	Author       string   `json:"author,omitempty" jsonschema:"Optional author login"`
	Association  string   `json:"author_association,omitempty" jsonschema:"Optional GitHub author association"`
	Assignee     string   `json:"assignee,omitempty" jsonschema:"Optional assignee login"`
	Labels       []string `json:"labels,omitempty" jsonschema:"Labels that must all be present"`
	UpdatedAfter string   `json:"updated_after,omitempty" jsonschema:"Optional RFC 3339 lower bound"`
	Limit        int      `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
	Cursor       string   `json:"cursor,omitempty" jsonschema:"Opaque cursor returned by the previous page"`
	Sort         string   `json:"sort,omitempty" jsonschema:"Order: relevance or updated"`
}

// GetRepositoryDossierInput selects a persisted repository dossier.
type GetRepositoryDossierInput RepoInput

// ExplainMatchInput identifies an exact stored result and its original query.
type ExplainMatchInput struct {
	Query  string `json:"query,omitempty" jsonschema:"Original search query"`
	Owner  string `json:"owner" jsonschema:"Repository owner"`
	Repo   string `json:"repo" jsonschema:"Repository name"`
	Kind   string `json:"kind,omitempty" jsonschema:"Match kind: repo, issue, pull_request, or code"`
	Number int    `json:"number,omitempty" jsonschema:"Thread number for issue or pull_request matches"`
	Path   string `json:"path,omitempty" jsonschema:"File path for code matches"`
	Commit string `json:"commit,omitempty" jsonschema:"Commit SHA for code matches"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum explanation facets from 1 to 100"`
}

// ExplainMatchOutput reports the stored facts that contributed to a match score.
type ExplainMatchOutput struct {
	Query           string                `json:"query"`
	Kind            string                `json:"kind"`
	Owner           string                `json:"owner"`
	Repo            string                `json:"repo"`
	Number          int                   `json:"number,omitempty"`
	Path            string                `json:"path,omitempty"`
	Commit          string                `json:"commit,omitempty"`
	State           string                `json:"state,omitempty"`
	Title           string                `json:"title"`
	Snippet         string                `json:"snippet,omitempty"`
	MatchSource     string                `json:"match_source,omitempty" jsonschema:"Stored search document or hydrated facet that matched"`
	RetrievalRank   *float64              `json:"retrieval_rank,omitempty" jsonschema:"Lower-is-better retrieval rank"`
	RankingMethod   string                `json:"ranking_method,omitempty" jsonschema:"Retrieval ranking method"`
	SearchTruncated bool                  `json:"search_truncated,omitempty" jsonschema:"Whether indexed hydrated text was bounded"`
	Reason          string                `json:"reason"`
	SourceRevision  string                `json:"source_revision,omitempty"`
	Facets          []FacetCoverageOutput `json:"facets,omitempty"`
	AsOf            string                `json:"as_of,omitempty"`
}

// GetJobInput selects a durable job by opaque ID.
type GetJobInput struct {
	ID string `json:"id" jsonschema:"Durable job ID"`
}

// GetJobOutput reports durable state and structured progress for a job.
type GetJobOutput struct {
	ID                    string `json:"id"`
	Kind                  string `json:"kind"`
	Status                string `json:"status"`
	Request               any    `json:"request,omitempty"`
	Result                any    `json:"result,omitempty"`
	Error                 string `json:"error,omitempty"`
	Phase                 string `json:"phase,omitempty"`
	CompletedItems        int    `json:"completed_items"`
	TotalItems            int    `json:"total_items"`
	ProgressPercent       int    `json:"progress_percent"`
	RetryAfterMS          int    `json:"retry_after_ms,omitempty"`
	CreatedAt             string `json:"created_at"`
	StartedAt             string `json:"started_at,omitempty"`
	CompletedAt           string `json:"completed_at,omitempty"`
	CancelledAt           string `json:"cancelled_at,omitempty"`
	CancellationRequested bool   `json:"cancellation_requested"`
}

// ThreadByNumberInput identifies a stored issue or pull request by number.
type ThreadByNumberInput struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Number int    `json:"number"`
}

// JobReference is returned by long-running tools that submit durable jobs.
type JobReference struct {
	ID               string            `json:"id"`
	Ref              string            `json:"ref"`
	Kind             string            `json:"kind"`
	Status           string            `json:"status"`
	Message          string            `json:"message"`
	PollAfterMS      int               `json:"poll_after_ms,omitempty"`
	SuggestedActions []SuggestedAction `json:"suggested_actions,omitempty"`
}

// V1 operation inputs and outputs.

// BuildRepositoryDossierInput selects a repository for durable dossier generation.
type BuildRepositoryDossierInput RepoInput

// CreateWorkspaceInput configures a durable managed-workspace creation job.
type CreateWorkspaceInput struct {
	InvestigationID string `json:"investigation_id" jsonschema:"Investigation ID"`
	Remote          string `json:"remote,omitempty" jsonschema:"Git remote URL to clone; defaults to the investigation repository"`
	BaseRef         string `json:"base_ref,omitempty" jsonschema:"Base ref to resolve; defaults to the remote HEAD"`
	CandidateRef    string `json:"candidate_ref,omitempty" jsonschema:"Candidate ref to resolve; defaults to the investigation commit"`
	Name            string `json:"name,omitempty" jsonschema:"Workspace name; defaults to a generated ID"`
}

// AdoptWorkspaceInput identifies an existing local worktree and an already
// available base revision. Adoption never fetches or changes the worktree.
type AdoptWorkspaceInput struct {
	InvestigationID string `json:"investigation_id" jsonschema:"Investigation ID"`
	Path            string `json:"path" jsonschema:"Existing local worktree root"`
	BaseRef         string `json:"base_ref" jsonschema:"Base ref already available in the repository"`
	Name            string `json:"name,omitempty" jsonschema:"Workspace name; defaults to a generated ID"`
}

// AdoptWorkspaceOutput deliberately omits host paths and remote URLs.
type AdoptWorkspaceOutput struct {
	ID              string `json:"id" jsonschema:"Workspace ID"`
	InvestigationID string `json:"investigation_id" jsonschema:"Investigation ID"`
	Owner           string `json:"owner" jsonschema:"Repository owner"`
	Repo            string `json:"repo" jsonschema:"Repository name"`
	BaseSHA         string `json:"base_sha" jsonschema:"Resolved base commit"`
	CandidateSHA    string `json:"candidate_sha" jsonschema:"Worktree HEAD observed during adoption"`
	MergeBase       string `json:"merge_base" jsonschema:"Merge base of base and candidate commits"`
	Dirty           bool   `json:"dirty" jsonschema:"Whether tracked or untracked changes were observed"`
	HasUntracked    bool   `json:"has_untracked" jsonschema:"Whether untracked non-ignored files were observed"`
	Ownership       string `json:"ownership" jsonschema:"Workspace ownership classification"`
}

// StartInvestigationInput creates a local investigation for a repository revision.
type StartInvestigationInput struct {
	Owner     string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo      string `json:"repo" jsonschema:"GitHub repository name"`
	CommitSHA string `json:"commit_sha,omitempty" jsonschema:"Required commit SHA unless number selects a stored thread"`
	Lens      string `json:"lens,omitempty" jsonschema:"Optional lens name"`
	Kind      string `json:"kind,omitempty" jsonschema:"Optional stored thread kind"`
	Number    int    `json:"number,omitempty" jsonschema:"Stored thread number for atomic baseline creation"`
}

// RecordHypothesisInput records a structured hypothesis and its provenance.
type RecordHypothesisInput struct {
	InvestigationID    string      `json:"investigation_id" jsonschema:"Investigation ID"`
	Title              string      `json:"title" jsonschema:"Hypothesis title"`
	Description        string      `json:"description" jsonschema:"Hypothesis description"`
	Category           string      `json:"category" jsonschema:"Category such as bug, performance, or documentation"`
	ExpectedBehavior   string      `json:"expected_behavior,omitempty" jsonschema:"Expected behavior"`
	ObservedBehavior   string      `json:"observed_behavior,omitempty" jsonschema:"Observed behavior"`
	PotentialImpact    string      `json:"potential_impact,omitempty" jsonschema:"Potential impact"`
	OpenQuestions      []string    `json:"open_questions,omitempty" jsonschema:"Open questions"`
	AffectedComponents []string    `json:"affected_components,omitempty" jsonschema:"Affected components"`
	SourceRefs         []SourceRef `json:"source_refs,omitempty" jsonschema:"Source references"`
}

// HypothesisOutput is the stable MCP representation of a hypothesis.
type HypothesisOutput struct {
	ID                 string      `json:"id"`
	InvestigationID    string      `json:"investigation_id"`
	Title              string      `json:"title"`
	Description        string      `json:"description"`
	Category           string      `json:"category"`
	ExpectedBehavior   string      `json:"expected_behavior,omitempty"`
	ObservedBehavior   string      `json:"observed_behavior,omitempty"`
	PotentialImpact    string      `json:"potential_impact,omitempty"`
	OpenQuestions      []string    `json:"open_questions,omitempty"`
	AffectedComponents []string    `json:"affected_components,omitempty"`
	SourceRefs         []SourceRef `json:"source_refs,omitempty"`
	Status             string      `json:"status"`
	CreatedAt          string      `json:"created_at"`
	UpdatedAt          string      `json:"updated_at"`
}

// CheckDuplicatesInput selects a hypothesis or opportunity for duplicate analysis.
type CheckDuplicatesInput struct {
	Target string `json:"target" jsonschema:"Target scope: hypothesis or opportunity"`
	ID     string `json:"id" jsonschema:"Hypothesis or opportunity ID"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum findings from 1 to 100"`
}

// CheckCollisionsInput selects a hypothesis or opportunity for collision analysis.
type CheckCollisionsInput CheckDuplicatesInput

// CheckOutput contains evidence-backed duplicate or collision findings.
type CheckOutput struct {
	Target         string         `json:"target"`
	ID             string         `json:"id"`
	Repo           string         `json:"repo,omitempty"`
	Query          string         `json:"query,omitempty"`
	Total          int            `json:"total"`
	Findings       []EvidenceItem `json:"findings,omitempty"`
	SourceRevision string         `json:"source_revision,omitempty"`
	Limit          int            `json:"limit"`
}

// PromoteOpportunityInput converts a hypothesis into a scoped opportunity.
type PromoteOpportunityInput struct {
	HypothesisID        string      `json:"hypothesis_id" jsonschema:"Hypothesis ID to promote"`
	ProblemStatement    string      `json:"problem_statement" jsonschema:"Problem statement"`
	Scope               string      `json:"scope" jsonschema:"Scope of the opportunity"`
	Impact              string      `json:"impact" jsonschema:"Impact of the opportunity"`
	ExpectedEffort      string      `json:"expected_effort" jsonschema:"Expected effort"`
	Confidence          float64     `json:"confidence" jsonschema:"Confidence from 0.0 to 1.0"`
	Dependencies        []string    `json:"dependencies,omitempty" jsonschema:"Dependencies"`
	MaintainerAlignment string      `json:"maintainer_alignment,omitempty" jsonschema:"Maintainer alignment note"`
	SourceRefs          []SourceRef `json:"source_refs,omitempty" jsonschema:"Source references"`
}

// CancelJobInput selects durable jobs for bounded, persisted cancellation.
type CancelJobInput struct {
	IDs []string `json:"ids" jsonschema:"One to 100 durable job IDs"`
}

func (s *Server) registerV1() {
	readOnly := readOnlyAnnotations()
	localWrite := localWriteAnnotations(false)
	addCatalogTool(s, catalogTool[SearchRepositoriesInput, SearchRepositoriesOutput]{
		name: ToolSearchRepositories, title: "Search stored repositories",
		description: "Search stored repository names, topics, and descriptions. Supports relevance or updated order; never contacts GitHub.",
		annotations: readOnly, input: inputSchema[SearchRepositoriesInput](func(schema *schemaBuilder) {
			setRange(schema, "limit", 1, 100)
			setDefault(schema, "limit", 20)
			setEnum(schema, "sort", "relevance", "updated")
		}), output: outputSchema[SearchRepositoriesOutput]("One page of stored repository matches."), handler: s.searchRepositories,
	})
	addCatalogTool(s, catalogTool[SearchThreadsInput, SearchOutput]{
		name: ToolSearchThreads, title: "Search stored issues and pull requests",
		description: "Search stored issue and PR titles, labels, bodies, and hydrated text. Supports relevance or updated order; never contacts GitHub.",
		annotations: readOnly, input: inputSchema[SearchThreadsInput](func(schema *schemaBuilder) {
			setEnum(schema, "kind", "issue", "pull_request")
			setEnum(schema, "state", "open", "closed")
			setEnum(schema, "state_reason", "completed", "not_planned")
			setEnum(schema, "sort", "relevance", "updated")
			setRange(schema, "limit", 1, 100)
			setDefault(schema, "limit", 20)
		}), output: outputSchema[SearchOutput]("One page of stored issue and pull-request matches."), handler: s.searchThreads,
	})
	addCatalogTool(s, catalogTool[GetRepositoryDossierInput, DossierOutput]{
		name: ToolGetRepositoryDossier, title: "Get repository dossier",
		description: "Read the latest persisted source-backed dossier for one repository. Use " + ToolBuildRepositoryDossier + " only when the local dossier must be regenerated; this read never performs that write.",
		annotations: readOnly, input: inputSchema[GetRepositoryDossierInput](noSchemaCustomization),
		output: outputSchema[DossierOutput]("Persisted source-backed repository dossier."), handler: s.getRepositoryDossier,
	})
	addCatalogTool(s, catalogTool[ExplainMatchInput, ExplainMatchOutput]{
		name: ToolExplainMatch, title: "Explain a stored search match",
		description: "Read the FTS5 rank, stored match source, source revision, and coverage for one prior repository, thread, or code result. It does not reimplement token matching; this tool is offline.",
		annotations: readOnly, input: inputSchema[ExplainMatchInput](func(schema *schemaBuilder) {
			setEnum(schema, "kind", "repo", "issue", "pull_request", "code")
			setMinimum(schema, "number", 1)
			setRange(schema, "limit", 1, 100)
			setDefault(schema, "limit", 20)
		}), output: outputSchema[ExplainMatchOutput]("Stored facts and score signals explaining one search match."), handler: s.explainMatch,
	})
	addCatalogTool(s, catalogTool[BuildRepositoryDossierInput, JobReference]{
		name: ToolBuildRepositoryDossier, title: "Build repository dossier",
		description: "Start an asynchronous local job that rebuilds and persists a source-backed dossier from the existing corpus. It performs no network access; use " + ToolGetRepositoryDossier + " after the job succeeds.",
		annotations: localWriteAnnotations(true), supportedBy: supports[Operator], input: inputSchema[BuildRepositoryDossierInput](noSchemaCustomization),
		output: outputSchema[JobReference]("Reference to a newly queued dossier build job."), handler: s.buildRepositoryDossier,
	})
	addCatalogTool(s, catalogTool[CreateWorkspaceInput, JobReference]{
		name: ToolCreateWorkspace, title: "Create managed Git workspace",
		description: "Start an asynchronous job that clones the specified remote and creates a managed worktree for an investigation. This performs network reads, Git process execution, filesystem writes, and local metadata writes, but never mutates GitHub.",
		annotations: networkReadAnnotations(), supportedBy: supports[WorkspaceCreator], input: inputSchema[CreateWorkspaceInput](noSchemaCustomization),
		output: outputSchema[JobReference]("Reference to a newly queued workspace creation job."), handler: s.createWorkspace,
	})
	addCatalogTool(s, catalogTool[AdoptWorkspaceInput, AdoptWorkspaceOutput]{
		name: ToolAdoptWorkspace, title: "Adopt existing Git worktree",
		description: "Inspect and record an existing local Git worktree for an investigation. This runs read-only Git commands and writes local metadata; it never fetches, changes refs or files, takes deletion ownership, or contacts GitHub.",
		annotations: localWriteAnnotations(true), supportedBy: supports[WorkspaceAdopter], input: inputSchema[AdoptWorkspaceInput](noSchemaCustomization),
		output: outputSchema[AdoptWorkspaceOutput]("Persisted external-worktree identity without host paths or remote URLs."), handler: s.adoptWorkspace,
	})
	addCatalogTool(s, catalogTool[RunValidationInput, JobReference]{
		name: ToolRunValidation, title: "Run stored validation command",
		description: "Execute one stored shell-free validation command against its base or candidate workspace and persist the run asynchronously. This can modify the workspace or host through the authorized command and requires execute=true.",
		annotations: executionAnnotations(), supportedBy: supports[Operator], input: inputSchema[RunValidationInput](func(schema *schemaBuilder) {
			setEnum(schema, "kind", "base", "candidate")
			setConst(schema, "execute", true)
		}), output: outputSchema[JobReference]("Reference to a newly queued validation execution job."), handler: s.runValidation,
	})
	addCatalogTool(s, catalogTool[RunRepeatedValidationInput, JobReference]{
		name: ToolRunRepeatedValidation, title: "Run repeated validation",
		description: "Execute a stored shell-free validation repeatedly with bounded concurrency, independent per-attempt deadlines, process-tree resource telemetry, cleanup checks, and semantic aggregation. Requires execute=true and never contacts GitHub.",
		annotations: executionAnnotations(), supportedBy: supports[Operator], input: inputSchema[RunRepeatedValidationInput](func(schema *schemaBuilder) {
			setEnum(schema, "target", "base", "candidate", "both")
			setRange(schema, "run_count", 1, 100)
			setDefault(schema, "run_count", 3)
			setRange(schema, "concurrency", 1, 16)
			setDefault(schema, "concurrency", 1)
			setDefault(schema, "sample_interval", "100ms")
			setConst(schema, "execute", true)
		}), output: outputSchema[JobReference]("Reference to a newly queued repeat validation job."), handler: s.runRepeatedValidation,
	})
	addCatalogTool(s, catalogTool[StartInvestigationInput, InvestigationOutput]{
		name: ToolStartInvestigation, title: "Start local investigation",
		description: "Create a local investigation from a commit SHA, or atomically create its initial baseline hypothesis from a stored issue or pull-request number. This does not create a Git worktree or contact GitHub; use " + ToolCreateWorkspace + " separately when filesystem work is authorized.",
		annotations: localWrite, supportedBy: supports[Operator], input: inputSchema[StartInvestigationInput](noSchemaCustomization),
		output: outputSchema[InvestigationOutput]("Newly created local investigation."), handler: s.startInvestigation,
	})
	addCatalogTool(s, catalogTool[RecordHypothesisInput, HypothesisOutput]{
		name: ToolRecordHypothesis, title: "Record investigation hypothesis",
		description: "Persist a structured hypothesis and source references in an existing local investigation. Use this only after the problem is concrete enough to state expected or observed behavior; it performs no network access.",
		annotations: localWrite, supportedBy: supports[Operator], input: inputSchema[RecordHypothesisInput](func(schema *schemaBuilder) {
			setEnum(schema, "category", "bug", "performance", "architecture", "testing", "documentation", "maintenance", "compatibility", "security", "other")
		}), output: outputSchema[HypothesisOutput]("Newly recorded structured hypothesis."), handler: s.recordHypothesis,
	})
	addCatalogTool(s, catalogTool[CheckDuplicatesInput, CheckOutput]{
		name: ToolCheckDuplicates, title: "Find issue and PR duplicates",
		description: "Search the local thread corpus for issues or pull requests that may duplicate one hypothesis or opportunity. This records no evidence and performs no network access; refresh the corpus explicitly if coverage is stale.",
		annotations: readOnly, supportedBy: supports[Operator], input: inputSchema[CheckDuplicatesInput](func(schema *schemaBuilder) {
			setEnum(schema, "target", "hypothesis", "opportunity")
			setRange(schema, "limit", 1, 100)
			setDefault(schema, "limit", 20)
		}), output: outputSchema[CheckOutput]("Evidence-backed duplicate candidates from the local corpus."), handler: s.checkDuplicates,
	})
	addCatalogTool(s, catalogTool[CheckCollisionsInput, CheckOutput]{
		name: ToolFindCompetingWork, title: "Find competing open pull requests",
		description: "Search locally stored open pull requests for semantically or explicitly overlapping work for one hypothesis or opportunity. This does not test Git merge conflicts and performs no network access.",
		annotations: readOnly, supportedBy: supports[Operator], input: inputSchema[CheckCollisionsInput](func(schema *schemaBuilder) {
			setEnum(schema, "target", "hypothesis", "opportunity")
			setRange(schema, "limit", 1, 100)
			setDefault(schema, "limit", 20)
		}), output: outputSchema[CheckOutput]("Evidence-backed competing open pull requests."), handler: s.checkCollisions,
	})
	addCatalogTool(s, catalogTool[PromoteOpportunityInput, OpportunityOutput]{
		name: ToolPromoteOpportunity, title: "Promote hypothesis to opportunity",
		description: "Persist a scoped contribution opportunity from an existing hypothesis, including impact, effort, confidence, dependencies, and source references. This changes local workflow state but never contacts or mutates GitHub.",
		annotations: localWrite, supportedBy: supports[Operator], input: inputSchema[PromoteOpportunityInput](func(schema *schemaBuilder) {
			setRange(schema, "confidence", 0, 1)
		}), output: outputSchema[OpportunityOutput]("Newly promoted local contribution opportunity."), handler: s.promoteOpportunity,
	})
	addCatalogTool(s, catalogTool[DefineValidationInput, ValidationOutput]{
		name: ToolDefineValidation, title: "Define validation command",
		description: "Parse and persist a shell-free validation command for managed workspace IDs belonging to the investigation, with an environment allowlist, timeout, output bound, and optional declared MCP stdio adapter. This does not execute the command; use " + ToolRunValidation + " separately with explicit authorization.",
		annotations: localWrite, supportedBy: supports[Operator], input: inputSchema[DefineValidationInput](func(schema *schemaBuilder) {
			setDefault(schema, "timeout", "30m")
			setRange(schema, "max_output_bytes", 1, 64*1024*1024)
			setDefault(schema, "max_output_bytes", 64*1024)
			setEnum(schema, "protocol", "mcp_stdio")
			configureValidationObservationSchema(schema)
		}), output: outputSchema[ValidationOutput]("Persisted validation definition."), handler: s.defineValidation,
	})
	addCatalogTool(s, catalogTool[PrepareContributionInput, DraftOutput]{
		name: ToolPrepareContribution, title: "Prepare pull request or issue draft",
		description: "Render and persist a pull request or issue draft from stored evidence, supplied changes, or a verified workspace diff; it inspects the managed workspace with non-mutating Git when changes are omitted. Never posts or mutates GitHub.",
		annotations: localWrite, supportedBy: supports[Operator], input: inputSchema[PrepareContributionInput](func(schema *schemaBuilder) {
			setEnum(schema, "kind", "issue", "pull_request")
		}), output: outputSchema[DraftOutput]("Newly rendered and persisted local contribution draft."), handler: s.prepareContribution,
	})
	addCatalogTool(s, catalogTool[ExportManifestInput, ManifestOutput]{
		name: ToolExportManifest, title: "Export contribution evidence manifest",
		description: "Generate and persist a deterministic local evidence manifest from SQLite and an optional managed workspace snapshot. It may run non-mutating Git commands but never contacts GitHub; sync exact GitHub facets separately before export.",
		annotations: localWrite, supportedBy: supports[Operator], input: inputSchema[ExportManifestInput](nil),
		output: outputSchema[ManifestOutput]("Digest-bound contribution evidence statement with explicit completeness gaps."), handler: s.exportManifest,
	})
	addCatalogTool(s, catalogTool[CancelJobInput, GetJobsOutput]{
		name: ToolCancelJob, title: "Cancel durable jobs in one batch",
		description: "Cancel up to 100 durable jobs in order with isolated item outcomes; repeated cancellation is safe.",
		annotations: cancellationAnnotations(), supportedBy: supports[Operator], input: inputSchema[CancelJobInput](func(sc *schemaBuilder) { setArrayBounds(sc, "ids", 1, 100) }),
		output: outputSchema[GetJobsOutput]("Ordered durable job states after cancellation requests."), handler: s.cancelJob,
	})
	s.registerConcernTools()
}

func (s *Server) searchRepositories(ctx context.Context, _ *mcp.CallToolRequest, in SearchRepositoriesInput) (*mcp.CallToolResult, SearchRepositoriesOutput, error) {
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return nil, SearchRepositoriesOutput{}, InvalidArgument("limit", "must be between 1 and 100", map[string]any{"limit": 20})
	}
	if (in.Owner == "") != (in.Repo == "") {
		return nil, SearchRepositoriesOutput{}, InvalidArgument("owner", "owner and repo must be provided together", map[string]any{"owner": "acme", "repo": "rocket"})
	}
	if in.Sort != "" && in.Sort != "relevance" && in.Sort != "updated" {
		return nil, SearchRepositoriesOutput{}, InvalidArgument("sort", "must be relevance or updated", map[string]any{"sort": "updated"})
	}
	out, err := s.reader.SearchRepositories(ctx, in)
	return nil, out, err
}

func (s *Server) searchThreads(ctx context.Context, _ *mcp.CallToolRequest, in SearchThreadsInput) (*mcp.CallToolResult, SearchOutput, error) {
	if in.Query == "" {
		return nil, SearchOutput{}, InvalidArgument("query", "is required", map[string]any{"query": "music"})
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return nil, SearchOutput{}, InvalidArgument("limit", "must be between 1 and 100", map[string]any{"limit": 20})
	}
	if (in.Owner == "") != (in.Repo == "") {
		return nil, SearchOutput{}, InvalidArgument("owner", "owner and repo must be provided together", map[string]any{"owner": "acme", "repo": "rocket"})
	}
	if in.Kind != "" && in.Kind != "issue" && in.Kind != "pull_request" {
		return nil, SearchOutput{}, InvalidArgument("kind", "must be issue or pull_request", map[string]any{"kind": "issue"})
	}
	if in.State != "" && in.State != "open" && in.State != "closed" {
		return nil, SearchOutput{}, InvalidArgument("state", "must be open or closed", map[string]any{"state": "open"})
	}
	if in.Sort != "" && in.Sort != "relevance" && in.Sort != "updated" {
		return nil, SearchOutput{}, InvalidArgument("sort", "must be relevance or updated", map[string]any{"sort": "updated"})
	}
	if in.StateReason != "" && in.StateReason != "completed" && in.StateReason != "not_planned" {
		return nil, SearchOutput{}, InvalidArgument("state_reason", "must be completed or not_planned", map[string]any{"state_reason": "completed"})
	}
	searchIn := SearchInput{
		Query: in.Query, Owner: in.Owner, Repo: in.Repo, Kind: in.Kind, State: in.State,
		StateReason: in.StateReason, Merged: in.Merged, Author: in.Author, Association: in.Association,
		Assignee: in.Assignee, Labels: in.Labels, UpdatedAfter: in.UpdatedAfter, Limit: in.Limit, Cursor: in.Cursor,
		Sort: in.Sort,
	}
	out, err := s.reader.Search(ctx, searchIn)
	return nil, out, err
}

func (s *Server) getRepositoryDossier(ctx context.Context, _ *mcp.CallToolRequest, in GetRepositoryDossierInput) (*mcp.CallToolResult, DossierOutput, error) {
	if err := validateRepo(RepoInput(in)); err != nil {
		return nil, DossierOutput{}, err
	}
	out, err := s.reader.Dossier(ctx, RepoInput(in))
	return nil, out, err
}

func (s *Server) explainMatch(ctx context.Context, _ *mcp.CallToolRequest, in ExplainMatchInput) (*mcp.CallToolResult, ExplainMatchOutput, error) {
	if err := validateRepo(RepoInput{Owner: in.Owner, Repo: in.Repo}); err != nil {
		return nil, ExplainMatchOutput{}, err
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return nil, ExplainMatchOutput{}, errors.New("limit must be between 1 and 100")
	}
	if in.Kind != "" && in.Kind != "repo" && in.Kind != "issue" && in.Kind != "pull_request" && in.Kind != "code" {
		return nil, ExplainMatchOutput{}, errors.New("kind must be repo, issue, pull_request, or code")
	}
	if (in.Kind == "issue" || in.Kind == "pull_request") && in.Number < 1 {
		return nil, ExplainMatchOutput{}, errors.New("number is required for issue and pull_request matches")
	}
	if in.Kind == "code" && (strings.TrimSpace(in.Path) == "" || strings.TrimSpace(in.Commit) == "") {
		return nil, ExplainMatchOutput{}, errors.New("path and commit are required for code matches")
	}
	if in.Kind == "repo" && (in.Number != 0 || in.Path != "" || in.Commit != "") || (in.Kind == "issue" || in.Kind == "pull_request") && (in.Path != "" || in.Commit != "") || in.Kind == "code" && in.Number != 0 {
		return nil, ExplainMatchOutput{}, errors.New("identity fields do not match kind; use number for threads, path and commit for code, and neither for repositories")
	}
	out, err := s.reader.ExplainMatch(ctx, in)
	return nil, out, err
}

func (s *Server) getJob(ctx context.Context, _ *mcp.CallToolRequest, in GetJobInput) (*mcp.CallToolResult, GetJobOutput, error) {
	id, err := normalizeID("id", in.ID)
	if err != nil {
		return nil, GetJobOutput{}, err
	}
	in.ID = id
	out, err := s.reader.GetJob(ctx, in)
	return nil, out, err
}

func (s *Server) buildRepositoryDossier(ctx context.Context, _ *mcp.CallToolRequest, in BuildRepositoryDossierInput) (*mcp.CallToolResult, JobReference, error) {
	if err := validateRepo(RepoInput(in)); err != nil {
		return nil, JobReference{}, err
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, JobReference{}, errors.New("dossier build is not available")
	}
	out, err := operator.BuildRepositoryDossier(ctx, in)
	return nil, out, err
}

func (s *Server) adoptWorkspace(ctx context.Context, _ *mcp.CallToolRequest, in AdoptWorkspaceInput) (*mcp.CallToolResult, AdoptWorkspaceOutput, error) {
	var err error
	if in.InvestigationID, err = normalizeID("investigation_id", in.InvestigationID); err != nil {
		return nil, AdoptWorkspaceOutput{}, err
	}
	in.Path, in.BaseRef, in.Name = strings.TrimSpace(in.Path), strings.TrimSpace(in.BaseRef), strings.TrimSpace(in.Name)
	if in.Path == "" || in.BaseRef == "" {
		return nil, AdoptWorkspaceOutput{}, errors.New("path and base_ref are required")
	}
	operator, ok := s.reader.(WorkspaceAdopter)
	if !ok {
		return nil, AdoptWorkspaceOutput{}, errors.New("workspace adoption is not available")
	}
	out, err := operator.AdoptWorkspace(ctx, in)
	return nil, out, err
}

func (s *Server) createWorkspace(ctx context.Context, _ *mcp.CallToolRequest, in CreateWorkspaceInput) (*mcp.CallToolResult, JobReference, error) {
	if _, err := normalizeID("investigation_id", in.InvestigationID); err != nil {
		return nil, JobReference{}, err
	}
	in.Remote = strings.TrimSpace(in.Remote)
	in.BaseRef = strings.TrimSpace(in.BaseRef)
	in.CandidateRef = strings.TrimSpace(in.CandidateRef)
	in.Name = strings.TrimSpace(in.Name)
	operator, ok := s.reader.(WorkspaceCreator)
	if !ok {
		return nil, JobReference{}, errors.New("workspace creation is not available")
	}
	out, err := operator.CreateWorkspace(ctx, in)
	return nil, out, err
}

func (s *Server) startInvestigation(ctx context.Context, _ *mcp.CallToolRequest, in StartInvestigationInput) (*mcp.CallToolResult, InvestigationOutput, error) {
	if err := validateRepo(RepoInput{Owner: in.Owner, Repo: in.Repo}); err != nil {
		return nil, InvestigationOutput{}, err
	}
	if in.Number > 0 {
		if in.CommitSHA != "" || in.Lens != "" {
			return nil, InvestigationOutput{}, InvalidArgument("number", "cannot be combined with commit_sha or lens", map[string]any{"owner": in.Owner, "repo": in.Repo, "kind": "issue", "number": in.Number})
		}
		if in.Kind != "" && in.Kind != "issue" && in.Kind != "pull_request" {
			return nil, InvestigationOutput{}, InvalidArgument("kind", "must be issue or pull_request", map[string]any{"kind": "issue"})
		}
	} else if strings.TrimSpace(in.CommitSHA) == "" {
		return nil, InvestigationOutput{}, InvalidArgument("commit_sha", "provide commit_sha or a positive stored thread number", map[string]any{"commit_sha": "<sha>"})
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, InvestigationOutput{}, errors.New("investigations are not available")
	}
	out, err := operator.StartInvestigation(ctx, in)
	return nil, out, err
}

func (s *Server) recordHypothesis(ctx context.Context, _ *mcp.CallToolRequest, in RecordHypothesisInput) (*mcp.CallToolResult, HypothesisOutput, error) {
	if _, err := normalizeID("investigation_id", in.InvestigationID); err != nil {
		return nil, HypothesisOutput{}, err
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	in.Category = strings.TrimSpace(in.Category)
	if in.Title == "" || in.Description == "" || in.Category == "" {
		return nil, HypothesisOutput{}, errors.New("title, description, and category are required")
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, HypothesisOutput{}, errors.New("hypothesis recording is not available")
	}
	out, err := operator.RecordHypothesis(ctx, in)
	return nil, out, err
}

func (s *Server) checkDuplicates(ctx context.Context, _ *mcp.CallToolRequest, in CheckDuplicatesInput) (*mcp.CallToolResult, CheckOutput, error) {
	if err := validateCheckInput(&in); err != nil {
		return nil, CheckOutput{}, err
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return nil, CheckOutput{}, errors.New("limit must be between 1 and 100")
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, CheckOutput{}, errors.New("duplicate checks are not available")
	}
	out, err := operator.CheckDuplicates(ctx, in)
	return nil, out, err
}

func (s *Server) checkCollisions(ctx context.Context, _ *mcp.CallToolRequest, in CheckCollisionsInput) (*mcp.CallToolResult, CheckOutput, error) {
	if err := validateCheckInput((*CheckDuplicatesInput)(&in)); err != nil {
		return nil, CheckOutput{}, err
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return nil, CheckOutput{}, errors.New("limit must be between 1 and 100")
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, CheckOutput{}, errors.New("collision checks are not available")
	}
	out, err := operator.CheckCollisions(ctx, in)
	return nil, out, err
}

func validateCheckInput(in *CheckDuplicatesInput) error {
	if _, err := normalizeID("id", in.ID); err != nil {
		return err
	}
	in.Target = strings.ToLower(strings.TrimSpace(in.Target))
	if in.Target != "hypothesis" && in.Target != "opportunity" {
		return errors.New("target must be hypothesis or opportunity")
	}
	return nil
}

func (s *Server) promoteOpportunity(ctx context.Context, _ *mcp.CallToolRequest, in PromoteOpportunityInput) (*mcp.CallToolResult, OpportunityOutput, error) {
	if _, err := normalizeID("hypothesis_id", in.HypothesisID); err != nil {
		return nil, OpportunityOutput{}, err
	}
	if strings.TrimSpace(in.ProblemStatement) == "" || strings.TrimSpace(in.Scope) == "" || strings.TrimSpace(in.Impact) == "" || strings.TrimSpace(in.ExpectedEffort) == "" {
		return nil, OpportunityOutput{}, errors.New("problem_statement, scope, impact, and expected_effort are required")
	}
	if in.Confidence < 0 || in.Confidence > 1 {
		return nil, OpportunityOutput{}, errors.New("confidence must be between 0.0 and 1.0")
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, OpportunityOutput{}, errors.New("opportunity promotion is not available")
	}
	out, err := operator.PromoteOpportunity(ctx, in)
	return nil, out, err
}

func (s *Server) cancelJob(ctx context.Context, _ *mcp.CallToolRequest, in CancelJobInput) (*mcp.CallToolResult, GetJobsOutput, error) {
	if len(in.IDs) < 1 || len(in.IDs) > 100 {
		return nil, GetJobsOutput{}, errors.New("ids must contain 1 to 100 items")
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, GetJobsOutput{}, errors.New("job cancellation is not available")
	}
	out, err := operator.CancelJobs(ctx, in)
	return nil, out, err
}

func parsePositiveNumber(parts []string, idx int) (int, error) {
	if len(parts) <= idx {
		return 0, errors.New("missing path segment")
	}
	n, err := strconv.Atoi(parts[idx])
	if err != nil || n < 1 {
		return 0, errors.New("invalid positive number")
	}
	return n, nil
}
