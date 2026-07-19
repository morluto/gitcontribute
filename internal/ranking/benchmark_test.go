package ranking_test

import (
	"fmt"
	"sort"
	"testing"

	"github.com/morluto/gitcontribute/internal/ranking"
)

func BenchmarkTopK(b *testing.B) {
	values := make([]rankedValue, 10_000)
	for i := range values {
		values[i] = rankedValue{Score: float64((i * 7919) % 997), Ref: fmt.Sprintf("ref-%05d", i)}
	}
	better := func(a, b rankedValue) bool {
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		return a.Ref < b.Ref
	}
	b.Run("heap-100", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			_ = ranking.TopK(values, 100, better)
		}
	})
	b.Run("full-sort-100", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			copyValues := append([]rankedValue(nil), values...)
			sort.Slice(copyValues, func(i, j int) bool { return better(copyValues[i], copyValues[j]) })
			_ = copyValues[:100]
		}
	})
}
