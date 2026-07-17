package discovery

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

func gzipLines(lines ...[]byte) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	for _, line := range lines {
		gw.Write(line)
		gw.Write([]byte("\n"))
	}
	gw.Close()
	return buf.Bytes()
}

func eventLine(t string, payload map[string]any) []byte {
	ev := map[string]any{
		"id":         "1",
		"type":       t,
		"actor":      map[string]any{"id": 1, "login": "alice"},
		"repo":       map[string]any{"id": 1, "name": "owner/repo"},
		"payload":    payload,
		"created_at": "2023-01-01T00:00:00Z",
	}
	b, _ := json.Marshal(ev)
	return b
}

func TestArchiveReaderSkipsMalformedLines(t *testing.T) {
	lines := [][]byte{
		eventLine("PushEvent", map[string]any{"ref": "refs/heads/main", "head": "abc", "size": 1}),
		[]byte("this is not json"),
		[]byte(`{"type":"IssuesEvent",`),
		[]byte(""),
	}
	reader := NewArchiveReader(nil, nil)
	var got []Signal
	err := reader.Read(context.Background(), time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), bytes.NewReader(gzipLines(lines...)), func(s Signal) error {
		got = append(got, s)
		return nil
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(got))
	}
	if got[0].EventType != PushEvent {
		t.Fatalf("expected PushEvent, got %v", got[0].EventType)
	}
}

func TestArchiveReaderFiltersEventTypes(t *testing.T) {
	lines := [][]byte{
		eventLine("PushEvent", map[string]any{"ref": "refs/heads/main", "head": "abc", "size": 1}),
		eventLine("IssuesEvent", map[string]any{"action": "opened", "issue": map[string]any{"number": 1, "title": "x", "state": "open", "user": map[string]any{"login": "u"}}}),
		eventLine("WatchEvent", map[string]any{"action": "started"}),
	}
	reader := NewArchiveReader([]string{"IssuesEvent", "PullRequestEvent"}, nil)
	var got []Signal
	err := reader.Read(context.Background(), time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), bytes.NewReader(gzipLines(lines...)), func(s Signal) error {
		got = append(got, s)
		return nil
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(got))
	}
	if got[0].EventType != IssuesEvent {
		t.Fatalf("expected IssuesEvent, got %v", got[0].EventType)
	}
}

func TestArchiveReaderDuplicateHour(t *testing.T) {
	store := NewMemoryCheckpointStore()
	hour := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	_ = store.MarkImported(context.Background(), HourKey(hour))

	reader := NewArchiveReader(nil, store)
	var got []Signal
	err := reader.Read(context.Background(), hour, bytes.NewReader(gzipLines(eventLine("PushEvent", map[string]any{}))), func(s Signal) error {
		got = append(got, s)
		return nil
	})
	if !errors.Is(err, ErrAlreadyImported) {
		t.Fatalf("expected ErrAlreadyImported, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 signals, got %d", len(got))
	}
}

func TestArchiveReaderContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	reader := NewArchiveReader(nil, nil)
	err := reader.Read(ctx, time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), bytes.NewReader(gzipLines(eventLine("PushEvent", map[string]any{}))), func(s Signal) error {
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestArchiveReaderRepresentativeEvents(t *testing.T) {
	hour := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	lines := [][]byte{
		eventLine("PushEvent", map[string]any{
			"ref":    "refs/heads/main",
			"head":   "abc123",
			"before": "000000",
			"size":   2,
			"commits": []any{
				map[string]any{"sha": "a"},
				map[string]any{"sha": "b"},
			},
		}),
		eventLine("IssuesEvent", map[string]any{
			"action": "opened",
			"issue": map[string]any{
				"number": 42,
				"title":  "A bug",
				"state":  "open",
				"user":   map[string]any{"login": "bob"},
			},
		}),
		eventLine("PullRequestEvent", map[string]any{
			"action": "opened",
			"number": 7,
			"pull_request": map[string]any{
				"number": 7,
				"title":  "A PR",
				"state":  "open",
				"user":   map[string]any{"login": "carol"},
			},
		}),
		eventLine("WatchEvent", map[string]any{"action": "started"}),
		eventLine("ForkEvent", map[string]any{"forkee": map[string]any{"full_name": "forker/repo", "id": 99}}),
	}

	reader := NewArchiveReader(nil, nil)
	var got []Signal
	err := reader.Read(context.Background(), hour, bytes.NewReader(gzipLines(lines...)), func(s Signal) error {
		got = append(got, s)
		return nil
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("expected 5 signals, got %d", len(got))
	}

	push := got[0]
	if push.EventType != PushEvent || push.Action != "pushed" || push.Ref != "refs/heads/main" || push.SHA != "abc123" || push.Size != 2 {
		t.Fatalf("unexpected push signal: %+v", push)
	}

	issue := got[1]
	if issue.EventType != IssuesEvent || issue.Action != "opened" || issue.ThreadKind != domain.IssueKind || issue.ThreadNumber != 42 || issue.ThreadTitle != "A bug" || issue.ThreadAuthor != "bob" || issue.ThreadState != domain.OpenState {
		t.Fatalf("unexpected issue signal: %+v", issue)
	}

	pr := got[2]
	if pr.EventType != PullRequestEvent || pr.Action != "opened" || pr.ThreadKind != domain.PullRequestKind || pr.ThreadNumber != 7 || pr.ThreadTitle != "A PR" || pr.ThreadAuthor != "carol" || pr.ThreadState != domain.OpenState {
		t.Fatalf("unexpected pr signal: %+v", pr)
	}

	watch := got[3]
	if watch.EventType != WatchEvent || watch.Action != "started" {
		t.Fatalf("unexpected watch signal: %+v", watch)
	}

	fork := got[4]
	if fork.EventType != ForkEvent || fork.Action != "forked" {
		t.Fatalf("unexpected fork signal: %+v", fork)
	}
}

func TestHourKey(t *testing.T) {
	h := time.Date(2023, 1, 1, 12, 30, 0, 0, time.FixedZone("+0530", 5*3600+30*60))
	if got := HourKey(h); !strings.HasPrefix(got, "2023010107") {
		t.Fatalf("expected UTC hour key 2023010107*, got %q", got)
	}
}

func TestArchiveReaderBoundsEventLines(t *testing.T) {
	reader := NewArchiveReader(nil, nil)
	reader.MaxEventBytes = 128
	line := eventLine("PushEvent", map[string]any{"head": strings.Repeat("a", 256)})
	err := reader.Read(context.Background(), time.Now(), bytes.NewReader(gzipLines(line)), func(Signal) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "token too long") {
		t.Fatalf("expected bounded-line error, got %v", err)
	}
}

func TestMemoryCheckpointStoreRespectsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := NewMemoryCheckpointStore()
	if err := store.MarkImported(ctx, "hour"); !errors.Is(err, context.Canceled) {
		t.Fatalf("MarkImported error = %v, want context.Canceled", err)
	}
}

func TestArchiveReaderRejectsDecompressionBomb(t *testing.T) {
	// Build a gzip stream with many small lines that decompresses to more
	// than the MaxTotalBytes limit.
	var uncompressed bytes.Buffer
	for i := 0; i < 1000; i++ {
		uncompressed.WriteString(`{"id":"` + fmt.Sprintf("%d", i) + `","type":"PushEvent","actor":{"id":1,"login":"a"},"repo":{"id":1,"name":"owner/repo"},"payload":{"ref":"refs/heads/main","head":"abc","size":1},"created_at":"2023-01-01T00:00:00Z"}` + "\n")
	}
	var gz bytes.Buffer
	gw := gzip.NewWriter(&gz)
	gw.Write(uncompressed.Bytes())
	gw.Close()

	reader := NewArchiveReader(nil, nil)
	reader.MaxTotalBytes = 256
	var count int
	err := reader.Read(context.Background(), time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), bytes.NewReader(gz.Bytes()), func(Signal) error {
		count++
		return nil
	})
	if !errors.Is(err, ErrDecompressedTooLarge) {
		t.Fatalf("expected ErrDecompressedTooLarge, got %v", err)
	}
}

func TestArchiveReaderSkipsMalformedAndUnknownEvents(t *testing.T) {
	hour := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	valid := eventLine("IssuesEvent", map[string]any{
		"action": "opened",
		"issue":  map[string]any{"number": 1, "title": "bug", "state": "open", "user": map[string]any{"login": "u"}},
	})
	unknown := []byte(`{"id":"2","type":"UnknownEvent","actor":{"id":1,"login":"a"},"repo":{"id":1,"name":"owner/repo"},"payload":{},"created_at":"2023-01-01T00:00:00Z"}`)
	malformed := []byte(`this is not json`)
	incomplete := []byte(`{"type":"IssuesEvent",`)

	reader := NewArchiveReader(nil, nil)
	var got []Signal
	err := reader.Read(context.Background(), hour, bytes.NewReader(gzipLines(valid, unknown, malformed, incomplete)), func(s Signal) error {
		got = append(got, s)
		return nil
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 1 || got[0].EventType != IssuesEvent {
		t.Fatalf("expected 1 IssuesEvent, got %+v", got)
	}
}

func TestArchiveHourRange(t *testing.T) {
	now := time.Date(2023, 1, 3, 5, 15, 0, 0, time.UTC)
	start, end := ArchiveHourRange(2*time.Hour, now)
	wantStart := time.Date(2023, 1, 3, 3, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2023, 1, 3, 4, 0, 0, 0, time.UTC)
	if !start.Equal(wantStart) {
		t.Fatalf("start = %v, want %v", start, wantStart)
	}
	if !end.Equal(wantEnd) {
		t.Fatalf("end = %v, want %v", end, wantEnd)
	}

	hour := time.Date(2023, 1, 3, 5, 0, 0, 0, time.UTC)
	start, end = ArchiveHourRange(time.Hour, hour)
	wantEnd = time.Date(2023, 1, 3, 4, 0, 0, 0, time.UTC)
	wantStart = wantEnd
	if !start.Equal(wantStart) || !end.Equal(wantEnd) {
		t.Fatalf("on-the-hour bounds = %v to %v, want %v to %v", start, end, wantStart, wantEnd)
	}
}
