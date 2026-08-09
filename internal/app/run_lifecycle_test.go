package app

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestFailRunOnErrorJoinsFinalizationFailure(t *testing.T) {
	t.Parallel()
	svc := newJobTestService(t)
	if err := svc.corpus.Close(); err != nil {
		t.Fatalf("close corpus: %v", err)
	}

	original := errors.New("acquisition failed")
	resultErr := original
	failRunOnError(context.Background(), svc.corpus, 42, &resultErr)

	if !errors.Is(resultErr, original) {
		t.Fatalf("original operation error was lost: %v", resultErr)
	}
	if !strings.Contains(resultErr.Error(), "record failed run 42") {
		t.Fatalf("finalization failure was not reported: %v", resultErr)
	}
}
