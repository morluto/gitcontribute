package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/morluto/gitcontribute/internal/discovery"
	"github.com/morluto/gitcontribute/internal/lens"
)

const maxSearchLimit = 100

// CLI is a Kong-based adapter that parses arguments and dispatches to product-
// owned application services. It owns no domain logic.
type CLI struct {
	svc    Service
	runner MCPRunner
	stdout io.Writer
	stderr io.Writer
}

// New constructs a CLI that writes results to stdout and progress to stderr.
func New(service Service, runner MCPRunner, stdout, stderr io.Writer) *CLI {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	return &CLI{svc: service, runner: runner, stdout: stdout, stderr: stderr}
}

type rootCmd struct {
	Init          initCmd          `cmd:"" help:"Initialize the local corpus"`
	Status        statusCmd        `cmd:"" help:"Show corpus status"`
	Sync          syncCmd          `cmd:"" help:"Sync a repository into the corpus"`
	Search        searchCmd        `cmd:"" help:"Search the local corpus"`
	Dossier       dossierCmd       `cmd:"" help:"Show repository dossier"`
	Index         indexCmd         `cmd:"" help:"Index a clean local checkout at its current commit"`
	Source        sourceCmd        `cmd:"" help:"Manage repository discovery sources"`
	Crawl         crawlCmd         `cmd:"" help:"Run a named discovery source"`
	Investigation investigationCmd `cmd:"" help:"Manage investigations"`
	Hypothesis    hypothesisCmd    `cmd:"" help:"Manage hypotheses"`
	Opportunity   opportunityCmd   `cmd:"" help:"Manage opportunities"`
	Workspace     workspaceCmd     `cmd:"" help:"Manage workspaces"`
	Validation    validationCmd    `cmd:"" help:"Manage validation definitions and runs"`
	Evidence      evidenceCmd      `cmd:"" help:"Show evidence packets"`
	Prepare       prepareCmd       `cmd:"" help:"Prepare contribution drafts"`
	Archive       archiveCmd       `cmd:"" help:"Synchronize and hydrate the local archive"`
	Coverage      coverageCmd      `cmd:"" help:"Show local repository facet coverage"`
	Runs          runsCmd          `cmd:"" help:"List durable operation runs"`
	Neighbors     neighborsCmd     `cmd:"" help:"Find similar local threads"`
	Export        exportCmd        `cmd:"" help:"Export redacted local bundles"`
	Clusters      clustersCmd      `cmd:"" help:"Compute and list duplicate-candidate clusters for a repository"`
	Cluster       clusterCmd       `cmd:"" help:"Inspect duplicate-candidate clusters"`
	Lens          lensCmd          `cmd:"" help:"Manage saved lenses"`
	Collection    collectionCmd    `cmd:"" help:"Manage named collections"`
	MCP           mcpCmd           `cmd:"" name:"mcp" help:"Run the MCP server"`
}

type initCmd struct {
	JSON bool `name:"json" help:"Print the result as JSON"`
}

type statusCmd struct {
	JSON bool `name:"json" help:"Print the result as JSON"`
}

type syncCmd struct {
	OwnerRepo string `arg:"" name:"owner/repo" help:"Repository as OWNER/REPO"`
	JSON      bool   `name:"json" help:"Print the result as JSON"`
}

type searchCmd struct {
	Query string `arg:"" name:"query" help:"Search query"`
	Kind  string `name:"kind" short:"k" default:"all" enum:"repos,issues,prs,threads,code,all" help:"Search kind"`
	Repo  string `name:"repo" help:"Restrict to repository OWNER/REPO"`
	Limit int    `name:"limit" default:"20" help:"Maximum number of results"`
	JSON  bool   `name:"json" help:"Print the result as JSON"`
}

type dossierCmd struct {
	OwnerRepo string `arg:"" name:"owner/repo" help:"Repository as OWNER/REPO"`
	JSON      bool   `name:"json" help:"Print the result as JSON"`
}

type indexCmd struct {
	OwnerRepo string `arg:"" name:"owner/repo" help:"Repository as OWNER/REPO"`
	Path      string `arg:"" optional:"" default:"." help:"Path to a clean repository checkout"`
	JSON      bool   `name:"json" help:"Print the result as JSON"`
}

type sourceCmd struct {
	Add  sourceAddCmd  `cmd:"" help:"Add or update a source"`
	List sourceListCmd `cmd:"" help:"List sources"`
	Show sourceShowCmd `cmd:"" help:"Show a source"`
}

type sourceAddCmd struct {
	Search    sourceAddSearchCmd    `cmd:"" help:"Add a GitHub repository Search source"`
	Repos     sourceAddReposCmd     `cmd:"" help:"Add explicit repositories as a source"`
	GHArchive sourceAddGHArchiveCmd `cmd:"" name:"gharchive" help:"Add a GH Archive source"`
}

type sourceAddSearchCmd struct {
	Name  string `name:"name" required:"" help:"Stable source name"`
	Query string `name:"query" required:"" help:"GitHub repository search query"`
	JSON  bool   `name:"json" help:"Print the result as JSON"`
}

type sourceAddReposCmd struct {
	Name  string   `name:"name" help:"Stable source name (defaults to the first repo)"`
	Repos []string `arg:"" optional:"" help:"Repositories as OWNER/REPO or GitHub URLs"`
	JSON  bool     `name:"json" help:"Print the result as JSON"`
}

type sourceAddGHArchiveCmd struct {
	Name   string `name:"name" optional:"" default:"gharchive" help:"Stable source name"`
	Events string `name:"events" required:"" help:"Comma-separated event allowlist (or 'all')"`
	JSON   bool   `name:"json" help:"Print the result as JSON"`
}

type sourceShowCmd struct {
	Name string `arg:"" help:"Source name"`
	JSON bool   `name:"json" help:"Print the result as JSON"`
}

type sourceListCmd struct {
	JSON bool `name:"json" help:"Print the result as JSON"`
}

type crawlCmd struct {
	Name   string        `arg:"" help:"Source name"`
	Since  time.Duration `name:"since" default:"720h" help:"Initial historical window"`
	Budget int           `name:"budget" default:"500" help:"Maximum GitHub API requests"`
	JSON   bool          `name:"json" help:"Print the result as JSON"`
}

type investigationCmd struct {
	Start startInvestigationCmd `cmd:"" help:"Start an investigation"`
	Show  showInvestigationCmd  `cmd:"" help:"Show an investigation"`
	List  listInvestigationCmd  `cmd:"" help:"List investigations"`
}

type startInvestigationCmd struct {
	OwnerRepo string `arg:"" name:"owner/repo" help:"Repository as OWNER/REPO"`
	Commit    string `name:"commit" help:"Optional base commit SHA"`
	Lens      string `name:"lens" help:"Optional lens name"`
	JSON      bool   `name:"json" help:"Print the result as JSON"`
}

type showInvestigationCmd struct {
	ID   string `arg:"" help:"Investigation ID"`
	JSON bool   `name:"json" help:"Print the result as JSON"`
}

type listInvestigationCmd struct {
	JSON bool `name:"json" help:"Print the result as JSON"`
}

type hypothesisCmd struct {
	Add  addHypothesisCmd  `cmd:"" help:"Add a hypothesis"`
	List listHypothesisCmd `cmd:"" help:"List hypotheses"`
}

type addHypothesisCmd struct {
	InvestigationID string `arg:"" help:"Investigation ID"`
	Title           string `name:"title" required:"" help:"Hypothesis title"`
	Description     string `name:"description" required:"" help:"Hypothesis description"`
	Category        string `name:"category" required:"" enum:"bug,performance,architecture,testing,documentation,maintenance,compatibility,security,other" help:"Hypothesis category"`
	JSON            bool   `name:"json" help:"Print the result as JSON"`
}

type listHypothesisCmd struct {
	InvestigationID string `arg:"" help:"Investigation ID"`
	JSON            bool   `name:"json" help:"Print the result as JSON"`
}

type opportunityCmd struct {
	Promote   promoteOpportunityCmd   `cmd:"" help:"Promote a hypothesis to an opportunity"`
	Show      showOpportunityCmd      `cmd:"" help:"Show an opportunity"`
	List      listOpportunityCmd      `cmd:"" help:"List opportunities"`
	SetStatus setStatusOpportunityCmd `cmd:"" name:"set-status" help:"Set opportunity status"`
}

type promoteOpportunityCmd struct {
	HypothesisID string  `arg:"" help:"Hypothesis ID"`
	Problem      string  `name:"problem" required:"" help:"Problem statement"`
	Scope        string  `name:"scope" required:"" help:"Scope of the opportunity"`
	Impact       string  `name:"impact" required:"" help:"Impact description"`
	Effort       string  `name:"effort" required:"" help:"Expected effort"`
	Confidence   float64 `name:"confidence" required:"" help:"Confidence (0-1)"`
	JSON         bool    `name:"json" help:"Print the result as JSON"`
}

type showOpportunityCmd struct {
	ID   string `arg:"" help:"Opportunity ID"`
	JSON bool   `name:"json" help:"Print the result as JSON"`
}

type listOpportunityCmd struct {
	Investigation string `name:"investigation" help:"Filter by investigation ID"`
	JSON          bool   `name:"json" help:"Print the result as JSON"`
}

type setStatusOpportunityCmd struct {
	ID        string `arg:"" help:"Opportunity ID"`
	Status    string `arg:"" enum:"hypothesis,reproduced,validated,maintainer_aligned,implemented,submitted,merged,rejected,deferred,superseded" help:"Target status"`
	Rationale string `name:"rationale" required:"" help:"Rationale for the transition"`
	JSON      bool   `name:"json" help:"Print the result as JSON"`
}

type workspaceCmd struct {
	Create createWorkspaceCmd `cmd:"" help:"Create a workspace for an investigation"`
	Show   showWorkspaceCmd   `cmd:"" help:"Show a workspace"`
}

type createWorkspaceCmd struct {
	InvestigationID string `arg:"" help:"Investigation ID"`
	Remote          string `name:"remote" help:"Git remote URL (defaults to https://github.com/OWNER/REPO.git)"`
	Base            string `name:"base" default:"main" help:"Base ref"`
	Candidate       string `name:"candidate" help:"Candidate ref (defaults to investigation commit)"`
	Name            string `name:"name" help:"Workspace name (defaults to generated ID)"`
	JSON            bool   `name:"json" help:"Print the result as JSON"`
}

type showWorkspaceCmd struct {
	ID   string `arg:"" help:"Workspace ID"`
	JSON bool   `name:"json" help:"Print the result as JSON"`
}

type validationCmd struct {
	Define  defineValidationCmd  `cmd:"" help:"Define a validation"`
	Run     runValidationCmd     `cmd:"" help:"Run a validation definition"`
	Compare compareValidationCmd `cmd:"" help:"Compare two validation runs"`
}

type defineValidationCmd struct {
	InvestigationID string        `arg:"" help:"Investigation ID"`
	Kind            string        `name:"kind" required:"" help:"Validation kind"`
	Command         string        `name:"command" required:"" help:"Command argv as a single string"`
	WorkingDir      string        `name:"working-dir" help:"Working directory for both runs"`
	BaseWorkingDir  string        `name:"base-working-dir" help:"Base workspace directory"`
	CandidateDir    string        `name:"candidate-dir" help:"Candidate workspace directory"`
	Env             []string      `name:"env" help:"Host environment variable names to pass through"`
	Timeout         time.Duration `name:"timeout" help:"Maximum execution time"`
	MaxOutput       int64         `name:"max-output" help:"Maximum captured output bytes per stream"`
	JSON            bool          `name:"json" help:"Print the result as JSON"`
}

type runValidationCmd struct {
	ID      string `arg:"" help:"Validation definition ID"`
	Kind    string `name:"kind" required:"" enum:"base,candidate" help:"Run kind"`
	Execute bool   `name:"execute" help:"Authorize execution of the displayed command on the host"`
	JSON    bool   `name:"json" help:"Print the result as JSON"`
}

type compareValidationCmd struct {
	BaseRunID      string `arg:"" help:"Base run ID"`
	CandidateRunID string `arg:"" help:"Candidate run ID"`
	JSON           bool   `name:"json" help:"Print the result as JSON"`
}

type evidenceCmd struct {
	Show showEvidenceCmd `cmd:"" help:"Show evidence for an investigation"`
}

type showEvidenceCmd struct {
	InvestigationID string `arg:"" help:"Investigation ID"`
	JSON            bool   `name:"json" help:"Print the result as JSON"`
}

type prepareCmd struct {
	Issue issueCmd `cmd:"" help:"Prepare an issue draft"`
	PR    prCmd    `cmd:"" name:"pr" help:"Prepare a pull request draft"`
}

type issueCmd struct {
	OpportunityID string `arg:"" help:"Opportunity ID"`
	Guidance      string `name:"guidance" help:"Repository contribution guidance"`
	Success       string `name:"success" help:"Success criteria"`
	JSON          bool   `name:"json" help:"Print the result as JSON"`
}

type prCmd struct {
	OpportunityID string `arg:"" help:"Opportunity ID"`
	WorkspaceID   string `name:"workspace" help:"Workspace ID to include diff as changes"`
	Approach      string `name:"approach" required:"" help:"Approach description"`
	Changes       string `name:"changes" help:"Focused changes description"`
	Compatibility string `name:"compatibility" help:"Compatibility notes"`
	Limitations   string `name:"limitations" help:"Limitations"`
	LinkedIssue   string `name:"linked-issue" help:"Linked issue"`
	Guidance      string `name:"guidance" help:"Repository contribution guidance"`
	JSON          bool   `name:"json" help:"Print the result as JSON"`
}

type clustersCmd struct {
	OwnerRepo string `arg:"" name:"owner/repo" help:"Repository as OWNER/REPO"`
	Limit     int    `name:"limit" default:"50" help:"Maximum clusters to return"`
	JSON      bool   `name:"json" help:"Print the result as JSON"`
}

type clusterCmd struct {
	Show clusterShowCmd `cmd:"" help:"Show a cluster by stable id"`
}

type clusterShowCmd struct {
	ID    string `arg:"" help:"Cluster stable id"`
	Limit int    `name:"limit" default:"100" help:"Maximum members to show"`
	JSON  bool   `name:"json" help:"Print the result as JSON"`
}

type archiveCmd struct {
	Sync    archiveSyncCmd    `cmd:"" help:"Synchronize repository threads"`
	Hydrate archiveHydrateCmd `cmd:"" help:"Hydrate one issue or pull request"`
}

type archiveSyncCmd struct {
	OwnerRepo string        `arg:"" name:"owner/repo" help:"Repository as OWNER/REPO"`
	State     string        `name:"state" default:"all" enum:"open,closed,all" help:"Thread state"`
	Since     time.Duration `name:"since" help:"Only threads updated within this duration"`
	Numbers   string        `name:"numbers" help:"Comma-separated exact thread numbers"`
	MaxPages  int           `name:"max-pages" default:"1000" help:"Maximum issue-list pages"`
	JSON      bool          `name:"json" help:"Print the result as JSON"`
}

type archiveHydrateCmd struct {
	Thread   string `arg:"" name:"owner/repo#number" help:"Thread as OWNER/REPO#NUMBER"`
	With     string `name:"with" help:"Comma-separated facets (defaults to all applicable facets)"`
	MaxPages int    `name:"max-pages" default:"50" help:"Maximum pages per facet"`
	JSON     bool   `name:"json" help:"Print the result as JSON"`
}

type coverageCmd struct {
	OwnerRepo string `arg:"" name:"owner/repo" help:"Repository as OWNER/REPO"`
	JSON      bool   `name:"json" help:"Print the result as JSON"`
}

type runsCmd struct {
	Limit int  `name:"limit" default:"50" help:"Maximum runs to return"`
	JSON  bool `name:"json" help:"Print the result as JSON"`
}

type neighborsCmd struct {
	Thread string `arg:"" name:"owner/repo#number" help:"Thread as OWNER/REPO#NUMBER"`
	Kind   string `name:"kind" required:"" enum:"issue,pull_request" help:"Thread kind"`
	Limit  int    `name:"limit" default:"10" help:"Maximum neighbors to return"`
	JSON   bool   `name:"json" help:"Print the result as JSON"`
}

type exportCmd struct {
	Dossier  exportDossierCmd  `cmd:"" help:"Export a repository dossier"`
	Evidence exportEvidenceCmd `cmd:"" help:"Export investigation evidence"`
}

type exportDossierCmd struct {
	OwnerRepo string `arg:"" name:"owner/repo" help:"Repository as OWNER/REPO"`
	Format    string `name:"format" default:"markdown" enum:"json,markdown,md" help:"Export format"`
	Output    string `name:"output" help:"Write to a file instead of stdout"`
}

type exportEvidenceCmd struct {
	InvestigationID string `arg:"" help:"Investigation ID"`
	Format          string `name:"format" default:"markdown" enum:"json,markdown,md" help:"Export format"`
	Output          string `name:"output" help:"Write to a file instead of stdout"`
}

type lensCmd struct {
	Add  lensAddCmd  `cmd:"" help:"Add or replace a saved lens from a JSON file"`
	List lensListCmd `cmd:"" help:"List saved lenses"`
	Show lensShowCmd `cmd:"" help:"Show a saved lens"`
}

type lensAddCmd struct {
	Name string `arg:"" help:"Lens name"`
	File string `name:"file" required:"" help:"Path to JSON lens definition"`
	JSON bool   `name:"json" help:"Print the result as JSON"`
}

type lensListCmd struct {
	JSON bool `name:"json" help:"Print the result as JSON"`
}

type lensShowCmd struct {
	Name string `arg:"" help:"Lens name"`
	JSON bool   `name:"json" help:"Print the result as JSON"`
}

type collectionCmd struct {
	Create collectionCreateCmd `cmd:"" help:"Create a named collection"`
	Add    collectionAddCmd    `cmd:"" help:"Add typed references to a collection"`
	List   collectionListCmd   `cmd:"" help:"List collections"`
}

type collectionCreateCmd struct {
	Name string `arg:"" help:"Collection name"`
	JSON bool   `name:"json" help:"Print the result as JSON"`
}

type collectionAddCmd struct {
	Name    string   `arg:"" help:"Collection name"`
	Members []string `arg:"" help:"Members as kind:ref (e.g. repo:owner/repo, issue:owner/repo#12, pr:owner/repo#12)"`
	JSON    bool     `name:"json" help:"Print the result as JSON"`
}

type collectionListCmd struct {
	JSON bool `name:"json" help:"Print the result as JSON"`
}

type mcpCmd struct {
	Transport string `name:"transport" default:"stdio" enum:"stdio" help:"MCP transport protocol"`
}

// Run parses arguments and dispatches to the appropriate Service or MCPRunner
// method. It respects context cancellation.
func (c *CLI) Run(ctx context.Context, args []string) error {
	var cli rootCmd
	parser, err := kong.New(&cli,
		kong.Name("gitcontribute"),
		kong.Description("GitHub contribution research workbench"),
		kong.UsageOnError(),
	)
	if err != nil {
		return NewCLIError(ExitGeneral, err)
	}
	kctx, err := parser.Parse(args)
	if err != nil {
		return NewCLIError(ExitUsage, err)
	}

	var commandParts []string
	for _, trace := range kctx.Path {
		if trace.Command != nil {
			commandParts = append(commandParts, trace.Command.Name)
		}
	}
	command := strings.Join(commandParts, " ")
	cmd := command
	if idx := strings.IndexByte(command, ' '); idx >= 0 {
		cmd = command[:idx]
	}
	switch cmd {
	case "init":
		return c.runInit(ctx, &cli.Init)
	case "status":
		return c.runStatus(ctx, &cli.Status)
	case "sync":
		return c.runSync(ctx, &cli.Sync)
	case "search":
		return c.runSearch(ctx, &cli.Search)
	case "dossier":
		return c.runDossier(ctx, &cli.Dossier)
	case "index":
		return c.runIndex(ctx, &cli.Index)
	case "source":
		return c.runSource(ctx, command, &cli.Source)
	case "crawl":
		return c.runCrawl(ctx, &cli.Crawl)
	case "investigation":
		return c.runInvestigation(ctx, command, &cli.Investigation)
	case "hypothesis":
		return c.runHypothesis(ctx, command, &cli.Hypothesis)
	case "opportunity":
		return c.runOpportunity(ctx, command, &cli.Opportunity)
	case "workspace":
		return c.runWorkspace(ctx, command, &cli.Workspace)
	case "validation":
		return c.runValidation(ctx, command, &cli.Validation)
	case "evidence":
		return c.runEvidence(ctx, command, &cli.Evidence)
	case "prepare":
		return c.runPrepare(ctx, command, &cli.Prepare)
	case "archive":
		return c.runArchive(ctx, command, &cli.Archive)
	case "coverage":
		return c.runCoverage(ctx, &cli.Coverage)
	case "runs":
		return c.runRuns(ctx, &cli.Runs)
	case "neighbors":
		return c.runNeighbors(ctx, &cli.Neighbors)
	case "export":
		return c.runExport(ctx, command, &cli.Export)
	case "clusters":
		return c.runClusters(ctx, &cli.Clusters)
	case "cluster":
		return c.runCluster(ctx, command, &cli.Cluster)
	case "lens":
		return c.runLens(ctx, command, &cli.Lens)
	case "collection":
		return c.runCollection(ctx, command, &cli.Collection)
	case "mcp":
		return c.runMCP(ctx, &cli.MCP)
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown command: %s", cmd))
	}
}

func (c *CLI) discoveryService() (DiscoveryService, error) {
	service, ok := c.svc.(DiscoveryService)
	if !ok {
		return nil, NewCLIError(ExitNotWired, ErrNotWired)
	}
	return service, nil
}

func (c *CLI) investigationService() (InvestigationService, error) {
	service, ok := c.svc.(InvestigationService)
	if !ok {
		return nil, NewCLIError(ExitNotWired, ErrNotWired)
	}
	return service, nil
}

func (c *CLI) workspaceService() (WorkspaceService, error) {
	service, ok := c.svc.(WorkspaceService)
	if !ok {
		return nil, NewCLIError(ExitNotWired, ErrNotWired)
	}
	return service, nil
}

func (c *CLI) validationService() (ValidationService, error) {
	service, ok := c.svc.(ValidationService)
	if !ok {
		return nil, NewCLIError(ExitNotWired, ErrNotWired)
	}
	return service, nil
}

func (c *CLI) evidenceService() (EvidenceService, error) {
	service, ok := c.svc.(EvidenceService)
	if !ok {
		return nil, NewCLIError(ExitNotWired, ErrNotWired)
	}
	return service, nil
}

func (c *CLI) contributionService() (ContributionService, error) {
	service, ok := c.svc.(ContributionService)
	if !ok {
		return nil, NewCLIError(ExitNotWired, ErrNotWired)
	}
	return service, nil
}

func (c *CLI) clusteringService() (ClusteringService, error) {
	service, ok := c.svc.(ClusteringService)
	if !ok {
		return nil, NewCLIError(ExitNotWired, ErrNotWired)
	}
	return service, nil
}

func (c *CLI) lensService() (LensService, error) {
	service, ok := c.svc.(LensService)
	if !ok {
		return nil, NewCLIError(ExitNotWired, ErrNotWired)
	}
	return service, nil
}

func (c *CLI) collectionService() (CollectionService, error) {
	service, ok := c.svc.(CollectionService)
	if !ok {
		return nil, NewCLIError(ExitNotWired, ErrNotWired)
	}
	return service, nil
}

func (c *CLI) archiveService() (ArchiveService, error) {
	service, ok := c.svc.(ArchiveService)
	if !ok {
		return nil, NewCLIError(ExitNotWired, ErrNotWired)
	}
	return service, nil
}

func (c *CLI) localQueryService() (LocalQueryService, error) {
	service, ok := c.svc.(LocalQueryService)
	if !ok {
		return nil, NewCLIError(ExitNotWired, ErrNotWired)
	}
	return service, nil
}

func (c *CLI) exportService() (ExportService, error) {
	service, ok := c.svc.(ExportService)
	if !ok {
		return nil, NewCLIError(ExitNotWired, ErrNotWired)
	}
	return service, nil
}

func (c *CLI) runSource(ctx context.Context, command string, cmd *sourceCmd) error {
	service, err := c.discoveryService()
	if err != nil {
		return err
	}
	switch command {
	case "source add search":
		result, err := service.AddSearchSource(ctx, cmd.Add.Search.Name, cmd.Add.Search.Query)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Add.Search.JSON, result)
	case "source add repos":
		refs, name, err := parseRepoSourceArgs(cmd.Add.Repos)
		if err != nil {
			return NewCLIError(ExitUsage, err)
		}
		result, err := service.AddRepoSource(ctx, name, refs)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Add.Repos.JSON, result)
	case "source add gharchive":
		events, err := parseGHArchiveEvents(cmd.Add.GHArchive.Events)
		if err != nil {
			return NewCLIError(ExitUsage, err)
		}
		result, err := service.AddGHArchiveSource(ctx, cmd.Add.GHArchive.Name, events)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Add.GHArchive.JSON, result)
	case "source list":
		result, err := service.ListSources(ctx)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.List.JSON, result)
	case "source show":
		result, err := service.ShowSource(ctx, cmd.Show.Name)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Show.JSON, result)
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown source command: %s", command))
	}
}

func parseRepoSourceArgs(cmd sourceAddReposCmd) ([]RepoRef, string, error) {
	if len(cmd.Repos) == 0 {
		return nil, "", errors.New("at least one repository is required")
	}
	refs := make([]RepoRef, 0, len(cmd.Repos))
	for _, raw := range cmd.Repos {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		dr, err := discovery.ParseRepoRef(raw)
		if err != nil {
			return nil, "", err
		}
		refs = append(refs, RepoRef{Owner: dr.Owner, Repo: dr.Repo})
	}
	if len(refs) == 0 {
		return nil, "", errors.New("at least one valid repository is required")
	}
	name := strings.TrimSpace(cmd.Name)
	if name == "" {
		if len(refs) > 1 {
			return nil, "", errors.New("--name is required when adding multiple repositories")
		}
		name = fmt.Sprintf("%s-%s", refs[0].Owner, refs[0].Repo)
		name = strings.ToLower(name)
	}
	return refs, name, nil
}

func parseGHArchiveEvents(events string) ([]string, error) {
	events = strings.TrimSpace(events)
	if events == "" {
		return nil, errors.New("event allowlist is required")
	}
	if strings.ToLower(events) == "all" {
		return nil, nil
	}
	parts := strings.Split(events, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{})
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !discovery.IsKnownEventType(p) {
			return nil, fmt.Errorf("unknown GH Archive event type %q", p)
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, errors.New("event allowlist is required")
	}
	return out, nil
}

func (c *CLI) runCrawl(ctx context.Context, cmd *crawlCmd) error {
	if cmd.Since <= 0 || cmd.Budget <= 0 || cmd.Budget > 5000 {
		return NewCLIError(ExitUsage, errors.New("--since must be positive and --budget must be between 1 and 5000"))
	}
	service, err := c.discoveryService()
	if err != nil {
		return err
	}
	fmt.Fprintf(c.stderr, "crawling %s...\n", cmd.Name)
	result, err := service.Crawl(ctx, cmd.Name, CrawlOptions{Since: cmd.Since, Budget: cmd.Budget})
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}

func (c *CLI) runInvestigation(ctx context.Context, command string, cmd *investigationCmd) error {
	service, err := c.investigationService()
	if err != nil {
		return err
	}
	switch command {
	case "investigation start":
		repo, err := parseRepo(cmd.Start.OwnerRepo)
		if err != nil {
			return err
		}
		fmt.Fprintf(c.stderr, "starting investigation for %s...\n", repo)
		result, err := service.StartInvestigation(ctx, repo, cmd.Start.Commit, cmd.Start.Lens)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Start.JSON, result)
	case "investigation show":
		result, err := service.ShowInvestigation(ctx, cmd.Show.ID)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Show.JSON, result)
	case "investigation list":
		result, err := service.ListInvestigations(ctx)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.List.JSON, result)
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown investigation command: %s", command))
	}
}

func (c *CLI) runHypothesis(ctx context.Context, command string, cmd *hypothesisCmd) error {
	service, err := c.investigationService()
	if err != nil {
		return err
	}
	switch command {
	case "hypothesis add":
		fmt.Fprintf(c.stderr, "recording hypothesis for investigation %s...\n", cmd.Add.InvestigationID)
		result, err := service.AddHypothesis(ctx, cmd.Add.InvestigationID, cmd.Add.Title, cmd.Add.Description, cmd.Add.Category)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Add.JSON, result)
	case "hypothesis list":
		result, err := service.ListHypotheses(ctx, cmd.List.InvestigationID)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.List.JSON, result)
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown hypothesis command: %s", command))
	}
}

func (c *CLI) runOpportunity(ctx context.Context, command string, cmd *opportunityCmd) error {
	service, err := c.investigationService()
	if err != nil {
		return err
	}
	switch command {
	case "opportunity promote":
		fmt.Fprintf(c.stderr, "promoting hypothesis %s to opportunity...\n", cmd.Promote.HypothesisID)
		result, err := service.PromoteOpportunity(ctx, cmd.Promote.HypothesisID, cmd.Promote.Problem, cmd.Promote.Scope, cmd.Promote.Impact, cmd.Promote.Effort, cmd.Promote.Confidence)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Promote.JSON, result)
	case "opportunity show":
		result, err := service.ShowOpportunity(ctx, cmd.Show.ID)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Show.JSON, result)
	case "opportunity list":
		result, err := service.ListOpportunities(ctx, cmd.List.Investigation)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.List.JSON, result)
	case "opportunity set-status":
		result, err := service.SetOpportunityStatus(ctx, cmd.SetStatus.ID, cmd.SetStatus.Status, cmd.SetStatus.Rationale)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.SetStatus.JSON, result)
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown opportunity command: %s", command))
	}
}

func (c *CLI) runWorkspace(ctx context.Context, command string, cmd *workspaceCmd) error {
	service, err := c.workspaceService()
	if err != nil {
		return err
	}
	switch command {
	case "workspace create":
		fmt.Fprintf(c.stderr, "creating workspace for investigation %s...\n", cmd.Create.InvestigationID)
		result, err := service.CreateWorkspace(ctx, cmd.Create.InvestigationID, WorkspaceCreateOptions{
			Remote:       cmd.Create.Remote,
			BaseRef:      cmd.Create.Base,
			CandidateRef: cmd.Create.Candidate,
			Name:         cmd.Create.Name,
		})
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Create.JSON, result)
	case "workspace show":
		result, err := service.ShowWorkspace(ctx, cmd.Show.ID)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Show.JSON, result)
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown workspace command: %s", command))
	}
}

func (c *CLI) runValidation(ctx context.Context, command string, cmd *validationCmd) error {
	service, err := c.validationService()
	if err != nil {
		return err
	}
	switch command {
	case "validation define":
		fmt.Fprintf(c.stderr, "defining validation for investigation %s...\n", cmd.Define.InvestigationID)
		result, err := service.DefineValidation(ctx, cmd.Define.InvestigationID, DefineValidationOptions{
			Kind:           cmd.Define.Kind,
			Command:        cmd.Define.Command,
			WorkingDir:     cmd.Define.WorkingDir,
			BaseWorkingDir: cmd.Define.BaseWorkingDir,
			CandidateDir:   cmd.Define.CandidateDir,
			Env:            cmd.Define.Env,
			Timeout:        cmd.Define.Timeout,
			MaxOutputBytes: cmd.Define.MaxOutput,
		})
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Define.JSON, result)
	case "validation run":
		definition, err := service.ShowValidation(ctx, cmd.Run.ID)
		if err != nil {
			return c.mapError(err)
		}
		dir := definition.WorkingDir
		if cmd.Run.Kind == "base" && definition.BaseWorkingDir != "" {
			dir = definition.BaseWorkingDir
		}
		if cmd.Run.Kind == "candidate" && definition.CandidateDir != "" {
			dir = definition.CandidateDir
		}
		visible := formatCommand(definition.Command)
		if !cmd.Run.Execute {
			return NewCLIError(ExitUsage, fmt.Errorf("host execution requires --execute; command: %s (directory: %s)", visible, dir))
		}
		fmt.Fprintf(c.stderr, "executing in %s: %s\n", dir, visible)
		result, err := service.RunValidation(ctx, cmd.Run.ID, RunValidationOptions{Kind: cmd.Run.Kind, Execute: true})
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Run.JSON, result)
	case "validation compare":
		result, err := service.CompareValidation(ctx, cmd.Compare.BaseRunID, cmd.Compare.CandidateRunID)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Compare.JSON, result)
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown validation command: %s", command))
	}
}

func formatCommand(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = strconv.Quote(arg)
	}
	return strings.Join(quoted, " ")
}

func (c *CLI) runEvidence(ctx context.Context, command string, cmd *evidenceCmd) error {
	service, err := c.evidenceService()
	if err != nil {
		return err
	}
	switch command {
	case "evidence show":
		result, err := service.ShowEvidence(ctx, cmd.Show.InvestigationID)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Show.JSON, result)
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown evidence command: %s", command))
	}
}

func (c *CLI) runPrepare(ctx context.Context, command string, cmd *prepareCmd) error {
	service, err := c.contributionService()
	if err != nil {
		return err
	}
	switch command {
	case "prepare issue":
		fmt.Fprintf(c.stderr, "preparing issue draft for opportunity %s...\n", cmd.Issue.OpportunityID)
		result, err := service.PrepareIssue(ctx, cmd.Issue.OpportunityID, PrepareIssueOptions{
			Guidance: cmd.Issue.Guidance,
			Success:  cmd.Issue.Success,
		})
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Issue.JSON, result)
	case "prepare pr":
		fmt.Fprintf(c.stderr, "preparing pull request draft for opportunity %s...\n", cmd.PR.OpportunityID)
		result, err := service.PreparePullRequest(ctx, cmd.PR.OpportunityID, PreparePROptions{
			WorkspaceID:   cmd.PR.WorkspaceID,
			Approach:      cmd.PR.Approach,
			Changes:       cmd.PR.Changes,
			Compatibility: cmd.PR.Compatibility,
			Limitations:   cmd.PR.Limitations,
			LinkedIssue:   cmd.PR.LinkedIssue,
			Guidance:      cmd.PR.Guidance,
		})
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.PR.JSON, result)
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown prepare command: %s", command))
	}
}

func (c *CLI) runIndex(ctx context.Context, cmd *indexCmd) error {
	repo, err := parseRepo(cmd.OwnerRepo)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.stderr, "indexing %s from %s...\n", repo, cmd.Path)
	result, err := c.svc.Index(ctx, repo, cmd.Path)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}

func (c *CLI) runInit(ctx context.Context, cmd *initCmd) error {
	fmt.Fprintln(c.stderr, "initializing...")
	res, err := c.svc.Init(ctx)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, res)
}

func (c *CLI) runStatus(ctx context.Context, cmd *statusCmd) error {
	res, err := c.svc.Status(ctx)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, res)
}

func (c *CLI) runSync(ctx context.Context, cmd *syncCmd) error {
	repo, err := parseRepo(cmd.OwnerRepo)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.stderr, "syncing %s...\n", repo)
	res, err := c.svc.Sync(ctx, repo)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, res)
}

func (c *CLI) runSearch(ctx context.Context, cmd *searchCmd) error {
	opts := SearchOptions{Kind: cmd.Kind, Repo: cmd.Repo, Limit: cmd.Limit}
	if opts.Limit <= 0 || opts.Limit > maxSearchLimit {
		return NewCLIError(ExitUsage, fmt.Errorf("limit must be between 1 and %d", maxSearchLimit))
	}
	if opts.Repo != "" {
		if _, err := parseRepo(opts.Repo); err != nil {
			return NewCLIError(ExitUsage, fmt.Errorf("invalid --repo value: %w", err))
		}
	}
	res, err := c.svc.Search(ctx, cmd.Query, opts)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, res)
}

func (c *CLI) runDossier(ctx context.Context, cmd *dossierCmd) error {
	repo, err := parseRepo(cmd.OwnerRepo)
	if err != nil {
		return err
	}
	res, err := c.svc.Dossier(ctx, repo)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, res)
}

func (c *CLI) runMCP(ctx context.Context, cmd *mcpCmd) error {
	fmt.Fprintf(c.stderr, "starting mcp server (transport=%s)...\n", cmd.Transport)
	return c.mapError(c.runner.Run(ctx, MCPOptions{Transport: cmd.Transport}))
}

func (c *CLI) render(json bool, v any) error {
	if json {
		return writeJSON(c.stdout, v)
	}
	s, err := humanOutput(v)
	if err != nil {
		return NewCLIError(ExitGeneral, err)
	}
	_, err = fmt.Fprintln(c.stdout, s)
	return err
}

func (c *CLI) mapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return NewCLIError(ExitCancelled, err)
	}
	var ce *CLIError
	if errors.As(err, &ce) {
		return ce
	}
	return NewCLIError(ExitGeneral, err)
}

func parseRepo(s string) (RepoRef, error) {
	parts := strings.Split(s, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return RepoRef{}, NewCLIError(ExitUsage, fmt.Errorf("invalid repository %q: expected OWNER/REPO", s))
	}
	return RepoRef{Owner: parts[0], Repo: parts[1]}, nil
}

func (c *CLI) runClusters(ctx context.Context, cmd *clustersCmd) error {
	repo, err := parseRepo(cmd.OwnerRepo)
	if err != nil {
		return err
	}
	if cmd.Limit <= 0 || cmd.Limit > 1000 {
		return NewCLIError(ExitUsage, fmt.Errorf("limit must be between 1 and 1000"))
	}
	service, err := c.clusteringService()
	if err != nil {
		return err
	}
	fmt.Fprintf(c.stderr, "computing clusters for %s...\n", repo)
	res, err := service.Clusters(ctx, repo, cmd.Limit)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, res)
}

func (c *CLI) runCluster(ctx context.Context, command string, cmd *clusterCmd) error {
	if command != "cluster show" {
		return NewCLIError(ExitUsage, fmt.Errorf("unknown cluster command: %s", command))
	}
	show := &cmd.Show
	if strings.TrimSpace(show.ID) == "" {
		return NewCLIError(ExitUsage, errors.New("cluster id is required"))
	}
	if show.Limit <= 0 || show.Limit > 1000 {
		return NewCLIError(ExitUsage, fmt.Errorf("limit must be between 1 and 1000"))
	}
	service, err := c.clusteringService()
	if err != nil {
		return err
	}
	res, err := service.Cluster(ctx, show.ID, show.Limit)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(show.JSON, res)
}

func (c *CLI) runArchive(ctx context.Context, command string, cmd *archiveCmd) error {
	service, err := c.archiveService()
	if err != nil {
		return err
	}
	switch command {
	case "archive sync":
		repo, err := parseRepo(cmd.Sync.OwnerRepo)
		if err != nil {
			return err
		}
		numbers, err := parseNumberList(cmd.Sync.Numbers)
		if err != nil {
			return NewCLIError(ExitUsage, err)
		}
		if cmd.Sync.Since < 0 {
			return NewCLIError(ExitUsage, errors.New("since duration cannot be negative"))
		}
		if len(numbers) > 0 && (cmd.Sync.State != "all" || cmd.Sync.Since != 0) {
			return NewCLIError(ExitUsage, errors.New("state and since filters cannot be combined with exact thread numbers"))
		}
		fmt.Fprintf(c.stderr, "syncing archive for %s...\n", repo)
		result, err := service.ArchiveSync(ctx, repo, ArchiveSyncOptions{
			State: cmd.Sync.State, Since: cmd.Sync.Since, Numbers: numbers, MaxPages: cmd.Sync.MaxPages,
		})
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Sync.JSON, result)
	case "archive hydrate":
		repo, number, err := parseThreadRef(cmd.Hydrate.Thread)
		if err != nil {
			return NewCLIError(ExitUsage, err)
		}
		fmt.Fprintf(c.stderr, "hydrating %s#%d...\n", repo, number)
		result, err := service.Hydrate(ctx, repo, number, HydrateOptions{
			Facets: splitCSV(cmd.Hydrate.With), MaxPages: cmd.Hydrate.MaxPages,
		})
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Hydrate.JSON, result)
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown archive command: %s", command))
	}
}

func (c *CLI) runCoverage(ctx context.Context, cmd *coverageCmd) error {
	repo, err := parseRepo(cmd.OwnerRepo)
	if err != nil {
		return err
	}
	service, err := c.localQueryService()
	if err != nil {
		return err
	}
	result, err := service.Coverage(ctx, repo)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}

func (c *CLI) runRuns(ctx context.Context, cmd *runsCmd) error {
	if cmd.Limit <= 0 || cmd.Limit > 1000 {
		return NewCLIError(ExitUsage, errors.New("limit must be between 1 and 1000"))
	}
	service, err := c.localQueryService()
	if err != nil {
		return err
	}
	result, err := service.RunHistory(ctx, cmd.Limit)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}

func (c *CLI) runNeighbors(ctx context.Context, cmd *neighborsCmd) error {
	repo, number, err := parseThreadRef(cmd.Thread)
	if err != nil {
		return NewCLIError(ExitUsage, err)
	}
	if cmd.Limit <= 0 || cmd.Limit > 1000 {
		return NewCLIError(ExitUsage, errors.New("limit must be between 1 and 1000"))
	}
	service, err := c.localQueryService()
	if err != nil {
		return err
	}
	result, err := service.NeighborQuery(ctx, repo, cmd.Kind, number, cmd.Limit)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}

func (c *CLI) runExport(ctx context.Context, command string, cmd *exportCmd) error {
	service, err := c.exportService()
	if err != nil {
		return err
	}
	var result *ExportResult
	var output string
	switch command {
	case "export dossier":
		repo, err := parseRepo(cmd.Dossier.OwnerRepo)
		if err != nil {
			return err
		}
		result, err = service.ExportDossier(ctx, repo, cmd.Dossier.Format)
		output = cmd.Dossier.Output
	case "export evidence":
		result, err = service.ExportEvidence(ctx, cmd.Evidence.InvestigationID, cmd.Evidence.Format)
		output = cmd.Evidence.Output
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown export command: %s", command))
	}
	if err != nil {
		return c.mapError(err)
	}
	if output != "" {
		if err := os.WriteFile(output, []byte(result.Content), 0600); err != nil {
			return c.mapError(fmt.Errorf("write export: %w", err))
		}
		fmt.Fprintf(c.stderr, "wrote %s %s export to %s\n", result.Kind, result.Format, output)
		return nil
	}
	_, err = io.WriteString(c.stdout, result.Content)
	if err == nil && !strings.HasSuffix(result.Content, "\n") {
		_, err = io.WriteString(c.stdout, "\n")
	}
	return c.mapError(err)
}

func parseThreadRef(raw string) (RepoRef, int, error) {
	idx := strings.LastIndexByte(raw, '#')
	if idx <= 0 || idx == len(raw)-1 {
		return RepoRef{}, 0, fmt.Errorf("invalid thread reference %q: expected OWNER/REPO#NUMBER", raw)
	}
	repo, err := parseRepo(raw[:idx])
	if err != nil {
		return RepoRef{}, 0, err
	}
	number, err := strconv.Atoi(raw[idx+1:])
	if err != nil || number <= 0 {
		return RepoRef{}, 0, fmt.Errorf("invalid thread reference %q: expected positive number", raw)
	}
	return repo, number, nil
}

func parseNumberList(raw string) ([]int, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parts := splitCSV(raw)
	numbers := make([]int, len(parts))
	for i, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number <= 0 {
			return nil, fmt.Errorf("invalid thread number %q", part)
		}
		numbers[i] = number
	}
	return numbers, nil
}

func splitCSV(raw string) []string {
	var values []string
	for _, value := range strings.Split(raw, ",") {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func (c *CLI) runLens(ctx context.Context, command string, cmd *lensCmd) error {
	service, err := c.lensService()
	if err != nil {
		return err
	}
	switch command {
	case "lens add":
		data, err := os.ReadFile(cmd.Add.File)
		if err != nil {
			return NewCLIError(ExitUsage, fmt.Errorf("read lens file: %w", err))
		}
		var def lens.Definition
		if err := json.Unmarshal(data, &def); err != nil {
			return NewCLIError(ExitUsage, fmt.Errorf("parse lens file: %w", err))
		}
		if strings.TrimSpace(cmd.Add.Name) == "" {
			return NewCLIError(ExitUsage, errors.New("lens name is required"))
		}
		fmt.Fprintf(c.stderr, "saving lens %s...\n", cmd.Add.Name)
		res, err := service.AddLens(ctx, cmd.Add.Name, def)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Add.JSON, res)
	case "lens list":
		res, err := service.ListLenses(ctx)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.List.JSON, res)
	case "lens show":
		res, err := service.ShowLens(ctx, cmd.Show.Name)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Show.JSON, res)
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown lens command: %s", command))
	}
}

func (c *CLI) runCollection(ctx context.Context, command string, cmd *collectionCmd) error {
	service, err := c.collectionService()
	if err != nil {
		return err
	}
	switch command {
	case "collection create":
		fmt.Fprintf(c.stderr, "creating collection %s...\n", cmd.Create.Name)
		res, err := service.CreateCollection(ctx, cmd.Create.Name)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Create.JSON, res)
	case "collection add":
		if len(cmd.Add.Members) == 0 {
			return NewCLIError(ExitUsage, errors.New("at least one member is required"))
		}
		members := make([]CollectionMember, len(cmd.Add.Members))
		for i, raw := range cmd.Add.Members {
			member, err := parseCollectionMember(raw)
			if err != nil {
				return NewCLIError(ExitUsage, err)
			}
			members[i] = member
		}
		fmt.Fprintf(c.stderr, "adding %d member(s) to collection %s...\n", len(members), cmd.Add.Name)
		res, err := service.AddCollectionMembers(ctx, cmd.Add.Name, members)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Add.JSON, res)
	case "collection list":
		res, err := service.ListCollections(ctx)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.List.JSON, res)
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown collection command: %s", command))
	}
}

func parseCollectionMember(raw string) (CollectionMember, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return CollectionMember{}, errors.New("collection member cannot be empty")
	}

	var kind, ref string
	if idx := strings.IndexByte(raw, ':'); idx >= 0 {
		kind = strings.ToLower(strings.TrimSpace(raw[:idx]))
		ref = strings.TrimSpace(raw[idx+1:])
	} else {
		ref = raw
	}

	switch kind {
	case "", "repo", "repository":
		kind = "repository"
	case "issue", "issues":
		kind = "issue"
	case "pr", "pull_request", "pullrequest":
		kind = "pull_request"
	default:
		return CollectionMember{}, fmt.Errorf("unknown collection member kind %q", kind)
	}

	if kind == "repository" {
		if _, err := parseRepo(ref); err != nil {
			return CollectionMember{}, fmt.Errorf("invalid repository reference %q", ref)
		}
	} else {
		if err := parseCollectionThreadRef(ref); err != nil {
			return CollectionMember{}, err
		}
	}

	return CollectionMember{Kind: kind, Ref: ref}, nil
}

func parseCollectionThreadRef(ref string) error {
	parts := strings.Split(ref, "#")
	if len(parts) != 2 {
		return fmt.Errorf("invalid thread reference %q: expected OWNER/REPO#NUMBER", ref)
	}
	if _, err := parseRepo(parts[0]); err != nil {
		return fmt.Errorf("invalid thread reference %q: expected OWNER/REPO#NUMBER", ref)
	}
	if n, err := strconv.Atoi(strings.TrimSpace(parts[1])); err != nil || n <= 0 {
		return fmt.Errorf("invalid thread reference %q: expected positive number", ref)
	}
	return nil
}
