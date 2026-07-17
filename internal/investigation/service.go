package investigation

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
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

// CreateHypothesisInput carries all structured fields for a new hypothesis.
type CreateHypothesisInput struct {
	Title              string
	Description        string
	Category           Category
	ExpectedBehavior   string
	ObservedBehavior   string
	PotentialImpact    string
	OpenQuestions      []string
	AffectedComponents []string
	SourceRefs         []domain.SourceRef
	Links              []Link
}

// UpdateHypothesisInput carries all structured fields for a deliberate update.
type UpdateHypothesisInput struct {
	Title              string
	Description        string
	Category           Category
	ExpectedBehavior   string
	ObservedBehavior   string
	PotentialImpact    string
	OpenQuestions      []string
	AffectedComponents []string
	SourceRefs         []domain.SourceRef
	Links              []Link
	Rationale          string
}

// PromoteOpportunityInput carries the full context for promoting a hypothesis.
type PromoteOpportunityInput struct {
	ProblemStatement    string
	Scope               string
	Impact              string
	ExpectedEffort      string
	Confidence          float64
	Dependencies        []string
	MaintainerAlignment string
	EvidenceIDs         []string
	SourceRefs          []domain.SourceRef
}

// RecordHypothesis stores a new hypothesis under an investigation.
func (s *Service) RecordHypothesis(ctx context.Context, investigationID, title, description string, category Category, refs []domain.SourceRef) (*Hypothesis, error) {
	return s.CreateHypothesis(ctx, investigationID, CreateHypothesisInput{
		Title:       title,
		Description: description,
		Category:    category,
		SourceRefs:  refs,
	})
}

// CreateHypothesis stores a fully structured hypothesis under an investigation.
func (s *Service) CreateHypothesis(ctx context.Context, investigationID string, in CreateHypothesisInput) (*Hypothesis, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, ErrMissingTitle
	}
	if !isValidCategory(in.Category) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidCategory, in.Category)
	}
	if _, err := s.repo.GetInvestigation(ctx, investigationID); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	h := &Hypothesis{
		ID:                 uuid.NewString(),
		InvestigationID:    investigationID,
		Title:              title,
		Description:        in.Description,
		Category:           in.Category,
		ExpectedBehavior:   in.ExpectedBehavior,
		ObservedBehavior:   in.ObservedBehavior,
		PotentialImpact:    in.PotentialImpact,
		OpenQuestions:      append([]string(nil), in.OpenQuestions...),
		AffectedComponents: append([]string(nil), in.AffectedComponents...),
		SourceRefs:         append([]domain.SourceRef(nil), in.SourceRefs...),
		Links:              append([]Link(nil), in.Links...),
		Status:             HypothesisProposed,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := s.repo.SaveHypothesis(ctx, h); err != nil {
		return nil, err
	}
	return h, nil
}

// UpdateHypothesis deliberately overwrites the structured fields of a hypothesis.
// A non-empty rationale is recorded in the audit trail.
func (s *Service) UpdateHypothesis(ctx context.Context, id string, in UpdateHypothesisInput) (*Hypothesis, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, ErrMissingTitle
	}
	if !isValidCategory(in.Category) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidCategory, in.Category)
	}
	h, err := s.repo.GetHypothesis(ctx, id)
	if err != nil {
		return nil, err
	}
	h.Title = title
	h.Description = in.Description
	h.Category = in.Category
	h.ExpectedBehavior = in.ExpectedBehavior
	h.ObservedBehavior = in.ObservedBehavior
	h.PotentialImpact = in.PotentialImpact
	h.OpenQuestions = append([]string(nil), in.OpenQuestions...)
	h.AffectedComponents = append([]string(nil), in.AffectedComponents...)
	h.SourceRefs = append([]domain.SourceRef(nil), in.SourceRefs...)
	h.Links = append([]Link(nil), in.Links...)
	if in.Rationale != "" {
		h.AuditTrail = append(h.AuditTrail, StatusChange{
			From:      string(h.Status),
			To:        string(h.Status),
			Rationale: in.Rationale,
			At:        time.Now().UTC(),
		})
	}
	h.UpdatedAt = time.Now().UTC()
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

// GetInvestigation returns an investigation by ID.
func (s *Service) GetInvestigation(ctx context.Context, id string) (*Investigation, error) {
	return s.repo.GetInvestigation(ctx, id)
}

// ListInvestigations returns all investigations ordered by creation time.
func (s *Service) ListInvestigations(ctx context.Context) ([]*Investigation, error) {
	return s.repo.ListInvestigations(ctx)
}

// GetHypothesis returns a hypothesis by ID.
func (s *Service) GetHypothesis(ctx context.Context, id string) (*Hypothesis, error) {
	return s.repo.GetHypothesis(ctx, id)
}

// ListHypotheses returns hypotheses for an investigation.
func (s *Service) ListHypotheses(ctx context.Context, investigationID string) ([]*Hypothesis, error) {
	return s.repo.ListHypotheses(ctx, investigationID)
}

// GetOpportunity returns an opportunity by ID.
func (s *Service) GetOpportunity(ctx context.Context, id string) (*Opportunity, error) {
	return s.repo.GetOpportunity(ctx, id)
}

// ListOpportunities returns opportunities, optionally filtered to one investigation.
func (s *Service) ListOpportunities(ctx context.Context, investigationID string) ([]*Opportunity, error) {
	return s.repo.ListOpportunities(ctx, investigationID)
}

// PromoteOpportunity converts a confirmed hypothesis into an opportunity.
func (s *Service) PromoteOpportunity(ctx context.Context, hypothesisID, problem, scope, impact, effort string, confidence float64) (*Opportunity, error) {
	return s.PromoteOpportunityWithInput(ctx, hypothesisID, PromoteOpportunityInput{
		ProblemStatement: problem,
		Scope:            scope,
		Impact:           impact,
		ExpectedEffort:   effort,
		Confidence:       confidence,
	})
}

// PromoteOpportunityWithInput converts a confirmed hypothesis into an opportunity
// using the full promotion context, including dependencies and maintainer alignment.
func (s *Service) PromoteOpportunityWithInput(ctx context.Context, hypothesisID string, in PromoteOpportunityInput) (*Opportunity, error) {
	problem := strings.TrimSpace(in.ProblemStatement)
	if problem == "" {
		return nil, ErrMissingProblem
	}
	if math.IsNaN(in.Confidence) || math.IsInf(in.Confidence, 0) || in.Confidence < 0 || in.Confidence > 1 {
		return nil, fmt.Errorf("confidence must be between 0 and 1")
	}
	storedHypothesis, err := s.repo.GetHypothesis(ctx, hypothesisID)
	if err != nil {
		return nil, err
	}
	if storedHypothesis.Status != HypothesisProposed {
		return nil, fmt.Errorf("%w: hypothesis must be proposed to promote, got %s", ErrInvalidTransition, storedHypothesis.Status)
	}
	h := *storedHypothesis
	h.SourceRefs = append([]domain.SourceRef(nil), storedHypothesis.SourceRefs...)
	h.AuditTrail = append([]StatusChange(nil), storedHypothesis.AuditTrail...)
	if err := h.Transition(HypothesisPromoted, "promoted to opportunity"); err != nil {
		return nil, err
	}

	sourceRefs := h.SourceRefs
	if in.SourceRefs != nil {
		sourceRefs = append([]domain.SourceRef(nil), in.SourceRefs...)
	}

	now := time.Now().UTC()
	o := &Opportunity{
		ID:                  uuid.NewString(),
		InvestigationID:     h.InvestigationID,
		HypothesisID:        h.ID,
		Title:               h.Title,
		ProblemStatement:    problem,
		Category:            h.Category,
		Scope:               in.Scope,
		Impact:              in.Impact,
		Confidence:          in.Confidence,
		ExpectedEffort:      in.ExpectedEffort,
		Dependencies:        append([]string(nil), in.Dependencies...),
		MaintainerAlignment: in.MaintainerAlignment,
		EvidenceIDs:         append([]string(nil), in.EvidenceIDs...),
		CollisionStatus:     CollisionUnknown,
		SourceRefs:          sourceRefs,
		Status:              OpportunityHypothesis,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	var alignmentEvidence *evidence.Evidence
	if in.MaintainerAlignment != "" {
		alignmentEvidence = &evidence.Evidence{
			ID:              uuid.NewString(),
			InvestigationID: h.InvestigationID,
			HypothesisID:    h.ID,
			OpportunityID:   o.ID,
			Type:            evidence.EvidenceTypeManualObservation,
			Relation:        evidence.RelationSupporting,
			Description:     "maintainer alignment: " + in.MaintainerAlignment,
			SourceRefs:      append([]domain.SourceRef(nil), in.SourceRefs...),
			CreatedAt:       now,
		}
		o.EvidenceIDs = append(o.EvidenceIDs, alignmentEvidence.ID)
	}

	if err := s.repo.PromoteHypothesisWithEvidence(ctx, &h, o, alignmentEvidence); err != nil {
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
	if e == nil {
		return nil, errors.New("evidence is required")
	}
	opportunity, err := s.repo.GetOpportunity(ctx, opportunityID)
	if err != nil {
		return nil, err
	}
	if e.InvestigationID != "" && e.InvestigationID != opportunity.InvestigationID {
		return nil, errors.New("evidence investigation does not match opportunity")
	}
	if e.HypothesisID != "" && e.HypothesisID != opportunity.HypothesisID {
		return nil, errors.New("evidence hypothesis does not match opportunity")
	}
	e.OpportunityID = opportunityID
	e.InvestigationID = opportunity.InvestigationID
	e.HypothesisID = opportunity.HypothesisID
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
	if !isValidCollisionStatus(status) {
		return nil, fmt.Errorf("invalid collision status %q", status)
	}
	o, err := s.repo.GetOpportunity(ctx, id)
	if err != nil {
		return nil, err
	}
	previous := o.CollisionStatus
	if previous == status {
		return o, nil
	}
	now := time.Now().UTC()
	o.CollisionStatus = status
	o.AuditTrail = append(o.AuditTrail, StatusChange{
		From:      string(previous),
		To:        string(status),
		Rationale: rationale,
		At:        now,
	})
	o.UpdatedAt = now
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

func isValidCollisionStatus(status CollisionStatus) bool {
	switch status {
	case CollisionUnknown, CollisionNone, CollisionPossible, CollisionConfirmed, CollisionBlocked:
		return true
	}
	return false
}
