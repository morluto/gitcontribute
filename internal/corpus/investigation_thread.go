package corpus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/morluto/gitcontribute/internal/investigation"
)

// StartThreadInvestigation atomically inserts an investigation and its seed
// hypothesis. If the same thread already has an open investigation, the stored
// pair is returned without changing its original baseline.
func (c *Corpus) StartThreadInvestigation(ctx context.Context, item *investigation.Investigation, hypothesis *investigation.Hypothesis) (_ *investigation.Investigation, _ *investigation.Hypothesis, _ bool, returnErr error) {
	if err := validateThreadInvestigationPair(item, hypothesis); err != nil {
		return nil, nil, false, err
	}
	investigationPayload, err := marshalWorkflow(item)
	if err != nil {
		return nil, nil, false, err
	}
	hypothesisPayload, err := marshalWorkflow(hypothesis)
	if err != nil {
		return nil, nil, false, err
	}
	originKey := item.ThreadBaseline.OriginKey()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, false, fmt.Errorf("begin thread investigation: %w", err)
	}
	defer func() {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			returnErr = errors.Join(returnErr, fmt.Errorf("rollback thread investigation: %w", rollbackErr))
		}
	}()

	result, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO investigations
			(id, repo_owner, repo_name, status, origin_key, payload, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, item.ID, item.Repo.Owner, item.Repo.Repo, item.Status, originKey,
		investigationPayload, encodeTime(item.CreatedAt), encodeTime(item.UpdatedAt))
	if err != nil {
		return nil, nil, false, fmt.Errorf("insert thread investigation: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return nil, nil, false, fmt.Errorf("read thread investigation insert result: %w", err)
	}
	if inserted == 0 {
		existingInvestigation, existingHypothesis, loadErr := loadOpenThreadInvestigation(ctx, tx, originKey)
		if loadErr != nil {
			return nil, nil, false, fmt.Errorf("resolve existing thread investigation: %w", loadErr)
		}
		return existingInvestigation, existingHypothesis, false, nil
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO hypotheses (id, investigation_id, category, status, payload, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, hypothesis.ID, hypothesis.InvestigationID, hypothesis.Category, hypothesis.Status,
		hypothesisPayload, encodeTime(hypothesis.CreatedAt), encodeTime(hypothesis.UpdatedAt)); err != nil {
		return nil, nil, false, fmt.Errorf("insert thread seed hypothesis: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, false, fmt.Errorf("commit thread investigation: %w", err)
	}
	return item, hypothesis, true, nil
}

func validateThreadInvestigationPair(item *investigation.Investigation, hypothesis *investigation.Hypothesis) error {
	if item == nil || item.ID == "" || item.ThreadBaseline == nil {
		return fmt.Errorf("%w: investigation identity and baseline are required", investigation.ErrInvalidThreadBaseline)
	}
	if err := item.ThreadBaseline.Validate(); err != nil {
		return err
	}
	if !strings.EqualFold(item.Repo.Owner, item.ThreadBaseline.Repo.Owner) || !strings.EqualFold(item.Repo.Repo, item.ThreadBaseline.Repo.Repo) {
		return fmt.Errorf("%w: investigation repository does not match its thread", investigation.ErrInvalidThreadBaseline)
	}
	if item.Status != investigation.InvestigationOpen {
		return fmt.Errorf("%w: thread investigation must start open", investigation.ErrInvalidThreadBaseline)
	}
	if hypothesis == nil || hypothesis.ID == "" || hypothesis.InvestigationID != item.ID || item.SeedHypothesisID != hypothesis.ID {
		return fmt.Errorf("%w: seed hypothesis does not match investigation", investigation.ErrInvalidThreadBaseline)
	}
	return nil
}

func loadOpenThreadInvestigation(ctx context.Context, tx *sql.Tx, originKey string) (*investigation.Investigation, *investigation.Hypothesis, error) {
	var investigationPayload string
	err := tx.QueryRowContext(ctx, `
		SELECT payload FROM investigations
		WHERE origin_key=? AND status=?
		ORDER BY created_at, id
		LIMIT 1
	`, originKey, investigation.InvestigationOpen).Scan(&investigationPayload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, errors.New("origin conflict did not identify an open investigation")
	}
	if err != nil {
		return nil, nil, err
	}
	var item investigation.Investigation
	if err := unmarshalWorkflow(investigationPayload, &item); err != nil {
		return nil, nil, err
	}
	if item.SeedHypothesisID == "" {
		return nil, nil, errors.New("stored thread investigation has no seed hypothesis")
	}
	var hypothesisPayload string
	if err := tx.QueryRowContext(ctx, `SELECT payload FROM hypotheses WHERE id=?`, item.SeedHypothesisID).Scan(&hypothesisPayload); err != nil {
		return nil, nil, fmt.Errorf("read stored seed hypothesis: %w", err)
	}
	var hypothesis investigation.Hypothesis
	if err := unmarshalWorkflow(hypothesisPayload, &hypothesis); err != nil {
		return nil, nil, err
	}
	return &item, &hypothesis, nil
}

func investigationOriginKey(item *investigation.Investigation) any {
	if item == nil || item.ThreadBaseline == nil {
		return nil
	}
	return item.ThreadBaseline.OriginKey()
}
