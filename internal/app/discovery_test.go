package app

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/discovery"
	"github.com/morluto/gitcontribute/internal/domain"
)

// fakeArchiveFetcher serves GH Archive hour files for tests.
type fakeArchiveFetcher struct {
	mu       sync.Mutex
	fetched  []string
	files    map[string][]byte
	failHour string
}

func (f *fakeArchiveFetcher) Fetch(ctx context.Context, hour time.Time) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := fmt.Sprintf("%04d-%02d-%02d-%d.json.gz", hour.Year(), hour.Month(), hour.Day(), hour.Hour())
	f.fetched = append(f.fetched, key)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if f.failHour != "" && f.failHour == key {
		return nil, errors.New("injected failure")
	}
	data, ok := f.files[key]
	if !ok {
		return nil, fmt.Errorf("not found: %s", key)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (f *fakeArchiveFetcher) fetchedKeys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.fetched...)
}

func ghArchiveEventLine(t string, payload map[string]any) []byte {
	ev := map[string]any{
		"id":         "1",
		"type":       t,
		"actor":      map[string]any{"id": 1, "login": "alice"},
		"repo":       map[string]any{"id": 1, "name": "owner/repo"},
		"payload":    payload,
		"created_at": "2023-01-01T00:00:00Z",
	}
	b, _ := json.Marshal(ev)
	return b
}

func ghArchiveGZIP(lines ...[]byte) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	for _, line := range lines {
		gw.Write(line)
		gw.Write([]byte("\n"))
	}
	gw.Close()
	return buf.Bytes()
}

func TestAddRepoSourceAndCrawl(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := newTestServer("octocat", "test")
	defer srv.Close()
	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()

	source, err := svc.AddRepoSource(ctx, "my-repos", []cli.RepoRef{{Owner: "golang", Repo: "go"}, {Owner: "octocat", Repo: "hello-world"}})
	if err != nil {
		t.Fatalf("add repo source: %v", err)
	}
	if source.Name != "my-repos" || source.Kind != "repos" {
		t.Fatalf("unexpected source: %+v", source)
	}

	result, err := svc.Crawl(ctx, "my-repos", cli.CrawlOptions{Since: time.Hour, Budget: 10})
	if err != nil {
		t.Fatalf("crawl repo source: %v", err)
	}
	if result.Repositories != 2 {
		t.Fatalf("repositories = %d, want 2", result.Repositories)
	}
	if result.Requests != 0 {
		t.Fatalf("requests = %d, want 0", result.Requests)
	}

	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, ownerRepo := range []struct{ owner, repo string }{{"golang", "go"}, {"octocat", "hello-world"}} {
		r, err := c.GetRepository(ctx, ownerRepo.owner, ownerRepo.repo)
		if err != nil {
			t.Fatal(err)
		}
		if r == nil {
			t.Fatalf("missing repository %s/%s", ownerRepo.owner, ownerRepo.repo)
		}
		item, err := c.GetFrontierItem(ctx, fmt.Sprintf("repository:%s/%s:threads", ownerRepo.owner, ownerRepo.repo))
		if err != nil {
			t.Fatal(err)
		}
		if item == nil {
			t.Fatalf("missing frontier for %s/%s", ownerRepo.owner, ownerRepo.repo)
		}
	}
}

func TestRepoSourceCrawlDeduplicates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := newTestServer("octocat", "test")
	defer srv.Close()
	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()

	source, err := svc.AddRepoSource(ctx, "dupes", []cli.RepoRef{
		{Owner: "golang", Repo: "go"},
		{Owner: "golang", Repo: "go"},
		{Owner: "golang", Repo: "go"},
	})
	if err != nil {
		t.Fatalf("add repo source: %v", err)
	}
	if _, err := svc.Crawl(ctx, source.Name, cli.CrawlOptions{Since: time.Hour, Budget: 10}); err != nil {
		t.Fatalf("crawl: %v", err)
	}
	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	status, err := c.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Repositories != 1 {
		t.Fatalf("repositories = %d, want canonical dedupe to 1", status.Repositories)
	}
}

func TestRepoSourceCrawlRespectsBudget(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := newTestServer("octocat", "test")
	defer srv.Close()
	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()

	refs := []cli.RepoRef{
		{Owner: "a", Repo: "one"},
		{Owner: "a", Repo: "two"},
		{Owner: "a", Repo: "three"},
	}
	source, err := svc.AddRepoSource(ctx, "budgeted", refs)
	if err != nil {
		t.Fatalf("add repo source: %v", err)
	}
	result, err := svc.Crawl(ctx, source.Name, cli.CrawlOptions{Since: time.Hour, Budget: 2})
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}
	if result.Repositories != 2 {
		t.Fatalf("repositories = %d, want 2", result.Repositories)
	}
}

func TestRepoSourceCrawlNoNetwork(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// Do not set a GitHub reader; the explicit repo source should never need it.
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()

	source, err := svc.AddRepoSource(ctx, "offline", []cli.RepoRef{{Owner: "a", Repo: "b"}})
	if err != nil {
		t.Fatalf("add repo source: %v", err)
	}
	if _, err := svc.Crawl(ctx, source.Name, cli.CrawlOptions{Since: time.Hour, Budget: 10}); err != nil {
		t.Fatalf("crawl: %v", err)
	}
}

func TestAddGHArchiveSourceAndCrawl(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := newTestServer("octocat", "test")
	defer srv.Close()
	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()

	// Crawl a 2-hour window. The latest complete hour is one hour before now.
	now := time.Date(2023, 1, 1, 4, 15, 0, 0, time.UTC)
	startHour, endHour := discovery.ArchiveHourRange(2*time.Hour, now)
	if !startHour.Equal(time.Date(2023, 1, 1, 2, 0, 0, 0, time.UTC)) || !endHour.Equal(time.Date(2023, 1, 1, 3, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected hour bounds: %v to %v", startHour, endHour)
	}

	fetcher := &fakeArchiveFetcher{files: map[string][]byte{
		"2023-01-01-2.json.gz": ghArchiveGZIP(ghArchiveEventLine("IssuesEvent", map[string]any{
			"action": "opened",
			"issue":  map[string]any{"number": 1, "title": "bug", "state": "open", "user": map[string]any{"login": "u"}},
		})),
		"2023-01-01-3.json.gz": ghArchiveGZIP(ghArchiveEventLine("IssuesEvent", map[string]any{
			"action": "opened",
			"issue":  map[string]any{"number": 2, "title": "bug2", "state": "open", "user": map[string]any{"login": "v"}},
		})),
	}}
	svc.SetClock(func() time.Time { return now })
	svc.SetArchiveFetcher(fetcher)

	source, err := svc.AddGHArchiveSource(ctx, "archive-test", []string{"IssuesEvent"})
	if err != nil {
		t.Fatalf("add gharchive source: %v", err)
	}
	if source.Kind != "gharchive" {
		t.Fatalf("unexpected source kind %q", source.Kind)
	}

	result, err := svc.Crawl(ctx, source.Name, cli.CrawlOptions{Since: 2 * time.Hour, Budget: 10})
	if err != nil {
		t.Fatalf("crawl gharchive: %v", err)
	}
	if result.Windows != 2 {
		t.Fatalf("windows = %d, want 2", result.Windows)
	}
	if result.Imported != 2 {
		t.Fatalf("imported = %d, want 2", result.Imported)
	}
	if result.Events != 2 {
		t.Fatalf("events = %d, want 2", result.Events)
	}
	if result.Repositories != 1 || result.Threads != 2 {
		t.Fatalf("unexpected repositories/threads: %d/%d", result.Repositories, result.Threads)
	}

	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	storedSource, err := c.GetDiscoverySource(ctx, source.Name)
	if err != nil {
		t.Fatal(err)
	}
	for _, hour := range []time.Time{startHour, endHour} {
		key := archiveImportKey(storedSource, hour)
		imported, err := c.IsImported(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if !imported {
			t.Fatalf("hour %s not marked imported", key)
		}
	}
	r, err := c.GetRepository(ctx, "owner", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if r == nil {
		t.Fatal("missing repository from archive")
	}
	for _, n := range []int{1, 2} {
		thread, err := c.GetThread(ctx, r.ID, corpus.ThreadKindIssue, n)
		if err != nil {
			t.Fatal(err)
		}
		if thread == nil {
			t.Fatalf("missing thread %d", n)
		}
	}
	frontier, err := c.GetFrontierItem(ctx, "repository:owner/repo:threads")
	if err != nil {
		t.Fatal(err)
	}
	if frontier == nil {
		t.Fatal("missing frontier item")
	}
}

func TestGHArchiveCrawlSkipsImportedHours(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := newTestServer("octocat", "test")
	defer srv.Close()
	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()

	now := time.Date(2023, 1, 1, 2, 15, 0, 0, time.UTC)
	event := ghArchiveEventLine("IssuesEvent", map[string]any{
		"action": "opened",
		"issue":  map[string]any{"number": 1, "title": "bug", "state": "open", "user": map[string]any{"login": "u"}},
	})
	fetcher := &fakeArchiveFetcher{files: map[string][]byte{
		"2023-01-01-0.json.gz": ghArchiveGZIP(event),
		"2023-01-01-1.json.gz": ghArchiveGZIP(event),
	}}
	svc.SetClock(func() time.Time { return now })
	svc.SetArchiveFetcher(fetcher)

	source, err := svc.AddGHArchiveSource(ctx, "replay", []string{"IssuesEvent"})
	if err != nil {
		t.Fatalf("add source: %v", err)
	}
	if _, err := svc.Crawl(ctx, source.Name, cli.CrawlOptions{Since: 2 * time.Hour, Budget: 10}); err != nil {
		t.Fatalf("first crawl: %v", err)
	}
	first := len(fetcher.fetchedKeys())
	if first == 0 {
		t.Fatal("no hours fetched")
	}

	result, err := svc.Crawl(ctx, source.Name, cli.CrawlOptions{Since: 2 * time.Hour, Budget: 10})
	if err != nil {
		t.Fatalf("second crawl: %v", err)
	}
	if result.Skipped != 2 {
		t.Fatalf("skipped = %d, want 2", result.Skipped)
	}
	if result.Imported != 0 {
		t.Fatalf("imported = %d, want 0 on replay", result.Imported)
	}
	if len(fetcher.fetchedKeys()) != first {
		t.Fatalf("fetched %d times on replay, want %d", len(fetcher.fetchedKeys()), first)
	}
}

func TestGHArchiveCrawlScopesImportsBySourceDefinition(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := newTestServer("octocat", "test")
	defer srv.Close()
	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()

	now := time.Date(2023, 1, 1, 1, 15, 0, 0, time.UTC)
	fetcher := &fakeArchiveFetcher{files: map[string][]byte{
		"2023-01-01-0.json.gz": ghArchiveGZIP(
			ghArchiveEventLine("IssuesEvent", map[string]any{"action": "opened", "issue": map[string]any{"number": 1}}),
			ghArchiveEventLine("PullRequestEvent", map[string]any{"action": "opened", "pull_request": map[string]any{"number": 2}}),
		),
	}}
	svc.SetClock(func() time.Time { return now })
	svc.SetArchiveFetcher(fetcher)

	issues, err := svc.AddGHArchiveSource(ctx, "issues", []string{"IssuesEvent"})
	if err != nil {
		t.Fatal(err)
	}
	pulls, err := svc.AddGHArchiveSource(ctx, "pulls", []string{"PullRequestEvent"})
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []*cli.SourceResult{issues, pulls} {
		if _, err := svc.Crawl(ctx, source.Name, cli.CrawlOptions{Since: time.Hour, Budget: 1}); err != nil {
			t.Fatalf("crawl %s: %v", source.Name, err)
		}
	}
	if got := len(fetcher.fetchedKeys()); got != 2 {
		t.Fatalf("fetches = %d, want 2 independently scoped imports", got)
	}
}

func TestGHArchiveCrawlBudgetEnforcement(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := newTestServer("octocat", "test")
	defer srv.Close()
	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()

	now := time.Date(2023, 1, 1, 2, 15, 0, 0, time.UTC)
	fetcher := &fakeArchiveFetcher{files: map[string][]byte{
		"2023-01-01-0.json.gz": ghArchiveGZIP(ghArchiveEventLine("IssuesEvent", map[string]any{
			"action": "opened",
			"issue":  map[string]any{"number": 1, "title": "bug", "state": "open", "user": map[string]any{"login": "u"}},
		})),
		"2023-01-01-1.json.gz": ghArchiveGZIP(ghArchiveEventLine("IssuesEvent", map[string]any{
			"action": "opened",
			"issue":  map[string]any{"number": 2, "title": "bug2", "state": "open", "user": map[string]any{"login": "v"}},
		})),
	}}
	svc.SetClock(func() time.Time { return now })
	svc.SetArchiveFetcher(fetcher)

	source, err := svc.AddGHArchiveSource(ctx, "budget", nil)
	if err != nil {
		t.Fatalf("add source: %v", err)
	}
	result, err := svc.Crawl(ctx, source.Name, cli.CrawlOptions{Since: 2 * time.Hour, Budget: 1})
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}
	if result.Requests != 1 {
		t.Fatalf("requests = %d, want 1", result.Requests)
	}
	if result.Imported != 1 {
		t.Fatalf("imported = %d, want 1", result.Imported)
	}
	if result.Windows != 1 {
		t.Fatalf("windows = %d, want 1", result.Windows)
	}
	if result.Checkpoint != "2023-01-01T00:00:00Z" {
		t.Fatalf("checkpoint = %q, want first processed hour", result.Checkpoint)
	}
}

func TestGHArchiveCrawlRepositoryDedupe(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := newTestServer("octocat", "test")
	defer srv.Close()
	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()

	now := time.Date(2023, 1, 1, 1, 15, 0, 0, time.UTC)
	lines := []struct {
		n int
	}{
		{1},
		{2},
		{1},
		{2},
		{3},
	}
	var eventLines [][]byte
	for _, l := range lines {
		eventLines = append(eventLines, ghArchiveEventLine("IssuesEvent", map[string]any{
			"action": "opened",
			"issue":  map[string]any{"number": l.n, "title": fmt.Sprintf("bug%d", l.n), "state": "open", "user": map[string]any{"login": "u"}},
		}))
	}
	fetcher := &fakeArchiveFetcher{files: map[string][]byte{
		"2023-01-01-0.json.gz": ghArchiveGZIP(eventLines...),
	}}
	svc.SetClock(func() time.Time { return now })
	svc.SetArchiveFetcher(fetcher)

	source, err := svc.AddGHArchiveSource(ctx, "dedupe", []string{"IssuesEvent"})
	if err != nil {
		t.Fatalf("add source: %v", err)
	}
	result, err := svc.Crawl(ctx, source.Name, cli.CrawlOptions{Since: time.Hour, Budget: 10})
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}
	if result.Events != 5 {
		t.Fatalf("events = %d, want 5", result.Events)
	}
	if result.Repositories != 1 || result.Threads != 3 {
		t.Fatalf("repositories=%d threads=%d, want 1/3", result.Repositories, result.Threads)
	}
}

func TestGHArchiveCrawlMalformedArchive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := newTestServer("octocat", "test")
	defer srv.Close()
	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()

	now := time.Date(2023, 1, 1, 1, 15, 0, 0, time.UTC)
	fetcher := &fakeArchiveFetcher{files: map[string][]byte{
		"2023-01-01-0.json.gz": []byte("not a gzip file"),
	}}
	svc.SetClock(func() time.Time { return now })
	svc.SetArchiveFetcher(fetcher)

	source, err := svc.AddGHArchiveSource(ctx, "bad", []string{"IssuesEvent"})
	if err != nil {
		t.Fatalf("add source: %v", err)
	}
	result, err := svc.Crawl(ctx, source.Name, cli.CrawlOptions{Since: time.Hour, Budget: 10})
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}
	if result.Failures != 1 {
		t.Fatalf("failures = %d, want 1", result.Failures)
	}
	if result.Checkpoint != "" {
		t.Fatalf("checkpoint = %q, want empty after first-hour failure", result.Checkpoint)
	}
	c, _ := svc.openCorpus(ctx)
	runs, err := c.ListRuns(ctx, 1)
	if err != nil || len(runs) != 1 || runs[0].Status != corpus.RunStatusFailed {
		t.Fatalf("latest run = %+v, err=%v; want failed", runs, err)
	}
}

func TestGHArchiveCrawlFetchFailureContinues(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := newTestServer("octocat", "test")
	defer srv.Close()
	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()

	now := time.Date(2023, 1, 1, 2, 15, 0, 0, time.UTC)
	fetcher := &fakeArchiveFetcher{
		files: map[string][]byte{
			"2023-01-01-0.json.gz": ghArchiveGZIP(ghArchiveEventLine("IssuesEvent", map[string]any{
				"action": "opened",
				"issue":  map[string]any{"number": 1, "title": "bug", "state": "open", "user": map[string]any{"login": "u"}},
			})),
		},
		failHour: "2023-01-01-1.json.gz",
	}
	svc.SetClock(func() time.Time { return now })
	svc.SetArchiveFetcher(fetcher)

	source, err := svc.AddGHArchiveSource(ctx, "partial", []string{"IssuesEvent"})
	if err != nil {
		t.Fatalf("add source: %v", err)
	}
	result, err := svc.Crawl(ctx, source.Name, cli.CrawlOptions{Since: 2 * time.Hour, Budget: 10})
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}
	if result.Imported != 1 || result.Failures != 1 {
		t.Fatalf("imported=%d failures=%d, want 1/1", result.Imported, result.Failures)
	}
	if result.Checkpoint != "2023-01-01T00:00:00Z" {
		t.Fatalf("checkpoint = %q, want last contiguous successful hour", result.Checkpoint)
	}
	c, _ := svc.openCorpus(ctx)
	runs, err := c.ListRuns(ctx, 1)
	if err != nil || len(runs) != 1 || runs[0].Status != corpus.RunStatusPartial {
		t.Fatalf("latest run = %+v, err=%v; want partial", runs, err)
	}
}

func TestArchiveMergePreservesNewerProjection(t *testing.T) {
	t.Parallel()
	newer := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	signal := discovery.Signal{
		Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, RepoID: 42,
		ThreadKind: domain.IssueKind, ThreadNumber: 7, ThreadState: domain.ThreadState("closed"),
		ObservedAt: newer.Add(-time.Hour),
	}
	repo := corpus.Repository{ID: 1, Owner: "owner", Name: "repo", Description: "current", SourceUpdatedAt: newer}
	if got := mergeArchiveRepo(signal, &repo); got.Description != "current" || !got.SourceUpdatedAt.IsZero() {
		t.Fatalf("repository regressed: %+v", got)
	}
	thread := corpus.Thread{ID: 2, RepositoryID: 1, Kind: corpus.ThreadKindIssue, Number: 7, State: "open", Title: "current", SourceUpdatedAt: newer}
	if got := mergeArchiveThread(signal, 1, &thread); got.State != "open" || got.Title != "current" || !got.SourceUpdatedAt.IsZero() {
		t.Fatalf("thread regressed: %+v", got)
	}
}

func TestArchiveDiscoveryCannotOutrankCanonicalSync(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()
	c, _ := svc.openCorpus(ctx)

	signal := discovery.Signal{
		Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, RepoID: 42,
		ThreadKind: domain.IssueKind, ThreadNumber: 7, ThreadTitle: "sparse archive title",
		ObservedAt: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
	}
	archiveRepo, err := c.UpsertRepository(ctx, mergeArchiveRepo(signal, nil), `{"source":"archive"}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.UpsertThread(ctx, mergeArchiveThread(signal, archiveRepo.ID, nil), `{"source":"archive"}`); err != nil {
		t.Fatal(err)
	}

	canonicalTime := signal.ObservedAt.Add(-time.Hour)
	canonicalRepo, err := c.UpsertRepository(ctx, corpus.Repository{
		Owner: "owner", Name: "repo", ExternalID: "R_42", Description: "canonical metadata", Stars: 99, SourceUpdatedAt: canonicalTime,
	}, `{"source":"github"}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.UpsertThread(ctx, corpus.Thread{
		RepositoryID: canonicalRepo.ID, Kind: corpus.ThreadKindIssue, Number: 7, Title: "canonical title", Body: "canonical body", SourceUpdatedAt: canonicalTime,
	}, `{"source":"github"}`); err != nil {
		t.Fatal(err)
	}

	gotRepo, _ := c.GetRepository(ctx, "owner", "repo")
	if gotRepo.Description != "canonical metadata" || gotRepo.Stars != 99 {
		t.Fatalf("canonical repository did not win: %+v", gotRepo)
	}
	gotThread, _ := c.GetThread(ctx, canonicalRepo.ID, corpus.ThreadKindIssue, 7)
	if gotThread.Title != "canonical title" || gotThread.Body != "canonical body" {
		t.Fatalf("canonical thread did not win: %+v", gotThread)
	}
}

func TestGHArchiveCrawlCancellation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := newTestServer("octocat", "test")
	defer srv.Close()
	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()

	now := time.Date(2023, 1, 1, 1, 15, 0, 0, time.UTC)
	fetcher := &fakeArchiveFetcher{files: map[string][]byte{
		"2023-01-01-0.json.gz": ghArchiveGZIP(ghArchiveEventLine("IssuesEvent", map[string]any{
			"action": "opened",
			"issue":  map[string]any{"number": 1, "title": "bug", "state": "open", "user": map[string]any{"login": "u"}},
		})),
	}}
	svc.SetClock(func() time.Time { return now })
	svc.SetArchiveFetcher(fetcher)

	source, err := svc.AddGHArchiveSource(ctx, "cancel", []string{"IssuesEvent"})
	if err != nil {
		t.Fatalf("add source: %v", err)
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = svc.Crawl(ctx, source.Name, cli.CrawlOptions{Since: time.Hour, Budget: 10})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestAddRepoSourceRejectsInvalidURL(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := newTestServer("octocat", "test")
	defer srv.Close()
	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()

	_, err := svc.AddRepoSource(ctx, "bad", []cli.RepoRef{{Owner: "gitlab.com/foo", Repo: "bar"}})
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("expected invalid repo error, got %v", err)
	}
}

func newTestServiceNoNetwork(t *testing.T) *Service {
	t.Helper()
	ctx := context.Background()
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if _, err := svc.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}
	return svc
}
