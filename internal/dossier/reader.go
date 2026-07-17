package dossier

import (
	"context"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/repository"
)

// ThreadQuery is a narrow, deterministic read request for threads.
// An empty State means any state. Merged is ignored unless Kind is PullRequestKind.
type ThreadQuery struct {
	Kind   domain.ThreadKind
	State  domain.ThreadState
	Merged *bool
	Limit  int
}

// Reader is a narrow source interface for building dossiers.
// Implementations must return deterministic, source-backed data with references.
type Reader interface {
	repository.Reader
	ReadThreads(ctx context.Context, ref domain.RepoRef, q ThreadQuery) ([]domain.Thread, []domain.SourceRef, error)
}
