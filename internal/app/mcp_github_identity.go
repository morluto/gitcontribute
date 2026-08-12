package app

import (
	"context"
	"errors"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// GetAuthenticatedIdentity resolves the configured GitHub account without
// persisting an actor or starting broader discovery.
func (r *MCPReader) GetAuthenticatedIdentity(ctx context.Context, _ mcpcontract.GetAuthenticatedIdentityInput) (mcpcontract.AuthenticatedIdentityOutput, error) {
	reader, err := r.githubReader() //nolint:contextcheck // construction performs no request
	if err != nil {
		return mcpcontract.AuthenticatedIdentityOutput{}, err
	}
	identityReader, ok := reader.(github.IdentityReader)
	if !ok {
		return mcpcontract.AuthenticatedIdentityOutput{}, errors.New("GitHub reader does not support authenticated identity lookup")
	}
	identity, rate, err := identityReader.GetAuthenticatedIdentity(ctx)
	if err != nil {
		return mcpcontract.AuthenticatedIdentityOutput{}, err
	}
	if identity.Login == "" {
		return mcpcontract.AuthenticatedIdentityOutput{}, errors.New("authenticated GitHub identity has no login")
	}
	return mcpcontract.AuthenticatedIdentityOutput{
		Login: identity.Login, DatabaseID: identity.ID, NodeID: identity.NodeID,
		ObservedAt: formatTime(r.now()), Rate: githubRateOutput(rate),
	}, nil
}

// CompareFork verifies one explicit fork relationship and default-branch
// ancestry. It performs only bounded provider reads and never updates the fork.
func (r *MCPReader) CompareFork(ctx context.Context, in mcpcontract.CompareForkInput) (mcpcontract.ForkFreshnessOutput, error) {
	upstream, err := domain.NewRepoRef(in.Upstream.Owner, in.Upstream.Repo)
	if err != nil {
		return mcpcontract.ForkFreshnessOutput{}, err
	}
	fork, err := domain.NewRepoRef(in.Fork.Owner, in.Fork.Repo)
	if err != nil {
		return mcpcontract.ForkFreshnessOutput{}, err
	}
	in.Upstream = mcpcontract.RepositoryRef{Owner: upstream.Owner(), Repo: upstream.Repo()}
	in.Fork = mcpcontract.RepositoryRef{Owner: fork.Owner(), Repo: fork.Repo()}
	if sameGitHubRepository(in.Fork.Owner, in.Fork.Repo, in.Upstream) {
		return mcpcontract.ForkFreshnessOutput{}, errors.New("fork repository must differ from upstream")
	}
	reader, err := r.githubReader() //nolint:contextcheck // construction performs no request
	if err != nil {
		return mcpcontract.ForkFreshnessOutput{}, err
	}
	return compareExplicitForkFreshness(ctx, reader, in.Upstream, forkComparisonContext{
		ref: in.Fork, branch: in.ContributionBranch, sha: in.ContributionSHA,
	}, forkFreshnessRequestCost)
}
