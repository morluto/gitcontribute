package concern

import "errors"

var (
	// ErrNotFound indicates that a concern ID is absent.
	ErrNotFound = errors.New("concern not found")
	// ErrInvalidStatus indicates an unsupported lifecycle status.
	ErrInvalidStatus = errors.New("invalid concern status")
	// ErrInvalidTransition indicates a forbidden lifecycle transition.
	ErrInvalidTransition = errors.New("invalid concern status transition")
	// ErrInvalidLink indicates a malformed or unsupported relationship.
	ErrInvalidLink = errors.New("invalid concern link")
)
