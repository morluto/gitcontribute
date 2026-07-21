package corpus

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"strconv"

	"github.com/morluto/gitcontribute/internal/domain"
)

// RepositoryRemovalPlan is an exact, non-mutating preview of repository-owned
// durable observations and derived data. Local workflow records are preserved
// because investigations, opportunities, and workspaces may span repositories.
type RepositoryRemovalPlan struct {
	Ref                          domain.RepoRef
	RepositoryID                 int64
	Revision                     string
	RepositoryObservations       int
	Threads                      int
	ThreadObservations           int
	FacetObservations            int
	FacetCoverage                int
	CodeSnapshots                int
	CodeDocuments                int
	Dossiers                     int
	ClusterRuns                  int
	Clusters                     int
	FrontierItems                int
	DetachedTriageEvents         int
	RemovedPortfolioLinks        int
	RemovedResolutionRecords     int
	RemovedSignalSnapshots       int
	DetachedClusterMembers       int
	PreservedInvestigations      int
	PreservedCrossRepoReferences int
}

// RepositoryRemovalResult reports the exact plan that was applied.
type RepositoryRemovalResult struct {
	Plan *RepositoryRemovalPlan
}

// ErrRepositoryRemovalPlanStale indicates that a confirmed removal preview changed.
var ErrRepositoryRemovalPlanStale = errors.New("repository removal plan is stale")

// PlanRepositoryRemoval previews removal without mutating the corpus.
func (c *Corpus) PlanRepositoryRemoval(ctx context.Context, ref domain.RepoRef) (plan *RepositoryRemovalPlan, err error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin repository removal preview: %w", err)
	}
	defer rollbackSQLOnReturn(tx, &err)
	plan, err = planRepositoryRemoval(ctx, tx, ref)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("finish repository removal preview: %w", err)
	}
	return plan, nil
}

// ApplyRepositoryRemoval verifies the preview inside a transaction and then
// removes only the named repository's observations and replaceable projections.
func (c *Corpus) ApplyRepositoryRemoval(ctx context.Context, ref domain.RepoRef, plan *RepositoryRemovalPlan) (_ *RepositoryRemovalResult, err error) {
	if plan == nil {
		return nil, errors.New("repository removal plan is required")
	}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	if plan.Ref != ref {
		return nil, fmt.Errorf("repository removal plan scope %q does not match repository %q", plan.Ref, ref)
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin repository removal: %w", err)
	}
	defer rollbackSQLOnReturn(tx, &err)

	current, err := planRepositoryRemoval(ctx, tx, ref)
	if err != nil {
		return nil, err
	}
	if current.RepositoryID != plan.RepositoryID || current.Revision != plan.Revision {
		return nil, fmt.Errorf("%w: repository contents changed", ErrRepositoryRemovalPlanStale)
	}
	if err := deleteRepositoryScope(ctx, tx, current); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit repository removal: %w", err)
	}
	return &RepositoryRemovalResult{Plan: current}, nil
}

type repositoryRemovalQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func planRepositoryRemoval(ctx context.Context, db repositoryRemovalQuerier, ref domain.RepoRef) (*RepositoryRemovalPlan, error) {
	plan := &RepositoryRemovalPlan{Ref: ref}
	err := db.QueryRowContext(ctx, `
		SELECT id
		FROM repositories WHERE owner = ? AND name = ?
	`, ref.Owner, ref.Repo).Scan(&plan.RepositoryID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRepositoryNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("plan repository removal: select repository: %w", err)
	}

	counts := []struct {
		destination *int
		query       string
		args        []any
	}{
		{&plan.RepositoryObservations, `SELECT COUNT(*) FROM repository_observations WHERE repository_id = ?`, []any{plan.RepositoryID}},
		{&plan.Threads, `SELECT COUNT(*) FROM threads WHERE repository_id = ?`, []any{plan.RepositoryID}},
		{&plan.ThreadObservations, `SELECT COUNT(*) FROM thread_observations WHERE thread_id IN (SELECT id FROM threads WHERE repository_id = ?)`, []any{plan.RepositoryID}},
		{&plan.FacetObservations, `SELECT COUNT(*) FROM facet_observations WHERE repository_id = ?`, []any{plan.RepositoryID}},
		{&plan.FacetCoverage, `SELECT COUNT(*) FROM facet_coverage WHERE repository_id = ?`, []any{plan.RepositoryID}},
		{&plan.CodeSnapshots, `SELECT COUNT(*) FROM code_snapshots WHERE repo_owner = ? AND repo_name = ?`, []any{ref.Owner, ref.Repo}},
		{&plan.CodeDocuments, `SELECT COUNT(*) FROM code_documents WHERE snapshot_id IN (SELECT id FROM code_snapshots WHERE repo_owner = ? AND repo_name = ?)`, []any{ref.Owner, ref.Repo}},
		{&plan.Dossiers, `SELECT COUNT(*) FROM dossiers WHERE repository_id = ?`, []any{plan.RepositoryID}},
		{&plan.ClusterRuns, `SELECT COUNT(*) FROM cluster_runs WHERE repo_owner = ? AND repo_name = ?`, []any{ref.Owner, ref.Repo}},
		{&plan.Clusters, `SELECT COUNT(*) FROM clusters WHERE repo_owner = ? AND repo_name = ?`, []any{ref.Owner, ref.Repo}},
		{&plan.FrontierItems, `SELECT COUNT(*) FROM frontier_items WHERE owner = ? AND repo = ?`, []any{ref.Owner, ref.Repo}},
		{&plan.DetachedTriageEvents, `SELECT COUNT(*) FROM triage_events WHERE repository_id = ? OR thread_id IN (SELECT id FROM threads WHERE repository_id = ?)`, []any{plan.RepositoryID, plan.RepositoryID}},
		{&plan.RemovedPortfolioLinks, `SELECT COUNT(*) FROM portfolio_links WHERE pull_request_thread_id IN (SELECT id FROM threads WHERE repository_id = ?)`, []any{plan.RepositoryID}},
		{&plan.RemovedResolutionRecords, `SELECT COUNT(*) FROM resolution_records WHERE thread_id IN (SELECT id FROM threads WHERE repository_id = ?)`, []any{plan.RepositoryID}},
		{&plan.RemovedSignalSnapshots, `SELECT COUNT(*) FROM portfolio_signal_snapshots WHERE subject_kind = ? AND CAST(subject_ref AS INTEGER) IN (SELECT id FROM threads WHERE repository_id = ?)`, []any{PortfolioSubjectPullRequest, plan.RepositoryID}},
		{&plan.DetachedClusterMembers, `SELECT COUNT(*) FROM cluster_members WHERE thread_id IN (SELECT id FROM threads WHERE repository_id = ?)`, []any{plan.RepositoryID}},
		{&plan.PreservedInvestigations, `SELECT COUNT(*) FROM investigations WHERE repo_owner = ? AND repo_name = ?`, []any{ref.Owner, ref.Repo}},
		{&plan.PreservedCrossRepoReferences, `
			SELECT COUNT(*)
			FROM portfolio_links link
			JOIN threads thread ON thread.id = link.pull_request_thread_id
			WHERE thread.repository_id <> ? AND (
				link.opportunity_id IN (
					SELECT opportunity.id FROM opportunities opportunity
					JOIN investigations investigation ON investigation.id = opportunity.investigation_id
					WHERE investigation.repo_owner = ? AND investigation.repo_name = ?
				) OR link.workspace_id IN (
					SELECT workspace.id FROM workspaces workspace
					JOIN investigations investigation ON investigation.id = workspace.investigation_id
					WHERE investigation.repo_owner = ? AND investigation.repo_name = ?
				)
			)
		`, []any{plan.RepositoryID, ref.Owner, ref.Repo, ref.Owner, ref.Repo}},
	}
	for _, count := range counts {
		if err := db.QueryRowContext(ctx, count.query, count.args...).Scan(count.destination); err != nil {
			return nil, fmt.Errorf("plan repository removal: count scope: %w", err)
		}
	}

	// Bind authorization to the complete contents of every population that the
	// removal deletes, detaches, or reports as preserved. Counts alone cannot
	// detect a delete-and-reinsert that leaves the preview totals unchanged.
	fingerprintQueries := []struct {
		name  string
		query string
		args  []any
	}{
		{"repository", `SELECT * FROM repositories WHERE id = ? ORDER BY id`, []any{plan.RepositoryID}},
		{"repository observations", `SELECT * FROM repository_observations WHERE repository_id = ? ORDER BY id`, []any{plan.RepositoryID}},
		{"threads", `SELECT * FROM threads WHERE repository_id = ? ORDER BY id`, []any{plan.RepositoryID}},
		{"thread observations", `SELECT * FROM thread_observations WHERE thread_id IN (SELECT id FROM threads WHERE repository_id = ?) ORDER BY id`, []any{plan.RepositoryID}},
		{"facet observations", `SELECT * FROM facet_observations WHERE repository_id = ? ORDER BY id`, []any{plan.RepositoryID}},
		{"facet coverage", `SELECT * FROM facet_coverage WHERE repository_id = ? ORDER BY id`, []any{plan.RepositoryID}},
		{"code snapshots", `SELECT * FROM code_snapshots WHERE repo_owner = ? AND repo_name = ? ORDER BY id`, []any{ref.Owner, ref.Repo}},
		{"code documents", `SELECT * FROM code_documents WHERE snapshot_id IN (SELECT id FROM code_snapshots WHERE repo_owner = ? AND repo_name = ?) ORDER BY id`, []any{ref.Owner, ref.Repo}},
		{"dossiers", `SELECT * FROM dossiers WHERE repository_id = ? ORDER BY id`, []any{plan.RepositoryID}},
		{"dossier sources", `SELECT * FROM dossier_sources WHERE dossier_id IN (SELECT id FROM dossiers WHERE repository_id = ?) ORDER BY id`, []any{plan.RepositoryID}},
		{"cluster runs", `SELECT * FROM cluster_runs WHERE repo_owner = ? AND repo_name = ? ORDER BY id`, []any{ref.Owner, ref.Repo}},
		{"clusters", `SELECT * FROM clusters WHERE repo_owner = ? AND repo_name = ? ORDER BY id`, []any{ref.Owner, ref.Repo}},
		{"cluster members", `SELECT * FROM cluster_members WHERE cluster_id IN (SELECT id FROM clusters WHERE repo_owner = ? AND repo_name = ?) OR thread_id IN (SELECT id FROM threads WHERE repository_id = ?) ORDER BY id`, []any{ref.Owner, ref.Repo, plan.RepositoryID}},
		{"cluster overrides", `SELECT * FROM cluster_overrides WHERE cluster_id IN (SELECT id FROM clusters WHERE repo_owner = ? AND repo_name = ?) OR target_cluster_id IN (SELECT id FROM clusters WHERE repo_owner = ? AND repo_name = ?) ORDER BY id`, []any{ref.Owner, ref.Repo, ref.Owner, ref.Repo}},
		{"cluster projection", `SELECT * FROM cluster_projection_state WHERE repo_owner = ? AND repo_name = ? ORDER BY repo_owner, repo_name`, []any{ref.Owner, ref.Repo}},
		{"frontier items", `SELECT * FROM frontier_items WHERE owner = ? AND repo = ? ORDER BY id`, []any{ref.Owner, ref.Repo}},
		{"triage events", `SELECT * FROM triage_events WHERE repository_id = ? OR thread_id IN (SELECT id FROM threads WHERE repository_id = ?) ORDER BY id`, []any{plan.RepositoryID, plan.RepositoryID}},
		{"portfolio links", `SELECT * FROM portfolio_links WHERE pull_request_thread_id IN (SELECT id FROM threads WHERE repository_id = ?) OR opportunity_id IN (SELECT id FROM opportunities WHERE investigation_id IN (SELECT id FROM investigations WHERE repo_owner = ? AND repo_name = ?)) OR workspace_id IN (SELECT id FROM workspaces WHERE investigation_id IN (SELECT id FROM investigations WHERE repo_owner = ? AND repo_name = ?)) ORDER BY id`, []any{plan.RepositoryID, ref.Owner, ref.Repo, ref.Owner, ref.Repo}},
		{"signal snapshots", `SELECT * FROM portfolio_signal_snapshots WHERE subject_kind = ? AND CAST(subject_ref AS INTEGER) IN (SELECT id FROM threads WHERE repository_id = ?) ORDER BY id`, []any{PortfolioSubjectPullRequest, plan.RepositoryID}},
		{"signals", `SELECT * FROM portfolio_signals WHERE snapshot_id IN (SELECT id FROM portfolio_signal_snapshots WHERE subject_kind = ? AND CAST(subject_ref AS INTEGER) IN (SELECT id FROM threads WHERE repository_id = ?)) ORDER BY snapshot_id, position`, []any{PortfolioSubjectPullRequest, plan.RepositoryID}},
		{"signal projections", `SELECT * FROM portfolio_signal_projections WHERE subject_kind = ? AND CAST(subject_ref AS INTEGER) IN (SELECT id FROM threads WHERE repository_id = ?) ORDER BY subject_kind, subject_ref, facet`, []any{PortfolioSubjectPullRequest, plan.RepositoryID}},
		{"resolution records", `SELECT * FROM resolution_records WHERE thread_id IN (SELECT id FROM threads WHERE repository_id = ?) ORDER BY id`, []any{plan.RepositoryID}},
		{"resolution projections", `SELECT * FROM resolution_projections WHERE thread_id IN (SELECT id FROM threads WHERE repository_id = ?) ORDER BY thread_id`, []any{plan.RepositoryID}},
		{"investigations", `SELECT * FROM investigations WHERE repo_owner = ? AND repo_name = ? ORDER BY id`, []any{ref.Owner, ref.Repo}},
	}
	digest := sha256.New()
	for _, fingerprint := range fingerprintQueries {
		if err := appendRemovalFingerprint(ctx, digest, db, fingerprint.name, fingerprint.query, fingerprint.args...); err != nil {
			return nil, err
		}
	}
	plan.Revision = hex.EncodeToString(digest.Sum(nil))
	return plan, nil
}

func appendRemovalFingerprint(ctx context.Context, digest hash.Hash, db repositoryRemovalQuerier, name, query string, args ...any) (returnErr error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("plan repository removal: fingerprint %s: %w", name, err)
	}
	defer func() {
		returnErr = errors.Join(returnErr, rows.Close())
	}()

	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("plan repository removal: fingerprint %s columns: %w", name, err)
	}
	if err := fingerprintField(digest, name); err != nil {
		return fmt.Errorf("plan repository removal: fingerprint %s label: %w", name, err)
	}
	for _, column := range columns {
		if err := fingerprintField(digest, column); err != nil {
			return fmt.Errorf("plan repository removal: fingerprint %s column: %w", name, err)
		}
	}
	for rows.Next() {
		values := make([]any, len(columns))
		destinations := make([]any, len(columns))
		for index := range values {
			destinations[index] = &values[index]
		}
		if err := rows.Scan(destinations...); err != nil {
			return fmt.Errorf("plan repository removal: fingerprint %s row: %w", name, err)
		}
		for _, value := range values {
			if err := fingerprintField(digest, fmt.Sprintf("%T:%v", value, value)); err != nil {
				return fmt.Errorf("plan repository removal: fingerprint %s value: %w", name, err)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("plan repository removal: fingerprint %s rows: %w", name, err)
	}
	return nil
}

func fingerprintField(digest hash.Hash, value string) error {
	_, err := fmt.Fprintf(digest, "%d:%s", len(value), value)
	return err
}

func deleteRepositoryScope(ctx context.Context, tx *sql.Tx, plan *RepositoryRemovalPlan) error {
	repoID := plan.RepositoryID
	ref := plan.Ref
	statements := []struct {
		name  string
		query string
		args  []any
	}{
		{"detach cross-repository cluster members", `UPDATE cluster_members SET thread_id = NULL WHERE thread_id IN (SELECT id FROM threads WHERE repository_id = ?)`, []any{repoID}},
		{"delete pull-request signal snapshots", `DELETE FROM portfolio_signal_snapshots WHERE subject_kind = ? AND CAST(subject_ref AS INTEGER) IN (SELECT id FROM threads WHERE repository_id = ?)`, []any{PortfolioSubjectPullRequest, repoID}},
		{"delete cluster projection state", `DELETE FROM cluster_projection_state WHERE repo_owner = ? AND repo_name = ?`, []any{ref.Owner, ref.Repo}},
		{"delete repository clusters", `DELETE FROM clusters WHERE repo_owner = ? AND repo_name = ?`, []any{ref.Owner, ref.Repo}},
		{"delete repository cluster runs", `DELETE FROM cluster_runs WHERE repo_owner = ? AND repo_name = ?`, []any{ref.Owner, ref.Repo}},
		{"delete code snapshots", `DELETE FROM code_snapshots WHERE repo_owner = ? AND repo_name = ?`, []any{ref.Owner, ref.Repo}},
		{"delete frontier items", `DELETE FROM frontier_items WHERE owner = ? AND repo = ?`, []any{ref.Owner, ref.Repo}},
		{"delete facet observations", `DELETE FROM facet_observations WHERE repository_id = ?`, []any{repoID}},
		{"delete facet coverage", `DELETE FROM facet_coverage WHERE repository_id = ?`, []any{repoID}},
		{"delete thread observations", `DELETE FROM thread_observations WHERE thread_id IN (SELECT id FROM threads WHERE repository_id = ?)`, []any{repoID}},
		{"delete threads", `DELETE FROM threads WHERE repository_id = ?`, []any{repoID}},
		{"delete repository observations", `DELETE FROM repository_observations WHERE repository_id = ?`, []any{repoID}},
		{"delete repository", `DELETE FROM repositories WHERE id = ?`, []any{repoID}},
	}
	for _, statement := range statements {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return fmt.Errorf("%s: %w", statement.name, err)
		}
	}
	if err := foreignKeyCheck(ctx, tx); err != nil {
		return err
	}
	return nil
}

func foreignKeyCheck(ctx context.Context, tx *sql.Tx) (returnErr error) {
	rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("check repository removal referential integrity: %w", err)
	}
	defer func() {
		returnErr = errors.Join(returnErr, rows.Close())
	}()
	if rows.Next() {
		var table string
		var rowID sql.NullInt64
		var parent string
		var foreignKey int
		if err := rows.Scan(&table, &rowID, &parent, &foreignKey); err != nil {
			return fmt.Errorf("check repository removal referential integrity: %w", err)
		}
		return fmt.Errorf("repository removal would violate foreign key %s[%s] -> %s (%d)", table, strconv.FormatInt(rowID.Int64, 10), parent, foreignKey)
	}
	return rows.Err()
}
