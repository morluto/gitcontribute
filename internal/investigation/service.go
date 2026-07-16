package investigation

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
)

// Service manages investigations, hypotheses, opportunities, and their evidence.
type Service struct {
	repo     Repository
	evidence EvidenceStore
}

// NewService returns an InvestigationService backed by repo and evidence store.
func NewService(repo Repository, evidence EvidenceStore) *Service {
	return &Service{repo: repo, evidence: evidence}
}

// StartInvestigation creates a new investigation for a repository and commit.
func (s *Service) StartInvestigation(ctx context.Context, repo domain.RepoRef, commitSHA, lens string) (*Investigation, error) {
	if err := repo.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRepo, err)
	}
	now := time.Now().UTC()
	inv := &Investigation{
		ID:        uuid.NewString(),
		Repo:      repo,
		CommitSHA: commitSHA,
		Lens:      lens,
		Status:    InvestigationOpen,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.repo.SaveInvestigation(ctx, inv); err != nil {
		return nil, err
	}
	return inv, nil
}

// RecordHypothesis stores a new hypothesis under an investigation.
func (s *Service) RecordHypothesis(ctx context.Context, investigationID, title, description string, category Category, refs []domain.SourceRef) (*Hypothesis, error) {
	if title == "" {
		return nil, ErrMissingTitle
	}
	if !isValidCategory(category) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidCategory, category)
	}
	if _, err := s.repo.GetInvestigation(ctx, investigationID); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	h := &Hypothesis{
		ID:              uuid.NewString(),
		InvestigationID: investigationID,
		Title:           title,
		Description:     description,
		Category:        category,
		SourceRefs:      append([]domain.SourceRef(nil), refs...),
		Status:          HypothesisProposed,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.repo.SaveHypothesis(ctx, h); err != nil {
		return nil, err
	}
	return h, nil
}

// TransitionHypothesis advances a hypothesis through its lifecycle.
func (s *Service) TransitionHypothesis(ctx context.Context, id string, to HypothesisStatus, rationale string) (*Hypothesis, error) {
	h, err := s.repo.GetHypothesis(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := h.Transition(to, rationale); err != nil {
		return nil, err
	}
	if err := s.repo.SaveHypothesis(ctx, h); err != nil {
		return nil, err
	}
	return h, nil
}

// PromoteOpportunity converts a confirmed hypothesis into an opportunity.
func (s *Service) PromoteOpportunity(ctx context.Context, hypothesisID, problem, scope, impact, effort string, confidence float64) (*Opportunity, error) {
	h, err := s.repo.GetHypothesis(ctx, hypothesisID)
	if err != nil {
		return nil, err
	}
	if h.Status != HypothesisProposed {
		return nil, fmt.Errorf("%w: hypothesis must be proposed to promote, got %s", ErrInvalidTransition, h.Status)
	}
	if err := h.Transition(HypothesisPromoted, "promoted to opportunity"); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	o := &Opportunity{
		ID:               uuid.NewString(),
		InvestigationID:  h.InvestigationID,
		HypothesisID:     h.ID,
		Title:            h.Title,
		ProblemStatement: problem,
		Category:         h.Category,
		Scope:            scope,
		Impact:           impact,
		Confidence:       confidence,
		ExpectedEffort:   effort,
		CollisionStatus:  CollisionUnknown,
		SourceRefs:       append([]domain.SourceRef(nil), h.SourceRefs...),
		Status:           OpportunityHypothesis,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	_ = o.Transition(OpportunityHypothesis, "promoted from hypothesis")
	if err := s.repo.SaveHypothesis(ctx, h); err != nil {
		return nil, err
	}
	if err := s.repo.SaveOpportunity(ctx, o); err != nil {
		return nil, err
	}
	return o, nil
}

// SetOpportunityStatus transitions an opportunity, blocking advancement when
// contradicting evidence is present.
func (s *Service) SetOpportunityStatus(ctx context.Context, id string, to OpportunityStatus, rationale string) (*Opportunity, error) {
	o, err := s.repo.GetOpportunity(ctx, id)
	if err != nil {
		return nil, err
	}

	advancing := isAdvancingStatus(to) && o.Status != to
	if advancing {
		all, err := s.evidence.ListEvidence(ctx, evidence.EvidenceFilter{OpportunityID: id})
		if err != nil {
			return nil, err
		}
		for _, e := range all {
			if e.Relation == evidence.RelationContradicting {
				return nil, fmt.Errorf("%w: %s", ErrContradictingEvidence, e.Description)
			}
		}
	}

	if err := o.Transition(to, rationale); err != nil {
		return nil, err
	}
	if err := s.repo.SaveOpportunity(ctx, o); err != nil {
		return nil, err
	}
	return o, nil
}

// RecordEvidence attaches an evidence item to an opportunity and stores it.
func (s *Service) RecordEvidence(ctx context.Context, opportunityID string, e *evidence.Evidence) (*evidence.Evidence, error) {
	if _, err := s.repo.GetOpportunity(ctx, opportunityID); err != nil {
		return nil, err
	}
	e.OpportunityID = opportunityID
	if err := s.evidence.CreateEvidence(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

// SummarizeEvidence separates supporting and contradicting evidence for an opportunity.
func (s *Service) SummarizeEvidence(ctx context.Context, opportunityID string) ([]*evidence.Evidence, []*evidence.Evidence, error) {
	if _, err := s.repo.GetOpportunity(ctx, opportunityID); err != nil {
		return nil, nil, err
	}
	all, err := s.evidence.ListEvidence(ctx, evidence.EvidenceFilter{OpportunityID: opportunityID})
	if err != nil {
		return nil, nil, err
	}
	var supporting, contradicting []*evidence.Evidence
	for _, e := range all {
		switch e.Relation {
		case evidence.RelationSupporting:
			supporting = append(supporting, e)
		case evidence.RelationContradicting:
			contradicting = append(contradicting, e)
		}
	}
	return supporting, contradicting, nil
}

// UpdateCollisionStatus explicitly sets the collision status with rationale.
func (s *Service) UpdateCollisionStatus(ctx context.Context, id string, status CollisionStatus, rationale string) (*Opportunity, error) {
	o, err := s.repo.GetOpportunity(ctx, id)
	if err != nil {
		return nil, err
	}
	o.CollisionStatus = status
	o.AuditTrail = append(o.AuditTrail, StatusChange{
		From:      string(o.CollisionStatus),
		To:        string(status),
		Rationale: rationale,
		At:        time.Now().UTC(),
	})
	o.UpdatedAt = time.Now().UTC()
	if err := s.repo.SaveOpportunity(ctx, o); err != nil {
		return nil, err
	}
	return o, nil
}

// CheckDuplicates returns source references for known related work in the same repository.
func (s *Service) CheckDuplicates(ctx context.Context, investigationID string) ([]domain.SourceRef, error) {
	inv, err := s.repo.GetInvestigation(ctx, investigationID)
	if err != nil {
		return nil, err
	}
	return s.repo.FindRelated(ctx, inv.Repo, "")
}

func isValidCategory(c Category) bool {
	switch c {
	case CategoryBug, CategoryPerformance, CategoryArchitecture, CategoryTesting,
		CategoryDocumentation, CategoryMaintenance, CategoryCompatibility, CategorySecurity, CategoryOther:
		return true
	}
	return false
}

func isAdvancingStatus(status OpportunityStatus) bool {
	switch status {
	case OpportunityValidated, OpportunityMaintainerAligned, OpportunityImplemented,
		OpportunitySubmitted, OpportunityMerged:
		return true
	}
	return false
}
