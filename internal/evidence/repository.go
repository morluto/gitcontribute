package evidence

import "context"

// Repository is a narrow persistence boundary for validation definitions, runs,
// and evidence. Concrete implementations live outside this package; production
// code never uses an in-memory store.
type Repository interface {
	SaveValidationDefinition(ctx context.Context, d *ValidationDefinition) error
	GetValidationDefinition(ctx context.Context, id string) (*ValidationDefinition, error)
	SaveValidationRun(ctx context.Context, r *ValidationRun) error
	GetValidationRun(ctx context.Context, id string) (*ValidationRun, error)
	SaveValidationRunGroup(ctx context.Context, group *ValidationRunGroup) error
	GetValidationRunGroup(ctx context.Context, id string) (*ValidationRunGroup, error)
	SaveEvidence(ctx context.Context, e *Evidence) error
	ListEvidence(ctx context.Context, filter EvidenceFilter) ([]*Evidence, error)
}

// EvidenceFilter selects evidence by related identifiers or relation.
type EvidenceFilter struct {
	InvestigationID string
	HypothesisID    string
	OpportunityID   string
	Relation        Relation
}
