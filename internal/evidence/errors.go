package evidence

import "errors"

var (
	ErrNotFound               = errors.New("evidence: not found")
	ErrMissingCommand         = errors.New("evidence: command argv is required")
	ErrMissingWorkspace       = errors.New("evidence: workspace path is required")
	ErrInvalidWorkspace       = errors.New("evidence: workspace path is not a directory")
	ErrInvalidEvidenceType    = errors.New("evidence: invalid evidence type")
	ErrInvalidRelation        = errors.New("evidence: invalid relation")
	ErrMissingRunKind         = errors.New("evidence: run kind is required")
	ErrInvalidComparison      = errors.New("evidence: comparison requires one base and one candidate run")
	ErrInvalidOutputLimit     = errors.New("evidence: output limit is invalid")
	ErrInvalidTimeout         = errors.New("evidence: timeout is invalid")
	ErrInvalidEnvironment     = errors.New("evidence: environment allowlist is invalid")
	ErrExecutionNotAuthorized = errors.New("evidence: host execution requires explicit authorization")
)
