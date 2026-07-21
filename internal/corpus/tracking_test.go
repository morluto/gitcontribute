package corpus

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
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
	source, err := c.CurrentSourceRevision(ctx, evidence.SourceSubject{Kind: evidence.SourceSubjectRepository, Owner: "owner", Repo: "repo"})
	if err != nil || source == nil {
		t.Fatalf("current repository source = (%+v, %v)", source, err)
	}

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
	evSvc := evidence.NewService(c, evidence.NewExecRunner())
	if err := evSvc.CreateEvidence(ctx, &evidence.Evidence{
		ID:              "ev-1",
		InvestigationID: inv.ID,
		HypothesisID:    h.ID,
		OpportunityID:   opp.ID,
		Type:            evidence.EvidenceTypeGitHubSource,
		Relation:        evidence.RelationSupporting,
		Description:     "repository metadata supported the opportunity",
		SourceProvenance: []evidence.SourceRevision{
			*source,
		},
		CreatedAt: time.Unix(3, 0).UTC(),
	}); err != nil {
		t.Fatalf("create evidence: %v", err)
	}

	bundle, err := svc.ExportLocalMetadata(ctx, tracking.ExportOptions{})
	if err != nil {
		t.Fatalf("export local metadata: %v", err)
	}
	if bundle.SchemaVersion != tracking.CurrentBundleSchemaVersion ||
		len(bundle.TriageEvents) != 1 || len(bundle.Contributions) != 1 || len(bundle.ContributionOutcomes) != 1 || len(bundle.Evidence) != 1 {
		t.Fatalf("unexpected bundle: %+v", bundle)
	}
	if len(bundle.Evidence[0].SourceProvenance) != 1 {
		t.Fatalf("evidence provenance missing from bundle: %+v", bundle.Evidence[0])
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

	items, err := c.ListEvidence(ctx, evidence.EvidenceFilter{OpportunityID: opp.ID})
	if err != nil {
		t.Fatalf("list evidence: %v", err)
	}
	if len(items) != 1 || items[0].ID != "ev-1" || len(items[0].SourceProvenance) != 1 {
		t.Fatalf("unexpected imported evidence: %+v", items)
	}
}

func TestImportLocalMetadataBundleVersionCompatibility(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	svc := tracking.NewService(c)

	legacy := &tracking.Bundle{TriageEvents: []*tracking.TriageEvent{
		{ID: "legacy", TargetKind: tracking.TargetRepository, TargetRef: "owner/repo", Outcome: tracking.OutcomeSaved},
	}}
	if err := svc.ImportLocalMetadata(ctx, legacy); err != nil {
		t.Fatalf("legacy import: %v", err)
	}

	future := &tracking.Bundle{
		SchemaVersion: tracking.CurrentBundleSchemaVersion + 1,
		TriageEvents: []*tracking.TriageEvent{
			{ID: "future", TargetKind: tracking.TargetRepository, TargetRef: "owner/repo", Outcome: tracking.OutcomeIgnored},
		},
	}
	if err := svc.ImportLocalMetadata(ctx, future); err == nil {
		t.Fatal("expected future schema version error")
	}
	events, err := svc.ListTriageEvents(ctx, tracking.TriageEventFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ID != "legacy" {
		t.Fatalf("future import wrote before rejecting: %+v", events)
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

func TestImportLocalMetadataIsAtomicOnReferentialFailure(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	svc := tracking.NewService(c)
	invSvc := investigation.NewService(c, c)

	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("apply repository: %v", err)
	}
	_ = repo

	inv, err := invSvc.StartInvestigation(ctx, domain.RepoRef{Owner: "owner", Repo: "repo"}, "sha", "")
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

	bundle := &tracking.Bundle{
		TriageEvents: []*tracking.TriageEvent{
			{ID: "t1", TargetKind: tracking.TargetRepository, TargetRef: "owner/repo", Outcome: tracking.OutcomeSaved},
		},
		Contributions: []*tracking.Contribution{
			{ID: "c1", OpportunityID: opp.ID, Kind: "issue", Title: "t"},
		},
		ContributionOutcomes: []*tracking.ContributionOutcome{
			{ID: "o1", ContributionID: "missing", Outcome: tracking.OutcomeSubmitted},
		},
	}

	if err := svc.ImportLocalMetadata(ctx, bundle); err == nil {
		t.Fatal("expected error for missing contribution reference")
	}

	events, err := svc.ListTriageEvents(ctx, tracking.TriageEventFilter{})
	if err != nil {
		t.Fatalf("list triage events: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("triage events written before rollback: %d", len(events))
	}

	contribs, err := svc.ListContributions(ctx, tracking.ContributionFilter{})
	if err != nil {
		t.Fatalf("list contributions: %v", err)
	}
	if len(contribs) != 0 {
		t.Fatalf("contributions written before rollback: %d", len(contribs))
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
		Reason:     strings.Join([]string{"token", ": test-token and path /home/user/private-file"}, ""),
	})
	if err != nil {
		t.Fatalf("record triage event: %v", err)
	}
	evSvc := evidence.NewService(c, evidence.NewExecRunner())
	if err := evSvc.CreateEvidence(ctx, &evidence.Evidence{
		ID:          "ev-secret",
		Type:        evidence.EvidenceTypeManualObservation,
		Relation:    evidence.RelationSupporting,
		Description: strings.Join([]string{"api_key", "=supersecret from /home/user/private-file"}, ""),
		SourceRefs: []domain.SourceRef{
			{Source: "local", URL: "/home/user/private-file"},
		},
	}); err != nil {
		t.Fatalf("record evidence: %v", err)
	}

	bundle, err := svc.ExportLocalMetadata(ctx, tracking.ExportOptions{})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(bundle.TriageEvents) != 1 || len(bundle.Evidence) != 1 {
		t.Fatalf("unexpected export counts: %+v", bundle)
	}
	reason := bundle.TriageEvents[0].Reason
	if containsAny(reason, []string{"test-token", "/home/user/"}) {
		t.Fatalf("export reason not redacted: %s", reason)
	}
	if !containsAny(reason, []string{"[REDACTED]", "[REDACTED_PATH]"}) {
		t.Fatalf("expected redaction markers in reason: %s", reason)
	}
	evidenceItem := bundle.Evidence[0]
	if containsAny(evidenceItem.Description, []string{"supersecret", "/home/user/"}) {
		t.Fatalf("export evidence description not redacted: %s", evidenceItem.Description)
	}
	if len(evidenceItem.SourceRefs) != 1 || containsAny(evidenceItem.SourceRefs[0].URL, []string{"/home/user/"}) {
		t.Fatalf("export evidence source ref not redacted: %+v", evidenceItem.SourceRefs)
	}
}

func TestMalformedContributionMetadataIsPropagated(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	invSvc := investigation.NewService(c, c)
	inv, err := invSvc.StartInvestigation(ctx, domain.RepoRef{Owner: "owner", Repo: "repo"}, "sha", "")
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
	contribution, err := svc.RecordContribution(ctx, &tracking.Contribution{
		OpportunityID: opp.ID,
		Kind:          "issue",
		Title:         "title",
	})
	if err != nil {
		t.Fatalf("record contribution: %v", err)
	}

	if _, err := c.db.ExecContext(ctx, `UPDATE contributions SET payload = ? WHERE id = ?`, "not-json", contribution.ID); err != nil {
		t.Fatalf("corrupt contribution payload: %v", err)
	}

	if _, err := svc.GetContribution(ctx, contribution.ID); err == nil {
		t.Fatal("expected error getting contribution with malformed metadata")
	}
	if _, err := svc.ListContributions(ctx, tracking.ContributionFilter{}); err == nil {
		t.Fatal("expected error listing contributions with malformed metadata")
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
