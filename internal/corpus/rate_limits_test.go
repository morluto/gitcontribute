package corpus

import (
	"context"
	"testing"
	"time"
)

func TestRecordRateLimitObservation(t *testing.T) {
	c, _ := openTestCorpus(t)
	ctx := context.Background()
	now := time.Unix(100, 0).UTC()
	err := c.RecordRateLimitObservation(ctx, RateLimitObservation{
		Attempt: 2, StatusCode: 429, Resource: "search", Limit: 30, Remaining: 0, Used: 30,
		ResetAt: now.Add(time.Minute), Delay: time.Second, APIVersion: "2022-11-28",
		SourceURL: "https://api.github.com/search/repositories?q=%5BREDACTED%5D", ObservedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	var attempt, status, remaining, delay int
	var source string
	if err := c.db.QueryRowContext(ctx, `SELECT attempt, status_code, remaining, delay_ms, source_url FROM rate_limit_observations`).Scan(&attempt, &status, &remaining, &delay, &source); err != nil {
		t.Fatal(err)
	}
	if attempt != 2 || status != 429 || remaining != 0 || delay != 1000 || source == "" {
		t.Fatalf("stored observation = attempt:%d status:%d remaining:%d delay:%d source:%q", attempt, status, remaining, delay, source)
	}
}

func TestRecordRateLimitObservationStoresMissingResetAsZero(t *testing.T) {
	c, _ := openTestCorpus(t)
	ctx := context.Background()
	if err := c.RecordRateLimitObservation(ctx, RateLimitObservation{
		Attempt: 1, StatusCode: 200, ObservedAt: time.Unix(100, 0).UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	var resetAt int64
	if err := c.db.QueryRowContext(ctx, `SELECT reset_at FROM rate_limit_observations`).Scan(&resetAt); err != nil {
		t.Fatal(err)
	}
	if resetAt != 0 {
		t.Fatalf("reset_at = %d, want 0", resetAt)
	}
}
