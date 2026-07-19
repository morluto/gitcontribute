package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// V1 read-only tool inputs and outputs.

// SearchRepositoriesInput describes an offline repository search page.
type SearchRepositoriesInput struct {
	Query  string `json:"query,omitempty" jsonschema:"Full-text query over repository owner, name, and description"`
	Owner  string `json:"owner,omitempty" jsonschema:"Optional repository owner"`
	Repo   string `json:"repo,omitempty" jsonschema:"Optional repository name"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
	Cursor string `json:"cursor,omitempty" jsonschema:"Opaque cursor returned by the previous page"`
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
	Query        string   `json:"query" jsonschema:"Full-text query over thread titles and bodies"`
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
	Query          string                `json:"query"`
	Kind           string                `json:"kind"`
	Owner          string                `json:"owner"`
	Repo           string                `json:"repo"`
	Number         int                   `json:"number,omitempty"`
	Path           string                `json:"path,omitempty"`
	Commit         string                `json:"commit,omitempty"`
	State          string                `json:"state,omitempty"`
	Title          string                `json:"title"`
	Snippet        string                `json:"snippet,omitempty"`
	MatchedFields  []string              `json:"matched_fields,omitempty"`
	Score          float64               `json:"score"`
	Reason         string                `json:"reason"`
	SourceRevision string                `json:"source_revision,omitempty"`
	Facets         []FacetCoverageOutput `json:"facets,omitempty"`
	AsOf           string                `json:"as_of,omitempty"`
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
	Remote          string `json:"remote" jsonschema:"Git remote URL to clone"`
	BaseRef         string `json:"base_ref" jsonschema:"Base ref to resolve"`
	CandidateRef    string `json:"candidate_ref" jsonschema:"Candidate ref to resolve"`
	Name            string `json:"name" jsonschema:"Workspace name"`
}

// RunValidationInput selects a validation definition and explicitly authorizes execution.
type RunValidationInput struct {
	ID      string `json:"id" jsonschema:"Validation definition ID"`
	Kind    string `json:"kind" jsonschema:"Run kind: base or candidate"`
	Execute bool   `json:"execute" jsonschema:"Must be true to authorize host execution"`
}

// StartInvestigationInput creates a local investigation for a repository revision.
type StartInvestigationInput struct {
	Owner     string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo      string `json:"repo" jsonschema:"GitHub repository name"`
	CommitSHA string `json:"commit_sha,omitempty" jsonschema:"Optional commit SHA"`
	Lens      string `json:"lens,omitempty" jsonschema:"Optional lens name"`
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

// DefineValidationInput records a bounded validation command without executing it.
type DefineValidationInput struct {
	InvestigationID string   `json:"investigation_id" jsonschema:"Investigation ID"`
	Kind            string   `json:"kind" jsonschema:"Validation kind"`
	Command         string   `json:"command" jsonschema:"Shell-free command to execute"`
	WorkingDir      string   `json:"working_dir" jsonschema:"Working directory"`
	BaseWorkingDir  string   `json:"base_working_dir,omitempty" jsonschema:"Base workspace directory"`
	CandidateDir    string   `json:"candidate_dir,omitempty" jsonschema:"Candidate workspace directory"`
	Env             []string `json:"env,omitempty" jsonschema:"Allowed environment variable names"`
	Timeout         string   `json:"timeout,omitempty" jsonschema:"Positive Go duration; defaults to 30m"`
	MaxOutputBytes  int64    `json:"max_output_bytes,omitempty" jsonschema:"Maximum captured bytes per output stream; defaults to 65536"`
}

// ValidationOutput is the stable MCP representation of a validation definition.
type ValidationOutput struct {
	ID              string   `json:"id"`
	InvestigationID string   `json:"investigation_id"`
	Kind            string   `json:"kind"`
	Command         []string `json:"command"`
	WorkingDir      string   `json:"working_dir"`
	BaseWorkingDir  string   `json:"base_working_dir,omitempty"`
	CandidateDir    string   `json:"candidate_dir,omitempty"`
	Env             []string `json:"environment_allowlist,omitempty"`
	Timeout         string   `json:"timeout,omitempty"`
	MaxOutputBytes  int64    `json:"max_output_bytes,omitempty"`
	CreatedAt       string   `json:"created_at"`
}

// PrepareContributionInput renders a local issue or pull-request draft.
type PrepareContributionInput struct {
	OpportunityID string `json:"opportunity_id" jsonschema:"Opportunity ID"`
	Kind          string `json:"kind" jsonschema:"Contribution kind: issue or pull_request"`
	WorkspaceID   string `json:"workspace_id,omitempty" jsonschema:"Workspace ID for pull_request drafts"`
	Approach      string `json:"approach,omitempty" jsonschema:"Approach summary for pull requests"`
	Changes       string `json:"changes,omitempty" jsonschema:"Changes summary for pull requests"`
	Compatibility string `json:"compatibility,omitempty" jsonschema:"Compatibility notes for pull requests"`
	Limitations   string `json:"limitations,omitempty" jsonschema:"Limitations for pull requests"`
	LinkedIssue   string `json:"linked_issue,omitempty" jsonschema:"Linked issue for pull requests"`
	Guidance      string `json:"guidance,omitempty" jsonschema:"Optional guidance to include"`
	Success       string `json:"success,omitempty" jsonschema:"Success criteria for issue drafts"`
}

// DraftOutput contains a rendered contribution draft.
type DraftOutput struct {
	OpportunityID string `json:"opportunity_id"`
	Kind          string `json:"kind"`
	Title         string `json:"title"`
	Body          string `json:"body"`
	RenderedAt    string `json:"rendered_at"`
}

// CancelJobInput selects durable jobs for bounded, persisted cancellation.
type CancelJobInput struct {
	IDs []string `json:"ids" jsonschema:"One to 100 durable job IDs"`
}

func (s *Server) registerV1() {
	readOnly := readOnlyAnnotations()
	localWrite := localWriteAnnotations(false)
	addCatalogTool(s.server, catalogTool[SearchRepositoriesInput, SearchRepositoriesOutput]{
		name: ToolSearchRepositories, title: "Search stored repositories",
		description: "Search local repository owner, name, and description fields, or list a specific repository when owner and repo are supplied together. Results are paginated and this tool never contacts GitHub.",
		annotations: readOnly, input: inputSchema[SearchRepositoriesInput](func(schema *jsonschema.Schema) {
			setRange(schema, "limit", 1, 100)
			setDefault(schema, "limit", 20)
		}), output: outputSchema[SearchRepositoriesOutput]("One page of stored repository matches."), handler: s.searchRepositories,
	})
	addCatalogTool(s.server, catalogTool[SearchThreadsInput, SearchOutput]{
		name: ToolSearchThreads, title: "Search stored issues and pull requests",
		description: "Search locally stored issue and pull-request titles and bodies, optionally restricted to one repository and thread kind. Use the returned cursor for the next page; this tool never contacts GitHub.",
		annotations: readOnly, input: inputSchema[SearchThreadsInput](func(schema *jsonschema.Schema) {
			setEnum(schema, "kind", "issue", "pull_request")
			setEnum(schema, "state", "open", "closed")
			setEnum(schema, "state_reason", "completed", "not_planned")
			setRange(schema, "limit", 1, 100)
			setDefault(schema, "limit", 20)
		}), output: outputSchema[SearchOutput]("One page of stored issue and pull-request matches."), handler: s.searchThreads,
	})
	addCatalogTool(s.server, catalogTool[GetRepositoryDossierInput, DossierOutput]{
		name: ToolGetRepositoryDossier, title: "Get repository dossier",
		description: "Read the latest persisted source-backed dossier for one repository. Use " + ToolBuildRepositoryDossier + " only when the local dossier must be regenerated; this read never performs that write.",
		annotations: readOnly, input: inputSchema[GetRepositoryDossierInput](noSchemaCustomization),
		output: outputSchema[DossierOutput]("Persisted source-backed repository dossier."), handler: s.getRepositoryDossier,
	})
	addCatalogTool(s.server, catalogTool[ExplainMatchInput, ExplainMatchOutput]{
		name: ToolExplainMatch, title: "Explain a stored search match",
		description: "Explain why one repository, thread, or code result matched a prior local query, including score signals, source revision, and facet coverage. Supply the identity fields for the selected match; this tool is offline.",
		annotations: readOnly, input: inputSchema[ExplainMatchInput](func(schema *jsonschema.Schema) {
			setEnum(schema, "kind", "repo", "issue", "pull_request", "code")
			setMinimum(schema, "number", 1)
			setRange(schema, "limit", 1, 100)
			setDefault(schema, "limit", 20)
		}), output: outputSchema[ExplainMatchOutput]("Stored facts and score signals explaining one search match."), handler: s.explainMatch,
	})
	addCatalogTool(s.server, catalogTool[BuildRepositoryDossierInput, JobReference]{
		name: ToolBuildRepositoryDossier, title: "Build repository dossier",
		description: "Start an asynchronous local job that rebuilds and persists a source-backed dossier from the existing corpus. It performs no network access; use " + ToolGetRepositoryDossier + " after the job succeeds.",
		annotations: localWriteAnnotations(true), input: inputSchema[BuildRepositoryDossierInput](noSchemaCustomization),
		output: outputSchema[JobReference]("Reference to a newly queued dossier build job."), handler: s.buildRepositoryDossier,
	})
	addCatalogTool(s.server, catalogTool[CreateWorkspaceInput, JobReference]{
		name: ToolCreateWorkspace, title: "Create managed Git workspace",
		description: "Start an asynchronous job that clones the specified remote and creates a managed worktree for an investigation. This performs network reads, Git process execution, filesystem writes, and local metadata writes, but never mutates GitHub.",
		annotations: networkReadAnnotations(), input: inputSchema[CreateWorkspaceInput](noSchemaCustomization),
		output: outputSchema[JobReference]("Reference to a newly queued workspace creation job."), handler: s.createWorkspace,
	})
	addCatalogTool(s.server, catalogTool[RunValidationInput, JobReference]{
		name: ToolRunValidation, title: "Run stored validation command",
		description: "Execute one stored shell-free validation command against its base or candidate workspace and persist the run asynchronously. This can modify the workspace or host through the authorized command and requires execute=true.",
		annotations: executionAnnotations(), input: inputSchema[RunValidationInput](func(schema *jsonschema.Schema) {
			setEnum(schema, "kind", "base", "candidate")
			setConst(schema, "execute", true)
		}), output: outputSchema[JobReference]("Reference to a newly queued validation execution job."), handler: s.runValidation,
	})
	addCatalogTool(s.server, catalogTool[StartInvestigationInput, InvestigationOutput]{
		name: ToolStartInvestigation, title: "Start local investigation",
		description: "Create and persist a local investigation for one stored repository revision. This does not create a Git worktree or contact GitHub; use " + ToolCreateWorkspace + " separately when filesystem work is authorized.",
		annotations: localWrite, input: inputSchema[StartInvestigationInput](noSchemaCustomization),
		output: outputSchema[InvestigationOutput]("Newly created local investigation."), handler: s.startInvestigation,
	})
	addCatalogTool(s.server, catalogTool[RecordHypothesisInput, HypothesisOutput]{
		name: ToolRecordHypothesis, title: "Record investigation hypothesis",
		description: "Persist a structured hypothesis and source references in an existing local investigation. Use this only after the problem is concrete enough to state expected or observed behavior; it performs no network access.",
		annotations: localWrite, input: inputSchema[RecordHypothesisInput](func(schema *jsonschema.Schema) {
			setEnum(schema, "category", "bug", "performance", "architecture", "testing", "documentation", "maintenance", "compatibility", "security", "other")
		}), output: outputSchema[HypothesisOutput]("Newly recorded structured hypothesis."), handler: s.recordHypothesis,
	})
	addCatalogTool(s.server, catalogTool[CheckDuplicatesInput, CheckOutput]{
		name: ToolCheckDuplicates, title: "Find issue and PR duplicates",
		description: "Search the local thread corpus for issues or pull requests that may duplicate one hypothesis or opportunity. This records no evidence and performs no network access; refresh the corpus explicitly if coverage is stale.",
		annotations: readOnly, input: inputSchema[CheckDuplicatesInput](func(schema *jsonschema.Schema) {
			setEnum(schema, "target", "hypothesis", "opportunity")
			setRange(schema, "limit", 1, 100)
			setDefault(schema, "limit", 20)
		}), output: outputSchema[CheckOutput]("Evidence-backed duplicate candidates from the local corpus."), handler: s.checkDuplicates,
	})
	addCatalogTool(s.server, catalogTool[CheckCollisionsInput, CheckOutput]{
		name: ToolFindCompetingWork, title: "Find competing open pull requests",
		description: "Search locally stored open pull requests for semantically or explicitly overlapping work for one hypothesis or opportunity. This does not test Git merge conflicts and performs no network access.",
		annotations: readOnly, input: inputSchema[CheckCollisionsInput](func(schema *jsonschema.Schema) {
			setEnum(schema, "target", "hypothesis", "opportunity")
			setRange(schema, "limit", 1, 100)
			setDefault(schema, "limit", 20)
		}), output: outputSchema[CheckOutput]("Evidence-backed competing open pull requests."), handler: s.checkCollisions,
	})
	addCatalogTool(s.server, catalogTool[PromoteOpportunityInput, OpportunityOutput]{
		name: ToolPromoteOpportunity, title: "Promote hypothesis to opportunity",
		description: "Persist a scoped contribution opportunity from an existing hypothesis, including impact, effort, confidence, dependencies, and source references. This changes local workflow state but never contacts or mutates GitHub.",
		annotations: localWrite, input: inputSchema[PromoteOpportunityInput](func(schema *jsonschema.Schema) {
			setRange(schema, "confidence", 0, 1)
		}), output: outputSchema[OpportunityOutput]("Newly promoted local contribution opportunity."), handler: s.promoteOpportunity,
	})
	addCatalogTool(s.server, catalogTool[DefineValidationInput, ValidationOutput]{
		name: ToolDefineValidation, title: "Define validation command",
		description: "Parse and persist a shell-free validation command, working directory, environment allowlist, timeout, and output bound for an investigation. This does not execute the command; use " + ToolRunValidation + " separately with explicit authorization.",
		annotations: localWrite, input: inputSchema[DefineValidationInput](func(schema *jsonschema.Schema) {
			setDefault(schema, "timeout", "30m")
			setRange(schema, "max_output_bytes", 1, 64*1024*1024)
			setDefault(schema, "max_output_bytes", 64*1024)
		}), output: outputSchema[ValidationOutput]("Persisted validation definition."), handler: s.defineValidation,
	})
	addCatalogTool(s.server, catalogTool[PrepareContributionInput, DraftOutput]{
		name: ToolPrepareContribution, title: "Prepare local contribution draft",
		description: "Render and persist a local issue or pull-request draft from an opportunity and supplied evidence summaries. Pull-request drafts require explicit workspace_id, approach, and changes; this tool never inspects a workspace, runs Git, posts, or mutates GitHub.",
		annotations: localWrite, input: inputSchema[PrepareContributionInput](func(schema *jsonschema.Schema) {
			setEnum(schema, "kind", "issue", "pull_request")
		}), output: outputSchema[DraftOutput]("Newly rendered and persisted local contribution draft."), handler: s.prepareContribution,
	})
	addCatalogTool(s.server, catalogTool[CancelJobInput, GetJobsOutput]{
		name: ToolCancelJob, title: "Cancel durable jobs in one batch",
		description: "Cancel up to 100 durable jobs in order with isolated item outcomes; repeated cancellation is safe.",
		annotations: cancellationAnnotations(), input: inputSchema[CancelJobInput](func(sc *jsonschema.Schema) { setArrayBounds(sc, "ids", 1, 100) }),
		output: outputSchema[GetJobsOutput]("Ordered durable job states after cancellation requests."), handler: s.cancelJob,
	})

	s.registerV1ResourceTemplates()
}

func (s *Server) searchRepositories(ctx context.Context, _ *mcp.CallToolRequest, in SearchRepositoriesInput) (*mcp.CallToolResult, SearchRepositoriesOutput, error) {
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return nil, SearchRepositoriesOutput{}, errors.New("limit must be between 1 and 100")
	}
	if (in.Owner == "") != (in.Repo == "") {
		return nil, SearchRepositoriesOutput{}, InvalidArgument("owner", "owner and repo must be provided together", map[string]any{"owner": "acme", "repo": "rocket"})
	}
	out, err := s.reader.SearchRepositories(ctx, in)
	return nil, out, err
}

func (s *Server) searchThreads(ctx context.Context, _ *mcp.CallToolRequest, in SearchThreadsInput) (*mcp.CallToolResult, SearchOutput, error) {
	if in.Query == "" {
		return nil, SearchOutput{}, errors.New("query is required")
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return nil, SearchOutput{}, errors.New("limit must be between 1 and 100")
	}
	if (in.Owner == "") != (in.Repo == "") {
		return nil, SearchOutput{}, InvalidArgument("owner", "owner and repo must be provided together", map[string]any{"owner": "acme", "repo": "rocket"})
	}
	if in.Kind != "" && in.Kind != "issue" && in.Kind != "pull_request" {
		return nil, SearchOutput{}, errors.New("kind must be issue or pull_request")
	}
	if in.State != "" && in.State != "open" && in.State != "closed" {
		return nil, SearchOutput{}, errors.New("state must be open or closed")
	}
	if in.StateReason != "" && in.StateReason != "completed" && in.StateReason != "not_planned" {
		return nil, SearchOutput{}, errors.New("state_reason must be completed or not_planned")
	}
	searchIn := SearchInput{
		Query: in.Query, Owner: in.Owner, Repo: in.Repo, Kind: in.Kind, State: in.State,
		StateReason: in.StateReason, Merged: in.Merged, Author: in.Author, Association: in.Association,
		Assignee: in.Assignee, Labels: in.Labels, UpdatedAfter: in.UpdatedAfter, Limit: in.Limit, Cursor: in.Cursor,
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

func (s *Server) createWorkspace(ctx context.Context, _ *mcp.CallToolRequest, in CreateWorkspaceInput) (*mcp.CallToolResult, JobReference, error) {
	if _, err := normalizeID("investigation_id", in.InvestigationID); err != nil {
		return nil, JobReference{}, err
	}
	in.Remote = strings.TrimSpace(in.Remote)
	if in.Remote == "" {
		return nil, JobReference{}, errors.New("remote is required")
	}
	in.BaseRef = strings.TrimSpace(in.BaseRef)
	in.CandidateRef = strings.TrimSpace(in.CandidateRef)
	in.Name = strings.TrimSpace(in.Name)
	if in.BaseRef == "" || in.CandidateRef == "" || in.Name == "" {
		return nil, JobReference{}, errors.New("base_ref, candidate_ref, and name are required")
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, JobReference{}, errors.New("workspace creation is not available")
	}
	out, err := operator.CreateWorkspace(ctx, in)
	return nil, out, err
}

func (s *Server) runValidation(ctx context.Context, _ *mcp.CallToolRequest, in RunValidationInput) (*mcp.CallToolResult, JobReference, error) {
	id, err := normalizeID("id", in.ID)
	if err != nil {
		return nil, JobReference{}, err
	}
	in.ID = id
	if in.Kind != "base" && in.Kind != "candidate" {
		return nil, JobReference{}, errors.New("kind must be base or candidate")
	}
	if !in.Execute {
		return nil, JobReference{}, errors.New("execute must be true to authorize host command execution")
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, JobReference{}, errors.New("validation is not available")
	}
	out, err := operator.RunValidation(ctx, in)
	return nil, out, err
}

func (s *Server) startInvestigation(ctx context.Context, _ *mcp.CallToolRequest, in StartInvestigationInput) (*mcp.CallToolResult, InvestigationOutput, error) {
	if err := validateRepo(RepoInput{Owner: in.Owner, Repo: in.Repo}); err != nil {
		return nil, InvestigationOutput{}, err
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

func (s *Server) defineValidation(ctx context.Context, _ *mcp.CallToolRequest, in DefineValidationInput) (*mcp.CallToolResult, ValidationOutput, error) {
	if _, err := normalizeID("investigation_id", in.InvestigationID); err != nil {
		return nil, ValidationOutput{}, err
	}
	in.Kind = strings.TrimSpace(in.Kind)
	in.Command = strings.TrimSpace(in.Command)
	in.WorkingDir = strings.TrimSpace(in.WorkingDir)
	if in.Kind == "" || in.Command == "" || in.WorkingDir == "" {
		return nil, ValidationOutput{}, errors.New("investigation_id, kind, command, and working_dir are required")
	}
	if in.Timeout != "" {
		if _, err := time.ParseDuration(in.Timeout); err != nil {
			return nil, ValidationOutput{}, fmt.Errorf("invalid timeout duration: %w", err)
		}
	}
	if in.MaxOutputBytes < 0 {
		return nil, ValidationOutput{}, errors.New("max_output_bytes cannot be negative")
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, ValidationOutput{}, errors.New("validation definition is not available")
	}
	out, err := operator.DefineValidation(ctx, in)
	return nil, out, err
}

func (s *Server) prepareContribution(ctx context.Context, _ *mcp.CallToolRequest, in PrepareContributionInput) (*mcp.CallToolResult, DraftOutput, error) {
	if _, err := normalizeID("opportunity_id", in.OpportunityID); err != nil {
		return nil, DraftOutput{}, err
	}
	in.Kind = strings.ToLower(strings.TrimSpace(in.Kind))
	if in.Kind != "issue" && in.Kind != "pull_request" {
		return nil, DraftOutput{}, errors.New("kind must be issue or pull_request")
	}
	if in.Kind == "pull_request" && strings.TrimSpace(in.WorkspaceID) == "" {
		return nil, DraftOutput{}, errors.New("workspace_id is required for pull_request drafts")
	}
	if in.Kind == "pull_request" && strings.TrimSpace(in.Approach) == "" {
		return nil, DraftOutput{}, errors.New("approach is required for pull_request drafts")
	}
	if in.Kind == "pull_request" && strings.TrimSpace(in.Changes) == "" {
		return nil, DraftOutput{}, errors.New("changes is required for pull_request drafts; inspect the workspace explicitly before preparing the draft")
	}
	if in.Kind == "issue" && (in.WorkspaceID != "" || in.Approach != "" || in.Changes != "" || in.Compatibility != "" || in.Limitations != "" || in.LinkedIssue != "") {
		return nil, DraftOutput{}, errors.New("pull-request-only fields are not accepted for issue drafts")
	}
	if in.Kind == "pull_request" && in.Success != "" {
		return nil, DraftOutput{}, errors.New("success is only accepted for issue drafts")
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, DraftOutput{}, errors.New("contribution preparation is not available")
	}
	out, err := operator.PrepareContribution(ctx, in)
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
