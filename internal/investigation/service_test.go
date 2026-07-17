package investigation

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
)

type fakeRepo struct {
	investigations    map[string]*Investigation
	hypotheses        map[string]*Hypothesis
	opportunities     map[string]*Opportunity
	related           []domain.SourceRef
	promotionEvidence []*evidence.Evidence
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		investigations: make(map[string]*Investigation),
		hypotheses:     make(map[string]*Hypothesis),
		opportunities:  make(map[string]*Opportunity),
	}
}

func (r *fakeRepo) SaveInvestigation(_ context.Context, i *Investigation) error {
	r.investigations[i.ID] = i
	return nil
}

func (r *fakeRepo) GetInvestigation(_ context.Context, id string) (*Investigation, error) {
	i, ok := r.investigations[id]
	if !ok {
		return nil, ErrNotFound
	}
	return i, nil
}

func (r *fakeRepo) ListInvestigations(_ context.Context) ([]*Investigation, error) {
	var out []*Investigation
	for _, i := range r.investigations {
		out = append(out, i)
	}
	return out, nil
}

func (r *fakeRepo) SaveHypothesis(_ context.Context, h *Hypothesis) error {
	r.hypotheses[h.ID] = h
	return nil
}

func (r *fakeRepo) GetHypothesis(_ context.Context, id string) (*Hypothesis, error) {
	h, ok := r.hypotheses[id]
	if !ok {
		return nil, ErrNotFound
	}
	return h, nil
}

func (r *fakeRepo) ListHypotheses(_ context.Context, investigationID string) ([]*Hypothesis, error) {
	var out []*Hypothesis
	for _, h := range r.hypotheses {
		if h.InvestigationID == investigationID {
			out = append(out, h)
		}
	}
	return out, nil
}

func (r *fakeRepo) SaveOpportunity(_ context.Context, o *Opportunity) error {
	r.opportunities[o.ID] = o
	return nil
}

func (r *fakeRepo) PromoteHypothesis(_ context.Context, h *Hypothesis, o *Opportunity) error {
	r.hypotheses[h.ID] = h
	r.opportunities[o.ID] = o
	return nil
}

func (r *fakeRepo) PromoteHypothesisWithEvidence(_ context.Context, h *Hypothesis, o *Opportunity, e *evidence.Evidence) error {
	r.hypotheses[h.ID] = h
	r.opportunities[o.ID] = o
	if e != nil {
		r.promotionEvidence = append(r.promotionEvidence, e)
	}
	return nil
}

type failingPromotionRepo struct{ *fakeRepo }

func (r *failingPromotionRepo) PromoteHypothesis(context.Context, *Hypothesis, *Opportunity) error {
	return errors.New("promotion write failed")
}

func (r *failingPromotionRepo) PromoteHypothesisWithEvidence(context.Context, *Hypothesis, *Opportunity, *evidence.Evidence) error {
	return errors.New("promotion write failed")
}

func (r *fakeRepo) GetOpportunity(_ context.Context, id string) (*Opportunity, error) {
	o, ok := r.opportunities[id]
	if !ok {
		return nil, ErrNotFound
	}
	return o, nil
}

func (r *fakeRepo) ListOpportunities(_ context.Context, investigationID string) ([]*Opportunity, error) {
	var out []*Opportunity
	for _, o := range r.opportunities {
		if investigationID == "" || o.InvestigationID == investigationID {
			out = append(out, o)
		}
	}
	return out, nil
}

func (r *fakeRepo) FindRelated(_ context.Context, _ domain.RepoRef, _ Category) ([]domain.SourceRef, error) {
	return r.related, nil
}

type fakeEvidenceStore struct {
	evidence []*evidence.Evidence
}

func (s *fakeEvidenceStore) CreateEvidence(_ context.Context, e *evidence.Evidence) error {
	s.evidence = append(s.evidence, e)
	return nil
}

func (s *fakeEvidenceStore) ListEvidence(_ context.Context, filter evidence.EvidenceFilter) ([]*evidence.Evidence, error) {
	var out []*evidence.Evidence
	for _, e := range s.evidence {
		if filter.OpportunityID != "" && e.OpportunityID != filter.OpportunityID {
			continue
		}
		if filter.HypothesisID != "" && e.HypothesisID != filter.HypothesisID {
			continue
		}
		if filter.InvestigationID != "" && e.InvestigationID != filter.InvestigationID {
			continue
		}
		if filter.Relation != "" && e.Relation != filter.Relation {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func TestStartInvestigation(t *testing.T) {
	svc := NewService(newFakeRepo(), &fakeEvidenceStore{})
	inv, err := svc.StartInvestigation(context.Background(), domain.RepoRef{Owner: "owner", Repo: "repo"}, "abc", "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if inv.ID == "" {
		t.Fatal("expected investigation ID")
	}
	if inv.Status != InvestigationOpen {
		t.Fatalf("status: got %q", inv.Status)
	}
}

func TestRecordHypothesis(t *testing.T) {
	svc := NewService(newFakeRepo(), &fakeEvidenceStore{})
	inv, _ := svc.StartInvestigation(context.Background(), domain.RepoRef{Owner: "owner", Repo: "repo"}, "abc", "")
	h, err := svc.RecordHypothesis(context.Background(), inv.ID, "race in foo", "data race under load", CategoryBug, []domain.SourceRef{
		{Source: "github", URL: "https://github.com/owner/repo/issues/1", ObservedAt: time.Now().UTC()},
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if h.Status != HypothesisProposed {
		t.Fatalf("status: got %q", h.Status)
	}
	if len(h.SourceRefs) != 1 {
		t.Fatalf("expected 1 source ref, got %d", len(h.SourceRefs))
	}
}

func TestPromoteOpportunity(t *testing.T) {
	svc := NewService(newFakeRepo(), &fakeEvidenceStore{})
	inv, _ := svc.StartInvestigation(context.Background(), domain.RepoRef{Owner: "owner", Repo: "repo"}, "abc", "")
	h, _ := svc.RecordHypothesis(context.Background(), inv.ID, "race", "race desc", CategoryBug, nil)

	o, err := svc.PromoteOpportunity(context.Background(), h.ID, "data race under load", "pkg/foo", "crashes under contention", "small", 0.8)
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if o.Status != OpportunityHypothesis {
		t.Fatalf("opportunity status: got %q, want hypothesis", o.Status)
	}
	if o.CollisionStatus != CollisionUnknown {
		t.Fatalf("collision status: got %q", o.CollisionStatus)
	}
}

func TestPromoteOpportunityFailureDoesNotMutateStoredHypothesis(t *testing.T) {
	repo := &failingPromotionRepo{fakeRepo: newFakeRepo()}
	svc := NewService(repo, &fakeEvidenceStore{})
	inv, _ := svc.StartInvestigation(context.Background(), domain.RepoRef{Owner: "owner", Repo: "repo"}, "abc", "")
	h, _ := svc.RecordHypothesis(context.Background(), inv.ID, "race", "race desc", CategoryBug, nil)
	if _, err := svc.PromoteOpportunity(context.Background(), h.ID, "problem", "scope", "impact", "small", 0.8); err == nil {
		t.Fatal("expected promotion failure")
	}
	stored, err := repo.GetHypothesis(context.Background(), h.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != HypothesisProposed {
		t.Fatalf("failed promotion mutated hypothesis: %+v", stored)
	}
}

func TestPromoteOpportunityRejectsInvalidInputs(t *testing.T) {
	svc := NewService(newFakeRepo(), &fakeEvidenceStore{})
	if _, err := svc.PromoteOpportunity(context.Background(), "missing", " ", "scope", "impact", "small", 0.5); !errors.Is(err, ErrMissingProblem) {
		t.Fatalf("blank problem error = %v", err)
	}
	if _, err := svc.PromoteOpportunity(context.Background(), "missing", "problem", "scope", "impact", "small", math.NaN()); err == nil {
		t.Fatal("expected NaN confidence error")
	}
}

func TestInvalidOpportunityTransition(t *testing.T) {
	svc := NewService(newFakeRepo(), &fakeEvidenceStore{})
	inv, _ := svc.StartInvestigation(context.Background(), domain.RepoRef{Owner: "owner", Repo: "repo"}, "abc", "")
	h, _ := svc.RecordHypothesis(context.Background(), inv.ID, "race", "race desc", CategoryBug, nil)
	o, _ := svc.PromoteOpportunity(context.Background(), h.ID, "problem", "scope", "impact", "small", 0.5)

	// Cannot jump from hypothesis directly to merged.
	_, err := svc.SetOpportunityStatus(context.Background(), o.ID, OpportunityMerged, "skip steps")
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}

	// Cannot move to rejected and then back.
	_, err = svc.SetOpportunityStatus(context.Background(), o.ID, OpportunityRejected, "no longer relevant")
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	_, err = svc.SetOpportunityStatus(context.Background(), o.ID, OpportunityValidated, "try again")
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition after terminal rejection, got %v", err)
	}
}

func TestOpportunityTransitionAuditTrail(t *testing.T) {
	svc := NewService(newFakeRepo(), &fakeEvidenceStore{})
	inv, _ := svc.StartInvestigation(context.Background(), domain.RepoRef{Owner: "owner", Repo: "repo"}, "abc", "")
	h, _ := svc.RecordHypothesis(context.Background(), inv.ID, "race", "race desc", CategoryBug, nil)
	o, _ := svc.PromoteOpportunity(context.Background(), h.ID, "problem", "scope", "impact", "small", 0.5)

	_, err := svc.SetOpportunityStatus(context.Background(), o.ID, OpportunityReproduced, "base branch fails")
	if err != nil {
		t.Fatalf("reproduce: %v", err)
	}
	got, _ := svc.repo.GetOpportunity(context.Background(), o.ID)
	if len(got.AuditTrail) != 1 {
		t.Fatalf("expected 1 status change, got %d", len(got.AuditTrail))
	}
}

func TestUpdateCollisionStatusRecordsPreviousValue(t *testing.T) {
	svc := NewService(newFakeRepo(), &fakeEvidenceStore{})
	inv, _ := svc.StartInvestigation(context.Background(), domain.RepoRef{Owner: "owner", Repo: "repo"}, "abc", "")
	h, _ := svc.RecordHypothesis(context.Background(), inv.ID, "race", "race desc", CategoryBug, nil)
	o, _ := svc.PromoteOpportunity(context.Background(), h.ID, "problem", "scope", "impact", "small", 0.5)
	updated, err := svc.UpdateCollisionStatus(context.Background(), o.ID, CollisionPossible, "similar open PR")
	if err != nil {
		t.Fatal(err)
	}
	change := updated.AuditTrail[len(updated.AuditTrail)-1]
	if change.From != string(CollisionUnknown) || change.To != string(CollisionPossible) {
		t.Fatalf("collision audit = %+v", change)
	}
}

func TestContradictingEvidenceBlocksValidation(t *testing.T) {
	repo := newFakeRepo()
	store := &fakeEvidenceStore{}
	svc := NewService(repo, store)

	inv, _ := svc.StartInvestigation(context.Background(), domain.RepoRef{Owner: "owner", Repo: "repo"}, "abc", "")
	h, _ := svc.RecordHypothesis(context.Background(), inv.ID, "race", "race desc", CategoryBug, nil)
	o, _ := svc.PromoteOpportunity(context.Background(), h.ID, "problem", "scope", "impact", "small", 0.5)

	_, err := svc.SetOpportunityStatus(context.Background(), o.ID, OpportunityReproduced, "base branch fails")
	if err != nil {
		t.Fatalf("reproduce: %v", err)
	}

	// Supporting evidence lets the opportunity advance.
	recorded, err := svc.RecordEvidence(context.Background(), o.ID, &evidence.Evidence{
		ID:          "ev-support",
		Type:        evidence.EvidenceTypeBaseFailingRegression,
		Relation:    evidence.RelationSupporting,
		Description: "base fails as expected",
	})
	if err != nil {
		t.Fatalf("record supporting: %v", err)
	}
	if recorded.InvestigationID != inv.ID || recorded.HypothesisID != h.ID || recorded.OpportunityID != o.ID {
		t.Fatalf("evidence scope = %+v", recorded)
	}
	if _, err := svc.RecordEvidence(context.Background(), o.ID, &evidence.Evidence{
		ID: "wrong-scope", InvestigationID: "different-investigation",
	}); err == nil {
		t.Fatal("accepted evidence with a conflicting investigation")
	}

	_, err = svc.SetOpportunityStatus(context.Background(), o.ID, OpportunityValidated, "candidate fixes it")
	if err != nil {
		t.Fatalf("validated: %v", err)
	}

	// But adding contradicting evidence and trying to advance further is blocked.
	_, err = svc.RecordEvidence(context.Background(), o.ID, &evidence.Evidence{
		ID:          "ev-contradict",
		Type:        evidence.EvidenceTypeCandidatePassingRegression,
		Relation:    evidence.RelationContradicting,
		Description: "candidate also fails in another scenario",
	})
	if err != nil {
		t.Fatalf("record contradicting: %v", err)
	}

	_, err = svc.SetOpportunityStatus(context.Background(), o.ID, OpportunityMaintainerAligned, "looks good")
	if !errors.Is(err, ErrContradictingEvidence) {
		t.Fatalf("expected ErrContradictingEvidence, got %v", err)
	}
}

func TestSummarizeEvidence(t *testing.T) {
	repo := newFakeRepo()
	store := &fakeEvidenceStore{}
	svc := NewService(repo, store)

	inv, _ := svc.StartInvestigation(context.Background(), domain.RepoRef{Owner: "owner", Repo: "repo"}, "abc", "")
	h, _ := svc.RecordHypothesis(context.Background(), inv.ID, "race", "race desc", CategoryBug, nil)
	o, _ := svc.PromoteOpportunity(context.Background(), h.ID, "problem", "scope", "impact", "small", 0.5)

	_, _ = svc.RecordEvidence(context.Background(), o.ID, &evidence.Evidence{
		ID:          "s1",
		Type:        evidence.EvidenceTypeManualObservation,
		Relation:    evidence.RelationSupporting,
		Description: "supports",
	})
	_, _ = svc.RecordEvidence(context.Background(), o.ID, &evidence.Evidence{
		ID:          "c1",
		Type:        evidence.EvidenceTypeManualObservation,
		Relation:    evidence.RelationContradicting,
		Description: "contradicts",
	})

	supporting, contradicting, err := svc.SummarizeEvidence(context.Background(), o.ID)
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}
	if len(supporting) != 1 || len(contradicting) != 1 {
		t.Fatalf("expected 1 supporting and 1 contradicting, got %d/%d", len(supporting), len(contradicting))
	}
}

func TestCreateHypothesisWithStructuredFields(t *testing.T) {
	svc := NewService(newFakeRepo(), &fakeEvidenceStore{})
	inv, _ := svc.StartInvestigation(context.Background(), domain.RepoRef{Owner: "owner", Repo: "repo"}, "abc", "")
	h, err := svc.CreateHypothesis(context.Background(), inv.ID, CreateHypothesisInput{
		Title:              "race in parser",
		Description:        "data race under load",
		Category:           CategoryBug,
		ExpectedBehavior:   "parser should not panic",
		ObservedBehavior:   "parser panics",
		PotentialImpact:    "crash",
		OpenQuestions:      []string{"reproducible?"},
		AffectedComponents: []string{"pkg/parser"},
		SourceRefs: []domain.SourceRef{
			{Source: "github", URL: "https://github.com/owner/repo/issues/1", ObservedAt: time.Now().UTC()},
		},
		Links: []Link{{Kind: "issue", Ref: "owner/repo#1"}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if h.Status != HypothesisProposed {
		t.Fatalf("status: got %q", h.Status)
	}
	if h.ExpectedBehavior == "" || len(h.OpenQuestions) != 1 || len(h.Links) != 1 {
		t.Fatalf("structured fields missing: %+v", h)
	}
	if len(h.SourceRefs) != 1 {
		t.Fatalf("expected 1 source ref, got %d", len(h.SourceRefs))
	}
}

func TestUpdateHypothesisRecordsRationale(t *testing.T) {
	svc := NewService(newFakeRepo(), &fakeEvidenceStore{})
	inv, _ := svc.StartInvestigation(context.Background(), domain.RepoRef{Owner: "owner", Repo: "repo"}, "abc", "")
	h, _ := svc.CreateHypothesis(context.Background(), inv.ID, CreateHypothesisInput{
		Title:       "race",
		Description: "desc",
		Category:    CategoryBug,
	})
	updated, err := svc.UpdateHypothesis(context.Background(), h.ID, UpdateHypothesisInput{
		Title:       "race in parser",
		Description: "data race under load",
		Category:    CategoryBug,
		Rationale:   "refined after reproducing",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Title != "race in parser" {
		t.Fatalf("title not updated: %q", updated.Title)
	}
	if len(updated.AuditTrail) != 1 || updated.AuditTrail[0].Rationale != "refined after reproducing" {
		t.Fatalf("expected rationale audit, got %+v", updated.AuditTrail)
	}
}

func TestTransitionHypothesisWithRationale(t *testing.T) {
	svc := NewService(newFakeRepo(), &fakeEvidenceStore{})
	inv, _ := svc.StartInvestigation(context.Background(), domain.RepoRef{Owner: "owner", Repo: "repo"}, "abc", "")
	h, _ := svc.CreateHypothesis(context.Background(), inv.ID, CreateHypothesisInput{Title: "race", Description: "desc", Category: CategoryBug})
	updated, err := svc.TransitionHypothesis(context.Background(), h.ID, HypothesisRejected, "not reproducible")
	if err != nil {
		t.Fatalf("transition: %v", err)
	}
	if updated.Status != HypothesisRejected {
		t.Fatalf("status: got %q", updated.Status)
	}
	if len(updated.AuditTrail) != 1 || updated.AuditTrail[0].From != string(HypothesisProposed) || updated.AuditTrail[0].To != string(HypothesisRejected) {
		t.Fatalf("expected transition audit, got %+v", updated.AuditTrail)
	}
}

func TestPromoteOpportunityWithInput(t *testing.T) {
	repo := newFakeRepo()
	store := &fakeEvidenceStore{}
	svc := NewService(repo, store)
	inv, _ := svc.StartInvestigation(context.Background(), domain.RepoRef{Owner: "owner", Repo: "repo"}, "abc", "")
	h, _ := svc.CreateHypothesis(context.Background(), inv.ID, CreateHypothesisInput{Title: "race", Description: "desc", Category: CategoryBug})
	o, err := svc.PromoteOpportunityWithInput(context.Background(), h.ID, PromoteOpportunityInput{
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
	if o.Status != OpportunityHypothesis {
		t.Fatalf("status: got %q", o.Status)
	}
	if len(o.Dependencies) != 1 || o.MaintainerAlignment == "" {
		t.Fatalf("missing promotion fields: %+v", o)
	}
	if len(o.EvidenceIDs) != 1 {
		t.Fatalf("expected maintainer-alignment evidence id, got %+v", o.EvidenceIDs)
	}
	if len(repo.promotionEvidence) != 1 || repo.promotionEvidence[0].Relation != evidence.RelationSupporting {
		t.Fatalf("expected supporting evidence, got %+v", repo.promotionEvidence)
	}
}
