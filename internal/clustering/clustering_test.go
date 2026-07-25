package clustering_test

import (
	"context"
	"errors"
	"testing"

	"github.com/morluto/gitcontribute/internal/clustering"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/similarity"
)

func TestDefaultComparisonBudgetHasExecutableBoundary(t *testing.T) {
	budget := clustering.DefaultComparisonBudget()
	if got := budget.MaxCandidates(); got != 4472 {
		t.Fatalf("maximum candidates = %d, want 4472", got)
	}
	if got, err := budget.Possible(4472); err != nil || got != 9_997_156 {
		t.Fatalf("possible pairs at boundary = (%d, %v), want (9997156, nil)", got, err)
	}
	_, err := budget.Possible(4473)
	var capacity *clustering.CapacityError
	if !errors.As(err, &capacity) {
		t.Fatalf("error = %v, want CapacityError", err)
	}
	if capacity.CandidateCount != 4473 || capacity.PossiblePairs != 10_001_628 || capacity.AllowedPairs != 10_000_000 {
		t.Fatalf("capacity error = %+v", capacity)
	}
}

func TestEngineHonorsCancellationBeforeExactWork(t *testing.T) {
	engine := defaultEngine(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := engine.Cluster(ctx, []clustering.Candidate{
		{Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, Kind: "issue", Number: 1, Title: "same"},
		{Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, Kind: "issue", Number: 2, Title: "same"},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cluster error = %v, want context.Canceled", err)
	}
}

func TestNeighborsHonorsCancellationBeforeScoring(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := clustering.Neighbors(ctx, clustering.Candidate{}, []clustering.Candidate{{}}, 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("neighbors error = %v, want context.Canceled", err)
	}
}

func TestEngineProducesExplainableSignals(t *testing.T) {
	clusters := clusterCandidates(t, []clustering.Candidate{
		{Title: "fix login crash", Body: "login crashes on startup", Author: "alice", Labels: []string{"bug"}},
		{Title: "fix login crash", Body: "login also crashes", Author: "alice", Labels: []string{"bug"}},
	})
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(clusters))
	}
	for _, member := range clusters[0].Members {
		if member.Reason == "" || member.Score <= 0 || member.Score > 1 {
			t.Fatalf("invalid member score explanation: %+v", member)
		}
	}
}

func TestEngineUsesExplicitReferences(t *testing.T) {
	clusters := clusterCandidates(t, []clustering.Candidate{
		{Repo: domain.RepoRef{Owner: "o", Repo: "r"}, Kind: "issue", Number: 1, Title: "bug", Body: "first"},
		{Repo: domain.RepoRef{Owner: "o", Repo: "r"}, Kind: "issue", Number: 2, Title: "other", Body: "duplicate of #1"},
		{Repo: domain.RepoRef{Owner: "o", Repo: "r"}, Kind: "issue", Number: 3, Title: "unrelated", Body: "nothing"},
	})
	if len(clusters) != 1 || len(clusters[0].Members) != 2 || clusters[0].Canonical.Number != 1 {
		t.Fatalf("explicit-reference cluster = %+v", clusters)
	}
	for _, member := range clusters[0].Members {
		if member.Ref.Number == 2 && (member.Score < 0.4 || member.Reason != "explicit reference") {
			t.Fatalf("explicit-reference member = %+v", member)
		}
	}
}

func TestEngineRejectsUnrelatedCandidates(t *testing.T) {
	clusters := clusterCandidates(t, []clustering.Candidate{
		{Repo: domain.RepoRef{Owner: "o", Repo: "r"}, Kind: "issue", Number: 1, Title: "fix login crash", Body: "crash"},
		{Repo: domain.RepoRef{Owner: "o", Repo: "r"}, Kind: "issue", Number: 2, Title: "add dark mode", Body: "theme"},
	})
	if len(clusters) != 0 {
		t.Fatalf("expected no clusters, got %d", len(clusters))
	}
}

func TestStableIDIsDeterministic(t *testing.T) {
	a := clustering.Candidate{Repo: domain.RepoRef{Owner: "o", Repo: "r"}, Kind: "issue", Number: 1, Title: "duplicate title"}
	b := clustering.Candidate{Repo: domain.RepoRef{Owner: "o", Repo: "r"}, Kind: "issue", Number: 2, Title: "duplicate title"}
	first := clusterCandidates(t, []clustering.Candidate{a, b})
	second := clusterCandidates(t, []clustering.Candidate{b, a})
	if len(first) != 1 || len(second) != 1 || first[0].StableID != second[0].StableID {
		t.Fatalf("stable ids drifted: %+v / %+v", first, second)
	}
}

func TestEngineReportsPossibleAndScoredPairs(t *testing.T) {
	engine := defaultEngine(t)
	result, err := engine.Cluster(context.Background(), []clustering.Candidate{
		{Repo: domain.RepoRef{Owner: "o", Repo: "r"}, Kind: "issue", Number: 1, Title: "fix login crash"},
		{Repo: domain.RepoRef{Owner: "o", Repo: "r"}, Kind: "issue", Number: 2, Title: "fix login crash"},
		{Repo: domain.RepoRef{Owner: "o", Repo: "r"}, Kind: "issue", Number: 3, Title: "different", Body: "duplicate of #1"},
		{Repo: domain.RepoRef{Owner: "o", Repo: "r"}, Kind: "issue", Number: 4, Title: "unrelated"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.PossiblePairs != 6 || result.ScoredPairs != 2 {
		t.Fatalf("pair stats = %d possible, %d scored; want 6 possible, 2 scored", result.PossiblePairs, result.ScoredPairs)
	}
	if len(result.Clusters) != 1 || len(result.Clusters[0].Members) != 3 {
		t.Fatalf("clusters = %+v", result.Clusters)
	}
}

func TestEngineEnforcesWorstCaseComparisonBudget(t *testing.T) {
	engine, err := clustering.NewEngine(similarity.DefaultDuplicateRule(), clustering.ComparisonBudget(1))
	if err != nil {
		t.Fatal(err)
	}
	_, err = engine.Cluster(context.Background(), []clustering.Candidate{{}, {}, {}})
	if err == nil {
		t.Fatal("expected comparison-budget error")
	}
}

func TestSourceRevisionIncludesContentButIgnoresLabelOrder(t *testing.T) {
	base := clustering.Candidate{Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, Kind: "issue", Number: 1, Title: "title", Body: "original", Labels: []string{"bug", "help wanted"}}
	if got := len(clustering.SourceRevision([]clustering.Candidate{base})); got != 64 {
		t.Fatalf("source revision length = %d, want full SHA-256 hex digest", got)
	}
	reordered := base
	reordered.Labels = []string{"help wanted", "bug"}
	if clustering.SourceRevision([]clustering.Candidate{base}) != clustering.SourceRevision([]clustering.Candidate{reordered}) {
		t.Fatal("label order changed source revision")
	}
	changed := base
	changed.Body = "corrected"
	if clustering.SourceRevision([]clustering.Candidate{base}) == clustering.SourceRevision([]clustering.Candidate{changed}) {
		t.Fatal("content change did not change source revision")
	}
}

func TestDuplicateLabelsDoNotInflateSimilarity(t *testing.T) {
	clusters := clusterCandidates(t, []clustering.Candidate{
		{Repo: domain.RepoRef{Owner: "o", Repo: "r"}, Kind: "issue", Number: 1, Labels: []string{"bug"}},
		{Repo: domain.RepoRef{Owner: "o", Repo: "r"}, Kind: "issue", Number: 2, Labels: []string{"bug", "bug"}},
	})
	if len(clusters) != 0 {
		t.Fatalf("duplicate labels inflated similarity into %d cluster(s)", len(clusters))
	}
}

func defaultEngine(t *testing.T) clustering.Engine {
	t.Helper()
	engine, err := clustering.NewEngine(similarity.DefaultDuplicateRule(), clustering.DefaultComparisonBudget())
	if err != nil {
		t.Fatal(err)
	}
	return engine
}

func clusterCandidates(t *testing.T, candidates []clustering.Candidate) []clustering.Cluster {
	t.Helper()
	result, err := defaultEngine(t).Cluster(context.Background(), candidates)
	if err != nil {
		t.Fatal(err)
	}
	return result.Clusters
}
