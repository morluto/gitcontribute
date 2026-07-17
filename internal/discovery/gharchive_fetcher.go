package discovery

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// DefaultArchiveBaseURL is the canonical GH Archive hourly endpoint.
	DefaultArchiveBaseURL = "https://data.gharchive.org"
	// DefaultArchiveTimeout bounds a single hourly download.
	DefaultArchiveTimeout = 30 * time.Second
	// DefaultArchiveMaxBytes is the maximum compressed response body for one
	// hourly file (256 MiB). GH Archive hour files are typically well under
	// this size.
	DefaultArchiveMaxBytes = 256 << 20
)

var (
	// ErrHourNotAvailable is returned when the requested hour file does not
	// exist (HTTP 404).
	ErrHourNotAvailable = errors.New("archive hour not available")
	// ErrResponseTooLarge is returned when a response exceeds the configured
	// byte limit.
	ErrResponseTooLarge = errors.New("archive response exceeds size limit")
	// ErrBadStatus is returned for unexpected HTTP status codes.
	ErrBadStatus = errors.New("unexpected archive status")
)

// ArchiveFetcher downloads a single hourly GH Archive gzip file.
type ArchiveFetcher interface {
	Fetch(ctx context.Context, hour time.Time) (io.ReadCloser, error)
}

// ArchiveClient is a context-aware, bounded HTTP fetcher for GH Archive.
type ArchiveClient struct {
	baseURL    string
	httpClient *http.Client
	timeout    time.Duration
	maxBytes   int64
}

// NewArchiveClient returns an ArchiveFetcher with sensible production defaults.
func NewArchiveClient() ArchiveFetcher {
	return NewArchiveClientWithOptions(
		DefaultArchiveBaseURL,
		&http.Client{Timeout: DefaultArchiveTimeout},
		DefaultArchiveTimeout,
		DefaultArchiveMaxBytes,
	)
}

// NewArchiveClientWithOptions returns a fetcher using the supplied parameters.
// It is intended for tests and advanced configuration; callers should avoid
// exposing arbitrary base URLs to untrusted input.
func NewArchiveClientWithOptions(baseURL string, client *http.Client, timeout time.Duration, maxBytes int64) *ArchiveClient {
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	return &ArchiveClient{
		baseURL:    baseURL,
		httpClient: client,
		timeout:    timeout,
		maxBytes:   maxBytes,
	}
}

// Fetch builds the canonical https://data.gharchive.org/YYYY-MM-DD-H.json.gz
// URL, applies a per-request timeout, checks status and response size, and
// returns a ReadCloser that enforces maxBytes while streaming.
func (c *ArchiveClient) Fetch(ctx context.Context, hour time.Time) (io.ReadCloser, error) {
	hour = hour.UTC()
	url := fmt.Sprintf("%s/%04d-%02d-%02d-%d.json.gz",
		c.baseURL, hour.Year(), hour.Month(), hour.Day(), hour.Hour())

	cancel := func() {}
	if c.timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("build archive request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("fetch archive %s: %w", url, err)
	}
	closeBody := func() error {
		err := resp.Body.Close()
		cancel()
		return err
	}

	if resp.StatusCode == http.StatusNotFound {
		_ = closeBody()
		return nil, fmt.Errorf("%w: %s", ErrHourNotAvailable, url)
	}
	if resp.StatusCode != http.StatusOK {
		_ = closeBody()
		return nil, fmt.Errorf("%w: %s returned %d", ErrBadStatus, url, resp.StatusCode)
	}

	if c.maxBytes > 0 && resp.ContentLength > c.maxBytes {
		_ = closeBody()
		return nil, fmt.Errorf("%w: Content-Length %d for %s", ErrResponseTooLarge, resp.ContentLength, url)
	}

	if c.maxBytes <= 0 {
		return &ownedReadCloser{Reader: resp.Body, closeFunc: closeBody}, nil
	}
	return NewLimitedReader(resp.Body, c.maxBytes, closeBody, ErrResponseTooLarge), nil
}

type ownedReadCloser struct {
	io.Reader
	closeFunc func() error
}

func (r *ownedReadCloser) Close() error {
	return r.closeFunc()
}
