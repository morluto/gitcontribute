package cli_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

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

type coreOnlyService struct{}

func (coreOnlyService) Init(context.Context) (*cli.InitResult, error) { return nil, nil }
func (coreOnlyService) Status(context.Context) (*cli.StatusResult, error) {
	return nil, nil
}
func (coreOnlyService) Sync(context.Context, cli.RepoRef) (*cli.SyncResult, error) {
	return nil, nil
}
func (coreOnlyService) Search(context.Context, string, cli.SearchOptions) (*cli.SearchResult, error) {
	return nil, nil
}
func (coreOnlyService) Dossier(context.Context, cli.RepoRef) (*cli.DossierResult, error) {
	return nil, nil
}
func (coreOnlyService) Index(context.Context, cli.RepoRef, string) (*cli.IndexResult, error) {
	return nil, nil
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
