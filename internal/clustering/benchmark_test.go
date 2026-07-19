package clustering_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/morluto/gitcontribute/internal/clustering"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/similarity"
)

func BenchmarkClusterEngine(b *testing.B) {
	engine, err := clustering.NewEngine(similarity.DefaultDuplicateRule(), clustering.DefaultExactPairBudget())
	if err != nil {
		b.Fatal(err)
	}
	for _, size := range []int{100, 1000, 4472} {
		b.Run(fmt.Sprintf("%d", size), func(b *testing.B) {
			candidates := benchmarkCandidates(size)
			required, err := clustering.DefaultExactPairBudget().Required(size)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ReportMetric(float64(required), "pairs/op")
			b.ResetTimer()
			for range b.N {
				result, err := engine.Cluster(context.Background(), candidates)
				if err != nil {
					b.Fatal(err)
				}
				if result.ComparedPairs != required {
					b.Fatalf("compared %d pairs, want %d", result.ComparedPairs, required)
				}
			}
		})
	}
}

func benchmarkCandidates(count int) []clustering.Candidate {
	candidates := make([]clustering.Candidate, count)
	for i := range candidates {
		candidates[i] = clustering.Candidate{Repo: domain.RepoRef{Owner: "bench", Repo: "repo"}, Kind: "issue", Number: i + 1, Title: fmt.Sprintf("token%08d", i)}
	}
	return candidates
}
