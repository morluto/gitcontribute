package app

import (
	"fmt"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/facets"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

type parsedCoverageTarget interface {
	repository() domain.RepoRef
	thread() (domain.ThreadKind, int, bool)
	wire() mcpcontract.CoverageTarget
	key() string
	expectedFacets() []string
}

type repositoryCoverageTarget struct{ repo domain.RepoRef }

func (t repositoryCoverageTarget) repository() domain.RepoRef { return t.repo }
func (repositoryCoverageTarget) thread() (domain.ThreadKind, int, bool) {
	return "", 0, false
}
func (t repositoryCoverageTarget) wire() mcpcontract.CoverageTarget {
	return mcpcontract.CoverageTarget{Type: mcpcontract.CoverageTargetRepository, Repository: mcpcontract.RepositoryRef{Owner: t.repo.Owner(), Repo: t.repo.Repo()}}
}
func (t repositoryCoverageTarget) key() string { return t.repo.String() }
func (repositoryCoverageTarget) expectedFacets() []string {
	return []string{"metadata", "threads", FacetContributionGuidance}
}

type threadCoverageTarget struct {
	repo   domain.RepoRef
	kind   domain.ThreadKind
	number int
}

func (t threadCoverageTarget) repository() domain.RepoRef { return t.repo }
func (t threadCoverageTarget) thread() (domain.ThreadKind, int, bool) {
	return t.kind, t.number, true
}
func (t threadCoverageTarget) wire() mcpcontract.CoverageTarget {
	return mcpcontract.CoverageTarget{
		Type:       mcpcontract.CoverageTargetExactThread,
		Repository: mcpcontract.RepositoryRef{Owner: t.repo.Owner(), Repo: t.repo.Repo()},
		Thread:     &mcpcontract.ExactCoverageThread{Kind: string(t.kind), Number: t.number},
	}
}
func (t threadCoverageTarget) key() string {
	return fmt.Sprintf("%s/%s#%d", t.repo, t.kind, t.number)
}
func (t threadCoverageTarget) expectedFacets() []string { return facets.DefaultFor(string(t.kind)) }

func parseCoverageTarget(input mcpcontract.CoverageTarget) (parsedCoverageTarget, mcpcontract.CoverageTarget, error) {
	repo, err := domain.NewRepoRef(input.Repository.Owner, input.Repository.Repo)
	if err != nil {
		return nil, mcpcontract.CoverageTarget{}, fmt.Errorf("%w: %w", errInvalidCoverageTarget, err)
	}
	switch input.Type {
	case mcpcontract.CoverageTargetRepository:
		if input.Thread != nil {
			return nil, mcpcontract.CoverageTarget{}, errInvalidCoverageTarget
		}
		parsed := repositoryCoverageTarget{repo: repo}
		return parsed, parsed.wire(), nil
	case mcpcontract.CoverageTargetExactThread:
		if input.Thread == nil || input.Thread.Number <= 0 {
			return nil, mcpcontract.CoverageTarget{}, errInvalidCoverageTarget
		}
		var kind domain.ThreadKind
		switch input.Thread.Kind {
		case string(domain.IssueKind):
			kind = domain.IssueKind
		case string(domain.PullRequestKind):
			kind = domain.PullRequestKind
		default:
			return nil, mcpcontract.CoverageTarget{}, errInvalidCoverageTarget
		}
		parsed := threadCoverageTarget{repo: repo, kind: kind, number: input.Thread.Number}
		return parsed, parsed.wire(), nil
	default:
		return nil, mcpcontract.CoverageTarget{}, errInvalidCoverageTarget
	}
}
