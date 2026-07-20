package cli_test

import (
	"context"
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/cli"
)

func TestSync(t *testing.T) {
	svc := &fakeService{
		syncResult:     &cli.SyncResult{Repo: cli.RepoRef{Owner: "o", Repo: "r"}, Updated: 7, Requests: 10, PlannedRequests: 100, RequestBudget: 100, Message: "ok"},
		syncPlanResult: &cli.SyncPlanResult{Repo: cli.RepoRef{Owner: "o", Repo: "r"}, FixedRequests: 9, ThreadRequestCeiling: 91, PlannedRequests: 100, RequestBudget: 100},
	}
	c, stdout, stderr := newTestCLI(svc, nil)

	err := c.Run(context.Background(), []string{"sync", "o/r"})
	requireNoErr(t, err)

	if !svc.syncCalled || !svc.syncPlanCalled {
		t.Fatal("Sync was not planned and called")
	}
	if svc.lastSyncArg != (cli.RepoRef{Owner: "o", Repo: "r"}) {
		t.Fatalf("sync repo=%+v, want o/r", svc.lastSyncArg)
	}
	want := "Synced o/r: 7 updated. Requests: 10 actual, up to 100 planned (budget 100).\nok\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout=%q, want %q", got, want)
	}
	if got := stderr.String(); !strings.HasPrefix(got, "planned sync for o/r: up to 100 requests (9 fixed + up to 91 thread requests; budget 100)\nsyncing o/r...") {
		t.Fatalf("stderr=%q", got)
	}
}
