package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// beginCorpusRead captures the identity shared by all reads in one composed
// operation. A caller-supplied pin is a precondition, not a refresh request.
func beginCorpusRead(ctx context.Context, c *corpus.Corpus, pin *int64) (int64, error) {
	if pin != nil && *pin < 0 {
		return 0, mcpcontract.InvalidArgument("corpus_revision", "must be non-negative", map[string]any{"corpus_revision": 0})
	}
	revision, err := c.CorpusRevision(ctx)
	if err != nil {
		return 0, err
	}
	if pin != nil && *pin != revision {
		return 0, mcpcontract.Unavailable("corpus_revision_stale", fmt.Sprintf("requested corpus revision %d is no longer current; current revision is %d; reread after an explicit sync", *pin, revision))
	}
	return revision, nil
}

// finishCorpusRead proves that no corpus mutation occurred while a composed
// result was assembled. This is deliberately checked before the result is
// returned so a multi-read response cannot carry a silently mixed state.
func finishCorpusRead(ctx context.Context, c *corpus.Corpus, revision int64) error {
	if err := c.RequireCorpusRevision(ctx, revision); err != nil {
		var stale *corpus.StaleCorpusRevisionError
		if errors.As(err, &stale) {
			return mcpcontract.Unavailable("corpus_revision_stale", fmt.Sprintf("corpus changed while the read was assembled: started at revision %d, current revision is %d; reread after an explicit sync", stale.Expected, stale.Current))
		}
		return err
	}
	return nil
}
