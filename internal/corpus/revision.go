package corpus

import (
	"context"
	"errors"
	"fmt"
)

// StaleCorpusRevisionError means a caller pinned a read to a corpus state
// that is no longer current. Callers must reread after an explicit sync; a
// read must never refresh the corpus implicitly.
type StaleCorpusRevisionError struct {
	Expected int64
	Current  int64
}

func (e *StaleCorpusRevisionError) Error() string {
	return fmt.Sprintf("corpus revision %d is stale; current revision is %d", e.Expected, e.Current)
}

// CorpusRevision returns the product-owned logical revision of the corpus.
// It is intentionally independent from SQLite schema and observation
// sequence versions, so it can be passed between independently bounded reads.
func (c *Corpus) CorpusRevision(ctx context.Context) (int64, error) {
	var revision int64
	if err := c.db.QueryRowContext(ctx, `SELECT revision FROM corpus_state WHERE id = 1`).Scan(&revision); err != nil {
		return 0, fmt.Errorf("read corpus revision: %w", err)
	}
	return revision, nil
}

// RequireCorpusRevision checks a previously captured read identity.
func (c *Corpus) RequireCorpusRevision(ctx context.Context, expected int64) error {
	current, err := c.CorpusRevision(ctx)
	if err != nil {
		return err
	}
	if current != expected {
		return &StaleCorpusRevisionError{Expected: expected, Current: current}
	}
	return nil
}

// IsStaleCorpusRevision reports whether err is the typed stale-read result.
func IsStaleCorpusRevision(err error) bool {
	var stale *StaleCorpusRevisionError
	return errors.As(err, &stale)
}
