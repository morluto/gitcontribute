package cli_test

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
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
	t.Parallel()
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

func TestDoctorStrictReturnsFailureAfterRenderingDiagnostics(t *testing.T) {
	t.Parallel()
	svc := &fakeControlService{
		fakeService: &fakeService{},
		doctor: &cli.DoctorResult{Healthy: false, Checks: []cli.DoctorCheck{{
			Name: "database", Status: "error", Required: true, Message: "unavailable",
		}}},
	}
	c, stdout, _ := newTestCLI(svc, nil)
	err := c.Run(context.Background(), []string{"doctor", "--json", "--strict"})
	if !containsText(stdout.String(), `"healthy": false`) {
		t.Fatalf("doctor output=%q", stdout.String())
	}
	var cliErr *cli.CLIError
	if !errors.As(err, &cliErr) || cliErr.Code != cli.ExitGeneral || !strings.Contains(cliErr.Error(), "required diagnostic checks failed") {
		t.Fatalf("doctor error = %v", err)
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
	t.Parallel()
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
	t.Parallel()
	runner := &fakeTUIRunner{}
	c, _, _ := newTestCLI(&fakeService{}, nil)
	c.SetTUIRunner(runner)
	requireNoErr(t, c.Run(context.Background(), []string{"tui", "owner/repo", "--json"}))
	if !runner.called || runner.opts.Repo.String() != "owner/repo" || !runner.opts.JSON {
		t.Fatalf("runner state: called=%v opts=%+v", runner.called, runner.opts)
	}
}

func TestBareInvocationDispatchesTUI(t *testing.T) {
	t.Parallel()
	runner := &fakeTUIRunner{}
	c, _, _ := newTestCLI(&fakeService{}, nil)
	c.SetInput(strings.NewReader(""))
	c.SetTUIRunner(runner)

	requireNoErr(t, c.Run(context.Background(), nil))
	if !runner.called || runner.opts.Repo.Owner != "" || runner.opts.Repo.Repo != "" || runner.opts.JSON {
		t.Fatalf("runner state: called=%v opts=%+v", runner.called, runner.opts)
	}
}

func TestBareInvocationWithoutTerminalReturnsConciseUsageError(t *testing.T) {
	t.Parallel()
	input, output, err := os.Pipe()
	if err != nil {
		t.Fatalf("create input pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = input.Close()
		_ = output.Close()
	})

	runner := &fakeTUIRunner{}
	c, _, _ := newTestCLI(&fakeService{}, nil)
	c.SetInput(input)
	c.SetTUIRunner(runner)

	err = c.Run(context.Background(), nil)
	var cliErr *cli.CLIError
	if !errors.As(err, &cliErr) || cliErr.Code != cli.ExitUsage {
		t.Fatalf("expected usage error, got %T: %v", err, err)
	}
	if got := cliErr.Error(); got != "interactive interface requires a terminal; run gitcontribute --help for commands" {
		t.Fatalf("error = %q", got)
	}
	if runner.called {
		t.Fatal("TUI runner called without an interactive terminal")
	}
}

func TestBareInvocationRequiresInteractiveOutput(t *testing.T) {
	t.Parallel()
	outputReader, outputWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("create output pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = outputReader.Close()
		_ = outputWriter.Close()
	})

	runner := &fakeTUIRunner{}
	c := cli.New(&fakeService{}, nil, outputWriter, io.Discard)
	c.SetInput(strings.NewReader(""))
	c.SetTUIRunner(runner)

	err = c.Run(context.Background(), nil)
	var cliErr *cli.CLIError
	if !errors.As(err, &cliErr) || cliErr.Code != cli.ExitUsage {
		t.Fatalf("expected usage error, got %T: %v", err, err)
	}
	if runner.called {
		t.Fatal("TUI runner called with non-interactive output")
	}
}

func TestMCPServeCommand(t *testing.T) {
	t.Parallel()
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
