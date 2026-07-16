package cli

import "errors"

// Exit codes returned by the CLI.
const (
	ExitOK        = 0
	ExitGeneral   = 1
	ExitUsage     = 2
	ExitNotFound  = 3
	ExitNotWired  = 4
	ExitCancelled = 130
)

// ErrNotWired is returned by the bootstrap placeholder until a real service or
// runner is integrated.
var ErrNotWired = errors.New("not wired: integration not yet implemented")

// CLIError attaches a stable exit code to an error.
type CLIError struct {
	Code int
	Err  error
}

func (e *CLIError) Error() string { return e.Err.Error() }
func (e *CLIError) Unwrap() error { return e.Err }

// NewCLIError returns an error with a specific exit code.
func NewCLIError(code int, err error) error {
	return &CLIError{Code: code, Err: err}
}
