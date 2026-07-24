package cli_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/health"
)

func TestIndex(t *testing.T) {
	t.Parallel()
	svc := &fakeService{indexResult: &cli.IndexResult{Repo: cli.RepoRef{Owner: "o", Repo: "r"}, Commit: "abc", Files: 2}}
	c, stdout, stderr := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"index", "o/r", "/checkout"}))
	if !svc.indexCalled || svc.lastIndexRepo.String() != "o/r" || svc.lastIndexPath != "/checkout" {
		t.Fatalf("index call = called:%v repo:%v path:%q", svc.indexCalled, svc.lastIndexRepo, svc.lastIndexPath)
	}
	if !strings.Contains(stdout.String(), "abc") || !strings.Contains(stderr.String(), "indexing") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestAcquire(t *testing.T) {
	t.Parallel()
	svc := &fakeService{acquisitionResult: &cli.AcquisitionResult{
		Repo: cli.RepoRef{Owner: "o", Repo: "r"}, CommitSHA: "abc", Indexed: true,
	}}
	c, stdout, stderr := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"acquire", "o/r", "--remote", "https://example.test/o/r.git", "--json"}))
	if !svc.acquireCalled || svc.lastIndexRepo.String() != "o/r" || svc.lastAcquireRemote != "https://example.test/o/r.git" {
		t.Fatalf("acquire args: called=%v repo=%v remote=%q", svc.acquireCalled, svc.lastIndexRepo, svc.lastAcquireRemote)
	}
	if !strings.Contains(stdout.String(), `"commit_sha": "abc"`) || !strings.Contains(stderr.String(), "acquiring and indexing o/r") {
		t.Fatalf("stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestHealth(t *testing.T) {
	t.Parallel()
	svc := &fakeService{healthResult: &health.Report{Repo: health.RepoRef{Owner: "o", Repo: "r"}}}
	c, stdout, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"health", "o/r", "--start", "2026-07-01T00:00:00Z", "--end", "2026-07-17T00:00:00Z", "--stale-after", "240h", "--json"}))
	if !svc.healthCalled || svc.lastIndexRepo.String() != "o/r" || svc.lastHealthOpts.StaleThreshold != 240*time.Hour || svc.lastHealthOpts.Start.IsZero() {
		t.Fatalf("health args: called=%v repo=%v opts=%+v", svc.healthCalled, svc.lastIndexRepo, svc.lastHealthOpts)
	}
	if !strings.Contains(stdout.String(), `"repo": {`) {
		t.Fatalf("stdout=%q", stdout)
	}
}

func TestSourceAddSearchAndList(t *testing.T) {
	t.Parallel()
	svc := &fakeService{
		sourceResult:     &cli.SourceResult{Name: "active-go", Kind: "search", Definition: `{"query":"language:go"}`, Enabled: true},
		sourceListResult: &cli.SourceListResult{Sources: []cli.SourceResult{{Name: "active-go", Kind: "search", Enabled: true}}},
	}
	c, stdout, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"source", "add", "search", "--name", "active-go", "--query", "language:go"}))
	if !svc.addSourceCalled || svc.lastSourceName != "active-go" || svc.lastSourceQuery != "language:go" {
		t.Fatalf("add source call = called:%v name:%q query:%q", svc.addSourceCalled, svc.lastSourceName, svc.lastSourceQuery)
	}
	if !strings.Contains(stdout.String(), "Source active-go") {
		t.Fatalf("stdout = %q", stdout.String())
	}

	stdout.Reset()
	requireNoErr(t, c.Run(context.Background(), []string{"source", "list"}))
	if !svc.listSourcesCalled || !strings.Contains(stdout.String(), "active-go") {
		t.Fatalf("listed=%v stdout=%q", svc.listSourcesCalled, stdout.String())
	}
}

func TestSourceAddRepos(t *testing.T) {
	t.Parallel()
	svc := &fakeService{
		sourceResult: &cli.SourceResult{Name: "golang-go", Kind: "repos", Definition: `{"repositories":[{"owner":"golang","repo":"go"}]}`, Enabled: true},
	}
	c, stdout, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"source", "add", "repos", "golang/go"}))
	if !svc.addRepoSourceCalled || svc.lastSourceName != "golang-go" || len(svc.lastSourceRefs) != 1 || svc.lastSourceRefs[0].String() != "golang/go" {
		t.Fatalf("add repo source call = called:%v name:%q refs:%+v", svc.addRepoSourceCalled, svc.lastSourceName, svc.lastSourceRefs)
	}
	if !strings.Contains(stdout.String(), "Source golang-go") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestSourceAddReposAcceptsURL(t *testing.T) {
	t.Parallel()
	svc := &fakeService{sourceResult: &cli.SourceResult{Name: "x-y", Kind: "repos"}}
	c, _, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"source", "add", "repos", "https://github.com/X/Y"}))
	if len(svc.lastSourceRefs) != 1 || svc.lastSourceRefs[0].String() != "X/Y" {
		t.Fatalf("refs = %+v", svc.lastSourceRefs)
	}
}

func TestSourceAddReposImportsStructuredStdin(t *testing.T) {
	t.Parallel()
	svc := &fakeService{sourceResult: &cli.SourceResult{Name: "imported", Kind: "repos", Enabled: true}}
	c, _, _ := newTestCLI(svc, nil)
	c.SetInput(strings.NewReader(`{"repositories":["one/first",{"owner":"two","repo":"second"},{"full_name":"three/third"}]}`))
	requireNoErr(t, c.Run(context.Background(), []string{"source", "add", "repos", "--name", "imported", "--file", "-"}))
	if len(svc.lastSourceRefs) != 3 || svc.lastSourceRefs[1].String() != "two/second" {
		t.Fatalf("refs = %+v", svc.lastSourceRefs)
	}
}

func TestSourceAddReposImportsLineFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "repositories.txt")
	if err := os.WriteFile(path, []byte("# favorites\none/first\nhttps://github.com/two/second\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := &fakeService{sourceResult: &cli.SourceResult{Name: "imported", Kind: "repos", Enabled: true}}
	c, _, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"source", "add", "repos", "--name", "imported", "--file", path}))
	if len(svc.lastSourceRefs) != 2 || svc.lastSourceRefs[1].String() != "two/second" {
		t.Fatalf("refs = %+v", svc.lastSourceRefs)
	}
}

func TestSourceAddReposRejectsInvalidURL(t *testing.T) {
	t.Parallel()
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)
	err := c.Run(context.Background(), []string{"source", "add", "repos", "https://gitlab.com/X/Y"})
	requireCLIError(t, err, cli.ExitUsage)
	if svc.addRepoSourceCalled {
		t.Fatal("add repo source should not be called for invalid URL")
	}
}

func TestSourceAddGHArchive(t *testing.T) {
	t.Parallel()
	svc := &fakeService{
		sourceResult: &cli.SourceResult{Name: "gharchive", Kind: "gharchive", Definition: `{"events":["PushEvent","IssuesEvent"]}`, Enabled: true},
	}
	c, stdout, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"source", "add", "gharchive", "--events", "PushEvent,IssuesEvent"}))
	if !svc.addGHArchiveSourceCalled || svc.lastSourceName != "gharchive" || len(svc.lastSourceEvents) != 2 {
		t.Fatalf("add gharchive call = called:%v name:%q events:%+v", svc.addGHArchiveSourceCalled, svc.lastSourceName, svc.lastSourceEvents)
	}
	if !strings.Contains(stdout.String(), "Source gharchive") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestSourceAddGHArchiveRejectsUnknownEvent(t *testing.T) {
	t.Parallel()
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)
	err := c.Run(context.Background(), []string{"source", "add", "gharchive", "--events", "PushEvent,UnknownEvent"})
	requireCLIError(t, err, cli.ExitUsage)
	if svc.addGHArchiveSourceCalled {
		t.Fatal("add gharchive source should not be called for invalid event")
	}
}

func TestSourceShow(t *testing.T) {
	t.Parallel()
	svc := &fakeService{sourceResult: &cli.SourceResult{Name: "gharchive", Kind: "gharchive", Definition: `{}`, Enabled: true}}
	c, stdout, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"source", "show", "gharchive"}))
	if !svc.showSourceCalled || svc.lastShowSourceName != "gharchive" {
		t.Fatalf("show source not called correctly: called=%v arg=%q", svc.showSourceCalled, svc.lastShowSourceName)
	}
	if !strings.Contains(stdout.String(), "gharchive") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestCrawlDispatchesBoundedOptions(t *testing.T) {
	t.Parallel()
	svc := &fakeService{crawlResult: &cli.CrawlResult{
		Source: "active-go", Windows: 2, Repositories: 7, Requests: 4, Checkpoint: "2026-07-16T00:00:00Z",
	}}
	c, stdout, stderr := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"crawl", "active-go", "--since", "48h", "--budget", "25"}))
	if !svc.crawlCalled || svc.lastCrawlName != "active-go" || svc.lastCrawlOpts.Since != 48*time.Hour || svc.lastCrawlOpts.Budget != 25 {
		t.Fatalf("crawl call = called:%v name:%q opts:%+v", svc.crawlCalled, svc.lastCrawlName, svc.lastCrawlOpts)
	}
	if !strings.Contains(stdout.String(), "7 repositories") || !strings.Contains(stderr.String(), "crawling active-go") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestCrawlRejectsInvalidBudgetBeforeDispatch(t *testing.T) {
	t.Parallel()
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)
	err := c.Run(context.Background(), []string{"crawl", "active-go", "--budget", "5001"})
	requireCLIError(t, err, cli.ExitUsage)
	if svc.crawlCalled {
		t.Fatal("crawl should not be called with an invalid budget")
	}
}

func TestTailDispatchesBoundedOptions(t *testing.T) {
	t.Parallel()
	svc := &fakeService{}
	c, _, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"tail", "events", "--since", "3h", "--interval", "10m", "--budget", "25", "--once"}))
	if !svc.tailCalled || svc.lastCrawlName != "events" {
		t.Fatalf("tail not called: %+v", svc)
	}
	if svc.lastTailOpts.Since != 3*time.Hour || svc.lastTailOpts.Interval != 10*time.Minute || svc.lastTailOpts.Budget != 25 || !svc.lastTailOpts.Once {
		t.Fatalf("tail options = %+v", svc.lastTailOpts)
	}
}
