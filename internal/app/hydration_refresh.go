package app

import (
	"context"
	"fmt"
)

// refreshHydrationThreadHeader fetches the current exact thread header before
// child facets. It reuses the sync projection path so hydration cannot derive
// coverage freshness from a stale or missing local header.
func (s *Service) refreshHydrationThreadHeader(ctx context.Context, target hydrationTarget) error {
	ref, number := target.repository, target.number

	c, err := s.openCorpus(ctx)
	if err != nil {
		return err
	}
	repository, err := c.GetRepository(ctx, ref.Owner(), ref.Repo())
	if err != nil {
		return fmt.Errorf("get repository: %w", err)
	}
	if repository == nil {
		return fmt.Errorf("repository %s has not been synced", ref)
	}
	// Reader construction has no context parameter; the exact GitHub request
	// below receives ctx and remains cancellation-aware.
	reader, err := s.githubReader() //nolint:contextcheck
	if err != nil {
		return err
	}

	writer := &syncThreadWriter{
		ctx:          ctx,
		corpus:       c,
		owner:        ref.Owner(),
		repo:         ref.Repo(),
		repositoryID: repository.ID,
		kind:         target.kind.syncKind(),
	}
	_, err = syncExactThreadHeaders(ctx, reader, ref, []int{number}, newSyncRequestBudget(1), writer)
	return err
}
