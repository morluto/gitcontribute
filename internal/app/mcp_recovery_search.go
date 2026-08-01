package app

import "github.com/morluto/gitcontribute/internal/mcpcontract"

func localThreadSearchRecovery(in mcpcontract.SearchInput) *mcpcontract.RecoveryPlan {
	if in.Owner != "" && in.Repo != "" {
		return recoveryPlan(
			"coverage_unknown",
			"Local thread search does not establish repository-wide coverage. Refresh this repository's stored thread population, poll the job, then repeat the search before inferring absence.",
			mcpcontract.RecoveryAction(mcpcontract.EnsureCoverageInput{Target: mcpcontract.CoverageTarget{
				Type:       mcpcontract.CoverageTargetRepository,
				Repository: mcpcontract.RepositoryRef{Owner: in.Owner, Repo: in.Repo},
			}}),
		)
	}
	return recoveryPlan(
		"coverage_unknown",
		"Local thread search is limited to the stored corpus and has no global completeness proof. Use live repository search to select exact repositories, then synchronize their thread headers before treating an empty result as absence.",
		mcpcontract.RecoveryAction(mcpcontract.SearchGitHubRepositoriesInput{Text: in.Query, Limit: searchRecoveryLimit(in.Limit)}),
	)
}

func localRepositorySearchRecovery(in mcpcontract.SearchRepositoriesInput) *mcpcontract.RecoveryPlan {
	if in.Owner != "" && in.Repo != "" {
		return recoveryPlan(
			"coverage_unknown",
			"The local repository projection is not an exhaustive source. Refresh this exact repository context, then repeat the search.",
			syncRepositoryContextCall(in.Owner, in.Repo),
		)
	}
	return recoveryPlan(
		"coverage_unknown",
		"Local repository search is limited to the stored corpus. Use the live repository search to verify remote candidates before inferring absence.",
		mcpcontract.RecoveryAction(mcpcontract.SearchGitHubRepositoriesInput{Text: in.Query, Limit: searchRecoveryLimit(in.Limit)}),
	)
}

func searchRecoveryLimit(limit int) int {
	if limit < 1 || limit > 100 {
		return 20
	}
	return limit
}
