package github

import (
	"net/http"
	"testing"
	"time"
)

func TestNewClientUsesBoundedDefaultAttemptTimeout(t *testing.T) {
	client, err := NewClient(Config{})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if client.downloadClient.Timeout != 0 {
		t.Fatalf("default client timeout = %s, want no whole-request timeout", client.downloadClient.Timeout)
	}
	transport, ok := client.downloadClient.Transport.(*attemptTimeoutTransport)
	if !ok || transport.Timeout != defaultHTTPTimeout {
		t.Fatalf("default download transport = %#v, want %s per attempt", client.downloadClient.Transport, defaultHTTPTimeout)
	}
}

func TestNewClientPreservesExplicitHTTPTimeout(t *testing.T) {
	const timeout = 7 * time.Second
	client, err := NewClient(Config{HTTPClient: &http.Client{Timeout: timeout}})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if client.downloadClient.Timeout != timeout {
		t.Fatalf("explicit HTTP timeout = %s, want %s", client.downloadClient.Timeout, timeout)
	}
}
