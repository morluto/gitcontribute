package corpus

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/morluto/gitcontribute/internal/contribution"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/investigation"
	"github.com/morluto/gitcontribute/internal/workspace"
)

var (
	_ investigation.Repository    = (*Corpus)(nil)
	_ investigation.EvidenceStore = (*Corpus)(nil)
	_ evidence.Repository         = (*Corpus)(nil)
	_ contribution.Repository     = (*Corpus)(nil)
)

func marshalWorkflow(value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal workflow record: %w", err)
	}
	return string(payload), nil
}

func unmarshalWorkflow(payload string, value any) error {
	if err := json.Unmarshal([]byte(payload), value); err != nil {
		return fmt.Errorf("decode workflow record: %w", err)
	}
	return nil
}

func listWorkflowPayloads[T any](ctx context.Context, db *sql.DB, operation, query string, args ...any) (out []*T, err error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	defer func() {
		if closeErr := rows.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var item T
		if err := unmarshalWorkflow(payload, &item); err != nil {
			return nil, err
		}
		out = append(out, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// SaveWorkspace inserts or replaces a workspace record.
func (c *Corpus) SaveWorkspace(ctx context.Context, item *workspace.Workspace) error {
	if item == nil || item.Name == "" {
		return errors.New("workspace name is required")
	}
	payload, err := marshalWorkflow(item)
	if err != nil {
		return err
	}
	_, err = c.db.ExecContext(ctx, `
		INSERT INTO workspaces (id, investigation_id, payload, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET investigation_id=excluded.investigation_id,
			payload=excluded.payload, created_at=excluded.created_at
	`, item.Name, item.InvestigationID, payload, encodeTime(item.CreatedAt))
	if err != nil {
		return fmt.Errorf("save workspace: %w", err)
	}
	return nil
}

// GetWorkspace returns a workspace by ID.
func (c *Corpus) GetWorkspace(ctx context.Context, id string) (*workspace.Workspace, error) {
	var payload string
	var createdAt int64
	err := c.db.QueryRowContext(ctx, `SELECT payload, created_at FROM workspaces WHERE id=?`, id).Scan(&payload, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, workspace.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get workspace: %w", err)
	}
	var item workspace.Workspace
	if err := unmarshalWorkflow(payload, &item); err != nil {
		return nil, err
	}
	item.CreatedAt = scanTime(createdAt)
	return &item, nil
}

// BindWorkspacePath atomically returns an existing exact path binding or
// inserts item. The immediate transaction prevents concurrent adopters from
// assigning the same external path to different workspace IDs.
func (c *Corpus) BindWorkspacePath(ctx context.Context, item *workspace.Workspace) (bound *workspace.Workspace, inserted bool, err error) {
	if item == nil || item.Name == "" || item.Path == "" {
		return nil, false, errors.New("workspace name and path are required")
	}
	conn, err := c.db.Conn(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("reserve workspace binding connection: %w", err)
	}
	defer func() {
		if closeErr := conn.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close workspace binding connection: %w", closeErr)
		}
	}()
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return nil, false, fmt.Errorf("begin workspace binding: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			if _, rollbackErr := conn.ExecContext(context.WithoutCancel(ctx), `ROLLBACK`); err == nil && rollbackErr != nil {
				err = fmt.Errorf("rollback workspace binding: %w", rollbackErr)
			}
		}
	}()

	var payload string
	var createdAt int64
	err = conn.QueryRowContext(ctx, `
		SELECT payload, created_at FROM workspaces
		WHERE json_extract(payload, '$.Path')=?
		LIMIT 1
	`, item.Path).Scan(&payload, &createdAt)
	if err == nil {
		var existing workspace.Workspace
		if err := unmarshalWorkflow(payload, &existing); err != nil {
			return nil, false, err
		}
		existing.CreatedAt = scanTime(createdAt)
		return &existing, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, fmt.Errorf("find workspace by path: %w", err)
	}
	var exists int
	if err := conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM workspaces WHERE id=?)`, item.Name).Scan(&exists); err != nil {
		return nil, false, fmt.Errorf("check workspace name: %w", err)
	}
	if exists != 0 {
		return nil, false, workspace.ErrExists
	}
	payload, err = marshalWorkflow(item)
	if err != nil {
		return nil, false, err
	}
	if _, err := conn.ExecContext(ctx, `
		INSERT INTO workspaces (id, investigation_id, payload, created_at)
		VALUES (?, ?, ?, ?)
	`, item.Name, item.InvestigationID, payload, encodeTime(item.CreatedAt)); err != nil {
		return nil, false, fmt.Errorf("bind workspace path: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return nil, false, fmt.Errorf("commit workspace binding: %w", err)
	}
	committed = true
	result := *item
	return &result, true, nil
}

// SaveInvestigation inserts or updates an investigation record.
func (c *Corpus) SaveInvestigation(ctx context.Context, item *investigation.Investigation) error {
	if item == nil || item.ID == "" {
		return errors.New("investigation id is required")
	}
	payload, err := marshalWorkflow(item)
	if err != nil {
		return err
	}
	_, err = c.db.ExecContext(ctx, `
		INSERT INTO investigations (id, repo_owner, repo_name, status, origin_key, payload, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET repo_owner=excluded.repo_owner, repo_name=excluded.repo_name,
			status=excluded.status, origin_key=excluded.origin_key, payload=excluded.payload, updated_at=excluded.updated_at
	`, item.ID, item.Repo.Owner, item.Repo.Repo, item.Status, investigationOriginKey(item), payload, encodeTime(item.CreatedAt), encodeTime(item.UpdatedAt))
	if err != nil {
		return fmt.Errorf("save investigation: %w", err)
	}
	return nil
}

// GetInvestigation returns an investigation by ID, or nil when absent.
func (c *Corpus) GetInvestigation(ctx context.Context, id string) (*investigation.Investigation, error) {
	var payload string
	err := c.db.QueryRowContext(ctx, `SELECT payload FROM investigations WHERE id=?`, id).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, investigation.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get investigation: %w", err)
	}
	var item investigation.Investigation
	if err := unmarshalWorkflow(payload, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

// ListInvestigations returns investigations in deterministic creation order.
func (c *Corpus) ListInvestigations(ctx context.Context) ([]*investigation.Investigation, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT payload FROM investigations ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list investigations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*investigation.Investigation
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var item investigation.Investigation
		if err := unmarshalWorkflow(payload, &item); err != nil {
			return nil, err
		}
		out = append(out, &item)
	}
	return out, rows.Err()
}

// SaveHypothesis inserts or updates a hypothesis and its structured fields.
func (c *Corpus) SaveHypothesis(ctx context.Context, item *investigation.Hypothesis) error {
	if item == nil || item.ID == "" {
		return errors.New("hypothesis id is required")
	}
	payload, err := marshalWorkflow(item)
	if err != nil {
		return err
	}
	_, err = c.db.ExecContext(ctx, `
		INSERT INTO hypotheses (id, investigation_id, category, status, payload, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET investigation_id=excluded.investigation_id,
			category=excluded.category, status=excluded.status, payload=excluded.payload, updated_at=excluded.updated_at
	`, item.ID, item.InvestigationID, item.Category, item.Status, payload, encodeTime(item.CreatedAt), encodeTime(item.UpdatedAt))
	if err != nil {
		return fmt.Errorf("save hypothesis: %w", err)
	}
	return nil
}

// UpdateHypothesis conditionally replaces the exact revision read by the
// caller, preserving concurrent status and audit updates.
func (c *Corpus) UpdateHypothesis(ctx context.Context, previous, next *investigation.Hypothesis) error {
	if previous == nil || next == nil || previous.ID == "" || previous.ID != next.ID {
		return errors.New("matching hypothesis revisions are required")
	}
	previousPayload, err := marshalWorkflow(previous)
	if err != nil {
		return err
	}
	nextPayload, err := marshalWorkflow(next)
	if err != nil {
		return err
	}
	result, err := c.db.ExecContext(ctx, `
		UPDATE hypotheses SET investigation_id=?, category=?, status=?, payload=?, updated_at=?
		WHERE id=? AND payload=?
	`, next.InvestigationID, next.Category, next.Status, nextPayload,
		encodeTime(next.UpdatedAt), next.ID, previousPayload)
	if err != nil {
		return fmt.Errorf("update hypothesis: %w", err)
	}
	return requireWorkflowRevision(result)
}

// GetHypothesis returns a hypothesis by ID, or nil when absent.
func (c *Corpus) GetHypothesis(ctx context.Context, id string) (*investigation.Hypothesis, error) {
	var payload string
	err := c.db.QueryRowContext(ctx, `SELECT payload FROM hypotheses WHERE id=?`, id).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, investigation.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get hypothesis: %w", err)
	}
	var item investigation.Hypothesis
	if err := unmarshalWorkflow(payload, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

// ListHypotheses returns hypotheses belonging to an investigation.
func (c *Corpus) ListHypotheses(ctx context.Context, investigationID string) ([]*investigation.Hypothesis, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT payload FROM hypotheses WHERE investigation_id=? ORDER BY created_at, id`, investigationID)
	if err != nil {
		return nil, fmt.Errorf("list hypotheses: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*investigation.Hypothesis
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var item investigation.Hypothesis
		if err := unmarshalWorkflow(payload, &item); err != nil {
			return nil, err
		}
		out = append(out, &item)
	}
	return out, rows.Err()
}

// SaveOpportunity atomically persists an opportunity and its dependencies and
// source references.
func (c *Corpus) SaveOpportunity(ctx context.Context, item *investigation.Opportunity) error {
	if item == nil || item.ID == "" {
		return errors.New("opportunity id is required")
	}
	payload, err := marshalWorkflow(item)
	if err != nil {
		return err
	}
	_, err = c.db.ExecContext(ctx, `
		INSERT INTO opportunities (id, investigation_id, hypothesis_id, category, status, payload, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET investigation_id=excluded.investigation_id,
			hypothesis_id=excluded.hypothesis_id, category=excluded.category, status=excluded.status,
			payload=excluded.payload, updated_at=excluded.updated_at
	`, item.ID, item.InvestigationID, item.HypothesisID, item.Category, item.Status, payload, encodeTime(item.CreatedAt), encodeTime(item.UpdatedAt))
	if err != nil {
		return fmt.Errorf("save opportunity: %w", err)
	}
	return nil
}

// UpdateOpportunity conditionally replaces the exact revision read by the
// caller. For advancing transitions, the same SQL statement also rejects any
// contradicting evidence visible when the status write is serialized.
func (c *Corpus) UpdateOpportunity(ctx context.Context, previous, next *investigation.Opportunity, blockContradicting bool) error {
	if previous == nil || next == nil || previous.ID == "" || previous.ID != next.ID {
		return errors.New("matching opportunity revisions are required")
	}
	previousPayload, err := marshalWorkflow(previous)
	if err != nil {
		return err
	}
	nextPayload, err := marshalWorkflow(next)
	if err != nil {
		return err
	}
	block := 0
	if blockContradicting {
		block = 1
	}
	result, err := c.db.ExecContext(ctx, `
		UPDATE opportunities SET investigation_id=?, hypothesis_id=?, category=?, status=?, payload=?, updated_at=?
		WHERE id=? AND payload=?
		  AND (?=0 OR NOT EXISTS (
			SELECT 1 FROM evidence
			WHERE opportunity_id=? AND relation=?
		  ))
	`, next.InvestigationID, next.HypothesisID, next.Category, next.Status,
		nextPayload, encodeTime(next.UpdatedAt), next.ID, previousPayload,
		block, next.ID, evidence.RelationContradicting)
	if err != nil {
		return fmt.Errorf("update opportunity: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read workflow update result: %w", err)
	}
	if changed == 1 {
		return nil
	}
	if blockContradicting {
		var exists int
		if err := c.db.QueryRowContext(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM evidence
				WHERE opportunity_id=? AND relation=?
			)
		`, next.ID, evidence.RelationContradicting).Scan(&exists); err != nil {
			return fmt.Errorf("check contradicting evidence: %w", err)
		}
		if exists != 0 {
			return investigation.ErrContradictingEvidence
		}
	}
	return investigation.ErrConflict
}

func requireWorkflowRevision(result sql.Result) error {
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read workflow update result: %w", err)
	}
	if changed != 1 {
		return investigation.ErrConflict
	}
	return nil
}

// PromoteHypothesis atomically stores the promoted hypothesis and its new
// opportunity so a partial write cannot strand the hypothesis.
func (c *Corpus) PromoteHypothesis(ctx context.Context, hypothesis *investigation.Hypothesis, opportunity *investigation.Opportunity) error {
	return c.promoteHypothesis(ctx, hypothesis, opportunity, nil)
}

// PromoteHypothesisWithEvidence stores an optional promotion evidence record
// in the same transaction as the promoted hypothesis and opportunity.
func (c *Corpus) PromoteHypothesisWithEvidence(ctx context.Context, hypothesis *investigation.Hypothesis, opportunity *investigation.Opportunity, item *evidence.Evidence) error {
	if opportunity == nil || opportunity.ID == "" {
		return errors.New("promoted opportunity identity is required")
	}
	if item != nil && (item.ID == "" || item.OpportunityID != opportunity.ID) {
		return errors.New("promotion evidence must identify the promoted opportunity")
	}
	return c.promoteHypothesis(ctx, hypothesis, opportunity, item)
}

func (c *Corpus) promoteHypothesis(ctx context.Context, hypothesis *investigation.Hypothesis, opportunity *investigation.Opportunity, item *evidence.Evidence) error {
	if hypothesis == nil || hypothesis.ID == "" || opportunity == nil || opportunity.ID == "" {
		return errors.New("promoted hypothesis and opportunity identities are required")
	}
	hypothesisPayload, err := marshalWorkflow(hypothesis)
	if err != nil {
		return err
	}
	opportunityPayload, err := marshalWorkflow(opportunity)
	if err != nil {
		return err
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin hypothesis promotion: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
		UPDATE hypotheses SET investigation_id=?, category=?, status=?, payload=?, updated_at=?
		WHERE id=? AND status=?
	`, hypothesis.InvestigationID, hypothesis.Category, hypothesis.Status,
		hypothesisPayload, encodeTime(hypothesis.UpdatedAt), hypothesis.ID,
		investigation.HypothesisProposed)
	if err != nil {
		return fmt.Errorf("save promoted hypothesis: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read promoted hypothesis result: %w", err)
	}
	if changed != 1 {
		return fmt.Errorf("%w: hypothesis is no longer proposed", investigation.ErrInvalidTransition)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO opportunities (id, investigation_id, hypothesis_id, category, status, payload, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, opportunity.ID, opportunity.InvestigationID, opportunity.HypothesisID,
		opportunity.Category, opportunity.Status, opportunityPayload,
		encodeTime(opportunity.CreatedAt), encodeTime(opportunity.UpdatedAt)); err != nil {
		return fmt.Errorf("save promoted opportunity: %w", err)
	}
	if item != nil {
		if err := c.insertEvidenceTx(ctx, tx, item); err != nil {
			return fmt.Errorf("save promotion evidence: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit hypothesis promotion: %w", err)
	}
	return nil
}
