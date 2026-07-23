package github

import (
	"testing"

	gh "github.com/google/go-github/v89/github"
)

func TestConversionsHandleMissingNestedModels(t *testing.T) {
	if got := convertRepository(&gh.Repository{}); got.Owner != "" || got.License != "" {
		t.Fatalf("repository = %+v, want empty nested fields", got)
	}
	if got := convertIssue(&gh.Issue{}); got.Author != "" || got.Milestone != "" {
		t.Fatalf("issue = %+v, want empty nested fields", got)
	}
	if got := convertIssueComment(&gh.IssueComment{}); got.Author != "" {
		t.Fatalf("issue comment = %+v, want empty author", got)
	}
	if got := convertPullRequestDetails(&gh.PullRequest{}); got.Author != "" || got.Milestone != "" {
		t.Fatalf("pull request = %+v, want empty nested fields", got)
	}
	if got := convertReview(&gh.PullRequestReview{}); got.Author != "" {
		t.Fatalf("review = %+v, want empty author", got)
	}
	if got := convertReviewComment(&gh.PullRequestComment{}); got.Author != "" {
		t.Fatalf("review comment = %+v, want empty author", got)
	}
}
