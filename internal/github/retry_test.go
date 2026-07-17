package github

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type fakeSleeper struct {
	mu    sync.Mutex
	calls []time.Duration
	err   error
}

func (s *fakeSleeper) Sleep(ctx context.Context, d time.Duration) error {
	s.mu.Lock()
	s.calls = append(s.calls, d)
	err := s.err
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return ctx.Err()
}

func (s *fakeSleeper) Calls() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]time.Duration, len(s.calls))
	copy(out, s.calls)
	return out
}

type fakeResult struct {
	status int
	header http.Header
	body   string
	err    error
}

type fakeTransport struct {
	results  []fakeResult
	index    int
	requests []*http.Request
}

func (f *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	f.requests = append(f.requests, req)
	if f.index >= len(f.results) {
		return nil, errors.New("no more results")
	}
	r := f.results[f.index]
	f.index++
	if r.err != nil {
		return nil, r.err
	}
	header := r.header
	if header == nil {
		header = make(http.Header)
	}
	return &http.Response{
		StatusCode: r.status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(r.body)),
		Request:    req,
	}, nil
}

func newGetRequest(t *testing.T, rawurl string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, rawurl, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	return req
}

func TestRetryTransportRetries5xx(t *testing.T) {
	ft := &fakeTransport{
		results: []fakeResult{
			{status: http.StatusInternalServerError, body: "boom"},
			{status: http.StatusInternalServerError, body: "boom"},
			{status: http.StatusOK, body: "ok"},
		},
	}
	sleeper := &fakeSleeper{}
	rt := &retryTransport{
		Base: ft,
		Config: &RetryConfig{
			MaxAttempts: 3,
			BaseDelay:   time.Nanosecond,
			MaxDelay:    time.Nanosecond,
			Sleeper:     sleeper.Sleep,
		},
	}

	req := newGetRequest(t, "http://example.com/repos/o/r")
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ft.index != 3 {
		t.Fatalf("attempts = %d, want 3", ft.index)
	}
	calls := sleeper.Calls()
	if len(calls) != 2 {
		t.Fatalf("sleeps = %d, want 2", len(calls))
	}
	for i, d := range calls {
		if d != time.Nanosecond {
			t.Fatalf("sleep[%d] = %v, want %v", i, d, time.Nanosecond)
		}
	}
}

func TestRetryTransportNoRetryOnTerminal4xx(t *testing.T) {
	ft := &fakeTransport{
		results: []fakeResult{
			{status: http.StatusNotFound, body: `{"message":"not found"}`},
		},
	}
	sleeper := &fakeSleeper{}
	rt := &retryTransport{
		Base: ft,
		Config: &RetryConfig{
			MaxAttempts: 5,
			BaseDelay:   time.Nanosecond,
			MaxDelay:    time.Nanosecond,
			Sleeper:     sleeper.Sleep,
		},
	}

	req := newGetRequest(t, "http://example.com/repos/o/r")
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	if ft.index != 1 {
		t.Fatalf("attempts = %d, want 1", ft.index)
	}
	if len(sleeper.Calls()) != 0 {
		t.Fatalf("sleeps = %d, want 0", len(sleeper.Calls()))
	}
}

func TestRetryTransportNoRetryOnNonIdempotentMethod(t *testing.T) {
	ft := &fakeTransport{
		results: []fakeResult{
			{status: http.StatusInternalServerError, body: "boom"},
		},
	}
	sleeper := &fakeSleeper{}
	rt := &retryTransport{
		Base: ft,
		Config: &RetryConfig{
			MaxAttempts: 5,
			BaseDelay:   time.Nanosecond,
			MaxDelay:    time.Nanosecond,
			Sleeper:     sleeper.Sleep,
		},
	}

	req, err := http.NewRequest(http.MethodPost, "http://example.com/repos/o/r", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
	if ft.index != 1 {
		t.Fatalf("attempts = %d, want 1", ft.index)
	}
	if len(sleeper.Calls()) != 0 {
		t.Fatalf("sleeps = %d, want 0", len(sleeper.Calls()))
	}
}

func TestRetryTransportClonesAndPreservesHeaders(t *testing.T) {
	ft := &fakeTransport{
		results: []fakeResult{
			{status: http.StatusInternalServerError, body: "boom"},
			{status: http.StatusInternalServerError, body: "boom"},
			{status: http.StatusOK, body: "ok"},
		},
	}
	rt := &retryTransport{
		Base: ft,
		Config: &RetryConfig{
			MaxAttempts: 3,
			BaseDelay:   time.Nanosecond,
			MaxDelay:    time.Nanosecond,
			Sleeper: func(ctx context.Context, d time.Duration) error {
				return nil
			},
		},
	}

	req := newGetRequest(t, "http://example.com/repos/o/r")
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if len(ft.requests) != 3 {
		t.Fatalf("attempts = %d, want 3", len(ft.requests))
	}
	for i, r := range ft.requests {
		if r.Method != http.MethodGet {
			t.Fatalf("request[%d] method = %q, want GET", i, r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Fatalf("request[%d] Authorization = %q", i, got)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Fatalf("request[%d] Accept = %q", i, got)
		}
		if r.URL.String() != "http://example.com/repos/o/r" {
			t.Fatalf("request[%d] URL = %q", i, r.URL.String())
		}
	}
	if req.Header.Get("Authorization") != "Bearer secret-token" {
		t.Fatal("original request header mutated")
	}
}

func TestRetryTransportHonorsRetryAfter(t *testing.T) {
	ft := &fakeTransport{
		results: []fakeResult{
			{status: http.StatusTooManyRequests, header: http.Header{"Retry-After": []string{"2"}}},
			{status: http.StatusOK, body: "ok"},
		},
	}
	sleeper := &fakeSleeper{}
	rt := &retryTransport{
		Base: ft,
		Config: &RetryConfig{
			MaxAttempts: 2,
			BaseDelay:   time.Millisecond,
			MaxDelay:    time.Hour,
			Sleeper:     sleeper.Sleep,
		},
	}

	req := newGetRequest(t, "http://example.com/repos/o/r")
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	calls := sleeper.Calls()
	if len(calls) != 1 || calls[0] != 2*time.Second {
		t.Fatalf("sleeps = %v, want [2s]", calls)
	}
}

func TestRetryTransportRetriesSecondaryRateLimit(t *testing.T) {
	ft := &fakeTransport{
		results: []fakeResult{
			{status: http.StatusForbidden, header: http.Header{"Retry-After": []string{"2"}}},
			{status: http.StatusOK, body: "ok"},
		},
	}
	sleeper := &fakeSleeper{}
	rt := &retryTransport{
		Base: ft,
		Config: &RetryConfig{
			MaxAttempts: 2,
			BaseDelay:   time.Millisecond,
			MaxDelay:    time.Hour,
			Sleeper:     sleeper.Sleep,
		},
	}

	resp, err := rt.RoundTrip(newGetRequest(t, "http://example.com/repos/o/r"))
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if calls := sleeper.Calls(); len(calls) != 1 || calls[0] != 2*time.Second {
		t.Fatalf("sleeps = %v, want [2s]", calls)
	}
}

func TestRetryTransportDoesNotRetryOrdinaryForbidden(t *testing.T) {
	ft := &fakeTransport{results: []fakeResult{{status: http.StatusForbidden}}}
	rt := &retryTransport{Base: ft, Config: (&RetryConfig{MaxAttempts: 2}).withDefaults()}

	resp, err := rt.RoundTrip(newGetRequest(t, "http://example.com/repos/o/r"))
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
	if len(ft.requests) != 1 {
		t.Fatalf("attempts = %d, want 1", len(ft.requests))
	}
}

func TestRetryTransportDoesNotRetryForbiddenWithStaleReset(t *testing.T) {
	now := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now}
	reset := now.Add(-5 * time.Second).Unix()

	ft := &fakeTransport{
		results: []fakeResult{
			{
				status: http.StatusForbidden,
				header: http.Header{
					"X-Ratelimit-Remaining": []string{"0"},
					"X-Ratelimit-Reset":     []string{strconv.FormatInt(reset, 10)},
				},
			},
		},
	}
	rt := &retryTransport{
		Base: ft,
		Config: &RetryConfig{
			MaxAttempts: 2,
			BaseDelay:   time.Nanosecond,
			MaxDelay:    time.Nanosecond,
			Clock:       clock.Now,
		},
	}

	resp, err := rt.RoundTrip(newGetRequest(t, "http://example.com/repos/o/r"))
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
	if len(ft.requests) != 1 {
		t.Fatalf("attempts = %d, want 1", len(ft.requests))
	}
}

func TestRetryTransportDoesNotInferPrimaryLimitWithoutRemainingHeader(t *testing.T) {
	now := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	reset := now.Add(5 * time.Second).Unix()
	ft := &fakeTransport{results: []fakeResult{{
		status: http.StatusForbidden,
		header: http.Header{"X-Ratelimit-Reset": []string{strconv.FormatInt(reset, 10)}},
	}}}
	rt := &retryTransport{Base: ft, Config: &RetryConfig{
		MaxAttempts: 2,
		Clock:       func() time.Time { return now },
	}}

	resp, err := rt.RoundTrip(newGetRequest(t, "http://example.com/repos/o/r"))
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
	if len(ft.requests) != 1 {
		t.Fatalf("attempts = %d, want 1", len(ft.requests))
	}
}

func TestRetryTransportHonorsPrimary403Reset(t *testing.T) {
	now := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now}
	reset := now.Add(7 * time.Second).Unix()

	ft := &fakeTransport{
		results: []fakeResult{
			{
				status: http.StatusForbidden,
				header: http.Header{
					"X-Ratelimit-Remaining": []string{"0"},
					"X-Ratelimit-Reset":     []string{strconv.FormatInt(reset, 10)},
				},
			},
			{status: http.StatusOK, body: "ok"},
		},
	}
	sleeper := &fakeSleeper{}
	rt := &retryTransport{
		Base: ft,
		Config: &RetryConfig{
			MaxAttempts: 2,
			BaseDelay:   time.Millisecond,
			MaxDelay:    time.Hour,
			Clock:       clock.Now,
			Sleeper:     sleeper.Sleep,
		},
	}

	req := newGetRequest(t, "http://example.com/repos/o/r")
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	calls := sleeper.Calls()
	if len(calls) != 1 || calls[0] != 7*time.Second {
		t.Fatalf("sleeps = %v, want [7s]", calls)
	}
}

func TestRetryTransportHonorsPrimaryReset(t *testing.T) {
	now := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now}
	reset := now.Add(5 * time.Second).Unix()

	ft := &fakeTransport{
		results: []fakeResult{
			{
				status: http.StatusTooManyRequests,
				header: http.Header{
					"X-Ratelimit-Remaining": []string{"0"},
					"X-Ratelimit-Reset":     []string{strconv.FormatInt(reset, 10)},
				},
			},
			{status: http.StatusOK, body: "ok"},
		},
	}
	sleeper := &fakeSleeper{}
	rt := &retryTransport{
		Base: ft,
		Config: &RetryConfig{
			MaxAttempts: 2,
			BaseDelay:   time.Millisecond,
			MaxDelay:    time.Hour,
			Clock:       clock.Now,
			Sleeper:     sleeper.Sleep,
		},
	}

	req := newGetRequest(t, "http://example.com/repos/o/r")
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	calls := sleeper.Calls()
	if len(calls) != 1 || calls[0] != 5*time.Second {
		t.Fatalf("sleeps = %v, want [5s]", calls)
	}
}

func TestRetryTransportBoundedExponentialBackoff(t *testing.T) {
	ft := &fakeTransport{
		results: []fakeResult{
			{status: http.StatusInternalServerError, body: "boom"},
			{status: http.StatusInternalServerError, body: "boom"},
			{status: http.StatusInternalServerError, body: "boom"},
			{status: http.StatusInternalServerError, body: "boom"},
			{status: http.StatusInternalServerError, body: "boom"},
		},
	}
	sleeper := &fakeSleeper{}
	rt := &retryTransport{
		Base: ft,
		Config: &RetryConfig{
			MaxAttempts: 5,
			BaseDelay:   time.Millisecond,
			MaxDelay:    5 * time.Millisecond,
			Sleeper:     sleeper.Sleep,
		},
	}

	req := newGetRequest(t, "http://example.com/repos/o/r")
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
	want := []time.Duration{
		time.Millisecond,
		2 * time.Millisecond,
		4 * time.Millisecond,
		5 * time.Millisecond,
	}
	if diff := cmp.Diff(want, sleeper.Calls()); diff != "" {
		t.Fatalf("sleeps mismatch (-want +got):\n%s", diff)
	}
}

func TestRetryTransportContextCancelStopsRetries(t *testing.T) {
	ft := &fakeTransport{
		results: []fakeResult{
			{status: http.StatusInternalServerError, body: "boom"},
		},
	}
	sleeper := &fakeSleeper{}
	rt := &retryTransport{
		Base: ft,
		Config: &RetryConfig{
			MaxAttempts: 5,
			BaseDelay:   time.Hour,
			MaxDelay:    time.Hour,
			Sleeper:     sleeper.Sleep,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := newGetRequest(t, "http://example.com/repos/o/r").WithContext(ctx)
	_, err := rt.RoundTrip(req)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if ft.index != 0 {
		t.Fatalf("attempts = %d, want 0", ft.index)
	}
	if len(sleeper.Calls()) != 0 {
		t.Fatalf("sleeps = %d, want 0", len(sleeper.Calls()))
	}
}

func TestRetryTransportSleepErrorPropagated(t *testing.T) {
	ft := &fakeTransport{
		results: []fakeResult{
			{status: http.StatusInternalServerError, body: "boom"},
		},
	}
	wantErr := errors.New("sleeper aborted")
	rt := &retryTransport{
		Base: ft,
		Config: &RetryConfig{
			MaxAttempts: 5,
			BaseDelay:   time.Hour,
			MaxDelay:    time.Hour,
			Sleeper: func(ctx context.Context, d time.Duration) error {
				return wantErr
			},
		},
	}

	req := newGetRequest(t, "http://example.com/repos/o/r")
	_, err := rt.RoundTrip(req)
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if ft.index != 1 {
		t.Fatalf("attempts = %d, want 1", ft.index)
	}
}

func TestRetryTransportObservation(t *testing.T) {
	now := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now}
	reset := now.Add(time.Hour).Unix()

	ft := &fakeTransport{
		results: []fakeResult{
			{
				status: http.StatusTooManyRequests,
				header: http.Header{
					"Retry-After":           []string{"1"},
					"X-Ratelimit-Limit":     []string{"5000"},
					"X-Ratelimit-Remaining": []string{"0"},
					"X-Ratelimit-Reset":     []string{strconv.FormatInt(reset, 10)},
					http.CanonicalHeaderKey("X-GitHub-Api-Version"): []string{"2022-11-28"},
				},
			},
			{status: http.StatusOK, body: "ok"},
		},
	}
	var obs []RetryObservation
	rt := &retryTransport{
		Base: ft,
		Config: &RetryConfig{
			MaxAttempts: 2,
			BaseDelay:   time.Millisecond,
			MaxDelay:    time.Hour,
			Clock:       clock.Now,
			Sleeper:     func(ctx context.Context, d time.Duration) error { return nil },
			OnAttempt:   func(o RetryObservation) { obs = append(obs, o) },
		},
	}

	secretFixture := strings.Join([]string{"fixture", "access", "token"}, "-")
	sourceURL, err := url.Parse("http://example.com/repos/o/r")
	if err != nil {
		t.Fatalf("parse fixture URL: %v", err)
	}
	query := sourceURL.Query()
	query.Set("access_token", secretFixture)
	query.Set("other", "ok")
	sourceURL.RawQuery = query.Encode()
	req := newGetRequest(t, sourceURL.String())
	req.Header.Set("Authorization", "Bearer "+secretFixture)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if len(obs) != 2 {
		t.Fatalf("observations = %d, want 2", len(obs))
	}

	first := obs[0]
	if first.Attempt != 1 {
		t.Fatalf("first attempt = %d, want 1", first.Attempt)
	}
	if first.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("first status = %d, want 429", first.StatusCode)
	}
	if first.RateLimit.Limit != 5000 || first.RateLimit.Remaining != 0 {
		t.Fatalf("first rate limit = %+v", first.RateLimit)
	}
	if first.RateLimit.Reset.Unix() != reset {
		t.Fatalf("first rate limit reset = %v, want %v", first.RateLimit.Reset, time.Unix(reset, 0))
	}
	if first.Delay != time.Hour {
		t.Fatalf("first delay = %v, want 1h", first.Delay)
	}
	if first.APIVersion != "2022-11-28" {
		t.Fatalf("api version = %q, want 2022-11-28", first.APIVersion)
	}

	u, err := url.Parse(first.SourceURL)
	if err != nil {
		t.Fatalf("parse source URL: %v", err)
	}
	if u.Query().Get("access_token") != "[REDACTED]" {
		t.Fatalf("access_token not redacted: %q", u.Query().Get("access_token"))
	}
	if u.Query().Get("other") != "ok" {
		t.Fatalf("other query param altered: %q", u.Query().Get("other"))
	}
	if strings.Contains(first.SourceURL, secretFixture) {
		t.Fatalf("secret leaked in source URL: %s", first.SourceURL)
	}

	second := obs[1]
	if second.Attempt != 2 || second.StatusCode != http.StatusOK || second.Delay != 0 {
		t.Fatalf("second observation = %+v", second)
	}
}

func TestRetryTransportRedactsURLUserinfo(t *testing.T) {
	ft := &fakeTransport{
		results: []fakeResult{
			{status: http.StatusInternalServerError, body: "boom"},
		},
	}
	var got RetryObservation
	rt := &retryTransport{
		Base: ft,
		Config: &RetryConfig{
			MaxAttempts: 1,
			BaseDelay:   0,
			Sleeper:     func(ctx context.Context, d time.Duration) error { return nil },
			OnAttempt:   func(o RetryObservation) { got = o },
		},
	}

	source := &url.URL{
		Scheme: "http", Host: "example.com", Path: "/repos/o/r",
		User: url.UserPassword(
			strings.Join([]string{"fixture", "user"}, "-"),
			strings.Join([]string{"fixture", "password"}, "-"),
		),
	}
	req := newGetRequest(t, source.String())
	_, _ = rt.RoundTrip(req)

	u, err := url.Parse(got.SourceURL)
	if err != nil {
		t.Fatalf("parse source URL: %v", err)
	}
	if u.User != nil {
		t.Fatalf("userinfo not removed from source URL: %v", got.SourceURL)
	}
}

func TestRetryTransportRetriesNetworkErrors(t *testing.T) {
	ft := &fakeTransport{
		results: []fakeResult{
			{err: errors.New("connection refused")},
			{status: http.StatusOK, body: "ok"},
		},
	}
	sleeper := &fakeSleeper{}
	rt := &retryTransport{
		Base: ft,
		Config: &RetryConfig{
			MaxAttempts: 2,
			BaseDelay:   time.Nanosecond,
			MaxDelay:    time.Nanosecond,
			Sleeper:     sleeper.Sleep,
		},
	}

	req := newGetRequest(t, "http://example.com/repos/o/r")
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ft.index != 2 {
		t.Fatalf("attempts = %d, want 2", ft.index)
	}
	if len(sleeper.Calls()) != 1 {
		t.Fatalf("sleeps = %d, want 1", len(sleeper.Calls()))
	}
}

func TestRetryClientRetries5xxThenSucceeds(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]any{"message": "boom"})
			return
		}
		setRateHeaders(w.Header())
		writeJSON(w, repoPayload(123, testRepo, testOwner))
	}))
	defer srv.Close()

	sleeper := &fakeSleeper{}
	client, err := NewClient(Config{
		BaseURL:   srv.URL,
		UploadURL: srv.URL,
		Limiter:   noopLimiter{},
		Retry: &RetryConfig{
			MaxAttempts: 5,
			BaseDelay:   time.Nanosecond,
			MaxDelay:    time.Nanosecond,
			Sleeper:     sleeper.Sleep,
		},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	repo, _, err := client.GetRepository(context.Background(), testOwner, testRepo)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if repo.Name != testRepo {
		t.Fatalf("repo.Name = %q, want %q", repo.Name, testRepo)
	}
	if attempts != 3 {
		t.Fatalf("server attempts = %d, want 3", attempts)
	}
	if len(sleeper.Calls()) != 2 {
		t.Fatalf("sleeps = %d, want 2", len(sleeper.Calls()))
	}
}

func TestRetryClientDoesNotRetryNotFound(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusNotFound)
		writeJSON(w, map[string]any{"message": "Not Found"})
	}))
	defer srv.Close()

	sleeper := &fakeSleeper{}
	client, err := NewClient(Config{
		BaseURL:   srv.URL,
		UploadURL: srv.URL,
		Limiter:   noopLimiter{},
		Retry: &RetryConfig{
			MaxAttempts: 5,
			BaseDelay:   time.Nanosecond,
			MaxDelay:    time.Nanosecond,
			Sleeper:     sleeper.Sleep,
		},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, _, err = client.GetRepository(context.Background(), testOwner, testRepo)
	if err == nil {
		t.Fatal("expected error")
	}
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("err = %T %v, want *NotFoundError", err, err)
	}
	if attempts != 1 {
		t.Fatalf("server attempts = %d, want 1", attempts)
	}
	if len(sleeper.Calls()) != 0 {
		t.Fatalf("sleeps = %d, want 0", len(sleeper.Calls()))
	}
}

func TestRetryClientPreservesRateLimiterBehavior(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]any{"message": "boom"})
			return
		}
		setRateHeaders(w.Header())
		writeJSON(w, repoPayload(1, testRepo, testOwner))
	}))
	defer srv.Close()

	lim := &countingLimiter{}
	client, err := NewClient(Config{
		BaseURL:   srv.URL,
		UploadURL: srv.URL,
		Limiter:   lim,
		Retry: &RetryConfig{
			MaxAttempts: 3,
			BaseDelay:   time.Nanosecond,
			MaxDelay:    time.Nanosecond,
			Sleeper:     func(ctx context.Context, d time.Duration) error { return nil },
		},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, _, err = client.GetRepository(context.Background(), testOwner, testRepo)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("server attempts = %d, want 2", attempts)
	}
	if lim.calls != 1 {
		t.Fatalf("Limiter.WaitN called %d times, want 1", lim.calls)
	}
}

func TestRetryClientContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for cancelled context")
	}))
	defer srv.Close()

	client, err := NewClient(Config{
		BaseURL:   srv.URL,
		UploadURL: srv.URL,
		Limiter:   noopLimiter{},
		Retry: &RetryConfig{
			MaxAttempts: 5,
			BaseDelay:   time.Hour,
			MaxDelay:    time.Hour,
			Sleeper:     func(ctx context.Context, d time.Duration) error { return nil },
		},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = client.GetRepository(ctx, testOwner, testRepo)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}
