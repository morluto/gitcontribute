package github

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRetryTransportRetriesExplicitGraphQLReadAndRecreatesBody(t *testing.T) {
	ft := &fakeTransport{results: []fakeResult{
		{status: http.StatusInternalServerError, body: "boom"},
		{status: http.StatusOK, body: "ok"},
	}}
	rt := &retryTransport{
		Base: ft,
		Config: &RetryConfig{
			MaxAttempts: 2,
			BaseDelay:   time.Nanosecond,
			MaxDelay:    time.Nanosecond,
			Sleeper:     (&fakeSleeper{}).Sleep,
		},
	}
	req, err := http.NewRequest(http.MethodPost, "http://example.com/graphql", bytes.NewBufferString(`{"query":"query ReadOnly { viewer { login } }"}`))
	if err != nil {
		t.Fatal(err)
	}
	req = markReplayableRead(req)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || ft.index != 2 {
		t.Fatalf("GraphQL retry status=%d attempts=%d", resp.StatusCode, ft.index)
	}
	for i, attempt := range ft.requests {
		body, err := io.ReadAll(attempt.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != `{"query":"query ReadOnly { viewer { login } }"}` {
			t.Fatalf("attempt %d body = %q", i+1, body)
		}
	}
}

type trackingReadCloser struct {
	reader *strings.Reader
	read   int
	closed bool
}

func (r *trackingReadCloser) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.read += n
	return n, err
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return nil
}

func TestDrainAndCloseIsBounded(t *testing.T) {
	body := &trackingReadCloser{reader: strings.NewReader(strings.Repeat("x", 2*maxRetryDrainBytes))}
	drainAndClose(&http.Response{Body: body})
	if body.read != maxRetryDrainBytes || !body.closed {
		t.Fatalf("drained=%d closed=%v", body.read, body.closed)
	}
}
