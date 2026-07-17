package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// V1 read-only tool inputs and outputs.

type SearchRepositoriesInput struct {
	Query string `json:"query,omitempty" jsonschema:"Full-text query over repository owner, name, and description"`
	Owner string `json:"owner,omitempty" jsonschema:"Optional repository owner"`
	Repo  string `json:"repo,omitempty" jsonschema:"Optional repository name"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
}

type SearchRepositoriesOutput struct {
	Query   string             `json:"query"`
	Total   int                `json:"total"`
	Matches []RepositoryOutput `json:"matches"`
}

type SearchThreadsInput struct {
	Query string `json:"query" jsonschema:"Full-text query over thread titles and bodies"`
	Owner string `json:"owner,omitempty" jsonschema:"Optional repository owner"`
	Repo  string `json:"repo,omitempty" jsonschema:"Optional repository name"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
}

type GetRepositoryDossierInput RepoInput

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

type GetJobInput struct {
	ID string `json:"id" jsonschema:"Durable job ID"`
}

type GetJobOutput struct {
	ID                    string `json:"id"`
	Kind                  string `json:"kind"`
	Status                string `json:"status"`
	Request               string `json:"request,omitempty"`
	Result                string `json:"result,omitempty"`
	Error                 string `json:"error,omitempty"`
	Progress              string `json:"progress,omitempty"`
	Statistics            string `json:"statistics,omitempty"`
	CreatedAt             string `json:"created_at"`
	StartedAt             string `json:"started_at,omitempty"`
	CompletedAt           string `json:"completed_at,omitempty"`
	CancelledAt           string `json:"cancelled_at,omitempty"`
	CancellationRequested bool   `json:"cancellation_requested"`
}

type ThreadByNumberInput struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Number int    `json:"number"`
}

// JobReference is returned by long-running tools that submit durable jobs.
type JobReference struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// V1 operation inputs and outputs.

type HydrateRepositoryInput struct {
	Owner    string   `json:"owner" jsonschema:"GitHub repository owner"`
	Repo     string   `json:"repo" jsonschema:"GitHub repository name"`
	Facets   []string `json:"facets,omitempty" jsonschema:"Facets to hydrate; empty selects all applicable facets"`
	MaxPages int      `json:"max_pages,omitempty" jsonschema:"Maximum pages per facet from 1 to 100"`
	State    string   `json:"state,omitempty" jsonschema:"Optional thread state filter: open, closed, or all"`
	Numbers  []int    `json:"numbers,omitempty" jsonschema:"Optional exact thread numbers to hydrate"`
}

type BuildRepositoryDossierInput RepoInput

type StartCrawlInput struct {
	Source string `json:"source" jsonschema:"Crawl source name"`
	Since  string `json:"since,omitempty" jsonschema:"Optional Go duration such as 720h"`
	Budget int    `json:"budget,omitempty" jsonschema:"Maximum number of repository windows to crawl"`
}

type CreateWorkspaceInput struct {
	InvestigationID string `json:"investigation_id" jsonschema:"Investigation ID"`
	Remote          string `json:"remote" jsonschema:"Git remote URL to clone"`
	BaseRef         string `json:"base_ref" jsonschema:"Base ref to resolve"`
	CandidateRef    string `json:"candidate_ref" jsonschema:"Candidate ref to resolve"`
	Name            string `json:"name" jsonschema:"Workspace name"`
}

type RunValidationInput struct {
	ID      string `json:"id" jsonschema:"Validation definition ID"`
	Kind    string `json:"kind" jsonschema:"Run kind: base or candidate"`
	Execute bool   `json:"execute" jsonschema:"Must be true to authorize host execution"`
}

type StartInvestigationInput struct {
	Owner     string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo      string `json:"repo" jsonschema:"GitHub repository name"`
	CommitSHA string `json:"commit_sha,omitempty" jsonschema:"Optional commit SHA"`
	Lens      string `json:"lens,omitempty" jsonschema:"Optional lens name"`
}

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

type CheckDuplicatesInput struct {
	Target string `json:"target" jsonschema:"Target scope: hypothesis or opportunity"`
	ID     string `json:"id" jsonschema:"Hypothesis or opportunity ID"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum findings from 1 to 100"`
}

type CheckCollisionsInput CheckDuplicatesInput

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

type DefineValidationInput struct {
	InvestigationID string   `json:"investigation_id" jsonschema:"Investigation ID"`
	Kind            string   `json:"kind" jsonschema:"Validation kind"`
	Command         string   `json:"command" jsonschema:"Shell-free command to execute"`
	WorkingDir      string   `json:"working_dir" jsonschema:"Working directory"`
	BaseWorkingDir  string   `json:"base_working_dir,omitempty" jsonschema:"Base workspace directory"`
	CandidateDir    string   `json:"candidate_dir,omitempty" jsonschema:"Candidate workspace directory"`
	Env             []string `json:"env,omitempty" jsonschema:"Allowed environment variable names"`
	Timeout         string   `json:"timeout,omitempty" jsonschema:"Go duration such as 30s"`
	MaxOutputBytes  int64    `json:"max_output_bytes,omitempty" jsonschema:"Maximum captured output bytes"`
}

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

type DraftOutput struct {
	OpportunityID string `json:"opportunity_id"`
	Kind          string `json:"kind"`
	Title         string `json:"title"`
	Body          string `json:"body"`
	RenderedAt    string `json:"rendered_at"`
}

type CancelJobInput GetJobInput

func (s *Server) registerV1() {
	readOnly := &mcp.ToolAnnotations{
		Title:           "Read local GitContribute corpus",
		ReadOnlyHint:    true,
		IdempotentHint:  true,
		OpenWorldHint:   boolPtr(false),
		DestructiveHint: boolPtr(false),
	}
	networkWrite := &mcp.ToolAnnotations{
		Title:           "Read GitHub and update the local corpus",
		ReadOnlyHint:    false,
		IdempotentHint:  false,
		OpenWorldHint:   boolPtr(true),
		DestructiveHint: boolPtr(false),
	}
	localWrite := &mcp.ToolAnnotations{
		Title:           "Write to the local corpus",
		ReadOnlyHint:    false,
		IdempotentHint:  false,
		OpenWorldHint:   boolPtr(false),
		DestructiveHint: boolPtr(false),
	}
	validationOp := &mcp.ToolAnnotations{
		Title:           "Execute a validation command in a workspace",
		ReadOnlyHint:    false,
		IdempotentHint:  false,
		OpenWorldHint:   boolPtr(false),
		DestructiveHint: boolPtr(true),
	}
	cancelOp := &mcp.ToolAnnotations{
		Title:           "Cancel a durable job",
		ReadOnlyHint:    false,
		IdempotentHint:  true,
		OpenWorldHint:   boolPtr(false),
		DestructiveHint: boolPtr(false),
	}

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "search_repositories",
		Description: "Search the local repository index without network access",
		Annotations: readOnly,
	}, s.searchRepositories)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "search_threads",
		Description: "Search the local issue and pull request index without network access",
		Annotations: readOnly,
	}, s.searchThreads)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_repository_dossier",
		Description: "Read a source-backed repository dossier from the local corpus",
		Annotations: readOnly,
	}, s.getRepositoryDossier)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "explain_match",
		Description: "Explain why a search result matched, including score signals, source revision, and coverage",
		Annotations: readOnly,
	}, s.explainMatch)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_job",
		Description: "Read a durable job by ID",
		Annotations: readOnly,
	}, s.getJob)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "start_crawl",
		Description: "Start a durable crawl job that discovers repositories from a configured source",
		Annotations: networkWrite,
	}, s.startCrawl)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "hydrate_repository",
		Description: "Start a durable job that hydrates selected facets for repository threads from GitHub",
		Annotations: networkWrite,
	}, s.hydrateRepository)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "build_repository_dossier",
		Description: "Start a durable job that builds a source-backed repository dossier from the local corpus",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Build a repository dossier",
			ReadOnlyHint:    false,
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
			DestructiveHint: boolPtr(false),
		},
	}, s.buildRepositoryDossier)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "create_workspace",
		Description: "Start a durable job that clones a remote and creates a managed worktree",
		Annotations: networkWrite,
	}, s.createWorkspace)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "run_validation",
		Description: "Start a durable validation run that executes a stored command in a workspace",
		Annotations: validationOp,
	}, s.runValidation)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "start_investigation",
		Description: "Create a local investigation workspace",
		Annotations: localWrite,
	}, s.startInvestigation)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "record_hypothesis",
		Description: "Record a hypothesis in an investigation",
		Annotations: localWrite,
	}, s.recordHypothesis)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "check_duplicates",
		Description: "Find corpus threads that may duplicate a hypothesis or opportunity without network access",
		Annotations: readOnly,
	}, s.checkDuplicates)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "check_collisions",
		Description: "Find open pull requests that may collide with a hypothesis or opportunity without network access",
		Annotations: readOnly,
	}, s.checkCollisions)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "promote_opportunity",
		Description: "Promote a hypothesis to an opportunity",
		Annotations: localWrite,
	}, s.promoteOpportunity)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "define_validation",
		Description: "Define a validation command for an investigation",
		Annotations: localWrite,
	}, s.defineValidation)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "prepare_contribution",
		Description: "Render a contribution draft for an opportunity",
		Annotations: localWrite,
	}, s.prepareContribution)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "cancel_job",
		Description: "Cancel a durable job",
		Annotations: cancelOp,
	}, s.cancelJob)

	s.registerV1ResourceTemplates()
}

func (s *Server) registerV1ResourceTemplates() {
	templates := []struct {
		template, name, description string
	}{
		{"github-index://repositories/{owner}/{repo}", "Repository", "Local repository record"},
		{"github-index://threads/{owner}/{repo}/{number}", "Thread", "Local issue or pull request by number"},
		{"github-index://dossiers/{owner}/{repo}", "Dossier", "Local source-backed repository dossier"},
		{"github-index://investigations/{id}", "Investigation", "Local investigation workspace"},
		{"github-index://opportunities/{id}", "Opportunity", "Local contribution opportunity"},
		{"github-index://evidence/{investigation_id}", "Evidence", "Evidence for an investigation"},
		{"github-index://lenses/{name}", "Lens", "Saved lens definition"},
		{"github-index://jobs/{id}", "Job", "Durable job state"},
	}
	for _, t := range templates {
		s.server.AddResourceTemplate(&mcp.ResourceTemplate{
			URITemplate: t.template,
			Name:        t.name,
			Description: t.description,
			MIMEType:    "application/json",
		}, s.readResource)
	}
}

func (s *Server) searchRepositories(ctx context.Context, _ *mcp.CallToolRequest, in SearchRepositoriesInput) (*mcp.CallToolResult, SearchRepositoriesOutput, error) {
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return nil, SearchRepositoriesOutput{}, errors.New("limit must be between 1 and 100")
	}
	if (in.Owner == "") != (in.Repo == "") {
		return nil, SearchRepositoriesOutput{}, errors.New("owner and repo must be provided together")
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
		return nil, SearchOutput{}, errors.New("owner and repo must be provided together")
	}
	searchIn := SearchInput{
		Query: in.Query,
		Owner: in.Owner,
		Repo:  in.Repo,
		Limit: in.Limit,
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

func (s *Server) startCrawl(ctx context.Context, _ *mcp.CallToolRequest, in StartCrawlInput) (*mcp.CallToolResult, JobReference, error) {
	in.Source = strings.TrimSpace(in.Source)
	if in.Source == "" {
		return nil, JobReference{}, errors.New("source is required")
	}
	if in.Since != "" {
		if _, err := time.ParseDuration(in.Since); err != nil {
			return nil, JobReference{}, fmt.Errorf("invalid since duration: %w", err)
		}
	}
	if in.Budget < 0 {
		return nil, JobReference{}, errors.New("budget cannot be negative")
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, JobReference{}, errors.New("crawl is not available")
	}
	out, err := operator.StartCrawl(ctx, in)
	return nil, out, err
}

func (s *Server) hydrateRepository(ctx context.Context, _ *mcp.CallToolRequest, in HydrateRepositoryInput) (*mcp.CallToolResult, JobReference, error) {
	if err := validateRepo(RepoInput{Owner: in.Owner, Repo: in.Repo}); err != nil {
		return nil, JobReference{}, err
	}
	if in.MaxPages == 0 {
		in.MaxPages = 50
	}
	if in.MaxPages < 1 || in.MaxPages > 100 {
		return nil, JobReference{}, errors.New("max_pages must be between 1 and 100")
	}
	if in.State != "" && in.State != "open" && in.State != "closed" && in.State != "all" {
		return nil, JobReference{}, errors.New("state must be open, closed, or all")
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, JobReference{}, errors.New("hydration is not available")
	}
	out, err := operator.HydrateRepository(ctx, in)
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
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, DraftOutput{}, errors.New("contribution preparation is not available")
	}
	out, err := operator.PrepareContribution(ctx, in)
	return nil, out, err
}

func (s *Server) cancelJob(ctx context.Context, _ *mcp.CallToolRequest, in CancelJobInput) (*mcp.CallToolResult, GetJobOutput, error) {
	id, err := normalizeID("id", in.ID)
	if err != nil {
		return nil, GetJobOutput{}, err
	}
	in.ID = id
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, GetJobOutput{}, errors.New("job cancellation is not available")
	}
	out, err := operator.CancelJob(ctx, in)
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
