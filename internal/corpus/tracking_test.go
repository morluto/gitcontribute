package corpus

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/investigation"
	"github.com/morluto/gitcontribute/internal/tracking"
)

func TestTrackingMigrationCreatesTables(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	var count int
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('triage_events', 'contributions', 'contribution_outcomes')`).Scan(&count); err != nil {
		t.Fatalf("count tracking tables: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 tracking tables, got %d", count)
	}
}

func TestTriageEventPersistsWithOptionalForeignKeyLinks(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "123", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("seed repository: %v", err)
	}
	thread, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 1, "open", "bug", "body", "alice", time.Unix(2, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("seed thread: %v", err)
	}

	svc := tracking.NewService(c)
	event, err := svc.RecordTriageEvent(ctx, &tracking.TriageEvent{
		TargetKind: tracking.TargetIssue,
		TargetRef:  "owner/repo#1",
		Outcome:    tracking.OutcomeViewed,
		Reason:     "interesting",
	})
	if err != nil {
		t.Fatalf("record triage event: %v", err)
	}
	if event.ID == "" {
		t.Fatal("expected durable triage event id")
	}
	if event.RepositoryID == nil || *event.RepositoryID != repo.ID {
		t.Fatalf("expected repository link %d, got %v", repo.ID, event.RepositoryID)
	}
	if event.ThreadID == nil || *event.ThreadID != thread.ID {
		t.Fatalf("expected thread link %d, got %v", thread.ID, event.ThreadID)
	}

	// Outcomes for missing entities should be stored without FK links.
	missing, err := svc.RecordTriageEvent(ctx, &tracking.TriageEvent{
		TargetKind: tracking.TargetOpportunity,
		TargetRef:  "missing-opportunity-id",
		Outcome:    tracking.OutcomeIgnored,
		Reason:     "not relevant",
	})
	if err != nil {
		t.Fatalf("record triage event for missing target: %v", err)
	}
	if missing.OpportunityID != "" {
		t.Fatalf("expected empty opportunity link for missing target, got %q", missing.OpportunityID)
	}

	events, err := svc.ListTriageEvents(ctx, tracking.TriageEventFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list outcomes: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 outcomes, got %d", len(events))
	}
}

func TestTriageEventOrderingIsDeterministic(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	svc := tracking.NewService(c)

	base := time.Unix(1000, 0).UTC()
	ids := []string{"viewed-repo", "ignored-repo", "saved-repo"}
	outcomes := []tracking.Outcome{tracking.OutcomeViewed, tracking.OutcomeIgnored, tracking.OutcomeSaved}
	for i := range ids {
		_, err := svc.RecordTriageEvent(ctx, &tracking.TriageEvent{
			ID:            ids[i],
			TargetKind:    tracking.TargetRepository,
			TargetRef:     "owner/repo",
			Outcome:       outcomes[i],
			SourceEventAt: base.Add(time.Duration(i) * time.Second),
		})
		if err != nil {
			t.Fatalf("record event %d: %v", i, err)
		}
	}

	events, err := svc.ListTriageEvents(ctx, tracking.TriageEventFilter{})
	if err != nil {
		t.Fatalf("list outcomes: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	for i := 0; i < len(events)-1; i++ {
		if events[i].SourceEventAt.After(events[i+1].SourceEventAt) {
			t.Fatalf("events not ordered by source_event_at: %v after %v", events[i], events[i+1])
		}
	}
}

func TestContributionLifecyclePersists(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	invSvc := investigation.NewService(c, c)
	inv, err := invSvc.StartInvestigation(ctx, domain.RepoRef{Owner: "owner", Repo: "repo"}, "abc", "")
	if err != nil {
		t.Fatalf("start investigation: %v", err)
	}
	h, err := invSvc.RecordHypothesis(ctx, inv.ID, "panic", "desc", investigation.CategoryBug, nil)
	if err != nil {
		t.Fatalf("record hypothesis: %v", err)
	}
	opp, err := invSvc.PromoteOpportunity(ctx, h.ID, "panic", "parser", "crash", "small", 0.8)
	if err != nil {
		t.Fatalf("promote opportunity: %v", err)
	}

	svc := tracking.NewService(c)
	preparedAt := time.Unix(3000, 0).UTC()
	contribution, err := svc.RecordContribution(ctx, &tracking.Contribution{
		OpportunityID: opp.ID,
		Kind:          "issue",
		Title:         "parser panics",
		Body:          "problem description",
		PreparedAt:    preparedAt,
	})
	if err != nil {
		t.Fatalf("record contribution: %v", err)
	}
	if contribution.ID == "" {
		t.Fatal("expected durable contribution id")
	}

	outcome, err := svc.RecordContributionOutcome(ctx, &tracking.ContributionOutcome{
		ContributionID: contribution.ID,
		Outcome:        tracking.OutcomeSubmitted,
		Reason:         "opened manually",
		SourceEventAt:  time.Unix(4000, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("record contribution outcome: %v", err)
	}
	if outcome.ID == "" {
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
		t.Fatalf("list outcomes: %v", err)
	}
	if len(outcomes) != 1 || outcomes[0].Outcome != tracking.OutcomeSubmitted {
		t.Fatalf("unexpected outcomes: %+v", outcomes)
	}
}

func TestContributionRequiresExistingOpportunity(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	svc := tracking.NewService(c)
	_, err := svc.RecordContribution(ctx, &tracking.Contribution{
		OpportunityID: "missing-opp",
		Kind:          "issue",
		Title:         "x",
	})
	if err == nil {
		t.Fatal("expected error for missing opportunity")
	}
}

func TestExportImportLocalMetadataIsIdempotent(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	invSvc := investigation.NewService(c, c)
	inv, _ := invSvc.StartInvestigation(ctx, domain.RepoRef{Owner: "owner", Repo: "repo"}, "abc", "")
	h, _ := invSvc.RecordHypothesis(ctx, inv.ID, "panic", "desc", investigation.CategoryBug, nil)
	opp, _ := invSvc.PromoteOpportunity(ctx, h.ID, "panic", "parser", "crash", "small", 0.8)

	svc := tracking.NewService(c)
	repo, _ := c.ApplyRepositoryObservation(ctx, "owner", "repo", "123", time.Unix(1, 0).UTC(), `{}`)
	c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 1, "open", "bug", "body", "alice", time.Unix(2, 0).UTC(), `{}`)

	event, _ := svc.RecordTriageEvent(ctx, &tracking.TriageEvent{
		TargetKind: tracking.TargetIssue,
		TargetRef:  "owner/repo#1",
		Outcome:    tracking.OutcomeSaved,
	})
	contribution, _ := svc.RecordContribution(ctx, &tracking.Contribution{
		OpportunityID: opp.ID,
		Kind:          "issue",
		Title:         "parser panics",
		Body:          "problem description",
	})
	svc.RecordContributionOutcome(ctx, &tracking.ContributionOutcome{
		ContributionID: contribution.ID,
		Outcome:        tracking.OutcomeSubmitted,
	})

	bundle, err := svc.ExportLocalMetadata(ctx, tracking.ExportOptions{})
	if err != nil {
		t.Fatalf("export local metadata: %v", err)
	}
	if len(bundle.TriageEvents) != 1 || len(bundle.Contributions) != 1 || len(bundle.ContributionOutcomes) != 1 {
		t.Fatalf("unexpected bundle: %+v", bundle)
	}

	if err := svc.ImportLocalMetadata(ctx, bundle); err != nil {
		t.Fatalf("import local metadata: %v", err)
	}
	if err := svc.ImportLocalMetadata(ctx, bundle); err != nil {
		t.Fatalf("re-import local metadata should be idempotent: %v", err)
	}

	events, _ := svc.ListTriageEvents(ctx, tracking.TriageEventFilter{})
	if len(events) != 1 {
		t.Fatalf("expected 1 triage event after import, got %d", len(events))
	}
	if events[0].ID != event.ID {
		t.Fatalf("expected event id %q, got %q", event.ID, events[0].ID)
	}

	contribs, _ := svc.ListContributions(ctx, tracking.ContributionFilter{})
	if len(contribs) != 1 {
		t.Fatalf("expected 1 contribution after import, got %d", len(contribs))
	}

	// The imported contribution should still be linked to the original opportunity.
	if contribs[0].OpportunityID != opp.ID {
		t.Fatalf("expected contribution opportunity %q, got %q", opp.ID, contribs[0].OpportunityID)
	}
}

func TestImportLocalMetadataRejectsInvalidBundleBeforeWriting(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	svc := tracking.NewService(c)
	bundle := &tracking.Bundle{TriageEvents: []*tracking.TriageEvent{
		{ID: "valid", TargetKind: tracking.TargetRepository, TargetRef: "owner/repo", Outcome: tracking.OutcomeSaved},
		nil,
	}}

	if err := svc.ImportLocalMetadata(ctx, bundle); err == nil {
		t.Fatal("expected invalid bundle error")
	}
	events, err := svc.ListTriageEvents(ctx, tracking.TriageEventFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("import wrote %d events before rejecting the bundle", len(events))
	}
}

func TestExportRedactsSecretsAndLocalPaths(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	svc := tracking.NewService(c)

	_, err := svc.RecordTriageEvent(ctx, &tracking.TriageEvent{
		TargetKind: tracking.TargetRepository,
		TargetRef:  "owner/repo",
		Outcome:    tracking.OutcomeIgnored,
		Reason:     "token: ghp_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa and path /home/user/secret",
	})
	if err != nil {
		t.Fatalf("record triage event: %v", err)
	}

	bundle, err := svc.ExportLocalMetadata(ctx, tracking.ExportOptions{})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(bundle.TriageEvents) != 1 {
		t.Fatalf("expected 1 event, got %d", len(bundle.TriageEvents))
	}
	reason := bundle.TriageEvents[0].Reason
	if containsAny(reason, []string{"ghp_", "/home/user/"}) {
		t.Fatalf("export reason not redacted: %s", reason)
	}
	if !containsAny(reason, []string{"[REDACTED]", "[REDACTED_PATH]"}) {
		t.Fatalf("expected redaction markers in reason: %s", reason)
	}
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if contains(s, sub) {
			return true
		}
	}
	return false
}

func contains(s, sub string) bool {
	return len(sub) > 0 && strings.Contains(s, sub)
}
