package cli_test

import (
	"context"
	"testing"

	"github.com/morluto/gitcontribute/internal/cli"
)

type fakeControlService struct {
	*fakeService
	configureOpts cli.ConfigureOptions
	metadata      *cli.MetadataResult
	configured    *cli.ConfigureResult
	controlStatus *cli.ControlStatusResult
	doctor        *cli.DoctorResult
}

func (f *fakeControlService) Metadata(context.Context) (*cli.MetadataResult, error) {
	return f.metadata, nil
}

func (f *fakeControlService) Configure(_ context.Context, opts cli.ConfigureOptions) (*cli.ConfigureResult, error) {
	f.configureOpts = opts
	return f.configured, nil
}

func (f *fakeControlService) ControlStatus(context.Context) (*cli.ControlStatusResult, error) {
	return f.controlStatus, nil
}

func (f *fakeControlService) Doctor(context.Context) (*cli.DoctorResult, error) {
	return f.doctor, nil
}

func TestControlCommands(t *testing.T) {
	svc := &fakeControlService{
		fakeService: &fakeService{},
		metadata:    &cli.MetadataResult{Name: "gitcontribute", Version: "test", Capabilities: []string{"mcp-stdio"}},
		configured:  &cli.ConfigureResult{Path: "/tmp/config.toml", Changed: true, Config: cli.ConfigResult{CrawlBudget: 25}},
		controlStatus: &cli.ControlStatusResult{
			Healthy: true, Counts: cli.ControlCounts{Repositories: 2}, Warnings: []string{},
		},
		doctor: &cli.DoctorResult{Healthy: true, Checks: []cli.DoctorCheck{{Name: "git", Status: "ok"}}},
	}

	c, stdout, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"metadata", "--json"}))
	if !containsText(stdout.String(), `"name": "gitcontribute"`) {
		t.Fatalf("metadata output=%q", stdout.String())
	}

	c, stdout, _ = newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"configure", "--crawl-budget", "25", "--dry-run", "--json"}))
	if svc.configureOpts.CrawlBudget == nil || *svc.configureOpts.CrawlBudget != 25 || !svc.configureOpts.DryRun {
		t.Fatalf("configure opts=%+v", svc.configureOpts)
	}

	c, stdout, _ = newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"status", "--json"}))
	if !containsText(stdout.String(), `"repositories": 2`) {
		t.Fatalf("status output=%q", stdout.String())
	}

	c, stdout, _ = newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"doctor", "--json"}))
	if !containsText(stdout.String(), `"name": "git"`) {
		t.Fatalf("doctor output=%q", stdout.String())
	}
}

type fakeJobService struct {
	*fakeService
	listedStatus string
	shownID      string
	cancelledID  string
	job          *cli.JobResult
}

func (f *fakeJobService) ListJobs(_ context.Context, status string, _ int) (*cli.JobListResult, error) {
	f.listedStatus = status
	return &cli.JobListResult{Jobs: []cli.JobResult{*f.job}}, nil
}

func (f *fakeJobService) GetJob(_ context.Context, id string) (*cli.JobResult, error) {
	f.shownID = id
	return f.job, nil
}

func (f *fakeJobService) CancelJob(_ context.Context, id string) (*cli.JobResult, error) {
	f.cancelledID = id
	return f.job, nil
}

func TestJobCommands(t *testing.T) {
	svc := &fakeJobService{fakeService: &fakeService{}, job: &cli.JobResult{ID: "job-1", Kind: "crawl", Status: "running", CreatedAt: "now"}}
	c, stdout, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"jobs", "--status", "running", "--json"}))
	if svc.listedStatus != "running" || !containsText(stdout.String(), "job-1") {
		t.Fatalf("status=%q output=%q", svc.listedStatus, stdout.String())
	}

	c, _, _ = newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"job", "show", "job-1"}))
	if svc.shownID != "job-1" {
		t.Fatalf("shown id=%q", svc.shownID)
	}

	c, _, _ = newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"job", "cancel", "job-1"}))
	if svc.cancelledID != "job-1" {
		t.Fatalf("cancelled id=%q", svc.cancelledID)
	}
}

type fakeTUIRunner struct {
	called bool
	opts   cli.TUIOptions
}

func (f *fakeTUIRunner) Run(_ context.Context, opts cli.TUIOptions) error {
	f.called = true
	f.opts = opts
	return nil
}

func TestTUICommandDispatch(t *testing.T) {
	runner := &fakeTUIRunner{}
	c, _, _ := newTestCLI(&fakeService{}, nil)
	c.SetTUIRunner(runner)
	requireNoErr(t, c.Run(context.Background(), []string{"tui", "owner/repo", "--json"}))
	if !runner.called || runner.opts.Repo.String() != "owner/repo" || !runner.opts.JSON {
		t.Fatalf("runner state: called=%v opts=%+v", runner.called, runner.opts)
	}
}

func TestMCPServeCommand(t *testing.T) {
	runner := &fakeMCPRunner{}
	c, _, _ := newTestCLI(nil, runner)
	requireNoErr(t, c.Run(context.Background(), []string{"mcp", "serve", "--transport", "stdio"}))
	if !runner.called || runner.opts.Transport != "stdio" {
		t.Fatalf("runner state: called=%v opts=%+v", runner.called, runner.opts)
	}
}

func containsText(value, substring string) bool {
	for i := 0; i+len(substring) <= len(value); i++ {
		if value[i:i+len(substring)] == substring {
			return true
		}
	}
	return false
}
