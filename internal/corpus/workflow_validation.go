package corpus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/morluto/gitcontribute/internal/contribution"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/investigation"
	"github.com/morluto/gitcontribute/internal/manifest"
)

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
	query += ` ORDER BY created_at, id`
	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list opportunities: %w", err)
	}
	defer func() { _ = rows.Close() }()
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
	defer func() { _ = rows.Close() }()
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
		`SELECT payload FROM validation_definitions WHERE opportunity_id=? ORDER BY created_at, id`,
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
		`SELECT payload FROM validation_runs WHERE opportunity_id=? ORDER BY completed_at, id`,
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
