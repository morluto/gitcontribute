package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/investigation"
)

// StartInvestigation creates a new investigation scoped to a repository.
func (s *Service) StartInvestigation(ctx context.Context, repo cli.RepoRef, commitSHA, lens string) (*cli.InvestigationResult, error) {
	ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Repo}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	invSvc, err := s.writeInvestigationSvc(ctx)
	if err != nil {
		return nil, err
	}
	inv, err := invSvc.StartInvestigation(ctx, ref, commitSHA, lens)
	if err != nil {
		return nil, err
	}
	return investigationResult(inv), nil
}

// ShowInvestigation returns an investigation by ID.
func (s *Service) ShowInvestigation(ctx context.Context, id string) (*cli.InvestigationResult, error) {
	invSvc, err := s.readInvestigationSvc(ctx)
	if err != nil {
		return nil, err
	}
	inv, err := invSvc.GetInvestigation(ctx, id)
	if err != nil {
		return nil, mapInvestigationError(err)
	}
	return investigationResult(inv), nil
}

// ListInvestigations returns all investigations.
func (s *Service) ListInvestigations(ctx context.Context) (*cli.InvestigationListResult, error) {
	invSvc, err := s.readInvestigationSvc(ctx)
	if err != nil {
		return nil, err
	}
	items, err := invSvc.ListInvestigations(ctx)
	if err != nil {
		return nil, err
	}
	result := &cli.InvestigationListResult{Investigations: make([]cli.InvestigationResult, len(items))}
	for i, inv := range items {
		result.Investigations[i] = *investigationResult(inv)
	}
	return result, nil
}

// AddHypothesis records a hypothesis under an investigation.
func (s *Service) AddHypothesis(ctx context.Context, investigationID, title, description, category string) (*cli.HypothesisResult, error) {
	invSvc, err := s.writeInvestigationSvc(ctx)
	if err != nil {
		return nil, err
	}
	h, err := invSvc.RecordHypothesis(ctx, investigationID, title, description, investigation.Category(category), nil)
	if err != nil {
		return nil, err
	}
	return hypothesisResult(h), nil
}

// ListHypotheses returns hypotheses for an investigation.
func (s *Service) ListHypotheses(ctx context.Context, investigationID string) (*cli.HypothesisListResult, error) {
	invSvc, err := s.readInvestigationSvc(ctx)
	if err != nil {
		return nil, err
	}
	items, err := invSvc.ListHypotheses(ctx, investigationID)
	if err != nil {
		return nil, err
	}
	result := &cli.HypothesisListResult{Hypotheses: make([]cli.HypothesisResult, len(items))}
	for i, h := range items {
		result.Hypotheses[i] = *hypothesisResult(h)
	}
	return result, nil
}

// PromoteOpportunity converts a proposed hypothesis into an opportunity.
func (s *Service) PromoteOpportunity(ctx context.Context, hypothesisID, problem, scope, impact, effort string, confidence float64) (*cli.OpportunityResult, error) {
	invSvc, err := s.writeInvestigationSvc(ctx)
	if err != nil {
		return nil, err
	}
	o, err := invSvc.PromoteOpportunity(ctx, hypothesisID, problem, scope, impact, effort, confidence)
	if err != nil {
		return nil, err
	}
	return opportunityResult(o), nil
}

// ShowOpportunity returns an opportunity by ID.
func (s *Service) ShowOpportunity(ctx context.Context, id string) (*cli.OpportunityResult, error) {
	invSvc, err := s.readInvestigationSvc(ctx)
	if err != nil {
		return nil, err
	}
	o, err := invSvc.GetOpportunity(ctx, id)
	if err != nil {
		return nil, mapInvestigationError(err)
	}
	return opportunityResult(o), nil
}

// ListOpportunities returns opportunities, optionally filtered to one investigation.
func (s *Service) ListOpportunities(ctx context.Context, investigationID string) (*cli.OpportunityListResult, error) {
	invSvc, err := s.readInvestigationSvc(ctx)
	if err != nil {
		return nil, err
	}
	items, err := invSvc.ListOpportunities(ctx, investigationID)
	if err != nil {
		return nil, err
	}
	result := &cli.OpportunityListResult{
		Opportunities: make([]cli.OpportunityResult, len(items)),
		Filter:        investigationID,
	}
	for i, o := range items {
		result.Opportunities[i] = *opportunityResult(o)
	}
	return result, nil
}

// SetOpportunityStatus transitions an opportunity with a recorded rationale.
func (s *Service) SetOpportunityStatus(ctx context.Context, id, status, rationale string) (*cli.OpportunityResult, error) {
	opStatus, err := parseOpportunityStatus(status)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(rationale) == "" {
		return nil, errors.New("rationale is required")
	}
	invSvc, err := s.writeInvestigationSvc(ctx)
	if err != nil {
		return nil, err
	}
	o, err := invSvc.SetOpportunityStatus(ctx, id, opStatus, rationale)
	if err != nil {
		return nil, err
	}
	return opportunityResult(o), nil
}

// CreateHypothesis records a fully structured hypothesis under an investigation.
func (s *Service) CreateHypothesis(ctx context.Context, investigationID string, input investigation.CreateHypothesisInput) (*investigation.Hypothesis, error) {
	invSvc, err := s.writeInvestigationSvc(ctx)
	if err != nil {
		return nil, err
	}
	return invSvc.CreateHypothesis(ctx, investigationID, input)
}

// UpdateHypothesis deliberately overwrites a hypothesis with rationale.
func (s *Service) UpdateHypothesis(ctx context.Context, hypothesisID string, input investigation.UpdateHypothesisInput) (*investigation.Hypothesis, error) {
	invSvc, err := s.writeInvestigationSvc(ctx)
	if err != nil {
		return nil, err
	}
	return invSvc.UpdateHypothesis(ctx, hypothesisID, input)
}

// TransitionHypothesis advances a hypothesis through its lifecycle with rationale.
func (s *Service) TransitionHypothesis(ctx context.Context, hypothesisID, status, rationale string) (*investigation.Hypothesis, error) {
	hStatus, err := parseHypothesisStatus(status)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(rationale) == "" {
		return nil, errors.New("rationale is required")
	}
	invSvc, err := s.writeInvestigationSvc(ctx)
	if err != nil {
		return nil, err
	}
	return invSvc.TransitionHypothesis(ctx, hypothesisID, hStatus, rationale)
}

// PromoteOpportunityWithInput promotes a hypothesis with dependencies and
// maintainer-alignment evidence.
func (s *Service) PromoteOpportunityWithInput(ctx context.Context, hypothesisID string, input investigation.PromoteOpportunityInput) (*investigation.Opportunity, error) {
	invSvc, err := s.writeInvestigationSvc(ctx)
	if err != nil {
		return nil, err
	}
	return invSvc.PromoteOpportunityWithInput(ctx, hypothesisID, input)
}

// UpdateOpportunityCollisionStatus explicitly sets the collision status with rationale.
func (s *Service) UpdateOpportunityCollisionStatus(ctx context.Context, opportunityID, status, rationale string) (*investigation.Opportunity, error) {
	cStatus, err := parseCollisionStatus(status)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(rationale) == "" {
		return nil, errors.New("rationale is required")
	}
	invSvc, err := s.writeInvestigationSvc(ctx)
	if err != nil {
		return nil, err
	}
	return invSvc.UpdateCollisionStatus(ctx, opportunityID, cStatus, rationale)
}

func (s *Service) writeInvestigationSvc(ctx context.Context) (*investigation.Service, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	return investigation.NewService(c, c), nil
}

func (s *Service) readInvestigationSvc(ctx context.Context) (*investigation.Service, error) {
	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return nil, err
	}
	return investigation.NewService(c, c), nil
}

func investigationResult(inv *investigation.Investigation) *cli.InvestigationResult {
	return &cli.InvestigationResult{
		ID: inv.ID, Repo: cli.RepoRef{Owner: inv.Repo.Owner, Repo: inv.Repo.Repo},
		CommitSHA: inv.CommitSHA, Lens: inv.Lens, Status: string(inv.Status),
		ThreadBaseline: threadBaselineResult(inv.ThreadBaseline), SeedHypothesisID: inv.SeedHypothesisID,
		AuditTrail: workflowAuditResults(inv.AuditTrail),
		CreatedAt:  formatTime(inv.CreatedAt), UpdatedAt: formatTime(inv.UpdatedAt),
	}
}

func hypothesisResult(h *investigation.Hypothesis) *cli.HypothesisResult {
	return &cli.HypothesisResult{
		ID:              h.ID,
		InvestigationID: h.InvestigationID,
		Title:           h.Title,
		Description:     h.Description,
		Category:        string(h.Category),
		Status:          string(h.Status),
		SourceRefs:      workflowSourceRefResults(h.SourceRefs),
		Links:           workflowLinkResults(h.Links),
		AuditTrail:      workflowAuditResults(h.AuditTrail),
		CreatedAt:       formatTime(h.CreatedAt),
		UpdatedAt:       formatTime(h.UpdatedAt),
	}
}

func opportunityResult(o *investigation.Opportunity) *cli.OpportunityResult {
	return &cli.OpportunityResult{
		ID:               o.ID,
		InvestigationID:  o.InvestigationID,
		HypothesisID:     o.HypothesisID,
		Title:            o.Title,
		ProblemStatement: o.ProblemStatement,
		Category:         string(o.Category),
		Scope:            o.Scope,
		Impact:           o.Impact,
		ExpectedEffort:   o.ExpectedEffort,
		Confidence:       o.Confidence,
		CollisionStatus:  string(o.CollisionStatus),
		Status:           string(o.Status),
		CreatedAt:        formatTime(o.CreatedAt),
		UpdatedAt:        formatTime(o.UpdatedAt),
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func mapInvestigationError(err error) error {
	if errors.Is(err, investigation.ErrNotFound) {
		return cli.NewCLIError(cli.ExitNotFound, err)
	}
	return err
}

func parseOpportunityStatus(status string) (investigation.OpportunityStatus, error) {
	switch investigation.OpportunityStatus(status) {
	case investigation.OpportunityHypothesis,
		investigation.OpportunityReproduced,
		investigation.OpportunityValidated,
		investigation.OpportunityMaintainerAligned,
		investigation.OpportunityImplemented,
		investigation.OpportunitySubmitted,
		investigation.OpportunityMerged,
		investigation.OpportunityRejected,
		investigation.OpportunityDeferred,
		investigation.OpportunitySuperseded:
		return investigation.OpportunityStatus(status), nil
	}
	return "", fmt.Errorf("invalid opportunity status %q", status)
}

func parseHypothesisStatus(status string) (investigation.HypothesisStatus, error) {
	switch investigation.HypothesisStatus(status) {
	case investigation.HypothesisProposed,
		investigation.HypothesisPromoted,
		investigation.HypothesisRejected,
		investigation.HypothesisDeferred,
		investigation.HypothesisSuperseded:
		return investigation.HypothesisStatus(status), nil
	}
	return "", fmt.Errorf("invalid hypothesis status %q", status)
}

func parseCollisionStatus(status string) (investigation.CollisionStatus, error) {
	switch investigation.CollisionStatus(status) {
	case investigation.CollisionUnknown,
		investigation.CollisionNone,
		investigation.CollisionPossible,
		investigation.CollisionConfirmed,
		investigation.CollisionBlocked:
		return investigation.CollisionStatus(status), nil
	}
	return "", fmt.Errorf("invalid collision status %q", status)
}
