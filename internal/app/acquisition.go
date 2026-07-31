package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/acquire"
	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/domain"
)

// Acquire clones or fetches a repository into the managed cache, records the
// resolved remote URL/default branch/commit SHA/acquired time, and indexes the
// clean checkout into the corpus. It does not execute repository code.
func (s *Service) Acquire(ctx context.Context, repo contracts.RepoRef, remote string) (result *contracts.AcquisitionResult, returnErr error) {
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
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err := mgr.Cleanup(cleanupCtx, acq); err != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("cleanup acquired checkout: %w", err))
		}
	}()

	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	existing, err := c.CodeSnapshot(ctx, ref, acq.CommitSHA)
	if err != nil {
		return nil, fmt.Errorf("find code snapshot: %w", err)
	}
	if existing != nil && existing.Manifest.FormatVersion == codeindex.FormatVersion {
		_, confirmedCommit, err := codeindex.Probe(ctx, acq.Path)
		if err != nil {
			return nil, fmt.Errorf("verify acquired checkout: %w", err)
		}
		if confirmedCommit != acq.CommitSHA {
			return nil, fmt.Errorf("acquired checkout changed before snapshot reuse: commit %q", confirmedCommit)
		}
		revision, err := c.CorpusRevision(ctx)
		if err != nil {
			return nil, err
		}
		return &contracts.AcquisitionResult{
			Repo:           repo,
			Remote:         acq.Remote,
			DefaultBranch:  acq.DefaultBranch,
			CommitSHA:      acq.CommitSHA,
			Files:          existing.Manifest.IndexedFiles,
			Bytes:          existing.TotalBytes,
			Indexed:        true,
			Inserted:       false,
			AcquiredAt:     formatTime(acq.AcquiredAt),
			Message:        "acquired; snapshot already indexed",
			IndexManifest:  existing.Manifest,
			CorpusRevision: revision,
		}, nil
	}

	snapshot, err := codeindex.Index(ctx, acq.Path, codeindex.Options{})
	if err != nil {
		return nil, fmt.Errorf("index acquired checkout: %w", err)
	}
	_, inserted, revision, err := c.StoreCodeSnapshotWithRevision(ctx, ref, snapshot)
	if err != nil {
		return nil, fmt.Errorf("store code snapshot: %w", err)
	}

	message := "acquired and indexed"
	if !inserted {
		message = "acquired; snapshot already indexed"
	}

	return &contracts.AcquisitionResult{
		Repo:           repo,
		Remote:         acq.Remote,
		DefaultBranch:  acq.DefaultBranch,
		CommitSHA:      acq.CommitSHA,
		Files:          len(snapshot.Documents),
		Bytes:          snapshot.TotalBytes,
		Indexed:        true,
		Inserted:       inserted,
		AcquiredAt:     formatTime(acq.AcquiredAt),
		Message:        message,
		IndexManifest:  snapshot.Manifest,
		CorpusRevision: revision,
	}, nil
}
