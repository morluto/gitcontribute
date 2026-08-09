package github

import (
	"context"
	"io"
	"net/http"
	"time"
)

// attemptTimeoutTransport bounds one network attempt without consuming retry
// backoff or rate-limiter wait time from the caller's request budget.
type attemptTimeoutTransport struct {
	Base    http.RoundTripper
	Timeout time.Duration
}

func (t *attemptTimeoutTransport) base() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return http.DefaultTransport
}

func (t *attemptTimeoutTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.Timeout <= 0 {
		return t.base().RoundTrip(req)
	}
	ctx, cancel := context.WithTimeout(req.Context(), t.Timeout)
	resp, err := t.base().RoundTrip(req.Clone(ctx))
	if err != nil {
		cancel()
		return nil, err
	}
	if resp.Body == nil {
		cancel()
		return resp, nil
	}
	resp.Body = &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r *cancelOnClose) Close() error {
	err := r.ReadCloser.Close()
	r.cancel()
	return err
}
