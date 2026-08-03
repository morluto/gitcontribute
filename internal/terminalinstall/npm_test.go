package terminalinstall

import (
	"errors"
	"strings"
	"testing"
)

func TestCommandFailureIncludesOutputWithoutDroppingCause(t *testing.T) {
	cause := errors.New("exit status 1")
	err := commandFailure("install persistent CLI", []byte("permission denied\n"), cause)
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("command failure = %v", err)
	}
}

func TestCommandFailureOmitsEmptyOutput(t *testing.T) {
	err := commandFailure("resolve prefix", nil, errors.New("failed"))
	if err.Error() != "resolve prefix: failed" {
		t.Fatalf("command failure = %q", err)
	}
}
