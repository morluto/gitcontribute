package app

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/clustering"
	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
)

func newNeighborService(t *testing.T) *Service {
	t.Helper()
	ctx := context.Background()
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if _, err := svc.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}
	return svc
}

func seedRepoForNeighbors(t *testing.T, c *corpus.Corpus) *corpus.Repository {
	t.Helper()
	ctx := context.Background()
	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "123", time.Unix(1, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("seed repository: %v", err)
	}
	return repo
}

func seedPullRequestForNeighbors(t *testing.T, c *corpus.Corpus, repoID int64, number int, title, body, author, state, baseRef string) *corpus.Thread {
	t.Helper()
	ctx := context.Background()
	updated := time.Unix(int64(number)*1000, 0).UTC()
	payload := fmt.Sprintf(`{"BaseRef":"%s","HeadRef":"feature-%d","Title":"%s","Body":"%s","Author":"%s"}`, baseRef, number, title, body, author)
	thread, err := c.UpsertThread(ctx, corpus.Thread{
		RepositoryID:    repoID,
		Kind:            corpus.ThreadKindPullRequest,
		Number:          number,
		State:           state,
		Title:           title,
		Body:            body,
		Author:          author,
		SourceCreatedAt: updated,
		SourceUpdatedAt: updated,
	}, payload)
	if err != nil {
		t.Fatalf("seed pull request %d: %v", number, err)
	}
	return thread
}

func seedIssueForNeighbors(t *testing.T, c *corpus.Corpus, repoID int64, number int, title, body, author string, labels []string) *corpus.Thread {
	t.Helper()
	ctx := context.Background()
	updated := time.Unix(int64(number)*1000, 0).UTC()
	thread, err := c.UpsertThread(ctx, corpus.Thread{
		RepositoryID:    repoID,
		Kind:            corpus.ThreadKindIssue,
		Number:          number,
		State:           "open",
		Title:           title,
		Body:            body,
		Author:          author,
		Labels:          labels,
		SourceCreatedAt: updated,
		SourceUpdatedAt: updated,
	}, `{}`)
	if err != nil {
		t.Fatalf("seed issue %d: %v", number, err)
	}
	return thread
}

func TestNeighborsRankedWithScoreAndReason(t *testing.T) {
	ctx := context.Background()
	svc := newNeighborService(t)
	defer func() { _ = svc.Close() }()

	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	repo := seedRepoForNeighbors(t, c)
	seedIssueForNeighbors(t, c, repo.ID, 1, "fix login crash", "login crashes on startup", "alice", []string{"bug"})
	seedIssueForNeighbors(t, c, repo.ID, 2, "login crash on startup", "the login page crashes", "alice", []string{"bug"})
	seedIssueForNeighbors(t, c, repo.ID, 3, "add dark mode", "theme support", "bob", []string{"enhancement"})

	res, err := svc.Neighbors(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, "issue", 1, 10)
	if err != nil {
		t.Fatalf("neighbors: %v", err)
	}
	if res.Total != 2 {
		t.Fatalf("total = %d, want 2", res.Total)
	}
	if res.SourceRevision == "" {
		t.Fatal("source revision is empty")
	}
	if res.Limit != 10 {
		t.Fatalf("limit = %d, want 10", res.Limit)
	}

	wantNumbers := []int{2, 3}
	for i, wn := range wantNumbers {
		if res.Neighbors[i].Number != wn {
			t.Fatalf("neighbor[%d].Number = %d, want %d", i, res.Neighbors[i].Number, wn)
		}
		if res.Neighbors[i].Score < 0 || res.Neighbors[i].Score > 1 {
			t.Fatalf("neighbor[%d].Score = %f, want [0,1]", i, res.Neighbors[i].Score)
		}
		if res.Neighbors[i].Reason == "" {
			t.Fatalf("neighbor[%d].Reason is empty", i)
		}
		if i > 0 && res.Neighbors[i-1].Score < res.Neighbors[i].Score {
			t.Fatalf("neighbor[%d].Score %f should be >= neighbor[%d].Score %f", i-1, res.Neighbors[i-1].Score, i, res.Neighbors[i].Score)
		}
	}

	limited, err := svc.Neighbors(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, "issue", 1, 1)
	if err != nil {
		t.Fatalf("limited neighbors: %v", err)
	}
	if limited.Total != 1 || limited.Neighbors[0].Number != 2 {
		t.Fatalf("limited = %+v", limited)
	}
}

func TestNeighborsStableTieOrdering(t *testing.T) {
	ctx := context.Background()
	svc := newNeighborService(t)
	defer func() { _ = svc.Close() }()

	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	repo := seedRepoForNeighbors(t, c)
	seedIssueForNeighbors(t, c, repo.ID, 1, "identical title", "identical body", "alice", []string{"bug"})
	for _, n := range []int{2, 3, 4} {
		seedIssueForNeighbors(t, c, repo.ID, n, "identical title", "identical body", "alice", []string{"bug"})
	}

	res, err := svc.Neighbors(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, "issue", 1, 2)
	if err != nil {
		t.Fatalf("neighbors: %v", err)
	}
	if len(res.Neighbors) != 2 {
		t.Fatalf("got %d neighbors, want 2", len(res.Neighbors))
	}
	if res.Neighbors[0].Number != 2 || res.Neighbors[1].Number != 3 {
		t.Fatalf("stable ordering failed: %+v", res.Neighbors)
	}
	if res.Neighbors[0].Score != res.Neighbors[1].Score {
		t.Fatalf("expected tied scores, got %f and %f", res.Neighbors[0].Score, res.Neighbors[1].Score)
	}
}

func TestNeighborsExcludesQueryAndMissingThread(t *testing.T) {
	ctx := context.Background()
	svc := newNeighborService(t)
	defer func() { _ = svc.Close() }()

	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	repo := seedRepoForNeighbors(t, c)
	seedIssueForNeighbors(t, c, repo.ID, 1, "query", "body", "alice", nil)
	seedIssueForNeighbors(t, c, repo.ID, 2, "other", "body", "bob", nil)

	res, err := svc.Neighbors(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, "issue", 1, 10)
	if err != nil {
		t.Fatalf("neighbors: %v", err)
	}
	for _, n := range res.Neighbors {
		if n.Number == 1 {
			t.Fatal("neighbor result includes the query")
		}
	}

	if _, err := svc.Neighbors(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, "issue", 99, 10); err == nil {
		t.Fatal("expected error for missing thread")
	}
	if _, err := svc.Neighbors(ctx, cli.RepoRef{Owner: "unknown", Repo: "repo"}, "issue", 1, 10); err == nil {
		t.Fatal("expected error for missing repository")
	}
}

func TestDuplicateCandidatesReturnsClusterMembers(t *testing.T) {
	ctx := context.Background()
	svc := newNeighborService(t)
	defer func() { _ = svc.Close() }()

	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	repo := seedRepoForNeighbors(t, c)
	seedIssueForNeighbors(t, c, repo.ID, 1, "fix login crash", "login crashes on startup", "alice", []string{"bug"})
	seedIssueForNeighbors(t, c, repo.ID, 2, "fix login crash", "the login page crashes", "alice", []string{"bug"})

	store := c.Clustering()
	if _, _, err := store.ComputeForRepo(ctx, domain.RepoRef{Owner: "owner", Repo: "repo"}, clustering.DefaultConfig()); err != nil {
		t.Fatalf("compute clusters: %v", err)
	}

	res, err := svc.DuplicateCandidates(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, "issue", 1, 10)
	if err != nil {
		t.Fatalf("duplicate candidates: %v", err)
	}
	if res.Total != 1 {
		t.Fatalf("total = %d, want 1", res.Total)
	}
	if res.StableID == "" {
		t.Fatal("stable id is empty")
	}
	if res.SourceRevision == "" {
		t.Fatal("source revision is empty")
	}
	if res.Canonical.Number != 1 {
		t.Fatalf("canonical = %+v, want issue 1", res.Canonical)
	}
	if res.Candidates[0].Number != 2 {
		t.Fatalf("candidate = %+v", res.Candidates[0])
	}
	if res.Candidates[0].Score <= 0 || res.Candidates[0].Score > 1 {
		t.Fatalf("candidate score out of range: %f", res.Candidates[0].Score)
	}
	if res.Candidates[0].Reason == "" {
		t.Fatal("candidate reason is empty")
	}
}

func TestDuplicateCandidatesEmptyWhenNoCluster(t *testing.T) {
	ctx := context.Background()
	svc := newNeighborService(t)
	defer func() { _ = svc.Close() }()

	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	repo := seedRepoForNeighbors(t, c)
	seedIssueForNeighbors(t, c, repo.ID, 1, "unrelated one", "body", "alice", nil)
	seedIssueForNeighbors(t, c, repo.ID, 2, "unrelated two", "body", "bob", nil)

	store := c.Clustering()
	if _, _, err := store.ComputeForRepo(ctx, domain.RepoRef{Owner: "owner", Repo: "repo"}, clustering.DefaultConfig()); err != nil {
		t.Fatalf("compute clusters: %v", err)
	}

	res, err := svc.DuplicateCandidates(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, "issue", 1, 10)
	if err != nil {
		t.Fatalf("duplicate candidates: %v", err)
	}
	if res.Total != 0 || res.StableID != "" || len(res.Candidates) != 0 {
		t.Fatalf("expected empty duplicate result, got %+v", res)
	}
}

func TestNeighborQueriesRejectInvalidInput(t *testing.T) {
	ctx := context.Background()
	svc := newNeighborService(t)
	defer func() { _ = svc.Close() }()

	cases := []struct {
		repo   cli.RepoRef
		kind   string
		number int
	}{
		{repo: cli.RepoRef{Owner: "", Repo: "repo"}, kind: "issue", number: 1},
		{repo: cli.RepoRef{Owner: "owner", Repo: "repo"}, kind: "unknown", number: 1},
		{repo: cli.RepoRef{Owner: "owner", Repo: "repo"}, kind: "issue", number: 0},
	}
	for _, tc := range cases {
		if _, err := svc.Neighbors(ctx, tc.repo, tc.kind, tc.number, 10); err == nil {
			t.Fatalf("expected error for %+v", tc)
		}
	}
}

// Ensure the neighbor source revision changes when the candidate population
// changes, but is stable for identical input.
func TestNeighborsSourceRevisionReflectsPopulation(t *testing.T) {
	ctx := context.Background()
	svc := newNeighborService(t)
	defer func() { _ = svc.Close() }()

	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	repo := seedRepoForNeighbors(t, c)
	seedIssueForNeighbors(t, c, repo.ID, 1, "query", "body", "alice", nil)
	seedIssueForNeighbors(t, c, repo.ID, 2, "neighbor", "body", "bob", nil)

	first, err := svc.Neighbors(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, "issue", 1, 10)
	if err != nil {
		t.Fatalf("first neighbors: %v", err)
	}

	seedIssueForNeighbors(t, c, repo.ID, 3, "another", "body", "carol", nil)
	second, err := svc.Neighbors(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, "issue", 1, 10)
	if err != nil {
		t.Fatalf("second neighbors: %v", err)
	}
	if first.SourceRevision == second.SourceRevision {
		t.Fatal("source revision did not change when population changed")
	}

	third, err := svc.Neighbors(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, "issue", 1, 10)
	if err != nil {
		t.Fatalf("third neighbors: %v", err)
	}
	if second.SourceRevision != third.SourceRevision {
		t.Fatalf("source revision not stable for identical population: %s vs %s", second.SourceRevision, third.SourceRevision)
	}
}

func TestNeighborResultJSONShape(t *testing.T) {
	ctx := context.Background()
	svc := newNeighborService(t)
	defer func() { _ = svc.Close() }()

	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	repo := seedRepoForNeighbors(t, c)
	seedIssueForNeighbors(t, c, repo.ID, 1, "fix login crash", "body", "alice", []string{"bug"})
	seedIssueForNeighbors(t, c, repo.ID, 2, "login crash on startup", "body", "alice", []string{"bug"})

	res, err := svc.Neighbors(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, "issue", 1, 10)
	if err != nil {
		t.Fatalf("neighbors: %v", err)
	}
	if diff := cmp.Diff("owner/repo", res.Repo); diff != "" {
		t.Fatalf("repo mismatch: %s", diff)
	}
	if res.Kind != "issue" || res.Number != 1 {
		t.Fatalf("unexpected query fields: %+v", res)
	}
	for _, n := range res.Neighbors {
		if n.Owner == "" || n.Repo == "" || n.Kind == "" {
			t.Fatalf("incomplete neighbor: %+v", n)
		}
	}
}

func TestPullRequestCollisions(t *testing.T) {
	ctx := context.Background()
	svc := newNeighborService(t)
	defer func() { _ = svc.Close() }()

	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	repo := seedRepoForNeighbors(t, c)
	seedPullRequestForNeighbors(t, c, repo.ID, 1, "fix parser", "parser fix", "alice", "open", "main")
	seedPullRequestForNeighbors(t, c, repo.ID, 2, "another parser fix", "depends on #1", "bob", "open", "main")
	seedPullRequestForNeighbors(t, c, repo.ID, 3, "add theme", "theme support", "alice", "open", "main")
	seedPullRequestForNeighbors(t, c, repo.ID, 4, "old fix", "body", "dave", "closed", "main")
	seedPullRequestForNeighbors(t, c, repo.ID, 5, "other branch", "body", "eve", "open", "dev")

	res, err := svc.PullRequestCollisions(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, 1, 10)
	if err != nil {
		t.Fatalf("collisions: %v", err)
	}
	if res.SourceRevision == "" {
		t.Fatal("source revision is empty")
	}
	if len(res.Collisions) != 2 {
		t.Fatalf("collisions = %d, want 2: %+v", len(res.Collisions), res.Collisions)
	}

	want := []struct {
		number int
		score  float64
	}{
		{2, 0.75},
		{3, 0.45},
	}
	for i, w := range want {
		if res.Collisions[i].Number != w.number {
			t.Fatalf("collision[%d].Number = %d, want %d", i, res.Collisions[i].Number, w.number)
		}
		if math.Abs(res.Collisions[i].Score-w.score) > 1e-9 {
			t.Fatalf("collision[%d].Score = %f, want %f", i, res.Collisions[i].Score, w.score)
		}
		if res.Collisions[i].Reason == "" {
			t.Fatalf("collision[%d].Reason is empty", i)
		}
		if i > 0 && res.Collisions[i-1].Score < res.Collisions[i].Score {
			t.Fatalf("collision ordering wrong: %v", res.Collisions)
		}
	}

	limited, err := svc.PullRequestCollisions(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, 1, 1)
	if err != nil {
		t.Fatalf("limited collisions: %v", err)
	}
	if len(limited.Collisions) != 1 || limited.Collisions[0].Number != 2 {
		t.Fatalf("limited collisions = %+v", limited.Collisions)
	}

	closed, err := svc.PullRequestCollisions(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, 4, 10)
	if err != nil {
		t.Fatalf("closed query collisions: %v", err)
	}
	if len(closed.Collisions) == 0 {
		t.Fatalf("expected at least one collision for closed query PR with same base open PRs")
	}
}
