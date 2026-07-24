package corpus

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/investigation"
	"github.com/morluto/gitcontribute/internal/tracking"
)

// dbExecer matches the ExecContext methods used by *sql.DB and *sql.Tx so
// upsert helpers can run inside or outside a transaction.
type dbExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type dbQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

var _ tracking.Repository = (*Corpus)(nil)

// RecordTriageEvent stores a triage event with optional foreign-key-safe links.
func (c *Corpus) RecordTriageEvent(ctx context.Context, e *tracking.TriageEvent) error {
	if err := resolveTriageLinks(ctx, c, e); err != nil {
		return err
	}
	return c.recordTriageEventTx(ctx, c.db, e)
}

func (c *Corpus) recordTriageEventTx(ctx context.Context, db dbExecer, e *tracking.TriageEvent) error {
	now := encodeTime(time.Now())
	createdAt := encodeTime(e.CreatedAt)
	updatedAt := encodeTime(e.UpdatedAt)
	if createdAt == 0 {
		createdAt = now
	}
	if updatedAt == 0 {
		updatedAt = now
	}
	sourceEventAt := encodeTime(e.SourceEventAt)
	if sourceEventAt == 0 {
		sourceEventAt = now
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO triage_events (id, target_kind, target_ref, outcome, reason, lens, source_event_at, created_at, updated_at, repository_id, thread_id, investigation_id, opportunity_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			target_kind=excluded.target_kind,
			target_ref=excluded.target_ref,
			outcome=excluded.outcome,
			reason=excluded.reason,
			lens=excluded.lens,
			source_event_at=excluded.source_event_at,
			created_at=excluded.created_at,
			updated_at=excluded.updated_at,
			repository_id=excluded.repository_id,
			thread_id=excluded.thread_id,
			investigation_id=excluded.investigation_id,
			opportunity_id=excluded.opportunity_id
	`, e.ID, string(e.TargetKind), e.TargetRef, string(e.Outcome), e.Reason, e.Lens, sourceEventAt, createdAt, updatedAt,
		nullInt64(e.RepositoryID), nullInt64(e.ThreadID), nullString(e.InvestigationID), nullString(e.OpportunityID))
	if err != nil {
		return fmt.Errorf("record triage event: %w", err)
	}
	return nil
}

func (c *Corpus) importTriageEventTx(ctx context.Context, tx *sql.Tx, e *tracking.TriageEvent) error {
	createdAt := encodeTime(e.CreatedAt)
	updatedAt := encodeTime(e.UpdatedAt)
	sourceEventAt := encodeTime(e.SourceEventAt)
	result, err := tx.ExecContext(ctx, `
		INSERT INTO triage_events (id, target_kind, target_ref, outcome, reason, lens, source_event_at, created_at, updated_at, repository_id, thread_id, investigation_id, opportunity_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			target_kind=excluded.target_kind, target_ref=excluded.target_ref,
			outcome=excluded.outcome, reason=excluded.reason, lens=excluded.lens,
			source_event_at=excluded.source_event_at, created_at=excluded.created_at,
			updated_at=excluded.updated_at, repository_id=excluded.repository_id,
			thread_id=excluded.thread_id, investigation_id=excluded.investigation_id,
			opportunity_id=excluded.opportunity_id
		WHERE excluded.updated_at > triage_events.updated_at
	`, e.ID, string(e.TargetKind), e.TargetRef, string(e.Outcome), e.Reason, e.Lens,
		sourceEventAt, createdAt, updatedAt, nullInt64(e.RepositoryID), nullInt64(e.ThreadID),
		nullString(e.InvestigationID), nullString(e.OpportunityID))
	if err != nil {
		return fmt.Errorf("import triage event: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed == 1 {
		return err
	}
	var storedUpdatedAt int64
	var same int
	err = tx.QueryRowContext(ctx, `
		SELECT updated_at,
		       target_kind=? AND target_ref=? AND outcome=? AND reason=? AND lens=?
		       AND source_event_at=? AND created_at=?
		       AND repository_id IS ? AND thread_id IS ?
		       AND investigation_id IS ? AND opportunity_id IS ?
		FROM triage_events WHERE id=?
	`, string(e.TargetKind), e.TargetRef, string(e.Outcome), e.Reason, e.Lens,
		sourceEventAt, createdAt, nullInt64(e.RepositoryID), nullInt64(e.ThreadID),
		nullString(e.InvestigationID), nullString(e.OpportunityID), e.ID).Scan(&storedUpdatedAt, &same)
	if err != nil {
		return err
	}
	if updatedAt < storedUpdatedAt || same != 0 {
		return nil
	}
	return fmt.Errorf("%w: triage event %q has different equal-timestamp content", tracking.ErrImportConflict, e.ID)
}

func resolveTriageLinks(ctx context.Context, c *Corpus, e *tracking.TriageEvent) error {
	// Verify any carried foreign keys still exist in this corpus and clear stale
	// ones so imports remain safe across corpora.
	if e.RepositoryID != nil && !c.repoExists(ctx, *e.RepositoryID) {
		e.RepositoryID = nil
	}
	if e.ThreadID != nil && !c.threadExists(ctx, *e.ThreadID) {
		e.ThreadID = nil
	}
	if e.InvestigationID != "" && !c.investigationExists(ctx, e.InvestigationID) {
		e.InvestigationID = ""
	}
	if e.OpportunityID != "" && !c.opportunityExists(ctx, e.OpportunityID) {
		e.OpportunityID = ""
	}

	if e.RepositoryID == nil && e.TargetKind == tracking.TargetRepository {
		ref, err := parseRepoRef(e.TargetRef)
		if err == nil {
			if repo, err := c.GetRepository(ctx, ref.Owner, ref.Repo); err == nil && repo != nil {
				e.RepositoryID = &repo.ID
			}
		}
	}
	if e.OpportunityID == "" && e.TargetKind == tracking.TargetOpportunity {
		if _, err := c.GetOpportunity(ctx, e.TargetRef); err == nil {
			e.OpportunityID = e.TargetRef
		} else if !errors.Is(err, investigation.ErrNotFound) {
			return fmt.Errorf("resolve opportunity link: %w", err)
		}
	}
	if e.InvestigationID == "" && e.TargetKind == tracking.TargetInvestigation {
		if _, err := c.GetInvestigation(ctx, e.TargetRef); err == nil {
			e.InvestigationID = e.TargetRef
		} else if !errors.Is(err, investigation.ErrNotFound) {
			return fmt.Errorf("resolve investigation link: %w", err)
		}
	}
	if e.ThreadID == nil && (e.TargetKind == tracking.TargetIssue || e.TargetKind == tracking.TargetPullRequest || e.TargetKind == tracking.TargetThread) {
		repoRef, number, ok := parseThreadRef(e.TargetRef)
		if ok {
			if repo, err := c.GetRepository(ctx, repoRef.Owner, repoRef.Repo); err == nil && repo != nil {
				e.RepositoryID = &repo.ID
				kind := ""
				switch e.TargetKind {
				case tracking.TargetIssue:
					kind = ThreadKindIssue
				case tracking.TargetPullRequest:
					kind = ThreadKindPullRequest
				}
				if kind != "" {
					if thread, err := c.GetThread(ctx, repo.ID, kind, number); err == nil && thread != nil {
						e.ThreadID = &thread.ID
					}
				} else {
					if thread, err := c.GetThreadByNumber(ctx, repo.ID, number); err == nil && thread != nil {
						e.ThreadID = &thread.ID
					}
				}
			}
		}
	}
	return nil
}

func parseRepoRef(ref string) (domain.RepoRef, error) {
	parts := strings.Split(ref, "/")
	if len(parts) != 2 {
		return domain.RepoRef{}, fmt.Errorf("invalid repository reference")
	}
	r := domain.RepoRef{Owner: strings.TrimSpace(parts[0]), Repo: strings.TrimSpace(parts[1])}
	if err := r.Validate(); err != nil {
		return domain.RepoRef{}, err
	}
	return r, nil
}

func parseThreadRef(ref string) (domain.RepoRef, int, bool) {
	repoRef, numText, ok := strings.Cut(ref, "#")
	if !ok {
		return domain.RepoRef{}, 0, false
	}
	repo, err := parseRepoRef(repoRef)
	if err != nil {
		return domain.RepoRef{}, 0, false
	}
	number, err := strconv.Atoi(strings.TrimSpace(numText))
	if err != nil || number <= 0 {
		return domain.RepoRef{}, 0, false
	}
	return repo, number, true
}

func (c *Corpus) repoExists(ctx context.Context, id int64) bool {
	var one int
	err := c.db.QueryRowContext(ctx, `SELECT 1 FROM repositories WHERE id=?`, id).Scan(&one)
	return err == nil
}

func (c *Corpus) threadExists(ctx context.Context, id int64) bool {
	var one int
	err := c.db.QueryRowContext(ctx, `SELECT 1 FROM threads WHERE id=?`, id).Scan(&one)
	return err == nil
}

func (c *Corpus) investigationExists(ctx context.Context, id string) bool {
	var one int
	err := c.db.QueryRowContext(ctx, `SELECT 1 FROM investigations WHERE id=?`, id).Scan(&one)
	return err == nil
}

func (c *Corpus) opportunityExists(ctx context.Context, id string) bool {
	var one int
	err := c.db.QueryRowContext(ctx, `SELECT 1 FROM opportunities WHERE id=?`, id).Scan(&one)
	return err == nil
}

func nullInt64(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}

func nullString(v string) sql.NullString {
	if v == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: v, Valid: true}
}

// ListTriageEvents returns triage events in source-event order.
func (c *Corpus) ListTriageEvents(ctx context.Context, filter tracking.TriageEventFilter) ([]*tracking.TriageEvent, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 1000
	}
	if limit > 10000 {
		return nil, fmt.Errorf("triage event limit cannot exceed 10000")
	}
	query := `SELECT id, target_kind, target_ref, outcome, reason, lens, source_event_at, created_at, updated_at, repository_id, thread_id, investigation_id, opportunity_id FROM triage_events WHERE 1=1`
	var args []any
	if filter.TargetKind != "" {
		query += ` AND target_kind=?`
		args = append(args, string(filter.TargetKind))
	}
	if filter.TargetRef != "" {
		query += ` AND target_ref=?`
		args = append(args, filter.TargetRef)
	}
	if filter.Outcome != "" {
		query += ` AND outcome=?`
		args = append(args, string(filter.Outcome))
	}
	if filter.Lens != "" {
		query += ` AND lens=?`
		args = append(args, filter.Lens)
	}
	query += ` ORDER BY source_event_at, id LIMIT ?`
	args = append(args, limit)

	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list triage events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*tracking.TriageEvent
	for rows.Next() {
		e, err := scanTriageEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func scanTriageEvent(rows interface {
	Scan(dest ...any) error
}) (*tracking.TriageEvent, error) {
	var e tracking.TriageEvent
	var sourceEventAt, createdAt, updatedAt int64
	var repositoryID, threadID sql.NullInt64
	var investigationID, opportunityID sql.NullString
	err := rows.Scan(&e.ID, &e.TargetKind, &e.TargetRef, &e.Outcome, &e.Reason, &e.Lens, &sourceEventAt, &createdAt, &updatedAt, &repositoryID, &threadID, &investigationID, &opportunityID)
	if err != nil {
		return nil, err
	}
	e.SourceEventAt = scanTime(sourceEventAt)
	e.CreatedAt = scanTime(createdAt)
	e.UpdatedAt = scanTime(updatedAt)
	if repositoryID.Valid {
		e.RepositoryID = &repositoryID.Int64
	}
	if threadID.Valid {
		e.ThreadID = &threadID.Int64
	}
	e.InvestigationID = investigationID.String
	e.OpportunityID = opportunityID.String
	return &e, nil
}

// SaveContribution stores contribution metadata separate from GitHub state.
func (c *Corpus) SaveContribution(ctx context.Context, item *tracking.Contribution) error {
	if _, err := c.GetOpportunity(ctx, item.OpportunityID); err != nil {
		if errors.Is(err, investigation.ErrNotFound) {
			return fmt.Errorf("opportunity %q not found", item.OpportunityID)
		}
		return fmt.Errorf("resolve contribution opportunity: %w", err)
	}
	return c.saveContributionTx(ctx, c.db, item)
}

func (c *Corpus) saveContributionTx(ctx context.Context, db dbExecer, item *tracking.Contribution) error {
	payload, err := marshalContributionPayload(item.Metadata)
	if err != nil {
		return err
	}
	now := encodeTime(time.Now())
	createdAt := encodeTime(item.CreatedAt)
	updatedAt := encodeTime(item.UpdatedAt)
	preparedAt := encodeTime(item.PreparedAt)
	submittedAt := encodeOptionalTime(item.SubmittedAt)
	if createdAt == 0 {
		createdAt = now
	}
	if updatedAt == 0 {
		updatedAt = now
	}
	if preparedAt == 0 {
		preparedAt = now
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO contributions (id, opportunity_id, kind, title, body, reference, reference_url, prepared_at, submitted_at, created_at, updated_at, payload)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			opportunity_id=excluded.opportunity_id,
			kind=excluded.kind,
			title=excluded.title,
			body=excluded.body,
			reference=excluded.reference,
			reference_url=excluded.reference_url,
			prepared_at=excluded.prepared_at,
			submitted_at=excluded.submitted_at,
			updated_at=excluded.updated_at,
			payload=excluded.payload
	`, item.ID, item.OpportunityID, item.Kind, item.Title, item.Body, item.Reference, item.ReferenceURL, preparedAt, submittedAt, createdAt, updatedAt, payload)
	if err != nil {
		return fmt.Errorf("save contribution: %w", err)
	}
	return nil
}

func (c *Corpus) importContributionTx(ctx context.Context, tx *sql.Tx, item *tracking.Contribution) error {
	payload, err := marshalContributionPayload(item.Metadata)
	if err != nil {
		return err
	}
	createdAt, updatedAt := encodeTime(item.CreatedAt), encodeTime(item.UpdatedAt)
	preparedAt, submittedAt := encodeTime(item.PreparedAt), encodeOptionalTime(item.SubmittedAt)
	result, err := tx.ExecContext(ctx, `
		INSERT INTO contributions (id, opportunity_id, kind, title, body, reference, reference_url, prepared_at, submitted_at, created_at, updated_at, payload)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			opportunity_id=excluded.opportunity_id, kind=excluded.kind,
			title=excluded.title, body=excluded.body, reference=excluded.reference,
			reference_url=excluded.reference_url, prepared_at=excluded.prepared_at,
			submitted_at=excluded.submitted_at, updated_at=excluded.updated_at,
			payload=excluded.payload
		WHERE excluded.updated_at > contributions.updated_at
	`, item.ID, item.OpportunityID, item.Kind, item.Title, item.Body, item.Reference,
		item.ReferenceURL, preparedAt, submittedAt, createdAt, updatedAt, payload)
	if err != nil {
		return fmt.Errorf("import contribution: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed == 1 {
		return err
	}
	var storedUpdatedAt int64
	var same int
	err = tx.QueryRowContext(ctx, `
		SELECT updated_at,
		       opportunity_id=? AND kind=? AND title=? AND body=?
		       AND reference=? AND reference_url=? AND prepared_at=?
		       AND submitted_at IS ? AND created_at=? AND payload=?
		FROM contributions WHERE id=?
	`, item.OpportunityID, item.Kind, item.Title, item.Body, item.Reference,
		item.ReferenceURL, preparedAt, submittedAt, createdAt, payload, item.ID).Scan(&storedUpdatedAt, &same)
	if err != nil {
		return err
	}
	if updatedAt < storedUpdatedAt || same != 0 {
		return nil
	}
	return fmt.Errorf("%w: contribution %q has different equal-timestamp content", tracking.ErrImportConflict, item.ID)
}

func marshalContributionPayload(metadata map[string]any) (string, error) {
	if metadata == nil {
		return "{}", nil
	}
	b, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("marshal contribution metadata: %w", err)
	}
	return string(b), nil
}

func encodeOptionalTime(t *time.Time) sql.NullInt64 {
	if t == nil || t.IsZero() {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: encodeTime(*t), Valid: true}
}

// GetContribution returns a contribution by durable id.
func (c *Corpus) GetContribution(ctx context.Context, id string) (*tracking.Contribution, error) {
	row := c.db.QueryRowContext(ctx, `
		SELECT id, opportunity_id, kind, title, body, reference, reference_url, prepared_at, submitted_at, created_at, updated_at, payload
		FROM contributions WHERE id=?`, id)
	item, err := scanContribution(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get contribution: %w", err)
	}
	return item, nil
}

// ListContributions returns contributions in prepared-at order.
func (c *Corpus) ListContributions(ctx context.Context, filter tracking.ContributionFilter) ([]*tracking.Contribution, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 1000
	}
	if limit > 10000 {
		return nil, fmt.Errorf("contribution limit cannot exceed 10000")
	}
	query := `SELECT id, opportunity_id, kind, title, body, reference, reference_url, prepared_at, submitted_at, created_at, updated_at, payload FROM contributions WHERE 1=1`
	var args []any
	if filter.OpportunityID != "" {
		query += ` AND opportunity_id=?`
		args = append(args, filter.OpportunityID)
	}
	if filter.Kind != "" {
		query += ` AND kind=?`
		args = append(args, filter.Kind)
	}
	query += ` ORDER BY prepared_at, id LIMIT ?`
	args = append(args, limit)

	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list contributions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*tracking.Contribution
	for rows.Next() {
		item, err := scanContribution(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func scanContribution(scanner interface {
	Scan(dest ...any) error
}) (*tracking.Contribution, error) {
	var item tracking.Contribution
	var preparedAt, createdAt, updatedAt int64
	var submittedAt sql.NullInt64
	var payload string
	err := scanner.Scan(&item.ID, &item.OpportunityID, &item.Kind, &item.Title, &item.Body, &item.Reference, &item.ReferenceURL, &preparedAt, &submittedAt, &createdAt, &updatedAt, &payload)
	if err != nil {
		return nil, err
	}
	item.PreparedAt = scanTime(preparedAt)
	item.CreatedAt = scanTime(createdAt)
	item.UpdatedAt = scanTime(updatedAt)
	if submittedAt.Valid {
		t := scanTime(submittedAt.Int64)
		item.SubmittedAt = &t
	}
	metadata, err := unmarshalContributionPayload(payload)
	if err != nil {
		return nil, fmt.Errorf("decode contribution metadata: %w", err)
	}
	item.Metadata = metadata
	return &item, nil
}

func unmarshalContributionPayload(payload string) (map[string]any, error) {
	if payload == "" || payload == "{}" {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(payload), &m); err != nil {
		return nil, err
	}
	return m, nil
}

// RecordContributionOutcome stores a lifecycle event for a contribution.
func (c *Corpus) RecordContributionOutcome(ctx context.Context, o *tracking.ContributionOutcome) error {
	var exists int
	err := c.db.QueryRowContext(ctx, `SELECT 1 FROM contributions WHERE id=?`, o.ContributionID).Scan(&exists)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("contribution %q not found", o.ContributionID)
		}
		return fmt.Errorf("resolve contribution outcome: %w", err)
	}
	return c.recordContributionOutcomeTx(ctx, c.db, o)
}

func (c *Corpus) recordContributionOutcomeTx(ctx context.Context, db dbExecer, o *tracking.ContributionOutcome) error {
	createdAt := encodeTime(o.CreatedAt)
	if createdAt == 0 {
		createdAt = encodeTime(time.Now())
	}
	sourceEventAt := encodeTime(o.SourceEventAt)
	if sourceEventAt == 0 {
		sourceEventAt = createdAt
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO contribution_outcomes (id, contribution_id, outcome, reason, source_event_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			contribution_id=excluded.contribution_id,
			outcome=excluded.outcome,
			reason=excluded.reason,
			source_event_at=excluded.source_event_at,
			created_at=excluded.created_at
	`, o.ID, o.ContributionID, string(o.Outcome), o.Reason, sourceEventAt, createdAt)
	if err != nil {
		return fmt.Errorf("record contribution outcome: %w", err)
	}
	return nil
}

func (c *Corpus) importContributionOutcomeTx(ctx context.Context, tx *sql.Tx, o *tracking.ContributionOutcome) error {
	createdAt, sourceEventAt := encodeTime(o.CreatedAt), encodeTime(o.SourceEventAt)
	result, err := tx.ExecContext(ctx, `
		INSERT INTO contribution_outcomes (id, contribution_id, outcome, reason, source_event_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO NOTHING
	`, o.ID, o.ContributionID, string(o.Outcome), o.Reason, sourceEventAt, createdAt)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed == 1 {
		return err
	}
	var same int
	if err := tx.QueryRowContext(ctx, `
		SELECT contribution_id=? AND outcome=? AND reason=?
		       AND source_event_at=? AND created_at=?
		FROM contribution_outcomes WHERE id=?
	`, o.ContributionID, string(o.Outcome), o.Reason, sourceEventAt, createdAt, o.ID).Scan(&same); err != nil {
		return err
	}
	if same == 0 {
		return fmt.Errorf("%w: contribution outcome %q has different content", tracking.ErrImportConflict, o.ID)
	}
	return nil
}

// ListContributionOutcomes returns outcomes for a contribution.
func (c *Corpus) ListContributionOutcomes(ctx context.Context, contributionID string) ([]*tracking.ContributionOutcome, error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT id, contribution_id, outcome, reason, source_event_at, created_at
		FROM contribution_outcomes
		WHERE contribution_id=?
		ORDER BY source_event_at, id
	`, contributionID)
	if err != nil {
		return nil, fmt.Errorf("list contribution outcomes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*tracking.ContributionOutcome
	for rows.Next() {
		var o tracking.ContributionOutcome
		var sourceEventAt, createdAt int64
		if err := rows.Scan(&o.ID, &o.ContributionID, &o.Outcome, &o.Reason, &sourceEventAt, &createdAt); err != nil {
			return nil, err
		}
		o.SourceEventAt = scanTime(sourceEventAt)
		o.CreatedAt = scanTime(createdAt)
		out = append(out, &o)
	}
	return out, rows.Err()
}

// ExportLocalMetadata returns a redacted, deterministic snapshot of tracking
// metadata bounded by opts.Limit.
func (c *Corpus) ExportLocalMetadata(ctx context.Context, opts tracking.ExportOptions) (*tracking.Bundle, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 10000
	}
	if limit > 100000 {
		return nil, fmt.Errorf("export limit cannot exceed 100000")
	}

	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin tracking export snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, recordClass := range []struct {
		name  string
		table string
	}{
		{"triage events", "triage_events"},
		{"contributions", "contributions"},
		{"contribution outcomes", "contribution_outcomes"},
		{"evidence", "evidence"},
	} {
		var total int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+recordClass.table).Scan(&total); err != nil {
			return nil, fmt.Errorf("count %s: %w", recordClass.name, err)
		}
		if total > limit {
			return nil, fmt.Errorf("%w: %s total %d exceeds limit %d", tracking.ErrExportTruncated, recordClass.name, total, limit)
		}
	}

	bundle := &tracking.Bundle{SchemaVersion: tracking.CurrentBundleSchemaVersion}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, target_kind, target_ref, outcome, reason, lens, source_event_at,
		       created_at, updated_at, repository_id, thread_id, investigation_id, opportunity_id
		FROM triage_events ORDER BY source_event_at, id
	`)
	if err != nil {
		return nil, fmt.Errorf("export triage events: %w", err)
	}
	for rows.Next() {
		item, scanErr := scanTriageEvent(rows)
		if scanErr != nil {
			_ = rows.Close()
			return nil, scanErr
		}
		bundle.TriageEvents = append(bundle.TriageEvents, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	rows, err = tx.QueryContext(ctx, `
		SELECT id, opportunity_id, kind, title, body, reference, reference_url,
		       prepared_at, submitted_at, created_at, updated_at, payload
		FROM contributions ORDER BY prepared_at, id
	`)
	if err != nil {
		return nil, fmt.Errorf("export contributions: %w", err)
	}
	for rows.Next() {
		item, scanErr := scanContribution(rows)
		if scanErr != nil {
			_ = rows.Close()
			return nil, scanErr
		}
		bundle.Contributions = append(bundle.Contributions, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	if err := c.exportContributionOutcomes(ctx, tx, bundle); err != nil {
		return nil, err
	}
	bundle.Evidence, err = listEvidenceRows(ctx, tx, evidence.EvidenceFilter{}, 0)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tracking export snapshot: %w", err)
	}

	tracking.OrderBundle(bundle)
	return tracking.SanitizeBundle(bundle), nil
}
