package facets

import (
	"reflect"
	"testing"
)

func TestSelectionPolicy(t *testing.T) {
	if got, want := DefaultFor(IssueKind), []string{IssueComments}; !reflect.DeepEqual(got, want) {
		t.Fatalf("issue defaults = %v, want %v", got, want)
	}
	if got, want := DefaultFor(PullRequestKind), []string{IssueComments, PRDetails, PRReviews, PRReviewComments}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pull-request defaults = %v, want %v", got, want)
	}
	if got, want := SelectableFor(IssueKind), []string{IssueComments, IssueTimeline}; !reflect.DeepEqual(got, want) {
		t.Fatalf("issue selectable = %v, want %v", got, want)
	}
	if got, want := SelectableNames(), []string{IssueComments, IssueTimeline, PRDetails, PRReviews, PRReviewComments}; !reflect.DeepEqual(got, want) {
		t.Fatalf("schema names = %v, want %v", got, want)
	}
}

func TestSelectionPolicyReturnsIndependentSlices(t *testing.T) {
	first := DefaultFor(PullRequestKind)
	first[0] = "changed"
	second := DefaultFor(PullRequestKind)
	if second[0] != IssueComments {
		t.Fatalf("default policy was mutated through returned slice: %v", second)
	}
}
