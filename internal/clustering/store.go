package clustering

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

// Store persists and queries duplicate-candidate clusters in a SQLite corpus.
type Store struct {
	db *sql.DB
}

// NewStore returns a cluster store backed by the supplied database. The database
// must already contain the clustering schema; Corpus.Open applies it.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// ComputeForRepo loads thread candidates for a repository, clusters them,
// applies stored overrides, and persists the result.
func (s *Store) ComputeForRepo(ctx context.Context, repo domain.RepoRef, cfg Config) (*ClusterRun, []Cluster, error) {
	if err := repo.Validate(); err != nil {
		return nil, nil, fmt.Errorf("validate repo: %w", err)
	}

	candidates, err := s.loadCandidates(ctx, repo, cfg.MaxCandidates)
	if err != nil {
		return nil, nil, fmt.Errorf("load candidates: %w", err)
	}
	return s.compute(ctx, repo, candidates, cfg)
}

// Compute clusters a caller-provided candidate set and persists it,
// matching existing clusters by stable id so that ids and overrides survive.
func (s *Store) Compute(ctx context.Context, repo domain.RepoRef, candidates []Candidate, cfg Config) (*ClusterRun, []Cluster, error) {
	if err := repo.Validate(); err != nil {
		return nil, nil, fmt.Errorf("validate repo: %w", err)
	}
	return s.compute(ctx, repo, candidates, cfg)
}

func (s *Store) compute(ctx context.Context, repo domain.RepoRef, candidates []Candidate, cfg Config) (*ClusterRun, []Cluster, error) {
	if err := validateCandidates(repo, candidates); err != nil {
		return nil, nil, err
	}
	revision := SourceRevision(candidates)
	windowStart, windowEnd := SourceWindow(candidates)

	cl := NewClusterer(cfg)
	raw, err := cl.Cluster(candidates, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("cluster: %w", err)
	}

	// Match raw clusters to existing clusters by stable_id and apply overrides.
	existing, err := s.listClustersForRepo(ctx, repo, "", 0)
	if err != nil {
		return nil, nil, fmt.Errorf("list existing clusters: %w", err)
	}
	existingByStable := make(map[string]*Cluster, len(existing))
	for i := range existing {
		existingByStable[existing[i].StableID] = &existing[i]
	}

	var clusters []Cluster
	matched := make(map[string]struct{}, len(raw))
	now := time.Now().UTC()
	for _, c := range raw {
		ec, ok := existingByStable[c.StableID]
		if ok {
			matched[c.StableID] = struct{}{}
			c.ID = ec.ID
			c.CreatedAt = ec.CreatedAt
			c.State = ec.State
			if c.State == ClusterRetired {
				c.State = ClusterOpen
			}
			overrides, err := s.listOverridesForCluster(ctx, ec.ID, 0)
			if err != nil {
				return nil, nil, err
			}
			c = applyOverrides([]Cluster{c}, overrides, candidates)[0]
		}
		enrichMembers(candidates, &c)
		c.Repo = repo
		c.Revision = revision
		c.WindowStart = windowStart
		c.WindowEnd = windowEnd
		if c.State == "" {
			c.State = ClusterOpen
		}
		c.UpdatedAt = now
		if c.CreatedAt.IsZero() {
			c.CreatedAt = now
		}
		clusters = append(clusters, c)
	}

	// Explicit include/canonical decisions create durable cluster shape that
	// must survive even when the similarity computation no longer emits it.
	for i := range existing {
		ec := &existing[i]
		if _, ok := matched[ec.StableID]; ok {
			continue
		}
		overrides, err := s.listOverridesForCluster(ctx, ec.ID, 0)
		if err != nil {
			return nil, nil, err
		}
		if !hasPersistentGovernance(overrides) {
			continue
		}
		preserved, err := s.getClusterByID(ctx, ec.ID)
		if err != nil {
			return nil, nil, err
		}
		if preserved == nil {
			continue
		}
		cluster := applyOverrides([]Cluster{*preserved}, overrides, candidates)[0]
		enrichMembers(candidates, &cluster)
		cluster.Revision = revision
		cluster.WindowStart = windowStart
		cluster.WindowEnd = windowEnd
		cluster.UpdatedAt = now
		clusters = append(clusters, cluster)
	}
	sortClusters(clusters)

	run := &ClusterRun{
		Repo:           repo,
		SourceRevision: revision,
		WindowStart:    windowStart,
		WindowEnd:      windowEnd,
		ParamsHash:     cl.Config.ParamsHash(),
		Status:         "running",
		StartedAt:      now,
	}

	if err := s.persist(ctx, run, clusters); err != nil {
		return nil, nil, fmt.Errorf("persist: %w", err)
	}
	return run, clusters, nil
}

func hasPersistentGovernance(overrides []MembershipOverride) bool {
	for _, override := range overrides {
		if override.Action == OverrideInclude || override.Action == OverrideSetCanonical {
			return true
		}
	}
	return false
}

func (s *Store) loadCandidates(ctx context.Context, repo domain.RepoRef, limit int) ([]Candidate, error) {
	if limit <= 0 {
		limit = DefaultConfig().MaxCandidates
	}

	var count int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM threads t
		JOIN repositories r ON r.id = t.repository_id
		WHERE r.owner = ? AND r.name = ?
	`, repo.Owner, repo.Repo).Scan(&count); err != nil {
		return nil, fmt.Errorf("count candidates: %w", err)
	}
	if count > limit {
		return nil, fmt.Errorf("candidate count %d exceeds hard limit %d", count, limit)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.kind, t.number, t.state, t.title, t.body, t.author, t.labels,
		       t.source_created_at, t.source_updated_at
		FROM threads t
		JOIN repositories r ON r.id = t.repository_id
		WHERE r.owner = ? AND r.name = ?
		ORDER BY t.source_updated_at DESC, t.number DESC
	`, repo.Owner, repo.Repo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Candidate
	for rows.Next() {
		var c Candidate
		var labels sql.NullString
		var created, updated int64
		if err := rows.Scan(&c.ThreadID, &c.Kind, &c.Number, &c.State, &c.Title, &c.Body, &c.Author, &labels, &created, &updated); err != nil {
			return nil, err
		}
		c.Repo = repo
		c.Labels = splitLabels(labels.String)
		c.CreatedAt = scanTime(created)
		c.UpdatedAt = scanTime(updated)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) loadCandidatesForOverrides(ctx context.Context, repo domain.RepoRef, overrides []MembershipOverride) ([]Candidate, error) {
	refs := make(map[MemberRef]struct{}, len(overrides))
	for _, o := range overrides {
		refs[o.Ref] = struct{}{}
	}
	var candidates []Candidate
	for ref := range refs {
		c, err := s.loadCandidateForRef(ctx, repo, ref)
		if err != nil {
			return nil, err
		}
		if c != nil {
			candidates = append(candidates, *c)
		}
	}
	return candidates, nil
}

func (s *Store) loadCandidateForRef(ctx context.Context, repo domain.RepoRef, ref MemberRef) (*Candidate, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT t.id, t.kind, t.number, t.state, t.title, t.body, t.author, t.labels,
		       t.source_created_at, t.source_updated_at
		FROM threads t
		JOIN repositories r ON r.id = t.repository_id
		WHERE r.owner = ? AND r.name = ? AND t.kind = ? AND t.number = ?
	`, repo.Owner, repo.Repo, ref.Kind, ref.Number)
	var c Candidate
	var labels sql.NullString
	var created, updated int64
	err := row.Scan(&c.ThreadID, &c.Kind, &c.Number, &c.State, &c.Title, &c.Body, &c.Author, &labels, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Repo = repo
	c.Labels = splitLabels(labels.String)
	c.CreatedAt = scanTime(created)
	c.UpdatedAt = scanTime(updated)
	return &c, nil
}

func (s *Store) listClustersForRepo(ctx context.Context, repo domain.RepoRef, state ClusterState, limit int) ([]Cluster, error) {
	args := []any{strings.ToLower(repo.Owner), strings.ToLower(repo.Repo)}
	query := `SELECT id, stable_id, state, canonical_kind, canonical_owner, canonical_repo, canonical_number,
	               source_revision, source_window_start, source_window_end, created_at, updated_at
	        FROM clusters
	        WHERE repo_owner = ? AND repo_name = ?`
	if state != "" {
		query += ` AND state = ?`
		args = append(args, string(state))
	}
	query += ` ORDER BY canonical_kind, canonical_owner, canonical_repo, canonical_number, stable_id`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Cluster
	for rows.Next() {
		c, err := s.scanCluster(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) scanCluster(rows *sql.Rows) (Cluster, error) {
	var c Cluster
	var created, updated, winStart, winEnd int64
	var state string
	err := rows.Scan(&c.ID, &c.StableID, &state, &c.Canonical.Kind, &c.Canonical.Owner, &c.Canonical.Repo, &c.Canonical.Number,
		&c.Revision, &winStart, &winEnd, &created, &updated)
	if err != nil {
		return c, err
	}
	c.State = ClusterState(state)
	c.WindowStart = scanTime(winStart)
	c.WindowEnd = scanTime(winEnd)
	c.CreatedAt = scanTime(created)
	c.UpdatedAt = scanTime(updated)
	return c, nil
}

func (s *Store) listOverridesForCluster(ctx context.Context, clusterID int64, limit int) ([]MembershipOverride, error) {
	query := `SELECT id, cluster_id, kind, owner, repo, number, action, reason, created_at
	        FROM cluster_overrides
	        WHERE cluster_id = ?
	        ORDER BY id`
	args := []any{clusterID}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MembershipOverride
	for rows.Next() {
		var o MembershipOverride
		var created int64
		if err := rows.Scan(&o.ID, &o.ClusterID, &o.Ref.Kind, &o.Ref.Owner, &o.Ref.Repo, &o.Ref.Number, &o.Action, &o.Reason, &created); err != nil {
			return nil, err
		}
		o.CreatedAt = scanTime(created)
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) persist(ctx context.Context, run *ClusterRun, clusters []Cluster) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := encodeTime(time.Now())
	res, err := tx.ExecContext(ctx, `
		INSERT INTO cluster_runs (repo_owner, repo_name, source_revision, source_window_start, source_window_end, params_hash, status, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, strings.ToLower(run.Repo.Owner), strings.ToLower(run.Repo.Repo), run.SourceRevision, encodeTime(run.WindowStart), encodeTime(run.WindowEnd), run.ParamsHash, run.Status, encodeTime(run.StartedAt))
	if err != nil {
		return fmt.Errorf("insert cluster run: %w", err)
	}
	run.ID, err = res.LastInsertId()
	if err != nil {
		return fmt.Errorf("cluster run id: %w", err)
	}

	for i := range clusters {
		c := &clusters[i]
		if err := s.upsertClusterTx(ctx, tx, c, now); err != nil {
			return err
		}
		if err := s.replaceMembersTx(ctx, tx, c); err != nil {
			return err
		}
	}
	if err := s.retireMissingClustersTx(ctx, tx, run.Repo, clusters, now); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE cluster_runs SET status = ?, completed_at = ? WHERE id = ?
	`, "completed", now, run.ID); err != nil {
		return fmt.Errorf("complete cluster run: %w", err)
	}
	run.Status = "completed"
	completedAt := time.Now().UTC()
	run.CompletedAt = &completedAt

	return tx.Commit()
}

func (s *Store) retireMissingClustersTx(ctx context.Context, tx *sql.Tx, repo domain.RepoRef, current []Cluster, now int64) error {
	active := make(map[string]struct{}, len(current))
	for _, cluster := range current {
		active[cluster.StableID] = struct{}{}
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT id, stable_id
		FROM clusters
		WHERE repo_owner = ? AND repo_name = ? AND state != ?
	`, strings.ToLower(repo.Owner), strings.ToLower(repo.Repo), string(ClusterRetired))
	if err != nil {
		return fmt.Errorf("list clusters for reconciliation: %w", err)
	}
	var staleIDs []int64
	for rows.Next() {
		var id int64
		var stableID string
		if err := rows.Scan(&id, &stableID); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan cluster for reconciliation: %w", err)
		}
		if _, ok := active[stableID]; !ok {
			staleIDs = append(staleIDs, id)
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, id := range staleIDs {
		if _, err := tx.ExecContext(ctx, `DELETE FROM cluster_members WHERE cluster_id = ?`, id); err != nil {
			return fmt.Errorf("clear retired cluster members: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE clusters SET state = ?, updated_at = ? WHERE id = ?`, string(ClusterRetired), now, id); err != nil {
			return fmt.Errorf("retire cluster: %w", err)
		}
	}
	return nil
}

func (s *Store) upsertClusterTx(ctx context.Context, tx *sql.Tx, c *Cluster, now int64) error {
	if c.StableID == "" {
		return errors.New("cluster stable_id is required")
	}
	if c.ID == 0 {
		var id int64
		err := tx.QueryRowContext(ctx, `
			SELECT id FROM clusters WHERE stable_id = ?
		`, c.StableID).Scan(&id)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("lookup cluster: %w", err)
		}
		c.ID = id
	}

	if c.ID == 0 {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO clusters (stable_id, repo_owner, repo_name, state, canonical_kind, canonical_owner, canonical_repo, canonical_number,
			                      source_revision, source_window_start, source_window_end, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, c.StableID, strings.ToLower(c.Repo.Owner), strings.ToLower(c.Repo.Repo), string(c.State), c.Canonical.Kind, c.Canonical.Owner, c.Canonical.Repo, c.Canonical.Number,
			c.Revision, encodeTime(c.WindowStart), encodeTime(c.WindowEnd), encodeTime(c.CreatedAt), now)
		if err != nil {
			return fmt.Errorf("insert cluster: %w", err)
		}
		c.ID, err = res.LastInsertId()
		if err != nil {
			return fmt.Errorf("cluster id: %w", err)
		}
	} else {
		_, err := tx.ExecContext(ctx, `
			UPDATE clusters
			SET state = ?, canonical_kind = ?, canonical_owner = ?, canonical_repo = ?, canonical_number = ?,
			    source_revision = ?, source_window_start = ?, source_window_end = ?, updated_at = ?
			WHERE id = ?
		`, string(c.State), c.Canonical.Kind, c.Canonical.Owner, c.Canonical.Repo, c.Canonical.Number,
			c.Revision, encodeTime(c.WindowStart), encodeTime(c.WindowEnd), now, c.ID)
		if err != nil {
			return fmt.Errorf("update cluster: %w", err)
		}
	}
	return nil
}

func (s *Store) replaceMembersTx(ctx context.Context, tx *sql.Tx, c *Cluster) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM cluster_members WHERE cluster_id = ?`, c.ID); err != nil {
		return fmt.Errorf("delete members: %w", err)
	}
	now := encodeTime(time.Now())
	for _, m := range c.Members {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO cluster_members (cluster_id, thread_id, kind, owner, repo, number, title, state, score, reason, included, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, c.ID, nullableThreadID(m.ThreadID), m.Ref.Kind, m.Ref.Owner, m.Ref.Repo, m.Ref.Number, m.Title, m.State, m.Score, m.Reason, boolToInt(m.Included), now, now)
		if err != nil {
			return fmt.Errorf("insert member: %w", err)
		}
	}
	return nil
}

// ListClusters returns clusters for a repository, optionally filtered by state.
func (s *Store) ListClusters(ctx context.Context, repo domain.RepoRef, state ClusterState, limit int) ([]Cluster, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		return nil, fmt.Errorf("cluster list limit cannot exceed 1000")
	}
	clusters, err := s.listClustersForRepo(ctx, repo, state, limit)
	if err != nil {
		return nil, err
	}
	for i := range clusters {
		members, err := s.listMembersForCluster(ctx, clusters[i].ID, 0)
		if err != nil {
			return nil, err
		}
		clusters[i].Members = members
		clusters[i].Repo = repo
	}
	return clusters, nil
}

// GetCluster returns a cluster by stable id with its members.
func (s *Store) GetCluster(ctx context.Context, stableID string) (*Cluster, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, stable_id, state, canonical_kind, canonical_owner, canonical_repo, canonical_number,
		       source_revision, source_window_start, source_window_end, created_at, updated_at,
		       repo_owner, repo_name
		FROM clusters
		WHERE stable_id = ?
	`, stableID)
	var c Cluster
	var created, updated, winStart, winEnd int64
	var state string
	err := row.Scan(&c.ID, &c.StableID, &state, &c.Canonical.Kind, &c.Canonical.Owner, &c.Canonical.Repo, &c.Canonical.Number,
		&c.Revision, &winStart, &winEnd, &created, &updated, &c.Repo.Owner, &c.Repo.Repo)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.State = ClusterState(state)
	c.WindowStart = scanTime(winStart)
	c.WindowEnd = scanTime(winEnd)
	c.CreatedAt = scanTime(created)
	c.UpdatedAt = scanTime(updated)
	members, err := s.listMembersForCluster(ctx, c.ID, 0)
	if err != nil {
		return nil, err
	}
	c.Members = members
	return &c, nil
}

// GetClusterForMember returns the cluster that currently includes the given
// member reference, or nil if the member is not an included member of any
// cluster. The match is case-insensitive on owner and repo.
func (s *Store) GetClusterForMember(ctx context.Context, ref MemberRef) (*Cluster, error) {
	if err := validateMemberRef(ref); err != nil {
		return nil, err
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT c.id, c.stable_id, c.state, c.canonical_kind, c.canonical_owner, c.canonical_repo, c.canonical_number,
		       c.source_revision, c.source_window_start, c.source_window_end, c.created_at, c.updated_at,
		       c.repo_owner, c.repo_name
		FROM clusters c
		JOIN cluster_members m ON m.cluster_id = c.id
		WHERE m.kind = ?
		  AND LOWER(m.owner) = LOWER(?)
		  AND LOWER(m.repo) = LOWER(?)
		  AND m.number = ?
		  AND m.included = 1
		ORDER BY c.id DESC
		LIMIT 1
	`, ref.Kind, ref.Owner, ref.Repo, ref.Number)
	var c Cluster
	var created, updated, winStart, winEnd int64
	var state string
	err := row.Scan(&c.ID, &c.StableID, &state, &c.Canonical.Kind, &c.Canonical.Owner, &c.Canonical.Repo, &c.Canonical.Number,
		&c.Revision, &winStart, &winEnd, &created, &updated, &c.Repo.Owner, &c.Repo.Repo)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get cluster for member: %w", err)
	}
	c.State = ClusterState(state)
	c.WindowStart = scanTime(winStart)
	c.WindowEnd = scanTime(winEnd)
	c.CreatedAt = scanTime(created)
	c.UpdatedAt = scanTime(updated)
	members, err := s.listMembersForCluster(ctx, c.ID, 0)
	if err != nil {
		return nil, err
	}
	c.Members = members
	return &c, nil
}

func (s *Store) listMembersForCluster(ctx context.Context, clusterID int64, limit int) ([]Member, error) {
	query := `SELECT thread_id, kind, owner, repo, number, title, state, score, reason, included
	        FROM cluster_members
	        WHERE cluster_id = ?
	        ORDER BY score DESC, kind, owner, repo, number`
	args := []any{clusterID}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Member
	for rows.Next() {
		var m Member
		var tid sql.NullInt64
		var included int
		if err := rows.Scan(&tid, &m.Ref.Kind, &m.Ref.Owner, &m.Ref.Repo, &m.Ref.Number, &m.Title, &m.State, &m.Score, &m.Reason, &included); err != nil {
			return nil, err
		}
		m.ThreadID = tid.Int64
		m.Included = included != 0
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListOverrides returns membership overrides for a cluster.
func (s *Store) ListOverrides(ctx context.Context, clusterID int64, limit int) ([]MembershipOverride, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		return nil, fmt.Errorf("override list limit cannot exceed 1000")
	}
	return s.listOverridesForCluster(ctx, clusterID, limit)
}

// AddOverride records a membership override and applies it to the cluster.
func (s *Store) AddOverride(ctx context.Context, clusterID int64, ref MemberRef, action OverrideAction, reason string) error {
	if clusterID == 0 {
		return errors.New("cluster id is required")
	}
	if err := validateMemberRef(ref); err != nil {
		return err
	}
	switch action {
	case OverrideInclude, OverrideExclude, OverrideSetCanonical:
	default:
		return fmt.Errorf("unsupported override action %q", action)
	}
	if strings.TrimSpace(reason) == "" {
		return errors.New("reason is required")
	}
	cluster, err := s.getClusterByID(ctx, clusterID)
	if err != nil {
		return err
	}
	if cluster == nil {
		return fmt.Errorf("cluster %d not found", clusterID)
	}
	if action == OverrideExclude && sameMemberRef(ref, cluster.Canonical) {
		return errors.New("cannot exclude the canonical member")
	}
	now := encodeTime(time.Now())
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO cluster_overrides (cluster_id, kind, owner, repo, number, action, reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, clusterID, ref.Kind, ref.Owner, ref.Repo, ref.Number, string(action), strings.TrimSpace(reason), now); err != nil {
		return fmt.Errorf("insert override: %w", err)
	}
	return s.reapplyOverrides(ctx, clusterID)
}

func (s *Store) reapplyOverrides(ctx context.Context, clusterID int64) error {
	c, err := s.getClusterByID(ctx, clusterID)
	if err != nil {
		return err
	}
	if c == nil {
		return fmt.Errorf("cluster %d not found", clusterID)
	}
	overrides, err := s.listOverridesForCluster(ctx, clusterID, 0)
	if err != nil {
		return err
	}
	candidates, err := s.loadCandidatesForOverrides(ctx, c.Repo, overrides)
	if err != nil {
		return err
	}
	updated := applyOverrides([]Cluster{*c}, overrides, candidates)[0]
	enrichMembers(candidates, &updated)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.upsertClusterTx(ctx, tx, &updated, encodeTime(time.Now())); err != nil {
		return err
	}
	if err := s.replaceMembersTx(ctx, tx, &updated); err != nil {
		return err
	}
	return tx.Commit()
}

// CloseCluster marks a cluster closed without touching GitHub.
func (s *Store) CloseCluster(ctx context.Context, clusterID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE clusters SET state = ?, updated_at = ? WHERE id = ?`, string(ClusterClosed), encodeTime(time.Now()), clusterID)
	return err
}

// ReopenCluster reopens a closed cluster.
func (s *Store) ReopenCluster(ctx context.Context, clusterID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE clusters SET state = ?, updated_at = ? WHERE id = ?`, string(ClusterOpen), encodeTime(time.Now()), clusterID)
	return err
}

// MergeClusters moves all members from the source cluster into the target
// cluster and closes the source cluster. A merge override is recorded.
func (s *Store) MergeClusters(ctx context.Context, fromID, toID int64, reason string) error {
	if fromID == toID {
		return errors.New("cannot merge a cluster into itself")
	}
	from, err := s.getClusterByID(ctx, fromID)
	if err != nil {
		return err
	}
	to, err := s.getClusterByID(ctx, toID)
	if err != nil {
		return err
	}
	if from == nil || to == nil {
		return errors.New("cluster not found")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := encodeTime(time.Now())
	// Move members from -> to.
	for _, m := range from.Members {
		if !m.Included {
			continue
		}
		_, err := tx.ExecContext(ctx, `
			DELETE FROM cluster_members WHERE cluster_id = ? AND kind = ? AND owner = ? AND repo = ? AND number = ?
		`, toID, m.Ref.Kind, m.Ref.Owner, m.Ref.Repo, m.Ref.Number)
		if err != nil {
			return fmt.Errorf("remove duplicate target member: %w", err)
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO cluster_members (cluster_id, thread_id, kind, owner, repo, number, title, state, score, reason, included, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, toID, nullableThreadID(m.ThreadID), m.Ref.Kind, m.Ref.Owner, m.Ref.Repo, m.Ref.Number, m.Title, m.State, m.Score, m.Reason+" (merged)", boolToInt(m.Included), now, now)
		if err != nil {
			return fmt.Errorf("insert merged member: %w", err)
		}
		mergeReason := "merged from cluster " + from.StableID + ": " + strings.TrimSpace(reason)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO cluster_overrides (cluster_id, kind, owner, repo, number, action, reason, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, toID, m.Ref.Kind, m.Ref.Owner, m.Ref.Repo, m.Ref.Number, string(OverrideInclude), mergeReason, now); err != nil {
			return fmt.Errorf("record target merge membership: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO cluster_overrides (cluster_id, kind, owner, repo, number, action, reason, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, fromID, m.Ref.Kind, m.Ref.Owner, m.Ref.Repo, m.Ref.Number, string(OverrideExclude), "merged into cluster "+to.StableID, now); err != nil {
			return fmt.Errorf("record source merge membership: %w", err)
		}
	}

	// Remove members from source cluster once they have been moved.
	_, err = tx.ExecContext(ctx, `DELETE FROM cluster_members WHERE cluster_id = ?`, fromID)
	if err != nil {
		return fmt.Errorf("delete source members: %w", err)
	}

	// Record merge override and close source.
	_, err = tx.ExecContext(ctx, `
		UPDATE clusters SET state = ?, updated_at = ? WHERE id = ?
	`, string(ClusterClosed), now, fromID)
	if err != nil {
		return fmt.Errorf("close source cluster: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO cluster_overrides (cluster_id, kind, owner, repo, number, action, reason, target_cluster_id, created_at)
		VALUES (?, ?, ?, ?, ?, 'merge', ?, ?, ?)
	`, fromID, from.Canonical.Kind, from.Canonical.Owner, from.Canonical.Repo, from.Canonical.Number, reason, toID, now)
	if err != nil {
		return fmt.Errorf("record merge override: %w", err)
	}

	return tx.Commit()
}

// SplitCluster removes a member from a cluster and creates a new singleton
// cluster for that member.
func (s *Store) SplitCluster(ctx context.Context, clusterID int64, ref MemberRef, reason string) error {
	c, err := s.getClusterByID(ctx, clusterID)
	if err != nil {
		return err
	}
	if c == nil {
		return errors.New("cluster not found")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := encodeTime(time.Now())
	var member Member
	found := false
	for _, m := range c.Members {
		if m.Ref == ref {
			member = m
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("member %s not in cluster", ref.String())
	}
	if ref == c.Canonical {
		return errors.New("cannot split the canonical member from its cluster")
	}

	newStable := StableID(ref)
	res, err := tx.ExecContext(ctx, `
		INSERT INTO clusters (stable_id, repo_owner, repo_name, state, canonical_kind, canonical_owner, canonical_repo, canonical_number,
		                    source_revision, source_window_start, source_window_end, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, newStable, strings.ToLower(c.Repo.Owner), strings.ToLower(c.Repo.Repo), string(ClusterOpen), ref.Kind, ref.Owner, ref.Repo, ref.Number,
		c.Revision, encodeTime(c.WindowStart), encodeTime(c.WindowEnd), now, now)
	if err != nil {
		return fmt.Errorf("insert split cluster: %w", err)
	}
	newID, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("split cluster id: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO cluster_members (cluster_id, thread_id, kind, owner, repo, number, title, state, score, reason, included, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, newID, nullableThreadID(member.ThreadID), ref.Kind, ref.Owner, ref.Repo, ref.Number, member.Title, member.State, 1.0, "split canonical: "+reason, 1, now, now)
	if err != nil {
		return fmt.Errorf("insert split member: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO cluster_overrides (cluster_id, kind, owner, repo, number, action, reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, newID, ref.Kind, ref.Owner, ref.Repo, ref.Number, string(OverrideSetCanonical), "split: "+strings.TrimSpace(reason), now); err != nil {
		return fmt.Errorf("record split cluster governance: %w", err)
	}

	// Exclude from source cluster.
	_, err = tx.ExecContext(ctx, `
		INSERT INTO cluster_overrides (cluster_id, kind, owner, repo, number, action, reason, created_at)
		VALUES (?, ?, ?, ?, ?, 'exclude', ?, ?)
	`, clusterID, ref.Kind, ref.Owner, ref.Repo, ref.Number, reason, now)
	if err != nil {
		return fmt.Errorf("record split override: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE cluster_members SET included = 0, updated_at = ?
		WHERE cluster_id = ? AND kind = ? AND owner = ? AND repo = ? AND number = ?
	`, now, clusterID, ref.Kind, ref.Owner, ref.Repo, ref.Number)
	if err != nil {
		return fmt.Errorf("exclude split member: %w", err)
	}

	return tx.Commit()
}

func (s *Store) getClusterByID(ctx context.Context, id int64) (*Cluster, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, stable_id, state, canonical_kind, canonical_owner, canonical_repo, canonical_number,
		       source_revision, source_window_start, source_window_end, created_at, updated_at,
		       repo_owner, repo_name
		FROM clusters
		WHERE id = ?
	`, id)
	var c Cluster
	var created, updated, winStart, winEnd int64
	var state string
	err := row.Scan(&c.ID, &c.StableID, &state, &c.Canonical.Kind, &c.Canonical.Owner, &c.Canonical.Repo, &c.Canonical.Number,
		&c.Revision, &winStart, &winEnd, &created, &updated, &c.Repo.Owner, &c.Repo.Repo)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.State = ClusterState(state)
	c.WindowStart = scanTime(winStart)
	c.WindowEnd = scanTime(winEnd)
	c.CreatedAt = scanTime(created)
	c.UpdatedAt = scanTime(updated)
	members, err := s.listMembersForCluster(ctx, c.ID, 0)
	if err != nil {
		return nil, err
	}
	c.Members = members
	return &c, nil
}

// Report returns a concise cluster report for a repository.
func (s *Store) Report(ctx context.Context, repo domain.RepoRef, limit int) (*ClusterReport, error) {
	clusters, err := s.ListClusters(ctx, repo, ClusterOpen, limit)
	if err != nil {
		return nil, err
	}
	return &ClusterReport{Repo: repo, Clusters: clusters}, nil
}

// ClusterReport is a repository-level view of clusters.
type ClusterReport struct {
	Repo     domain.RepoRef
	Clusters []Cluster
}

func splitLabels(s string) []string {
	if s == "" {
		return nil
	}
	var decoded []string
	if json.Unmarshal([]byte(s), &decoded) == nil {
		return decoded
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func encodeTime(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UTC().UnixNano()
}

func scanTime(nsec int64) time.Time {
	if nsec == 0 {
		return time.Time{}
	}
	return time.Unix(0, nsec).UTC()
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func nullableThreadID(id int64) any {
	if id <= 0 {
		return nil
	}
	return id
}

func validateCandidates(repo domain.RepoRef, candidates []Candidate) error {
	seen := make(map[string]struct{}, len(candidates))
	for i, candidate := range candidates {
		if !strings.EqualFold(candidate.Repo.Owner, repo.Owner) || !strings.EqualFold(candidate.Repo.Repo, repo.Repo) {
			return fmt.Errorf("candidate %d belongs to %s, not %s", i, candidate.Repo.String(), repo.String())
		}
		if err := validateMemberRef(candidate.Ref()); err != nil {
			return fmt.Errorf("candidate %d: %w", i, err)
		}
		key := strings.ToLower(candidate.Ref().String())
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate candidate %s", candidate.Ref().String())
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateMemberRef(ref MemberRef) error {
	if strings.TrimSpace(ref.Owner) == "" || strings.TrimSpace(ref.Repo) == "" {
		return errors.New("member owner and repo are required")
	}
	if ref.Kind != "issue" && ref.Kind != "pull_request" {
		return fmt.Errorf("unsupported member kind %q", ref.Kind)
	}
	if ref.Number <= 0 {
		return errors.New("member number must be positive")
	}
	return nil
}

func sameMemberRef(a, b MemberRef) bool {
	return a.Number == b.Number && strings.EqualFold(a.Kind, b.Kind) &&
		strings.EqualFold(a.Owner, b.Owner) && strings.EqualFold(a.Repo, b.Repo)
}
