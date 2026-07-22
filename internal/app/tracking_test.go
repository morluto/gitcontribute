package app

import (
	"context"
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/tracking"
)

func TestServiceTrackingFlow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()

	inv, err := svc.StartInvestigation(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, "abc", "")
	if err != nil {
		t.Fatalf("start investigation: %v", err)
	}
	h, err := svc.AddHypothesis(ctx, inv.ID, "panic", "desc", "bug")
	if err != nil {
		t.Fatalf("add hypothesis: %v", err)
	}
	opp, err := svc.PromoteOpportunity(ctx, h.ID, "panic", "parser", "crash", "small", 0.8)
	if err != nil {
		t.Fatalf("promote opportunity: %v", err)
	}

	event, err := svc.RecordTriageEvent(ctx, cli.RecordTriageEventOptions{
		Target:  "opportunity:" + opp.ID,
		Outcome: string(tracking.OutcomeInvestigated),
		Reason:  "looks promising",
		Lens:    "active-go",
	})
	if err != nil {
		t.Fatalf("record triage event: %v", err)
	}
	if event.ID == "" {
		t.Fatal("expected durable event id")
	}
	if event.Lens != "active-go" {
		t.Fatalf("expected lens active-go, got %q", event.Lens)
	}

	list, err := svc.ListTriageEvents(ctx, cli.ListTriageEventsOptions{Lens: "active-go"})
	if err != nil {
		t.Fatalf("list triage events: %v", err)
	}
	if len(list.Events) != 1 || list.Events[0].Outcome != string(tracking.OutcomeInvestigated) {
		t.Fatalf("unexpected triage events: %+v", list.Events)
	}

	contribution, err := svc.RecordContribution(ctx, cli.RecordContributionOptions{
		OpportunityID: opp.ID,
		Kind:          "issue",
		Title:         "parser panics",
		Body:          "description",
	})
	if err != nil {
		t.Fatalf("record contribution: %v", err)
	}
	if contribution.ID == "" {
		t.Fatal("expected durable contribution id")
	}

	submitted, err := svc.RecordContributionOutcome(ctx, cli.RecordContributionOutcomeOptions{
		ContributionID: contribution.ID,
		Outcome:        string(tracking.OutcomeSubmitted),
		Reason:         "opened on GitHub",
	})
	if err != nil {
		t.Fatalf("record contribution outcome: %v", err)
	}
	if submitted.ID == "" {
		t.Fatal("expected durable outcome id")
	}

	got, err := svc.GetContribution(ctx, contribution.ID)
	if err != nil {
		t.Fatalf("get contribution: %v", err)
	}
	if got == nil || got.Title != "parser panics" {
		t.Fatalf("unexpected contribution: %+v", got)
	}

	outcomes, err := svc.ListContributionOutcomes(ctx, contribution.ID)
	if err != nil {
		t.Fatalf("list contribution outcomes: %v", err)
	}
	if len(outcomes.Outcomes) != 1 || outcomes.Outcomes[0].Outcome != string(tracking.OutcomeSubmitted) {
		t.Fatalf("unexpected contribution outcomes: %+v", outcomes)
	}
}

func TestServiceExportImportLocalMetadata(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()

	inv, _ := svc.StartInvestigation(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, "abc", "")
	h, _ := svc.AddHypothesis(ctx, inv.ID, "panic", "desc", "bug")
	opp, _ := svc.PromoteOpportunity(ctx, h.ID, "panic", "parser", "crash", "small", 0.8)

	_, err := svc.RecordTriageEvent(ctx, cli.RecordTriageEventOptions{
		Target:  "opportunity:" + opp.ID,
		Outcome: string(tracking.OutcomeSaved),
	})
	if err != nil {
		t.Fatalf("record triage event: %v", err)
	}

	contribution, err := svc.RecordContribution(ctx, cli.RecordContributionOptions{
		OpportunityID: opp.ID,
		Kind:          "issue",
		Title:         "parser panics",
		Body:          "problem",
	})
	if err != nil {
		t.Fatalf("record contribution: %v", err)
	}
	_, err = svc.RecordContributionOutcome(ctx, cli.RecordContributionOutcomeOptions{
		ContributionID: contribution.ID,
		Outcome:        string(tracking.OutcomeMerged),
	})
	if err != nil {
		t.Fatalf("record outcome: %v", err)
	}

	result, err := svc.ExportLocalMetadata(ctx, cli.MetadataExportOptions{})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if result.SchemaVersion != tracking.CurrentBundleSchemaVersion ||
		result.TriageEvents != 1 || result.Contributions != 1 || result.ContributionOutcomes != 1 || result.Evidence != 0 {
		t.Fatalf("unexpected export counts: %+v", result)
	}
	if !strings.Contains(string(result.Data), `"schema_version": 2`) || !strings.Contains(string(result.Data), "triage_events") {
		t.Fatalf("expected triage_events in JSON export")
	}

	imported, err := svc.ImportLocalMetadata(ctx, cli.MetadataImportOptions{Data: result.Data})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if imported.SchemaVersion != tracking.CurrentBundleSchemaVersion {
		t.Fatalf("unexpected import version: %+v", imported)
	}

	contribs, _ := svc.ListContributions(ctx, cli.ListContributionsOptions{})
	if len(contribs.Contributions) != 1 || contribs.Contributions[0].OpportunityID != opp.ID {
		t.Fatalf("unexpected imported contribution: %+v", contribs)
	}

	_, err = svc.ImportLocalMetadata(ctx, cli.MetadataImportOptions{Data: result.Data})
	if err != nil {
		t.Fatalf("second import should be idempotent: %v", err)
	}
	contribs2, _ := svc.ListContributions(ctx, cli.ListContributionsOptions{})
	if len(contribs2.Contributions) != 1 {
		t.Fatalf("expected 1 contribution after re-import, got %d", len(contribs2.Contributions))
	}
}

func TestCollectionValidationSupportsNewReferenceKinds(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()

	if _, err := svc.CreateCollection(ctx, "tracked"); err != nil {
		t.Fatalf("create collection: %v", err)
	}

	valid := []cli.CollectionMember{
		{Kind: "thread", Ref: "owner/repo#1"},
		{Kind: "opportunity", Ref: "11111111-1111-1111-1111-111111111111"},
		{Kind: "investigation", Ref: "22222222-2222-2222-2222-222222222222"},
	}
	if _, err := svc.AddCollectionMembers(ctx, "tracked", valid); err != nil {
		t.Fatalf("valid members rejected: %v", err)
	}

	invalid := []cli.CollectionMember{
		{Kind: "thread", Ref: "owner/repo#0"},
		{Kind: "opportunity", Ref: "not-a-uuid"},
		{Kind: "investigation", Ref: ""},
	}
	for _, m := range invalid {
		if _, err := svc.AddCollectionMembers(ctx, "tracked", []cli.CollectionMember{m}); err == nil {
			t.Fatalf("accepted invalid member %+v", m)
		}
	}

	cols, err := svc.ListCollections(ctx)
	if err != nil {
		t.Fatalf("list collections: %v", err)
	}
	if len(cols.Collections) != 1 || cols.Collections[0].MemberCount != 3 {
		t.Fatalf("unexpected collection count: %+v", cols)
	}
}
