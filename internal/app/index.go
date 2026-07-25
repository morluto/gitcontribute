package app

import (
	"context"

	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/domain"
)

// Index records a bounded immutable code snapshot from a clean local checkout.
func (s *Service) Index(ctx context.Context, repo contracts.RepoRef, path string) (*contracts.IndexResult, error) {
	ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Repo}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	repoPath, commit, err := codeindex.Probe(ctx, path)
	if err != nil {
		return nil, err
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	existing, err := c.CodeSnapshot(ctx, ref, commit)
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.Manifest.FormatVersion == codeindex.FormatVersion {
		confirmedPath, confirmedCommit, err := codeindex.Probe(ctx, repoPath)
		if err != nil {
			return nil, err
		}
		if confirmedPath == repoPath && confirmedCommit == commit {
			return &contracts.IndexResult{
				Repo: repo, Path: repoPath, Commit: commit,
				Files: existing.Manifest.IndexedFiles, Bytes: existing.TotalBytes,
				Inserted: false, Message: "snapshot already indexed",
			}, nil
		}
		repoPath = confirmedPath
	}
	snapshot, err := codeindex.Index(ctx, repoPath, codeindex.Options{})
	if err != nil {
		return nil, err
	}
	_, inserted, err := c.StoreCodeSnapshot(ctx, ref, snapshot)
	if err != nil {
		return nil, err
	}
	message := "snapshot already indexed"
	if inserted {
		message = "snapshot stored"
	}
	return &contracts.IndexResult{
		Repo: repo, Path: snapshot.RepoPath, Commit: snapshot.Commit,
		Files: len(snapshot.Documents), Bytes: snapshot.TotalBytes, Inserted: inserted, Message: message,
	}, nil
}
