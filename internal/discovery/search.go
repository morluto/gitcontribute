package discovery

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

// DefaultSearchLimit is the GitHub Search hard result ceiling that forces
// query windowing.
const DefaultSearchLimit = 1000

const searchTimeLayout = "2006-01-02T15:04:05Z"

// Qualifier selects which GitHub Search timestamp field to window.
type Qualifier string

const (
	// Created windows historical backfills.
	Created Qualifier = "created"
	// Updated windows incremental refresh.
	Updated Qualifier = "updated"
	// Pushed windows incremental repository-search refreshes. GitHub repository
	// search supports pushed, not updated.
	Pushed Qualifier = "pushed"
)

// SearchItem is a product-owned search result item used when partitioning.
type SearchItem struct {
	Repo   domain.RepoRef
	Kind   domain.ThreadKind
	Number int
	Title  string
}

// SearchResponse is the product-owned result of a search count/list call.
// Incomplete means GitHub returned incomplete_results.
type SearchResponse struct {
	Total      int
	Items      []SearchItem
	Incomplete bool
}

// Searcher is the capability required to partition a search query.
type Searcher interface {
	Search(ctx context.Context, query string) (SearchResponse, error)
}

// Window is a bounded GitHub Search query with its total result count and
// whether it could not be split further.
type Window struct {
	Query        string
	Qualifier    Qualifier
	Start        time.Time
	End          time.Time
	Total        int
	Incomplete   bool
	Unsplittable bool
}

// SearchPartitioner splits a base query into time windows that each fit under
// a result-count limit.
type SearchPartitioner struct {
	Searcher Searcher
	// Limit is the result count that triggers splitting. Zero means 1000.
	Limit int
}

func (p *SearchPartitioner) limit() int {
	if p.Limit > 0 {
		return p.Limit
	}
	return DefaultSearchLimit
}

// Partition recursively splits baseQuery over [start, end] (inclusive) using
// the chosen qualifier until each window's total result count is at or below
// the configured limit or the window cannot be split further. Unsplittable
// windows with more than the limit are returned with Unsplittable set.
func (p *SearchPartitioner) Partition(ctx context.Context, baseQuery string, start, end time.Time, qual Qualifier) ([]Window, error) {
	if p == nil || p.Searcher == nil {
		return nil, fmt.Errorf("searcher is required")
	}
	if start.IsZero() || end.IsZero() {
		return nil, fmt.Errorf("start and end must be set")
	}
	if end.Before(start) {
		return nil, fmt.Errorf("end %v before start %v", end, start)
	}
	if qual != Created && qual != Updated && qual != Pushed {
		return nil, fmt.Errorf("invalid qualifier %q", qual)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	start = start.UTC().Truncate(time.Second)
	end = end.UTC().Truncate(time.Second)

	var windows []Window
	if err := p.partition(ctx, baseQuery, start, end, qual, &windows); err != nil {
		return nil, err
	}
	return windows, nil
}

func (p *SearchPartitioner) partition(ctx context.Context, baseQuery string, start, end time.Time, qual Qualifier, out *[]Window) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	query := buildQuery(baseQuery, qual, start, end)
	resp, err := p.Searcher.Search(ctx, query)
	if err != nil {
		return err
	}
	if resp.Total < 0 {
		return fmt.Errorf("search returned negative total %d", resp.Total)
	}

	if resp.Total <= p.limit() || !canSplit(start, end) {
		w := Window{
			Query:      query,
			Qualifier:  qual,
			Start:      start,
			End:        end,
			Total:      resp.Total,
			Incomplete: resp.Incomplete,
		}
		if resp.Total > p.limit() && !canSplit(start, end) {
			w.Unsplittable = true
		}
		*out = append(*out, w)
		return nil
	}

	mid := splitMid(start, end)
	if err := p.partition(ctx, baseQuery, start, mid, qual, out); err != nil {
		return err
	}

	rightStart := mid.Add(time.Second)
	if rightStart.After(end) {
		rightStart = end
	}
	if rightStart.Before(start) {
		rightStart = start
	}
	if rightStart.After(end) {
		rightStart = end
	}

	if rightStart.Equal(end) || rightStart.Before(end) {
		if err := p.partition(ctx, baseQuery, rightStart, end, qual, out); err != nil {
			return err
		}
	}
	return nil
}

// Refresh partitions an incremental updated query from the most recent
// checkpoint, extended backwards by overlap. The new checkpoint is written to
// store as end on success.
func (p *SearchPartitioner) Refresh(ctx context.Context, key, baseQuery string, start, end time.Time, overlap time.Duration, store CheckpointStore) ([]Window, error) {
	if start.IsZero() || end.IsZero() {
		return nil, fmt.Errorf("start and end must be set")
	}
	if end.Before(start) {
		return nil, fmt.Errorf("end %v before start %v", end, start)
	}
	if store == nil {
		return nil, fmt.Errorf("checkpoint store is required")
	}
	if overlap < 0 {
		return nil, fmt.Errorf("overlap cannot be negative")
	}

	refreshStart := start
	if last, ok, err := store.GetTime(ctx, key); err != nil {
		return nil, err
	} else if ok {
		candidate := last.Add(-overlap)
		if candidate.After(refreshStart) {
			refreshStart = candidate
		}
	}

	windows, err := p.Partition(ctx, baseQuery, refreshStart, end, Updated)
	if err != nil {
		return nil, err
	}

	if err := store.SetTime(ctx, key, end); err != nil {
		return nil, err
	}
	return windows, nil
}

func buildQuery(base string, qual Qualifier, start, end time.Time) string {
	rangeVal := fmt.Sprintf("%s:%s..%s", qual, start.UTC().Format(searchTimeLayout), end.UTC().Format(searchTimeLayout))
	base = strings.TrimSpace(base)
	if base == "" {
		return rangeVal
	}
	return base + " " + rangeVal
}

func canSplit(start, end time.Time) bool {
	return end.Sub(start) >= time.Second
}

func splitMid(start, end time.Time) time.Time {
	dur := end.Sub(start)
	mid := start.Add(dur / 2).Truncate(time.Second)
	if mid.Before(start) {
		mid = start
	}
	if mid.After(end) {
		mid = end
	}
	return mid
}
