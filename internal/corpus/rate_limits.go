package corpus

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const maxRateLimitObservations = 10_000

// RateLimitObservation is a redacted GitHub request/rate-limit measurement.
type RateLimitObservation struct {
	Attempt    int
	StatusCode int
	Resource   string
	Limit      int
	Remaining  int
	Used       int
	ResetAt    time.Time
	Delay      time.Duration
	APIVersion string
	SourceURL  string
	ObservedAt time.Time
}

// LatestRateLimitObservations returns the newest observation for each reported
// GitHub resource, ordered newest first.
func (c *Corpus) LatestRateLimitObservations(ctx context.Context, limit int) ([]RateLimitObservation, error) {
	if limit <= 0 || limit > 20 {
		return nil, errors.New("rate-limit observation limit must be between 1 and 20")
	}
	rows, err := c.db.QueryContext(ctx, `
		SELECT attempt, status_code, resource, limit_value, remaining, used,
		       reset_at, delay_ms, api_version, source_url, observed_at
		FROM (
			SELECT *, ROW_NUMBER() OVER (
				PARTITION BY COALESCE(NULLIF(resource, ''), 'unknown')
				ORDER BY observed_at DESC, id DESC
			) AS resource_rank
			FROM rate_limit_observations
		)
		WHERE resource_rank = 1
		ORDER BY observed_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query latest rate-limit observations: %w", err)
	}
	defer rows.Close()

	var observations []RateLimitObservation
	for rows.Next() {
		var observation RateLimitObservation
		var resetAt, delayMS, observedAt int64
		if err := rows.Scan(
			&observation.Attempt, &observation.StatusCode, &observation.Resource,
			&observation.Limit, &observation.Remaining, &observation.Used,
			&resetAt, &delayMS, &observation.APIVersion, &observation.SourceURL, &observedAt,
		); err != nil {
			return nil, fmt.Errorf("scan latest rate-limit observation: %w", err)
		}
		if resetAt != 0 {
			observation.ResetAt = scanTime(resetAt)
		}
		observation.Delay = time.Duration(delayMS) * time.Millisecond
		observation.ObservedAt = scanTime(observedAt)
		observations = append(observations, observation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate latest rate-limit observations: %w", err)
	}
	return observations, nil
}

// RecordRateLimitObservation stores one bounded, redacted request observation.
func (c *Corpus) RecordRateLimitObservation(ctx context.Context, observation RateLimitObservation) error {
	if observation.Attempt < 1 {
		return errors.New("rate-limit observation attempt must be positive")
	}
	if observation.ObservedAt.IsZero() {
		observation.ObservedAt = time.Now().UTC()
	}
	var resetAt int64
	if !observation.ResetAt.IsZero() {
		resetAt = encodeTime(observation.ResetAt)
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin rate-limit observation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO rate_limit_observations
			(attempt, status_code, resource, limit_value, remaining, used, reset_at, delay_ms, api_version, source_url, observed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, observation.Attempt, observation.StatusCode, observation.Resource, observation.Limit,
		observation.Remaining, observation.Used, resetAt, observation.Delay.Milliseconds(),
		observation.APIVersion, observation.SourceURL, encodeTime(observation.ObservedAt)); err != nil {
		return fmt.Errorf("insert rate-limit observation: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM rate_limit_observations
		WHERE id NOT IN (SELECT id FROM rate_limit_observations ORDER BY observed_at DESC, id DESC LIMIT ?)
	`, maxRateLimitObservations); err != nil {
		return fmt.Errorf("bound rate-limit observations: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit rate-limit observation: %w", err)
	}
	return nil
}
