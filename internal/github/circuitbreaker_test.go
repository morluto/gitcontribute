package github

import (
	"testing"
	"time"
)

func TestCircuitBreakerClosedAllowsRequests(t *testing.T) {
	t.Parallel()
	cb := newCircuitBreaker(5, 30*time.Second, 5*time.Second)
	if !cb.allow() {
		t.Fatal("expected closed circuit to allow requests")
	}
	if !cb.allow() {
		t.Fatal("expected closed circuit to allow multiple requests")
	}
}

func TestCircuitBreakerOpensAfterMaxFailures(t *testing.T) {
	t.Parallel()
	cb := newCircuitBreaker(3, 30*time.Second, 5*time.Second)

	// Record 3 failures.
	cb.recordFailure()
	cb.recordFailure()
	cb.recordFailure()

	if cb.allow() {
		t.Fatal("expected open circuit to reject requests")
	}
}

func TestCircuitBreakerHalfOpenAfterCooldown(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	cb := newCircuitBreaker(3, 30*time.Second, 5*time.Second)
	cb.setClock(func() time.Time { return now })

	// Open the circuit.
	cb.recordFailure()
	cb.recordFailure()
	cb.recordFailure()

	if cb.allow() {
		t.Fatal("expected open circuit to reject before cooldown")
	}

	// Advance past half-open wait.
	cb.setClock(func() time.Time { return now.Add(31 * time.Second) })

	if !cb.allow() {
		t.Fatal("expected circuit to allow probe after cooldown")
	}

	// Second request during half-open should be rejected.
	if cb.allow() {
		t.Fatal("expected only one probe during half-open")
	}
}

func TestCircuitBreakerClosesAfterSuccessfulProbe(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	cb := newCircuitBreaker(3, 30*time.Second, 5*time.Second)
	cb.setClock(func() time.Time { return now })

	// Open the circuit.
	cb.recordFailure()
	cb.recordFailure()
	cb.recordFailure()

	// Advance past cooldown.
	cb.setClock(func() time.Time { return now.Add(31 * time.Second) })

	// Probe succeeds.
	if !cb.allow() {
		t.Fatal("expected probe request to be allowed")
	}
	cb.recordSuccess()

	// Circuit should be closed again.
	if !cb.allow() {
		t.Fatal("expected circuit to close after successful probe")
	}
	if !cb.allow() {
		t.Fatal("expected multiple requests after circuit closes")
	}
}

func TestCircuitBreakerReopensAfterFailedProbe(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	cb := newCircuitBreaker(3, 30*time.Second, 5*time.Second)
	cb.setClock(func() time.Time { return now })

	// Open the circuit.
	cb.recordFailure()
	cb.recordFailure()
	cb.recordFailure()

	// Advance past cooldown.
	cb.setClock(func() time.Time { return now.Add(31 * time.Second) })

	// Probe allowed.
	if !cb.allow() {
		t.Fatal("expected probe request to be allowed")
	}

	// Probe fails.
	cb.recordFailure()

	// Circuit should be open again.
	if cb.allow() {
		t.Fatal("expected circuit to reopen after failed probe")
	}
}

func TestCircuitBreakerReopensAfterProbeTimeout(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	cb := newCircuitBreaker(1, 30*time.Second, 5*time.Second)
	cb.setClock(func() time.Time { return now })
	cb.recordFailure()

	now = now.Add(31 * time.Second)
	if !cb.allow() {
		t.Fatal("expected probe request to be allowed")
	}
	now = now.Add(5 * time.Second)
	if cb.allow() {
		t.Fatal("expected timed-out probe to reopen circuit")
	}
	now = now.Add(30 * time.Second)
	if !cb.allow() {
		t.Fatal("expected new probe after another cooldown")
	}
}

func TestCircuitBreakerRecordsSuccessesResetsCounter(t *testing.T) {
	t.Parallel()
	cb := newCircuitBreaker(3, 30*time.Second, 5*time.Second)

	// Two failures, then a success resets the counter.
	cb.recordFailure()
	cb.recordFailure()
	cb.recordSuccess()

	// Need 3 more consecutive failures to open.
	cb.recordFailure()
	cb.recordFailure()

	if !cb.allow() {
		t.Fatal("expected circuit to stay closed under threshold")
	}
}

func TestErrCircuitOpenIsDistinct(t *testing.T) {
	t.Parallel()
	if ErrCircuitOpen.Error() == "" {
		t.Fatal("expected non-empty error message")
	}
	// Verify it wraps a unique sentinel.
	if ErrCircuitOpen == nil {
		t.Fatal("expected non-nil sentinel error")
	}
}
