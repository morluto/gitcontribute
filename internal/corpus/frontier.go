package corpus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// FrontierState describes the durable lifecycle of queued crawl work.
const (
	FrontierQueued    = "queued"
	FrontierLeased    = "leased"
	FrontierCompleted = "completed"
	FrontierFailed    = "failed"
)

// Frontier failure classifications keep terminal source outcomes distinct.
const (
	FrontierFailureTransientExhausted = "transient_exhausted"
	FrontierFailureAbsent             = "absent"
	FrontierFailureUnauthorized       = "unauthorized"
	FrontierFailureDeleted            = "deleted"
	FrontierFailureArchived           = "archived"
	FrontierFailurePermanent          = "permanent"
)

// FrontierItem is a deduplicated unit of repository, thread, or facet work.
// WorkKey is a stable product-owned identity chosen by the caller.
type FrontierItem struct {
	ID             int64
	WorkKey        string
	SubjectKind    string
	Owner          string
	Repo           string
	ThreadKind     string
	ThreadNumber   int
	Facet          string
	Priority       int
	Reason         string
	Source         string
	Attempts       int
	MaxAttempts    int
	EarliestRunAt  time.Time
	BudgetEstimate int
	State          string
	LeaseOwner     string
	LeaseExpiresAt *time.Time
	FailureKind    string
	LastError      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// EnqueueFrontierItem inserts work once. Replaying the same WorkKey returns
// the existing item without resetting attempts or terminal state.
func (c *Corpus) EnqueueFrontierItem(ctx context.Context, item FrontierItem) (*FrontierItem, bool, error) {
	if strings.TrimSpace(item.WorkKey) == "" {
		return nil, false, errors.New("frontier work key is required")
	}
	if strings.TrimSpace(item.SubjectKind) == "" {
		return nil, false, errors.New("frontier subject kind is required")
	}
	if item.MaxAttempts <= 0 {
		item.MaxAttempts = 3
	}
	if item.BudgetEstimate <= 0 {
		item.BudgetEstimate = 1
	}
	now := encodeTime(time.Now())
	res, err := c.db.ExecContext(ctx, `
		INSERT INTO frontier_items (
			work_key, subject_kind, owner, repo, thread_kind, thread_number, facet,
			priority, reason, source, max_attempts, earliest_run_at, budget_estimate,
			state, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (work_key) DO NOTHING
	`, item.WorkKey, item.SubjectKind, item.Owner, item.Repo, item.ThreadKind,
		item.ThreadNumber, item.Facet, item.Priority, item.Reason, item.Source,
		item.MaxAttempts, encodeTime(item.EarliestRunAt), item.BudgetEstimate,
		FrontierQueued, now, now)
	if err != nil {
		return nil, false, fmt.Errorf("enqueue frontier item: %w", err)
	}
	inserted, err := res.RowsAffected()
	if err != nil {
		return nil, false, fmt.Errorf("read enqueue result: %w", err)
	}
	stored, err := c.GetFrontierItem(ctx, item.WorkKey)
	return stored, inserted == 1, err
}

// GetFrontierItem returns work by its stable key, or nil when absent.
func (c *Corpus) GetFrontierItem(ctx context.Context, workKey string) (*FrontierItem, error) {
	row := c.db.QueryRowContext(ctx, frontierSelect+` WHERE work_key = ?`, workKey)
	item, err := scanFrontierItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get frontier item: %w", err)
	}
	return item, nil
}

// LeaseFrontierItems atomically claims ready work for a bounded interval.
// Expired leases are eligible for another worker. Higher priority wins, then
// earlier eligibility and insertion order.
func (c *Corpus) LeaseFrontierItems(ctx context.Context, worker string, now time.Time, leaseDuration time.Duration, limit, budget int) ([]FrontierItem, error) {
	if strings.TrimSpace(worker) == "" {
		return nil, errors.New("frontier worker is required")
	}
	if leaseDuration <= 0 {
		return nil, errors.New("frontier lease duration must be positive")
	}
	if limit <= 0 {
		return []FrontierItem{}, nil
	}
	if budget <= 0 {
		budget = int(^uint(0) >> 1)
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin frontier lease: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	nowEncoded := encodeTime(now)
	if _, err := tx.ExecContext(ctx, `
		UPDATE frontier_items
		SET state = ?, failure_kind = ?, lease_owner = NULL,
		    lease_expires_at = NULL, updated_at = ?
		WHERE state = ? AND lease_expires_at <= ? AND attempts >= max_attempts
	`, FrontierFailed, FrontierFailureTransientExhausted, nowEncoded,
		FrontierLeased, nowEncoded); err != nil {
		return nil, fmt.Errorf("expire exhausted frontier leases: %w", err)
	}
	rows, err := tx.QueryContext(ctx, frontierSelect+`
		WHERE attempts < max_attempts
		  AND earliest_run_at <= ?
		  AND (state = ? OR (state = ? AND lease_expires_at <= ?))
		ORDER BY priority DESC, earliest_run_at, id
		LIMIT ?
	`, nowEncoded, FrontierQueued, FrontierLeased, nowEncoded, limit)
	if err != nil {
		return nil, fmt.Errorf("select frontier lease candidates: %w", err)
	}
	candidates, err := scanFrontierItems(rows)
	_ = rows.Close()
	if err != nil {
		return nil, fmt.Errorf("scan frontier lease candidates: %w", err)
	}

	remaining := budget
	leased := make([]FrontierItem, 0, len(candidates))
	for _, item := range candidates {
		if item.BudgetEstimate > remaining {
			continue
		}
		expires := now.Add(leaseDuration)
		res, err := tx.ExecContext(ctx, `
			UPDATE frontier_items
			SET state = ?, lease_owner = ?, lease_expires_at = ?, attempts = attempts + 1, updated_at = ?
			WHERE id = ? AND attempts < max_attempts AND earliest_run_at <= ?
			  AND (state = ? OR (state = ? AND lease_expires_at <= ?))
		`, FrontierLeased, worker, encodeTime(expires), nowEncoded, item.ID,
			nowEncoded, FrontierQueued, FrontierLeased, nowEncoded)
		if err != nil {
			return nil, fmt.Errorf("lease frontier item %d: %w", item.ID, err)
		}
		changed, err := res.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("read frontier lease result: %w", err)
		}
		if changed == 0 {
			continue
		}
		item.State = FrontierLeased
		item.LeaseOwner = worker
		item.LeaseExpiresAt = &expires
		item.Attempts++
		item.UpdatedAt = now
		leased = append(leased, item)
		remaining -= item.BudgetEstimate
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit frontier lease: %w", err)
	}
	return leased, nil
}

// CompleteFrontierItem marks leased work complete. Only the lease owner can
// complete it, preventing a stale worker from overwriting a newer attempt.
func (c *Corpus) CompleteFrontierItem(ctx context.Context, id int64, worker string, now time.Time) error {
	return c.finishFrontierLease(ctx, id, worker, FrontierCompleted, "", "", time.Time{}, now)
}

// RetryFrontierItem releases leased work after a transient failure. Once the
// attempt limit is reached, the item becomes terminally failed.
func (c *Corpus) RetryFrontierItem(ctx context.Context, id int64, worker, message string, earliestRunAt, now time.Time) error {
	return c.finishFrontierLease(ctx, id, worker, FrontierQueued, "", message, earliestRunAt, now)
}

// FailFrontierItem marks a leased item terminally failed.
func (c *Corpus) FailFrontierItem(ctx context.Context, id int64, worker, failureKind, message string, now time.Time) error {
	if strings.TrimSpace(failureKind) == "" {
		return errors.New("frontier failure kind is required")
	}
	return c.finishFrontierLease(ctx, id, worker, FrontierFailed, failureKind, message, time.Time{}, now)
}

func (c *Corpus) finishFrontierLease(ctx context.Context, id int64, worker, requestedState, failureKind, message string, earliestRunAt, now time.Time) error {
	if requestedState == FrontierQueued {
		failureKind = FrontierFailureTransientExhausted
	}
	res, err := c.db.ExecContext(ctx, `
		UPDATE frontier_items
		SET state = CASE
		        WHEN ? = ? AND attempts >= max_attempts THEN ?
		        ELSE ?
		    END,
		    earliest_run_at = ?, lease_owner = NULL, lease_expires_at = NULL,
		    failure_kind = CASE
		        WHEN ? = ? AND attempts < max_attempts THEN NULL
		        ELSE NULLIF(?, '')
		    END,
		    last_error = NULLIF(?, ''), updated_at = ?
		WHERE id = ? AND state = ? AND lease_owner = ?
	`, requestedState, FrontierQueued, FrontierFailed, requestedState,
		encodeTime(earliestRunAt), requestedState, FrontierQueued, failureKind,
		message, encodeTime(now), id, FrontierLeased, worker)
	if err != nil {
		return fmt.Errorf("finish frontier item %d: %w", id, err)
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("read frontier finish result: %w", err)
	}
	if changed == 0 {
		return fmt.Errorf("frontier item %d is not leased by %q", id, worker)
	}
	return nil
}

const frontierSelect = `
	SELECT id, work_key, subject_kind, owner, repo, thread_kind, thread_number,
	       facet, priority, reason, source, attempts, max_attempts,
	       earliest_run_at, budget_estimate, state, lease_owner,
	       lease_expires_at, failure_kind, last_error, created_at, updated_at
	FROM frontier_items`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanFrontierItem(row rowScanner) (*FrontierItem, error) {
	var item FrontierItem
	var owner, repo, threadKind, facet, reason, source sql.NullString
	var leaseOwner, failureKind, lastError sql.NullString
	var threadNumber sql.NullInt64
	var earliest, created, updated int64
	var leaseExpires sql.NullInt64
	err := row.Scan(&item.ID, &item.WorkKey, &item.SubjectKind, &owner, &repo,
		&threadKind, &threadNumber, &facet, &item.Priority, &reason, &source,
		&item.Attempts, &item.MaxAttempts, &earliest, &item.BudgetEstimate,
		&item.State, &leaseOwner, &leaseExpires, &failureKind, &lastError, &created, &updated)
	if err != nil {
		return nil, err
	}
	item.Owner = owner.String
	item.Repo = repo.String
	item.ThreadKind = threadKind.String
	item.ThreadNumber = int(threadNumber.Int64)
	item.Facet = facet.String
	item.Reason = reason.String
	item.Source = source.String
	item.EarliestRunAt = scanTime(earliest)
	item.LeaseOwner = leaseOwner.String
	if leaseExpires.Valid {
		t := scanTime(leaseExpires.Int64)
		item.LeaseExpiresAt = &t
	}
	item.FailureKind = failureKind.String
	item.LastError = lastError.String
	item.CreatedAt = scanTime(created)
	item.UpdatedAt = scanTime(updated)
	return &item, nil
}

func scanFrontierItems(rows *sql.Rows) ([]FrontierItem, error) {
	var items []FrontierItem
	for rows.Next() {
		item, err := scanFrontierItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}
