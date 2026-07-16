package contribution

import "errors"

var (
	ErrNotFound           = errors.New("contribution: not found")
	ErrMissingOpportunity = errors.New("contribution: opportunity is required")
	ErrEmptyGuidance      = errors.New("contribution: guidance is required")
	ErrMissingApproach    = errors.New("contribution: approach is required")
)
