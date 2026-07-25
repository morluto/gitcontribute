package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/domain"
)

// refreshHydrationThreadHeader fetches the current exact thread header before
// child facets. It reuses the sync projection path so hydration cannot derive
// coverage freshness from a stale or missing local header.
func (s *Service) refreshHydrationThreadHeader(ctx context.Context, repo contracts.RepoRef, number int) error {
	ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Repo}
	if err := ref.Validate(); err != nil {
		return err
	}
	if number <= 0 {
		return errors.New("thread number must be positive")
	}

	c, err := s.openCorpus(ctx)
	if err != nil {
		return err
	}
	repository, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
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
		repositoryID: repository.ID,
		kind:         "both",
	}
	_, err = syncExactThreadHeaders(ctx, reader, ref, []int{number}, newSyncRequestBudget(1), writer)
	return err
}
