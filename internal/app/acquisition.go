package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/morluto/gitcontribute/internal/acquire"
	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/domain"
)

// Acquire clones or fetches a repository into the managed cache, records the
// resolved remote URL/default branch/commit SHA/acquired time, and indexes the
// clean checkout into the corpus. It does not execute repository code.
func (s *Service) Acquire(ctx context.Context, repo cli.RepoRef, remote string) (*cli.AcquisitionResult, error) {
	ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Repo}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	remote = strings.TrimSpace(remote)
	if remote == "" {
		remote = fmt.Sprintf("https://github.com/%s/%s.git", ref.Owner, ref.Repo)
	}

	cacheRoot, err := s.paths.AcquisitionCacheDir()
	if err != nil {
		return nil, err
	}

	mgr, err := acquire.NewManager(cacheRoot, nil)
	if err != nil {
		return nil, fmt.Errorf("create acquisition manager: %w", err)
	}

	acq, err := mgr.Acquire(ctx, ref.Owner, ref.Repo, remote)
	if err != nil {
		return nil, fmt.Errorf("acquire %s: %w", ref, err)
	}
	defer func() { _ = mgr.Cleanup(context.WithoutCancel(ctx), acq) }()

	snapshot, err := codeindex.Index(ctx, acq.Path, codeindex.Options{})
	if err != nil {
		return nil, fmt.Errorf("index acquired checkout: %w", err)
	}

	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	_, inserted, err := c.StoreCodeSnapshot(ctx, ref, snapshot)
	if err != nil {
		return nil, fmt.Errorf("store code snapshot: %w", err)
	}

	message := "acquired and indexed"
	if !inserted {
		message = "acquired; snapshot already indexed"
	}

	return &cli.AcquisitionResult{
		Repo:          repo,
		Remote:        acq.Remote,
		DefaultBranch: acq.DefaultBranch,
		CommitSHA:     acq.CommitSHA,
		Files:         len(snapshot.Documents),
		Bytes:         snapshot.TotalBytes,
		Indexed:       true,
		Inserted:      inserted,
		AcquiredAt:    formatTime(acq.AcquiredAt),
		Message:       message,
	}, nil
}
