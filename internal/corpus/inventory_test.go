package corpus

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/domain"
)

func TestCodeSnapshotPruneRejectsStalePlan(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	ref := domain.RepoRef{Owner: "owner", Repo: "repo"}
	for i, commit := range []string{"one", "two"} {
		snapshot := codeindex.Snapshot{RepoPath: "/repo", Commit: commit, CreatedAt: time.Unix(int64(i+1), 0), TotalBytes: 1, Documents: []codeindex.Document{{Path: commit, Content: commit, Bytes: 1}}}
		if _, _, err := c.StoreCodeSnapshot(ctx, ref, snapshot); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := c.PlanCodeSnapshotPrune(ctx, ref, 1)
	if err != nil {
		t.Fatal(err)
	}
	newer := codeindex.Snapshot{RepoPath: "/repo", Commit: "three", CreatedAt: time.Unix(3, 0), TotalBytes: 1, Documents: []codeindex.Document{{Path: "three", Content: "three", Bytes: 1}}}
	if _, _, err := c.StoreCodeSnapshot(ctx, ref, newer); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ApplyCodeSnapshotPrune(ctx, ref, plan); !errors.Is(err, ErrCodeSnapshotPrunePlanStale) {
		t.Fatalf("ApplyCodeSnapshotPrune error = %v, want stale plan", err)
	}
}

func TestRepositoryInventoryCountsAndSizes(t *testing.T) {
	c, _ := openTestCorpus(t)
	ctx := context.Background()

	owner, name := "owner", "repo"
	ref := domain.RepoRef{Owner: owner, Repo: name}

	repo, err := c.ApplyRepositoryObservation(ctx, owner, name, "1", time.Unix(1, 0).UTC(), `{}`)
	requireInventorySetup(t, "apply repository", err)
	if _, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 1, "open", "issue one", "body", "a", time.Unix(10, 0).UTC(), `{}`); err != nil {
		t.Fatalf("apply issue 1: %v", err)
	}
	if _, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 2, "open", "issue two", "body", "a", time.Unix(11, 0).UTC(), `{}`); err != nil {
		t.Fatalf("apply issue 2: %v", err)
	}
	if _, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindPullRequest, 3, "open", "pr one", "body", "a", time.Unix(12, 0).UTC(), `{}`); err != nil {
		t.Fatalf("apply pr: %v", err)
	}

	run, err := c.StartRun(ctx, "sync")
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if err := c.ApplyFacetObservationSet(ctx, repo.ID, nil, "metadata", time.Unix(20, 0).UTC(), []FacetObservationInput{
		{SourceUpdatedAt: time.Unix(18, 0).UTC(), Payload: `{"page":1}`},
		{SourceUpdatedAt: time.Unix(20, 0).UTC(), Payload: `{"page":2}`},
	}, true, run.ID); err != nil {
		t.Fatalf("apply facet observation set: %v", err)
	}

	first := codeindex.Snapshot{
		RepoPath: "/repo", Commit: "first", CreatedAt: time.Unix(100, 0).UTC(), TotalBytes: 13,
		Documents: []codeindex.Document{{Path: "old.go", Content: "legacy needle", Bytes: 13, LanguageHint: "go"}},
	}
	if _, _, err := c.StoreCodeSnapshot(ctx, ref, first); err != nil {
		t.Fatalf("store first snapshot: %v", err)
	}
	second := codeindex.Snapshot{
		RepoPath: "/repo", Commit: "second", CreatedAt: time.Unix(200, 0).UTC(), TotalBytes: 24,
		Documents: []codeindex.Document{
			{Path: "new.go", Content: "current needle", Bytes: 10, LanguageHint: "go"},
			{Path: "other.go", Content: "other", Bytes: 14, LanguageHint: "go"},
		},
	}
	if _, _, err := c.StoreCodeSnapshot(ctx, ref, second); err != nil {
		t.Fatalf("store second snapshot: %v", err)
	}

	inv, err := c.Inventory(ctx, owner, name)
	if err != nil {
		t.Fatalf("inventory: %v", err)
	}
	if inv == nil {
		t.Fatal("inventory is nil")
	}

	if inv.Issues != 2 {
		t.Fatalf("Issues = %d, want 2", inv.Issues)
	}
	if inv.PullRequests != 1 {
		t.Fatalf("PullRequests = %d, want 1", inv.PullRequests)
	}
	if inv.Threads != 3 {
		t.Fatalf("Threads = %d, want 3", inv.Threads)
	}
	if inv.RepositoryObservations != 1 {
		t.Fatalf("RepositoryObservations = %d, want 1", inv.RepositoryObservations)
	}
	if inv.ThreadObservations != 3 {
		t.Fatalf("ThreadObservations = %d, want 3", inv.ThreadObservations)
	}
	if inv.FacetObservations != 2 {
		t.Fatalf("FacetObservations = %d, want 2", inv.FacetObservations)
	}
	if inv.FacetCoverage != 1 {
		t.Fatalf("FacetCoverage = %d, want 1", inv.FacetCoverage)
	}
	if inv.TotalObservations != 6 {
		t.Fatalf("TotalObservations = %d, want 6", inv.TotalObservations)
	}
	if inv.CodeSnapshots != 2 {
		t.Fatalf("CodeSnapshots = %d, want 2", inv.CodeSnapshots)
	}
	if inv.CodeDocuments != 3 {
		t.Fatalf("CodeDocuments = %d, want 3", inv.CodeDocuments)
	}
	if inv.CodeBytes != 37 {
		t.Fatalf("CodeBytes = %d, want 37", inv.CodeBytes)
	}
	if inv.DBSize == 0 {
		t.Fatal("DBSize is zero")
	}
	if inv.TotalSize < inv.DBSize {
		t.Fatalf("TotalSize = %d, want >= DBSize %d", inv.TotalSize, inv.DBSize)
	}

	// Inventory for a missing repository returns a typed absence.
	missing, err := c.Inventory(ctx, "missing", "repo")
	if !errors.Is(err, ErrRepositoryNotFound) {
		t.Fatalf("inventory missing error = %v", err)
	}
	if missing != nil {
		t.Fatalf("missing inventory = %+v, want nil", missing)
	}
}

func TestListInventoryAggregatesEveryRepositoryScopeAndFreshness(t *testing.T) {
	c, _ := openTestCorpus(t)
	ctx := context.Background()

	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "observed", "1", time.Unix(10, 0).UTC(), `{"repo":true}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindPullRequest, 1, "open", "pr", "body", "author", time.Unix(20, 0).UTC(), `{"thread":true}`); err != nil {
		t.Fatal(err)
	}
	run, err := c.StartRun(ctx, "sync")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.ApplyFacetObservationSet(ctx, repo.ID, nil, "metadata", time.Unix(30, 0).UTC(), []FacetObservationInput{{
		SourceUpdatedAt: time.Unix(30, 0).UTC(), Payload: `{"facet":true}`,
	}}, true, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.ExecContext(ctx, `UPDATE repository_observations SET observed_at = ?`, encodeTime(time.Unix(10, 0).UTC())); err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.ExecContext(ctx, `UPDATE thread_observations SET observed_at = ?`, encodeTime(time.Unix(20, 0).UTC())); err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.ExecContext(ctx, `UPDATE facet_observations SET observed_at = ?`, encodeTime(time.Unix(30, 0).UTC())); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.StoreCodeSnapshot(ctx, domain.RepoRef{Owner: "code", Repo: "only"}, codeindex.Snapshot{
		RepoPath: "/code", Commit: "abc", CreatedAt: time.Unix(40, 0).UTC(), TotalBytes: 7,
		Documents: []codeindex.Document{{Path: "main.go", Content: "package", Bytes: 7}},
	}); err != nil {
		t.Fatal(err)
	}

	inv, err := c.ListInventory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Repositories) != 2 {
		t.Fatalf("repositories = %+v, want observed and code-only scopes", inv.Repositories)
	}
	if inv.Repositories[0].RepoOwner != "code" || inv.Repositories[0].CodeSnapshots != 1 || inv.Repositories[0].Threads != 0 {
		t.Fatalf("code-only inventory = %+v", inv.Repositories[0])
	}
	observed := inv.Repositories[1]
	if observed.RepoName != "observed" || observed.PullRequests != 1 || observed.TotalObservations != 3 {
		t.Fatalf("observed inventory = %+v", observed)
	}
	if got := observed.LatestObservationAt; !got.Equal(time.Unix(30, 0).UTC()) {
		t.Fatalf("latest observation = %s, want %s", got, time.Unix(30, 0).UTC())
	}
	if inv.ObservationPayloadBytes == 0 || inv.CodeBytes != 7 || inv.DBSize == 0 || inv.TotalSize < inv.DBSize {
		t.Fatalf("corpus inventory totals = %+v", inv)
	}
}

func TestCodeSnapshotPrunePreservesLatestN(t *testing.T) {
	c, _ := openTestCorpus(t)
	ctx := context.Background()

	owner, name := "owner", "repo"
	ref := domain.RepoRef{Owner: owner, Repo: name}

	repo, err := c.ApplyRepositoryObservation(ctx, owner, name, "1", time.Unix(1, 0).UTC(), `{}`)
	requireInventorySetup(t, "apply repository", err)
	if _, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 1, "open", "issue", "body", "a", time.Unix(10, 0).UTC(), `{}`); err != nil {
		t.Fatalf("apply issue: %v", err)
	}

	first := codeindex.Snapshot{
		RepoPath: "/repo", Commit: "first", CreatedAt: time.Unix(100, 0).UTC(), TotalBytes: 13,
		Documents: []codeindex.Document{{Path: "old.go", Content: "legacy needle", Bytes: 13, LanguageHint: "go"}},
	}
	if _, _, err := c.StoreCodeSnapshot(ctx, ref, first); err != nil {
		t.Fatalf("store first snapshot: %v", err)
	}
	second := codeindex.Snapshot{
		RepoPath: "/repo", Commit: "second", CreatedAt: time.Unix(200, 0).UTC(), TotalBytes: 24,
		Documents: []codeindex.Document{{Path: "new.go", Content: "current needle", Bytes: 24, LanguageHint: "go"}},
	}
	if _, _, err := c.StoreCodeSnapshot(ctx, ref, second); err != nil {
		t.Fatalf("store second snapshot: %v", err)
	}

	plan, err := c.PlanCodeSnapshotPrune(ctx, ref, 1)
	if err != nil {
		t.Fatalf("plan prune: %v", err)
	}
	if plan.TotalSnapshots != 2 {
		t.Fatalf("plan.TotalSnapshots = %d, want 2", plan.TotalSnapshots)
	}
	if len(plan.Keep) != 1 || plan.Keep[0].CommitSHA != "second" {
		t.Fatalf("plan.Keep = %+v, want [second]", plan.Keep)
	}
	if len(plan.Delete) != 1 || plan.Delete[0].CommitSHA != "first" {
		t.Fatalf("plan.Delete = %+v, want [first]", plan.Delete)
	}
	if plan.ReclaimBytes != 13 {
		t.Fatalf("plan.ReclaimBytes = %d, want 13", plan.ReclaimBytes)
	}

	result, err := c.ApplyCodeSnapshotPrune(ctx, ref, plan)
	if err != nil {
		t.Fatalf("apply prune: %v", err)
	}
	if result.Deleted != 1 || result.ReclaimBytes != 13 {
		t.Fatalf("result = %+v, want Deleted=1 ReclaimBytes=13", result)
	}

	inv, err := c.Inventory(ctx, owner, name)
	if err != nil {
		t.Fatalf("inventory after prune: %v", err)
	}
	if inv.CodeSnapshots != 1 || inv.CodeDocuments != 1 || inv.CodeBytes != 24 {
		t.Fatalf("inventory after prune = %+v", inv)
	}

	// Observations must not be affected by derived snapshot pruning.
	if inv.RepositoryObservations != 1 || inv.ThreadObservations != 1 {
		t.Fatalf("observations changed after prune = %+v", inv)
	}

	// The FTS index must reflect the deletion.
	matches, err := c.SearchCode(ctx, "legacy", ref, 10)
	if err != nil {
		t.Fatalf("search legacy: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("legacy matches = %d after prune, want 0: %+v", len(matches), matches)
	}
	matches, err = c.SearchCode(ctx, "current", ref, 10)
	if err != nil {
		t.Fatalf("search current: %v", err)
	}
	if len(matches) != 1 || matches[0].Commit != "second" {
		t.Fatalf("current matches = %+v", matches)
	}
}

func requireInventorySetup(t *testing.T, action string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", action, err)
	}
}

func TestCodeSnapshotPruneRequiresExactRepoScope(t *testing.T) {
	c, _ := openTestCorpus(t)
	ctx := context.Background()

	refA := domain.RepoRef{Owner: "owner", Repo: "repo"}
	first := codeindex.Snapshot{
		RepoPath: "/repo", Commit: "first", CreatedAt: time.Unix(100, 0).UTC(), TotalBytes: 13,
		Documents: []codeindex.Document{{Path: "a.go", Content: "needle", Bytes: 13, LanguageHint: "go"}},
	}
	if _, _, err := c.StoreCodeSnapshot(ctx, refA, first); err != nil {
		t.Fatalf("store snapshot: %v", err)
	}

	plan, err := c.PlanCodeSnapshotPrune(ctx, refA, 0)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	refB := domain.RepoRef{Owner: "other", Repo: "repo"}
	if _, err := c.ApplyCodeSnapshotPrune(ctx, refB, plan); err == nil || !strings.Contains(err.Error(), "scope") {
		t.Fatalf("apply with mismatched scope should fail, got: %v", err)
	}
}
