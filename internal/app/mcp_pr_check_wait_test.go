package app

import (
	"testing"

	"github.com/morluto/gitcontribute/internal/github"
)

func TestPullRequestChecksTerminalRequiresEveryCheck(t *testing.T) {
	checks := []github.PullRequestCheck{
		{Name: "build", Status: "COMPLETED", Conclusion: "SUCCESS"},
		{Name: "test", Status: "IN_PROGRESS"},
	}
	if terminal, failed := pullRequestChecksTerminal(checks, false); terminal || failed {
		t.Fatalf("terminal = %v, failed = %v; pending check must keep the watch open", terminal, failed)
	}
}

func TestPullRequestChecksFailFastReturnsFailure(t *testing.T) {
	checks := []github.PullRequestCheck{
		{Name: "long-test", Status: "IN_PROGRESS"},
		{Name: "lint", Status: "COMPLETED", Conclusion: "FAILURE"},
	}
	terminal, failed := pullRequestChecksTerminal(checks, true)
	if !terminal || !failed {
		t.Fatalf("terminal = %v, failed = %v; fail-fast should finish on a failed check", terminal, failed)
	}
	if pullRequestChecksAllTerminal(checks) {
		t.Fatal("fail-fast input with pending check reported all terminal")
	}
}

func TestPullRequestChecksExpectedAndCancelledAreNotPassing(t *testing.T) {
	for _, check := range []github.PullRequestCheck{
		{Name: "queued", Status: "EXPECTED"},
		{Name: "cancelled", Status: "CANCELLED"},
	} {
		terminal, failed := pullRequestChecksTerminal([]github.PullRequestCheck{check}, false)
		if check.Status == "EXPECTED" {
			if terminal || failed {
				t.Fatalf("EXPECTED check was treated as terminal/failing: terminal=%v failed=%v", terminal, failed)
			}
		} else if !terminal || !failed {
			t.Fatalf("cancelled check classified as terminal=%v failed=%v", terminal, failed)
		}
	}
}

func TestPullRequestChecksEmptyRollupIsNotTerminal(t *testing.T) {
	if terminal, failed := pullRequestChecksTerminal(nil, false); terminal || failed {
		t.Fatalf("terminal = %v, failed = %v; an empty rollup must wait for late registration", terminal, failed)
	}
}
