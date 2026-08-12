package app

import (
	"context"
	"errors"
	"strings"

	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

const forkFreshnessRequestCost = 5

type forkComparisonContext struct {
	ref    mcpcontract.RepositoryRef
	branch string
	sha    string
}

// compareExplicitForkFreshness verifies one caller-selected fork. It performs
// only provider reads and never fetches local refs or updates the fork.
func compareExplicitForkFreshness(
	ctx context.Context,
	reader github.Reader,
	upstream mcpcontract.RepositoryRef,
	forkContext forkComparisonContext,
	availableRequests int,
) (mcpcontract.ForkFreshnessOutput, error) {
	result := newForkFreshnessOutput(upstream, "")
	result.Fork = forkContext.ref
	result.ContributionBranch = forkContext.branch
	result.ContributionSHA = forkContext.sha
	if availableRequests < forkFreshnessRequestCost {
		result.Reason = "request budget cannot fund complete fork freshness coverage"
		return result, nil
	}
	branchReader, hasBranchReader := reader.(github.BranchReader)
	comparisonReader, hasComparisonReader := reader.(github.CommitComparisonReader)
	if !hasBranchReader {
		result.Reason = "configured GitHub reader does not support branch-tip reads"
		return result, nil
	}
	if !hasComparisonReader {
		result.Reason = "configured GitHub reader does not support fork ancestry comparison"
		return result, nil
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
		return result, nil
	}
	if strings.TrimSpace(upstreamRepo.DefaultBranch) == "" || strings.TrimSpace(forkRepo.DefaultBranch) == "" {
		result.Reason = "upstream or fork default branch is unavailable"
		return result, nil
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
		return result, nil
	}

	comparison, _, err := comparisonReader.CompareCommits(ctx, upstream.Owner, upstream.Repo, upstreamRepo.DefaultBranch, forkContext.ref.Owner+":"+forkRepo.DefaultBranch)
	if err != nil {
		return forkFreshnessUnavailable(result, "upstream and fork default branches could not be compared", err)
	}
	if comparison.BaseSHA != "" && !strings.EqualFold(comparison.BaseSHA, result.UpstreamSHA) {
		result.Reason = "comparison base SHA did not match the resolved upstream default branch"
		return result, nil
	}
	if comparison.MergeBaseSHA == "" {
		result.Reason = "comparison did not provide merge-base evidence"
		return result, nil
	}
	result.Status, result.NextAction = classifyForkFreshness(comparison.Status)
	if result.Status == "unavailable" {
		result.Reason = "GitHub returned an unsupported fork comparison status"
		return result, nil
	}
	result.MergeBaseSHA = comparison.MergeBaseSHA
	result.AheadBy = comparison.AheadBy
	result.BehindBy = comparison.BehindBy
	result.Coverage = "verified"
	result.EffectiveDiffRisk = result.Status != "current"
	return result, nil
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

func forkFreshnessUnavailable(result mcpcontract.ForkFreshnessOutput, reason string, err error) (mcpcontract.ForkFreshnessOutput, error) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return mcpcontract.ForkFreshnessOutput{}, err
	}
	result.Reason = reason
	return result, nil
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
