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
	"github.com/morluto/gitcontribute/internal/manifest"
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

// SaveWorkspace inserts or replaces a managed workspace record.
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

// GetWorkspace returns a managed workspace by ID, or nil when absent.
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

// GetOpportunity returns an opportunity with its dependencies and provenance.
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

// ListOpportunities returns opportunities belonging to an investigation.
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

// FindRelated returns stored source references related to a repository and category.
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

// SaveValidationDefinition persists a validation plan without executing it.
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

// GetValidationDefinition returns a validation plan by ID, or nil when absent.
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

// ListValidationDefinitions returns validation plans scoped to an opportunity.
func (c *Corpus) ListValidationDefinitions(ctx context.Context, opportunityID string) ([]*evidence.ValidationDefinition, error) {
	return listWorkflowPayloads[evidence.ValidationDefinition](
		ctx, c.db, "list validation definitions",
		`SELECT payload FROM validation_definitions WHERE opportunity_id=? ORDER BY created_at, id LIMIT 10000`,
		opportunityID,
	)
}

// SaveValidationRun persists the bounded result of an authorized validation execution.
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

// GetValidationRun returns a validation result by ID, or nil when absent.
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

// SaveValidationRunGroup persists one bounded repeat/stress aggregate.
func (c *Corpus) SaveValidationRunGroup(ctx context.Context, item *evidence.ValidationRunGroup) error {
	if item == nil || item.ID == "" {
		return errors.New("validation run group id is required")
	}
	payload, err := marshalWorkflow(item)
	if err != nil {
		return err
	}
	_, err = c.db.ExecContext(ctx, `
		INSERT INTO validation_run_groups (
			id, definition_id, investigation_id, opportunity_id, classification,
			requested_runs, completed_runs, payload, started_at, completed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, item.ID, item.DefinitionID, item.InvestigationID, item.OpportunityID, item.Classification,
		item.RequestedRuns, item.CompletedRuns, payload, encodeTime(item.StartedAt), encodeTime(item.CompletedAt))
	if err != nil {
		return fmt.Errorf("save validation run group: %w", err)
	}
	return nil
}

// GetValidationRunGroup returns one persisted repeat/stress aggregate.
func (c *Corpus) GetValidationRunGroup(ctx context.Context, id string) (*evidence.ValidationRunGroup, error) {
	var payload string
	err := c.db.QueryRowContext(ctx, `SELECT payload FROM validation_run_groups WHERE id=?`, id).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, evidence.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get validation run group: %w", err)
	}
	var item evidence.ValidationRunGroup
	if err := unmarshalWorkflow(payload, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

// ListValidationRuns returns validation runs scoped to an opportunity.
func (c *Corpus) ListValidationRuns(ctx context.Context, opportunityID string) ([]*evidence.ValidationRun, error) {
	return listWorkflowPayloads[evidence.ValidationRun](
		ctx, c.db, "list validation runs",
		`SELECT payload FROM validation_runs WHERE opportunity_id=? ORDER BY completed_at, id LIMIT 10000`,
		opportunityID,
	)
}

// SaveIssueDraft persists the latest rendered issue draft for an opportunity.
func (c *Corpus) SaveIssueDraft(ctx context.Context, item *contribution.IssueDraft) error {
	if item == nil {
		return errors.New("issue draft is required")
	}
	return c.saveDraft(ctx, item.OpportunityID, "issue", item, item.RenderedAt)
}

// GetIssueDraft returns the issue draft for an opportunity, or nil when absent.
func (c *Corpus) GetIssueDraft(ctx context.Context, opportunityID string) (*contribution.IssueDraft, error) {
	var item contribution.IssueDraft
	if err := c.getDraft(ctx, opportunityID, "issue", &item); err != nil {
		return nil, err
	}
	return &item, nil
}

// SavePullRequestDraft persists the latest pull-request draft for an opportunity.
func (c *Corpus) SavePullRequestDraft(ctx context.Context, item *contribution.PullRequestDraft) error {
	if item == nil {
		return errors.New("pull request draft is required")
	}
	return c.saveDraft(ctx, item.OpportunityID, "pull_request", item, item.RenderedAt)
}

// GetPullRequestDraft returns the pull-request draft for an opportunity, or nil when absent.
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

// SaveContributionManifest persists one deterministic evidence statement.
func (c *Corpus) SaveContributionManifest(ctx context.Context, item *manifest.Statement, workspaceID, pullRequestRef string) error {
	if item == nil {
		return errors.New("contribution manifest is required")
	}
	if err := item.Validate(); err != nil {
		return err
	}
	payload, err := marshalWorkflow(item)
	if err != nil {
		return err
	}
	_, err = c.db.ExecContext(ctx, `
		INSERT INTO contribution_manifests (id, opportunity_id, workspace_id, pull_request_ref, content_sha256, payload, generated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET workspace_id=excluded.workspace_id,
			pull_request_ref=excluded.pull_request_ref, content_sha256=excluded.content_sha256, payload=excluded.payload,
			generated_at=excluded.generated_at
	`, item.Predicate.ManifestID, item.Predicate.Opportunity.ID, workspaceID, pullRequestRef,
		item.Predicate.ContentSHA256, payload, encodeTime(item.Predicate.GeneratedAt))
	if err != nil {
		return fmt.Errorf("save contribution manifest: %w", err)
	}
	return nil
}

// GetContributionManifest reads one persisted evidence statement.
func (c *Corpus) GetContributionManifest(ctx context.Context, id string) (*manifest.Statement, error) {
	var payload string
	err := c.db.QueryRowContext(ctx, `SELECT payload FROM contribution_manifests WHERE id=?`, id).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, manifest.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get contribution manifest: %w", err)
	}
	var item manifest.Statement
	if err := unmarshalWorkflow(payload, &item); err != nil {
		return nil, err
	}
	if err := item.Validate(); err != nil {
		return nil, fmt.Errorf("validate contribution manifest: %w", err)
	}
	return &item, nil
}

// LatestContributionManifest reads the newest manifest for an opportunity.
func (c *Corpus) LatestContributionManifest(ctx context.Context, opportunityID string) (*manifest.Statement, error) {
	var payload string
	err := c.db.QueryRowContext(ctx, `SELECT payload FROM contribution_manifests WHERE opportunity_id=? ORDER BY generated_at DESC, id LIMIT 1`, opportunityID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, manifest.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get latest contribution manifest: %w", err)
	}
	var item manifest.Statement
	if err := unmarshalWorkflow(payload, &item); err != nil {
		return nil, err
	}
	if err := item.Validate(); err != nil {
		return nil, fmt.Errorf("validate contribution manifest: %w", err)
	}
	return &item, nil
}
