// Package crawl executes bounded batches from the durable crawl frontier.
package crawl

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/github"
)

const (
	defaultBatchSize      = 20
	maxBatchSize          = 100
	defaultLeaseDuration  = 2 * time.Minute
	maxLeaseDuration      = 30 * time.Minute
	defaultHandlerTimeout = 90 * time.Second
	defaultBackoff        = 30 * time.Second
	maxBackoff            = time.Hour
	maxErrorLength        = 4096
)

// Frontier is the durable lease boundary required by a Worker.
type Frontier interface {
	LeaseFrontierItems(ctx context.Context, worker string, now time.Time, leaseDuration time.Duration, limit, budget int) ([]corpus.FrontierItem, error)
	ReleaseFrontierItem(ctx context.Context, id int64, worker string, now time.Time) error
	CompleteFrontierItem(ctx context.Context, id int64, worker string, now time.Time) error
	RetryFrontierItem(ctx context.Context, id int64, worker, message string, earliestRunAt, now time.Time) error
	FailFrontierItem(ctx context.Context, id int64, worker, failureKind, message string, now time.Time) error
}

// Handler hydrates one leased frontier item. It must respect ctx and return
// only after all writes for the item are durable.
type Handler interface {
	Handle(context.Context, corpus.FrontierItem) error
}

// HandlerFunc adapts a function to Handler.
type HandlerFunc func(context.Context, corpus.FrontierItem) error

func (f HandlerFunc) Handle(ctx context.Context, item corpus.FrontierItem) error {
	return f(ctx, item)
}

// TerminalError tells the worker that retrying an item cannot succeed without
// a source-state change.
type TerminalError struct {
	Kind string
	Err  error
}

func (e *TerminalError) Error() string {
	if e.Err == nil {
		return e.Kind
	}
	return e.Err.Error()
}

func (e *TerminalError) Unwrap() error { return e.Err }

// Decision describes how a failed item returns to the frontier.
type Decision struct {
	Terminal    bool
	FailureKind string
	RetryAt     time.Time
}

// Stats reports actual work performed by one bounded batch.
type Stats struct {
	Leased    int
	Completed int
	Retried   int
	Failed    int
	Budget    int
}

// Worker leases and executes one bounded batch at a time.
type Worker struct {
	Frontier       Frontier
	Handler        Handler
	ID             string
	BatchSize      int
	Budget         int
	LeaseDuration  time.Duration
	HandlerTimeout time.Duration
	Backoff        time.Duration
	Now            func() time.Time
	Classify       func(error, corpus.FrontierItem, time.Time) Decision
}

// RunOnce executes at most one configured batch. It is intentionally
// synchronous: callers control scheduling and can run independent workers when
// they need concurrency.
func (w *Worker) RunOnce(ctx context.Context) (Stats, error) {
	cfg, err := w.config()
	if err != nil {
		return Stats{}, err
	}
	now := cfg.now()
	items, err := cfg.Frontier.LeaseFrontierItems(
		ctx, cfg.ID, now, cfg.LeaseDuration, cfg.BatchSize, cfg.Budget,
	)
	if err != nil {
		return Stats{}, err
	}
	stats := Stats{Leased: len(items)}
	for i, item := range items {
		if err := ctx.Err(); err != nil {
			if releaseErr := cfg.release(ctx, items[i:]); releaseErr != nil {
				return stats, errors.Join(err, releaseErr)
			}
			return stats, err
		}
		stats.Budget += item.BudgetEstimate
		itemCtx, cancel := context.WithTimeout(ctx, cfg.HandlerTimeout)
		handleErr := cfg.Handler.Handle(itemCtx, item)
		cancel()
		finishedAt := cfg.now()
		cleanup, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		if handleErr == nil {
			err = cfg.Frontier.CompleteFrontierItem(cleanup, item.ID, cfg.ID, finishedAt)
			cleanupCancel()
			if err != nil {
				return stats, err
			}
			stats.Completed++
			continue
		}

		decision := cfg.Classify(handleErr, item, finishedAt)
		message := boundedError(handleErr)
		if decision.Terminal {
			if strings.TrimSpace(decision.FailureKind) == "" {
				cleanupCancel()
				return stats, errors.New("terminal crawl decision requires a failure kind")
			}
			err = cfg.Frontier.FailFrontierItem(cleanup, item.ID, cfg.ID, decision.FailureKind, message, finishedAt)
			cleanupCancel()
			if err != nil {
				return stats, err
			}
			stats.Failed++
			continue
		}
		if decision.RetryAt.Before(finishedAt) {
			decision.RetryAt = finishedAt
		}
		err = cfg.Frontier.RetryFrontierItem(cleanup, item.ID, cfg.ID, message, decision.RetryAt, finishedAt)
		cleanupCancel()
		if err != nil {
			return stats, err
		}
		if item.Attempts >= item.MaxAttempts {
			stats.Failed++
		} else {
			stats.Retried++
		}
	}
	return stats, nil
}

func (w *Worker) config() (*Worker, error) {
	if w == nil || w.Frontier == nil || w.Handler == nil {
		return nil, errors.New("crawl frontier and handler are required")
	}
	cfg := *w
	cfg.ID = strings.TrimSpace(cfg.ID)
	if cfg.ID == "" {
		return nil, errors.New("crawl worker id is required")
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = defaultBatchSize
	}
	if cfg.BatchSize < 1 || cfg.BatchSize > maxBatchSize {
		return nil, fmt.Errorf("crawl batch size must be between 1 and %d", maxBatchSize)
	}
	if cfg.Budget <= 0 {
		return nil, errors.New("crawl budget must be positive")
	}
	if cfg.LeaseDuration == 0 {
		cfg.LeaseDuration = defaultLeaseDuration
	}
	if cfg.LeaseDuration <= 0 || cfg.LeaseDuration > maxLeaseDuration {
		return nil, fmt.Errorf("crawl lease duration must be positive and at most %s", maxLeaseDuration)
	}
	if cfg.HandlerTimeout == 0 {
		cfg.HandlerTimeout = defaultHandlerTimeout
	}
	if cfg.HandlerTimeout <= 0 || cfg.HandlerTimeout >= cfg.LeaseDuration {
		return nil, errors.New("crawl handler timeout must be positive and shorter than the lease")
	}
	if cfg.Backoff == 0 {
		cfg.Backoff = defaultBackoff
	}
	if cfg.Backoff < 0 || cfg.Backoff > maxBackoff {
		return nil, fmt.Errorf("crawl backoff must be between zero and %s", maxBackoff)
	}
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	if cfg.Classify == nil {
		cfg.Classify = func(err error, item corpus.FrontierItem, now time.Time) Decision {
			return defaultDecision(err, item, now, cfg.Backoff)
		}
	}
	return &cfg, nil
}

func (w *Worker) now() time.Time { return w.Now().UTC() }

func (w *Worker) release(parent context.Context, items []corpus.FrontierItem) error {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	defer cancel()
	now := w.now()
	var errs []error
	for _, item := range items {
		if err := w.Frontier.ReleaseFrontierItem(cleanup, item.ID, w.ID, now); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func defaultDecision(err error, item corpus.FrontierItem, now time.Time, baseBackoff time.Duration) Decision {
	var terminal *TerminalError
	if errors.As(err, &terminal) {
		return Decision{Terminal: true, FailureKind: terminal.Kind}
	}
	var notFound *github.NotFoundError
	if errors.As(err, &notFound) {
		return Decision{Terminal: true, FailureKind: corpus.FrontierFailureAbsent}
	}
	var denied *github.AccessDeniedError
	if errors.As(err, &denied) {
		return Decision{Terminal: true, FailureKind: corpus.FrontierFailureUnauthorized}
	}
	var gone *github.GoneError
	if errors.As(err, &gone) {
		return Decision{Terminal: true, FailureKind: corpus.FrontierFailureDeleted}
	}
	delay := retryDelay(err)
	if delay <= 0 {
		delay = exponentialBackoff(baseBackoff, item.Attempts)
	}
	return Decision{RetryAt: now.Add(delay)}
}

func retryDelay(err error) time.Duration {
	var primary *github.PrimaryRateLimitError
	if errors.As(err, &primary) {
		return primary.RetryAfter
	}
	var secondary *github.SecondaryRateLimitError
	if errors.As(err, &secondary) {
		return secondary.RetryAfter
	}
	return 0
}

func exponentialBackoff(base time.Duration, attempt int) time.Duration {
	if base <= 0 {
		return 0
	}
	delay := base
	for i := 1; i < attempt && delay < maxBackoff; i++ {
		if delay > maxBackoff/2 {
			return maxBackoff
		}
		delay *= 2
	}
	if delay > maxBackoff {
		return maxBackoff
	}
	return delay
}

func boundedError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.ToValidUTF8(err.Error(), "�")
	if len(message) <= maxErrorLength {
		return message
	}
	message = message[:maxErrorLength]
	for !utf8.ValidString(message) {
		message = message[:len(message)-1]
	}
	return message
}
