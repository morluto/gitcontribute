package corpus

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/contribution"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/investigation"
	"github.com/morluto/gitcontribute/internal/manifest"
	"github.com/morluto/gitcontribute/internal/workspace"
)

func TestContributionManifestPersistsAndSelectsLatest(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	svc := investigation.NewService(c, c)
	inv, err := svc.StartInvestigation(ctx, domain.MustRepoRef("owner", "repo"), "sha", "")
	if err != nil {
		t.Fatal(err)
	}
	hypothesis, err := svc.RecordHypothesis(ctx, inv.ID, "proof", "description", investigation.CategoryBug, nil)
	if err != nil {
		t.Fatal(err)
	}
	opportunity, err := svc.PromoteOpportunity(ctx, hypothesis.ID, "problem", "scope", "impact", "small", 0.8)
	if err != nil {
		t.Fatal(err)
	}
	statement, err := manifest.Finalize(manifest.Predicate{
		GeneratedAt: time.Unix(100, 0).UTC(), Repository: manifest.RepositoryIdentity{Owner: "owner", Repo: "repo", CommitSHA: "sha"},
		Opportunity: manifest.OpportunityRecord{ID: opportunity.ID, InvestigationID: inv.ID}, Status: "incomplete",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SaveContributionManifest(ctx, &statement, "", ""); err != nil {
		t.Fatal(err)
	}
	got, err := c.GetContributionManifest(ctx, statement.Predicate.ManifestID)
	if err != nil {
		t.Fatal(err)
	}
	latest, err := c.LatestContributionManifest(ctx, opportunity.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Predicate.ContentSHA256 != statement.Predicate.ContentSHA256 || latest.Predicate.ManifestID != statement.Predicate.ManifestID {
		t.Fatalf("manifest roundtrip mismatch: got=%+v latest=%+v", got.Predicate, latest.Predicate)
	}
	tampered := statement
	tampered.Predicate.Opportunity.ProblemStatement = "tampered"
	payload, err := json.Marshal(tampered)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.ExecContext(ctx, `UPDATE contribution_manifests SET payload=? WHERE id=?`, payload, statement.Predicate.ManifestID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetContributionManifest(ctx, statement.Predicate.ManifestID); err == nil {
		t.Fatal("tampered persisted manifest passed validation")
	}
}

func TestUnmarshalWorkflowRejectsInvalidLifecycleDiscriminators(t *testing.T) {
	ref := domain.MustRepoRef("owner", "repo")
	tests := []struct {
		name   string
		value  any
		target func() any
	}{
		{
			name:   "investigation status",
			value:  &investigation.Investigation{ID: "inv", Repo: ref, Status: "impossible"},
			target: func() any { return &investigation.Investigation{} },
		},
		{
			name:   "hypothesis category",
			value:  &investigation.Hypothesis{ID: "hyp", InvestigationID: "inv", Category: "impossible", Status: investigation.HypothesisProposed},
			target: func() any { return &investigation.Hypothesis{} },
		},
		{
			name:   "opportunity collision",
			value:  &investigation.Opportunity{ID: "opp", InvestigationID: "inv", HypothesisID: "hyp", Category: investigation.CategoryBug, Status: investigation.OpportunityHypothesis, CollisionStatus: "impossible"},
			target: func() any { return &investigation.Opportunity{} },
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			payload, err := marshalWorkflow(testCase.value)
			if err != nil {
				t.Fatal(err)
			}
			if err := unmarshalWorkflow(payload, testCase.target()); err == nil {
				t.Fatal("invalid durable discriminator was accepted")
			}
		})
	}
}

func TestSaveExternalValidationRollsBackDefinitionWhenRunFails(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	definition := &evidence.ValidationDefinition{ID: "external-definition", CreatedAt: time.Now().UTC()}
	run := &evidence.ValidationRun{
		ID: "external-run", DefinitionID: "different-definition", Kind: evidence.RunKindCandidate,
		Classification: evidence.RunClassificationPassing, ObservationStatus: evidence.ObservationNotEvaluated,
	}
	if err := c.SaveExternalValidation(ctx, definition, run); err == nil {
		t.Fatal("expected invalid run foreign key to fail")
	}
	if _, err := c.GetValidationDefinition(ctx, definition.ID); !errors.Is(err, evidence.ErrNotFound) {
		t.Fatalf("definition survived failed atomic save: %v", err)
	}
}

func TestSaveExternalValidationRejectsMismatchedExistingDefinition(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	paired := &evidence.ValidationDefinition{ID: "paired-definition", CreatedAt: time.Now().UTC()}
	other := &evidence.ValidationDefinition{ID: "other-definition", CreatedAt: time.Now().UTC()}
	if err := c.SaveValidationDefinition(ctx, other); err != nil {
		t.Fatal(err)
	}
	run := &evidence.ValidationRun{
		ID: "external-run", DefinitionID: other.ID, Kind: evidence.RunKindCandidate,
		Classification: evidence.RunClassificationPassing, ObservationStatus: evidence.ObservationNotEvaluated,
	}
	if err := c.SaveExternalValidation(ctx, paired, run); err == nil {
		t.Fatal("external validation accepted a run for a different definition")
	}
	if _, err := c.GetValidationDefinition(ctx, paired.ID); !errors.Is(err, evidence.ErrNotFound) {
		t.Fatalf("paired definition survived rejected save: %v", err)
	}
}

func TestOpportunityStoredParserAcceptsLegacyInitialCollisionAudit(t *testing.T) {
	item := &investigation.Opportunity{
		ID: "opp", InvestigationID: "inv", HypothesisID: "hyp", Category: investigation.CategoryBug,
		Status: investigation.OpportunityHypothesis, CollisionStatus: investigation.CollisionPossible,
		AuditTrail: []investigation.StatusChange{{From: "", To: string(investigation.CollisionPossible)}},
	}
	payload, err := marshalWorkflow(item)
	if err != nil {
		t.Fatal(err)
	}
	var decoded investigation.Opportunity
	if err := unmarshalWorkflow(payload, &decoded); err != nil {
		t.Fatal(err)
	}
}

func TestSaveEvidenceBatchRollsBackEarlierItemsOnFailure(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	first := &evidence.Evidence{
		ID: "first", Type: evidence.EvidenceTypeManualObservation,
		Relation: evidence.RelationSupporting, Description: "first claim",
	}
	if err := c.SaveEvidenceBatch(ctx, []*evidence.Evidence{first, nil}); err == nil {
		t.Fatal("expected invalid batch item to fail")
	}
	items, err := c.ListEvidence(ctx, evidence.EvidenceFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("evidence survived failed batch: %+v", items)
	}
}

func TestContributionWorkflowPersistsAcrossReopen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "workflow.db")
	c, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	invService := investigation.NewService(c, c)
	inv, err := invService.StartInvestigation(ctx, domain.MustRepoRef("owner", "repo"), "abc123", "go")
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
	byInvestigation, err := c.ListEvidence(ctx, evidence.EvidenceFilter{InvestigationID: inv.ID})
	if err != nil || len(byInvestigation) != 1 || byInvestigation[0].ID != proof.ID {
		t.Fatalf("investigation ListEvidence = (%+v, %v)", byInvestigation, err)
	}
	gotDraft, err := c.GetIssueDraft(ctx, opportunity.ID)
	if err != nil || gotDraft.Body != "proof" {
		t.Fatalf("GetIssueDraft = (%+v, %v)", gotDraft, err)
	}
}

func TestFindRelatedUsesRepositoryAndCategory(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	svc := investigation.NewService(c, c)
	inv, _ := svc.StartInvestigation(ctx, domain.MustRepoRef("owner", "repo"), "sha", "")
	_, err := svc.RecordHypothesis(ctx, inv.ID, "bug", "description", investigation.CategoryBug, []domain.SourceRef{{Source: "issue", URL: "https://github.com/owner/repo/issues/2"}})
	if err != nil {
		t.Fatal(err)
	}
	related, err := c.FindRelated(ctx, inv.Repo, investigation.CategoryBug)
	if err != nil || len(related) != 1 {
		t.Fatalf("FindRelated = (%+v, %v)", related, err)
	}
	other, err := c.FindRelated(ctx, domain.MustRepoRef("other", "repo"), investigation.CategoryBug)
	if err != nil || len(other) != 0 {
		t.Fatalf("other FindRelated = (%+v, %v)", other, err)
	}
}

func TestPromoteHypothesisRollsBackOnOpportunityConflict(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	svc := investigation.NewService(c, c)
	inv, err := svc.StartInvestigation(ctx, domain.MustRepoRef("owner", "repo"), "sha", "")
	if err != nil {
		t.Fatal(err)
	}
	first, err := svc.RecordHypothesis(ctx, inv.ID, "first", "description", investigation.CategoryBug, nil)
	if err != nil {
		t.Fatal(err)
	}
	existing, err := svc.PromoteOpportunity(ctx, first.ID, "first problem", "scope", "impact", "small", 0.8)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.RecordHypothesis(ctx, inv.ID, "second", "description", investigation.CategoryBug, nil)
	if err != nil {
		t.Fatal(err)
	}
	promoted := *second
	if err := promoted.Transition(investigation.HypothesisPromoted, "promotion test"); err != nil {
		t.Fatal(err)
	}
	conflict := &investigation.Opportunity{
		ID: existing.ID, InvestigationID: inv.ID, HypothesisID: second.ID,
		Title: second.Title, ProblemStatement: "second problem", Category: second.Category,
		Status: investigation.OpportunityHypothesis, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := c.PromoteHypothesis(ctx, &promoted, conflict); err == nil {
		t.Fatal("expected duplicate opportunity failure")
	}
	stored, err := c.GetHypothesis(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != investigation.HypothesisProposed {
		t.Fatalf("promotion rollback left hypothesis in %s", stored.Status)
	}
}

func TestPromoteHypothesisRejectsStaleConcurrentPromotion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	svc := investigation.NewService(c, c)
	inv, _ := svc.StartInvestigation(ctx, domain.MustRepoRef("owner", "repo"), "sha", "")
	hypothesis, _ := svc.RecordHypothesis(ctx, inv.ID, "race", "description", investigation.CategoryBug, nil)
	stale := *hypothesis
	if _, err := svc.PromoteOpportunity(ctx, hypothesis.ID, "first problem", "scope", "impact", "small", 0.8); err != nil {
		t.Fatal(err)
	}
	if err := stale.Transition(investigation.HypothesisPromoted, "stale promotion"); err != nil {
		t.Fatal(err)
	}
	staleOpportunity := &investigation.Opportunity{
		ID: "stale-opportunity", InvestigationID: inv.ID, HypothesisID: hypothesis.ID,
		Title: hypothesis.Title, ProblemStatement: "second problem", Category: hypothesis.Category,
		Status: investigation.OpportunityHypothesis, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	err := c.PromoteHypothesis(ctx, &stale, staleOpportunity)
	if !errors.Is(err, investigation.ErrInvalidTransition) {
		t.Fatalf("stale promotion error = %v", err)
	}
	if opportunity, err := c.GetOpportunity(ctx, staleOpportunity.ID); !errors.Is(err, investigation.ErrNotFound) || opportunity != nil {
		t.Fatalf("stale opportunity = (%+v, %v)", opportunity, err)
	}
}

func TestInvestigationAndOpportunityListQueries(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	svc := investigation.NewService(c, c)

	invA, err := svc.StartInvestigation(ctx, domain.MustRepoRef("owner", "a"), "sha-a", "")
	if err != nil {
		t.Fatal(err)
	}
	invB, err := svc.StartInvestigation(ctx, domain.MustRepoRef("owner", "b"), "sha-b", "")
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

func TestWorkspacePersistsAcrossReopen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "workspace.db")
	c, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}

	ws := &workspace.Workspace{
		Name:            "ws-1",
		InvestigationID: "inv-1",
		Path:            "/tmp/ws",
		Remote:          "https://github.com/o/r.git",
		BaseSHA:         "base-sha",
		CandidateSHA:    "candidate-sha",
		MergeBase:       "merge-base",
		CreatedAt:       time.Now().UTC(),
	}
	if err := c.SaveWorkspace(ctx, ws); err != nil {
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

	got, err := c.GetWorkspace(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "ws-1" || got.InvestigationID != "inv-1" || got.Remote != ws.Remote {
		t.Fatalf("workspace roundtrip failed: %+v", got)
	}

	_, err = c.GetWorkspace(ctx, "missing")
	if !errors.Is(err, workspace.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestBindWorkspacePathSerializesCompetingNames(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, err := Open(ctx, filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	start := make(chan struct{})
	type result struct {
		bound    *workspace.Workspace
		inserted bool
		err      error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, name := range []string{"first", "second"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			bound, inserted, err := c.BindWorkspacePath(ctx, &workspace.Workspace{Name: name, Path: "/tmp/shared", CreatedAt: time.Now().UTC()})
			results <- result{bound: bound, inserted: inserted, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	insertions := 0
	var boundName string
	for got := range results {
		if got.err != nil {
			t.Fatal(got.err)
		}
		if got.inserted {
			insertions++
		}
		if boundName == "" {
			boundName = got.bound.Name
		} else if got.bound.Name != boundName {
			t.Fatalf("path bound to different names: %q and %q", boundName, got.bound.Name)
		}
	}
	if insertions != 1 {
		t.Fatalf("insertions = %d, want 1", insertions)
	}
}
