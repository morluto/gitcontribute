package app

import (
	"context"
	"errors"
	"testing"
)

func TestWorkflowGateSerializesAndCancelsWaiters(t *testing.T) {
	t.Parallel()
	var gate workflowGate
	release, err := gate.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := gate.acquire(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("blocked acquisition error = %v", err)
	}

	release()
	release() // release is deliberately idempotent.
	secondRelease, err := gate.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	secondRelease()
}
