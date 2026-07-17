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
	"github.com/morluto/gitcontribute/internal/investigation"
	"github.com/morluto/gitcontribute/internal/tracking"
)

var _ tracking.Repository = (*Corpus)(nil)

// RecordTriageEvent stores a triage event with optional foreign-key-safe links.
func (c *Corpus) RecordTriageEvent(ctx context.Context, e *tracking.TriageEvent) error {
	if err := resolveTriageLinks(ctx, c, e); err != nil {
		return err
	}
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
	_, err := c.db.ExecContext(ctx, `
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
	defer rows.Close()

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
	_, err = c.db.ExecContext(ctx, `
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
	defer rows.Close()

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
	item.Metadata, _ = unmarshalContributionPayload(payload)
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
	createdAt := encodeTime(o.CreatedAt)
	if createdAt == 0 {
		createdAt = encodeTime(time.Now())
	}
	sourceEventAt := encodeTime(o.SourceEventAt)
	if sourceEventAt == 0 {
		sourceEventAt = createdAt
	}
	_, err = c.db.ExecContext(ctx, `
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
	defer rows.Close()

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

	bundle := &tracking.Bundle{}

	triageEvents, err := c.ListTriageEvents(ctx, tracking.TriageEventFilter{Limit: limit})
	if err != nil {
		return nil, err
	}
	bundle.TriageEvents = triageEvents

	contributions, err := c.ListContributions(ctx, tracking.ContributionFilter{Limit: limit})
	if err != nil {
		return nil, err
	}
	bundle.Contributions = contributions

	if err := c.exportContributionOutcomes(ctx, bundle, limit); err != nil {
		return nil, err
	}

	tracking.OrderBundle(bundle)
	return tracking.SanitizeBundle(bundle), nil
}

func (c *Corpus) exportContributionOutcomes(ctx context.Context, bundle *tracking.Bundle, limit int) error {
	rows, err := c.db.QueryContext(ctx, `
		SELECT o.id, o.contribution_id, o.outcome, o.reason, o.source_event_at, o.created_at
		FROM contribution_outcomes AS o
		JOIN (
			SELECT id FROM contributions ORDER BY prepared_at, id LIMIT ?
		) AS selected ON selected.id = o.contribution_id
		ORDER BY o.source_event_at, o.id
		LIMIT ?
	`, limit, limit)
	if err != nil {
		return fmt.Errorf("export contribution outcomes: %w", err)
	}
	defer rows.Close()

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

// ImportLocalMetadata imports a bounded bundle idempotently.
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
	for _, e := range bundle.TriageEvents {
		if err := c.RecordTriageEvent(ctx, e); err != nil {
			return fmt.Errorf("import triage event %q: %w", e.ID, err)
		}
	}
	for _, item := range bundle.Contributions {
		if err := c.SaveContribution(ctx, item); err != nil {
			return fmt.Errorf("import contribution %q: %w", item.ID, err)
		}
	}
	for _, o := range bundle.ContributionOutcomes {
		if err := c.RecordContributionOutcome(ctx, o); err != nil {
			return fmt.Errorf("import contribution outcome %q: %w", o.ID, err)
		}
	}
	return nil
}
