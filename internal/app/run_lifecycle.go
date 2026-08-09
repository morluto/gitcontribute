package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
)

const runCleanupTimeout = 5 * time.Second

// failRunOnError records an operation failure after its caller's context may
// have been cancelled. If persistence also fails, retain both errors so callers
// never mistake an unrecorded failed run for a cleanly finalized one.
func failRunOnError(ctx context.Context, c *corpus.Corpus, runID int64, resultErr *error) {
	if *resultErr == nil {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runCleanupTimeout)
	defer cancel()
	if err := c.FailRun(cleanupCtx, runID, (*resultErr).Error()); err != nil {
		*resultErr = errors.Join(*resultErr, fmt.Errorf("record failed run %d: %w", runID, err))
	}
}
