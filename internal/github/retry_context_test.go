package github

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestRetryObservationCarriesRequestContext(t *testing.T) {
	type ctxKey string
	const key ctxKey = "retry-observation-context"

	ft := &fakeTransport{results: []fakeResult{
		{status: http.StatusInternalServerError, body: "boom"},
		{status: http.StatusOK, body: "ok"},
	}}
	var observations []RetryObservation
	rt := &retryTransport{Base: ft, Config: &RetryConfig{
		MaxAttempts: 2, BaseDelay: time.Nanosecond, MaxDelay: time.Nanosecond,
		Sleeper:   func(context.Context, time.Duration) error { return nil },
		OnAttempt: func(o RetryObservation) { observations = append(observations, o) },
	}}

	ctx := context.WithValue(context.Background(), key, "present")
	resp, err := rt.RoundTrip(newGetRequest(t, "http://example.com/repos/o/r").WithContext(ctx))
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("close response body: %v", err)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if len(observations) != 2 {
		t.Fatalf("observations = %d, want 2", len(observations))
	}
	for i, obs := range observations {
		if obs.Context == nil || obs.Context.Value(key) != "present" {
			t.Fatalf("observation[%d].Context missing expected value", i)
		}
	}
}

func TestRetryObservationContextRespectsCancellation(t *testing.T) {
	ft := &fakeTransport{results: []fakeResult{
		{status: http.StatusInternalServerError, body: "boom"},
		{status: http.StatusOK, body: "ok"},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	signaled := make(chan struct{})

	var obs RetryObservation
	rt := &retryTransport{Base: ft, Config: &RetryConfig{
		MaxAttempts: 2, BaseDelay: time.Hour, MaxDelay: time.Hour,
		Sleeper: func(ctx context.Context, _ time.Duration) error {
			close(signaled)
			<-ctx.Done()
			return ctx.Err()
		},
		OnAttempt: func(o RetryObservation) { obs = o },
	}}

	go func() {
		<-signaled
		cancel()
	}()

	resp, err := rt.RoundTrip(newGetRequest(t, "http://example.com/repos/o/r").WithContext(ctx))
	if resp != nil {
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Errorf("close response body: %v", err)
			}
		}()
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if obs.Context == nil {
		t.Fatal("RetryObservation.Context is nil")
	}
	if !errors.Is(obs.Context.Err(), context.Canceled) {
		t.Fatalf("observation context err = %v, want context.Canceled", obs.Context.Err())
	}
}
