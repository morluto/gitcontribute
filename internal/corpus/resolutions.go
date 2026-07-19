package corpus

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ResolutionRecord is a deterministic local derivation over immutable source
// observations. It is not a root-cause claim and must identify its rule set.
type ResolutionRecord struct {
	ID                    int64            `json:"id"`
	ThreadID              int64            `json:"thread_id"`
	Kind                  string           `json:"kind"`
	Summary               string           `json:"summary"`
	RuleVersion           string           `json:"rule_version"`
	SourceUpdatedAt       time.Time        `json:"source_updated_at"`
	ObservationSequence   int64            `json:"observation_sequence"`
	SourceObservationRefs []ObservationRef `json:"source_observation_refs"`
	DerivedAt             time.Time        `json:"derived_at"`
}

// SaveResolutionRecord appends a derivation and advances the current
// projection only when its source clock is newer.
func (c *Corpus) SaveResolutionRecord(ctx context.Context, record ResolutionRecord) (*ResolutionRecord, error) {
	if record.ThreadID <= 0 || strings.TrimSpace(record.Kind) == "" || strings.TrimSpace(record.RuleVersion) == "" || record.SourceUpdatedAt.IsZero() || len(record.SourceObservationRefs) == 0 {
		return nil, errors.New("resolution thread, kind, rule version, source time, and observation refs are required")
	}
	for _, ref := range record.SourceObservationRefs {
		if strings.TrimSpace(ref.Kind) == "" || ref.ID <= 0 {
			return nil, errors.New("invalid resolution source observation reference")
		}
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin resolution record: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if record.ObservationSequence == 0 {
		record.ObservationSequence, err = c.nextSequence(ctx, tx)
		if err != nil {
			return nil, err
		}
	}
	if record.DerivedAt.IsZero() {
		record.DerivedAt = time.Now().UTC()
	}
	refs, err := json.Marshal(record.SourceObservationRefs)
	if err != nil {
		return nil, fmt.Errorf("encode resolution observation refs: %w", err)
	}
	if err := validateObservationRefsTx(ctx, tx, record.SourceObservationRefs); err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO resolution_records
			(thread_id, kind, summary, rule_version, source_updated_at, observation_sequence, source_observation_refs, derived_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, record.ThreadID, record.Kind, record.Summary, record.RuleVersion, encodeTime(record.SourceUpdatedAt), record.ObservationSequence, string(refs), encodeTime(record.DerivedAt))
	if err != nil {
		return nil, fmt.Errorf("insert resolution record: %w", err)
	}
	record.ID, err = result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("read resolution record id: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO resolution_projections (thread_id, resolution_record_id, source_updated_at, observation_sequence)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (thread_id) DO UPDATE SET
			resolution_record_id=excluded.resolution_record_id,
			source_updated_at=excluded.source_updated_at,
			observation_sequence=excluded.observation_sequence
		WHERE resolution_projections.source_updated_at < excluded.source_updated_at
		   OR (resolution_projections.source_updated_at = excluded.source_updated_at
		       AND resolution_projections.observation_sequence < excluded.observation_sequence)
	`, record.ThreadID, record.ID, encodeTime(record.SourceUpdatedAt), record.ObservationSequence); err != nil {
		return nil, fmt.Errorf("advance resolution projection: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit resolution record: %w", err)
	}
	return &record, nil
}

// GetResolutionRecord returns the current stale-safe resolution projection.
func (c *Corpus) GetResolutionRecord(ctx context.Context, threadID int64) (*ResolutionRecord, error) {
	var record ResolutionRecord
	var sourceUpdated, derived int64
	var refs string
	err := c.db.QueryRowContext(ctx, `
		SELECT r.id, r.thread_id, r.kind, r.summary, r.rule_version, r.source_updated_at,
		       r.observation_sequence, r.source_observation_refs, r.derived_at
		FROM resolution_projections p JOIN resolution_records r ON r.id=p.resolution_record_id
		WHERE p.thread_id=?
	`, threadID).Scan(&record.ID, &record.ThreadID, &record.Kind, &record.Summary, &record.RuleVersion, &sourceUpdated, &record.ObservationSequence, &refs, &derived)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get resolution record: %w", err)
	}
	if err := json.Unmarshal([]byte(refs), &record.SourceObservationRefs); err != nil {
		return nil, fmt.Errorf("decode resolution refs: %w", err)
	}
	record.SourceUpdatedAt, record.DerivedAt = scanTime(sourceUpdated), scanTime(derived)
	return &record, nil
}
