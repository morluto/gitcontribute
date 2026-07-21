package corpus

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/domain"
)

func TestRepositoryRemovalDryRunAndExactScope(t *testing.T) {
	c, _ := openTestCorpus(t)
	ctx := context.Background()
	target := domain.RepoRef{Owner: "owner", Repo: "target"}
	other := domain.RepoRef{Owner: "owner", Repo: "other"}
	targetRepo, targetThread := seedRemovalRepository(ctx, t, c, target, 1)
	_, otherThread := seedRemovalRepository(ctx, t, c, other, 2)

	_, _, err := c.StoreCodeSnapshot(ctx, target, codeindex.Snapshot{RepoPath: "/target", Commit: "target-sha", TotalBytes: 6, CreatedAt: time.Unix(3, 0), Documents: []codeindex.Document{{Path: "target.go", Content: "target", Bytes: 6}}})
	requireRemovalSetup(t, "store target snapshot", err)
	if _, _, err := c.StoreCodeSnapshot(ctx, other, codeindex.Snapshot{RepoPath: "/other", Commit: "other-sha", TotalBytes: 5, CreatedAt: time.Unix(4, 0), Documents: []codeindex.Document{{Path: "other.go", Content: "other", Bytes: 5}}}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixNano()
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO investigations (id, repo_owner, repo_name, status, payload, created_at, updated_at) VALUES ('target-investigation', ?, ?, 'open', '{}', ?, ?)`, []any{target.Owner, target.Repo, now, now}},
		{`INSERT INTO hypotheses (id, investigation_id, category, status, payload, created_at, updated_at) VALUES ('target-hypothesis', 'target-investigation', 'bug', 'promoted', '{}', ?, ?)`, []any{now, now}},
		{`INSERT INTO opportunities (id, investigation_id, hypothesis_id, category, status, payload, created_at, updated_at) VALUES ('target-opportunity', 'target-investigation', 'target-hypothesis', 'bug', 'validated', '{}', ?, ?)`, []any{now, now}},
		{`INSERT INTO portfolio_links (pull_request_thread_id, opportunity_id, created_at) VALUES (?, 'target-opportunity', ?)`, []any{otherThread, now}},
		{`INSERT INTO cluster_runs (repo_owner, repo_name, source_revision, source_window_start, source_window_end, status, started_at, governance_revision, rule_version, statistics_json) VALUES (?, ?, 'rev', 0, 1, 'completed', ?, 0, 'v1', '{}')`, []any{other.Owner, other.Repo, now}},
		{`INSERT INTO clusters (stable_id, repo_owner, repo_name, state, canonical_kind, canonical_owner, canonical_repo, canonical_number, source_revision, source_window_start, source_window_end, created_at, updated_at) VALUES ('other-cluster', ?, ?, 'active', 'issue', ?, ?, 2, 'rev', 0, 1, ?, ?)`, []any{other.Owner, other.Repo, other.Owner, other.Repo, now, now}},
	}
	for _, statement := range statements {
		_, err := c.db.ExecContext(ctx, statement.query, statement.args...)
		requireRemovalSetup(t, "seed workflow record", err)
	}
	var clusterID int64
	requireRemovalSetup(t, "read seeded cluster", c.db.QueryRowContext(ctx, `SELECT id FROM clusters WHERE stable_id = 'other-cluster'`).Scan(&clusterID))
	_, err = c.db.ExecContext(ctx, `INSERT INTO cluster_members (cluster_id, thread_id, kind, owner, repo, number, title, state, score, reason, created_at, updated_at) VALUES (?, ?, 'issue', ?, ?, 1, 'target', 'open', 0.9, 'shared', ?, ?)`, clusterID, targetThread, target.Owner, target.Repo, now, now)
	requireRemovalSetup(t, "seed cluster member", err)

	plan, err := c.PlanRepositoryRemoval(ctx, target)
	requireRemovalSetup(t, "plan repository removal", err)
	if plan == nil || plan.RepositoryID != targetRepo || plan.Threads != 1 || plan.CodeSnapshots != 1 || plan.PreservedInvestigations != 1 || plan.PreservedCrossRepoReferences != 1 {
		t.Fatalf("plan = %+v", plan)
	}
	// Planning is non-mutating.
	if inventory, err := c.Inventory(ctx, target.Owner, target.Repo); err != nil || inventory == nil {
		t.Fatalf("target inventory after plan = (%+v, %v)", inventory, err)
	}

	_, err = c.ApplyRepositoryRemoval(ctx, target, plan)
	requireRemovalSetup(t, "apply repository removal", err)
	if inventory, err := c.Inventory(ctx, target.Owner, target.Repo); !errors.Is(err, ErrRepositoryNotFound) || inventory != nil {
		t.Fatalf("target inventory after removal = (%+v, %v)", inventory, err)
	}
	if inventory, err := c.Inventory(ctx, other.Owner, other.Repo); err != nil || inventory == nil || inventory.Threads != 1 || inventory.CodeSnapshots != 1 {
		t.Fatalf("other inventory after removal = (%+v, %v)", inventory, err)
	}
	var preserved int
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM investigations WHERE id = 'target-investigation'`).Scan(&preserved); err != nil || preserved != 1 {
		t.Fatalf("preserved investigation = %d, err=%v", preserved, err)
	}
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM portfolio_links WHERE pull_request_thread_id = ? AND opportunity_id = 'target-opportunity'`, otherThread).Scan(&preserved); err != nil || preserved != 1 {
		t.Fatalf("preserved cross-repository workflow link = %d, err=%v", preserved, err)
	}
	var detached sql.NullInt64
	if err := c.db.QueryRowContext(ctx, `SELECT thread_id FROM cluster_members WHERE cluster_id = ?`, clusterID).Scan(&detached); err != nil || detached.Valid {
		t.Fatalf("cross-repository cluster member thread = %+v, err=%v", detached, err)
	}
	var otherThreadCount int
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM threads WHERE id = ?`, otherThread).Scan(&otherThreadCount); err != nil || otherThreadCount != 1 {
		t.Fatalf("other thread count = %d, err=%v", otherThreadCount, err)
	}
}

func requireRemovalSetup(t *testing.T, action string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", action, err)
	}
}

func TestRepositoryRemovalRejectsStalePlan(t *testing.T) {
	c, _ := openTestCorpus(t)
	ctx := context.Background()
	ref := domain.RepoRef{Owner: "owner", Repo: "target"}
	repoID, _ := seedRemovalRepository(ctx, t, c, ref, 1)
	plan, err := c.PlanRepositoryRemoval(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ApplyThreadObservation(ctx, repoID, ThreadKindIssue, 2, "open", "new", "", "author", time.Unix(3, 0), `{}`); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ApplyRepositoryRemoval(ctx, ref, plan); !errors.Is(err, ErrRepositoryRemovalPlanStale) {
		t.Fatalf("ApplyRepositoryRemoval error = %v, want stale plan", err)
	}
	if inventory, err := c.Inventory(ctx, ref.Owner, ref.Repo); err != nil || inventory == nil || inventory.Threads != 2 {
		t.Fatalf("inventory after rejected plan = (%+v, %v)", inventory, err)
	}
}

func TestRepositoryRemovalRejectsSameCountReplacement(t *testing.T) {
	c, _ := openTestCorpus(t)
	ctx := context.Background()
	ref := domain.RepoRef{Owner: "owner", Repo: "target"}
	seedRemovalRepository(ctx, t, c, ref, 1)
	if _, _, err := c.StoreCodeSnapshot(ctx, ref, codeindex.Snapshot{RepoPath: "/target", Commit: "old", TotalBytes: 3, CreatedAt: time.Unix(3, 0), Documents: []codeindex.Document{{Path: "old.go", Content: "old", Bytes: 3}}}); err != nil {
		t.Fatal(err)
	}
	plan, err := c.PlanRepositoryRemoval(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.ExecContext(ctx, `DELETE FROM code_snapshots WHERE repo_owner = ? AND repo_name = ?`, ref.Owner, ref.Repo); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.StoreCodeSnapshot(ctx, ref, codeindex.Snapshot{RepoPath: "/target", Commit: "new", TotalBytes: 3, CreatedAt: time.Unix(4, 0), Documents: []codeindex.Document{{Path: "new.go", Content: "new", Bytes: 3}}}); err != nil {
		t.Fatal(err)
	}

	if _, err := c.ApplyRepositoryRemoval(ctx, ref, plan); !errors.Is(err, ErrRepositoryRemovalPlanStale) {
		t.Fatalf("ApplyRepositoryRemoval error = %v, want stale plan", err)
	}
	if inventory, err := c.Inventory(ctx, ref.Owner, ref.Repo); err != nil || inventory == nil || inventory.CodeSnapshots != 1 {
		t.Fatalf("inventory after rejected plan = (%+v, %v)", inventory, err)
	}
}

func TestRepositoryRemovalCancellationRollsBack(t *testing.T) {
	c, _ := openTestCorpus(t)
	ref := domain.RepoRef{Owner: "owner", Repo: "target"}
	seedRemovalRepository(context.Background(), t, c, ref, 1)
	plan, err := c.PlanRepositoryRemoval(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.ApplyRepositoryRemoval(ctx, ref, plan); !errors.Is(err, context.Canceled) {
		t.Fatalf("ApplyRepositoryRemoval error = %v, want context canceled", err)
	}
	if inventory, err := c.Inventory(context.Background(), ref.Owner, ref.Repo); err != nil || inventory == nil || inventory.Threads != 1 {
		t.Fatalf("inventory after cancellation = (%+v, %v)", inventory, err)
	}
}

func seedRemovalRepository(ctx context.Context, t *testing.T, c *Corpus, ref domain.RepoRef, number int) (int64, int64) {
	t.Helper()
	repo, err := c.ApplyRepositoryObservation(ctx, ref.Owner, ref.Repo, ref.String(), time.Unix(int64(number), 0), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, number, "open", ref.String(), "body", "author", time.Unix(int64(number+1), 0), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	return repo.ID, thread.ID
}
