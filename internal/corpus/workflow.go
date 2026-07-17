package corpus

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/morluto/gitcontribute/internal/contribution"
	"github.com/morluto/gitcontribute/internal/domain"
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

func (c *Corpus) SaveInvestigation(ctx context.Context, item *investigation.Investigation) error {
	if item == nil || item.ID == "" {
		return errors.New("investigation id is required")
	}
	payload, err := marshalWorkflow(item)
	if err != nil {
		return err
	}
	_, err = c.db.ExecContext(ctx, `
		INSERT INTO investigations (id, repo_owner, repo_name, status, payload, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET repo_owner=excluded.repo_owner, repo_name=excluded.repo_name,
			status=excluded.status, payload=excluded.payload, updated_at=excluded.updated_at
	`, item.ID, item.Repo.Owner, item.Repo.Repo, item.Status, payload, encodeTime(item.CreatedAt), encodeTime(item.UpdatedAt))
	if err != nil {
		return fmt.Errorf("save investigation: %w", err)
	}
	return nil
}

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

func (c *Corpus) ListInvestigations(ctx context.Context) ([]*investigation.Investigation, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT payload FROM investigations ORDER BY created_at, id LIMIT 10000`)
	if err != nil {
		return nil, fmt.Errorf("list investigations: %w", err)
	}
	defer rows.Close()
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

func (c *Corpus) ListHypotheses(ctx context.Context, investigationID string) ([]*investigation.Hypothesis, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT payload FROM hypotheses WHERE investigation_id=? ORDER BY created_at, id LIMIT 10000`, investigationID)
	if err != nil {
		return nil, fmt.Errorf("list hypotheses: %w", err)
	}
	defer rows.Close()
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

// PromoteHypothesis atomically stores the promoted hypothesis and its new
// opportunity so a partial write cannot strand the hypothesis.
func (c *Corpus) PromoteHypothesis(ctx context.Context, hypothesis *investigation.Hypothesis, opportunity *investigation.Opportunity) error {
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
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit hypothesis promotion: %w", err)
	}
	return nil
}

func (c *Corpus) GetOpportunity(ctx context.Context, id string) (*investigation.Opportunity, error) {
	var payload string
	err := c.db.QueryRowContext(ctx, `SELECT payload FROM opportunities WHERE id=?`, id).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, investigation.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get opportunity: %w", err)
	}
	var item investigation.Opportunity
	if err := unmarshalWorkflow(payload, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

func (c *Corpus) ListOpportunities(ctx context.Context, investigationID string) ([]*investigation.Opportunity, error) {
	query := `SELECT payload FROM opportunities`
	var args []any
	if investigationID != "" {
		query += ` WHERE investigation_id=?`
		args = append(args, investigationID)
	}
	query += ` ORDER BY created_at, id LIMIT 10000`
	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list opportunities: %w", err)
	}
	defer rows.Close()
	var out []*investigation.Opportunity
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var item investigation.Opportunity
		if err := unmarshalWorkflow(payload, &item); err != nil {
			return nil, err
		}
		out = append(out, &item)
	}
	return out, rows.Err()
}

func (c *Corpus) FindRelated(ctx context.Context, ref domain.RepoRef, category investigation.Category) ([]domain.SourceRef, error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT h.payload FROM hypotheses h JOIN investigations i ON i.id=h.investigation_id
		WHERE i.repo_owner=? AND i.repo_name=? AND (?='' OR h.category=?) ORDER BY h.created_at
	`, ref.Owner, ref.Repo, category, category)
	if err != nil {
		return nil, fmt.Errorf("find related investigations: %w", err)
	}
	defer rows.Close()
	var out []domain.SourceRef
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var item investigation.Hypothesis
		if err := unmarshalWorkflow(payload, &item); err != nil {
			return nil, err
		}
		out = append(out, item.SourceRefs...)
	}
	return out, rows.Err()
}

func (c *Corpus) SaveValidationDefinition(ctx context.Context, item *evidence.ValidationDefinition) error {
	if item == nil || item.ID == "" {
		return errors.New("validation definition id is required")
	}
	payload, err := marshalWorkflow(item)
	if err != nil {
		return err
	}
	_, err = c.db.ExecContext(ctx, `
		INSERT INTO validation_definitions (id, investigation_id, hypothesis_id, opportunity_id, payload, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET investigation_id=excluded.investigation_id,
			hypothesis_id=excluded.hypothesis_id, opportunity_id=excluded.opportunity_id, payload=excluded.payload
	`, item.ID, item.InvestigationID, item.HypothesisID, item.OpportunityID, payload, encodeTime(item.CreatedAt))
	if err != nil {
		return fmt.Errorf("save validation definition: %w", err)
	}
	return nil
}

func (c *Corpus) GetValidationDefinition(ctx context.Context, id string) (*evidence.ValidationDefinition, error) {
	var payload string
	err := c.db.QueryRowContext(ctx, `SELECT payload FROM validation_definitions WHERE id=?`, id).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, evidence.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get validation definition: %w", err)
	}
	var item evidence.ValidationDefinition
	if err := unmarshalWorkflow(payload, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

func (c *Corpus) SaveValidationRun(ctx context.Context, item *evidence.ValidationRun) error {
	if item == nil || item.ID == "" {
		return errors.New("validation run id is required")
	}
	payload, err := marshalWorkflow(item)
	if err != nil {
		return err
	}
	_, err = c.db.ExecContext(ctx, `
		INSERT INTO validation_runs (id, definition_id, investigation_id, hypothesis_id, opportunity_id, kind, classification, payload, started_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET definition_id=excluded.definition_id,
			investigation_id=excluded.investigation_id, hypothesis_id=excluded.hypothesis_id,
			opportunity_id=excluded.opportunity_id, kind=excluded.kind,
			classification=excluded.classification, payload=excluded.payload, completed_at=excluded.completed_at
	`, item.ID, item.DefinitionID, item.InvestigationID, item.HypothesisID, item.OpportunityID, item.Kind, item.Classification, payload, encodeTime(item.StartedAt), encodeTime(item.CompletedAt))
	if err != nil {
		return fmt.Errorf("save validation run: %w", err)
	}
	return nil
}

func (c *Corpus) GetValidationRun(ctx context.Context, id string) (*evidence.ValidationRun, error) {
	var payload string
	err := c.db.QueryRowContext(ctx, `SELECT payload FROM validation_runs WHERE id=?`, id).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, evidence.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get validation run: %w", err)
	}
	var item evidence.ValidationRun
	if err := unmarshalWorkflow(payload, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

func (c *Corpus) SaveEvidence(ctx context.Context, item *evidence.Evidence) error {
	if item == nil || item.ID == "" {
		return errors.New("evidence id is required")
	}
	payload, err := marshalWorkflow(item)
	if err != nil {
		return err
	}
	_, err = c.db.ExecContext(ctx, `
		INSERT INTO evidence (id, investigation_id, hypothesis_id, opportunity_id, relation, evidence_type, payload, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET investigation_id=excluded.investigation_id,
			hypothesis_id=excluded.hypothesis_id, opportunity_id=excluded.opportunity_id,
			relation=excluded.relation, evidence_type=excluded.evidence_type, payload=excluded.payload
	`, item.ID, item.InvestigationID, item.HypothesisID, item.OpportunityID, item.Relation, item.Type, payload, encodeTime(item.CreatedAt))
	if err != nil {
		return fmt.Errorf("save evidence: %w", err)
	}
	return nil
}

func (c *Corpus) CreateEvidence(ctx context.Context, item *evidence.Evidence) error {
	return c.SaveEvidence(ctx, item)
}

func (c *Corpus) ListEvidence(ctx context.Context, filter evidence.EvidenceFilter) ([]*evidence.Evidence, error) {
	query := `SELECT e.payload FROM evidence e WHERE 1=1`
	var args []any
	if filter.InvestigationID != "" {
		query += ` AND (
			e.investigation_id=?
			OR e.opportunity_id IN (SELECT id FROM opportunities WHERE investigation_id=?)
			OR e.hypothesis_id IN (SELECT id FROM hypotheses WHERE investigation_id=?)
		)`
		args = append(args, filter.InvestigationID, filter.InvestigationID, filter.InvestigationID)
	}
	if filter.HypothesisID != "" {
		query += ` AND e.hypothesis_id=?`
		args = append(args, filter.HypothesisID)
	}
	if filter.OpportunityID != "" {
		query += ` AND e.opportunity_id=?`
		args = append(args, filter.OpportunityID)
	}
	if filter.Relation != "" {
		query += ` AND e.relation=?`
		args = append(args, filter.Relation)
	}
	query += ` ORDER BY e.created_at, e.id LIMIT 10000`
	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list evidence: %w", err)
	}
	defer rows.Close()
	var out []*evidence.Evidence
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var item evidence.Evidence
		if err := unmarshalWorkflow(payload, &item); err != nil {
			return nil, err
		}
		out = append(out, &item)
	}
	return out, rows.Err()
}

func (c *Corpus) SaveIssueDraft(ctx context.Context, item *contribution.IssueDraft) error {
	if item == nil {
		return errors.New("issue draft is required")
	}
	return c.saveDraft(ctx, item.OpportunityID, "issue", item, item.RenderedAt)
}
func (c *Corpus) GetIssueDraft(ctx context.Context, opportunityID string) (*contribution.IssueDraft, error) {
	var item contribution.IssueDraft
	if err := c.getDraft(ctx, opportunityID, "issue", &item); err != nil {
		return nil, err
	}
	return &item, nil
}
func (c *Corpus) SavePullRequestDraft(ctx context.Context, item *contribution.PullRequestDraft) error {
	if item == nil {
		return errors.New("pull request draft is required")
	}
	return c.saveDraft(ctx, item.OpportunityID, "pull_request", item, item.RenderedAt)
}
func (c *Corpus) GetPullRequestDraft(ctx context.Context, opportunityID string) (*contribution.PullRequestDraft, error) {
	var item contribution.PullRequestDraft
	if err := c.getDraft(ctx, opportunityID, "pull_request", &item); err != nil {
		return nil, err
	}
	return &item, nil
}

func (c *Corpus) saveDraft(ctx context.Context, opportunityID, kind string, item any, renderedAt time.Time) error {
	if opportunityID == "" {
		return errors.New("draft opportunity id is required")
	}
	payload, err := marshalWorkflow(item)
	if err != nil {
		return err
	}
	_, err = c.db.ExecContext(ctx, `INSERT INTO contribution_drafts (opportunity_id, kind, payload, rendered_at) VALUES (?, ?, ?, ?) ON CONFLICT (opportunity_id, kind) DO UPDATE SET payload=excluded.payload, rendered_at=excluded.rendered_at`, opportunityID, kind, payload, encodeTime(renderedAt))
	if err != nil {
		return fmt.Errorf("save contribution draft: %w", err)
	}
	return nil
}

func (c *Corpus) getDraft(ctx context.Context, opportunityID, kind string, target any) error {
	var payload string
	err := c.db.QueryRowContext(ctx, `SELECT payload FROM contribution_drafts WHERE opportunity_id=? AND kind=?`, opportunityID, kind).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return contribution.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("get contribution draft: %w", err)
	}
	return unmarshalWorkflow(payload, target)
}
