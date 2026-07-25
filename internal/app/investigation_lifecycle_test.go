package app

import (
	"context"
	"testing"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/investigation"
)

func TestCreateAndUpdateHypothesis(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newLocalService(t)
	defer func() { _ = svc.Close() }()

	inv, err := svc.StartInvestigation(ctx, contracts.RepoRef{Owner: "owner", Repo: "repo"}, "abc", "")
	if err != nil {
		t.Fatalf("start investigation: %v", err)
	}
	h, err := svc.CreateHypothesis(ctx, inv.ID, investigation.CreateHypothesisInput{
		Title:              "race in parser",
		Description:        "data race under load",
		Category:           investigation.CategoryBug,
		ExpectedBehavior:   "parser should not panic",
		ObservedBehavior:   "parser panics",
		PotentialImpact:    "crash",
		OpenQuestions:      []string{"reproducible?"},
		AffectedComponents: []string{"pkg/parser"},
		SourceRefs: []domain.SourceRef{
			{Source: "github", URL: "https://github.com/owner/repo/issues/1"},
		},
	})
	if err != nil {
		t.Fatalf("create hypothesis: %v", err)
	}
	if h.Status != investigation.HypothesisProposed {
		t.Fatalf("unexpected status: %q", h.Status)
	}
	if len(h.SourceRefs) != 1 || h.ExpectedBehavior == "" {
		t.Fatalf("structured fields missing: %+v", h)
	}

	updated, err := svc.UpdateHypothesis(ctx, h.ID, investigation.UpdateHypothesisInput{
		Title:       "race in parser (confirmed)",
		Description: "data race under load",
		Category:    investigation.CategoryBug,
		Rationale:   "confirmed by stress test",
	})
	if err != nil {
		t.Fatalf("update hypothesis: %v", err)
	}
	if updated.Title != "race in parser (confirmed)" || len(updated.AuditTrail) != 1 {
		t.Fatalf("update failed: %+v", updated)
	}

	trans, err := svc.TransitionHypothesis(ctx, h.ID, "rejected", "not reproducible")
	if err != nil {
		t.Fatalf("transition hypothesis: %v", err)
	}
	if trans.Status != investigation.HypothesisRejected {
		t.Fatalf("unexpected status after transition: %q", trans.Status)
	}
}

func TestRecordEvidence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newLocalService(t)
	defer func() { _ = svc.Close() }()

	inv, err := svc.StartInvestigation(ctx, contracts.RepoRef{Owner: "owner", Repo: "repo"}, "abc", "")
	if err != nil {
		t.Fatalf("start investigation: %v", err)
	}
	h, err := svc.CreateHypothesis(ctx, inv.ID, investigation.CreateHypothesisInput{
		Title: "race", Description: "desc", Category: investigation.CategoryBug,
	})
	if err != nil {
		t.Fatalf("create hypothesis: %v", err)
	}
	e, err := svc.RecordEvidence(ctx, contracts.RecordEvidenceInput{
		HypothesisID: h.ID,
		Type:         string(evidence.EvidenceTypeManualObservation),
		Relation:     string(evidence.RelationSupporting),
		Description:  "stress test reproduces panic",
	})
	if err != nil {
		t.Fatalf("record evidence: %v", err)
	}
	if e.InvestigationID != inv.ID || e.HypothesisID != h.ID || e.OpportunityID != "" {
		t.Fatalf("evidence scope wrong: %+v", e)
	}
	if e.Type != evidence.EvidenceTypeManualObservation || e.Relation != evidence.RelationSupporting {
		t.Fatalf("evidence fields wrong: %+v", e)
	}
}

func TestPromoteOpportunityWithDependencies(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newLocalService(t)
	defer func() { _ = svc.Close() }()

	inv, err := svc.StartInvestigation(ctx, contracts.RepoRef{Owner: "owner", Repo: "repo"}, "abc", "")
	if err != nil {
		t.Fatalf("start investigation: %v", err)
	}
	h, err := svc.CreateHypothesis(ctx, inv.ID, investigation.CreateHypothesisInput{
		Title: "race", Description: "desc", Category: investigation.CategoryBug,
	})
	if err != nil {
		t.Fatalf("create hypothesis: %v", err)
	}
	o, err := svc.PromoteOpportunityWithInput(ctx, h.ID, investigation.PromoteOpportunityInput{
		ProblemStatement:    "parser panics",
		Scope:               "pkg/parser",
		Impact:              "crash",
		ExpectedEffort:      "small",
		Confidence:          0.8,
		Dependencies:        []string{"go1.22"},
		MaintainerAlignment: "maintainer confirmed scope",
	})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if o.Status != investigation.OpportunityHypothesis {
		t.Fatalf("unexpected status: %q", o.Status)
	}
	if len(o.Dependencies) != 1 || o.MaintainerAlignment == "" {
		t.Fatalf("missing opportunity fields: %+v", o)
	}
	if len(o.EvidenceIDs) != 1 {
		t.Fatalf("expected maintainer-alignment evidence, got %+v", o.EvidenceIDs)
	}
	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	items, err := c.ListEvidence(ctx, evidence.EvidenceFilter{OpportunityID: o.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != o.EvidenceIDs[0] {
		t.Fatalf("promotion evidence was not stored atomically: %+v", items)
	}
}
