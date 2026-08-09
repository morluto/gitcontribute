package app

import (
	"errors"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
)

func parseGitHubMergeStatus(details github.PullRequestDetails) (domain.MergeStatus, error) {
	if !details.Merged {
		if details.MergedAt != nil {
			return domain.MergeStatus{}, errors.New("GitHub pull request is unmerged but has a merge time")
		}
		return domain.UnmergedStatus(), nil
	}
	mergedAt := time.Time{}
	if details.MergedAt != nil {
		mergedAt = *details.MergedAt
	}
	return domain.MergedStatus(mergedAt), nil
}
