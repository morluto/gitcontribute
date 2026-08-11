package app

import (
	"errors"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// parsedThreadReference is the canonical identity of one thread lookup. Kind
// explicitly represents either an exact issue/pull-request kind or both kinds.
// Boundary DTOs are converted once before corpus batch keys are built.
type parsedThreadReference struct {
	repository domain.RepoRef
	kind       corpus.ThreadKindFilter
	number     int
}

func parseThreadReference(input mcpcontract.ThreadRef) (parsedThreadReference, error) {
	repository, err := domain.NewRepoRef(input.Owner, input.Repo)
	if err != nil {
		return parsedThreadReference{}, err
	}
	kind, err := corpus.ParseThreadKindFilter(input.Kind)
	if err != nil {
		return parsedThreadReference{}, err
	}
	if input.Number < 1 {
		return parsedThreadReference{}, errors.New("thread number must be positive")
	}
	return parsedThreadReference{repository: repository, kind: kind, number: input.Number}, nil
}

func (r parsedThreadReference) repositoryKey() corpus.RepositoryKey {
	return corpus.RepositoryKey{Owner: r.repository.Owner(), Name: r.repository.Repo()}
}

func (r parsedThreadReference) threadKey(repositoryID int64) corpus.ThreadKey {
	return corpus.ThreadKey{RepositoryID: repositoryID, Kind: r.kind, Number: r.number}
}

func (r parsedThreadReference) wire() mcpcontract.ThreadRef {
	return mcpcontract.ThreadRef{Owner: r.repository.Owner(), Repo: r.repository.Repo(), Kind: r.kind.String(), Number: r.number}
}
