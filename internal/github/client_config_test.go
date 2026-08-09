package github

import (
	"net/http"
	"testing"
	"time"
)

func TestNewClientUsesBoundedDefaultHTTPTimeout(t *testing.T) {
	client, err := NewClient(Config{})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if client.downloadClient.Timeout != defaultHTTPTimeout {
		t.Fatalf("default HTTP timeout = %s, want %s", client.downloadClient.Timeout, defaultHTTPTimeout)
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
