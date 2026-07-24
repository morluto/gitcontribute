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

// ErrNotWired reports that an optional capability is unavailable from the
// connected service or runner.
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
