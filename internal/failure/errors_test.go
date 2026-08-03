package failure

import (
	"errors"
	"strings"
	"testing"
)

func TestNotFoundPreservesCauseAndKind(t *testing.T) {
	cause := errors.New("missing repository")
	err := NotFound(cause)
	if !Is(err, KindNotFound) || !errors.Is(err, cause) {
		t.Fatalf("classified error = %v", err)
	}
	if got := Format(err); !strings.HasPrefix(got, "not_found: missing repository") {
		t.Fatalf("formatted error = %q", got)
	}
}

func TestNotFoundWithoutCauseHasStableMessage(t *testing.T) {
	err := NotFound(nil)
	if !Is(err, KindNotFound) || err.Error() != "requested object not found" {
		t.Fatalf("not found error = %v", err)
	}
}
