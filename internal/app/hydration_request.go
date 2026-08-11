package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/facets"
)

type hydrateThreadInput struct {
	Kind     string
	Facets   []string
	MaxPages int
}

type hydrationTarget struct {
	repository domain.RepoRef
	number     int
	kind       hydrationThreadKind
	selection  facets.Selection
	maxPages   int
}

func parseHydrationTarget(repo contracts.RepoRef, number int, input hydrateThreadInput) (hydrationTarget, error) {
	repository, err := domain.NewRepoRef(repo.Owner, repo.Repo)
	if err != nil {
		return hydrationTarget{}, err
	}
	if number <= 0 {
		return hydrationTarget{}, errors.New("thread number must be positive")
	}
	kind, err := parseHydrationThreadKind(input.Kind)
	if err != nil {
		return hydrationTarget{}, err
	}
	selection, err := facets.ParseSelection(input.Facets)
	if err != nil {
		return hydrationTarget{}, err
	}
	maxPages := input.MaxPages
	if maxPages <= 0 {
		maxPages = 50
	}
	if maxPages > maxHydrationPages {
		return hydrationTarget{}, fmt.Errorf("max pages cannot exceed %d", maxHydrationPages)
	}
	return hydrationTarget{repository: repository, number: number, kind: kind, selection: selection, maxPages: maxPages}, nil
}

func (t hydrationTarget) wireRepository() contracts.RepoRef {
	return contracts.RepoRef{Owner: t.repository.Owner(), Repo: t.repository.Repo()}
}

type hydrationThreadKind struct {
	value domain.ThreadKind
}

func parseHydrationThreadKind(value string) (hydrationThreadKind, error) {
	if strings.TrimSpace(value) == "" {
		return hydrationThreadKind{}, nil
	}
	kind, err := domain.ParseThreadKind(value)
	if err != nil {
		return hydrationThreadKind{}, errors.New("thread kind must be issue or pull_request")
	}
	return hydrationThreadKind{value: kind}, nil
}

func (k hydrationThreadKind) specified() bool { return k.value != "" }

func (k hydrationThreadKind) syncKind() syncThreadKind {
	switch k.value {
	case domain.IssueKind:
		return syncIssues
	case domain.PullRequestKind:
		return syncPullRequests
	default:
		return syncAllThreads
	}
}
