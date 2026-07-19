package corpus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/clustering"
	"github.com/morluto/gitcontribute/internal/domain"
)

// AddClusterOverride validates and records one explicit membership decision,
// then advances the repository governance revision in the same transaction.
// It does not recompute clusters; the next explicit refresh applies the
// decision. The canonical member cannot be excluded.
func (c *Corpus) AddClusterOverride(ctx context.Context, clusterID int64, ref clustering.MemberRef, action clustering.OverrideAction, reason string) (err error) {
	if clusterID < 1 {
		return errors.New("cluster id is required")
	}
	if err := validateClusterMemberRef(ref); err != nil {
		return err
	}
	switch action {
	case clustering.OverrideInclude, clustering.OverrideExclude, clustering.OverrideSetCanonical:
	default:
		return fmt.Errorf("unsupported override action %q", action)
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return errors.New("reason is required")
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackSQLOnReturn(tx, &err)
	var repo domain.RepoRef
	var canonical clustering.MemberRef
	err = tx.QueryRowContext(ctx, `SELECT repo_owner, repo_name, canonical_kind, canonical_owner, canonical_repo, canonical_number
		FROM clusters WHERE id=?`, clusterID).Scan(&repo.Owner, &repo.Repo, &canonical.Kind, &canonical.Owner, &canonical.Repo, &canonical.Number)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("cluster %d not found", clusterID)
	}
	if err != nil {
		return err
	}
	if action == clustering.OverrideExclude && sameClusterMemberRef(ref, canonical) {
		return errors.New("cannot exclude the canonical member")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO cluster_overrides
		(cluster_id, kind, owner, repo, number, action, reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, clusterID, ref.Kind, ref.Owner, ref.Repo, ref.Number, string(action), reason, encodeTime(time.Now().UTC())); err != nil {
		return fmt.Errorf("insert cluster override: %w", err)
	}
	if err := advanceClusterGovernanceTx(ctx, tx, repo); err != nil {
		return err
	}
	return tx.Commit()
}

func advanceClusterGovernanceTx(ctx context.Context, tx *sql.Tx, repo domain.RepoRef) error {
	owner, name := strings.ToLower(repo.Owner), strings.ToLower(repo.Repo)
	if _, err := tx.ExecContext(ctx, `INSERT INTO cluster_projection_state (repo_owner, repo_name, governance_revision)
		VALUES (?, ?, 1)
		ON CONFLICT(repo_owner, repo_name) DO UPDATE SET governance_revision=governance_revision+1`, owner, name); err != nil {
		return fmt.Errorf("advance cluster governance revision: %w", err)
	}
	return nil
}

func validateClusterMemberRef(ref clustering.MemberRef) error {
	if strings.TrimSpace(ref.Owner) == "" || strings.TrimSpace(ref.Repo) == "" {
		return errors.New("member owner and repo are required")
	}
	if ref.Kind != ThreadKindIssue && ref.Kind != ThreadKindPullRequest {
		return fmt.Errorf("unsupported member kind %q", ref.Kind)
	}
	if ref.Number < 1 {
		return errors.New("member number must be positive")
	}
	return nil
}

func sameClusterMemberRef(a, b clustering.MemberRef) bool {
	return a.Number == b.Number && a.Kind == b.Kind && strings.EqualFold(a.Owner, b.Owner) && strings.EqualFold(a.Repo, b.Repo)
}
