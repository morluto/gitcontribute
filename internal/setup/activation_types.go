package setup

import "fmt"

// ActivationRollbackError reports an activation failure whose rollback could
// not safely restore every registration.
type ActivationRollbackError struct {
	Cause    error
	Rollback error
}

func (e *ActivationRollbackError) Error() string {
	return fmt.Sprintf("%v; activation rollback incomplete: %v", e.Cause, e.Rollback)
}

// Unwrap exposes both the activation and rollback failures.
func (e *ActivationRollbackError) Unwrap() []error {
	return []error{e.Cause, e.Rollback}
}
