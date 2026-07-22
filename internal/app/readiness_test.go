package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/investigation"
	"github.com/morluto/gitcontribute/internal/research"
)

func TestCandidateImprovesBaselineRequiresMatchedObservations(t *testing.T) {
	t.Parallel()
	evaluator := &readinessEvaluator{
		ctx: context.Background(), opportunity: &investigation.Opportunity{ID: "opp"},
		runs: []*evidence.ValidationRun{
			{
				ID: "base", DefinitionID: "def", Kind: evidence.RunKindBase,
				Classification:    evidence.RunClassificationFailing,
				ObservationStatus: evidence.ObservationMismatched,
			},
			{
				ID: "candidate", DefinitionID: "def", Kind: evidence.RunKindCandidate,
				Classification:    evidence.RunClassificationPassing,
				ObservationStatus: evidence.ObservationMatched,
			},
		},
	}
	check, err := evaluator.candidateImprovesBaseline()
	if err != nil {
		t.Fatalf("candidate check: %v", err)
	}
	if check.Status != readinessWarn || !strings.Contains(check.Summary, "observation contract") {
		t.Fatalf("check = %+v, want observation warning", check)
	}
}

func TestOpportunityReadinessReportsPassWarnBlockUnknown(t *testing.T) {
	t.Parallel()
	fixture := newResearchFixture(t)
	started, err := fixture.svc.StartInvestigationFromThread(fixture.ctx, research.ThreadRef{
		Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, Kind: domain.IssueKind, Number: 1,
	})
	if err != nil {
		t.Fatalf("start from thread: %v", err)
	}
	opp, err := fixture.svc.PromoteOpportunity(fixture.ctx, started.Hypothesis.ID, "parser cancellation is unbounded", "parser", "hang", "small", 0.8)
	if err != nil {
		t.Fatalf("promote opportunity: %v", err)
	}
	if _, err := fixture.svc.UpdateOpportunityCollisionStatus(fixture.ctx, opp.ID, string(investigation.CollisionPossible), "similar PR may exist"); err != nil {
		t.Fatalf("update collision: %v", err)
	}

	supporting, err := fixture.svc.RecordEvidence(fixture.ctx, RecordEvidenceInput{
		OpportunityID: opp.ID,
		Type:          string(evidence.EvidenceTypeGitHubSource),
		Relation:      string(evidence.RelationSupporting),
		Description:   "maintainer requested a cancellation regression test",
	})
	if err != nil {
		t.Fatalf("record supporting evidence: %v", err)
	}
	contradicting, err := fixture.svc.RecordEvidence(fixture.ctx, RecordEvidenceInput{
		OpportunityID: opp.ID,
		Type:          string(evidence.EvidenceTypeManualObservation),
		Relation:      string(evidence.RelationContradicting),
		Description:   "local smoke test did not reproduce the hang",
	})
	if err != nil {
		t.Fatalf("record contradicting evidence: %v", err)
	}

	def := &evidence.ValidationDefinition{
		ID:              "def-readiness",
		InvestigationID: started.Investigation.ID,
		HypothesisID:    started.Hypothesis.ID,
		OpportunityID:   opp.ID,
		Command:         []string{"go", "test", "./..."},
		WorkingDir:      "/repo",
		CreatedAt:       fixture.now,
	}
	if err := fixture.svc.corpus.SaveValidationDefinition(fixture.ctx, def); err != nil {
		t.Fatalf("save validation definition: %v", err)
	}
	for _, run := range []*evidence.ValidationRun{
		{
			ID: "base-run", DefinitionID: def.ID, InvestigationID: started.Investigation.ID,
			HypothesisID: started.Hypothesis.ID, OpportunityID: opp.ID,
			Kind: evidence.RunKindBase, Classification: evidence.RunClassificationFailing,
			StartedAt: fixture.now, CompletedAt: fixture.now.Add(time.Minute),
		},
		{
			ID: "candidate-run", DefinitionID: def.ID, InvestigationID: started.Investigation.ID,
			HypothesisID: started.Hypothesis.ID, OpportunityID: opp.ID,
			Kind: evidence.RunKindCandidate, Classification: evidence.RunClassificationFailing,
			StartedAt: fixture.now.Add(2 * time.Minute), CompletedAt: fixture.now.Add(3 * time.Minute),
		},
	} {
		if err := fixture.svc.corpus.SaveValidationRun(fixture.ctx, run); err != nil {
			t.Fatalf("save validation run %s: %v", run.ID, err)
		}
	}

	if _, err := fixture.svc.PrepareIssue(fixture.ctx, opp.ID, cli.PrepareIssueOptions{Success: "passing cancellation regression"}); err != nil {
		t.Fatalf("prepare issue: %v", err)
	}

	thread, err := fixture.svc.corpus.GetThreadByNumber(fixture.ctx, fixture.repoID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.svc.corpus.UpsertThread(fixture.ctx, corpus.Thread{
		RepositoryID: fixture.repoID, Kind: corpus.ThreadKindIssue, Number: 1, State: "open",
		Title: "Retry parser cancellation updated", Body: thread.Body, Author: thread.Author,
		AuthorAssociation: thread.AuthorAssociation, Labels: thread.Labels,
		SourceCreatedAt: thread.SourceCreatedAt, SourceUpdatedAt: fixture.now.Add(time.Hour),
	}, `{"revision":"newer"}`); err != nil {
		t.Fatal(err)
	}

	before := captureReadinessCorpusState(t, fixture, opp.ID)

	report, err := fixture.svc.OpportunityReadiness(fixture.ctx, opp.ID)
	if err != nil {
		t.Fatalf("readiness: %v", err)
	}
	if report.Status != readinessBlock || report.RuleSetVersion != readinessRuleSetVersion {
		t.Fatalf("unexpected report header: %+v", report)
	}
	wantOrder := []string{
		"repository_archived", "target_thread_open", "baseline_freshness", "guidance_present", "collision_status",
		"validation_presence", "candidate_improves_baseline", "evidence_freshness", "contradicting_evidence", "draft_references_evidence",
	}
	if got := readinessRuleIDs(report.Checks); strings.Join(got, ",") != strings.Join(wantOrder, ",") {
		t.Fatalf("rule order = %v", got)
	}
	status := readinessStatusByRule(report.Checks)
	assertReadinessStatus(t, status, "repository_archived", readinessPass)
	assertReadinessStatus(t, status, "target_thread_open", readinessPass)
	assertReadinessStatus(t, status, "baseline_freshness", readinessWarn)
	assertReadinessStatus(t, status, "guidance_present", readinessUnknown)
	assertReadinessStatus(t, status, "collision_status", readinessWarn)
	assertReadinessStatus(t, status, "validation_presence", readinessPass)
	assertReadinessStatus(t, status, "candidate_improves_baseline", readinessBlock)
	assertReadinessStatus(t, status, "evidence_freshness", readinessWarn)
	assertReadinessStatus(t, status, "contradicting_evidence", readinessBlock)
	assertReadinessStatus(t, status, "draft_references_evidence", readinessPass)
	if !readinessRefsContain(report.Checks, "evidence_freshness", "evidence:"+supporting.ID) {
		t.Fatalf("stale evidence ref missing from report: %+v", report.Checks)
	}
	if !readinessRefsContain(report.Checks, "contradicting_evidence", "evidence:"+contradicting.ID) {
		t.Fatalf("contradicting evidence ref missing from report: %+v", report.Checks)
	}

	explained, err := fixture.svc.ExplainReadiness(fixture.ctx, opp.ID+":candidate_improves_baseline")
	if err != nil {
		t.Fatalf("explain readiness: %v", err)
	}
	if explained.RuleID != "candidate_improves_baseline" || explained.Status != readinessBlock {
		t.Fatalf("unexpected explained check: %+v", explained)
	}

	after := captureReadinessCorpusState(t, fixture, opp.ID)
	assertReadinessDidNotMutateCorpus(t, before, after)
}

type readinessCorpusState struct {
	evidenceCount int
	runCount      int
	draftBody     string
}

func captureReadinessCorpusState(t *testing.T, fixture researchFixture, opportunityID string) readinessCorpusState {
	t.Helper()
	items, err := fixture.svc.corpus.ListEvidence(fixture.ctx, evidence.EvidenceFilter{OpportunityID: opportunityID})
	if err != nil {
		t.Fatal(err)
	}
	runs, err := fixture.svc.corpus.ListValidationRuns(fixture.ctx, opportunityID)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := fixture.svc.corpus.GetIssueDraft(fixture.ctx, opportunityID)
	if err != nil {
		t.Fatal(err)
	}
	if draft == nil {
		t.Fatal("expected issue draft")
	}
	return readinessCorpusState{evidenceCount: len(items), runCount: len(runs), draftBody: draft.Body}
}

func assertReadinessDidNotMutateCorpus(t *testing.T, before, after readinessCorpusState) {
	t.Helper()
	if before != after {
		t.Fatalf("readiness mutated corpus state: before %+v after %+v", before, after)
	}
}

func readinessRuleIDs(checks []cli.ReadinessCheck) []string {
	out := make([]string, len(checks))
	for i, check := range checks {
		out[i] = check.RuleID
	}
	return out
}

func readinessStatusByRule(checks []cli.ReadinessCheck) map[string]string {
	out := make(map[string]string, len(checks))
	for _, check := range checks {
		out[check.RuleID] = check.Status
	}
	return out
}

func assertReadinessStatus(t *testing.T, got map[string]string, ruleID, want string) {
	t.Helper()
	if got[ruleID] != want {
		t.Fatalf("%s status = %q, want %q", ruleID, got[ruleID], want)
	}
}

func readinessRefsContain(checks []cli.ReadinessCheck, ruleID, ref string) bool {
	for _, check := range checks {
		if check.RuleID != ruleID {
			continue
		}
		for _, got := range check.EvidenceRefs {
			if got == ref {
				return true
			}
		}
	}
	return false
}
