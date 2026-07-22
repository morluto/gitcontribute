package corpus

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/domain"
)

func TestProjectionStatesSeededByOpen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	threads, err := c.GetProjectionState(ctx, ProjectionNameThreadsFTS)
	if err != nil {
		t.Fatalf("get threads_fts state: %v", err)
	}
	if threads.Name != ProjectionNameThreadsFTS || threads.Version != ProjectionVersionThreadsFTS || threads.Status != ProjectionStatusCurrent {
		t.Fatalf("threads_fts state = %+v", threads)
	}

	code, err := c.GetProjectionState(ctx, ProjectionNameCodeDocumentsFTS)
	if err != nil {
		t.Fatalf("get code_documents_fts state: %v", err)
	}
	if code.Name != ProjectionNameCodeDocumentsFTS || code.Version != ProjectionVersionCodeDocumentsFTS || code.Status != ProjectionStatusCurrent {
		t.Fatalf("code_documents_fts state = %+v", code)
	}

	states, err := c.ListProjectionStates(ctx)
	if err != nil {
		t.Fatalf("list projection states: %v", err)
	}
	if len(states) != 3 {
		t.Fatalf("projection states = %d, want 3", len(states))
	}
	if states[0].Name != ProjectionNameCodeDocumentsFTS || states[1].Name != ProjectionNameFacetObservationsFTS || states[2].Name != ProjectionNameThreadsFTS {
		t.Fatalf("projection states order = %v", states)
	}
}

func TestGetProjectionStateMissing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	_, err := c.GetProjectionState(ctx, "missing_fts")
	if err == nil || !errors.Is(err, ErrProjectionNotFound) {
		t.Fatalf("expected ErrProjectionNotFound, got %v", err)
	}
}

func TestGetProjectionStateReportsKnownAbsentProjection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	if _, err := c.db.ExecContext(ctx, `DELETE FROM projection_states WHERE name = ?`, ProjectionNameThreadsFTS); err != nil {
		t.Fatal(err)
	}
	state, err := c.GetProjectionState(ctx, ProjectionNameThreadsFTS)
	if err != nil {
		t.Fatalf("get absent projection: %v", err)
	}
	if state.Name != ProjectionNameThreadsFTS || state.Status != ProjectionStatusAbsent || !state.RefreshedAt.IsZero() {
		t.Fatalf("absent projection = %+v", state)
	}
	if err := c.RequireProjection(ctx, ProjectionNameThreadsFTS, ProjectionVersionThreadsFTS); !errors.Is(err, ErrProjectionStale) {
		t.Fatalf("require absent projection error = %v, want ErrProjectionStale", err)
	}
}

func TestThreadSearchRequiresFacetProjection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	if _, err := c.db.ExecContext(ctx, `UPDATE projection_states SET status = ? WHERE name = ?`, ProjectionStatusStale, ProjectionNameFacetObservationsFTS); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SearchThreads(ctx, "term", 10); !errors.Is(err, ErrProjectionStale) {
		t.Fatalf("SearchThreads error = %v, want ErrProjectionStale", err)
	}
	state, err := c.RebuildFacetSearchProjection(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.Name != ProjectionNameFacetObservationsFTS || state.Status != ProjectionStatusCurrent {
		t.Fatalf("facet projection = %+v", state)
	}
}

func TestRebuildThreadSearchProjectionIsAtomicAndSetsState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0).UTC(), `{}`)
	requireProjectionSetup(t, "apply repository", err)
	if _, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 1, "open", "searchable title", "body text", "a", time.Unix(2, 0).UTC(), `{}`); err != nil {
		t.Fatalf("apply thread: %v", err)
	}

	before, err := c.GetProjectionState(ctx, ProjectionNameThreadsFTS)
	requireProjectionSetup(t, "get state before rebuild", err)
	// Incremental FTS triggers keep row counts current.
	if before.RowCount != 1 {
		t.Fatalf("incremental row count = %d, want 1", before.RowCount)
	}

	state, err := c.RebuildThreadSearchProjection(ctx)
	requireProjectionSetup(t, "rebuild threads_fts", err)
	if state.Name != ProjectionNameThreadsFTS || state.Version != ProjectionVersionThreadsFTS || state.Status != ProjectionStatusCurrent {
		t.Fatalf("rebuild state = %+v", state)
	}
	if state.RowCount != 1 {
		t.Fatalf("row_count = %d, want 1", state.RowCount)
	}
	if state.RefreshedAt.IsZero() {
		t.Fatal("refreshed_at is zero after rebuild")
	}

	results, err := c.SearchThreads(ctx, "searchable", 10)
	requireProjectionSetup(t, "search threads", err)
	if len(results) != 1 || results[0].Number != 1 {
		t.Fatalf("search results = %+v", results)
	}

	// A second rebuild with unchanged source should remain current and stable.
	repeat, err := c.RebuildThreadSearchProjection(ctx)
	requireProjectionSetup(t, "second rebuild", err)
	if repeat.RowCount != state.RowCount || repeat.Status != ProjectionStatusCurrent {
		t.Fatalf("second rebuild changed row count or status: %+v", repeat)
	}
	if repeat.RefreshedAt != state.RefreshedAt || repeat.ContentHash != state.ContentHash || repeat.AttemptStartedAt != state.AttemptStartedAt {
		t.Fatalf("unchanged source was rebuilt: first=%+v repeat=%+v", state, repeat)
	}
	if state.SourceRevision == "" || state.ContentHash == "" || state.AttemptStatus != ProjectionAttemptSucceeded || state.AttemptFinishedAt.IsZero() {
		t.Fatalf("rebuild metadata = %+v", state)
	}
	if _, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 1, "open", "updated title", "body text", "a", time.Unix(3, 0).UTC(), `{}`); err != nil {
		t.Fatalf("update thread source: %v", err)
	}
	changed, err := c.GetProjectionState(ctx, ProjectionNameThreadsFTS)
	requireProjectionSetup(t, "get state after source update", err)
	if changed.SourceRevision != "" || changed.ContentHash != "" {
		t.Fatalf("incremental source change retained stale identity: %+v", changed)
	}
	rebuilt, err := c.RebuildThreadSearchProjection(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt.SourceRevision == "" || rebuilt.SourceRevision == state.SourceRevision {
		t.Fatalf("changed source was not re-identified: before=%+v after=%+v", state, rebuilt)
	}
}

func requireProjectionSetup(t *testing.T, action string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", action, err)
	}
}

func TestBuildingAndFailedAttemptKeepLastCompleteProjectionReadable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 1, "open", "durable result", "body", "a", time.Unix(2, 0).UTC(), `{}`); err != nil {
		t.Fatal(err)
	}
	complete, err := c.RebuildThreadSearchProjection(ctx)
	if err != nil {
		t.Fatal(err)
	}

	for _, status := range []ProjectionStatus{ProjectionStatusBuilding, ProjectionStatusFailed} {
		if _, err := c.db.ExecContext(ctx, `UPDATE projection_states SET status = ? WHERE name = ?`, status, ProjectionNameThreadsFTS); err != nil {
			t.Fatal(err)
		}
		results, err := c.SearchThreads(ctx, "durable", 10)
		if err != nil {
			t.Fatalf("search with %s attempt: %v", status, err)
		}
		if len(results) != 1 {
			t.Fatalf("search with %s attempt returned %d results", status, len(results))
		}
		state, err := c.GetProjectionState(ctx, ProjectionNameThreadsFTS)
		if err != nil {
			t.Fatal(err)
		}
		if state.Version != complete.Version || state.ContentHash != complete.ContentHash || state.RefreshedAt != complete.RefreshedAt {
			t.Fatalf("%s attempt replaced last complete metadata: complete=%+v state=%+v", status, complete, state)
		}
	}
}

func TestFailedRebuildRollsBackIndexAndPreservesLastCompleteMetadata(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 1, "open", "preserved result", "body", "a", time.Unix(2, 0).UTC(), `{}`); err != nil {
		t.Fatal(err)
	}
	complete, err := c.RebuildThreadSearchProjection(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.ExecContext(ctx, `UPDATE projection_states SET status = ? WHERE name = ?`, ProjectionStatusStale, ProjectionNameThreadsFTS); err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.ExecContext(ctx, `
		CREATE TRIGGER reject_thread_projection_completion
		BEFORE UPDATE ON projection_states
		WHEN NEW.name = 'threads_fts' AND NEW.status = 'current' AND OLD.status = 'building'
		BEGIN SELECT RAISE(ABORT, 'injected projection failure'); END
	`); err != nil {
		t.Fatal(err)
	}

	if _, err := c.RebuildThreadSearchProjection(ctx); err == nil {
		t.Fatal("rebuild unexpectedly succeeded")
	}
	failed, err := c.GetProjectionState(ctx, ProjectionNameThreadsFTS)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != ProjectionStatusFailed || failed.AttemptStatus != ProjectionAttemptFailed || failed.AttemptError == "" {
		t.Fatalf("failed attempt metadata = %+v", failed)
	}
	if failed.Version != complete.Version || failed.ContentHash != complete.ContentHash || failed.RefreshedAt != complete.RefreshedAt || failed.RowCount != complete.RowCount {
		t.Fatalf("failed rebuild replaced last complete metadata: complete=%+v failed=%+v", complete, failed)
	}
	results, err := c.SearchThreads(ctx, "preserved", 10)
	if err != nil {
		t.Fatalf("search after failed rebuild: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("search after failed rebuild returned %d results", len(results))
	}
}

func TestCancelledRebuildPreservesLastCompleteProjection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 1, "open", "cancel-safe result", "body", "a", time.Unix(2, 0).UTC(), `{}`); err != nil {
		t.Fatal(err)
	}
	complete, err := c.RebuildThreadSearchProjection(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.ExecContext(ctx, `UPDATE projection_states SET status = ? WHERE name = ?`, ProjectionStatusStale, ProjectionNameThreadsFTS); err != nil {
		t.Fatal(err)
	}
	started, err := c.startSearchProjectionRebuild(ctx, ProjectionNameThreadsFTS)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := c.executeSearchProjectionRebuild(cancelled, ProjectionNameThreadsFTS, ProjectionVersionThreadsFTS, started); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled rebuild error = %v, want context.Canceled", err)
	}

	failed, err := c.GetProjectionState(ctx, ProjectionNameThreadsFTS)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != ProjectionStatusFailed || failed.AttemptStatus != ProjectionAttemptFailed {
		t.Fatalf("cancelled attempt metadata = %+v", failed)
	}
	if failed.Version != complete.Version || failed.ContentHash != complete.ContentHash || failed.RefreshedAt != complete.RefreshedAt {
		t.Fatalf("cancelled rebuild replaced last complete metadata: complete=%+v failed=%+v", complete, failed)
	}
	results, err := c.SearchThreads(ctx, "cancel-safe", 10)
	if err != nil || len(results) != 1 {
		t.Fatalf("search after cancelled rebuild = (%d results, %v)", len(results), err)
	}
}

func TestRebuildCodeSearchProjectionIsAtomicAndSetsState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	ref := domain.RepoRef{Owner: "owner", Repo: "repo"}
	snapshot := codeindex.Snapshot{
		RepoPath:   "/repo",
		Commit:     "abc",
		CreatedAt:  time.Unix(100, 0).UTC(),
		TotalBytes: 100,
		Documents: []codeindex.Document{
			{Path: "a.go", Content: "term alpha", Bytes: 10, LanguageHint: "go"},
			{Path: "b.go", Content: "term beta", Bytes: 9, LanguageHint: "go"},
		},
	}
	if _, _, err := c.StoreCodeSnapshot(ctx, ref, snapshot); err != nil {
		t.Fatalf("store code snapshot: %v", err)
	}

	state, err := c.RebuildCodeSearchProjection(ctx)
	if err != nil {
		t.Fatalf("rebuild code_documents_fts: %v", err)
	}
	if state.Name != ProjectionNameCodeDocumentsFTS || state.Version != ProjectionVersionCodeDocumentsFTS || state.Status != ProjectionStatusCurrent {
		t.Fatalf("rebuild state = %+v", state)
	}
	if state.RowCount != 2 {
		t.Fatalf("row_count = %d, want 2", state.RowCount)
	}

	page, err := c.SearchCodeWithOptions(ctx, "term", CodeSearchOptions{Ref: ref, Limit: 10})
	if err != nil {
		t.Fatalf("search code: %v", err)
	}
	if len(page.Matches) != 2 {
		t.Fatalf("code search matches = %d, want 2", len(page.Matches))
	}
}

func TestSearchDoesNotSilentlyRebuildProjections(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("apply repository: %v", err)
	}
	if _, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 1, "open", "rebuild term", "body", "a", time.Unix(2, 0).UTC(), `{}`); err != nil {
		t.Fatalf("apply thread: %v", err)
	}

	if _, err := c.db.ExecContext(ctx, `
		UPDATE projection_states
		SET status = ?, version = ?
		WHERE name = ?
	`, ProjectionStatusStale, "threads-fts-v2", ProjectionNameThreadsFTS); err != nil {
		t.Fatalf("mark projection stale: %v", err)
	}

	stale, err := c.GetProjectionState(ctx, ProjectionNameThreadsFTS)
	if err != nil {
		t.Fatalf("get stale state: %v", err)
	}
	if stale.Status != ProjectionStatusStale || stale.Version != "threads-fts-v2" {
		t.Fatalf("stale state = %+v", stale)
	}

	// Search reports stale state and never repairs it implicitly.
	_, err = c.SearchThreads(ctx, "rebuild", 10)
	if !errors.Is(err, ErrProjectionStale) {
		t.Fatalf("search threads error = %v, want ErrProjectionStale", err)
	}

	afterSearch, err := c.GetProjectionState(ctx, ProjectionNameThreadsFTS)
	if err != nil {
		t.Fatalf("get state after search: %v", err)
	}
	if afterSearch.Status != ProjectionStatusStale || afterSearch.Version != "threads-fts-v2" {
		t.Fatalf("search silently changed projection state: %+v", afterSearch)
	}

	rebuilt, err := c.RebuildThreadSearchProjection(ctx)
	if err != nil {
		t.Fatalf("rebuild threads_fts: %v", err)
	}
	if rebuilt.Status != ProjectionStatusCurrent || rebuilt.Version != ProjectionVersionThreadsFTS {
		t.Fatalf("rebuilt state = %+v", rebuilt)
	}
}

func TestRebuildThreadSearchProjectionRestoresClearedIndex(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("apply repository: %v", err)
	}
	if _, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 1, "open", "lost term", "body", "a", time.Unix(2, 0).UTC(), `{}`); err != nil {
		t.Fatalf("apply thread: %v", err)
	}

	// Clear the FTS index manually and mark the projection stale.
	if _, err := c.db.ExecContext(ctx, `
		INSERT INTO threads_fts (threads_fts, rowid, title, body)
		SELECT 'delete', id, title, COALESCE(body, '')
		FROM threads
	`); err != nil {
		t.Fatalf("clear threads_fts: %v", err)
	}
	if _, err := c.db.ExecContext(ctx, `
		UPDATE projection_states SET status = ? WHERE name = ?
	`, ProjectionStatusStale, ProjectionNameThreadsFTS); err != nil {
		t.Fatalf("mark stale: %v", err)
	}

	if _, err := c.SearchThreads(ctx, "lost", 10); !errors.Is(err, ErrProjectionStale) {
		t.Fatalf("search after clear error = %v, want ErrProjectionStale", err)
	}

	if _, err := c.RebuildThreadSearchProjection(ctx); err != nil {
		t.Fatalf("rebuild threads_fts: %v", err)
	}

	restored, err := c.SearchThreads(ctx, "lost", 10)
	if err != nil {
		t.Fatalf("search after rebuild: %v", err)
	}
	if len(restored) != 1 {
		t.Fatalf("search after rebuild = %d, want 1", len(restored))
	}

	rebuilt, err := c.GetProjectionState(ctx, ProjectionNameThreadsFTS)
	if err != nil {
		t.Fatalf("get state after rebuild: %v", err)
	}
	if rebuilt.Status != ProjectionStatusCurrent || rebuilt.RowCount != 1 {
		t.Fatalf("rebuilt state = %+v", rebuilt)
	}
}
