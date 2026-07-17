package corpus

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/morluto/gitcontribute/internal/evidence"
)

// SaveEvidence inserts or updates an evidence record and its provenance.
func (c *Corpus) SaveEvidence(ctx context.Context, item *evidence.Evidence) error {
	if err := c.saveEvidenceTx(ctx, c.db, item); err != nil {
		return fmt.Errorf("save evidence: %w", err)
	}
	return nil
}

func (c *Corpus) saveEvidenceTx(ctx context.Context, db dbExecer, item *evidence.Evidence) error {
	payload, provenance, err := evidenceStorage(item)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO evidence (id, investigation_id, hypothesis_id, opportunity_id, relation, evidence_type, payload, created_at, source_provenance)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET investigation_id=excluded.investigation_id,
			hypothesis_id=excluded.hypothesis_id, opportunity_id=excluded.opportunity_id,
			relation=excluded.relation, evidence_type=excluded.evidence_type,
			payload=excluded.payload, source_provenance=excluded.source_provenance
	`, item.ID, item.InvestigationID, item.HypothesisID, item.OpportunityID, item.Relation, item.Type, payload, encodeTime(item.CreatedAt), provenance)
	return err
}

func (c *Corpus) insertEvidenceTx(ctx context.Context, tx *sql.Tx, item *evidence.Evidence) error {
	payload, provenance, err := evidenceStorage(item)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO evidence (id, investigation_id, hypothesis_id, opportunity_id, relation, evidence_type, payload, created_at, source_provenance)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, item.ID, item.InvestigationID, item.HypothesisID, item.OpportunityID, item.Relation, item.Type, payload, encodeTime(item.CreatedAt), provenance)
	return err
}

func evidenceStorage(item *evidence.Evidence) (string, string, error) {
	if item == nil || item.ID == "" {
		return "", "", errors.New("evidence id is required")
	}
	provenance, err := evidence.NormalizeSourceRevisions(item.SourceProvenance)
	if err != nil {
		return "", "", fmt.Errorf("normalize source provenance: %w", err)
	}
	stored := *item
	stored.SourceProvenance = provenance
	payload, err := marshalWorkflow(&stored)
	if err != nil {
		return "", "", err
	}
	encoded, err := json.Marshal(provenance)
	if err != nil {
		return "", "", fmt.Errorf("marshal source provenance: %w", err)
	}
	return payload, string(encoded), nil
}

// CreateEvidence inserts evidence through the repository boundary.
func (c *Corpus) CreateEvidence(ctx context.Context, item *evidence.Evidence) error {
	return c.SaveEvidence(ctx, item)
}

// ListEvidence returns evidence matching the supplied local filter.
func (c *Corpus) ListEvidence(ctx context.Context, filter evidence.EvidenceFilter) (out []*evidence.Evidence, err error) {
	query := `SELECT e.payload, e.source_provenance FROM evidence e WHERE 1=1`
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
	defer func() {
		if closeErr := rows.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	for rows.Next() {
		var payload, provenance string
		if err := rows.Scan(&payload, &provenance); err != nil {
			return nil, err
		}
		var item evidence.Evidence
		if err := unmarshalWorkflow(payload, &item); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(provenance), &item.SourceProvenance); err != nil {
			return nil, fmt.Errorf("unmarshal evidence %q source provenance: %w", item.ID, err)
		}
		item.SourceProvenance, err = evidence.NormalizeSourceRevisions(item.SourceProvenance)
		if err != nil {
			return nil, fmt.Errorf("normalize evidence %q source provenance: %w", item.ID, err)
		}
		out = append(out, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
