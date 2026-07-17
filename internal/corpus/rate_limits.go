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
