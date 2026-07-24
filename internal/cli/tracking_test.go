package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/cli"
)

func TestTriageRecordDispatchesOptions(t *testing.T) {
	t.Parallel()
	svc := &fakeService{triageEventResult: &cli.TriageEventResult{
		ID: "te-1", TargetKind: "opportunity", TargetRef: "opp-1", Outcome: "investigated", Lens: "active-go",
	}}
	c, stdout, stderr := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{
		"triage", "record", "opportunity:opp-1", "investigated",
		"--reason", "looks promising", "--lens", "active-go",
	}))
	if !svc.recordTriageCalled {
		t.Fatal("RecordTriageEvent was not called")
	}
	if svc.lastRecordTriageArgs.Target != "opportunity:opp-1" ||
		svc.lastRecordTriageArgs.Outcome != "investigated" ||
		svc.lastRecordTriageArgs.Reason != "looks promising" ||
		svc.lastRecordTriageArgs.Lens != "active-go" {
		t.Fatalf("unexpected record triage args: %+v", svc.lastRecordTriageArgs)
	}
	if !strings.Contains(stdout.String(), "te-1") || !strings.Contains(stderr.String(), "recording triage") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestTriageListDispatchesBoundedOptions(t *testing.T) {
	t.Parallel()
	svc := &fakeService{triageEventListResult: &cli.TriageEventListResult{Events: []cli.TriageEventResult{
		{ID: "te-1", TargetKind: "repository", TargetRef: "owner/repo", Outcome: "saved"},
	}}}
	c, stdout, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{
		"triage", "list", "--kind", "repository", "--outcome", "saved", "--limit", "10",
	}))
	if !svc.listTriageCalled || svc.lastListTriageArgs.TargetKind != "repository" ||
		svc.lastListTriageArgs.Outcome != "saved" || svc.lastListTriageArgs.Limit != 10 {
		t.Fatalf("unexpected list triage args: %+v", svc.lastListTriageArgs)
	}
	if !strings.Contains(stdout.String(), "te-1") {
		t.Fatalf("stdout=%q", stdout.String())
	}
}

func TestTriageListRejectsInvalidLimit(t *testing.T) {
	t.Parallel()
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)
	err := c.Run(context.Background(), []string{"triage", "list", "--limit", "0"})
	requireCLIError(t, err, cli.ExitUsage)
	if svc.listTriageCalled {
		t.Fatal("list triage should not be called with invalid limit")
	}
}

func TestContributionRecordDispatchesOptions(t *testing.T) {
	t.Parallel()
	svc := &fakeService{contributionResult: &cli.ContributionResult{
		ID: "c-1", OpportunityID: "opp-1", Kind: "issue", Title: "parser panics",
	}}
	c, stdout, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{
		"contribution", "record", "opp-1", "issue", "parser panics",
		"--body", "description", "--reference", "gh-123",
	}))
	if !svc.recordContributionCalled {
		t.Fatal("RecordContribution was not called")
	}
	if svc.lastRecordContributionArgs.OpportunityID != "opp-1" ||
		svc.lastRecordContributionArgs.Kind != "issue" ||
		svc.lastRecordContributionArgs.Title != "parser panics" ||
		svc.lastRecordContributionArgs.Body != "description" ||
		svc.lastRecordContributionArgs.Reference != "gh-123" {
		t.Fatalf("unexpected record contribution args: %+v", svc.lastRecordContributionArgs)
	}
	if !strings.Contains(stdout.String(), "c-1") {
		t.Fatalf("stdout=%q", stdout.String())
	}
}

func TestContributionListAndShow(t *testing.T) {
	t.Parallel()
	svc := &fakeService{
		contributionListResult: &cli.ContributionListResult{Contributions: []cli.ContributionResult{
			{ID: "c-1", OpportunityID: "opp-1", Kind: "issue", Title: "parser panics"},
		}},
		contributionResult: &cli.ContributionResult{
			ID: "c-1", OpportunityID: "opp-1", Kind: "issue", Title: "parser panics",
		},
	}

	c, stdout, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"contribution", "list", "--opportunity", "opp-1", "--limit", "5"}))
	if !svc.listContributionsCalled || svc.lastListContributionsArgs.OpportunityID != "opp-1" || svc.lastListContributionsArgs.Limit != 5 {
		t.Fatalf("unexpected list args: %+v", svc.lastListContributionsArgs)
	}
	if !strings.Contains(stdout.String(), "parser panics") {
		t.Fatalf("stdout=%q", stdout.String())
	}

	stdout.Reset()
	requireNoErr(t, c.Run(context.Background(), []string{"contribution", "show", "c-1"}))
	if !svc.getContributionCalled || svc.lastShowContributionArg != "c-1" {
		t.Fatalf("show not called correctly: %+v", svc.lastShowContributionArg)
	}
	if !strings.Contains(stdout.String(), "parser panics") {
		t.Fatalf("stdout=%q", stdout.String())
	}
}

func TestContributionOutcomeAndOutcomes(t *testing.T) {
	t.Parallel()
	svc := &fakeService{
		contributionOutcomeResult: &cli.ContributionOutcomeResult{
			ID: "co-1", ContributionID: "c-1", Outcome: "submitted",
		},
		contributionOutcomeListResult: &cli.ContributionOutcomeListResult{
			ContributionID: "c-1",
			Outcomes: []cli.ContributionOutcomeResult{
				{ID: "co-1", ContributionID: "c-1", Outcome: "submitted"},
			},
		},
	}
	c, stdout, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{
		"contribution", "outcome", "c-1", "submitted", "--reason", "opened manually",
	}))
	if !svc.recordOutcomeCalled || svc.lastRecordOutcomeArgs.ContributionID != "c-1" ||
		svc.lastRecordOutcomeArgs.Outcome != "submitted" || svc.lastRecordOutcomeArgs.Reason != "opened manually" {
		t.Fatalf("unexpected outcome args: %+v", svc.lastRecordOutcomeArgs)
	}

	stdout.Reset()
	requireNoErr(t, c.Run(context.Background(), []string{"contribution", "outcomes", "c-1"}))
	if !svc.listOutcomesCalled || svc.lastListOutcomesArg != "c-1" {
		t.Fatalf("outcomes not called correctly: %v", svc.lastListOutcomesArg)
	}
	if !strings.Contains(stdout.String(), "submitted") {
		t.Fatalf("stdout=%q", stdout.String())
	}
}

func TestTrackingExportWritesBundleToStdout(t *testing.T) {
	t.Parallel()
	svc := &fakeService{metadataExportResult: &cli.MetadataExportResult{
		SchemaVersion:        2,
		Data:                 []byte(`{"schema_version":2,"triage_events":[],"evidence":[]}`),
		TriageEvents:         0,
		Contributions:        0,
		ContributionOutcomes: 0,
		Evidence:             0,
	}}
	c, stdout, stderr := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"tracking", "export", "--limit", "100"}))
	if !svc.exportMetadataCalled || svc.lastExportMetadataArgs.Limit != 100 {
		t.Fatalf("unexpected export args: %+v", svc.lastExportMetadataArgs)
	}
	if !strings.Contains(stdout.String(), `"schema_version":2`) || !strings.Contains(stdout.String(), "evidence") {
		t.Fatalf("stdout=%q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "exporting local tracking metadata") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestTrackingExportWritesBundleToFile(t *testing.T) {
	t.Parallel()
	svc := &fakeService{metadataExportResult: &cli.MetadataExportResult{
		SchemaVersion:        2,
		Data:                 []byte(`{"schema_version":2,"triage_events":[],"evidence":[{}]}`),
		TriageEvents:         0,
		Contributions:        0,
		ContributionOutcomes: 0,
		Evidence:             1,
	}}
	path := filepath.Join(t.TempDir(), "metadata.json")
	c, _, stderr := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"tracking", "export", "--output", path}))
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("export file not written: %v", err)
	}
	if !strings.Contains(stderr.String(), path) || !strings.Contains(stderr.String(), "v2") || !strings.Contains(stderr.String(), "1 evidence") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestTrackingImportReadsStdin(t *testing.T) {
	t.Parallel()
	svc := &fakeService{metadataImportResult: &cli.MetadataImportResult{SchemaVersion: 2, Evidence: 1}}
	c, stdout, _ := newTestCLI(svc, nil)
	c.SetInput(strings.NewReader(`{"triage_events":[]}`))
	requireNoErr(t, c.Run(context.Background(), []string{"tracking", "import"}))
	if !svc.importMetadataCalled {
		t.Fatal("ImportLocalMetadata was not called")
	}
	if !bytes.Equal(svc.lastImportMetadataArgs.Data, []byte(`{"triage_events":[]}`)) {
		t.Fatalf("unexpected import data: %q", svc.lastImportMetadataArgs.Data)
	}
	if !strings.Contains(stdout.String(), "bundle v2") || !strings.Contains(stdout.String(), "1 evidence records") {
		t.Fatalf("stdout=%q", stdout.String())
	}
}

func TestTrackingImportReadsFile(t *testing.T) {
	t.Parallel()
	svc := &fakeService{metadataImportResult: &cli.MetadataImportResult{}}
	path := filepath.Join(t.TempDir(), "metadata.json")
	if err := os.WriteFile(path, []byte(`{"triage_events":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c, _, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"tracking", "import", "--file", path}))
	if !svc.importMetadataCalled {
		t.Fatal("ImportLocalMetadata was not called")
	}
	if !bytes.Equal(svc.lastImportMetadataArgs.Data, []byte(`{"triage_events":[]}`)) {
		t.Fatalf("unexpected import data: %q", svc.lastImportMetadataArgs.Data)
	}
}
