package github

import (
	"context"
	"errors"

	gh "github.com/google/go-github/v89/github"
)

// GetBranch reads one branch tip without requesting protection details or
// mutating local refs.
func (c *Client) GetBranch(ctx context.Context, owner, repo, branch string) (Branch, RateInfo, error) {
	result, resp, err := c.gh.Repositories.GetBranch(ctx, owner, repo, branch, 0)
	if err != nil {
		return Branch{}, RateInfo{}, classifyError(err)
	}
	if result == nil {
		return Branch{}, responseRateInfo(resp), errors.New("github returned an empty branch")
	}
	var commitSHA string
	if commit := result.GetCommit(); commit != nil {
		commitSHA = commit.GetSHA()
	}
	return Branch{Name: result.GetName(), CommitSHA: commitSHA}, rateInfo(resp.Rate), nil
}

// CompareCommits reads one bounded comparison. The provider accepts an
// owner-qualified head such as "fork-owner:main" when the repositories share
// a fork network, so callers can compare an upstream default branch with a
// contributor fork without fetching or writing local refs.
func (c *Client) CompareCommits(ctx context.Context, owner, repo, base, head string) (CommitComparison, RateInfo, error) {
	result, resp, err := c.gh.Repositories.CompareCommits(ctx, owner, repo, base, head, &gh.ListOptions{Page: 1, PerPage: 1})
	if err != nil {
		return CommitComparison{}, RateInfo{}, classifyError(err)
	}
	if result == nil {
		return CommitComparison{}, responseRateInfo(resp), errors.New("github returned an empty commit comparison")
	}
	comparison := CommitComparison{Status: result.GetStatus(), AheadBy: result.GetAheadBy(), BehindBy: result.GetBehindBy()}
	if base := result.GetBaseCommit(); base != nil {
		comparison.BaseSHA = base.GetSHA()
	}
	if mergeBase := result.GetMergeBaseCommit(); mergeBase != nil {
		comparison.MergeBaseSHA = mergeBase.GetSHA()
	}
	return comparison, rateInfo(resp.Rate), nil
}
