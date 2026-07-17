package clustering_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/morluto/gitcontribute/internal/clustering"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
)

func openTestCorpus(t *testing.T) (*corpus.Corpus, string) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "corpus.db")
	c, err := corpus.Open(ctx, path)
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, path
}

func seedRepoAndThreads(t *testing.T, c *corpus.Corpus) (int64, []clustering.Candidate) {
	t.Helper()
	ctx := context.Background()
	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "123", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("seed repository: %v", err)
	}

	threads := []struct {
		kind   string
		number int
		title  string
		body   string
		author string
		labels []string
	}{
		{corpus.ThreadKindIssue, 1, "fix login crash", "login crashes on startup", "alice", []string{"bug"}},
		{corpus.ThreadKindIssue, 2, "login crash on startup", "the login page crashes", "alice", []string{"bug"}},
		{corpus.ThreadKindIssue, 3, "unrelated feature", "add dark mode", "bob", nil},
		{corpus.ThreadKindIssue, 4, "fix login crash", "duplicate of #1", "alice", []string{"bug"}},
		{corpus.ThreadKindIssue, 5, "api network timeout", "requests time out", "carol", []string{"bug"}},
		{corpus.ThreadKindIssue, 6, "timeout in api requests", "network timeout", "carol", []string{"bug"}},
	}

	base := time.Unix(1000, 0).UTC()
	candidates := make([]clustering.Candidate, 0, len(threads))
	for i, th := range threads {
		updated := base.Add(time.Duration(i) * time.Second)
		thread, err := c.UpsertThread(ctx, corpus.Thread{
			RepositoryID:    repo.ID,
			Kind:            th.kind,
			Number:          th.number,
			State:           "open",
			Title:           th.title,
			Body:            th.body,
			Author:          th.author,
			Labels:          th.labels,
			SourceCreatedAt: updated,
			SourceUpdatedAt: updated,
		}, `{}`)
		if err != nil {
			t.Fatalf("seed thread %d: %v", th.number, err)
		}
		candidates = append(candidates, clustering.Candidate{
			ThreadID:  thread.ID,
			Repo:      domain.RepoRef{Owner: "owner", Repo: "repo"},
			Kind:      th.kind,
			Number:    th.number,
			State:     "open",
			Title:     th.title,
			Body:      th.body,
			Author:    th.author,
			Labels:    th.labels,
			UpdatedAt: updated,
		})
	}
	return repo.ID, candidates
}

func TestNormalizeAndTokens(t *testing.T) {
	text := "Hello, World! 123"
	got := clustering.Tokens(text, true)
	want := []string{"123", "hello", "world"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("tokens mismatch (-want +got):\n%s", diff)
	}

	stop := clustering.Tokens("the quick brown fox", true)
	for _, w := range []string{"the"} {
		for _, tok := range stop {
			if tok == w {
				t.Fatalf("stop word %q found in tokens", w)
			}
		}
	}
}

func TestExtractRefs(t *testing.T) {
	repo := domain.RepoRef{Owner: "owner", Repo: "repo"}

	cases := []struct {
		input string
		want  int
	}{
		{"see #42 for context", 1},
		{"fix owner/repo#7", 1},
		{"https://github.com/owner/repo/issues/3", 1},
		{"nothing here", 0},
	}

	for _, tc := range cases {
		refs := clustering.ExtractRefs(tc.input, repo)
		if len(refs) != tc.want {
			t.Fatalf("ExtractRefs(%q) = %d refs, want %d", tc.input, len(refs), tc.want)
		}
	}

	// Bare #42 resolves to the default repository with an empty kind.
	refs := clustering.ExtractRefs("see #42", repo)
	if len(refs) != 1 || refs[0].Number != 42 {
		t.Fatalf("bare ref mismatch: %+v", refs)
	}
	if refs[0].Kind != "" {
		t.Fatalf("bare ref kind = %q, want empty", refs[0].Kind)
	}
}

func TestSignalsExplainable(t *testing.T) {
	a := clustering.Candidate{Title: "fix login crash", Body: "login crashes on startup", Author: "alice", Labels: []string{"bug"}}
	b := clustering.Candidate{Title: "fix login crash", Body: "login also crashes", Author: "alice", Labels: []string{"bug"}}

	cfg := clustering.DefaultConfig()
	cl := clustering.NewClusterer(cfg)
	clusters, err := cl.Cluster([]clustering.Candidate{a, b}, nil)
	if err != nil {
		t.Fatalf("cluster: %v", err)
	}
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(clusters))
	}
	for _, m := range clusters[0].Members {
		if m.Reason == "" {
			t.Fatalf("member %d has empty reason", m.Ref.Number)
		}
		if m.Score <= 0 || m.Score > 1 {
			t.Fatalf("member %d score out of range: %f", m.Ref.Number, m.Score)
		}
	}
}

func TestClustererExplicitReference(t *testing.T) {
	a := clustering.Candidate{Repo: domain.RepoRef{Owner: "o", Repo: "r"}, Kind: "issue", Number: 1, Title: "bug", Body: "first"}
	b := clustering.Candidate{Repo: domain.RepoRef{Owner: "o", Repo: "r"}, Kind: "issue", Number: 2, Title: "other", Body: "duplicate of #1"}
	c := clustering.Candidate{Repo: domain.RepoRef{Owner: "o", Repo: "r"}, Kind: "issue", Number: 3, Title: "unrelated", Body: "nothing"}

	cl := clustering.NewClusterer(clustering.DefaultConfig())
	clusters, err := cl.Cluster([]clustering.Candidate{a, b, c}, nil)
	if err != nil {
		t.Fatalf("cluster: %v", err)
	}
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(clusters))
	}
	if len(clusters[0].Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(clusters[0].Members))
	}
	if clusters[0].Canonical.Number != 1 {
		t.Fatalf("canonical = %v, want issue 1", clusters[0].Canonical)
	}
	for _, member := range clusters[0].Members {
		if member.Ref.Number == 2 {
			if member.Score < 0.4 || member.Reason != "explicit reference" {
				t.Fatalf("explicit-reference member score/reason = %.2f/%q", member.Score, member.Reason)
			}
			return
		}
	}
	t.Fatal("missing explicitly linked member")
}

func TestClustererNoFalsePositive(t *testing.T) {
	a := clustering.Candidate{Repo: domain.RepoRef{Owner: "o", Repo: "r"}, Kind: "issue", Number: 1, Title: "fix login crash", Body: "crash"}
	b := clustering.Candidate{Repo: domain.RepoRef{Owner: "o", Repo: "r"}, Kind: "issue", Number: 2, Title: "add dark mode", Body: "theme"}

	cl := clustering.NewClusterer(clustering.DefaultConfig())
	clusters, err := cl.Cluster([]clustering.Candidate{a, b}, nil)
	if err != nil {
		t.Fatalf("cluster: %v", err)
	}
	if len(clusters) != 0 {
		t.Fatalf("expected no clusters, got %d", len(clusters))
	}
}

func TestStableIDDeterministic(t *testing.T) {
	a := clustering.Candidate{Repo: domain.RepoRef{Owner: "o", Repo: "r"}, Kind: "issue", Number: 1, Title: "duplicate title"}
	b := clustering.Candidate{Repo: domain.RepoRef{Owner: "o", Repo: "r"}, Kind: "issue", Number: 2, Title: "duplicate title"}

	cl := clustering.NewClusterer(clustering.DefaultConfig())
	first, err := cl.Cluster([]clustering.Candidate{a, b}, nil)
	if err != nil {
		t.Fatalf("first cluster: %v", err)
	}
	second, err := cl.Cluster([]clustering.Candidate{b, a}, nil)
	if err != nil {
		t.Fatalf("second cluster: %v", err)
	}
	if len(first) != 1 || len(second) != 1 {
		t.Fatal("expected one cluster each")
	}
	if first[0].StableID != second[0].StableID {
		t.Fatalf("stable id drift: %s vs %s", first[0].StableID, second[0].StableID)
	}
}

func TestClustererRespectsHardLimits(t *testing.T) {
	cfg := clustering.Config{Threshold: 0.5, MaxCandidates: 2, MaxPairs: 10, MaxBodyTokens: 100}
	cl := clustering.NewClusterer(cfg)

	candidates := []clustering.Candidate{
		{Repo: domain.RepoRef{Owner: "o", Repo: "r"}, Kind: "issue", Number: 1, Title: "a"},
		{Repo: domain.RepoRef{Owner: "o", Repo: "r"}, Kind: "issue", Number: 2, Title: "a"},
		{Repo: domain.RepoRef{Owner: "o", Repo: "r"}, Kind: "issue", Number: 3, Title: "a"},
	}
	_, err := cl.Cluster(candidates, nil)
	if err == nil {
		t.Fatal("expected error for exceeding candidate limit")
	}

	cfg.MaxCandidates = 10
	cfg.MaxPairs = 1
	cl = clustering.NewClusterer(cfg)
	_, err = cl.Cluster(candidates, nil)
	if err == nil {
		t.Fatal("expected error for exceeding pair limit")
	}
}

func TestStoreComputeForRepo(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	seedRepoAndThreads(t, c)

	store := c.Clustering()
	run, clusters, err := store.ComputeForRepo(ctx, domain.RepoRef{Owner: "owner", Repo: "repo"}, clustering.DefaultConfig())
	if err != nil {
		t.Fatalf("compute for repo: %v", err)
	}
	if run.ID == 0 {
		t.Fatal("run id not set")
	}
	if run.SourceRevision == "" {
		t.Fatal("source revision empty")
	}
	if len(clusters) == 0 {
		t.Fatal("expected clusters")
	}

	// Issues 1, 2, and 4 should cluster around the duplicate login crash.
	var found bool
	for _, cl := range clusters {
		if cl.Canonical.Number == 1 {
			found = true
			nums := make(map[int]struct{})
			for _, m := range cl.Members {
				if m.Included {
					nums[m.Ref.Number] = struct{}{}
				}
			}
			for _, n := range []int{1, 2, 4} {
				if _, ok := nums[n]; !ok {
					t.Fatalf("missing member %d in canonical cluster", n)
				}
			}
		}
	}
	if !found {
		t.Fatalf("canonical cluster with issue 1 not found")
	}
}

func TestStoreRetiresClustersMissingFromRecompute(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	store := c.Clustering()
	repo := domain.RepoRef{Owner: "owner", Repo: "repo"}
	candidates := []clustering.Candidate{
		{Repo: repo, Kind: "issue", Number: 1, Title: "same crash"},
		{Repo: repo, Kind: "issue", Number: 2, Title: "same crash"},
	}
	_, clusters, err := store.Compute(ctx, repo, candidates, clustering.DefaultConfig())
	if err != nil || len(clusters) != 1 {
		t.Fatalf("first compute clusters=%d err=%v", len(clusters), err)
	}
	stableID := clusters[0].StableID
	memberRef := clusters[0].Members[0].Ref

	if _, current, err := store.Compute(ctx, repo, candidates[:1], clustering.DefaultConfig()); err != nil || len(current) != 0 {
		t.Fatalf("second compute clusters=%d err=%v", len(current), err)
	}
	retired, err := store.GetCluster(ctx, stableID)
	if err != nil {
		t.Fatal(err)
	}
	if retired == nil || retired.State != clustering.ClusterRetired || len(retired.Members) != 0 {
		t.Fatalf("retired cluster = %+v", retired)
	}
	forMember, err := store.GetClusterForMember(ctx, memberRef)
	if err != nil {
		t.Fatal(err)
	}
	if forMember != nil {
		t.Fatalf("retired cluster still resolved for member: %+v", forMember)
	}

	_, reappeared, err := store.Compute(ctx, repo, candidates, clustering.DefaultConfig())
	if err != nil || len(reappeared) != 1 || reappeared[0].State != clustering.ClusterOpen {
		t.Fatalf("reappeared cluster = %+v, err=%v", reappeared, err)
	}
}

func TestStoreOverridesPersistAcrossRecompute(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	seedRepoAndThreads(t, c)

	store := c.Clustering()
	_, clusters, err := store.ComputeForRepo(ctx, domain.RepoRef{Owner: "owner", Repo: "repo"}, clustering.DefaultConfig())
	if err != nil {
		t.Fatalf("first compute: %v", err)
	}
	if len(clusters) == 0 {
		t.Fatal("expected clusters")
	}

	// Find the cluster containing issue #2 and exclude it.
	var cluster *clustering.Cluster
	for i := range clusters {
		for _, m := range clusters[i].Members {
			if m.Ref.Number == 2 {
				cluster = &clusters[i]
				break
			}
		}
		if cluster != nil {
			break
		}
	}
	if cluster == nil {
		t.Fatal("cluster containing issue 2 not found")
	}

	if err := store.AddOverride(ctx, cluster.ID, clustering.MemberRef{Owner: "owner", Repo: "repo", Kind: "issue", Number: 2}, clustering.OverrideExclude, "not a duplicate"); err != nil {
		t.Fatalf("add override: %v", err)
	}

	// Recompute should preserve the override and the cluster id/stable_id.
	_, recomputed, err := store.ComputeForRepo(ctx, domain.RepoRef{Owner: "owner", Repo: "repo"}, clustering.DefaultConfig())
	if err != nil {
		t.Fatalf("recompute: %v", err)
	}
	if len(recomputed) == 0 {
		t.Fatal("expected clusters after recompute")
	}
	found := false
	for _, rc := range recomputed {
		if rc.StableID == cluster.StableID {
			found = true
			if rc.ID != cluster.ID {
				t.Fatalf("cluster id changed: %d vs %d", rc.ID, cluster.ID)
			}
			hasExcluded := false
			for _, m := range rc.Members {
				if m.Ref.Number == 2 {
					if m.Included {
						t.Fatal("excluded member is included")
					}
					hasExcluded = true
				}
			}
			if !hasExcluded {
				t.Fatal("excluded member missing from cluster")
			}
		}
	}
	if !found {
		t.Fatal("stable cluster not preserved across recompute")
	}
}

func TestStoreMergeAndSplit(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	seedRepoAndThreads(t, c)

	store := c.Clustering()
	_, clusters, err := store.ComputeForRepo(ctx, domain.RepoRef{Owner: "owner", Repo: "repo"}, clustering.DefaultConfig())
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	if len(clusters) < 2 {
		t.Fatalf("need at least 2 clusters for merge/split, got %d", len(clusters))
	}

	// Identify the two clusters deterministically.
	var login, timeout *clustering.Cluster
	for i := range clusters {
		has := func(n int) bool {
			for _, m := range clusters[i].Members {
				if m.Ref.Number == n {
					return true
				}
			}
			return false
		}
		if has(1) {
			login = &clusters[i]
		} else if has(5) {
			timeout = &clusters[i]
		}
	}
	if login == nil || timeout == nil {
		t.Fatalf("expected login and timeout clusters, got login=%v timeout=%v", login, timeout)
	}

	// Merge timeout into login.
	if err := store.MergeClusters(ctx, timeout.ID, login.ID, "same root cause"); err != nil {
		t.Fatalf("merge: %v", err)
	}

	merged, err := store.GetCluster(ctx, login.StableID)
	if err != nil {
		t.Fatalf("get merged cluster: %v", err)
	}
	if merged == nil {
		t.Fatal("merged cluster not found")
	}
	afterMerge, err := store.GetCluster(ctx, timeout.StableID)
	if err != nil {
		t.Fatalf("get source cluster: %v", err)
	}
	if afterMerge == nil || afterMerge.State != clustering.ClusterClosed {
		t.Fatalf("source cluster not closed after merge: %+v", afterMerge)
	}

	// Split a non-canonical member from the merged cluster into its own cluster.
	splitRef := clustering.MemberRef{Owner: "owner", Repo: "repo", Kind: "issue", Number: 2}
	if err := store.SplitCluster(ctx, merged.ID, splitRef, "needs separate tracking"); err != nil {
		t.Fatalf("split: %v", err)
	}
	split, err := store.GetCluster(ctx, clustering.StableID(splitRef))
	if err != nil {
		t.Fatalf("get split cluster: %v", err)
	}
	if split == nil {
		t.Fatal("split cluster not found")
	}
	if split.Canonical != splitRef {
		t.Fatalf("split canonical mismatch: %v vs %v", split.Canonical, splitRef)
	}

	if _, _, err := store.ComputeForRepo(ctx, domain.RepoRef{Owner: "owner", Repo: "repo"}, clustering.DefaultConfig()); err != nil {
		t.Fatalf("recompute after governance operations: %v", err)
	}
	preservedSplit, err := store.GetCluster(ctx, clustering.StableID(splitRef))
	if err != nil {
		t.Fatal(err)
	}
	if preservedSplit == nil || preservedSplit.State != clustering.ClusterOpen || len(preservedSplit.Members) != 1 || !preservedSplit.Members[0].Included {
		t.Fatalf("split did not survive recompute: %+v", preservedSplit)
	}
	preservedTarget, err := store.GetCluster(ctx, login.StableID)
	if err != nil {
		t.Fatal(err)
	}
	for _, number := range []int{5, 6} {
		found := false
		for _, member := range preservedTarget.Members {
			if member.Ref.Number == number && member.Included {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("merged member %d did not survive recompute: %+v", number, preservedTarget.Members)
		}
	}
}

func TestCorpusReopenPreservesClusters(t *testing.T) {
	ctx := context.Background()
	c, path := openTestCorpus(t)
	seedRepoAndThreads(t, c)

	store := c.Clustering()
	_, clusters, err := store.ComputeForRepo(ctx, domain.RepoRef{Owner: "owner", Repo: "repo"}, clustering.DefaultConfig())
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	if len(clusters) == 0 {
		t.Fatal("expected clusters")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("close corpus: %v", err)
	}

	c2, err := corpus.Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen corpus: %v", err)
	}
	defer func() { _ = c2.Close() }()

	restored, err := c2.Clustering().ListClusters(ctx, domain.RepoRef{Owner: "owner", Repo: "repo"}, clustering.ClusterOpen, 100)
	if err != nil {
		t.Fatalf("list clusters after reopen: %v", err)
	}
	if len(restored) != len(clusters) {
		t.Fatalf("cluster count after reopen: %d vs %d", len(restored), len(clusters))
	}
}
