package github

import (
	"context"
	"net/http"

	"golang.org/x/time/rate"
)

// Limiter paces outbound requests. It matches the subset of rate.Limiter used by
// the transport so tests can inject a no-op or fake implementation.
type Limiter interface {
	WaitN(ctx context.Context, n int) error
}

// NewRateLimiter returns a token-bucket limiter suitable for production use.
func NewRateLimiter(rps float64, burst int) Limiter {
	return rate.NewLimiter(rate.Limit(rps), burst)
}

// RateLimitedTransport wraps an underlying RoundTripper with request pacing.
// It does not log request contents or tokens.
type RateLimitedTransport struct {
	Base    http.RoundTripper
	Limiter Limiter
}

func (t *RateLimitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.Limiter != nil {
		if err := t.Limiter.WaitN(req.Context(), 1); err != nil {
			return nil, err
		}
	}
	if t.Base == nil {
		return http.DefaultTransport.RoundTrip(req)
	}
	return t.Base.RoundTrip(req)
}
