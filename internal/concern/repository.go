package concern

import "context"

// ListResult separates a bounded concern window from its matching population.
type ListResult struct {
	Concerns []*Concern
	Total    int
	Limit    int
}

// Repository is the narrow persistence boundary for local concerns.
type Repository interface {
	SaveConcern(context.Context, *Concern) error
	UpdateConcern(context.Context, *Concern, *Concern) error
	GetConcern(context.Context, string) (*Concern, error)
	ListConcerns(context.Context, Filter) (*ListResult, error)
	AddConcernLink(context.Context, string, Link) error
}
