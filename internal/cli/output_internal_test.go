package cli

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/contracts"
)

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestConfirmSetupReturnsPromptWriteErrorWithoutReadingInput(t *testing.T) {
	want := errors.New("broken stderr")
	c := &CLI{
		stdin:  failingInput{},
		stderr: failingWriter{err: want},
	}
	confirmed, err := c.confirmSetup("Apply changes")
	if confirmed || !errors.Is(err, want) {
		t.Fatalf("confirm setup = (%t, %v), want false and %v", confirmed, err, want)
	}
}

type failingInput struct{}

func (failingInput) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestControlStatusHumanMarksExpiredRateLimitObservationStale(t *testing.T) {
	output := controlStatusHuman(&contracts.ControlStatusResult{
		RateLimits: []contracts.RateLimitState{{
			Resource: "core", Limit: 5000, Remaining: 4999,
			ObservedAt: "2026-08-08T23:00:00Z", ResetAt: "2026-08-08T23:59:00Z", Stale: true,
		}},
	})
	if !strings.Contains(output, "observed 2026-08-08T23:00:00Z), stale, resets 2026-08-08T23:59:00Z") {
		t.Fatalf("stale rate-limit observation was not identified in human output: %q", output)
	}
}
