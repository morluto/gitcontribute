package investigation

import "errors"

var (
	ErrNotFound              = errors.New("investigation: not found")
	ErrInvalidRepo           = errors.New("investigation: invalid repository reference")
	ErrInvalidTransition     = errors.New("investigation: invalid status transition")
	ErrMissingTitle          = errors.New("investigation: title is required")
	ErrMissingProblem        = errors.New("investigation: problem statement is required")
	ErrInvalidCategory       = errors.New("investigation: invalid category")
	ErrInvalidThreadBaseline = errors.New("investigation: invalid thread baseline")
	ErrContradictingEvidence = errors.New("investigation: contradicting evidence blocks this transition")
)
