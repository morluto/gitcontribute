package corpus

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/clustering"
	"github.com/morluto/gitcontribute/internal/clusterprojection"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/similarity"
)

// ListClusterProjection reads cluster headers and all returned children from a
// single read-only SQLite snapshot using two statements.
func (c *Corpus) ListClusterProjection(ctx context.Context, repo domain.RepoRef, state clustering.ClusterState, limit int) (clusterprojection.List, error) {
	if err := repo.Validate(); err != nil {
		return clusterprojection.List{}, err
	}
	if limit < 1 || limit > 1000 {
		return clusterprojection.List{}, errors.New("cluster list limit must be between 1 and 1000")
	}
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return clusterprojection.List{}, err
	}
	defer func() { _ = tx.Rollback() }()
	identity, _, err := loadProjectionStateTx(ctx, tx, repo)
	if err != nil {
		return clusterprojection.List{}, err
	}
	query := `SELECT id, stable_id, state, canonical_kind, canonical_owner, canonical_repo, canonical_number,
		source_revision, source_window_start, source_window_end, created_at, updated_at
		FROM clusters WHERE repo_owner=? AND repo_name=?`
	args := []any{strings.ToLower(repo.Owner), strings.ToLower(repo.Repo)}
	if state == "" {
		query += ` AND state != ?`
		args = append(args, string(clustering.ClusterRetired))
	} else {
		query += ` AND state = ?`
		args = append(args, string(state))
	}
	query += ` ORDER BY canonical_kind, canonical_owner, canonical_repo, canonical_number, stable_id LIMIT ?`
	args = append(args, limit)
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return clusterprojection.List{}, err
	}
	clusters, err := scanProjectionClusters(rows, repo)
	if closeErr := rows.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return clusterprojection.List{}, err
	}
	if len(clusters) > 0 {
		if err := loadProjectionMembersTx(ctx, tx, clusters); err != nil {
			return clusterprojection.List{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return clusterprojection.List{}, err
	}
	return clusterprojection.List{Repo: repo, Projection: identity, Clusters: clusters}, nil
}

// GetClusterProjection reads one cluster and its members from one read-only snapshot.
func (c *Corpus) GetClusterProjection(ctx context.Context, stableID string) (*clustering.Cluster, error) {
	return c.getClusterProjection(ctx, `WHERE stable_id=?`, []any{stableID})
}

// GetClusterProjectionForMember reads the current included cluster containing ref.
func (c *Corpus) GetClusterProjectionForMember(ctx context.Context, ref clustering.MemberRef) (*clustering.Cluster, error) {
	return c.getClusterProjection(ctx, `JOIN cluster_members member ON member.cluster_id=clusters.id
		WHERE member.kind=? AND LOWER(member.owner)=LOWER(?) AND LOWER(member.repo)=LOWER(?) AND member.number=? AND member.included=1
		ORDER BY clusters.id DESC LIMIT 1`, []any{ref.Kind, ref.Owner, ref.Repo, ref.Number})
}

func (c *Corpus) getClusterProjection(ctx context.Context, predicate string, args []any) (*clustering.Cluster, error) {
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	query := `SELECT clusters.id, clusters.stable_id, clusters.state, clusters.canonical_kind, clusters.canonical_owner, clusters.canonical_repo, clusters.canonical_number,
		clusters.source_revision, clusters.source_window_start, clusters.source_window_end, clusters.created_at, clusters.updated_at,
		clusters.repo_owner, clusters.repo_name FROM clusters ` + predicate
	var cluster clustering.Cluster
	if err := scanProjectionCluster(tx.QueryRowContext(ctx, query, args...), &cluster, true); errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	clusters := []clustering.Cluster{cluster}
	if err := loadProjectionMembersTx(ctx, tx, clusters); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &clusters[0], nil
}

// LoadClusterRefreshSnapshot reads every input needed by a refresh from one
// SQLite snapshot and closes the transaction before CPU-heavy pair evaluation.
func (c *Corpus) LoadClusterRefreshSnapshot(ctx context.Context, repo domain.RepoRef, maxCandidates int) (clusterprojection.RefreshSnapshot, error) {
	if err := repo.Validate(); err != nil {
		return clusterprojection.RefreshSnapshot{}, err
	}
	if maxCandidates < 1 {
		return clusterprojection.RefreshSnapshot{}, errors.New("max candidates must be positive")
	}
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return clusterprojection.RefreshSnapshot{}, err
	}
	defer func() { _ = tx.Rollback() }()

	snapshot := clusterprojection.RefreshSnapshot{Repo: repo, OverridesByCluster: make(map[string][]clustering.MembershipOverride)}
	snapshot.Candidates, err = loadClusterCandidatesTx(ctx, tx, repo, maxCandidates)
	if err != nil {
		return clusterprojection.RefreshSnapshot{}, err
	}
	snapshot.ReadStatements++
	snapshot.SourceRevision = clustering.SourceRevision(snapshot.Candidates)

	identity, governanceRevision, err := loadProjectionStateTx(ctx, tx, repo)
	if err != nil {
		return clusterprojection.RefreshSnapshot{}, err
	}
	snapshot.ReadStatements++
	snapshot.CurrentProjection = identity
	snapshot.GovernanceRevision = governanceRevision

	snapshot.ExistingClusters, err = loadProjectionClustersTx(ctx, tx, repo)
	if err != nil {
		return clusterprojection.RefreshSnapshot{}, err
	}
	snapshot.ReadStatements++
	if len(snapshot.ExistingClusters) > 0 {
		if err := loadProjectionMembersTx(ctx, tx, snapshot.ExistingClusters); err != nil {
			return clusterprojection.RefreshSnapshot{}, err
		}
		snapshot.ReadStatements++
	}
	if err := loadProjectionOverridesTx(ctx, tx, repo, snapshot.OverridesByCluster); err != nil {
		return clusterprojection.RefreshSnapshot{}, err
	}
	snapshot.ReadStatements++
	if err := tx.Commit(); err != nil {
		return clusterprojection.RefreshSnapshot{}, err
	}
	return snapshot, nil
}

func loadClusterCandidatesTx(ctx context.Context, tx *sql.Tx, repo domain.RepoRef, maxCandidates int) ([]clustering.Candidate, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT t.id, t.kind, t.number, t.state, t.title, t.body, t.author, t.labels,
		       t.source_created_at, t.source_updated_at
		FROM threads t
		JOIN repositories r ON r.id = t.repository_id
		WHERE r.owner = ? AND r.name = ?
		ORDER BY t.source_updated_at DESC, t.number DESC
		LIMIT ?
	`, repo.Owner, repo.Repo, maxCandidates+1)
	if err != nil {
		return nil, fmt.Errorf("load cluster candidates: %w", err)
	}
	defer rows.Close()
	out := make([]clustering.Candidate, 0, maxCandidates)
	for rows.Next() {
		var candidate clustering.Candidate
		var labels sql.NullString
		var created, updated int64
		if err := rows.Scan(&candidate.ThreadID, &candidate.Kind, &candidate.Number, &candidate.State, &candidate.Title, &candidate.Body, &candidate.Author, &labels, &created, &updated); err != nil {
			return nil, err
		}
		candidate.Repo = repo
		candidate.Labels = splitLabels(labels.String)
		candidate.CreatedAt = scanTime(created)
		candidate.UpdatedAt = scanTime(updated)
		out = append(out, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > maxCandidates {
		required, _ := clustering.DefaultExactPairBudget().Required(len(out))
		return nil, &clustering.CapacityError{CandidateCount: len(out), RequiredPairs: required, AllowedPairs: uint64(clustering.DefaultExactPairBudget())}
	}
	return out, nil
}

func loadProjectionStateTx(ctx context.Context, tx *sql.Tx, repo domain.RepoRef) (*clusterprojection.Identity, uint64, error) {
	var identity clusterprojection.Identity
	var runID sql.NullInt64
	var source, rule sql.NullString
	var currentGovernance, projectionGovernance uint64
	err := tx.QueryRowContext(ctx, `
		SELECT state.current_run_id, run.source_revision, state.governance_revision,
		       COALESCE(run.governance_revision, 0), run.rule_version
		FROM cluster_projection_state AS state
		LEFT JOIN cluster_runs AS run ON run.id=state.current_run_id
		WHERE state.repo_owner=? AND state.repo_name=?
	`, strings.ToLower(repo.Owner), strings.ToLower(repo.Repo)).Scan(&runID, &source, &currentGovernance, &projectionGovernance, &rule)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	if !runID.Valid {
		return nil, currentGovernance, nil
	}
	identity.RunID = runID.Int64
	identity.SourceRevision = source.String
	identity.GovernanceRevision = projectionGovernance
	identity.RuleVersion = similarity.RuleVersion(rule.String)
	return &identity, currentGovernance, nil
}

func loadProjectionClustersTx(ctx context.Context, tx *sql.Tx, repo domain.RepoRef) ([]clustering.Cluster, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, stable_id, state, canonical_kind, canonical_owner, canonical_repo, canonical_number,
		       source_revision, source_window_start, source_window_end, created_at, updated_at
		FROM clusters WHERE repo_owner=? AND repo_name=?
		ORDER BY canonical_kind, canonical_owner, canonical_repo, canonical_number, stable_id
	`, strings.ToLower(repo.Owner), strings.ToLower(repo.Repo))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProjectionClusters(rows, repo)
}

func scanProjectionClusters(rows *sql.Rows, repo domain.RepoRef) ([]clustering.Cluster, error) {
	var out []clustering.Cluster
	for rows.Next() {
		var cluster clustering.Cluster
		cluster.Repo = repo
		if err := scanProjectionCluster(rows, &cluster, false); err != nil {
			return nil, err
		}
		out = append(out, cluster)
	}
	return out, rows.Err()
}

type projectionScanner interface{ Scan(...any) error }

func scanProjectionCluster(scanner projectionScanner, cluster *clustering.Cluster, includeRepo bool) error {
	var state string
	var windowStart, windowEnd, created, updated int64
	destinations := []any{&cluster.ID, &cluster.StableID, &state, &cluster.Canonical.Kind, &cluster.Canonical.Owner, &cluster.Canonical.Repo, &cluster.Canonical.Number, &cluster.Revision, &windowStart, &windowEnd, &created, &updated}
	if includeRepo {
		destinations = append(destinations, &cluster.Repo.Owner, &cluster.Repo.Repo)
	}
	if err := scanner.Scan(destinations...); err != nil {
		return err
	}
	cluster.State = clustering.ClusterState(state)
	cluster.WindowStart, cluster.WindowEnd = scanTime(windowStart), scanTime(windowEnd)
	cluster.CreatedAt, cluster.UpdatedAt = scanTime(created), scanTime(updated)
	return nil
}

func loadProjectionMembersTx(ctx context.Context, tx *sql.Tx, clusters []clustering.Cluster) error {
	byID := make(map[int64]*clustering.Cluster, len(clusters))
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(clusters)), ",")
	args := make([]any, 0, len(clusters))
	for i := range clusters {
		byID[clusters[i].ID] = &clusters[i]
		args = append(args, clusters[i].ID)
	}
	rows, err := tx.QueryContext(ctx, `SELECT cluster_id, thread_id, kind, owner, repo, number, title, state, score, reason, included
		FROM cluster_members WHERE cluster_id IN (`+placeholders+`)
		ORDER BY cluster_id, score DESC, kind, owner, repo, number`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var clusterID int64
		var member clustering.Member
		var threadID sql.NullInt64
		var included int
		if err := rows.Scan(&clusterID, &threadID, &member.Ref.Kind, &member.Ref.Owner, &member.Ref.Repo, &member.Ref.Number, &member.Title, &member.State, &member.Score, &member.Reason, &included); err != nil {
			return err
		}
		member.ThreadID, member.Included = threadID.Int64, included != 0
		byID[clusterID].Members = append(byID[clusterID].Members, member)
	}
	return rows.Err()
}

func loadProjectionOverridesTx(ctx context.Context, tx *sql.Tx, repo domain.RepoRef, byStable map[string][]clustering.MembershipOverride) error {
	rows, err := tx.QueryContext(ctx, `SELECT c.stable_id, o.id, o.cluster_id, o.kind, o.owner, o.repo, o.number, o.action, o.reason, o.created_at
		FROM cluster_overrides o JOIN clusters c ON c.id=o.cluster_id
		WHERE c.repo_owner=? AND c.repo_name=? ORDER BY o.id`, strings.ToLower(repo.Owner), strings.ToLower(repo.Repo))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var stableID, action string
		var override clustering.MembershipOverride
		var created int64
		if err := rows.Scan(&stableID, &override.ID, &override.ClusterID, &override.Ref.Kind, &override.Ref.Owner, &override.Ref.Repo, &override.Ref.Number, &action, &override.Reason, &created); err != nil {
			return err
		}
		override.Action = clustering.OverrideAction(action)
		override.CreatedAt = scanTime(created)
		byStable[stableID] = append(byStable[stableID], override)
	}
	return rows.Err()
}

// CommitClusterProjection validates a complete refresh result, obtains the
// SQLite writer, rechecks source and governance revisions, and atomically
// advances the current projection. Exact computation must already be complete;
// this method never performs pair evaluation while holding the transaction.
func (c *Corpus) CommitClusterProjection(ctx context.Context, commit clusterprojection.Commit) (clusterprojection.CommitResult, error) {
	if err := ctx.Err(); err != nil {
		return clusterprojection.CommitResult{}, err
	}
	if err := commit.Repo.Validate(); err != nil {
		return clusterprojection.CommitResult{}, err
	}
	if commit.RuleVersion == "" {
		return clusterprojection.CommitResult{}, errors.New("cluster rule version is required")
	}
	if strings.TrimSpace(commit.ExpectedSource) == "" {
		return clusterprojection.CommitResult{}, errors.New("cluster source revision is required")
	}
	if commit.MaxCandidates < 1 {
		return clusterprojection.CommitResult{}, errors.New("max candidates must be positive")
	}
	for _, cluster := range commit.Clusters {
		if strings.TrimSpace(cluster.StableID) == "" {
			return clusterprojection.CommitResult{}, errors.New("cluster stable id is required")
		}
		if !strings.EqualFold(cluster.Repo.Owner, commit.Repo.Owner) || !strings.EqualFold(cluster.Repo.Repo, commit.Repo.Repo) {
			return clusterprojection.CommitResult{}, fmt.Errorf("cluster %q repository does not match commit", cluster.StableID)
		}
		if cluster.Revision != commit.ExpectedSource {
			return clusterprojection.CommitResult{}, fmt.Errorf("cluster %q source revision does not match commit", cluster.StableID)
		}
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return clusterprojection.CommitResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	owner, name := strings.ToLower(commit.Repo.Owner), strings.ToLower(commit.Repo.Repo)
	if _, err := tx.ExecContext(ctx, `INSERT INTO cluster_projection_state (repo_owner, repo_name, governance_revision)
		VALUES (?, ?, 0) ON CONFLICT(repo_owner, repo_name) DO NOTHING`, owner, name); err != nil {
		return clusterprojection.CommitResult{}, err
	}
	identity, actualGovernance, err := loadProjectionStateTx(ctx, tx, commit.Repo)
	if err != nil {
		return clusterprojection.CommitResult{}, err
	}
	candidates, err := loadClusterCandidatesTx(ctx, tx, commit.Repo, commit.MaxCandidates)
	if err != nil {
		return clusterprojection.CommitResult{}, err
	}
	actualSource := clustering.SourceRevision(candidates)
	if actualSource != commit.ExpectedSource || actualGovernance != commit.ExpectedGovernance {
		return clusterprojection.CommitResult{}, &clusterprojection.StaleInputError{ExpectedSource: commit.ExpectedSource, ActualSource: actualSource, ExpectedGovernance: commit.ExpectedGovernance, ActualGovernance: actualGovernance, CurrentCandidateCount: len(candidates)}
	}
	if identity != nil && identity.Matches(actualSource, actualGovernance, commit.RuleVersion) {
		return clusterprojection.CommitResult{Disposition: clusterprojection.AlreadyCurrent, Projection: *identity}, nil
	}
	statsJSON, err := json.Marshal(commit.Stats)
	if err != nil {
		return clusterprojection.CommitResult{}, err
	}
	windowStart, windowEnd := clustering.SourceWindow(candidates)
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, `INSERT INTO cluster_runs
		(repo_owner, repo_name, source_revision, source_window_start, source_window_end, status, started_at, completed_at, governance_revision, rule_version, statistics_json)
		VALUES (?, ?, ?, ?, ?, 'completed', ?, ?, ?, ?, ?)`, owner, name, actualSource, encodeTime(windowStart), encodeTime(windowEnd), encodeTime(now), encodeTime(now), actualGovernance, string(commit.RuleVersion), string(statsJSON))
	if err != nil {
		return clusterprojection.CommitResult{}, fmt.Errorf("insert cluster run: %w", err)
	}
	runID, err := result.LastInsertId()
	if err != nil {
		return clusterprojection.CommitResult{}, err
	}
	writes, err := persistProjectionClustersTx(ctx, tx, commit.Repo, commit.Clusters, now)
	if err != nil {
		return clusterprojection.CommitResult{}, err
	}
	commitStatements := writes + 6 // state ensure/read, source read, run insert, stats update, state advance
	commit.Stats.CommitQueries = commitStatements
	statsJSON, err = json.Marshal(commit.Stats)
	if err != nil {
		return clusterprojection.CommitResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE cluster_runs SET statistics_json=? WHERE id=?`, string(statsJSON), runID); err != nil {
		return clusterprojection.CommitResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE cluster_projection_state SET current_run_id=?, source_revision=?, governance_revision=?, rule_version=?, refreshed_at=?
		WHERE repo_owner=? AND repo_name=?`, runID, actualSource, actualGovernance, string(commit.RuleVersion), encodeTime(now), owner, name); err != nil {
		return clusterprojection.CommitResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return clusterprojection.CommitResult{}, err
	}
	return clusterprojection.CommitResult{Disposition: clusterprojection.Committed, Projection: clusterprojection.Identity{SourceRevision: actualSource, GovernanceRevision: actualGovernance, RuleVersion: commit.RuleVersion, RunID: runID}, WriteStatements: commitStatements}, nil
}

func persistProjectionClustersTx(ctx context.Context, tx *sql.Tx, repo domain.RepoRef, clusters []clustering.Cluster, now time.Time) (int, error) {
	lookup, err := tx.PrepareContext(ctx, `SELECT id FROM clusters WHERE stable_id=?`)
	if err != nil {
		return 0, err
	}
	defer lookup.Close()
	insertCluster, err := tx.PrepareContext(ctx, `INSERT INTO clusters (stable_id, repo_owner, repo_name, state, canonical_kind, canonical_owner, canonical_repo, canonical_number, source_revision, source_window_start, source_window_end, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer insertCluster.Close()
	updateCluster, err := tx.PrepareContext(ctx, `UPDATE clusters SET state=?, canonical_kind=?, canonical_owner=?, canonical_repo=?, canonical_number=?, source_revision=?, source_window_start=?, source_window_end=?, updated_at=? WHERE id=?`)
	if err != nil {
		return 0, err
	}
	defer updateCluster.Close()
	deleteMembers, err := tx.PrepareContext(ctx, `DELETE FROM cluster_members WHERE cluster_id=?`)
	if err != nil {
		return 0, err
	}
	defer deleteMembers.Close()
	insertMember, err := tx.PrepareContext(ctx, `INSERT INTO cluster_members (cluster_id, thread_id, kind, owner, repo, number, title, state, score, reason, included, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer insertMember.Close()

	writes := 0
	active := make(map[string]struct{}, len(clusters))
	for i := range clusters {
		cluster := &clusters[i]
		active[cluster.StableID] = struct{}{}
		if cluster.ID == 0 {
			err := lookup.QueryRowContext(ctx, cluster.StableID).Scan(&cluster.ID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return 0, err
			}
		}
		if cluster.ID == 0 {
			result, err := insertCluster.ExecContext(ctx, cluster.StableID, strings.ToLower(repo.Owner), strings.ToLower(repo.Repo), string(cluster.State), cluster.Canonical.Kind, cluster.Canonical.Owner, cluster.Canonical.Repo, cluster.Canonical.Number, cluster.Revision, encodeTime(cluster.WindowStart), encodeTime(cluster.WindowEnd), encodeTime(cluster.CreatedAt), encodeTime(now))
			if err != nil {
				return 0, err
			}
			cluster.ID, err = result.LastInsertId()
			if err != nil {
				return 0, err
			}
		} else {
			if _, err := updateCluster.ExecContext(ctx, string(cluster.State), cluster.Canonical.Kind, cluster.Canonical.Owner, cluster.Canonical.Repo, cluster.Canonical.Number, cluster.Revision, encodeTime(cluster.WindowStart), encodeTime(cluster.WindowEnd), encodeTime(now), cluster.ID); err != nil {
				return 0, err
			}
		}
		writes++
		if _, err := deleteMembers.ExecContext(ctx, cluster.ID); err != nil {
			return 0, err
		}
		writes++
		for _, member := range cluster.Members {
			var threadID any
			if member.ThreadID > 0 {
				threadID = member.ThreadID
			}
			if _, err := insertMember.ExecContext(ctx, cluster.ID, threadID, member.Ref.Kind, member.Ref.Owner, member.Ref.Repo, member.Ref.Number, member.Title, member.State, member.Score, member.Reason, boolInt(member.Included), encodeTime(now), encodeTime(now)); err != nil {
				return 0, err
			}
			writes++
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, stable_id FROM clusters WHERE repo_owner=? AND repo_name=? AND state != ?`, strings.ToLower(repo.Owner), strings.ToLower(repo.Repo), string(clustering.ClusterRetired))
	if err != nil {
		return 0, err
	}
	var retired []int64
	for rows.Next() {
		var id int64
		var stableID string
		if err := rows.Scan(&id, &stableID); err != nil {
			_ = rows.Close()
			return 0, err
		}
		if _, ok := active[stableID]; !ok {
			retired = append(retired, id)
		}
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	for _, id := range retired {
		if _, err := deleteMembers.ExecContext(ctx, id); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE clusters SET state=?, updated_at=? WHERE id=?`, string(clustering.ClusterRetired), encodeTime(now), id); err != nil {
			return 0, err
		}
		writes += 2
	}
	return writes, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
