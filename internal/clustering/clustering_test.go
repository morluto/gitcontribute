package clustering_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/morluto/gitcontribute/internal/clustering"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/similarity"
)

func TestDefaultExactPairBudgetHasExecutableBoundary(t *testing.T) {
	budget := clustering.DefaultExactPairBudget()
	if got := budget.MaxCandidates(); got != 4472 {
		t.Fatalf("maximum candidates = %d, want 4472", got)
	}
	if got, err := budget.Required(4472); err != nil || got != 9_997_156 {
		t.Fatalf("required pairs at boundary = (%d, %v), want (9997156, nil)", got, err)
	}
	_, err := budget.Required(4473)
	var capacity *clustering.CapacityError
	if !errors.As(err, &capacity) {
		t.Fatalf("error = %v, want CapacityError", err)
	}
	if capacity.CandidateCount != 4473 || capacity.RequiredPairs != 10_001_628 || capacity.AllowedPairs != 10_000_000 {
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

func TestSimilarityTextPreparation(t *testing.T) {
	got := similarity.Tokens("Hello, World! 123", true)
	want := []string{"123", "hello", "world"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("tokens mismatch (-want +got):\n%s", diff)
	}
	for _, token := range similarity.Tokens("the quick brown fox", true) {
		if token == "the" {
			t.Fatal("stop word found in tokens")
		}
	}
}

func TestSimilarityExtractsGitHubReferences(t *testing.T) {
	repo := domain.RepoRef{Owner: "owner", Repo: "repo"}
	for _, tc := range []struct {
		input string
		want  int
	}{
		{"see #42 for context", 1},
		{"fix owner/repo#7", 1},
		{"https://github.com/owner/repo/issues/3", 1},
		{"see github.com/owner/repo/pull/4", 1},
		{"https://notgithub.com/owner/repo/issues/3", 0},
		{"nothing here", 0},
	} {
		if refs := similarity.ExtractRefs(tc.input, repo); len(refs) != tc.want {
			t.Fatalf("ExtractRefs(%q) = %d refs, want %d", tc.input, len(refs), tc.want)
		}
	}
	refs := similarity.ExtractRefs("see #42", repo)
	if len(refs) != 1 || refs[0].Number != 42 || refs[0].Kind != "" {
		t.Fatalf("bare ref mismatch: %+v", refs)
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

func TestEngineEnforcesPairBudget(t *testing.T) {
	engine, err := clustering.NewEngine(similarity.DefaultDuplicateRule(), clustering.ExactPairBudget(1))
	if err != nil {
		t.Fatal(err)
	}
	_, err = engine.Cluster(context.Background(), []clustering.Candidate{{}, {}, {}})
	if err == nil {
		t.Fatal("expected pair-budget error")
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
	engine, err := clustering.NewEngine(similarity.DefaultDuplicateRule(), clustering.DefaultExactPairBudget())
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
