package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/health"
)

type fakeService struct {
	initCalled               bool
	statusCalled             bool
	syncCalled               bool
	syncPlanCalled           bool
	searchCalled             bool
	dossierCalled            bool
	indexCalled              bool
	acquireCalled            bool
	healthCalled             bool
	addSourceCalled          bool
	addRepoSourceCalled      bool
	addGHArchiveSourceCalled bool
	showSourceCalled         bool
	listSourcesCalled        bool
	crawlCalled              bool
	tailCalled               bool
	startInvCalled           bool
	showInvCalled            bool
	listInvCalled            bool
	addHypCalled             bool
	listHypCalled            bool
	promoteOppCalled         bool
	showOppCalled            bool
	listOppCalled            bool
	setStatusOppCalled       bool
	recordTriageCalled       bool
	listTriageCalled         bool
	recordContributionCalled bool
	getContributionCalled    bool
	listContributionsCalled  bool
	recordOutcomeCalled      bool
	listOutcomesCalled       bool
	exportMetadataCalled     bool
	importMetadataCalled     bool

	initResult         *cli.InitResult
	statusResult       *cli.StatusResult
	syncResult         *cli.SyncResult
	syncPlanResult     *cli.SyncPlanResult
	searchResult       *cli.SearchResult
	dossierResult      *cli.DossierResult
	indexResult        *cli.IndexResult
	acquisitionResult  *cli.AcquisitionResult
	healthResult       *health.Report
	sourceResult       *cli.SourceResult
	sourceListResult   *cli.SourceListResult
	crawlResult        *cli.CrawlResult
	startInvResult     *cli.InvestigationResult
	showInvResult      *cli.InvestigationResult
	listInvResult      *cli.InvestigationListResult
	addHypResult       *cli.HypothesisResult
	listHypResult      *cli.HypothesisListResult
	promoteOppResult   *cli.OpportunityResult
	showOppResult      *cli.OpportunityResult
	listOppResult      *cli.OpportunityListResult
	setStatusOppResult *cli.OpportunityResult

	triageEventResult             *cli.TriageEventResult
	triageEventListResult         *cli.TriageEventListResult
	contributionResult            *cli.ContributionResult
	contributionListResult        *cli.ContributionListResult
	contributionOutcomeResult     *cli.ContributionOutcomeResult
	contributionOutcomeListResult *cli.ContributionOutcomeListResult
	metadataExportResult          *cli.MetadataExportResult
	metadataImportResult          *cli.MetadataImportResult

	lastSyncArg    cli.RepoRef
	lastSearchArgs struct {
		Query string
		Opts  cli.SearchOptions
	}
	lastDossierArg     cli.RepoRef
	lastIndexRepo      cli.RepoRef
	setupResult        *cli.SetupReport
	lastSetup          cli.SetupOptions
	setupCalls         []cli.SetupOptions
	lastIndexPath      string
	lastAcquireRemote  string
	lastHealthOpts     health.Options
	lastSourceName     string
	lastSourceQuery    string
	lastSourceRefs     []cli.RepoRef
	lastSourceEvents   []string
	lastShowSourceName string
	lastCrawlName      string
	lastCrawlOpts      cli.CrawlOptions
	lastTailOpts       cli.TailOptions
	lastStartInvArgs   startInvArgs
	lastShowInvArg     string
	lastAddHypArgs     addHypArgs
	lastListHypArg     string
	lastPromoteArgs    promoteArgs
	lastShowOppArg     string
	lastListOppFilter  string
	lastSetStatusArgs  setStatusArgs

	lastRecordTriageArgs       cli.RecordTriageEventOptions
	lastListTriageArgs         cli.ListTriageEventsOptions
	lastRecordContributionArgs cli.RecordContributionOptions
	lastShowContributionArg    string
	lastListContributionsArgs  cli.ListContributionsOptions
	lastRecordOutcomeArgs      cli.RecordContributionOutcomeOptions
	lastListOutcomesArg        string
	lastExportMetadataArgs     cli.MetadataExportOptions
	lastImportMetadataArgs     cli.MetadataImportOptions

	err error
}

func (f *fakeService) Setup(_ context.Context, opts cli.SetupOptions) (*cli.SetupReport, error) {
	f.lastSetup = opts
	f.setupCalls = append(f.setupCalls, opts)
	if f.setupResult != nil {
		return f.setupResult, nil
	}
	return &cli.SetupReport{Operation: "setup", DryRun: opts.DryRun, Steps: []cli.SetupStep{{Name: "codex", Status: "configured"}}}, nil
}

type startInvArgs struct {
	Repo   cli.RepoRef
	Commit string
	Lens   string
}

type addHypArgs struct {
	InvestigationID string
	Title           string
	Description     string
	Category        string
}

type promoteArgs struct {
	HypothesisID string
	Problem      string
	Scope        string
	Impact       string
	Effort       string
	Confidence   float64
}

type setStatusArgs struct {
	ID        string
	Status    string
	Rationale string
}

func (f *fakeService) Init(ctx context.Context) (*cli.InitResult, error) {
	f.initCalled = true
	return f.initResult, f.err
}

func (f *fakeService) Status(ctx context.Context) (*cli.StatusResult, error) {
	f.statusCalled = true
	return f.statusResult, f.err
}

func (f *fakeService) Sync(ctx context.Context, repo cli.RepoRef) (*cli.SyncResult, error) {
	f.syncCalled = true
	f.lastSyncArg = repo
	return f.syncResult, f.err
}

func (f *fakeService) PlanArchiveSync(_ context.Context, _ cli.RepoRef, _ cli.ArchiveSyncOptions) (*cli.SyncPlanResult, error) {
	f.syncPlanCalled = true
	return f.syncPlanResult, f.err
}

func (f *fakeService) Search(ctx context.Context, query string, opts cli.SearchOptions) (*cli.SearchResult, error) {
	f.searchCalled = true
	f.lastSearchArgs.Query = query
	f.lastSearchArgs.Opts = opts
	return f.searchResult, f.err
}

func (f *fakeService) Dossier(ctx context.Context, repo cli.RepoRef) (*cli.DossierResult, error) {
	f.dossierCalled = true
	f.lastDossierArg = repo
	return f.dossierResult, f.err
}

func (f *fakeService) Index(ctx context.Context, repo cli.RepoRef, path string) (*cli.IndexResult, error) {
	f.indexCalled = true
	f.lastIndexRepo = repo
	f.lastIndexPath = path
	return f.indexResult, f.err
}

func (f *fakeService) Acquire(ctx context.Context, repo cli.RepoRef, remote string) (*cli.AcquisitionResult, error) {
	f.acquireCalled = true
	f.lastIndexRepo = repo
	f.lastAcquireRemote = remote
	return f.acquisitionResult, f.err
}

func (f *fakeService) RepositoryHealthWithOptions(ctx context.Context, repo cli.RepoRef, opts health.Options) (*health.Report, error) {
	f.healthCalled = true
	f.lastIndexRepo = repo
	f.lastHealthOpts = opts
	return f.healthResult, f.err
}

func (f *fakeService) AddSearchSource(ctx context.Context, name, query string) (*cli.SourceResult, error) {
	f.addSourceCalled = true
	f.lastSourceName = name
	f.lastSourceQuery = query
	return f.sourceResult, f.err
}

func (f *fakeService) AddRepoSource(ctx context.Context, name string, refs []cli.RepoRef) (*cli.SourceResult, error) {
	f.addRepoSourceCalled = true
	f.lastSourceName = name
	f.lastSourceRefs = refs
	return f.sourceResult, f.err
}

func (f *fakeService) AddGHArchiveSource(ctx context.Context, name string, events []string) (*cli.SourceResult, error) {
	f.addGHArchiveSourceCalled = true
	f.lastSourceName = name
	f.lastSourceEvents = events
	return f.sourceResult, f.err
}

func (f *fakeService) ShowSource(ctx context.Context, name string) (*cli.SourceResult, error) {
	f.showSourceCalled = true
	f.lastShowSourceName = name
	return f.sourceResult, f.err
}

func (f *fakeService) ListSources(ctx context.Context) (*cli.SourceListResult, error) {
	f.listSourcesCalled = true
	return f.sourceListResult, f.err
}

func (f *fakeService) Crawl(ctx context.Context, name string, opts cli.CrawlOptions) (*cli.CrawlResult, error) {
	f.crawlCalled = true
	f.lastCrawlName = name
	f.lastCrawlOpts = opts
	return f.crawlResult, f.err
}

func (f *fakeService) TailSource(ctx context.Context, name string, opts cli.TailOptions) (*cli.TailResult, error) {
	f.tailCalled = true
	f.lastCrawlName = name
	f.lastTailOpts = opts
	return &cli.TailResult{Source: name, Iterations: 1}, f.err
}

func (f *fakeService) StartInvestigation(ctx context.Context, repo cli.RepoRef, commit, lens string) (*cli.InvestigationResult, error) {
	f.startInvCalled = true
	f.lastStartInvArgs = startInvArgs{Repo: repo, Commit: commit, Lens: lens}
	return f.startInvResult, f.err
}

func (f *fakeService) ShowInvestigation(ctx context.Context, id string) (*cli.InvestigationResult, error) {
	f.showInvCalled = true
	f.lastShowInvArg = id
	return f.showInvResult, f.err
}

func (f *fakeService) ListInvestigations(ctx context.Context) (*cli.InvestigationListResult, error) {
	f.listInvCalled = true
	return f.listInvResult, f.err
}

func (f *fakeService) AddHypothesis(ctx context.Context, investigationID, title, description, category string) (*cli.HypothesisResult, error) {
	f.addHypCalled = true
	f.lastAddHypArgs = addHypArgs{InvestigationID: investigationID, Title: title, Description: description, Category: category}
	return f.addHypResult, f.err
}

func (f *fakeService) ListHypotheses(ctx context.Context, investigationID string) (*cli.HypothesisListResult, error) {
	f.listHypCalled = true
	f.lastListHypArg = investigationID
	return f.listHypResult, f.err
}

func (f *fakeService) PromoteOpportunity(ctx context.Context, hypothesisID, problem, scope, impact, effort string, confidence float64) (*cli.OpportunityResult, error) {
	f.promoteOppCalled = true
	f.lastPromoteArgs = promoteArgs{HypothesisID: hypothesisID, Problem: problem, Scope: scope, Impact: impact, Effort: effort, Confidence: confidence}
	return f.promoteOppResult, f.err
}

func (f *fakeService) ShowOpportunity(ctx context.Context, id string) (*cli.OpportunityResult, error) {
	f.showOppCalled = true
	f.lastShowOppArg = id
	return f.showOppResult, f.err
}

func (f *fakeService) ListOpportunities(ctx context.Context, investigationID string) (*cli.OpportunityListResult, error) {
	f.listOppCalled = true
	f.lastListOppFilter = investigationID
	return f.listOppResult, f.err
}

func (f *fakeService) SetOpportunityStatus(ctx context.Context, id, status, rationale string) (*cli.OpportunityResult, error) {
	f.setStatusOppCalled = true
	f.lastSetStatusArgs = setStatusArgs{ID: id, Status: status, Rationale: rationale}
	return f.setStatusOppResult, f.err
}

func (f *fakeService) RecordTriageEvent(ctx context.Context, opts cli.RecordTriageEventOptions) (*cli.TriageEventResult, error) {
	f.recordTriageCalled = true
	f.lastRecordTriageArgs = opts
	return f.triageEventResult, f.err
}

func (f *fakeService) ListTriageEvents(ctx context.Context, opts cli.ListTriageEventsOptions) (*cli.TriageEventListResult, error) {
	f.listTriageCalled = true
	f.lastListTriageArgs = opts
	return f.triageEventListResult, f.err
}

func (f *fakeService) RecordContribution(ctx context.Context, opts cli.RecordContributionOptions) (*cli.ContributionResult, error) {
	f.recordContributionCalled = true
	f.lastRecordContributionArgs = opts
	return f.contributionResult, f.err
}

func (f *fakeService) GetContribution(ctx context.Context, id string) (*cli.ContributionResult, error) {
	f.getContributionCalled = true
	f.lastShowContributionArg = id
	return f.contributionResult, f.err
}

func (f *fakeService) ListContributions(ctx context.Context, opts cli.ListContributionsOptions) (*cli.ContributionListResult, error) {
	f.listContributionsCalled = true
	f.lastListContributionsArgs = opts
	return f.contributionListResult, f.err
}

func (f *fakeService) RecordContributionOutcome(ctx context.Context, opts cli.RecordContributionOutcomeOptions) (*cli.ContributionOutcomeResult, error) {
	f.recordOutcomeCalled = true
	f.lastRecordOutcomeArgs = opts
	return f.contributionOutcomeResult, f.err
}

func (f *fakeService) ListContributionOutcomes(ctx context.Context, contributionID string) (*cli.ContributionOutcomeListResult, error) {
	f.listOutcomesCalled = true
	f.lastListOutcomesArg = contributionID
	return f.contributionOutcomeListResult, f.err
}

func (f *fakeService) ExportLocalMetadata(ctx context.Context, opts cli.MetadataExportOptions) (*cli.MetadataExportResult, error) {
	f.exportMetadataCalled = true
	f.lastExportMetadataArgs = opts
	return f.metadataExportResult, f.err
}

func (f *fakeService) ImportLocalMetadata(ctx context.Context, opts cli.MetadataImportOptions) (*cli.MetadataImportResult, error) {
	f.importMetadataCalled = true
	f.lastImportMetadataArgs = opts
	return f.metadataImportResult, f.err
}

func TestIndex(t *testing.T) {
	svc := &fakeService{indexResult: &cli.IndexResult{Repo: cli.RepoRef{Owner: "o", Repo: "r"}, Commit: "abc", Files: 2}}
	c, stdout, stderr := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"index", "o/r", "/checkout"}))
	if !svc.indexCalled || svc.lastIndexRepo.String() != "o/r" || svc.lastIndexPath != "/checkout" {
		t.Fatalf("index call = called:%v repo:%v path:%q", svc.indexCalled, svc.lastIndexRepo, svc.lastIndexPath)
	}
	if !strings.Contains(stdout.String(), "abc") || !strings.Contains(stderr.String(), "indexing") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestAcquire(t *testing.T) {
	svc := &fakeService{acquisitionResult: &cli.AcquisitionResult{
		Repo: cli.RepoRef{Owner: "o", Repo: "r"}, CommitSHA: "abc", Indexed: true,
	}}
	c, stdout, stderr := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"acquire", "o/r", "--remote", "https://example.test/o/r.git", "--json"}))
	if !svc.acquireCalled || svc.lastIndexRepo.String() != "o/r" || svc.lastAcquireRemote != "https://example.test/o/r.git" {
		t.Fatalf("acquire args: called=%v repo=%v remote=%q", svc.acquireCalled, svc.lastIndexRepo, svc.lastAcquireRemote)
	}
	if !strings.Contains(stdout.String(), `"commit_sha": "abc"`) || !strings.Contains(stderr.String(), "acquiring and indexing o/r") {
		t.Fatalf("stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestHealth(t *testing.T) {
	svc := &fakeService{healthResult: &health.Report{Repo: health.RepoRef{Owner: "o", Repo: "r"}}}
	c, stdout, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"health", "o/r", "--start", "2026-07-01T00:00:00Z", "--end", "2026-07-17T00:00:00Z", "--stale-after", "240h", "--json"}))
	if !svc.healthCalled || svc.lastIndexRepo.String() != "o/r" || svc.lastHealthOpts.StaleThreshold != 240*time.Hour || svc.lastHealthOpts.Start.IsZero() {
		t.Fatalf("health args: called=%v repo=%v opts=%+v", svc.healthCalled, svc.lastIndexRepo, svc.lastHealthOpts)
	}
	if !strings.Contains(stdout.String(), `"repo": {`) {
		t.Fatalf("stdout=%q", stdout)
	}
}

func TestSourceAddSearchAndList(t *testing.T) {
	svc := &fakeService{
		sourceResult:     &cli.SourceResult{Name: "active-go", Kind: "search", Definition: `{"query":"language:go"}`, Enabled: true},
		sourceListResult: &cli.SourceListResult{Sources: []cli.SourceResult{{Name: "active-go", Kind: "search", Enabled: true}}},
	}
	c, stdout, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"source", "add", "search", "--name", "active-go", "--query", "language:go"}))
	if !svc.addSourceCalled || svc.lastSourceName != "active-go" || svc.lastSourceQuery != "language:go" {
		t.Fatalf("add source call = called:%v name:%q query:%q", svc.addSourceCalled, svc.lastSourceName, svc.lastSourceQuery)
	}
	if !strings.Contains(stdout.String(), "Source active-go") {
		t.Fatalf("stdout = %q", stdout.String())
	}

	stdout.Reset()
	requireNoErr(t, c.Run(context.Background(), []string{"source", "list"}))
	if !svc.listSourcesCalled || !strings.Contains(stdout.String(), "active-go") {
		t.Fatalf("listed=%v stdout=%q", svc.listSourcesCalled, stdout.String())
	}
}

func TestSourceAddRepos(t *testing.T) {
	svc := &fakeService{
		sourceResult: &cli.SourceResult{Name: "golang-go", Kind: "repos", Definition: `{"repositories":[{"owner":"golang","repo":"go"}]}`, Enabled: true},
	}
	c, stdout, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"source", "add", "repos", "golang/go"}))
	if !svc.addRepoSourceCalled || svc.lastSourceName != "golang-go" || len(svc.lastSourceRefs) != 1 || svc.lastSourceRefs[0].String() != "golang/go" {
		t.Fatalf("add repo source call = called:%v name:%q refs:%+v", svc.addRepoSourceCalled, svc.lastSourceName, svc.lastSourceRefs)
	}
	if !strings.Contains(stdout.String(), "Source golang-go") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestSourceAddReposAcceptsURL(t *testing.T) {
	svc := &fakeService{sourceResult: &cli.SourceResult{Name: "x-y", Kind: "repos"}}
	c, _, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"source", "add", "repos", "https://github.com/X/Y"}))
	if len(svc.lastSourceRefs) != 1 || svc.lastSourceRefs[0].String() != "X/Y" {
		t.Fatalf("refs = %+v", svc.lastSourceRefs)
	}
}

func TestSourceAddReposImportsStructuredStdin(t *testing.T) {
	svc := &fakeService{sourceResult: &cli.SourceResult{Name: "imported", Kind: "repos", Enabled: true}}
	c, _, _ := newTestCLI(svc, nil)
	c.SetInput(strings.NewReader(`{"repositories":["one/first",{"owner":"two","repo":"second"},{"full_name":"three/third"}]}`))
	requireNoErr(t, c.Run(context.Background(), []string{"source", "add", "repos", "--name", "imported", "--file", "-"}))
	if len(svc.lastSourceRefs) != 3 || svc.lastSourceRefs[1].String() != "two/second" {
		t.Fatalf("refs = %+v", svc.lastSourceRefs)
	}
}

func TestSourceAddReposImportsLineFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repositories.txt")
	if err := os.WriteFile(path, []byte("# favorites\none/first\nhttps://github.com/two/second\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := &fakeService{sourceResult: &cli.SourceResult{Name: "imported", Kind: "repos", Enabled: true}}
	c, _, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"source", "add", "repos", "--name", "imported", "--file", path}))
	if len(svc.lastSourceRefs) != 2 || svc.lastSourceRefs[1].String() != "two/second" {
		t.Fatalf("refs = %+v", svc.lastSourceRefs)
	}
}

func TestSourceAddReposRejectsInvalidURL(t *testing.T) {
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)
	err := c.Run(context.Background(), []string{"source", "add", "repos", "https://gitlab.com/X/Y"})
	requireCLIError(t, err, cli.ExitUsage)
	if svc.addRepoSourceCalled {
		t.Fatal("add repo source should not be called for invalid URL")
	}
}

func TestSourceAddGHArchive(t *testing.T) {
	svc := &fakeService{
		sourceResult: &cli.SourceResult{Name: "gharchive", Kind: "gharchive", Definition: `{"events":["PushEvent","IssuesEvent"]}`, Enabled: true},
	}
	c, stdout, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"source", "add", "gharchive", "--events", "PushEvent,IssuesEvent"}))
	if !svc.addGHArchiveSourceCalled || svc.lastSourceName != "gharchive" || len(svc.lastSourceEvents) != 2 {
		t.Fatalf("add gharchive call = called:%v name:%q events:%+v", svc.addGHArchiveSourceCalled, svc.lastSourceName, svc.lastSourceEvents)
	}
	if !strings.Contains(stdout.String(), "Source gharchive") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestSourceAddGHArchiveRejectsUnknownEvent(t *testing.T) {
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)
	err := c.Run(context.Background(), []string{"source", "add", "gharchive", "--events", "PushEvent,UnknownEvent"})
	requireCLIError(t, err, cli.ExitUsage)
	if svc.addGHArchiveSourceCalled {
		t.Fatal("add gharchive source should not be called for invalid event")
	}
}

func TestSourceShow(t *testing.T) {
	svc := &fakeService{sourceResult: &cli.SourceResult{Name: "gharchive", Kind: "gharchive", Definition: `{}`, Enabled: true}}
	c, stdout, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"source", "show", "gharchive"}))
	if !svc.showSourceCalled || svc.lastShowSourceName != "gharchive" {
		t.Fatalf("show source not called correctly: called=%v arg=%q", svc.showSourceCalled, svc.lastShowSourceName)
	}
	if !strings.Contains(stdout.String(), "gharchive") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestCrawlDispatchesBoundedOptions(t *testing.T) {
	svc := &fakeService{crawlResult: &cli.CrawlResult{
		Source: "active-go", Windows: 2, Repositories: 7, Requests: 4, Checkpoint: "2026-07-16T00:00:00Z",
	}}
	c, stdout, stderr := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"crawl", "active-go", "--since", "48h", "--budget", "25"}))
	if !svc.crawlCalled || svc.lastCrawlName != "active-go" || svc.lastCrawlOpts.Since != 48*time.Hour || svc.lastCrawlOpts.Budget != 25 {
		t.Fatalf("crawl call = called:%v name:%q opts:%+v", svc.crawlCalled, svc.lastCrawlName, svc.lastCrawlOpts)
	}
	if !strings.Contains(stdout.String(), "7 repositories") || !strings.Contains(stderr.String(), "crawling active-go") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestCrawlRejectsInvalidBudgetBeforeDispatch(t *testing.T) {
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)
	err := c.Run(context.Background(), []string{"crawl", "active-go", "--budget", "5001"})
	requireCLIError(t, err, cli.ExitUsage)
	if svc.crawlCalled {
		t.Fatal("crawl should not be called with an invalid budget")
	}
}

func TestTailDispatchesBoundedOptions(t *testing.T) {
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"tail", "events", "--since", "3h", "--interval", "10m", "--budget", "25", "--once"}))
	if !svc.tailCalled || svc.lastCrawlName != "events" {
		t.Fatalf("tail not called: %+v", svc)
	}
	if svc.lastTailOpts.Since != 3*time.Hour || svc.lastTailOpts.Interval != 10*time.Minute || svc.lastTailOpts.Budget != 25 || !svc.lastTailOpts.Once {
		t.Fatalf("tail options = %+v", svc.lastTailOpts)
	}
}

type fakeMCPRunner struct {
	called bool
	opts   cli.MCPOptions
	err    error
}

func (f *fakeMCPRunner) Run(ctx context.Context, opts cli.MCPOptions) error {
	f.called = true
	f.opts = opts
	return f.err
}

func newTestCLI(svc cli.Service, runner cli.MCPRunner) (*cli.CLI, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	return cli.New(svc, runner, &stdout, &stderr), &stdout, &stderr
}

func requireNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func requireCLIError(t *testing.T, err error, wantCode int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected CLIError with code %d, got nil", wantCode)
	}
	var ce *cli.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *CLIError, got %T: %v", err, err)
	}
	if ce.Code != wantCode {
		t.Fatalf("exit code=%d, want %d", ce.Code, wantCode)
	}
}

func TestInit(t *testing.T) {
	svc := &fakeService{initResult: &cli.InitResult{Path: "/tmp/gc", Message: "ready"}}
	c, stdout, stderr := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"init"})
	requireNoErr(t, err)

	if !svc.initCalled {
		t.Fatal("Init was not called")
	}
	want := "Initialized corpus at /tmp/gc.\nready\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout=%q, want %q", got, want)
	}
	if got := stderr.String(); got != "initializing...\n" {
		t.Fatalf("stderr=%q, want progress message", got)
	}
}

func TestInitJSON(t *testing.T) {
	svc := &fakeService{initResult: &cli.InitResult{Path: "/tmp/gc", Message: "ready"}}
	c, stdout, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"init", "--json"})
	requireNoErr(t, err)

	var got cli.InitResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}
	if got.Path != "/tmp/gc" || got.Message != "ready" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestStatus(t *testing.T) {
	svc := &fakeService{statusResult: &cli.StatusResult{Healthy: true, Corpus: "gc", Version: "0.0.1", Message: "ok"}}
	c, stdout, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"status"})
	requireNoErr(t, err)

	if !svc.statusCalled {
		t.Fatal("Status was not called")
	}
	want := "Status: healthy (corpus=gc version=0.0.1)\nok\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout=%q, want %q", got, want)
	}
}

func TestSyncInvalidRepo(t *testing.T) {
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"sync", "invalid"})
	requireCLIError(t, err, cli.ExitUsage)

	if svc.syncCalled {
		t.Fatal("Sync should not be called with invalid repo")
	}
}

func TestSearchDefaults(t *testing.T) {
	svc := &fakeService{searchResult: &cli.SearchResult{
		Query: "test",
		Kind:  "all",
		Limit: 20,
		Total: 1,
		Matches: []cli.SearchMatch{{
			Kind:   "issue",
			Repo:   cli.RepoRef{Owner: "o", Repo: "r"},
			Title:  "foo",
			Number: 42,
			Score:  0.9,
		}},
	}}
	c, stdout, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"search", "test"})
	requireNoErr(t, err)

	if !svc.searchCalled {
		t.Fatal("Search was not called")
	}
	if svc.lastSearchArgs.Query != "test" {
		t.Fatalf("query=%q, want test", svc.lastSearchArgs.Query)
	}
	if svc.lastSearchArgs.Opts.Kind != "all" {
		t.Fatalf("kind=%q, want all", svc.lastSearchArgs.Opts.Kind)
	}
	if svc.lastSearchArgs.Opts.Limit != 20 {
		t.Fatalf("limit=%d, want 20", svc.lastSearchArgs.Opts.Limit)
	}
	want := "Search: test (kind=all, limit=20)\n1 matches:\n- issue o/r#42: foo (0.90)\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout=%q, want %q", got, want)
	}
}

func TestSearchJSONWithFlags(t *testing.T) {
	svc := &fakeService{searchResult: &cli.SearchResult{
		Query:   "good first issue",
		Kind:    "issues",
		Repo:    "o/r",
		Limit:   5,
		Total:   0,
		Matches: []cli.SearchMatch{},
	}}
	c, stdout, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"search", "issues", "good first issue", "--repo", "o/r", "--state", "open", "--author", "alice", "--label", "bug", "--updated-after", "2026-07-01T00:00:00Z", "--limit", "5", "--cursor", "next-page", "--json"})
	requireNoErr(t, err)

	if !svc.searchCalled {
		t.Fatal("Search was not called")
	}
	if svc.lastSearchArgs.Query != "good first issue" {
		t.Fatalf("query=%q", svc.lastSearchArgs.Query)
	}
	opts := svc.lastSearchArgs.Opts
	if opts.Kind != "issues" || opts.Repo != "o/r" || opts.State != "open" || opts.Author != "alice" || len(opts.Labels) != 1 || opts.Labels[0] != "bug" || opts.UpdatedAfter.Format(time.RFC3339) != "2026-07-01T00:00:00Z" || opts.Limit != 5 || opts.Cursor != "next-page" {
		t.Fatalf("unexpected options: %+v", opts)
	}

	var got cli.SearchResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}
	if got.Query != "good first issue" || got.Kind != "issues" || got.Repo != "o/r" || got.Limit != 5 {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestSearchNoNetworkImplied(t *testing.T) {
	// Search must be local; the CLI dispatches to the injected service without
	// any hidden network access.
	svc := &fakeService{searchResult: &cli.SearchResult{Query: "local", Total: 0, Matches: []cli.SearchMatch{}}}
	c, _, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"search", "local"})
	requireNoErr(t, err)
	if !svc.searchCalled {
		t.Fatal("Search was not called")
	}
}

func TestSearchInvalidLimit(t *testing.T) {
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"search", "x", "--limit", "0"})
	requireCLIError(t, err, cli.ExitUsage)
}

func TestSearchInvalidRepoFilter(t *testing.T) {
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"search", "x", "--repo", "bad"})
	requireCLIError(t, err, cli.ExitUsage)
}

func TestSearchRejectsUnsupportedFilterCombinations(t *testing.T) {
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)
	for _, args := range [][]string{
		{"search", "all", "x", "--cursor", "cursor"},
		{"search", "code", "x", "--state", "open"},
		{"search", "repos", "x", "--association", "OWNER"},
		{"search", "code", "x", "--assignee", "alice"},
	} {
		requireCLIError(t, c.Run(context.Background(), args), cli.ExitUsage)
	}
	if svc.searchCalled {
		t.Fatal("search should not be called for unsupported filter combinations")
	}
}

func TestSearchThreadMetadataFlags(t *testing.T) {
	svc := &fakeService{searchResult: &cli.SearchResult{
		Query:   "term",
		Kind:    "issues",
		Limit:   10,
		Total:   0,
		Matches: []cli.SearchMatch{},
	}}
	c, _, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"search", "issues", "term", "--association", "OWNER", "--assignee", "alice"})
	requireNoErr(t, err)

	if !svc.searchCalled {
		t.Fatal("Search was not called")
	}
	opts := svc.lastSearchArgs.Opts
	if opts.Association != "OWNER" {
		t.Fatalf("association = %q, want OWNER", opts.Association)
	}
	if opts.Assignee != "alice" {
		t.Fatalf("assignee = %q, want alice", opts.Assignee)
	}
}

func TestSearchWithLensFlag(t *testing.T) {
	svc := &fakeService{searchResult: &cli.SearchResult{
		Query:   "test",
		Kind:    "issues",
		Limit:   10,
		Total:   1,
		Matches: []cli.SearchMatch{},
	}}
	c, _, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"search", "issues", "test", "--lens", "active-go", "--limit", "10"})
	requireNoErr(t, err)

	if !svc.searchCalled {
		t.Fatal("Search was not called")
	}
	opts := svc.lastSearchArgs.Opts
	if opts.Lens != "active-go" {
		t.Fatalf("lens = %q, want active-go", opts.Lens)
	}
	if opts.Kind != "issues" || opts.Limit != 10 {
		t.Fatalf("unexpected options: %+v", opts)
	}
}

func TestSearchRejectsLensWithCursor(t *testing.T) {
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)
	err := c.Run(context.Background(), []string{"search", "issues", "test", "--lens", "active-go", "--cursor", "abc"})
	requireCLIError(t, err, cli.ExitUsage)
	if svc.searchCalled {
		t.Fatal("search should not be called when lens and cursor are combined")
	}
}

func TestDossier(t *testing.T) {
	svc := &fakeService{dossierResult: &cli.DossierResult{
		Repo:       cli.RepoRef{Owner: "o", Repo: "r"},
		Summary:    "A Go CLI",
		Language:   "Go",
		Stars:      100,
		OpenIssues: 5,
		Coverage:   []string{"metadata", "threads"},
		Freshness:  "2026-07-16T00:00:00Z",
	}}
	c, stdout, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"dossier", "o/r"})
	requireNoErr(t, err)

	if !svc.dossierCalled {
		t.Fatal("Dossier was not called")
	}
	if svc.lastDossierArg != (cli.RepoRef{Owner: "o", Repo: "r"}) {
		t.Fatalf("dossier repo=%+v", svc.lastDossierArg)
	}
	want := "Dossier: o/r\nSummary: A Go CLI\nLanguage: Go\nStars: 100\nOpen issues: 5\nCoverage: metadata, threads\nFreshness: 2026-07-16T00:00:00Z\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout=%q, want %q", got, want)
	}
}

func TestDossierJSON(t *testing.T) {
	svc := &fakeService{dossierResult: &cli.DossierResult{
		Repo:       cli.RepoRef{Owner: "o", Repo: "r"},
		Summary:    "A Go CLI",
		Language:   "Go",
		Stars:      100,
		OpenIssues: 5,
		Coverage:   []string{"metadata"},
		Freshness:  "now",
	}}
	c, stdout, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"dossier", "o/r", "--json"})
	requireNoErr(t, err)

	var got cli.DossierResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}
	if got.Repo.Owner != "o" || got.Summary != "A Go CLI" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestMCP(t *testing.T) {
	runner := &fakeMCPRunner{}
	c, stdout, stderr := newTestCLI(nil, runner)

	err := c.Run(context.Background(), []string{"mcp"})
	requireNoErr(t, err)

	if !runner.called {
		t.Fatal("MCP Run was not called")
	}
	if runner.opts.Transport != "stdio" {
		t.Fatalf("transport=%q, want stdio", runner.opts.Transport)
	}
	if stdout.String() != "" {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if stderr.String() != "starting mcp server (transport=stdio)...\n" {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestContextCancellation(t *testing.T) {
	svc := &fakeService{err: context.Canceled}
	c, _, _ := newTestCLI(svc, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := c.Run(ctx, []string{"status"})
	requireCLIError(t, err, cli.ExitCancelled)
}

func TestUnknownCommand(t *testing.T) {
	c, _, _ := newTestCLI(&fakeService{}, nil)
	err := c.Run(context.Background(), []string{"nope"})
	requireCLIError(t, err, cli.ExitUsage)
}

func TestSetupNonInteractiveJSON(t *testing.T) {
	svc := &fakeService{setupResult: &cli.SetupReport{Operation: "setup", DryRun: true, MCPCommand: &cli.SetupMCPCommand{Command: "/managed/gitcontribute", Args: []string{"mcp", "serve", "--transport=stdio"}}, Steps: []cli.SetupStep{{Name: "codex", Status: "would configure"}}}}
	var out bytes.Buffer
	c := cli.New(svc, &fakeMCPRunner{}, &out, io.Discard)
	if err := c.Run(context.Background(), []string{"setup", "--mode", "mcp", "--codex", "--token-source", "none", "--dry-run", "--json"}); err != nil {
		t.Fatal(err)
	}
	if len(svc.lastSetup.Clients) != 1 || svc.lastSetup.Clients[0] != "codex" || !svc.lastSetup.DryRun {
		t.Fatalf("options = %+v", svc.lastSetup)
	}
	if !strings.Contains(out.String(), `"would configure"`) {
		t.Fatalf("output = %s", out.String())
	}
}

func TestInvestigationStartShowAndList(t *testing.T) {
	svc := &fakeService{
		startInvResult: &cli.InvestigationResult{
			ID: "inv-1", Repo: cli.RepoRef{Owner: "o", Repo: "r"},
			CommitSHA: "abc", Lens: "go", Status: "open",
			CreatedAt: "2026-07-17T00:00:00Z", UpdatedAt: "2026-07-17T00:00:00Z",
		},
		showInvResult: &cli.InvestigationResult{
			ID: "inv-1", Repo: cli.RepoRef{Owner: "o", Repo: "r"},
			Status: "open", CreatedAt: "2026-07-17T00:00:00Z", UpdatedAt: "2026-07-17T00:00:00Z",
		},
		listInvResult: &cli.InvestigationListResult{
			Investigations: []cli.InvestigationResult{
				{ID: "inv-1", Repo: cli.RepoRef{Owner: "o", Repo: "r"}, Status: "open", CreatedAt: "2026-07-17T00:00:00Z", UpdatedAt: "2026-07-17T00:00:00Z"},
			},
		},
	}
	c, stdout, stderr := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"investigation", "start", "o/r", "--commit", "abc", "--lens", "go"})
	requireNoErr(t, err)
	if !svc.startInvCalled || svc.lastStartInvArgs.Repo.String() != "o/r" || svc.lastStartInvArgs.Commit != "abc" || svc.lastStartInvArgs.Lens != "go" {
		t.Fatalf("start investigation args = %+v", svc.lastStartInvArgs)
	}
	if !strings.Contains(stdout.String(), "inv-1") || !strings.Contains(stderr.String(), "starting investigation") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	stdout.Reset()
	err = c.Run(context.Background(), []string{"investigation", "show", "inv-1"})
	requireNoErr(t, err)
	if !svc.showInvCalled || svc.lastShowInvArg != "inv-1" {
		t.Fatalf("show investigation not called correctly: called=%v arg=%q", svc.showInvCalled, svc.lastShowInvArg)
	}
	if !strings.Contains(stdout.String(), "inv-1") {
		t.Fatalf("stdout=%q", stdout.String())
	}

	stdout.Reset()
	err = c.Run(context.Background(), []string{"investigation", "list"})
	requireNoErr(t, err)
	if !svc.listInvCalled {
		t.Fatal("list investigations not called")
	}
	if !strings.Contains(stdout.String(), "1 investigation") {
		t.Fatalf("stdout=%q", stdout.String())
	}
}

func TestInvestigationStartRejectsInvalidRepo(t *testing.T) {
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)
	err := c.Run(context.Background(), []string{"investigation", "start", "bad"})
	requireCLIError(t, err, cli.ExitUsage)
	if svc.startInvCalled {
		t.Fatal("start investigation should not be called with invalid repo")
	}
}

func TestHypothesisAddAndList(t *testing.T) {
	svc := &fakeService{
		addHypResult: &cli.HypothesisResult{
			ID: "hyp-1", InvestigationID: "inv-1", Title: "race", Description: "race desc", Category: "bug", Status: "proposed",
			CreatedAt: "2026-07-17T00:00:00Z", UpdatedAt: "2026-07-17T00:00:00Z",
		},
		listHypResult: &cli.HypothesisListResult{
			Hypotheses: []cli.HypothesisResult{
				{ID: "hyp-1", InvestigationID: "inv-1", Title: "race", Category: "bug", Status: "proposed"},
			},
		},
	}
	c, stdout, stderr := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"hypothesis", "add", "inv-1", "--title", "race", "--description", "race desc", "--category", "bug"})
	requireNoErr(t, err)
	if !svc.addHypCalled || svc.lastAddHypArgs.InvestigationID != "inv-1" || svc.lastAddHypArgs.Title != "race" || svc.lastAddHypArgs.Category != "bug" {
		t.Fatalf("add hypothesis args = %+v", svc.lastAddHypArgs)
	}
	if !strings.Contains(stdout.String(), "hyp-1") || !strings.Contains(stderr.String(), "recording hypothesis") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	stdout.Reset()
	err = c.Run(context.Background(), []string{"hypothesis", "list", "inv-1"})
	requireNoErr(t, err)
	if !svc.listHypCalled || svc.lastListHypArg != "inv-1" {
		t.Fatalf("list hypotheses not called correctly: called=%v arg=%q", svc.listHypCalled, svc.lastListHypArg)
	}
	if !strings.Contains(stdout.String(), "1 hypothesis") {
		t.Fatalf("stdout=%q", stdout.String())
	}
}

func TestOpportunityPromoteShowListAndSetStatus(t *testing.T) {
	svc := &fakeService{
		promoteOppResult: &cli.OpportunityResult{
			ID: "opp-1", InvestigationID: "inv-1", HypothesisID: "hyp-1", Title: "race",
			ProblemStatement: "data race", Scope: "pkg/foo", Impact: "crash", ExpectedEffort: "small",
			Confidence: 0.8, Category: "bug", CollisionStatus: "unknown", Status: "hypothesis",
			CreatedAt: "2026-07-17T00:00:00Z", UpdatedAt: "2026-07-17T00:00:00Z",
		},
		showOppResult: &cli.OpportunityResult{
			ID: "opp-1", Title: "race", Status: "reproduced", Confidence: 0.8,
			CreatedAt: "2026-07-17T00:00:00Z", UpdatedAt: "2026-07-17T00:01:00Z",
		},
		listOppResult: &cli.OpportunityListResult{
			Filter: "inv-1",
			Opportunities: []cli.OpportunityResult{
				{ID: "opp-1", Title: "race", Status: "reproduced", Confidence: 0.8, Category: "bug"},
			},
		},
		setStatusOppResult: &cli.OpportunityResult{
			ID: "opp-1", Title: "race", Status: "reproduced",
		},
	}
	c, stdout, stderr := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{
		"opportunity", "promote", "hyp-1",
		"--problem", "data race", "--scope", "pkg/foo", "--impact", "crash",
		"--effort", "small", "--confidence", "0.8",
	})
	requireNoErr(t, err)
	if !svc.promoteOppCalled || svc.lastPromoteArgs.HypothesisID != "hyp-1" || svc.lastPromoteArgs.Confidence != 0.8 {
		t.Fatalf("promote opportunity args = %+v", svc.lastPromoteArgs)
	}
	if !strings.Contains(stdout.String(), "opp-1") || !strings.Contains(stderr.String(), "promoting hypothesis") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	stdout.Reset()
	err = c.Run(context.Background(), []string{"opportunity", "show", "opp-1"})
	requireNoErr(t, err)
	if !svc.showOppCalled || svc.lastShowOppArg != "opp-1" {
		t.Fatalf("show opportunity not called correctly: called=%v arg=%q", svc.showOppCalled, svc.lastShowOppArg)
	}

	stdout.Reset()
	err = c.Run(context.Background(), []string{"opportunity", "list", "--investigation", "inv-1"})
	requireNoErr(t, err)
	if !svc.listOppCalled || svc.lastListOppFilter != "inv-1" {
		t.Fatalf("list opportunities filter = %q", svc.lastListOppFilter)
	}
	if !strings.Contains(stdout.String(), "1 opportunity") || !strings.Contains(stdout.String(), "(filter: inv-1)") {
		t.Fatalf("stdout=%q", stdout.String())
	}

	stdout.Reset()
	err = c.Run(context.Background(), []string{"opportunity", "set-status", "opp-1", "reproduced", "--rationale", "base branch fails"})
	requireNoErr(t, err)
	if !svc.setStatusOppCalled || svc.lastSetStatusArgs.ID != "opp-1" || svc.lastSetStatusArgs.Status != "reproduced" || svc.lastSetStatusArgs.Rationale != "base branch fails" {
		t.Fatalf("set-status args = %+v", svc.lastSetStatusArgs)
	}
}

func TestInvestigationCommandRequiresService(t *testing.T) {
	c, _, _ := newTestCLI(cli.NewBootstrapService(), nil)
	err := c.Run(context.Background(), []string{"investigation", "list"})
	requireCLIError(t, err, cli.ExitNotWired)
}
