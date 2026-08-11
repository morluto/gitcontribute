package app

import (
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func TestParsePullRequestCheckWaitInputCanonicalizesBoundary(t *testing.T) {
	request, err := parsePullRequestCheckWaitInput(mcpcontract.WaitPullRequestChecksInput{
		Owner: " Acme ", Repo: " Rocket ", Number: 7,
		ExpectedHeadSHA: " ABCDEF0123456789ABCDEF0123456789ABCDEF01 ",
		FailFast:        true,
	})
	if err != nil {
		t.Fatalf("parse wait request: %v", err)
	}
	if request.timeout != 30*time.Minute || request.pollInterval != 10*time.Second {
		t.Fatalf("wait bounds = %s/%s, want 30m/10s", request.timeout, request.pollInterval)
	}
	canonical := request.canonical()
	if canonical.Owner != "Acme" || canonical.Repo != "Rocket" || canonical.MaxPages != 10 {
		t.Fatalf("canonical repository/page = %s/%s/%d", canonical.Owner, canonical.Repo, canonical.MaxPages)
	}
	if canonical.ExpectedHeadSHA != "abcdef0123456789abcdef0123456789abcdef01" {
		t.Fatalf("canonical head = %q", canonical.ExpectedHeadSHA)
	}
	if !canonical.FailFast || !request.expectedHead.matches("ABCDEF0123456789ABCDEF0123456789ABCDEF01") {
		t.Fatal("parsed fail-fast policy or exact-head identity was lost")
	}
}

func TestPullRequestChecksTerminalRequiresEveryCheck(t *testing.T) {
	checks := []github.PullRequestCheck{
		{Name: "build", Status: "COMPLETED", Conclusion: "SUCCESS"},
		{Name: "test", Status: "IN_PROGRESS"},
	}
	if terminal, failed := pullRequestChecksTerminal(checks, pullRequestCheckCompletionAll); terminal || failed {
		t.Fatalf("terminal = %v, failed = %v; pending check must keep the watch open", terminal, failed)
	}
}

func TestPullRequestChecksFailFastReturnsFailure(t *testing.T) {
	checks := []github.PullRequestCheck{
		{Name: "long-test", Status: "IN_PROGRESS"},
		{Name: "lint", Status: "COMPLETED", Conclusion: "FAILURE"},
	}
	terminal, failed := pullRequestChecksTerminal(checks, pullRequestCheckCompletionFailFast)
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
		terminal, failed := pullRequestChecksTerminal([]github.PullRequestCheck{check}, pullRequestCheckCompletionAll)
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
	if terminal, failed := pullRequestChecksTerminal(nil, pullRequestCheckCompletionAll); terminal || failed {
		t.Fatalf("terminal = %v, failed = %v; an empty rollup must wait for late registration", terminal, failed)
	}
}
