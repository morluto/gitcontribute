package mcpcontract

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

// JobArtifactReference identifies a bounded durable result without exposing the
// job executor's stored request or result representation.
type JobArtifactReference struct {
	Kind                string               `json:"kind" jsonschema:"Artifact kind owned by GitContribute"`
	ID                  string               `json:"id,omitempty" jsonschema:"Stable artifact identifier when one exists"`
	URI                 string               `json:"uri,omitempty" jsonschema:"MCP resource URI when the artifact is readable as a resource"`
	Count               *NonNegativeInt      `json:"count,omitempty" jsonschema:"Known number of affected objects for a bounded collection, including zero"`
	References          []string             `json:"references,omitempty" jsonschema:"Bounded exact repository, thread, or pull-request references produced by the job"`
	ReferencesTruncated bool                 `json:"references_truncated,omitempty" jsonschema:"Whether more exact references exist than this bounded response includes"`
	Failures            []JobArtifactFailure `json:"failures,omitempty" jsonschema:"Bounded per-reference outcomes that require retry or recovery"`
}

// JobArtifactFailure preserves one actionable item-level outcome without
// exposing an executor's arbitrary stored result representation.
type JobArtifactFailure struct {
	Reference    string          `json:"reference"`
	Status       BatchItemStatus `json:"status"`
	Reason       string          `json:"reason,omitempty"`
	Message      string          `json:"message,omitempty"`
	RetryAfterMS NonNegativeInt  `json:"retry_after_ms,omitempty"`
}

// JobFollowUp points to the typed read plane for a job's durable result.
type JobFollowUp struct {
	Tool        string `json:"tool,omitempty" jsonschema:"Outcome-oriented tool to use next"`
	ResourceURI string `json:"resource_uri,omitempty" jsonschema:"MCP resource URI to read next"`
	Reason      string `json:"reason" jsonschema:"Why this follow-up is appropriate"`
}

// GetJobOutput reports durable state and structured progress for a job. Stored
// executor request and result blobs are intentionally not model-visible.
type GetJobOutput struct {
	ID                    string                 `json:"id"`
	Kind                  string                 `json:"kind" jsonschema:"Durable job kind"`
	Status                JobStatus              `json:"-"`
	ExecutionState        JobExecutionState      `json:"execution_state"`
	Outcome               JobOutcome             `json:"outcome,omitempty"`
	Summary               string                 `json:"summary"`
	Artifacts             []JobArtifactReference `json:"artifacts,omitempty"`
	FollowUp              *JobFollowUp           `json:"follow_up,omitempty"`
	Error                 string                 `json:"error,omitempty"`
	Phase                 string                 `json:"phase,omitempty" jsonschema:"Current bounded workflow phase"`
	CompletedItems        NonNegativeInt         `json:"completed_items"`
	TotalItems            NonNegativeInt         `json:"total_items"`
	ProgressPercent       ProgressPercent        `json:"progress_percent"`
	RetryAfterMS          NonNegativeInt         `json:"retry_after_ms,omitempty"`
	CreatedAt             string                 `json:"created_at"`
	StartedAt             string                 `json:"started_at,omitempty"`
	CompletedAt           string                 `json:"completed_at,omitempty"`
	CancelledAt           string                 `json:"cancelled_at,omitempty"`
	CancellationRequested bool                   `json:"cancellation_requested"`
}

// ThreadByNumberInput identifies a stored issue or pull request by number.
type ThreadByNumberInput struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Number int    `json:"number"`
}

// JobReference is returned by long-running tools that submit durable jobs.
type JobReference struct {
	ID          string         `json:"id"`
	Ref         string         `json:"ref"`
	Kind        string         `json:"kind"`
	Status      JobStatus      `json:"status"`
	Message     string         `json:"message"`
	PollAfterMS NonNegativeInt `json:"poll_after_ms,omitempty"`
	FollowUp    *JobFollowUp   `json:"follow_up,omitempty"`
}

// BuildRepositoryDossierInput selects a repository for durable dossier generation.
type BuildRepositoryDossierInput RepoInput

// DurableArtifactReference identifies a persisted object whose canonical
// detailed representation is available through MCP resources.
type DurableArtifactReference struct {
	Kind string `json:"kind" jsonschema:"Persisted artifact kind"`
	ID   string `json:"id" jsonschema:"Stable artifact identifier"`
	URI  string `json:"uri" jsonschema:"Exact opaque MCP resource URI to read"`
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

// FindRelatedWorkInput selects one workflow target and related-work populations.
type FindRelatedWorkInput struct {
	Target string   `json:"target" jsonschema:"Target scope: hypothesis or opportunity"`
	ID     string   `json:"id" jsonschema:"Hypothesis or opportunity ID"`
	Kinds  []string `json:"kinds,omitempty" jsonschema:"Related-work populations: duplicates and/or competing_pull_requests; defaults to both"`
	Limit  int      `json:"limit,omitempty" jsonschema:"Maximum findings per population from 1 to 100"`
}

// FindRelatedWorkOutput groups independently derived related-work populations.
type FindRelatedWorkOutput struct {
	Duplicates            *CheckOutput `json:"duplicates,omitempty"`
	CompetingPullRequests *CheckOutput `json:"competing_pull_requests,omitempty"`
}

// PromoteOpportunityInput converts a hypothesis into a scoped opportunity.
type PromoteOpportunityInput struct {
	HypothesisID        string      `json:"hypothesis_id" jsonschema:"Hypothesis ID to promote"`
	ProblemStatement    string      `json:"problem_statement" jsonschema:"Problem statement"`
	Scope               string      `json:"scope" jsonschema:"Scope of the opportunity"`
	Impact              string      `json:"impact" jsonschema:"Impact of the opportunity"`
	ExpectedEffort      string      `json:"expected_effort" jsonschema:"Expected effort"`
	Confidence          Probability `json:"confidence" jsonschema:"Confidence from 0.0 to 1.0"`
	Dependencies        []string    `json:"dependencies,omitempty" jsonschema:"Dependencies"`
	MaintainerAlignment string      `json:"maintainer_alignment,omitempty" jsonschema:"Maintainer alignment note"`
	SourceRefs          []SourceRef `json:"source_refs,omitempty" jsonschema:"Source references"`
}

// CancelJobInput selects durable jobs for bounded, persisted cancellation.
type CancelJobInput struct {
	IDs []string `json:"ids" jsonschema:"One to 100 durable job IDs"`
}

// RunValidationInput configures one bounded validation execution job.
type RunValidationInput struct {
	ID             string `json:"id" jsonschema:"Validation definition ID"`
	Target         string `json:"target" jsonschema:"Run target: base, candidate, or both"`
	RunCount       int    `json:"run_count,omitempty" jsonschema:"Attempts per target from 1 to 100; defaults to 1"`
	Concurrency    int    `json:"concurrency,omitempty" jsonschema:"Concurrent attempts from 1 to 16; defaults to 1"`
	PerRunTimeout  string `json:"per_run_timeout,omitempty" jsonschema:"Optional Go duration per attempt"`
	OverallTimeout string `json:"overall_timeout,omitempty" jsonschema:"Optional Go duration for the whole group"`
	SampleInterval string `json:"sample_interval,omitempty" jsonschema:"Process telemetry interval from 10ms to 10s"`
	Execute        bool   `json:"execute" jsonschema:"Must be true to authorize host execution"`
}

type AttachValidationReceiptInput struct {
	ReceiptJSON string `json:"receipt_json" jsonschema:"External validation receipt JSON using gitcontribute.external-validation.v1; maximum 2 MiB"`
}

type ExternalValidationReceiptOutput struct {
	RunID           string `json:"run_id"`
	DefinitionID    string `json:"definition_id"`
	InvestigationID string `json:"investigation_id"`
	Kind            string `json:"kind"`
	Classification  string `json:"classification"`
	ReceiptSHA256   string `json:"receipt_sha256"`
	Producer        string `json:"producer"`
	Incomplete      bool   `json:"incomplete"`
}

// DefineValidationInput records a bounded validation command without executing it.
type DefineValidationInput struct {
	InvestigationID      string                         `json:"investigation_id" jsonschema:"Investigation ID"`
	Kind                 string                         `json:"kind" jsonschema:"Validation kind"`
	Command              string                         `json:"command" jsonschema:"Shell-free command to execute"`
	WorkspaceID          string                         `json:"workspace_id,omitempty" jsonschema:"Managed workspace ID used for both run kinds"`
	BaseWorkspaceID      string                         `json:"base_workspace_id,omitempty" jsonschema:"Managed base workspace ID; requires candidate_workspace_id"`
	CandidateWorkspaceID string                         `json:"candidate_workspace_id,omitempty" jsonschema:"Managed candidate workspace ID; requires base_workspace_id"`
	Env                  []string                       `json:"env,omitempty" jsonschema:"Allowed environment variable names"`
	Timeout              string                         `json:"timeout,omitempty" jsonschema:"Positive Go duration; defaults to 30m"`
	MaxOutputBytes       int64                          `json:"max_output_bytes,omitempty" jsonschema:"Maximum captured bytes per output stream; defaults to 65536"`
	Observation          *ValidationObservationContract `json:"observation,omitempty" jsonschema:"Expected bounded observations over captured base and candidate output"`
	Protocol             string                         `json:"protocol,omitempty" jsonschema:"Structured protocol adapter: mcp_stdio"`
	ReadinessTimeout     string                         `json:"readiness_timeout,omitempty" jsonschema:"Protocol initialization deadline; defaults to 30s"`
}

// ValidationExpectedObservation is one output assertion evaluated without a shell.
type ValidationExpectedObservation struct {
	Run        string `json:"run" jsonschema:"Run kind: base or candidate"`
	Name       string `json:"name" jsonschema:"Short observation name"`
	Source     string `json:"source" jsonschema:"Captured source: stdout, stderr, or artifact"`
	Matcher    string `json:"matcher" jsonschema:"Matcher: exact or regexp"`
	Pattern    string `json:"pattern" jsonschema:"Bounded exact string or Go regular expression"`
	Occurrence string `json:"occurrence,omitempty" jsonschema:"Expected occurrence: present or absent; defaults to present"`
	Path       string `json:"path,omitempty" jsonschema:"Relative artifact path; valid only when source is artifact"`
}

// ValidationObservationContract ties output assertions to the claimed behavior.
type ValidationObservationContract struct {
	Intent       string                          `json:"intent" jsonschema:"Short proof intent or invariant"`
	Observations []ValidationExpectedObservation `json:"observations" jsonschema:"One to eight expected observations for each of base and candidate"`
}

// ValidationOutput is the stable MCP representation of a validation definition.
type ValidationOutput struct {
	ID                   string                         `json:"id"`
	InvestigationID      string                         `json:"investigation_id"`
	Kind                 string                         `json:"kind"`
	Command              []string                       `json:"command"`
	WorkingDir           string                         `json:"working_dir"`
	BaseWorkingDir       string                         `json:"base_working_dir,omitempty"`
	CandidateDir         string                         `json:"candidate_dir,omitempty"`
	WorkspaceID          string                         `json:"workspace_id,omitempty" jsonschema:"Managed workspace ID used for both run kinds"`
	BaseWorkspaceID      string                         `json:"base_workspace_id,omitempty" jsonschema:"Managed base workspace ID"`
	CandidateWorkspaceID string                         `json:"candidate_workspace_id,omitempty" jsonschema:"Managed candidate workspace ID"`
	Env                  []string                       `json:"environment_allowlist,omitempty"`
	Timeout              string                         `json:"timeout,omitempty"`
	MaxOutputBytes       int64                          `json:"max_output_bytes,omitempty"`
	Observation          *ValidationObservationContract `json:"observation,omitempty"`
	Protocol             string                         `json:"protocol,omitempty" jsonschema:"Declared structured protocol adapter"`
	ReadinessTimeout     string                         `json:"readiness_timeout,omitempty" jsonschema:"Protocol initialization deadline"`
	CreatedAt            string                         `json:"created_at"`
}

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
