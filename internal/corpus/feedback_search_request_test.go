package corpus

import "testing"

func TestParseFeedbackSearchQueryRejectsContradictoryBoundaryStates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input FeedbackSearchInput
	}{
		{name: "reversed created interval", input: FeedbackSearchInput{CreatedAfter: "2026-08-02T00:00:00Z", CreatedBefore: "2026-08-01T00:00:00Z"}},
		{name: "reversed updated interval", input: FeedbackSearchInput{UpdatedAfter: "2026-08-02T00:00:00Z", UpdatedBefore: "2026-08-01T00:00:00Z"}},
		{name: "merged open pull request", input: FeedbackSearchInput{State: "open", Merged: "true"}},
		{name: "resolution on issue comments", input: FeedbackSearchInput{Channel: "issue_comments", ThreadState: "resolved"}},
		{name: "unknown state", input: FeedbackSearchInput{State: "draft"}},
		{name: "unknown merge mode", input: FeedbackSearchInput{Merged: "maybe"}},
		{name: "unknown channel", input: FeedbackSearchInput{Channel: "commits"}},
		{name: "unknown sort", input: FeedbackSearchInput{Sort: "random"}},
		{name: "unknown order", input: FeedbackSearchInput{Order: "sideways"}},
		{name: "oversized page", input: FeedbackSearchInput{Limit: MaximumSearchPageSize + 1}},
		{name: "invalid timestamp", input: FeedbackSearchInput{CreatedAfter: "yesterday"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseFeedbackSearchQuery(test.input); err == nil {
				t.Fatal("feedback search accepted contradictory input")
			}
		})
	}
}

func TestParseFeedbackSearchQueryProducesCanonicalExecutableRequest(t *testing.T) {
	t.Parallel()
	query, err := ParseFeedbackSearchQuery(FeedbackSearchInput{FeedbackAuthor: " alice ", PullRequestAuthor: " bob ", Text: " latency "})
	if err != nil {
		t.Fatal(err)
	}
	if query.feedbackAuthor != "alice" || query.pullRequestAuthor != "bob" || query.text != "latency" {
		t.Fatalf("canonical text fields = %+v", query)
	}
	if !query.state.IsAny() || !query.merge.IsAny() || !query.threadState.isAny() || query.channel != 0 || query.sort.String() != "updated" || query.order.String() != "desc" || query.Limit() != DefaultSearchPageSize {
		t.Fatalf("canonical modes = %+v", query)
	}
	request, err := query.InRepository(42)
	if err != nil {
		t.Fatal(err)
	}
	if request.repositoryID != 42 {
		t.Fatalf("repository id = %d, want 42", request.repositoryID)
	}
	if _, err := query.InRepository(0); err == nil {
		t.Fatal("feedback search accepted an unbound repository")
	}
}
