package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
	"github.com/morluto/gitcontribute/internal/repositorycontext"
)

type repositoryContextSyncRequest struct {
	repositories []domain.RepoRef
	maxRequests  int
}

func parseRepositoryContextSyncInput(in mcpcontract.SyncRepositoryContextInput) (repositoryContextSyncRequest, mcpcontract.SyncRepositoryContextInput, error) {
	if len(in.Repositories) < 1 || len(in.Repositories) > 100 {
		return repositoryContextSyncRequest{}, mcpcontract.SyncRepositoryContextInput{}, errors.New("repositories must contain 1 to 100 items")
	}
	repositories := make([]domain.RepoRef, len(in.Repositories))
	canonical := make([]mcpcontract.RepositoryRef, len(in.Repositories))
	seen := make(map[string]struct{}, len(in.Repositories))
	for i, input := range in.Repositories {
		ref, err := domain.NewRepoRef(input.Owner, input.Repo)
		if err != nil {
			return repositoryContextSyncRequest{}, mcpcontract.SyncRepositoryContextInput{}, err
		}
		key := strings.ToLower(ref.String())
		if _, duplicate := seen[key]; duplicate {
			return repositoryContextSyncRequest{}, mcpcontract.SyncRepositoryContextInput{}, mcpcontract.InvalidArgument("repositories", fmt.Sprintf("duplicate repository %s", ref), nil)
		}
		seen[key] = struct{}{}
		repositories[i] = ref
		canonical[i] = mcpcontract.RepositoryRef{Owner: ref.Owner(), Repo: ref.Repo()}
	}
	request, err := newRepositoryContextSyncRequest(repositories, in.MaxRequests)
	if err != nil {
		return repositoryContextSyncRequest{}, mcpcontract.SyncRepositoryContextInput{}, err
	}
	return request, mcpcontract.SyncRepositoryContextInput{Repositories: canonical, MaxRequests: request.maxRequests}, nil
}

func newRepositoryContextSyncRequest(repositories []domain.RepoRef, maxRequests int) (repositoryContextSyncRequest, error) {
	if maxRequests == 0 {
		maxRequests = defaultSyncBatchMaxRequests
	}
	if maxRequests < repositorycontext.RequestCost() || maxRequests > defaultSyncBatchMaxRequests {
		return repositoryContextSyncRequest{}, fmt.Errorf(
			"max requests must be between %d and %d",
			repositorycontext.RequestCost(), defaultSyncBatchMaxRequests,
		)
	}
	return repositoryContextSyncRequest{repositories: append([]domain.RepoRef(nil), repositories...), maxRequests: maxRequests}, nil
}
