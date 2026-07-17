package github

import (
	"errors"
	"fmt"
	"time"
)

// NotFoundError indicates a requested GitHub resource was not found.
type NotFoundError struct {
	Resource string
}

// AccessDeniedError indicates that the current GitHub credentials cannot read
// a resource. It covers authenticated and unauthenticated denial responses.
type AccessDeniedError struct {
	StatusCode int
	Message    string
}

func (e *AccessDeniedError) Error() string {
	if e.Message == "" {
		return "github access denied"
	}
	return fmt.Sprintf("github access denied: %s", e.Message)
}

// GoneError indicates that GitHub reports a resource as permanently removed.
type GoneError struct {
	Resource string
}

func (e *GoneError) Error() string {
	if e.Resource == "" {
		return "github resource deleted"
	}
	return fmt.Sprintf("github resource deleted: %s", e.Resource)
}

func (e *NotFoundError) Error() string {
	if e.Resource == "" {
		return "github resource not found"
	}
	return fmt.Sprintf("github resource not found: %s", e.Resource)
}

// PrimaryRateLimitError is returned when GitHub's primary rate limit has been
// exceeded.
type PrimaryRateLimitError struct {
	Rate       RateInfo
	RetryAfter time.Duration
	Message    string
}

func (e *PrimaryRateLimitError) Error() string {
	if e.Message == "" {
		return "github primary rate limit exceeded"
	}
	return fmt.Sprintf("github primary rate limit exceeded: %s", e.Message)
}

// SecondaryRateLimitError is returned when GitHub's secondary (abuse) rate
// limit has been exceeded.
type SecondaryRateLimitError struct {
	RetryAfter time.Duration
	Message    string
}

func (e *SecondaryRateLimitError) Error() string {
	if e.Message == "" {
		return "github secondary rate limit exceeded"
	}
	return fmt.Sprintf("github secondary rate limit exceeded: %s", e.Message)
}

// TransientError wraps a potentially retryable error.
type TransientError struct {
	Cause error
}

func (e *TransientError) Error() string {
	return fmt.Sprintf("github transient error: %v", e.Cause)
}

func (e *TransientError) Unwrap() error {
	return e.Cause
}

// IsNoToken reports whether err is the sentinel no-token value.
func IsNoToken(err error) bool {
	return errors.Is(err, ErrNoToken)
}
