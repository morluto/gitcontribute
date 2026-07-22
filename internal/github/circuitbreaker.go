package github

import (
	"errors"
	"sync"
	"time"
)

const (
	defaultCBMaxFailures  = 5
	defaultCBHalfOpenWait = 30 * time.Second
	defaultCBProbeTimeout = 5 * time.Second
)

// CircuitState represents the state of the circuit breaker.
type CircuitState int

const (
	CircuitClosed   CircuitState = iota // normal operation, requests pass through
	CircuitOpen                         // failing fast, no requests allowed
	CircuitHalfOpen                     // probing, single request allowed
)

// circuitBreaker implements the circuit breaker pattern for external HTTP calls.
// It tracks consecutive failures and opens the circuit once a threshold is
// reached, preventing cascading failures and giving the downstream service
// time to recover.
type circuitBreaker struct {
	mu           sync.Mutex
	maxFailures  int
	halfOpenWait time.Duration
	probeTimeout time.Duration
	clock        func() time.Time

	consecutiveFailures int
	lastFailure         time.Time
	probeStarted        time.Time
	state               CircuitState
}

func newCircuitBreaker(maxFailures int, halfOpenWait, probeTimeout time.Duration) *circuitBreaker {
	if maxFailures <= 0 {
		maxFailures = defaultCBMaxFailures
	}
	if halfOpenWait <= 0 {
		halfOpenWait = defaultCBHalfOpenWait
	}
	if probeTimeout <= 0 {
		probeTimeout = defaultCBProbeTimeout
	}
	return &circuitBreaker{
		maxFailures:  maxFailures,
		halfOpenWait: halfOpenWait,
		probeTimeout: probeTimeout,
		clock:        time.Now,
		state:        CircuitClosed,
	}
}

// setClock overrides the time source for testing.
func (cb *circuitBreaker) setClock(clock func() time.Time) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.clock = clock
}

// allow reports whether a request should be permitted based on the current
// circuit state. If the circuit is half-open and a probe is in-flight,
// subsequent requests are rejected until the probe completes.
func (cb *circuitBreaker) allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		now := cb.clock()
		if now.Sub(cb.lastFailure) >= cb.halfOpenWait {
			cb.state = CircuitHalfOpen
			cb.probeStarted = now
			return true
		}
		return false
	case CircuitHalfOpen:
		now := cb.clock()
		if now.Sub(cb.probeStarted) >= cb.probeTimeout {
			cb.state = CircuitOpen
			cb.lastFailure = now
			cb.probeStarted = time.Time{}
		}
		return false
	default:
		return true
	}
}

// recordSuccess closes the circuit after a successful probe.
func (cb *circuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.consecutiveFailures = 0
	if cb.state == CircuitHalfOpen {
		cb.state = CircuitClosed
		cb.probeStarted = time.Time{}
	}
}

// recordFailure increments the failure counter and opens the circuit if the
// threshold is exceeded.
func (cb *circuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.consecutiveFailures++
	cb.lastFailure = cb.clock()
	if cb.consecutiveFailures >= cb.maxFailures {
		cb.state = CircuitOpen
		cb.probeStarted = time.Time{}
	}
}

// ErrCircuitOpen is returned when the circuit breaker rejects a request.
var ErrCircuitOpen = errors.New("circuit breaker is open")
