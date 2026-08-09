package app

import (
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/github"
)

func TestParseGitHubMergeStatusRejectsContradiction(t *testing.T) {
	t.Parallel()
	at := time.Unix(1, 0).UTC()
	if _, err := parseGitHubMergeStatus(github.PullRequestDetails{MergedAt: &at}); err == nil {
		t.Fatal("unmerged pull request with merge time parsed")
	}
}
