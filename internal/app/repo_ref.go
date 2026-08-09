package app

import (
	"errors"

	"github.com/morluto/gitcontribute/internal/domain"
)

func optionalRepoRef(owner, repo string) (domain.RepoRef, error) {
	if owner == "" && repo == "" {
		return domain.RepoRef{}, nil
	}
	if owner == "" || repo == "" {
		return domain.RepoRef{}, errors.New("repository owner and name must be provided together")
	}
	return domain.NewRepoRef(owner, repo)
}
