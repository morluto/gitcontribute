// Package failure defines application failure kinds without transport policy.
package failure

import (
	"errors"
	"fmt"
)

// Kind classifies a product failure for adapter-specific projection.
type Kind string

const (
	// KindNotFound identifies an absent requested product object.
	KindNotFound Kind = "not_found"
)

// Error attaches a product failure kind while preserving its cause.
type Error struct {
	Kind Kind
	Err  error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }
func (e *Error) Is(target error) bool {
	other, ok := target.(*Error)
	return ok && e.Kind == other.Kind
}

// NotFound classifies err as an absent requested object.
func NotFound(err error) error {
	if err == nil {
		err = errors.New("requested object not found")
	}
	return &Error{Kind: KindNotFound, Err: err}
}

// Is reports whether err carries kind.
func Is(err error, kind Kind) bool {
	var typed *Error
	return errors.As(err, &typed) && typed.Kind == kind
}

// Format provides a stable diagnostic when an adapter needs the kind.
func Format(err error) string {
	var typed *Error
	if errors.As(err, &typed) {
		return fmt.Sprintf("%s: %s", typed.Kind, typed.Err)
	}
	return err.Error()
}
