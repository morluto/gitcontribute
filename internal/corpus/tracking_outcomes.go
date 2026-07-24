package corpus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/investigation"
	"github.com/morluto/gitcontribute/internal/tracking"
)

func (c *Corpus) exportContributionOutcomes(ctx context.Context, db dbQueryer, bundle *tracking.Bundle) error {
	rows, err := db.QueryContext(ctx, `
		SELECT o.id, o.contribution_id, o.outcome, o.reason, o.source_event_at, o.created_at
		FROM contribution_outcomes AS o
		JOIN contributions AS selected ON selected.id = o.contribution_id
		ORDER BY o.source_event_at, o.id
	`)
	if err != nil {
		return fmt.Errorf("export contribution outcomes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var o tracking.ContributionOutcome
		var sourceEventAt, createdAt int64
		if err := rows.Scan(&o.ID, &o.ContributionID, &o.Outcome, &o.Reason, &sourceEventAt, &createdAt); err != nil {
			return err
		}
		o.SourceEventAt = scanTime(sourceEventAt)
		o.CreatedAt = scanTime(createdAt)
		bundle.ContributionOutcomes = append(bundle.ContributionOutcomes, &o)
	}
	return rows.Err()
}

// ImportLocalMetadata imports a bounded bundle idempotently. All writes happen
// in a single transaction; any referential or database failure leaves the
// corpus unchanged.
func (c *Corpus) ImportLocalMetadata(ctx context.Context, bundle *tracking.Bundle) error {
	if bundle == nil {
		return errors.New("bundle is required")
	}
	for i, e := range bundle.TriageEvents {
		if e == nil {
			return fmt.Errorf("triage event %d is null", i)
		}
	}
	for i, item := range bundle.Contributions {
		if item == nil {
			return fmt.Errorf("contribution %d is null", i)
		}
	}
	for i, o := range bundle.ContributionOutcomes {
		if o == nil {
			return fmt.Errorf("contribution outcome %d is null", i)
		}
	}
	for i, item := range bundle.Evidence {
		if item == nil {
			return fmt.Errorf("evidence %d is null", i)
		}
		if err := c.validateImportedEvidenceScope(ctx, item); err != nil {
			return fmt.Errorf("resolve evidence %q scope: %w", item.ID, err)
		}
	}

	// Resolve triage links and validate contribution/outcome references outside
	// the write transaction so the transaction contains only upserts and can be
	// rolled back atomically on any failure.
	resolvedEvents := make([]*tracking.TriageEvent, len(bundle.TriageEvents))
	for i, e := range bundle.TriageEvents {
		cp := *e
		if err := resolveTriageLinks(ctx, c, &cp); err != nil {
			return fmt.Errorf("resolve triage event %q: %w", cp.ID, err)
		}
		resolvedEvents[i] = &cp
	}

	for _, item := range bundle.Contributions {
		if _, err := c.GetOpportunity(ctx, item.OpportunityID); err != nil {
			if errors.Is(err, investigation.ErrNotFound) {
				return fmt.Errorf("opportunity %q not found for contribution %q", item.OpportunityID, item.ID)
			}
			return fmt.Errorf("resolve contribution %q opportunity: %w", item.ID, err)
		}
	}

	contributionIDs := make(map[string]struct{}, len(bundle.Contributions))
	for _, item := range bundle.Contributions {
		contributionIDs[item.ID] = struct{}{}
	}
	for _, o := range bundle.ContributionOutcomes {
		if _, ok := contributionIDs[o.ContributionID]; ok {
			continue
		}
		contrib, err := c.GetContribution(ctx, o.ContributionID)
		if err != nil {
			return fmt.Errorf("resolve outcome %q contribution: %w", o.ID, err)
		}
		if contrib == nil {
			return fmt.Errorf("contribution %q not found for outcome %q", o.ContributionID, o.ID)
		}
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin import local metadata: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, e := range resolvedEvents {
		if err := c.importTriageEventTx(ctx, tx, e); err != nil {
			return fmt.Errorf("import triage event %q: %w", e.ID, err)
		}
	}
	for _, item := range bundle.Contributions {
		if err := c.importContributionTx(ctx, tx, item); err != nil {
			return fmt.Errorf("import contribution %q: %w", item.ID, err)
		}
	}
	for _, o := range bundle.ContributionOutcomes {
		if err := c.importContributionOutcomeTx(ctx, tx, o); err != nil {
			return fmt.Errorf("import contribution outcome %q: %w", o.ID, err)
		}
	}
	for _, item := range bundle.Evidence {
		if err := c.importEvidenceTx(ctx, tx, item); err != nil {
			return fmt.Errorf("import evidence %q: %w", item.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit import local metadata: %w", err)
	}
	return nil
}

func (c *Corpus) importEvidenceTx(ctx context.Context, tx *sql.Tx, item *evidence.Evidence) error {
	payload, provenance, err := evidenceStorage(item)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO evidence (id, investigation_id, hypothesis_id, opportunity_id, relation, evidence_type, payload, created_at, source_provenance)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO NOTHING
	`, item.ID, item.InvestigationID, item.HypothesisID, item.OpportunityID,
		item.Relation, item.Type, payload, encodeTime(item.CreatedAt), provenance)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed == 1 {
		return err
	}
	var same int
	if err := tx.QueryRowContext(ctx, `
		SELECT investigation_id IS ? AND hypothesis_id IS ? AND opportunity_id IS ?
		       AND relation=? AND evidence_type=? AND payload=?
		       AND created_at=? AND source_provenance=?
		FROM evidence WHERE id=?
	`, nullString(item.InvestigationID), nullString(item.HypothesisID),
		nullString(item.OpportunityID), item.Relation, item.Type, payload,
		encodeTime(item.CreatedAt), provenance, item.ID).Scan(&same); err != nil {
		return err
	}
	if same == 0 {
		return fmt.Errorf("%w: evidence %q has different content", tracking.ErrImportConflict, item.ID)
	}
	return nil
}

func (c *Corpus) validateImportedEvidenceScope(ctx context.Context, item *evidence.Evidence) error {
	if item.InvestigationID != "" {
		if _, err := c.GetInvestigation(ctx, item.InvestigationID); err != nil {
			return err
		}
	}
	if item.HypothesisID != "" {
		hypothesis, err := c.GetHypothesis(ctx, item.HypothesisID)
		if err != nil {
			return err
		}
		if item.InvestigationID != "" && hypothesis.InvestigationID != item.InvestigationID {
			return errors.New("hypothesis belongs to another investigation")
		}
	}
	if item.OpportunityID != "" {
		opportunity, err := c.GetOpportunity(ctx, item.OpportunityID)
		if err != nil {
			return err
		}
		if item.InvestigationID != "" && opportunity.InvestigationID != item.InvestigationID {
			return errors.New("opportunity belongs to another investigation")
		}
		if item.HypothesisID != "" && opportunity.HypothesisID != item.HypothesisID {
			return errors.New("opportunity belongs to another hypothesis")
		}
	}
	return nil
}
