package corpus

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func openTestCorpus(t *testing.T) (*Corpus, string) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "corpus.db")
	c, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, path
}

func TestMigrationLoggerFatalfRecordsError(t *testing.T) {
	t.Parallel()
	logger := &migrationLogger{}
	logger.Fatalf("migration %d failed", 7)
	logger.Fatalf("later failure")

	if err := logger.Err(); err == nil || err.Error() != "migration 7 failed" {
		t.Fatalf("migration logger error = %v", err)
	}
}

func TestOpenAndPragmas(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	var fk, busy int
	var journal string
	if err := c.db.QueryRowContext(ctx, "PRAGMA foreign_keys;").Scan(&fk); err != nil {
		t.Fatalf("read foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Fatalf("foreign_keys = %d, want 1", fk)
	}
	if err := c.db.QueryRowContext(ctx, "PRAGMA busy_timeout;").Scan(&busy); err != nil {
		t.Fatalf("read busy_timeout: %v", err)
	}
	if busy == 0 {
		t.Fatalf("busy_timeout = 0")
	}
	if err := c.db.QueryRowContext(ctx, "PRAGMA journal_mode;").Scan(&journal); err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if journal != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journal)
	}
}

func TestOpenReopensPathAfterInitializationLeaseHandoff(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	target := filepath.Join(dir, "target.db")
	replacement := filepath.Join(dir, "replacement.db")

	replacementCorpus, err := Open(ctx, replacement)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := replacementCorpus.ApplyRepositoryObservation(ctx, "owner", "replacement", "external", time.Unix(1, 0), `{}`); err != nil {
		t.Fatal(err)
	}
	if err := replacementCorpus.Close(); err != nil {
		t.Fatal(err)
	}

	originalHandoff := openLeaseHandoff
	t.Cleanup(func() { openLeaseHandoff = originalHandoff })
	openLeaseHandoff = func(path string) error {
		if path != target {
			return fmt.Errorf("handoff path = %q, want %q", path, target)
		}
		return replaceDatabaseFile(replacement, target)
	}

	c, err := Open(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	repo, err := c.GetRepository(ctx, "owner", "replacement")
	if err != nil {
		t.Fatal(err)
	}
	if repo == nil {
		t.Fatal("opened corpus retained the initialized inode instead of reopening the replacement path")
	}
}

func TestConcurrentOpenUsesIndependentMigrationProviders(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	const count = 8
	errs := make(chan error, count)
	var wg sync.WaitGroup
	for i := range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := Open(ctx, filepath.Join(root, fmt.Sprintf("corpus-%d.db", i)))
			if err == nil {
				err = c.Close()
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent open: %v", err)
		}
	}
}

func TestIdempotentReopenAndMigration(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, path := openTestCorpus(t)

	want := time.Unix(1700000000, 0).UTC()
	_, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "123", want, `{"stars":1}`)
	if err != nil {
		t.Fatalf("apply repository observation: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("close corpus: %v", err)
	}

	// Reopening must be idempotent: migrations have already run and data is
	// still present.
	c2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen corpus: %v", err)
	}
	defer func() { _ = c2.Close() }()

	repo, err := c2.GetRepository(ctx, "owner", "repo")
	if err != nil {
		t.Fatalf("get repository after reopen: %v", err)
	}
	if repo == nil {
		t.Fatal("repository missing after reopen")
	}
	if !repo.SourceUpdatedAt.Equal(want) {
		t.Fatalf("source_updated_at = %v, want %v", repo.SourceUpdatedAt, want)
	}
}

func TestOpenRejectsSchemaNewerThanBinary(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, path := openTestCorpus(t)
	_, target, err := c.SchemaVersions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.ExecContext(ctx, `INSERT INTO goose_db_version (version_id, is_applied, tstamp) VALUES (?, 1, CURRENT_TIMESTAMP)`, target+1); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = Open(ctx, path)
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("schema version %d is newer", target+1)) {
		t.Fatalf("Open error = %v", err)
	}
}

func TestCheckWriteAccessReportsContentionImmediatelyAndRestoresTimeout(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, path := openTestCorpus(t)
	other, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Close() }()

	conn, err := c.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), `ROLLBACK`) }()

	started := time.Now()
	err = other.CheckWriteAccess(ctx)
	if err == nil || !strings.Contains(err.Error(), "database is locked") {
		t.Fatalf("CheckWriteAccess error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("CheckWriteAccess waited %v under contention", elapsed)
	}
	var busyTimeout int
	if err := other.db.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		t.Fatal(err)
	}
	if busyTimeout != 5000 {
		t.Fatalf("busy_timeout = %d, want 5000", busyTimeout)
	}
}

func TestRepositoryDelayedObservations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	newer := time.Unix(2000, 0).UTC()
	older := time.Unix(1000, 0).UTC()

	if _, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "first", newer, `{"stars":2}`); err != nil {
		t.Fatalf("apply newer observation: %v", err)
	}
	if _, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "second", older, `{"stars":1}`); err != nil {
		t.Fatalf("apply older observation: %v", err)
	}

	repo, err := c.GetRepository(ctx, "owner", "repo")
	if err != nil {
		t.Fatalf("get repository: %v", err)
	}
	if !repo.SourceUpdatedAt.Equal(newer) {
		t.Fatalf("source_updated_at = %v, want %v", repo.SourceUpdatedAt, newer)
	}
	if repo.ExternalID != "first" {
		t.Fatalf("external_id = %q, want first", repo.ExternalID)
	}

	obs, err := c.ListRepositoryObservations(ctx, repo.ID)
	if err != nil {
		t.Fatalf("list observations: %v", err)
	}
	if len(obs) != 2 {
		t.Fatalf("observations = %d, want 2", len(obs))
	}
}

func TestRepositoryObservationOrderingPreservesNanoseconds(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	base := time.Unix(2000, 100).UTC()
	newer := time.Unix(2000, 200).UTC()
	if _, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "old", base, `{}`); err != nil {
		t.Fatalf("apply base observation: %v", err)
	}
	if _, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "new", newer, `{}`); err != nil {
		t.Fatalf("apply newer observation: %v", err)
	}

	repo, err := c.GetRepository(ctx, "owner", "repo")
	if err != nil {
		t.Fatalf("get repository: %v", err)
	}
	if !repo.SourceUpdatedAt.Equal(newer) || repo.ExternalID != "new" {
		t.Fatalf("repository = %+v, want nanosecond-newer observation", repo)
	}
}

func TestSearchTreatsFTSOperatorsAndQuotesLiterally(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Now(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 1, "open", `fix OR unmatched " quote`, "body", "author", time.Now(), `{}`); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"OR", `unmatched "`, "fix"} {
		results, err := c.SearchThreads(ctx, query, 10)
		if err != nil {
			t.Fatalf("SearchThreads(%q): %v", query, err)
		}
		if len(results) != 1 {
			t.Fatalf("SearchThreads(%q) returned %d results", query, len(results))
		}
	}
}

func TestSourceAndLocalProjectionTimesRemainDistinct(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	sourceCreated := time.Unix(100, 123).UTC()
	sourceUpdated := time.Unix(200, 456).UTC()
	before := time.Now().UTC()
	repo, err := c.UpsertRepository(ctx, Repository{
		Owner: "owner", Name: "repo", SourceCreatedAt: sourceCreated, SourceUpdatedAt: sourceUpdated,
	}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if !repo.SourceCreatedAt.Equal(sourceCreated) || !repo.SourceUpdatedAt.Equal(sourceUpdated) {
		t.Fatalf("source times = (%v, %v)", repo.SourceCreatedAt, repo.SourceUpdatedAt)
	}
	if repo.CreatedAt.Before(before) || repo.UpdatedAt.Before(before) {
		t.Fatalf("local projection times reused source clocks: %+v", repo)
	}
	thread, err := c.UpsertThread(ctx, Thread{
		RepositoryID: repo.ID, Kind: ThreadKindIssue, Number: 1, State: "open", Title: "title",
		SourceCreatedAt: sourceCreated, SourceUpdatedAt: sourceUpdated,
	}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if !thread.SourceCreatedAt.Equal(sourceCreated) || thread.CreatedAt.Before(before) {
		t.Fatalf("thread times = %+v", thread)
	}
}

func TestRepositoryEqualTimestampSequenceOrdering(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	ts := time.Unix(3000, 0).UTC()
	if _, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "first", ts, `p1`); err != nil {
		t.Fatalf("apply first observation: %v", err)
	}
	if _, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "second", ts, `p2`); err != nil {
		t.Fatalf("apply second observation: %v", err)
	}

	repo, err := c.GetRepository(ctx, "owner", "repo")
	if err != nil {
		t.Fatalf("get repository: %v", err)
	}
	if !repo.SourceUpdatedAt.Equal(ts) {
		t.Fatalf("source_updated_at = %v, want %v", repo.SourceUpdatedAt, ts)
	}
	if repo.ExternalID != "second" {
		t.Fatalf("external_id = %q, want second", repo.ExternalID)
	}

	obs, err := c.ListRepositoryObservations(ctx, repo.ID)
	if err != nil {
		t.Fatalf("list observations: %v", err)
	}
	if len(obs) != 2 {
		t.Fatalf("observations = %d, want 2", len(obs))
	}
	if obs[0].ObservationSequence >= obs[1].ObservationSequence {
		t.Fatalf("observation sequences not increasing: %v >= %v", obs[0].ObservationSequence, obs[1].ObservationSequence)
	}
}

func TestThreadDelayedObservations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "1", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("apply repository: %v", err)
	}

	newer := time.Unix(2000, 0).UTC()
	older := time.Unix(1000, 0).UTC()

	if _, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 1, "open", "first title", "body", "a", newer, `{"comments":0}`); err != nil {
		t.Fatalf("apply newer thread observation: %v", err)
	}
	if _, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 1, "open", "stale title", "body", "a", older, `{"comments":0}`); err != nil {
		t.Fatalf("apply older thread observation: %v", err)
	}

	thread, err := c.GetThread(ctx, repo.ID, ThreadKindIssue, 1)
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if !thread.SourceUpdatedAt.Equal(newer) {
		t.Fatalf("source_updated_at = %v, want %v", thread.SourceUpdatedAt, newer)
	}
	if thread.Title != "first title" {
		t.Fatalf("title = %q, want %q", thread.Title, "first title")
	}

	obs, err := c.ListThreadObservations(ctx, thread.ID)
	if err != nil {
		t.Fatalf("list thread observations: %v", err)
	}
	if len(obs) != 2 {
		t.Fatalf("observations = %d, want 2", len(obs))
	}
}

func TestThreadObservationReplayIsIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "1", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	sourceUpdatedAt := time.Unix(1000, 0).UTC()
	for range 2 {
		if _, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 1, "open", "title", "body", "author", sourceUpdatedAt, `{"id":1}`); err != nil {
			t.Fatal(err)
		}
	}
	thread, err := c.GetThread(ctx, repo.ID, ThreadKindIssue, 1)
	if err != nil {
		t.Fatal(err)
	}
	observations, err := c.ListThreadObservations(ctx, thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 1 {
		t.Fatalf("replayed observations = %d, want 1", len(observations))
	}
}

func TestThreadEqualTimestampSequenceOrdering(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "1", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("apply repository: %v", err)
	}

	ts := time.Unix(4000, 0).UTC()
	if _, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 1, "open", "first", "body", "a", ts, `p1`); err != nil {
		t.Fatalf("apply first thread observation: %v", err)
	}
	if _, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 1, "open", "second", "body", "a", ts, `p2`); err != nil {
		t.Fatalf("apply second thread observation: %v", err)
	}

	thread, err := c.GetThread(ctx, repo.ID, ThreadKindIssue, 1)
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if !thread.SourceUpdatedAt.Equal(ts) {
		t.Fatalf("source_updated_at = %v, want %v", thread.SourceUpdatedAt, ts)
	}
	if thread.Title != "second" {
		t.Fatalf("title = %q, want second", thread.Title)
	}
}

func TestIndependentFacetAdvancement(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "1", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("apply repository: %v", err)
	}
	thread, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindPullRequest, 42, "open", "title", "body", "a", time.Unix(2, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("apply thread: %v", err)
	}

	run, err := c.StartRun(ctx, "hydrate")
	if err != nil {
		t.Fatalf("start run: %v", err)
	}

	tid := thread.ID
	if err := c.AdvanceFacet(ctx, repo.ID, &tid, "comments", time.Unix(1000, 0).UTC(), true, run.ID); err != nil {
		t.Fatalf("advance comments facet: %v", err)
	}
	if err := c.AdvanceFacet(ctx, repo.ID, &tid, "pr_details", time.Unix(900, 0).UTC(), false, run.ID); err != nil {
		t.Fatalf("advance pr_details facet: %v", err)
	}

	comments, err := c.GetCoverage(ctx, repo.ID, &tid, "comments")
	if err != nil {
		t.Fatalf("get comments coverage: %v", err)
	}
	if comments == nil || !comments.Complete || comments.SourceUpdatedAt.Unix() != 1000 {
		t.Fatalf("comments coverage mismatch: %+v", comments)
	}

	details, err := c.GetCoverage(ctx, repo.ID, &tid, "pr_details")
	if err != nil {
		t.Fatalf("get pr_details coverage: %v", err)
	}
	if details == nil || details.Complete || details.SourceUpdatedAt.Unix() != 900 {
		t.Fatalf("pr_details coverage mismatch: %+v", details)
	}

	// A stale comments facet should not overwrite a newer one.
	if err := c.AdvanceFacet(ctx, repo.ID, &tid, "comments", time.Unix(500, 0).UTC(), true, run.ID); err != nil {
		t.Fatalf("advance stale comments facet: %v", err)
	}
	comments, err = c.GetCoverage(ctx, repo.ID, &tid, "comments")
	if err != nil {
		t.Fatalf("get comments coverage after stale update: %v", err)
	}
	if comments.SourceUpdatedAt.Unix() != 1000 {
		t.Fatalf("comments coverage was overwritten by stale source_updated_at: %v", comments.SourceUpdatedAt)
	}
}

func TestInterruptedAndFailedRuns(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	run, err := c.StartRun(ctx, "sync")
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if run.Status != RunStatusRunning {
		t.Fatalf("run status = %q, want running", run.Status)
	}

	if err := c.RecordRunEvent(ctx, run.ID, "warn", "interrupted by signal"); err != nil {
		t.Fatalf("record run event: %v", err)
	}
	if err := c.FailRun(ctx, run.ID, "interrupted by signal"); err != nil {
		t.Fatalf("fail run: %v", err)
	}

	run, err = c.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != RunStatusFailed {
		t.Fatalf("run status = %q, want failed", run.Status)
	}
	if run.Error != "interrupted by signal" {
		t.Fatalf("run error = %q, want %q", run.Error, "interrupted by signal")
	}
	if run.CompletedAt == nil {
		t.Fatal("run completed_at is nil")
	}

	events, err := c.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatalf("list run events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("run events = %d, want 1", len(events))
	}
	if events[0].Message != "interrupted by signal" {
		t.Fatalf("event message = %q, want %q", events[0].Message, "interrupted by signal")
	}
}

func TestLocalSearch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "1", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("apply repository: %v", err)
	}
	if _, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 1, "open", "searchable term", "body text here", "a", time.Unix(2, 0).UTC(), `{}`); err != nil {
		t.Fatalf("apply matching thread: %v", err)
	}
	if _, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 2, "open", "unrelated", "nothing", "a", time.Unix(2, 0).UTC(), `{}`); err != nil {
		t.Fatalf("apply unrelated thread: %v", err)
	}
	if _, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindPullRequest, 3, "open", "another term", "more body", "a", time.Unix(2, 0).UTC(), `{}`); err != nil {
		t.Fatalf("apply pr thread: %v", err)
	}

	results, err := c.SearchThreads(ctx, "term", 10)
	if err != nil {
		t.Fatalf("search threads: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("search results = %d, want 2: %+v", len(results), results)
	}

	got := []int{results[0].Number, results[1].Number}
	sort.Ints(got)
	want := []int{1, 3}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("search result numbers mismatch (-want +got):\n%s", diff)
	}

	// Search body text too.
	bodyResults, err := c.SearchThreads(ctx, "body", 10)
	if err != nil {
		t.Fatalf("search body: %v", err)
	}
	if len(bodyResults) != 2 {
		t.Fatalf("body search results = %d, want 2", len(bodyResults))
	}
}

func TestCoverageIsIndependentFromProjections(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "1", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("apply repository: %v", err)
	}

	// Advance a repository-level metadata facet before any thread exists.
	run, err := c.StartRun(ctx, "metadata")
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if err := c.AdvanceFacet(ctx, repo.ID, nil, "metadata", time.Unix(5, 0).UTC(), true, run.ID); err != nil {
		t.Fatalf("advance repo metadata facet: %v", err)
	}

	cov, err := c.GetCoverage(ctx, repo.ID, nil, "metadata")
	if err != nil {
		t.Fatalf("get metadata coverage: %v", err)
	}
	if cov == nil || !cov.Complete || cov.SourceUpdatedAt.Unix() != 5 {
		t.Fatalf("metadata coverage mismatch: %+v", cov)
	}

	thread, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 1, "open", "title", "body", "a", time.Unix(10, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("apply thread: %v", err)
	}
	// Facet should not have changed because the thread projection is a
	// separate ordering surface.
	if cov.SourceUpdatedAt.Unix() != 5 {
		t.Fatalf("metadata coverage overwritten by thread projection: %v", cov.SourceUpdatedAt)
	}
	_ = thread
}

func TestRunCompletionAndStats(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	run, err := c.StartRun(ctx, "sync")
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if err := c.FinishRun(ctx, run.ID, `{"pages":3,"items":42}`); err != nil {
		t.Fatalf("finish run: %v", err)
	}

	run, err = c.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != RunStatusCompleted {
		t.Fatalf("run status = %q, want completed", run.Status)
	}
	if run.Stats != `{"pages":3,"items":42}` {
		t.Fatalf("run stats = %q", run.Stats)
	}
	if run.CompletedAt == nil {
		t.Fatal("run completed_at is nil")
	}
}

func TestProjectionIgnoresStaleThreadObservationsBySourceUpdatedAt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "1", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("apply repository: %v", err)
	}

	newer := time.Unix(2000, 0).UTC()
	older := time.Unix(1000, 0).UTC()

	// Apply observations out of chronological order.
	if _, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 1, "open", "new", "b", "a", newer, `{}`); err != nil {
		t.Fatalf("apply newer: %v", err)
	}
	if _, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 1, "open", "old", "b", "a", older, `{}`); err != nil {
		t.Fatalf("apply older: %v", err)
	}

	thread, err := c.GetThread(ctx, repo.ID, ThreadKindIssue, 1)
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if thread.Title != "new" {
		t.Fatalf("title = %q, want new", thread.Title)
	}
	if !thread.SourceUpdatedAt.Equal(newer) {
		t.Fatalf("source_updated_at = %v, want %v", thread.SourceUpdatedAt, newer)
	}
}

func ignoreVolatile() cmp.Option {
	return cmpopts.IgnoreFields(Repository{}, "ID", "CreatedAt", "UpdatedAt", "ObservationSequence")
}
