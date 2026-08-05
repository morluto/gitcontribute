package app

import (
	"context"
	"strings"

	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/gitremote"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
	"github.com/morluto/gitcontribute/internal/workspace"
)

const forkFreshnessRequestCost = 5

type preflightForkContext struct {
	ref    mcpcontract.RepositoryRef
	branch string
	sha    string
}

// checkPreflightForkFreshness performs only provider reads. It never fetches
// local refs or updates the fork; the compare API supplies the merge-base
// evidence across the upstream repository network.
func checkPreflightForkFreshness(
	ctx context.Context,
	reader github.Reader,
	upstream mcpcontract.RepositoryRef,
	fork *mcpcontract.RepositoryRef,
	identity string,
	candidate mcpcontract.ContributionPreflightCandidate,
	worktrees []workspace.LocalWorktree,
	existing *preflightExisting,
	maxRequests int,
	requests int,
) (mcpcontract.ForkFreshnessOutput, bool, error) {
	forkContext, shouldCheck, reason := resolvePreflightFork(upstream, fork, identity, candidate, worktrees, existing)
	if !shouldCheck {
		return mcpcontract.ForkFreshnessOutput{}, false, nil
	}
	result := newForkFreshnessOutput(upstream, reason)
	if forkContext != nil {
		result.Fork = forkContext.ref
		result.ContributionBranch = forkContext.branch
		result.ContributionSHA = forkContext.sha
	}
	if reason != "" {
		return result, true, nil
	}
	if forkContext == nil {
		result.Reason = "contributor fork could not be identified from the supplied context"
		return result, true, nil
	}
	if maxRequests-requests < forkFreshnessRequestCost {
		result.Reason = "request budget cannot fund complete fork freshness coverage"
		return result, true, nil
	}
	branchReader, hasBranchReader := reader.(github.BranchReader)
	comparisonReader, hasComparisonReader := reader.(github.CommitComparisonReader)
	if !hasBranchReader {
		result.Reason = "configured GitHub reader does not support branch-tip reads"
		return result, true, nil
	}
	if !hasComparisonReader {
		result.Reason = "configured GitHub reader does not support fork ancestry comparison"
		return result, true, nil
	}

	upstreamRepo, _, err := reader.GetRepository(ctx, upstream.Owner, upstream.Repo)
	if err != nil {
		return forkFreshnessUnavailable(result, "upstream repository metadata could not be read", err)
	}
	forkRepo, _, err := reader.GetRepository(ctx, forkContext.ref.Owner, forkContext.ref.Repo)
	if err != nil {
		return forkFreshnessUnavailable(result, "fork repository metadata could not be read", err)
	}
	if !forkRepo.Fork || forkRepo.Parent == nil || !sameGitHubRepository(forkRepo.Parent.Owner, forkRepo.Parent.Name, upstream) {
		result.Reason = "selected repository is not a fork of the requested upstream repository"
		return result, true, nil
	}
	if strings.TrimSpace(upstreamRepo.DefaultBranch) == "" || strings.TrimSpace(forkRepo.DefaultBranch) == "" {
		result.Reason = "upstream or fork default branch is unavailable"
		return result, true, nil
	}
	result.UpstreamBranch = upstreamRepo.DefaultBranch
	result.ForkBranch = forkRepo.DefaultBranch

	upstreamBranch, _, err := branchReader.GetBranch(ctx, upstream.Owner, upstream.Repo, upstreamRepo.DefaultBranch)
	if err != nil {
		return forkFreshnessUnavailable(result, "upstream default branch could not be read", err)
	}
	forkBranch, _, err := branchReader.GetBranch(ctx, forkContext.ref.Owner, forkContext.ref.Repo, forkRepo.DefaultBranch)
	if err != nil {
		return forkFreshnessUnavailable(result, "fork default branch could not be read", err)
	}
	result.UpstreamSHA = upstreamBranch.CommitSHA
	result.ForkSHA = forkBranch.CommitSHA
	if result.UpstreamSHA == "" || result.ForkSHA == "" {
		result.Reason = "upstream or fork default branch did not include a commit SHA"
		return result, true, nil
	}

	comparison, _, err := comparisonReader.CompareCommits(ctx, upstream.Owner, upstream.Repo, upstreamRepo.DefaultBranch, forkContext.ref.Owner+":"+forkRepo.DefaultBranch)
	if err != nil {
		return forkFreshnessUnavailable(result, "upstream and fork default branches could not be compared", err)
	}
	if comparison.BaseSHA != "" && !strings.EqualFold(comparison.BaseSHA, result.UpstreamSHA) {
		result.Reason = "comparison base SHA did not match the resolved upstream default branch"
		return result, true, nil
	}
	if comparison.MergeBaseSHA == "" {
		result.Reason = "comparison did not provide merge-base evidence"
		return result, true, nil
	}
	result.Status, result.NextAction = classifyForkFreshness(comparison.Status)
	if result.Status == "unavailable" {
		result.Reason = "GitHub returned an unsupported fork comparison status"
		return result, true, nil
	}
	result.MergeBaseSHA = comparison.MergeBaseSHA
	result.AheadBy = comparison.AheadBy
	result.BehindBy = comparison.BehindBy
	result.Coverage = "verified"
	result.EffectiveDiffRisk = result.Status != "current"
	return result, true, nil
}

func resolvePreflightFork(
	upstream mcpcontract.RepositoryRef,
	explicit *mcpcontract.RepositoryRef,
	identity string,
	candidate mcpcontract.ContributionPreflightCandidate,
	worktrees []workspace.LocalWorktree,
	existing *preflightExisting,
) (*preflightForkContext, bool, string) {
	if explicit != nil {
		return &preflightForkContext{ref: *explicit, branch: strings.TrimSpace(candidate.HeadRef), sha: strings.TrimSpace(candidate.HeadSHA)}, true, ""
	}
	if existing != nil && existing.details.HeadOwner != "" && existing.details.HeadRepo != "" && !sameGitHubRepository(existing.details.HeadOwner, existing.details.HeadRepo, upstream) {
		return &preflightForkContext{
			ref:    mcpcontract.RepositoryRef{Owner: existing.details.HeadOwner, Repo: existing.details.HeadRepo},
			branch: existing.details.HeadRef,
			sha:    existing.details.HeadSHA,
		}, true, ""
	}
	if len(worktrees) == 0 {
		return nil, false, ""
	}

	var candidates []preflightForkContext
	seen := make(map[string]struct{})
	for _, worktree := range worktrees {
		for _, urls := range worktree.Remotes {
			for _, remote := range urls {
				identityRef, err := gitremote.ParseRepositoryIdentity(remote)
				if err != nil || !strings.EqualFold(identityRef.Owner, identity) || sameGitHubRepository(identityRef.Owner, identityRef.Repo, upstream) {
					continue
				}
				ref := mcpcontract.RepositoryRef{Owner: identityRef.Owner, Repo: identityRef.Repo}
				key := strings.ToLower(ref.Owner + "/" + ref.Repo)
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				branch, sha := worktree.Branch, worktree.HeadSHA
				if candidate.HeadRef != "" {
					branch = candidate.HeadRef
				}
				if candidate.HeadSHA != "" {
					sha = candidate.HeadSHA
				}
				candidates = append(candidates, preflightForkContext{ref: ref, branch: branch, sha: sha})
			}
		}
	}
	if len(candidates) == 1 {
		return &candidates[0], true, ""
	}
	if len(candidates) > 1 {
		return nil, true, "multiple contributor fork remotes were found; provide fork explicitly"
	}
	return nil, true, "no contributor fork remote was found in the supplied workspaces"
}

func newForkFreshnessOutput(upstream mcpcontract.RepositoryRef, reason string) mcpcontract.ForkFreshnessOutput {
	return mcpcontract.ForkFreshnessOutput{
		Status:     "unavailable",
		Coverage:   "unavailable",
		Upstream:   upstream,
		Reason:     reason,
		NextAction: "provide_fork_context_or_retry_freshness_check",
	}
}

func forkFreshnessUnavailable(result mcpcontract.ForkFreshnessOutput, reason string, err error) (mcpcontract.ForkFreshnessOutput, bool, error) {
	if contextError(err) {
		return mcpcontract.ForkFreshnessOutput{}, false, err
	}
	result.Reason = reason
	return result, true, nil
}

func classifyForkFreshness(providerStatus string) (string, string) {
	switch providerStatus {
	case "identical":
		return "current", "publish_contribution"
	case "behind":
		return "behind", "sync_fork_or_fast_forward_only_update"
	case "ahead", "diverged":
		return "diverged", "inspect_fork_history_before_publishing"
	default:
		return "unavailable", "retry_freshness_check"
	}
}

func sameGitHubRepository(owner, repo string, ref mcpcontract.RepositoryRef) bool {
	return strings.EqualFold(strings.TrimSpace(owner), strings.TrimSpace(ref.Owner)) && strings.EqualFold(strings.TrimSpace(repo), strings.TrimSpace(ref.Repo))
}
