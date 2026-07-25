package cli_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/health"
)

type fakeService struct {
	initCalled               bool
	statusCalled             bool
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

	initResult         *contracts.InitResult
	statusResult       *contracts.StatusResult
	syncPlanResult     *contracts.SyncPlanResult
	searchResult       *contracts.SearchResult
	dossierResult      *contracts.DossierResult
	indexResult        *contracts.IndexResult
	acquisitionResult  *contracts.AcquisitionResult
	healthResult       *health.Report
	sourceResult       *contracts.SourceResult
	sourceListResult   *contracts.SourceListResult
	crawlResult        *contracts.CrawlResult
	startInvResult     *contracts.InvestigationResult
	showInvResult      *contracts.InvestigationResult
	listInvResult      *contracts.InvestigationListResult
	addHypResult       *contracts.HypothesisResult
	listHypResult      *contracts.HypothesisListResult
	promoteOppResult   *contracts.OpportunityResult
	showOppResult      *contracts.OpportunityResult
	listOppResult      *contracts.OpportunityListResult
	setStatusOppResult *contracts.OpportunityResult

	triageEventResult             *contracts.TriageEventResult
	triageEventListResult         *contracts.TriageEventListResult
	contributionResult            *contracts.ContributionResult
	contributionListResult        *contracts.ContributionListResult
	contributionOutcomeResult     *contracts.ContributionOutcomeResult
	contributionOutcomeListResult *contracts.ContributionOutcomeListResult
	metadataExportResult          *contracts.MetadataExportResult
	metadataImportResult          *contracts.MetadataImportResult

	lastSearchArgs struct {
		Query string
		Opts  contracts.SearchOptions
	}
	lastDossierArg     contracts.RepoRef
	lastIndexRepo      contracts.RepoRef
	setupResult        *contracts.SetupReport
	lastSetup          contracts.SetupOptions
	setupCalls         []contracts.SetupOptions
	lastIndexPath      string
	lastAcquireRemote  string
	lastHealthOpts     health.Options
	lastSourceName     string
	lastSourceQuery    string
	lastSourceRefs     []contracts.RepoRef
	lastSourceEvents   []string
	lastShowSourceName string
	lastCrawlName      string
	lastCrawlOpts      contracts.CrawlOptions
	lastTailOpts       contracts.TailOptions
	lastStartInvArgs   startInvArgs
	lastShowInvArg     string
	lastAddHypArgs     addHypArgs
	lastListHypArg     string
	lastPromoteArgs    promoteArgs
	lastShowOppArg     string
	lastListOppFilter  string
	lastSetStatusArgs  setStatusArgs

	lastRecordTriageArgs       contracts.RecordTriageEventOptions
	lastListTriageArgs         contracts.ListTriageEventsOptions
	lastRecordContributionArgs contracts.RecordContributionOptions
	lastShowContributionArg    string
	lastListContributionsArgs  contracts.ListContributionsOptions
	lastRecordOutcomeArgs      contracts.RecordContributionOutcomeOptions
	lastListOutcomesArg        string
	lastExportMetadataArgs     contracts.MetadataExportOptions
	lastImportMetadataArgs     contracts.MetadataImportOptions

	err error
}

type coreOnlyService struct{}

func (coreOnlyService) Init(context.Context) (*contracts.InitResult, error) { return nil, nil }
func (coreOnlyService) Status(context.Context) (*contracts.StatusResult, error) {
	return nil, nil
}
func (coreOnlyService) Search(context.Context, string, contracts.SearchOptions) (*contracts.SearchResult, error) {
	return nil, nil
}
func (coreOnlyService) Dossier(context.Context, contracts.RepoRef) (*contracts.DossierResult, error) {
	return nil, nil
}
func (coreOnlyService) Index(context.Context, contracts.RepoRef, string) (*contracts.IndexResult, error) {
	return nil, nil
}

type startInvArgs struct {
	Repo   contracts.RepoRef
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

func (f *fakeService) Init(ctx context.Context) (*contracts.InitResult, error) {
	f.initCalled = true
	return f.initResult, f.err
}

func (f *fakeService) Status(ctx context.Context) (*contracts.StatusResult, error) {
	f.statusCalled = true
	return f.statusResult, f.err
}

func (f *fakeService) PlanArchiveSync(_ context.Context, _ contracts.RepoRef, _ contracts.ArchiveSyncOptions) (*contracts.SyncPlanResult, error) {
	f.syncPlanCalled = true
	return f.syncPlanResult, f.err
}

func (f *fakeService) Search(ctx context.Context, query string, opts contracts.SearchOptions) (*contracts.SearchResult, error) {
	f.searchCalled = true
	f.lastSearchArgs.Query = query
	f.lastSearchArgs.Opts = opts
	return f.searchResult, f.err
}

func (f *fakeService) Dossier(ctx context.Context, repo contracts.RepoRef) (*contracts.DossierResult, error) {
	f.dossierCalled = true
	f.lastDossierArg = repo
	return f.dossierResult, f.err
}

func (f *fakeService) Index(ctx context.Context, repo contracts.RepoRef, path string) (*contracts.IndexResult, error) {
	f.indexCalled = true
	f.lastIndexRepo = repo
	f.lastIndexPath = path
	return f.indexResult, f.err
}

func (f *fakeService) Acquire(ctx context.Context, repo contracts.RepoRef, remote string) (*contracts.AcquisitionResult, error) {
	f.acquireCalled = true
	f.lastIndexRepo = repo
	f.lastAcquireRemote = remote
	return f.acquisitionResult, f.err
}

func (f *fakeService) RepositoryHealthWithOptions(ctx context.Context, repo contracts.RepoRef, opts health.Options) (*health.Report, error) {
	f.healthCalled = true
	f.lastIndexRepo = repo
	f.lastHealthOpts = opts
	return f.healthResult, f.err
}

func (f *fakeService) AddSearchSource(ctx context.Context, name, query string) (*contracts.SourceResult, error) {
	f.addSourceCalled = true
	f.lastSourceName = name
	f.lastSourceQuery = query
	return f.sourceResult, f.err
}

func (f *fakeService) AddRepoSource(ctx context.Context, name string, refs []contracts.RepoRef) (*contracts.SourceResult, error) {
	f.addRepoSourceCalled = true
	f.lastSourceName = name
	f.lastSourceRefs = refs
	return f.sourceResult, f.err
}

func (f *fakeService) AddGHArchiveSource(ctx context.Context, name string, events []string) (*contracts.SourceResult, error) {
	f.addGHArchiveSourceCalled = true
	f.lastSourceName = name
	f.lastSourceEvents = events
	return f.sourceResult, f.err
}

func (f *fakeService) ShowSource(ctx context.Context, name string) (*contracts.SourceResult, error) {
	f.showSourceCalled = true
	f.lastShowSourceName = name
	return f.sourceResult, f.err
}

func (f *fakeService) ListSources(ctx context.Context) (*contracts.SourceListResult, error) {
	f.listSourcesCalled = true
	return f.sourceListResult, f.err
}

func (f *fakeService) Crawl(ctx context.Context, name string, opts contracts.CrawlOptions) (*contracts.CrawlResult, error) {
	f.crawlCalled = true
	f.lastCrawlName = name
	f.lastCrawlOpts = opts
	return f.crawlResult, f.err
}

func (f *fakeService) TailSource(ctx context.Context, name string, opts contracts.TailOptions) (*contracts.TailResult, error) {
	f.tailCalled = true
	f.lastCrawlName = name
	f.lastTailOpts = opts
	return &contracts.TailResult{Source: name, Iterations: 1}, f.err
}

func (f *fakeService) StartInvestigation(ctx context.Context, repo contracts.RepoRef, commit, lens string) (*contracts.InvestigationResult, error) {
	f.startInvCalled = true
	f.lastStartInvArgs = startInvArgs{Repo: repo, Commit: commit, Lens: lens}
	return f.startInvResult, f.err
}

func (f *fakeService) ShowInvestigation(ctx context.Context, id string) (*contracts.InvestigationResult, error) {
	f.showInvCalled = true
	f.lastShowInvArg = id
	return f.showInvResult, f.err
}

func (f *fakeService) ListInvestigations(ctx context.Context) (*contracts.InvestigationListResult, error) {
	f.listInvCalled = true
	return f.listInvResult, f.err
}

func (f *fakeService) AddHypothesis(ctx context.Context, investigationID, title, description, category string) (*contracts.HypothesisResult, error) {
	f.addHypCalled = true
	f.lastAddHypArgs = addHypArgs{InvestigationID: investigationID, Title: title, Description: description, Category: category}
	return f.addHypResult, f.err
}

func (f *fakeService) ListHypotheses(ctx context.Context, investigationID string) (*contracts.HypothesisListResult, error) {
	f.listHypCalled = true
	f.lastListHypArg = investigationID
	return f.listHypResult, f.err
}

func (f *fakeService) PromoteOpportunity(ctx context.Context, hypothesisID, problem, scope, impact, effort string, confidence float64) (*contracts.OpportunityResult, error) {
	f.promoteOppCalled = true
	f.lastPromoteArgs = promoteArgs{HypothesisID: hypothesisID, Problem: problem, Scope: scope, Impact: impact, Effort: effort, Confidence: confidence}
	return f.promoteOppResult, f.err
}

func (f *fakeService) ShowOpportunity(ctx context.Context, id string) (*contracts.OpportunityResult, error) {
	f.showOppCalled = true
	f.lastShowOppArg = id
	return f.showOppResult, f.err
}

func (f *fakeService) ListOpportunities(ctx context.Context, investigationID string) (*contracts.OpportunityListResult, error) {
	f.listOppCalled = true
	f.lastListOppFilter = investigationID
	return f.listOppResult, f.err
}

func (f *fakeService) SetOpportunityStatus(ctx context.Context, id, status, rationale string) (*contracts.OpportunityResult, error) {
	f.setStatusOppCalled = true
	f.lastSetStatusArgs = setStatusArgs{ID: id, Status: status, Rationale: rationale}
	return f.setStatusOppResult, f.err
}

func (f *fakeService) RecordTriageEvent(ctx context.Context, opts contracts.RecordTriageEventOptions) (*contracts.TriageEventResult, error) {
	f.recordTriageCalled = true
	f.lastRecordTriageArgs = opts
	return f.triageEventResult, f.err
}

func (f *fakeService) ListTriageEvents(ctx context.Context, opts contracts.ListTriageEventsOptions) (*contracts.TriageEventListResult, error) {
	f.listTriageCalled = true
	f.lastListTriageArgs = opts
	return f.triageEventListResult, f.err
}

func (f *fakeService) RecordContribution(ctx context.Context, opts contracts.RecordContributionOptions) (*contracts.ContributionResult, error) {
	f.recordContributionCalled = true
	f.lastRecordContributionArgs = opts
	return f.contributionResult, f.err
}

func (f *fakeService) GetContribution(ctx context.Context, id string) (*contracts.ContributionResult, error) {
	f.getContributionCalled = true
	f.lastShowContributionArg = id
	return f.contributionResult, f.err
}

func (f *fakeService) ListContributions(ctx context.Context, opts contracts.ListContributionsOptions) (*contracts.ContributionListResult, error) {
	f.listContributionsCalled = true
	f.lastListContributionsArgs = opts
	return f.contributionListResult, f.err
}

func (f *fakeService) RecordContributionOutcome(ctx context.Context, opts contracts.RecordContributionOutcomeOptions) (*contracts.ContributionOutcomeResult, error) {
	f.recordOutcomeCalled = true
	f.lastRecordOutcomeArgs = opts
	return f.contributionOutcomeResult, f.err
}

func (f *fakeService) ListContributionOutcomes(ctx context.Context, contributionID string) (*contracts.ContributionOutcomeListResult, error) {
	f.listOutcomesCalled = true
	f.lastListOutcomesArg = contributionID
	return f.contributionOutcomeListResult, f.err
}

func (f *fakeService) ExportLocalMetadata(ctx context.Context, opts contracts.MetadataExportOptions) (*contracts.MetadataExportResult, error) {
	f.exportMetadataCalled = true
	f.lastExportMetadataArgs = opts
	return f.metadataExportResult, f.err
}

func (f *fakeService) ImportLocalMetadata(ctx context.Context, opts contracts.MetadataImportOptions) (*contracts.MetadataImportResult, error) {
	f.importMetadataCalled = true
	f.lastImportMetadataArgs = opts
	return f.metadataImportResult, f.err
}

type fakeMCPRunner struct {
	called bool
	opts   contracts.MCPOptions
	err    error
}

func (f *fakeMCPRunner) Run(ctx context.Context, opts contracts.MCPOptions) error {
	f.called = true
	f.opts = opts
	return f.err
}

func newTestCLI(svc contracts.Service, runner contracts.MCPRunner) (*cli.CLI, *bytes.Buffer, *bytes.Buffer) {
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
