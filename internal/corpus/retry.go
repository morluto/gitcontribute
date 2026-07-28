package corpus

import (
	"context"
	"errors"
	"time"

	"github.com/sethvargo/go-retry"
	sqlite3 "modernc.org/sqlite/lib"
)

const (
	sqliteBusyRetryBaseDelay = 25 * time.Millisecond
	sqliteBusyMaxRetries     = 3
)

type sqliteCodedError interface {
	Code() int
}

// RetryBusy runs one idempotent local database operation and retries only
// SQLite busy results. The callback must not contain network or process work.
func RetryBusy(ctx context.Context, fn func(context.Context) error) error {
	return retry.Do(ctx, sqliteBusyBackoff(), func(ctx context.Context) error {
		err := fn(ctx)
		if isSQLiteBusy(err) {
			return retry.RetryableError(err)
		}
		return err
	})
}

// RetryBusyValue is RetryBusy for an idempotent operation returning a value.
func RetryBusyValue[T any](ctx context.Context, fn func(context.Context) (T, error)) (T, error) {
	return retry.DoValue(ctx, sqliteBusyBackoff(), func(ctx context.Context) (T, error) {
		value, err := fn(ctx)
		if isSQLiteBusy(err) {
			return value, retry.RetryableError(err)
		}
		return value, err
	})
}

func sqliteBusyBackoff() retry.Backoff {
	backoff := retry.NewFibonacci(sqliteBusyRetryBaseDelay)
	backoff = retry.WithJitterPercent(25, backoff)
	backoff = retry.WithMaxRetries(sqliteBusyMaxRetries, backoff)
	return backoff
}

func isSQLiteBusy(err error) bool {
	var coded sqliteCodedError
	return errors.As(err, &coded) && coded.Code()&0xff == sqlite3.SQLITE_BUSY
}
