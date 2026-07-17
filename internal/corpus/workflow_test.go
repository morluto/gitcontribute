package corpus

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/morluto/gitcontribute/internal/contribution"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/investigation"
)

func TestContributionWorkflowPersistsAcrossReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "workflow.db")
	c, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	invService := investigation.NewService(c, c)
	inv, err := invService.StartInvestigation(ctx, domain.RepoRef{Owner: "owner", Repo: "repo"}, "abc123", "go")
	if err != nil {
		t.Fatal(err)
	}
	hypothesis, err := invService.RecordHypothesis(ctx, inv.ID, "panic", "reproducible panic", investigation.CategoryBug, []domain.SourceRef{{Source: "github", URL: "https://github.com/owner/repo/issues/1"}})
	if err != nil {
		t.Fatal(err)
	}
	opportunity, err := invService.PromoteOpportunity(ctx, hypothesis.ID, "panic on valid input", "parser", "crash", "small", 0.8)
	if err != nil {
		t.Fatal(err)
	}
	evidenceService := evidence.NewService(c, evidence.NewExecRunner())
	proof := &evidence.Evidence{OpportunityID: opportunity.ID, Type: evidence.EvidenceTypeMinimalReproduction, Relation: evidence.RelationSupporting, Description: "base crashes"}
	if err := evidenceService.CreateEvidence(ctx, proof); err != nil {
		t.Fatal(err)
	}
	draft := &contribution.IssueDraft{OpportunityID: opportunity.ID, Title: "parser panics", Body: "proof", RenderedAt: opportunity.CreatedAt}
	if err := c.SaveIssueDraft(ctx, draft); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}

	c, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	gotOpportunity, err := c.GetOpportunity(ctx, opportunity.ID)
	if err != nil || gotOpportunity.ProblemStatement != opportunity.ProblemStatement {
		t.Fatalf("GetOpportunity = (%+v, %v)", gotOpportunity, err)
	}
	proofs, err := c.ListEvidence(ctx, evidence.EvidenceFilter{OpportunityID: opportunity.ID})
	if err != nil || len(proofs) != 1 || proofs[0].Description != "base crashes" {
		t.Fatalf("ListEvidence = (%+v, %v)", proofs, err)
	}
	gotDraft, err := c.GetIssueDraft(ctx, opportunity.ID)
	if err != nil || gotDraft.Body != "proof" {
		t.Fatalf("GetIssueDraft = (%+v, %v)", gotDraft, err)
	}
}

func TestFindRelatedUsesRepositoryAndCategory(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	svc := investigation.NewService(c, c)
	inv, _ := svc.StartInvestigation(ctx, domain.RepoRef{Owner: "owner", Repo: "repo"}, "sha", "")
	_, err := svc.RecordHypothesis(ctx, inv.ID, "bug", "description", investigation.CategoryBug, []domain.SourceRef{{Source: "issue", URL: "https://github.com/owner/repo/issues/2"}})
	if err != nil {
		t.Fatal(err)
	}
	related, err := c.FindRelated(ctx, inv.Repo, investigation.CategoryBug)
	if err != nil || len(related) != 1 {
		t.Fatalf("FindRelated = (%+v, %v)", related, err)
	}
	other, err := c.FindRelated(ctx, domain.RepoRef{Owner: "other", Repo: "repo"}, investigation.CategoryBug)
	if err != nil || len(other) != 0 {
		t.Fatalf("other FindRelated = (%+v, %v)", other, err)
	}
}

func TestInvestigationAndOpportunityListQueries(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	svc := investigation.NewService(c, c)

	invA, err := svc.StartInvestigation(ctx, domain.RepoRef{Owner: "owner", Repo: "a"}, "sha-a", "")
	if err != nil {
		t.Fatal(err)
	}
	invB, err := svc.StartInvestigation(ctx, domain.RepoRef{Owner: "owner", Repo: "b"}, "sha-b", "")
	if err != nil {
		t.Fatal(err)
	}

	invA2, err := svc.ListInvestigations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(invA2) != 2 {
		t.Fatalf("expected 2 investigations, got %d", len(invA2))
	}

	hA, err := svc.RecordHypothesis(ctx, invA.ID, "bug in a", "desc", investigation.CategoryBug, nil)
	if err != nil {
		t.Fatal(err)
	}
	hB, err := svc.RecordHypothesis(ctx, invB.ID, "bug in b", "desc", investigation.CategoryBug, nil)
	if err != nil {
		t.Fatal(err)
	}

	opA, err := svc.PromoteOpportunity(ctx, hA.ID, "problem a", "pkg/a", "crash", "small", 0.7)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.PromoteOpportunity(ctx, hB.ID, "problem b", "pkg/b", "crash", "small", 0.6)
	if err != nil {
		t.Fatal(err)
	}

	all, err := svc.ListOpportunities(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 opportunities, got %d", len(all))
	}

	filtered, err := svc.ListOpportunities(ctx, invA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0].ID != opA.ID {
		t.Fatalf("expected 1 opportunity for invA, got %+v", filtered)
	}
}
