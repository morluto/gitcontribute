package cli_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
)

type fakeExtendedService struct {
	*fakeService

	createWorkspaceCalled   bool
	showWorkspaceCalled     bool
	defineValidationCalled  bool
	runValidationCalled     bool
	compareValidationCalled bool
	showEvidenceCalled      bool
	prepareIssueCalled      bool
	preparePRCalled         bool

	workspaceResult     *cli.WorkspaceResult
	validationResult    *cli.ValidationResult
	validationRunResult *cli.ValidationRunResult
	comparisonResult    *cli.ValidationComparisonResult
	evidenceResult      *cli.EvidenceResult
	draftResult         *cli.DraftResult

	lastWorkspaceInvestigation  string
	lastCreateWorkspaceOpts     cli.WorkspaceCreateOptions
	lastShowWorkspaceID         string
	lastValidationInvestigation string
	lastDefineValidationOpts    cli.DefineValidationOptions
	lastRunValidationID         string
	lastRunKind                 string
	lastCompareBase             string
	lastCompareCandidate        string
	lastEvidenceInvestigation   string
	lastPrepareIssueID          string
	lastPrepareIssueOpts        cli.PrepareIssueOptions
	lastPreparePRID             string
	lastPreparePROpts           cli.PreparePROptions
}

func (f *fakeExtendedService) CreateWorkspace(ctx context.Context, investigationID string, opts cli.WorkspaceCreateOptions) (*cli.WorkspaceResult, error) {
	f.createWorkspaceCalled = true
	f.lastWorkspaceInvestigation = investigationID
	f.lastCreateWorkspaceOpts = opts
	return f.workspaceResult, f.err
}

func (f *fakeExtendedService) ShowWorkspace(ctx context.Context, id string) (*cli.WorkspaceResult, error) {
	f.showWorkspaceCalled = true
	f.lastShowWorkspaceID = id
	return f.workspaceResult, f.err
}

func (f *fakeExtendedService) DefineValidation(ctx context.Context, investigationID string, opts cli.DefineValidationOptions) (*cli.ValidationResult, error) {
	f.defineValidationCalled = true
	f.lastValidationInvestigation = investigationID
	f.lastDefineValidationOpts = opts
	return f.validationResult, f.err
}

func (f *fakeExtendedService) RunValidation(ctx context.Context, id string, kind string) (*cli.ValidationRunResult, error) {
	f.runValidationCalled = true
	f.lastRunValidationID = id
	f.lastRunKind = kind
	return f.validationRunResult, f.err
}

func (f *fakeExtendedService) CompareValidation(ctx context.Context, baseRunID, candidateRunID string) (*cli.ValidationComparisonResult, error) {
	f.compareValidationCalled = true
	f.lastCompareBase = baseRunID
	f.lastCompareCandidate = candidateRunID
	return f.comparisonResult, f.err
}

func (f *fakeExtendedService) ShowEvidence(ctx context.Context, investigationID string) (*cli.EvidenceResult, error) {
	f.showEvidenceCalled = true
	f.lastEvidenceInvestigation = investigationID
	return f.evidenceResult, f.err
}

func (f *fakeExtendedService) PrepareIssue(ctx context.Context, opportunityID string, opts cli.PrepareIssueOptions) (*cli.DraftResult, error) {
	f.prepareIssueCalled = true
	f.lastPrepareIssueID = opportunityID
	f.lastPrepareIssueOpts = opts
	return f.draftResult, f.err
}

func (f *fakeExtendedService) PreparePullRequest(ctx context.Context, opportunityID string, opts cli.PreparePROptions) (*cli.DraftResult, error) {
	f.preparePRCalled = true
	f.lastPreparePRID = opportunityID
	f.lastPreparePROpts = opts
	return f.draftResult, f.err
}

func TestWorkspaceCreateAndShow(t *testing.T) {
	svc := &fakeExtendedService{
		fakeService: &fakeService{},
		workspaceResult: &cli.WorkspaceResult{
			ID:              "ws-1",
			InvestigationID: "inv-1",
			Repo:            cli.RepoRef{Owner: "o", Repo: "r"},
			Path:            "/tmp/ws",
			BaseSHA:         "abc",
			CandidateSHA:    "def",
			CreatedAt:       "2026-07-17T00:00:00Z",
		},
	}
	c, stdout, stderr := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"workspace", "create", "inv-1", "--remote", "https://example.com/repo.git", "--base", "main", "--candidate", "feature", "--name", "ws-1"})
	requireNoErr(t, err)
	if !svc.createWorkspaceCalled || svc.lastWorkspaceInvestigation != "inv-1" || svc.lastCreateWorkspaceOpts.Remote != "https://example.com/repo.git" {
		t.Fatalf("create workspace args = %+v", svc.lastCreateWorkspaceOpts)
	}
	if !strings.Contains(stdout.String(), "ws-1") || !strings.Contains(stderr.String(), "creating workspace") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	stdout.Reset()
	err = c.Run(context.Background(), []string{"workspace", "show", "ws-1"})
	requireNoErr(t, err)
	if !svc.showWorkspaceCalled || svc.lastShowWorkspaceID != "ws-1" {
		t.Fatalf("show workspace not called correctly: called=%v arg=%q", svc.showWorkspaceCalled, svc.lastShowWorkspaceID)
	}
	if !strings.Contains(stdout.String(), "ws-1") {
		t.Fatalf("stdout=%q", stdout.String())
	}
}

func TestValidationDefineRunAndCompare(t *testing.T) {
	svc := &fakeExtendedService{
		fakeService: &fakeService{},
		validationResult: &cli.ValidationResult{
			ID:              "val-1",
			InvestigationID: "inv-1",
			Kind:            "test",
			Command:         []string{"go", "test"},
			WorkingDir:      "/tmp/ws",
			Timeout:         "1m",
			MaxOutputBytes:  1024,
			CreatedAt:       "2026-07-17T00:00:00Z",
		},
		validationRunResult: &cli.ValidationRunResult{
			ID:             "run-1",
			DefinitionID:   "val-1",
			Kind:           "base",
			ExitCode:       0,
			Classification: "passing",
			StartedAt:      "2026-07-17T00:00:00Z",
			CompletedAt:    "2026-07-17T00:00:01Z",
		},
		comparisonResult: &cli.ValidationComparisonResult{
			Classification: "fixed",
			Explanation:    "base failed, candidate passed",
		},
	}
	c, stdout, stderr := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{
		"validation", "define", "inv-1",
		"--kind", "test",
		"--command", "go test",
		"--working-dir", "/tmp/ws",
		"--timeout", "1m",
		"--max-output", "1024",
	})
	requireNoErr(t, err)
	if !svc.defineValidationCalled || svc.lastValidationInvestigation != "inv-1" {
		t.Fatalf("define validation not called correctly: called=%v arg=%q", svc.defineValidationCalled, svc.lastValidationInvestigation)
	}
	if svc.lastDefineValidationOpts.Kind != "test" || svc.lastDefineValidationOpts.Command != "go test" || svc.lastDefineValidationOpts.WorkingDir != "/tmp/ws" {
		t.Fatalf("define validation opts = %+v", svc.lastDefineValidationOpts)
	}
	if svc.lastDefineValidationOpts.Timeout != time.Minute || svc.lastDefineValidationOpts.MaxOutputBytes != 1024 {
		t.Fatalf("timeout/output opts = %+v", svc.lastDefineValidationOpts)
	}
	if !strings.Contains(stdout.String(), "val-1") || !strings.Contains(stderr.String(), "defining validation") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	stdout.Reset()
	err = c.Run(context.Background(), []string{"validation", "run", "val-1", "--kind", "base"})
	requireNoErr(t, err)
	if !svc.runValidationCalled || svc.lastRunValidationID != "val-1" || svc.lastRunKind != "base" {
		t.Fatalf("run validation args = id:%q kind:%q", svc.lastRunValidationID, svc.lastRunKind)
	}

	stdout.Reset()
	err = c.Run(context.Background(), []string{"validation", "compare", "run-base", "run-candidate"})
	requireNoErr(t, err)
	if !svc.compareValidationCalled || svc.lastCompareBase != "run-base" || svc.lastCompareCandidate != "run-candidate" {
		t.Fatalf("compare validation args = %q %q", svc.lastCompareBase, svc.lastCompareCandidate)
	}
	if !strings.Contains(stdout.String(), "fixed") {
		t.Fatalf("stdout=%q", stdout.String())
	}
}

func TestEvidenceShow(t *testing.T) {
	svc := &fakeExtendedService{
		fakeService: &fakeService{},
		evidenceResult: &cli.EvidenceResult{
			InvestigationID: "inv-1",
			Evidence: []cli.EvidenceItem{
				{ID: "ev-1", Type: "manual_observation", Relation: "supporting", Description: "observed panic", CreatedAt: "2026-07-17T00:00:00Z"},
			},
		},
	}
	c, stdout, _ := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"evidence", "show", "inv-1"})
	requireNoErr(t, err)
	if !svc.showEvidenceCalled || svc.lastEvidenceInvestigation != "inv-1" {
		t.Fatalf("show evidence not called correctly: called=%v arg=%q", svc.showEvidenceCalled, svc.lastEvidenceInvestigation)
	}
	if !strings.Contains(stdout.String(), "ev-1") {
		t.Fatalf("stdout=%q", stdout.String())
	}
}

func TestPrepareIssueAndPullRequest(t *testing.T) {
	svc := &fakeExtendedService{
		fakeService: &fakeService{},
		draftResult: &cli.DraftResult{
			OpportunityID: "opp-1",
			Kind:          "issue",
			Title:         "Fix race",
			Body:          "Problem...",
			RenderedAt:    "2026-07-17T00:00:00Z",
		},
	}
	c, stdout, stderr := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"prepare", "issue", "opp-1", "--guidance", "add tests", "--success", "pass"})
	requireNoErr(t, err)
	if !svc.prepareIssueCalled || svc.lastPrepareIssueID != "opp-1" {
		t.Fatalf("prepare issue not called correctly: called=%v arg=%q", svc.prepareIssueCalled, svc.lastPrepareIssueID)
	}
	if svc.lastPrepareIssueOpts.Guidance != "add tests" || svc.lastPrepareIssueOpts.Success != "pass" {
		t.Fatalf("prepare issue opts = %+v", svc.lastPrepareIssueOpts)
	}
	if !strings.Contains(stdout.String(), "Fix race") || !strings.Contains(stderr.String(), "preparing issue draft") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	stdout.Reset()
	err = c.Run(context.Background(), []string{
		"prepare", "pr", "opp-1",
		"--workspace", "ws-1",
		"--approach", "use mutex",
		"--changes", "lock access",
		"--compatibility", "none",
		"--limitations", "none",
		"--linked-issue", "#1",
		"--guidance", "add tests",
	})
	requireNoErr(t, err)
	if !svc.preparePRCalled || svc.lastPreparePRID != "opp-1" {
		t.Fatalf("prepare pr not called correctly: called=%v arg=%q", svc.preparePRCalled, svc.lastPreparePRID)
	}
	if svc.lastPreparePROpts.WorkspaceID != "ws-1" || svc.lastPreparePROpts.Approach != "use mutex" {
		t.Fatalf("prepare pr opts = %+v", svc.lastPreparePROpts)
	}
}
