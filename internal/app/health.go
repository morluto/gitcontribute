package app

import (
	"context"
	"fmt"
	"time"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/health"
)

// RepositoryHealthWithOptions returns a deterministic repository health report
// using the provided analysis window and stale threshold.
func (s *Service) RepositoryHealthWithOptions(ctx context.Context, repo contracts.RepoRef, opts health.Options) (*health.Report, error) {
	ref, err := domain.NewRepoRef(repo.Owner, repo.Repo)
	if err != nil {
		return nil, err
	}

	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return nil, err
	}

	repoProjection, err := c.GetRepository(ctx, ref.Owner(), ref.Repo())
	if err != nil {
		return nil, fmt.Errorf("get repository: %w", err)
	}
	if repoProjection == nil {
		return nil, fmt.Errorf("%w: %s", errRepositoryNotFound, ref)
	}

	if opts.Now.IsZero() {
		opts.Now = s.now()
	}
	if opts.StaleThreshold == 0 {
		opts.StaleThreshold = 14 * 24 * time.Hour
	}

	return health.Compute(ctx, c, repoProjection.ID, opts)
}
