package facets

import (
	"reflect"
	"testing"

	"github.com/morluto/gitcontribute/internal/domain"
)

func TestSelectionPolicy(t *testing.T) {
	if got, want := DefaultFor(domain.IssueKind), []string{IssueComments}; !reflect.DeepEqual(got, want) {
		t.Fatalf("issue defaults = %v, want %v", got, want)
	}
	if got, want := DefaultFor(domain.PullRequestKind), []string{IssueComments, PRDetails, PRReviews, PRReviewComments}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pull-request defaults = %v, want %v", got, want)
	}
	if got, want := SelectableFor(domain.IssueKind), []string{IssueComments, IssueTimeline}; !reflect.DeepEqual(got, want) {
		t.Fatalf("issue selectable = %v, want %v", got, want)
	}
	if got, want := SelectableNames(), []string{IssueComments, IssueTimeline, PRDetails, PRReviews, PRReviewComments}; !reflect.DeepEqual(got, want) {
		t.Fatalf("schema names = %v, want %v", got, want)
	}
}

func TestParsedSelectionOwnsDeduplicationAndApplicability(t *testing.T) {
	t.Parallel()
	selection, err := ParseSelection([]string{" " + PRDetails + " ", PRDetails, PRReviews})
	if err != nil {
		t.Fatal(err)
	}
	names, err := selection.For(domain.PullRequestKind)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Fatalf("parsed selection = %v, want two names", names)
	}
	if got, want := []string{names[0].String(), names[1].String()}, []string{PRDetails, PRReviews}; !reflect.DeepEqual(got, want) {
		t.Fatalf("parsed selection = %v, want %v", got, want)
	}
	if _, err := selection.For(domain.IssueKind); err == nil {
		t.Fatal("pull-request-only facets were accepted for an issue")
	}
	if _, err := ParseSelection([]string{"unknown"}); err == nil {
		t.Fatal("unknown facet was accepted")
	}
}

func TestSelectionPolicyReturnsIndependentSlices(t *testing.T) {
	first := DefaultFor(domain.PullRequestKind)
	first[0] = "changed"
	second := DefaultFor(domain.PullRequestKind)
	if second[0] != IssueComments {
		t.Fatalf("default policy was mutated through returned slice: %v", second)
	}
}
