package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func snapshotIdentity(token string, revision int64) string {
	if token != "" {
		return token
	}
	return fmt.Sprintf("ephemeral:%d", revision)
}

// beginCorpusRead captures the identity shared by all reads in one composed
// operation. A caller-supplied pin is a precondition, not a refresh request.
func beginCorpusRead(ctx context.Context, c *corpus.Corpus, token string) (int64, error) {
	revision, err := c.CorpusRevision(ctx)
	if err != nil {
		return 0, err
	}
	if token == "" {
		return revision, nil
	}
	snapshot, err := c.ResolveReadSnapshot(ctx, token)
	if err != nil {
		return 0, mcpcontract.Unavailable("snapshot_unavailable", err.Error())
	}
	if snapshot.ObservationWatermark != revision {
		return 0, mcpcontract.Unavailable("snapshot_expired", fmt.Sprintf("snapshot observation watermark %d is no longer current; current watermark is %d", snapshot.ObservationWatermark, revision))
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
			return mcpcontract.Unavailable("snapshot_expired", fmt.Sprintf("corpus changed while the read was assembled: started at watermark %d, current watermark is %d; reread after an explicit sync", stale.Expected, stale.Current))
		}
		return err
	}
	return nil
}
