package corpus

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/morluto/gitcontribute/internal/concern"
	"github.com/morluto/gitcontribute/internal/investigation"
)

var _ concern.Repository = (*Corpus)(nil)

// SaveConcern stores one validated concern and updates its FTS document through
// database triggers.
func (c *Corpus) SaveConcern(ctx context.Context, item *concern.Concern) error {
	if item == nil || item.ID == "" {
		return errors.New("concern id is required")
	}
	payload, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("marshal concern: %w", err)
	}
	_, err = c.db.ExecContext(ctx, `
		INSERT INTO concerns (
			id, repo_owner, repo_name, commit_sha, workspace_id, title,
			problem_statement, suspected_owner, unknowns, success_criterion,
			status, confidence, payload, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			repo_owner=excluded.repo_owner, repo_name=excluded.repo_name,
			commit_sha=excluded.commit_sha, workspace_id=excluded.workspace_id,
			title=excluded.title, problem_statement=excluded.problem_statement,
			suspected_owner=excluded.suspected_owner, unknowns=excluded.unknowns,
			success_criterion=excluded.success_criterion, status=excluded.status,
			confidence=excluded.confidence, payload=excluded.payload,
			updated_at=excluded.updated_at
	`, item.ID, item.Repo.Owner, item.Repo.Repo, item.CommitSHA, item.WorkspaceID,
		item.Title, item.ProblemStatement, item.SuspectedOwner, strings.Join(item.Unknowns, "\n"),
		item.SuccessCriterion, item.Status, item.Confidence, payload,
		encodeTime(item.CreatedAt), encodeTime(item.UpdatedAt))
	if err != nil {
		return fmt.Errorf("save concern: %w", err)
	}
	return nil
}

// GetConcern returns one local concern with its explicit links.
func (c *Corpus) GetConcern(ctx context.Context, id string) (*concern.Concern, error) {
	var payload string
	err := c.db.QueryRowContext(ctx, `SELECT payload FROM concerns WHERE id=?`, id).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, concern.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get concern: %w", err)
	}
	item, err := decodeConcern(payload)
	if err != nil {
		return nil, err
	}
	item.Links, err = c.listConcernLinks(ctx, item.ID)
	if err != nil {
		return nil, err
	}
	return item, nil
}

// ListConcerns performs a bounded offline FTS5 search or updated-order list.
func (c *Corpus) ListConcerns(ctx context.Context, filter concern.Filter) (_ []*concern.Concern, err error) {
	query := literalFTSQuery(filter.Query)
	from, where := "FROM concerns c", []string{"1=1"}
	args := make([]any, 0, 4)
	order := "c.updated_at DESC, c.id"
	if query != "" {
		from = "FROM concerns_fts JOIN concerns c ON c.rowid=concerns_fts.rowid"
		where = append(where, "concerns_fts MATCH ?")
		args = append(args, query)
		order = "bm25(concerns_fts, 10.0, 5.0, 2.0, 1.0, 1.0), c.updated_at DESC, c.id"
	}
	if filter.Repo.Owner != "" {
		where = append(where, "c.repo_owner=? COLLATE NOCASE", "c.repo_name=? COLLATE NOCASE")
		args = append(args, filter.Repo.Owner, filter.Repo.Repo)
	}
	if filter.Status != "" {
		where = append(where, "c.status=?")
		args = append(args, filter.Status)
	}
	args = append(args, filter.Limit)
	rows, err := c.db.QueryContext(ctx, `SELECT c.payload `+from+` WHERE `+strings.Join(where, " AND ")+` ORDER BY `+order+` LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("list concerns: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close concern rows: %w", closeErr)
		}
	}()
	var items []*concern.Concern
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		item, err := decodeConcern(payload)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// AddConcernLink idempotently stores one typed relationship.
func (c *Corpus) AddConcernLink(ctx context.Context, id string, link concern.Link) error {
	result, err := c.db.ExecContext(ctx, `
		INSERT INTO concern_links (concern_id, kind, target_type, target_id, note, created_at)
		SELECT ?, ?, ?, ?, ?, ? WHERE EXISTS (SELECT 1 FROM concerns WHERE id=?)
		ON CONFLICT (concern_id, kind, target_type, target_id)
		DO UPDATE SET note=excluded.note
	`, id, link.Kind, link.TargetType, link.TargetID, link.Note, encodeTime(link.CreatedAt), id)
	if err != nil {
		return fmt.Errorf("add concern link: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read concern link result: %w", err)
	}
	if rows == 0 {
		return concern.ErrNotFound
	}
	return nil
}

func (c *Corpus) listConcernLinks(ctx context.Context, id string) (links []concern.Link, err error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT kind, target_type, target_id, note, created_at
		FROM concern_links WHERE concern_id=?
		ORDER BY kind, target_type, target_id
	`, id)
	if err != nil {
		return nil, fmt.Errorf("list concern links: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close concern link rows: %w", closeErr)
		}
	}()
	for rows.Next() {
		var link concern.Link
		var createdAt int64
		if err := rows.Scan(&link.Kind, &link.TargetType, &link.TargetID, &link.Note, &createdAt); err != nil {
			return nil, err
		}
		link.CreatedAt = scanTime(createdAt)
		links = append(links, link)
	}
	return links, rows.Err()
}

func decodeConcern(payload string) (*concern.Concern, error) {
	var item concern.Concern
	if err := json.Unmarshal([]byte(payload), &item); err != nil {
		return nil, fmt.Errorf("decode concern: %w", err)
	}
	return &item, nil
}

// PromoteConcern atomically creates the downstream workflow and marks the
// concern promoted. A nil opportunity promotes only to an investigation.
func (c *Corpus) PromoteConcern(ctx context.Context, id string, inv *investigation.Investigation, hypothesis *investigation.Hypothesis, opportunity *investigation.Opportunity) (_ *concern.Concern, err error) {
	if err := validateConcernPromotion(inv, hypothesis, opportunity); err != nil {
		return nil, err
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin concern promotion: %w", err)
	}
	defer rollbackSQLOnReturn(tx, &err)
	if err := c.promoteConcernTx(ctx, tx, id, inv, hypothesis, opportunity); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit concern promotion: %w", err)
	}
	return c.GetConcern(ctx, id)
}

func (c *Corpus) promoteConcernTx(ctx context.Context, tx *sql.Tx, id string, inv *investigation.Investigation, hypothesis *investigation.Hypothesis, opportunity *investigation.Opportunity) error {
	var payload string
	if err := tx.QueryRowContext(ctx, `SELECT payload FROM concerns WHERE id=?`, id).Scan(&payload); errors.Is(err, sql.ErrNoRows) {
		return concern.ErrNotFound
	} else if err != nil {
		return fmt.Errorf("read promoted concern: %w", err)
	}
	item, err := decodeConcern(payload)
	if err != nil {
		return err
	}
	if item.Status != concern.StatusAccepted && item.Status != concern.StatusInvestigating {
		return fmt.Errorf("%w: %s to promoted", concern.ErrInvalidTransition, item.Status)
	}
	if err := insertConcernWorkflowTx(ctx, tx, inv, hypothesis, opportunity); err != nil {
		return err
	}
	promotion := &concern.Promotion{Kind: "investigation", InvestigationID: inv.ID, HypothesisID: hypothesis.ID, PromotedAt: inv.CreatedAt}
	if opportunity != nil {
		promotion.Kind, promotion.OpportunityID = "opportunity", opportunity.ID
	}
	previous := item.Status
	item.Status, item.Promotion, item.UpdatedAt = concern.StatusPromoted, promotion, inv.CreatedAt
	item.AuditTrail = append(item.AuditTrail, concern.StatusChange{From: previous, To: concern.StatusPromoted, Rationale: "promoted to " + promotion.Kind, At: inv.CreatedAt})
	updatedPayload, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("marshal promoted concern: %w", err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE concerns SET status=?, payload=?, updated_at=? WHERE id=? AND status=?`, item.Status, updatedPayload, encodeTime(item.UpdatedAt), id, previous)
	if err != nil {
		return fmt.Errorf("mark concern promoted: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return fmt.Errorf("%w: concern changed during promotion", concern.ErrInvalidTransition)
	}
	links := []concern.Link{{Kind: concern.LinkInvestigation, TargetType: "investigation", TargetID: inv.ID, CreatedAt: inv.CreatedAt}}
	if opportunity != nil {
		links = append(links, concern.Link{Kind: concern.LinkOpportunity, TargetType: "opportunity", TargetID: opportunity.ID, CreatedAt: inv.CreatedAt})
	}
	for _, link := range links {
		if _, err := tx.ExecContext(ctx, `INSERT INTO concern_links (concern_id, kind, target_type, target_id, note, created_at) VALUES (?, ?, ?, ?, '', ?)`, id, link.Kind, link.TargetType, link.TargetID, encodeTime(link.CreatedAt)); err != nil {
			return fmt.Errorf("link concern promotion: %w", err)
		}
	}
	return nil
}

func validateConcernPromotion(inv *investigation.Investigation, hypothesis *investigation.Hypothesis, opportunity *investigation.Opportunity) error {
	if inv == nil || hypothesis == nil || inv.ID == "" || hypothesis.ID == "" || hypothesis.InvestigationID != inv.ID {
		return errors.New("promotion investigation and hypothesis are required")
	}
	if opportunity != nil && (opportunity.ID == "" || opportunity.InvestigationID != inv.ID || opportunity.HypothesisID != hypothesis.ID) {
		return errors.New("promotion opportunity does not match investigation hypothesis")
	}
	return nil
}

func insertConcernWorkflowTx(ctx context.Context, tx *sql.Tx, inv *investigation.Investigation, hypothesis *investigation.Hypothesis, opportunity *investigation.Opportunity) error {
	invPayload, err := marshalWorkflow(inv)
	if err != nil {
		return err
	}
	hypothesisPayload, err := marshalWorkflow(hypothesis)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO investigations (id, repo_owner, repo_name, status, origin_key, payload, created_at, updated_at)
		VALUES (?, ?, ?, ?, '', ?, ?, ?)
	`, inv.ID, inv.Repo.Owner, inv.Repo.Repo, inv.Status, invPayload, encodeTime(inv.CreatedAt), encodeTime(inv.UpdatedAt)); err != nil {
		return fmt.Errorf("save concern investigation: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO hypotheses (id, investigation_id, category, status, payload, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, hypothesis.ID, hypothesis.InvestigationID, hypothesis.Category, hypothesis.Status, hypothesisPayload, encodeTime(hypothesis.CreatedAt), encodeTime(hypothesis.UpdatedAt)); err != nil {
		return fmt.Errorf("save concern hypothesis: %w", err)
	}
	if opportunity == nil {
		return nil
	}
	opportunityPayload, err := marshalWorkflow(opportunity)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO opportunities (id, investigation_id, hypothesis_id, category, status, payload, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, opportunity.ID, opportunity.InvestigationID, opportunity.HypothesisID, opportunity.Category, opportunity.Status, opportunityPayload, encodeTime(opportunity.CreatedAt), encodeTime(opportunity.UpdatedAt)); err != nil {
		return fmt.Errorf("save concern opportunity: %w", err)
	}
	return nil
}
