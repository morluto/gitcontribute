package app

import (
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/research"
)

func TestLoadOpportunityEvidenceIncludesMatchedValidationObservation(t *testing.T) {
	fixture := newResearchFixture(t)
	ctx := fixture.ctx
	svc := fixture.svc
	started, err := svc.StartInvestigationFromThread(ctx, research.ThreadRef{
		Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, Kind: domain.IssueKind, Number: 1,
	})
	if err != nil {
		t.Fatalf("start investigation: %v", err)
	}
	opp, err := svc.PromoteOpportunity(ctx, started.Hypothesis.ID, "buffer is undersized", "pipeline", "invalid output", "small", 0.9)
	if err != nil {
		t.Fatalf("promote opportunity: %v", err)
	}
	now := time.Now().UTC()
	def := &evidence.ValidationDefinition{
		ID: "def", InvestigationID: started.Investigation.ID,
		HypothesisID: started.Hypothesis.ID, OpportunityID: opp.ID,
		Command: []string{"test"}, WorkingDir: "/tmp", CreatedAt: now,
	}
	if err := svc.corpus.SaveValidationDefinition(ctx, def); err != nil {
		t.Fatalf("save definition: %v", err)
	}
	run := &evidence.ValidationRun{
		ID: "run", DefinitionID: def.ID, InvestigationID: started.Investigation.ID,
		HypothesisID: started.Hypothesis.ID, OpportunityID: opp.ID,
		Kind: evidence.RunKindBase, Classification: evidence.RunClassificationFailing,
		ObservationStatus: evidence.ObservationMatched,
		Observations: []evidence.ObservationResult{{
			ExpectedObservation: evidence.ExpectedObservation{Name: "undersized buffer"},
			Status:              evidence.ObservationMatched, Excerpt: "!buffer<3>",
		}},
		StartedAt: now, CompletedAt: now,
	}
	if err := svc.corpus.SaveValidationRun(ctx, run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	item := &evidence.Evidence{
		ID: "evidence", InvestigationID: started.Investigation.ID,
		HypothesisID: started.Hypothesis.ID, OpportunityID: opp.ID, ValidationRunID: run.ID,
		Type: evidence.EvidenceTypeBaseFailingRegression, Relation: evidence.RelationSupporting,
		Description: "Base fails for the expected reason.", CreatedAt: now,
	}
	if err := svc.corpus.SaveEvidence(ctx, item); err != nil {
		t.Fatalf("save evidence: %v", err)
	}

	items, err := svc.loadOpportunityEvidence(ctx, svc.corpus, opp.ID)
	if err != nil {
		t.Fatalf("load evidence: %v", err)
	}
	if len(items) != 1 || !strings.Contains(items[0].Description, `Matched observation "undersized buffer": !buffer<3>`) {
		t.Fatalf("evidence = %#v, want matched observation", items)
	}
}
