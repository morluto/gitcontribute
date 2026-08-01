package app

import (
	"context"
	"strings"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// SearchRepositories performs a local-only repository search.
func (r *MCPReader) SearchRepositories(ctx context.Context, in mcpcontract.SearchRepositoriesInput) (mcpcontract.SearchRepositoriesOutput, error) {
	repoRef := domain.RepoRef{Owner: in.Owner, Repo: in.Repo}
	repoFilter := ""
	if in.Owner != "" || in.Repo != "" {
		if err := repoRef.Validate(); err != nil {
			return mcpcontract.SearchRepositoriesOutput{}, err
		}
		repoFilter = repoRef.String()
	}

	res, err := r.searchCorpus(ctx, in.Query, contracts.SearchOptions{
		Kind:          "repos",
		Repo:          repoFilter,
		Limit:         in.Limit,
		Cursor:        in.Cursor,
		Sort:          in.Sort,
		SnapshotToken: in.SnapshotToken,
	})
	if err != nil {
		return mcpcontract.SearchRepositoriesOutput{}, err
	}
	if len(res.Matches) == 0 {
		provenance, err := offlineReadProvenance("repository_search", res.ObservationWatermark, in, res.NextCursor == "", res.NextCursor != "", true)
		if err != nil {
			return mcpcontract.SearchRepositoriesOutput{}, err
		}
		recovery := localRepositorySearchRecovery(in)
		provenance.Recovery = recovery
		return mcpcontract.SearchRepositoriesOutput{Query: in.Query, Total: res.Total, Matches: []mcpcontract.RepositoryOutput{}, Incomplete: true, NextCursor: res.NextCursor, SnapshotToken: res.SnapshotToken, Recovery: recovery, Provenance: provenance}, nil
	}

	refs := make([]mcpcontract.RepositoryRef, len(res.Matches))
	for i, m := range res.Matches {
		refs[i] = mcpcontract.RepositoryRef{Owner: m.Repo.Owner, Repo: m.Repo.Repo}
	}
	batch, err := r.GetRepositories(ctx, mcpcontract.GetRepositoriesInput{Repositories: refs, SnapshotToken: in.SnapshotToken})
	if err != nil {
		return mcpcontract.SearchRepositoriesOutput{}, err
	}
	matches := make([]mcpcontract.RepositoryOutput, 0, len(batch.Items))
	missing := make([]mcpcontract.RepositoryRef, 0)
	for _, item := range batch.Items {
		if item.Value != nil {
			matches = append(matches, *item.Value)
		} else {
			owner, repo, ok := strings.Cut(item.Key, "/")
			if ok {
				missing = append(missing, mcpcontract.RepositoryRef{Owner: owner, Repo: repo})
			}
		}
	}
	provenance, err := offlineReadProvenance("repository_search", res.ObservationWatermark, in, res.NextCursor == "", res.NextCursor != "", true)
	if err != nil {
		return mcpcontract.SearchRepositoriesOutput{}, err
	}
	incomplete := len(missing) > 0 || batch.Status != "complete" || provenance.UnknownCoverage
	var recovery *mcpcontract.RecoveryPlan
	if len(missing) > 0 {
		calls := make([]mcpcontract.ToolCall, 0, len(missing))
		for _, ref := range missing {
			calls = append(calls, syncRepositoryContextCall(ref.Owner, ref.Repo))
		}
		recovery = recoveryPlan("repository_projection_unavailable", "Some repository search matches have no readable local projection. Refresh those exact repositories, then repeat the search.", calls...)
	} else if incomplete {
		calls := make([]mcpcontract.ToolCall, 0, len(matches))
		seen := make(map[string]struct{}, len(matches))
		for _, match := range matches {
			key := match.Owner + "/" + match.Repo
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			calls = append(calls, syncRepositoryContextCall(match.Owner, match.Repo))
		}
		if len(calls) > 0 {
			recovery = recoveryPlan("repository_projection_incomplete", "Some repository search projections are incomplete. Refresh the exact returned repositories, then repeat the search.", calls...)
		} else {
			recovery = localRepositorySearchRecovery(in)
		}
	}
	provenance.Recovery = recovery
	return mcpcontract.SearchRepositoriesOutput{Query: in.Query, Total: res.Total, Matches: matches, Incomplete: incomplete, Missing: missing, NextCursor: res.NextCursor, SnapshotToken: res.SnapshotToken, Recovery: recovery, Provenance: provenance}, nil
}
