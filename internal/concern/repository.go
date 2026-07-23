package concern

import "context"

// Repository is the narrow persistence boundary for local concerns.
type Repository interface {
	SaveConcern(context.Context, *Concern) error
	GetConcern(context.Context, string) (*Concern, error)
	ListConcerns(context.Context, Filter) ([]*Concern, error)
	AddConcernLink(context.Context, string, Link) error
}
