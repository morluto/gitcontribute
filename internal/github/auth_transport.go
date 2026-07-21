package github

import (
	"errors"
	"fmt"
	"net/http"
)

// authTransport resolves a TokenSource per request and injects the resulting
// Authorization header. Resolution is per logical HTTP request so caller
// cancellation and credential rotation remain observable; the retry transport
// is nested below this transport and reuses the injected header for retries.
type authTransport struct {
	Base   http.RoundTripper
	Source TokenSource
}

func (t *authTransport) base() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return http.DefaultTransport
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.Source == nil {
		return t.base().RoundTrip(req)
	}

	token, err := t.Source.Token(req.Context())
	if errors.Is(err, ErrNoToken) {
		token, err = "", nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve token: %w", err)
	}
	if token != "" {
		cloned := req.Clone(req.Context())
		cloned.Header.Set("Authorization", "Bearer "+token)
		req = cloned
	}
	return t.base().RoundTrip(req)
}
