package corpus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

// RepositoryInventory is a read-only summary of one repository in the corpus.
// It counts durable observations, derived projections, and code snapshots.
type RepositoryInventory struct {
	RepoOwner string
	RepoName  string

	Issues       int
	PullRequests int
	Threads      int

	RepositoryObservations int
	ThreadObservations     int
	FacetObservations      int
	FacetCoverage          int
	TotalObservations      int

	CodeSnapshots int
	CodeDocuments int
	CodeBytes     int64

	LatestObservationAt time.Time

	DBSize    int64
	WALSize   int64
	TotalSize int64
}

// CorpusInventory is a bounded, read-only summary of every repository-shaped
// scope in the corpus. Repository rows and code-only scopes are both included;
// individual observations and documents are never returned.
type CorpusInventory struct {
	Repositories []RepositoryInventory

	ObservationPayloadBytes int64
	CodeBytes               int64
	DBSize                  int64
	WALSize                 int64
	TotalSize               int64
}

// ListInventory aggregates the corpus by repository. Its result is bounded to
// one row per stored repository or code-index scope, while the SQL performs
// grouped scans rather than loading unbounded observation detail into memory.
func (c *Corpus) ListInventory(ctx context.Context) (*CorpusInventory, error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT owner, name FROM repositories
		UNION
		SELECT repo_owner, repo_name FROM code_snapshots
		ORDER BY 1, 2
	`)
	if err != nil {
		return nil, fmt.Errorf("inventory list repository scopes: %w", err)
	}
	defer rows.Close()

	out := &CorpusInventory{}
	byRef := make(map[string]*RepositoryInventory)
	for rows.Next() {
		var inv RepositoryInventory
		if err := rows.Scan(&inv.RepoOwner, &inv.RepoName); err != nil {
			return nil, fmt.Errorf("inventory scan repository scope: %w", err)
		}
		out.Repositories = append(out.Repositories, inv)
		item := &out.Repositories[len(out.Repositories)-1]
		byRef[inventoryRefKey(item.RepoOwner, item.RepoName)] = item
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("inventory list repository scopes: %w", err)
	}

	// Appending above can reallocate the slice and invalidate stored pointers.
	// Rebuild the lookup only after the complete scope list is known.
	byRef = make(map[string]*RepositoryInventory, len(out.Repositories))
	for i := range out.Repositories {
		item := &out.Repositories[i]
		byRef[inventoryRefKey(item.RepoOwner, item.RepoName)] = item
	}

	if err := c.readRepositoryInventoryAggregates(ctx, byRef, out); err != nil {
		return nil, err
	}
	dbPath, err := c.databasePath(ctx)
	if err != nil {
		return nil, err
	}
	out.DBSize, out.WALSize, err = corpusFileSizes(dbPath)
	if err != nil {
		return nil, err
	}
	out.TotalSize = out.DBSize + out.WALSize
	return out, nil
}

func inventoryRefKey(owner, name string) string { return owner + "\x00" + name }

func (c *Corpus) readRepositoryInventoryAggregates(ctx context.Context, byRef map[string]*RepositoryInventory, out *CorpusInventory) error {
	rows, err := c.db.QueryContext(ctx, `
		SELECT r.owner, r.name,
		       COALESCE(SUM(CASE WHEN t.kind = ? THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN t.kind = ? THEN 1 ELSE 0 END), 0),
		       COUNT(t.id)
		FROM repositories r
		LEFT JOIN threads t ON t.repository_id = r.id
		GROUP BY r.id, r.owner, r.name
	`, ThreadKindIssue, ThreadKindPullRequest)
	if err != nil {
		return fmt.Errorf("inventory aggregate threads: %w", err)
	}
	for rows.Next() {
		var owner, name string
		var issues, prs, threads int
		if err := rows.Scan(&owner, &name, &issues, &prs, &threads); err != nil {
			rows.Close()
			return fmt.Errorf("inventory scan thread aggregates: %w", err)
		}
		inv := byRef[inventoryRefKey(owner, name)]
		inv.Issues, inv.PullRequests, inv.Threads = issues, prs, threads
	}
	if err := rows.Close(); err != nil {
		return err
	}

	type observationQuery struct {
		query string
		apply func(*RepositoryInventory, int)
	}
	queries := []observationQuery{
		{`SELECT r.owner, r.name, COUNT(o.id), COALESCE(MAX(o.observed_at), 0), COALESCE(SUM(length(CAST(o.payload AS BLOB))), 0) FROM repositories r LEFT JOIN repository_observations o ON o.repository_id = r.id GROUP BY r.id, r.owner, r.name`, func(inv *RepositoryInventory, count int) { inv.RepositoryObservations = count }},
		{`SELECT r.owner, r.name, COUNT(o.id), COALESCE(MAX(o.observed_at), 0), COALESCE(SUM(length(CAST(o.payload AS BLOB))), 0) FROM repositories r LEFT JOIN threads t ON t.repository_id = r.id LEFT JOIN thread_observations o ON o.thread_id = t.id GROUP BY r.id, r.owner, r.name`, func(inv *RepositoryInventory, count int) { inv.ThreadObservations = count }},
		{`SELECT r.owner, r.name, COUNT(o.id), COALESCE(MAX(o.observed_at), 0), COALESCE(SUM(length(CAST(o.payload AS BLOB))), 0) FROM repositories r LEFT JOIN facet_observations o ON o.repository_id = r.id GROUP BY r.id, r.owner, r.name`, func(inv *RepositoryInventory, count int) { inv.FacetObservations = count }},
	}
	for _, query := range queries {
		rows, err = c.db.QueryContext(ctx, query.query)
		if err != nil {
			return fmt.Errorf("inventory aggregate observations: %w", err)
		}
		for rows.Next() {
			var owner, name string
			var count int
			var latest, payloadBytes int64
			if err := rows.Scan(&owner, &name, &count, &latest, &payloadBytes); err != nil {
				rows.Close()
				return fmt.Errorf("inventory scan observation aggregates: %w", err)
			}
			inv := byRef[inventoryRefKey(owner, name)]
			query.apply(inv, count)
			if latest > 0 && (inv.LatestObservationAt.IsZero() || latest > inv.LatestObservationAt.UnixNano()) {
				inv.LatestObservationAt = time.Unix(0, latest).UTC()
			}
			out.ObservationPayloadBytes += payloadBytes
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}

	rows, err = c.db.QueryContext(ctx, `SELECT r.owner, r.name, COUNT(fc.id) FROM repositories r LEFT JOIN facet_coverage fc ON fc.repository_id = r.id GROUP BY r.id, r.owner, r.name`)
	if err != nil {
		return fmt.Errorf("inventory aggregate facet coverage: %w", err)
	}
	for rows.Next() {
		var owner, name string
		var count int
		if err := rows.Scan(&owner, &name, &count); err != nil {
			rows.Close()
			return err
		}
		byRef[inventoryRefKey(owner, name)].FacetCoverage = count
	}
	if err := rows.Close(); err != nil {
		return err
	}

	rows, err = c.db.QueryContext(ctx, `
		SELECT s.repo_owner, s.repo_name, COUNT(DISTINCT s.id), COUNT(d.id), COALESCE(SUM(d.bytes), 0)
		FROM code_snapshots s LEFT JOIN code_documents d ON d.snapshot_id = s.id
		GROUP BY s.repo_owner, s.repo_name
	`)
	if err != nil {
		return fmt.Errorf("inventory aggregate code: %w", err)
	}
	for rows.Next() {
		var owner, name string
		var snapshots, documents int
		var codeBytes int64
		if err := rows.Scan(&owner, &name, &snapshots, &documents, &codeBytes); err != nil {
			rows.Close()
			return err
		}
		inv := byRef[inventoryRefKey(owner, name)]
		inv.CodeSnapshots, inv.CodeDocuments, inv.CodeBytes = snapshots, documents, codeBytes
		out.CodeBytes += codeBytes
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for i := range out.Repositories {
		inv := &out.Repositories[i]
		inv.TotalObservations = inv.RepositoryObservations + inv.ThreadObservations + inv.FacetObservations
	}
	return nil
}

// Inventory returns a read-only inventory for the named repository.
// It returns nil, nil when the repository has not been observed.
func (c *Corpus) Inventory(ctx context.Context, owner, name string) (*RepositoryInventory, error) {
	var repoID int64
	err := c.db.QueryRowContext(ctx, `SELECT id FROM repositories WHERE owner = ? AND name = ?`, owner, name).Scan(&repoID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inventory select repository: %w", err)
	}

	inv := &RepositoryInventory{RepoOwner: owner, RepoName: name}

	if err := c.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN kind = ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN kind = ? THEN 1 ELSE 0 END), 0),
			COUNT(*)
		FROM threads WHERE repository_id = ?
	`, ThreadKindIssue, ThreadKindPullRequest, repoID).Scan(&inv.Issues, &inv.PullRequests, &inv.Threads); err != nil {
		return nil, fmt.Errorf("inventory count threads: %w", err)
	}

	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM repository_observations WHERE repository_id = ?`, repoID).Scan(&inv.RepositoryObservations); err != nil {
		return nil, fmt.Errorf("inventory count repository observations: %w", err)
	}
	if err := c.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM thread_observations
		WHERE thread_id IN (SELECT id FROM threads WHERE repository_id = ?)
	`, repoID).Scan(&inv.ThreadObservations); err != nil {
		return nil, fmt.Errorf("inventory count thread observations: %w", err)
	}
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM facet_observations WHERE repository_id = ?`, repoID).Scan(&inv.FacetObservations); err != nil {
		return nil, fmt.Errorf("inventory count facet observations: %w", err)
	}
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM facet_coverage WHERE repository_id = ?`, repoID).Scan(&inv.FacetCoverage); err != nil {
		return nil, fmt.Errorf("inventory count facet coverage: %w", err)
	}
	inv.TotalObservations = inv.RepositoryObservations + inv.ThreadObservations + inv.FacetObservations

	if err := c.db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT s.id), COUNT(d.id), COALESCE(SUM(d.bytes), 0)
		FROM code_snapshots s
		LEFT JOIN code_documents d ON d.snapshot_id = s.id
		WHERE s.repo_owner = ? AND s.repo_name = ?
	`, owner, name).Scan(&inv.CodeSnapshots, &inv.CodeDocuments, &inv.CodeBytes); err != nil {
		return nil, fmt.Errorf("inventory count code snapshots: %w", err)
	}

	dbPath, err := c.databasePath(ctx)
	if err != nil {
		return nil, err
	}
	dbSize, walSize, err := corpusFileSizes(dbPath)
	if err != nil {
		return nil, err
	}
	inv.DBSize = dbSize
	inv.WALSize = walSize
	inv.TotalSize = dbSize + walSize

	return inv, nil
}

func (c *Corpus) databasePath(ctx context.Context) (string, error) {
	rows, err := c.db.QueryContext(ctx, `PRAGMA database_list`)
	if err != nil {
		return "", fmt.Errorf("inventory pragma database_list: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var seq int
		var name, file string
		if err := rows.Scan(&seq, &name, &file); err != nil {
			return "", fmt.Errorf("inventory scan database_list: %w", err)
		}
		if name == "main" {
			return file, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("inventory read database_list: %w", err)
	}
	return "", nil
}

func corpusFileSizes(dbPath string) (dbSize, walSize int64, err error) {
	if dbPath == "" || dbPath == ":memory:" {
		return 0, 0, nil
	}
	info, err := os.Stat(dbPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, fmt.Errorf("inventory stat db: %w", err)
	}
	dbSize = info.Size()
	walInfo, err := os.Stat(dbPath + "-wal")
	if err != nil {
		if !os.IsNotExist(err) {
			return 0, 0, fmt.Errorf("inventory stat wal: %w", err)
		}
		walSize = 0
	} else {
		walSize = walInfo.Size()
	}
	return dbSize, walSize, nil
}

// CodeSnapshotRef identifies one stored code snapshot for retention planning.
type CodeSnapshotRef struct {
	ID         int64
	CommitSHA  string
	CreatedAt  time.Time
	TotalBytes int64
}

// CodeSnapshotPrunePlan is a dry-run plan for pruning derived code snapshots.
// It never describes durable GitHub observations.
type CodeSnapshotPrunePlan struct {
	Ref            domain.RepoRef
	KeepLatest     int
	TotalSnapshots int
	Keep           []CodeSnapshotRef
	Delete         []CodeSnapshotRef
	ReclaimBytes   int64
}

// PlanCodeSnapshotPrune returns a dry-run plan that would keep the latest N
// derived code snapshots for a repository and delete the rest.
func (c *Corpus) PlanCodeSnapshotPrune(ctx context.Context, ref domain.RepoRef, keepLatest int) (*CodeSnapshotPrunePlan, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	if keepLatest < 0 {
		return nil, errors.New("keepLatest cannot be negative")
	}
	snapshots, err := c.listCodeSnapshotRetentionRefs(ctx, ref)
	if err != nil {
		return nil, err
	}

	plan := &CodeSnapshotPrunePlan{
		Ref:            ref,
		KeepLatest:     keepLatest,
		TotalSnapshots: len(snapshots),
	}
	if keepLatest >= len(snapshots) {
		plan.Keep = snapshots
	} else {
		plan.Keep = snapshots[:keepLatest]
		plan.Delete = snapshots[keepLatest:]
	}
	for _, s := range plan.Delete {
		plan.ReclaimBytes += s.TotalBytes
	}
	return plan, nil
}

// CodeSnapshotPruneResult reports how many derived code snapshots were removed.
type CodeSnapshotPruneResult struct {
	Ref          domain.RepoRef
	KeepLatest   int
	Deleted      int
	ReclaimBytes int64
}

var ErrCodeSnapshotPrunePlanStale = errors.New("code snapshot prune plan is stale")

// ApplyCodeSnapshotPrune transactionally prunes derived code snapshots so that
// only the latest N remain for the repository. It requires the plan's scope to
// match the supplied repository reference and recomputes the delete set inside
// the transaction to preserve the latest N against concurrent inserts.
func (c *Corpus) ApplyCodeSnapshotPrune(ctx context.Context, ref domain.RepoRef, plan *CodeSnapshotPrunePlan) (*CodeSnapshotPruneResult, error) {
	if plan == nil {
		return nil, errors.New("prune plan is required")
	}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	if plan.Ref != ref {
		return nil, fmt.Errorf("prune plan scope %q does not match repository %q", plan.Ref, ref)
	}
	if plan.KeepLatest < 0 {
		return nil, errors.New("prune plan keepLatest cannot be negative")
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin code snapshot prune: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	snapshots, err := listCodeSnapshotRetentionRefsTx(ctx, tx, ref)
	if err != nil {
		return nil, err
	}
	planned := append(append([]CodeSnapshotRef(nil), plan.Keep...), plan.Delete...)
	if len(planned) != len(snapshots) {
		return nil, fmt.Errorf("%w: snapshot population changed", ErrCodeSnapshotPrunePlanStale)
	}
	for i := range planned {
		if planned[i].ID != snapshots[i].ID {
			return nil, fmt.Errorf("%w: snapshot ordering changed", ErrCodeSnapshotPrunePlanStale)
		}
	}
	if plan.KeepLatest >= len(snapshots) {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit code snapshot prune: %w", err)
		}
		return &CodeSnapshotPruneResult{Ref: ref, KeepLatest: plan.KeepLatest}, nil
	}

	toDelete := snapshots[plan.KeepLatest:]
	var reclaim int64
	for _, s := range toDelete {
		reclaim += s.TotalBytes
		if err := deleteCodeSnapshotTx(ctx, tx, s.ID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit code snapshot prune: %w", err)
	}
	return &CodeSnapshotPruneResult{Ref: ref, KeepLatest: plan.KeepLatest, Deleted: len(toDelete), ReclaimBytes: reclaim}, nil
}

func (c *Corpus) listCodeSnapshotRetentionRefs(ctx context.Context, ref domain.RepoRef) ([]CodeSnapshotRef, error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT id, commit_sha, created_at, total_bytes
		FROM code_snapshots
		WHERE repo_owner = ? AND repo_name = ?
		ORDER BY created_at DESC, id DESC
	`, ref.Owner, ref.Repo)
	if err != nil {
		return nil, fmt.Errorf("list code snapshots for prune: %w", err)
	}
	defer rows.Close()
	return scanCodeSnapshotRetentionRefs(rows)
}

func listCodeSnapshotRetentionRefsTx(ctx context.Context, tx *sql.Tx, ref domain.RepoRef) ([]CodeSnapshotRef, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, commit_sha, created_at, total_bytes
		FROM code_snapshots
		WHERE repo_owner = ? AND repo_name = ?
		ORDER BY created_at DESC, id DESC
	`, ref.Owner, ref.Repo)
	if err != nil {
		return nil, fmt.Errorf("list code snapshots for prune: %w", err)
	}
	defer rows.Close()
	return scanCodeSnapshotRetentionRefs(rows)
}

func scanCodeSnapshotRetentionRefs(rows *sql.Rows) ([]CodeSnapshotRef, error) {
	var out []CodeSnapshotRef
	for rows.Next() {
		var s CodeSnapshotRef
		var createdAt int64
		if err := rows.Scan(&s.ID, &s.CommitSHA, &createdAt, &s.TotalBytes); err != nil {
			return nil, fmt.Errorf("scan code snapshot ref: %w", err)
		}
		s.CreatedAt = scanTime(createdAt)
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read code snapshot refs: %w", err)
	}
	return out, nil
}

func deleteCodeSnapshotTx(ctx context.Context, tx *sql.Tx, snapshotID int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM code_documents WHERE snapshot_id = ?`, snapshotID); err != nil {
		return fmt.Errorf("delete code documents for snapshot %d: %w", snapshotID, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM code_snapshots WHERE id = ?`, snapshotID); err != nil {
		return fmt.Errorf("delete code snapshot %d: %w", snapshotID, err)
	}
	return nil
}
