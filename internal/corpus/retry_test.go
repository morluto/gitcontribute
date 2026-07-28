package corpus

import (
	"context"
	"errors"
	"testing"

	sqlite3 "modernc.org/sqlite/lib"
)

type codedTestError struct {
	code int
}

func (e *codedTestError) Error() string { return "sqlite test error" }
func (e *codedTestError) Code() int     { return e.code }

func TestRetryBusyRetriesExtendedBusyAndPreservesValue(t *testing.T) {
	t.Parallel()
	attempts := 0
	value, err := RetryBusyValue(context.Background(), func(context.Context) (string, error) {
		attempts++
		if attempts < 3 {
			return "", &codedTestError{code: sqlite3.SQLITE_BUSY_SNAPSHOT}
		}
		return "stored", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if value != "stored" || attempts != 3 {
		t.Fatalf("retry result = (%q, %d attempts), want (stored, 3)", value, attempts)
	}
}

func TestRetryBusyDoesNotRetryOtherErrors(t *testing.T) {
	t.Parallel()
	want := errors.New("permanent")
	attempts := 0
	err := RetryBusy(context.Background(), func(context.Context) error {
		attempts++
		return want
	})
	if !errors.Is(err, want) || attempts != 1 {
		t.Fatalf("retry result = (%v, %d attempts), want permanent error once", err, attempts)
	}
}

func TestRetryBusyHonorsCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	err := RetryBusy(ctx, func(context.Context) error {
		attempts++
		cancel()
		return &codedTestError{code: sqlite3.SQLITE_BUSY}
	})
	if !errors.Is(err, context.Canceled) || attempts != 1 {
		t.Fatalf("retry result = (%v, %d attempts), want cancellation after one attempt", err, attempts)
	}
}
