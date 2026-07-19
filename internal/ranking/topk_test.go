package ranking_test

import (
	"slices"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/morluto/gitcontribute/internal/ranking"
)

type rankedValue struct {
	Score float64
	Ref   string
}

func betterRankedValue(a, b rankedValue) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	return a.Ref < b.Ref
}

func TestTopKMatchesCanonicalFullSortPrefix(t *testing.T) {
	input := []rankedValue{
		{Score: 0.4, Ref: "c"},
		{Score: 0.9, Ref: "b"},
		{Score: 0.9, Ref: "a"},
		{Score: 0.1, Ref: "d"},
	}
	original := slices.Clone(input)
	want := slices.Clone(input)
	sort.Slice(want, func(i, j int) bool { return betterRankedValue(want[i], want[j]) })
	want = want[:2]

	got := ranking.TopK(input, 2, betterRankedValue)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("top-k mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(original, input); diff != "" {
		t.Fatalf("TopK mutated input (-want +got):\n%s", diff)
	}
}
