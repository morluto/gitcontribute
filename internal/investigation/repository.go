package investigation

import (
	"context"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
)

// Repository is a narrow persistence boundary for investigations, hypotheses,
// and opportunities. Concrete stores live outside this package.
type Repository interface {
	SaveInvestigation(ctx context.Context, i *Investigation) error
	GetInvestigation(ctx context.Context, id string) (*Investigation, error)
	ListInvestigations(ctx context.Context) ([]*Investigation, error)
	SaveHypothesis(ctx context.Context, h *Hypothesis) error
	GetHypothesis(ctx context.Context, id string) (*Hypothesis, error)
	ListHypotheses(ctx context.Context, investigationID string) ([]*Hypothesis, error)
	SaveOpportunity(ctx context.Context, o *Opportunity) error
	GetOpportunity(ctx context.Context, id string) (*Opportunity, error)
	ListOpportunities(ctx context.Context, investigationID string) ([]*Opportunity, error)
	FindRelated(ctx context.Context, ref domain.RepoRef, category Category) ([]domain.SourceRef, error)
}

// EvidenceStore is the subset of evidence operations the investigation service needs.
type EvidenceStore interface {
	CreateEvidence(ctx context.Context, e *evidence.Evidence) error
	ListEvidence(ctx context.Context, filter evidence.EvidenceFilter) ([]*evidence.Evidence, error)
}
