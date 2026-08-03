package github

import (
	"context"
	"net/http"
	"testing"
)

func newGetRequest(t *testing.T, rawurl string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawurl, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	return req
}
