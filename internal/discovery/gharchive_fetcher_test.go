package discovery

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestArchiveClientFetchSuccess(t *testing.T) {
	body := gzipLines(eventLine("PushEvent", map[string]any{"ref": "refs/heads/main", "head": "abc", "size": 1}))
	fetched := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetched++
		if !strings.HasSuffix(r.URL.Path, ".json.gz") {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/gzip")
		w.Write(body)
	}))
	defer srv.Close()

	client := NewArchiveClientWithOptions(srv.URL, srv.Client(), 0, 1<<20)
	rc, err := client.Fetch(context.Background(), time.Date(2023, 1, 2, 3, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(data) != len(body) {
		t.Fatalf("got %d bytes, want %d", len(data), len(body))
	}
	if fetched != 1 {
		t.Fatalf("fetched %d times, want 1", fetched)
	}
}

func TestArchiveClientTimeoutCoversBodyStreaming(t *testing.T) {
	flushed := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(flushed)
		time.Sleep(20 * time.Millisecond)
		_, _ = w.Write([]byte("streamed body"))
	}))
	defer srv.Close()

	client := NewArchiveClientWithOptions(srv.URL, srv.Client(), time.Second, 1<<20)
	rc, err := client.Fetch(context.Background(), time.Date(2023, 1, 2, 3, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	defer rc.Close()
	<-flushed

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "streamed body" {
		t.Fatalf("body = %q, want %q", data, "streamed body")
	}
}

func TestArchiveClientNoSizeLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "body")
	}))
	defer srv.Close()

	client := NewArchiveClientWithOptions(srv.URL, srv.Client(), 0, 0)
	rc, err := client.Fetch(context.Background(), time.Date(2023, 1, 2, 3, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "body" {
		t.Fatalf("body = %q, want body", data)
	}
}

func TestArchiveClientFetchURLConstruction(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client := NewArchiveClientWithOptions(srv.URL, srv.Client(), 0, 1<<20)
	_, err := client.Fetch(context.Background(), time.Date(2023, 1, 2, 3, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected error")
	}
	want := "/2023-01-02-3.json.gz"
	if gotPath != want {
		t.Fatalf("path = %q, want %q", gotPath, want)
	}
}

func TestArchiveClientFetchNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client := NewArchiveClientWithOptions(srv.URL, srv.Client(), 0, 1<<20)
	_, err := client.Fetch(context.Background(), time.Date(2023, 1, 2, 3, 0, 0, 0, time.UTC))
	if !errors.Is(err, ErrHourNotAvailable) {
		t.Fatalf("expected ErrHourNotAvailable, got %v", err)
	}
}

func TestArchiveClientFetchBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewArchiveClientWithOptions(srv.URL, srv.Client(), 0, 1<<20)
	_, err := client.Fetch(context.Background(), time.Date(2023, 1, 2, 3, 0, 0, 0, time.UTC))
	if !errors.Is(err, ErrBadStatus) {
		t.Fatalf("expected ErrBadStatus, got %v", err)
	}
}

func TestArchiveClientFetchResponseSizeLimit(t *testing.T) {
	body := gzipLines([]byte(strings.Repeat("x", 1<<20)))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.Write(body)
	}))
	defer srv.Close()

	client := NewArchiveClientWithOptions(srv.URL, srv.Client(), 0, 64)
	rc, err := client.Fetch(context.Background(), time.Date(2023, 1, 2, 3, 0, 0, 0, time.UTC))
	if err != nil {
		if !errors.Is(err, ErrResponseTooLarge) {
			t.Fatalf("expected ErrResponseTooLarge from Fetch, got %v", err)
		}
		return
	}
	defer rc.Close()

	_, err = io.ReadAll(rc)
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("expected ErrResponseTooLarge, got %v", err)
	}
}

func TestArchiveClientFetchContentLengthRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1073741824") // 1 GiB
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewArchiveClientWithOptions(srv.URL, srv.Client(), 0, 1024)
	_, err := client.Fetch(context.Background(), time.Date(2023, 1, 2, 3, 0, 0, 0, time.UTC))
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("expected ErrResponseTooLarge, got %v", err)
	}
}

func TestArchiveClientFetchTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewArchiveClientWithOptions(srv.URL, srv.Client(), 10*time.Millisecond, 1<<20)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := client.Fetch(ctx, time.Date(2023, 1, 2, 3, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestArchiveClientFetchContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewArchiveClientWithOptions(srv.URL, srv.Client(), 0, 1<<20)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.Fetch(ctx, time.Date(2023, 1, 2, 3, 0, 0, 0, time.UTC))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestArchiveClientDefaultUsesProductionURL(t *testing.T) {
	f := NewArchiveClient()
	client, ok := f.(*ArchiveClient)
	if !ok {
		t.Fatalf("expected *ArchiveClient, got %T", f)
	}
	if client.baseURL != DefaultArchiveBaseURL {
		t.Fatalf("baseURL = %q, want %q", client.baseURL, DefaultArchiveBaseURL)
	}
}
