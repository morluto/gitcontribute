package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/tracking"
)

func TestServiceTrackingFlow(t *testing.T) {
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

	event, err := svc.RecordOutcome(ctx, &tracking.TriageEvent{
		TargetKind: tracking.TargetOpportunity,
		TargetRef:  opp.ID,
		Outcome:    tracking.OutcomeInvestigated,
		Reason:     "looks promising",
	})
	if err != nil {
		t.Fatalf("record outcome: %v", err)
	}
	if event.ID == "" {
		t.Fatal("expected durable event id")
	}

	events, err := svc.ListOutcomes(ctx, tracking.TriageEventFilter{})
	if err != nil {
		t.Fatalf("list outcomes: %v", err)
	}
	if len(events) != 1 || events[0].Outcome != tracking.OutcomeInvestigated {
		t.Fatalf("unexpected outcomes: %+v", events)
	}

	preparedAt := time.Unix(5000, 0).UTC()
	contribution, err := svc.RecordContribution(ctx, &tracking.Contribution{
		OpportunityID: opp.ID,
		Kind:          "issue",
		Title:         "parser panics",
		Body:          "description",
		PreparedAt:    preparedAt,
	})
	if err != nil {
		t.Fatalf("record contribution: %v", err)
	}
	if contribution.ID == "" {
		t.Fatal("expected durable contribution id")
	}

	submitted, err := svc.RecordContributionOutcome(ctx, &tracking.ContributionOutcome{
		ContributionID: contribution.ID,
		Outcome:        tracking.OutcomeSubmitted,
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
	if len(outcomes) != 1 || outcomes[0].Outcome != tracking.OutcomeSubmitted {
		t.Fatalf("unexpected contribution outcomes: %+v", outcomes)
	}
}

func TestServiceExportImportLocalMetadata(t *testing.T) {
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()

	inv, _ := svc.StartInvestigation(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, "abc", "")
	h, _ := svc.AddHypothesis(ctx, inv.ID, "panic", "desc", "bug")
	opp, _ := svc.PromoteOpportunity(ctx, h.ID, "panic", "parser", "crash", "small", 0.8)

	_, err := svc.RecordOutcome(ctx, &tracking.TriageEvent{
		TargetKind: tracking.TargetOpportunity,
		TargetRef:  opp.ID,
		Outcome:    tracking.OutcomeSaved,
	})
	if err != nil {
		t.Fatalf("record outcome: %v", err)
	}

	contribution, err := svc.RecordContribution(ctx, &tracking.Contribution{
		OpportunityID: opp.ID,
		Kind:          "issue",
		Title:         "parser panics",
		Body:          "problem",
	})
	if err != nil {
		t.Fatalf("record contribution: %v", err)
	}
	_, err = svc.RecordContributionOutcome(ctx, &tracking.ContributionOutcome{
		ContributionID: contribution.ID,
		Outcome:        tracking.OutcomeMerged,
	})
	if err != nil {
		t.Fatalf("record outcome: %v", err)
	}

	data, bundle, err := svc.ExportLocalMetadata(ctx, tracking.ExportOptions{})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(bundle.TriageEvents) != 1 || len(bundle.Contributions) != 1 || len(bundle.ContributionOutcomes) != 1 {
		t.Fatalf("unexpected bundle: triage=%d contributions=%d outcomes=%d", len(bundle.TriageEvents), len(bundle.Contributions), len(bundle.ContributionOutcomes))
	}
	if !strings.Contains(string(data), "triage_events") {
		t.Fatalf("expected triage_events in JSON export")
	}

	if err := svc.ImportLocalMetadata(ctx, data); err != nil {
		t.Fatalf("import: %v", err)
	}

	contribs, _ := svc.ListContributions(ctx, tracking.ContributionFilter{})
	if len(contribs) != 1 || contribs[0].OpportunityID != opp.ID {
		t.Fatalf("unexpected imported contribution: %+v", contribs)
	}

	if err := svc.ImportLocalMetadata(ctx, data); err != nil {
		t.Fatalf("second import should be idempotent: %v", err)
	}
	contribs2, _ := svc.ListContributions(ctx, tracking.ContributionFilter{})
	if len(contribs2) != 1 {
		t.Fatalf("expected 1 contribution after re-import, got %d", len(contribs2))
	}
}

func TestCollectionValidationSupportsNewReferenceKinds(t *testing.T) {
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
