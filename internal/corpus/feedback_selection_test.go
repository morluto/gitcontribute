package corpus

import (
	"slices"
	"testing"
)

func TestFeedbackSelectionCanonicalizesAndOwnsChannelSet(t *testing.T) {
	t.Parallel()
	selection, err := ParseFeedbackSelection([]string{"review_threads", "issue_comments"}, "unresolved")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"issue_comments", "review_threads"}
	if got := selection.Channels(); !slices.Equal(got, want) {
		t.Fatalf("channels = %v, want %v", got, want)
	}
	if selection.ThreadState() != "unresolved" || !selection.Includes(FeedbackIssueComments) || selection.Includes(FeedbackInlineComments) {
		t.Fatalf("selection = %+v", selection)
	}
	channels := selection.Channels()
	channels[0] = "mutated"
	if got := selection.Channels(); !slices.Equal(got, want) {
		t.Fatalf("returned channels mutated selection: %v", got)
	}
	reordered, err := ParseFeedbackSelection([]string{"issue_comments", "review_threads"}, "unresolved")
	if err != nil || !selection.Equal(reordered) {
		t.Fatalf("reordered selection = %+v, err=%v", reordered, err)
	}
}

func TestFeedbackSelectionRejectsInvalidCoverageStates(t *testing.T) {
	t.Parallel()
	for _, input := range []struct {
		channels []string
		state    string
	}{
		{state: "all"},
		{channels: []string{"issue_comments", "issue_comments"}, state: "all"},
		{channels: []string{"commits"}, state: "all"},
		{channels: []string{"issue_comments"}, state: "resolved"},
	} {
		if _, err := ParseFeedbackSelection(input.channels, input.state); err == nil {
			t.Fatalf("accepted invalid selection: %+v", input)
		}
	}
}
