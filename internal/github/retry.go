package github

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultRetryAttempts  = 5
	defaultRetryBaseDelay = 100 * time.Millisecond
	defaultRetryMaxDelay  = 30 * time.Second
)

// RetryConfig controls how the retry transport paces and observes retries.
type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Clock       func() time.Time
	Sleeper     func(context.Context, time.Duration) error
	OnAttempt   func(RetryObservation)
}

// RetryObservation reports the outcome and pacing of a single retry attempt.
type RetryObservation struct {
	Attempt    int
	StatusCode int
	RateLimit  RateInfo
	Delay      time.Duration
	APIVersion string
	SourceURL  string
}

// retryTransport wraps an http.RoundTripper with bounded retries for safe,
// idempotent reads. It honors Retry-After and primary rate-limit reset headers,
// supports injected clocks/sleepers for deterministic tests, and exposes an
// observation callback with redacted source URLs.
type retryTransport struct {
	Base   http.RoundTripper
	Config *RetryConfig
}

// DefaultRetryConfig returns a production retry policy.
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts: defaultRetryAttempts,
		BaseDelay:   defaultRetryBaseDelay,
		MaxDelay:    defaultRetryMaxDelay,
	}
}

func (r *RetryConfig) withDefaults() *RetryConfig {
	if r == nil {
		return DefaultRetryConfig()
	}
	c := *r
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = defaultRetryAttempts
	}
	if c.BaseDelay <= 0 {
		c.BaseDelay = defaultRetryBaseDelay
	}
	if c.MaxDelay <= 0 {
		c.MaxDelay = defaultRetryMaxDelay
	}
	return &c
}

func (r *RetryConfig) now() time.Time {
	if r != nil && r.Clock != nil {
		return r.Clock()
	}
	return time.Now()
}

func (r *RetryConfig) sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d <= 0 {
		return nil
	}
	if r != nil && r.Sleeper != nil {
		return r.Sleeper(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (t *retryTransport) base() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return http.DefaultTransport
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !isReplayable(req) {
		return t.base().RoundTrip(req)
	}

	cfg := t.Config
	ctx := req.Context()

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		clone, err := cloneRequest(req)
		if err != nil {
			return nil, fmt.Errorf("clone request for retry: %w", err)
		}

		resp, err := t.base().RoundTrip(clone)
		if err == nil && !shouldRetry(resp) {
			if cfg.OnAttempt != nil {
				cfg.OnAttempt(observation(req, resp, attempt, 0, cfg.now()))
			}
			return resp, nil
		}

		if err != nil && isContextError(err) {
			drainAndClose(resp)
			return nil, err
		}

		if attempt == cfg.MaxAttempts {
			if cfg.OnAttempt != nil {
				cfg.OnAttempt(observation(req, resp, attempt, 0, cfg.now()))
			}
			if err != nil {
				drainAndClose(resp)
				return nil, &TransientError{Cause: err}
			}
			return resp, nil
		}

		delay := cfg.delay(attempt, resp)
		if cfg.OnAttempt != nil {
			cfg.OnAttempt(observation(req, resp, attempt, delay, cfg.now()))
		}
		drainAndClose(resp)
		if err := cfg.sleep(ctx, delay); err != nil {
			return nil, err
		}
	}

	// Unreachable because MaxAttempts is always >= 1.
	return nil, errors.New("retry loop exhausted without result")
}

func isReplayable(req *http.Request) bool {
	if req == nil {
		return false
	}
	switch req.Method {
	case http.MethodGet, http.MethodHead:
	default:
		return false
	}
	if req.Body != nil && req.Body != http.NoBody {
		return false
	}
	return true
}

func cloneRequest(req *http.Request) (*http.Request, error) {
	if req == nil {
		return nil, errors.New("nil request")
	}
	clone := req.Clone(req.Context())
	if clone == nil {
		return nil, errors.New("request clone returned nil")
	}
	// Safe idempotent reads have no body; clear any body references to avoid
	// sharing or accidental replay of non-replayable payloads.
	clone.Body = nil
	clone.GetBody = nil
	return clone, nil
}

func shouldRetry(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		return true
	case http.StatusForbidden:
		// GitHub uses 403 plus Retry-After for secondary rate limits. Do not
		// retry ordinary authorization failures or long primary-limit resets.
		return strings.TrimSpace(resp.Header.Get("Retry-After")) != ""
	}
	return resp.StatusCode >= 500
}

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

func (r *RetryConfig) delay(attempt int, resp *http.Response) time.Duration {
	base := r.BaseDelay
	if base <= 0 {
		base = defaultRetryBaseDelay
	}
	max := r.MaxDelay
	if max <= 0 {
		max = defaultRetryMaxDelay
	}

	backoff := base
	for i := 1; i < attempt; i++ {
		if backoff > max/2 {
			backoff = max
			break
		}
		backoff *= 2
	}
	if backoff > max {
		backoff = max
	}

	if resp == nil {
		return backoff
	}

	now := r.now()
	serverDelay := time.Duration(0)
	if ra := parseRetryAfter(resp.Header.Get("Retry-After"), now); ra > 0 {
		serverDelay = ra
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		rem, _ := strconv.Atoi(resp.Header.Get("X-Ratelimit-Remaining"))
		if rem == 0 {
			if rd := parseResetDelay(resp.Header.Get("X-Ratelimit-Reset"), now); rd > 0 {
				if rd > serverDelay {
					serverDelay = rd
				}
			}
		}
	}

	delay := backoff
	if serverDelay > delay {
		delay = serverDelay
	}
	if delay > max {
		delay = max
	}
	return delay
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if sec, err := strconv.Atoi(value); err == nil && sec >= 0 {
		return time.Duration(sec) * time.Second
	}
	if t, err := http.ParseTime(value); err == nil {
		d := t.Sub(now)
		if d > 0 {
			return d
		}
	}
	return 0
}

func parseResetDelay(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	sec, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	reset := time.Unix(sec, 0)
	d := reset.Sub(now)
	if d > 0 {
		return d
	}
	return 0
}

func observation(req *http.Request, resp *http.Response, attempt int, delay time.Duration, now time.Time) RetryObservation {
	obs := RetryObservation{
		Attempt:   attempt,
		Delay:     delay,
		SourceURL: redactSourceURL(req),
	}
	if resp != nil {
		obs.StatusCode = resp.StatusCode
		obs.RateLimit = rateInfoFromHeaders(resp.Header, now)
		obs.APIVersion = resp.Header.Get("X-GitHub-Api-Version")
	}
	return obs
}

func rateInfoFromHeaders(h http.Header, now time.Time) RateInfo {
	parseInt := func(key string) int {
		v, _ := strconv.Atoi(h.Get(key))
		return v
	}
	ri := RateInfo{
		Limit:     parseInt("X-Ratelimit-Limit"),
		Remaining: parseInt("X-Ratelimit-Remaining"),
		Used:      parseInt("X-Ratelimit-Used"),
		Resource:  h.Get("X-Ratelimit-Resource"),
	}
	if v := h.Get("X-Ratelimit-Reset"); v != "" {
		if sec, err := strconv.ParseInt(v, 10, 64); err == nil {
			ri.Reset = time.Unix(sec, 0)
		}
	}
	return ri
}

func redactSourceURL(req *http.Request) string {
	if req == nil || req.URL == nil {
		return ""
	}
	u := *req.URL
	if u.User != nil {
		if _, hasPass := u.User.Password(); hasPass {
			u.User = url.UserPassword(u.User.Username(), "[REDACTED]")
		}
	}
	q := u.Query()
	for k := range q {
		lk := strings.ToLower(k)
		if isSensitiveQueryKey(lk) {
			q.Set(k, "[REDACTED]")
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func isSensitiveQueryKey(k string) bool {
	switch k {
	case "token", "access_token", "client_secret", "client_id", "code",
		"signature", "password", "secret":
		return true
	}
	if strings.Contains(k, "token") || strings.Contains(k, "secret") ||
		strings.Contains(k, "signature") || strings.Contains(k, "password") {
		return true
	}
	return false
}
