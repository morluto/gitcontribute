package discovery

import (
	"context"
	"strings"
	"testing"
	"time"
)

type fakeSearcher struct {
	fn func(ctx context.Context, query string) (SearchResponse, error)
}

func (f *fakeSearcher) Search(ctx context.Context, query string) (SearchResponse, error) {
	return f.fn(ctx, query)
}

func parseWindow(query string) (start, end time.Time, qual Qualifier) {
	for _, q := range []string{"created:", "updated:", "pushed:"} {
		idx := strings.Index(query, q)
		if idx == -1 {
			continue
		}
		qual = Qualifier(q[:len(q)-1])
		rangeStr := query[idx+len(q):]
		parts := strings.Split(rangeStr, "..")
		if len(parts) == 2 {
			start, _ = time.Parse(time.RFC3339, parts[0])
			end, _ = time.Parse(time.RFC3339, parts[1])
		}
		return
	}
	return
}

func countByDuration(query string) int {
	start, end, _ := parseWindow(query)
	if start.IsZero() || end.IsZero() {
		return 0
	}
	// inclusive second count
	secs := int(end.Sub(start).Seconds()) + 1
	if secs < 1 {
		secs = 1
	}
	return secs * 2
}

func TestPartitionSingleWindow(t *testing.T) {
	p := &SearchPartitioner{
		Searcher: &fakeSearcher{
			fn: func(ctx context.Context, query string) (SearchResponse, error) {
				return SearchResponse{Total: 500}, nil
			},
		},
	}
	start := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	windows, err := p.Partition(context.Background(), "language:go", start, end, Created)
	if err != nil {
		t.Fatalf("Partition: %v", err)
	}
	if len(windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(windows))
	}
	if windows[0].Total != 500 {
		t.Fatalf("expected total 500, got %d", windows[0].Total)
	}
	if windows[0].Unsplittable {
		t.Fatal("expected splittable window")
	}
}

func TestPartitionSplitsOverLimit(t *testing.T) {
	p := &SearchPartitioner{Searcher: &fakeSearcher{fn: func(ctx context.Context, query string) (SearchResponse, error) {
		n := countByDuration(query)
		return SearchResponse{Total: n}, nil
	}}}
	start := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(1000 * time.Second)
	windows, err := p.Partition(context.Background(), "language:go", start, end, Created)
	if err != nil {
		t.Fatalf("Partition: %v", err)
	}
	if len(windows) == 0 {
		t.Fatal("expected windows")
	}
	for i, w := range windows {
		if w.Total > p.limit() && !w.Unsplittable {
			t.Fatalf("window %d total %d over limit but not unsplittable", i, w.Total)
		}
	}
	for i := 1; i < len(windows); i++ {
		want := windows[i-1].End.Add(time.Second)
		if !windows[i].Start.Equal(want) {
			t.Fatalf("window %d starts at %v, want contiguous %v", i, windows[i].Start, want)
		}
	}
}

func TestPartitionRequiresSearcher(t *testing.T) {
	start := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := (&SearchPartitioner{}).Partition(context.Background(), "", start, start, Created); err == nil {
		t.Fatal("expected missing searcher error")
	}
}

func TestRefreshRejectsNegativeOverlap(t *testing.T) {
	start := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	p := &SearchPartitioner{Searcher: &fakeSearcher{fn: func(context.Context, string) (SearchResponse, error) {
		return SearchResponse{}, nil
	}}}
	_, err := p.Refresh(context.Background(), "key", "", start, start, -time.Second, NewMemoryCheckpointStore())
	if err == nil {
		t.Fatal("expected negative overlap error")
	}
}

func TestPartitionDetectsUnsplittable(t *testing.T) {
	p := &SearchPartitioner{
		Searcher: &fakeSearcher{
			fn: func(ctx context.Context, query string) (SearchResponse, error) {
				return SearchResponse{Total: 1001}, nil
			},
		},
	}
	start := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start // one-second window
	windows, err := p.Partition(context.Background(), "", start, end, Created)
	if err != nil {
		t.Fatalf("Partition: %v", err)
	}
	if len(windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(windows))
	}
	if !windows[0].Unsplittable {
		t.Fatalf("expected window to be unsplittable: %+v", windows[0])
	}
	if windows[0].Total != 1001 {
		t.Fatalf("expected total 1001, got %d", windows[0].Total)
	}
}

func TestPartitionRespectsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := &SearchPartitioner{
		Searcher: &fakeSearcher{
			fn: func(ctx context.Context, query string) (SearchResponse, error) {
				return SearchResponse{Total: 1}, nil
			},
		},
	}
	start := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	_, err := p.Partition(ctx, "", start, end, Created)
	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestPartitionValidatesBoundaries(t *testing.T) {
	p := &SearchPartitioner{Searcher: &fakeSearcher{fn: func(ctx context.Context, query string) (SearchResponse, error) {
		return SearchResponse{}, nil
	}}}
	start := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := p.Partition(context.Background(), "", time.Time{}, start, Created); err == nil {
		t.Fatal("expected error for zero start")
	}
	if _, err := p.Partition(context.Background(), "", start, start.Add(-time.Hour), Created); err == nil {
		t.Fatal("expected error for end before start")
	}
	if _, err := p.Partition(context.Background(), "", start, start.Add(time.Hour), Qualifier("foo")); err == nil {
		t.Fatal("expected error for invalid qualifier")
	}
}

func TestRefreshOverlapCheckpoint(t *testing.T) {
	store := NewMemoryCheckpointStore()
	start := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	last := start.Add(30 * time.Minute)
	_ = store.SetTime(context.Background(), "src", last)

	p := &SearchPartitioner{
		Searcher: &fakeSearcher{
			fn: func(ctx context.Context, query string) (SearchResponse, error) {
				s, e, q := parseWindow(query)
				if s.IsZero() || e.IsZero() {
					return SearchResponse{}, nil
				}
				if q != Updated {
					t.Fatalf("expected updated qualifier, got %q", q)
				}
				return SearchResponse{Total: 1}, nil
			},
		},
	}

	end := start.Add(time.Hour)
	overlap := 5 * time.Minute
	windows, err := p.Refresh(context.Background(), "src", "language:go", start, end, overlap, store)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if len(windows) == 0 {
		t.Fatal("expected windows")
	}

	wantStart := last.Add(-overlap)
	if !windows[0].Start.Equal(wantStart) {
		t.Fatalf("expected refresh start %v, got %v", wantStart, windows[0].Start)
	}

	cp, ok, err := store.GetTime(context.Background(), "src")
	if err != nil {
		t.Fatalf("GetTime: %v", err)
	}
	if !ok || !cp.Equal(end) {
		t.Fatalf("expected checkpoint %v, got %v", end, cp)
	}
}
