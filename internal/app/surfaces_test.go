package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/clustering"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/lens"
	"github.com/morluto/gitcontribute/internal/mcpserver"
)

func seedRepoAndThreads(t *testing.T, c *corpus.Corpus) {
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
	for i, th := range threads {
		updated := base.Add(time.Duration(i) * time.Second)
		if _, err := c.UpsertThread(ctx, corpus.Thread{
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
		}, `{}`); err != nil {
			t.Fatalf("seed thread %d: %v", th.number, err)
		}
	}
}

func TestServiceClustersAndCluster(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := newTestServer("owner", "repo")
	defer srv.Close()

	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()

	seedRepoAndThreads(t, svc.corpus)

	repo := cli.RepoRef{Owner: "owner", Repo: "repo"}
	before, err := svc.ListClusters(ctx, repo, 10)
	if err != nil {
		t.Fatalf("list clusters before refresh: %v", err)
	}
	if before.Total != 0 {
		t.Fatalf("unrefreshed list = %+v, want empty", before)
	}
	firstRefresh, err := svc.RefreshClusters(ctx, repo)
	if err != nil {
		t.Fatalf("refresh clusters: %v", err)
	}
	if firstRefresh.Disposition != "committed" || firstRefresh.Stats.ComparedPairs == 0 {
		t.Fatalf("first refresh = %+v", firstRefresh)
	}
	secondRefresh, err := svc.RefreshClusters(ctx, repo)
	if err != nil {
		t.Fatalf("unchanged refresh: %v", err)
	}
	if secondRefresh.Disposition != "unchanged" || secondRefresh.Stats.ComparedPairs != 0 || secondRefresh.Stats.RequiredPairs != 0 || secondRefresh.Stats.CommitQueries != 0 {
		t.Fatalf("second refresh = %+v, want unchanged with no pair or commit work", secondRefresh)
	}
	if secondRefresh.Stats.CandidateCount != firstRefresh.Stats.CandidateCount || secondRefresh.Stats.ClusterCount != firstRefresh.Stats.ClusterCount || secondRefresh.Stats.ClusterCount == 0 {
		t.Fatalf("second refresh = %+v, want current projection counts from %+v", secondRefresh, firstRefresh)
	}
	list, err := svc.ListClusters(ctx, repo, 10)
	if err != nil {
		t.Fatalf("clusters: %v", err)
	}
	if list.Total == 0 {
		t.Fatal("expected clusters")
	}

	var found bool
	for _, cl := range list.Clusters {
		if cl.Canonical.Number == 1 {
			found = true
			if cl.MemberCount < 2 {
				t.Fatalf("expected at least 2 members in login cluster, got %d", cl.MemberCount)
			}
			if len(cl.Members) != 0 {
				t.Fatalf("list result should not include member details, got %d", len(cl.Members))
			}
		}
	}
	if !found {
		t.Fatalf("expected canonical cluster with issue 1, got %+v", list.Clusters)
	}

	stableID := list.Clusters[0].StableID
	detail, err := svc.Cluster(ctx, stableID, 100)
	if err != nil {
		t.Fatalf("cluster show: %v", err)
	}
	if detail.StableID != stableID {
		t.Fatalf("stable id mismatch: %s vs %s", detail.StableID, stableID)
	}
	if len(detail.Members) == 0 {
		t.Fatal("expected cluster members in detail view")
	}
	stored, err := svc.corpus.GetClusterProjection(ctx, stableID)
	if err != nil {
		t.Fatal(err)
	}
	var governed clustering.MemberRef
	for _, member := range stored.Members {
		if member.Ref != stored.Canonical {
			governed = member.Ref
			break
		}
	}
	if governed.Number == 0 {
		t.Fatal("expected a non-canonical member for governance test")
	}
	if err := svc.corpus.AddClusterOverride(ctx, stored.ID, stored.Canonical, clustering.OverrideExclude, "remove canonical"); err == nil || err.Error() != "cannot exclude the canonical member" {
		t.Fatalf("canonical exclusion error = %v", err)
	}
	if err := svc.corpus.AddClusterOverride(ctx, stored.ID, governed, clustering.OverrideExclude, "not the same root cause"); err != nil {
		t.Fatal(err)
	}
	governedRefresh, err := svc.RefreshClusters(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if governedRefresh.Disposition != "committed" || governedRefresh.Projection.GovernanceRevision != 1 {
		t.Fatalf("governed refresh = %+v", governedRefresh)
	}

	if _, err := svc.Cluster(ctx, "nosuchcluster", 100); err == nil {
		t.Fatal("expected error for missing cluster")
	}
}

func TestUnchangedEmptyClusterProjectionReportsZeroCurrentCounts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := newTestServer("owner", "empty")
	defer srv.Close()
	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()
	if _, err := svc.corpus.ApplyRepositoryObservation(ctx, "owner", "empty", "123", time.Unix(1, 0).UTC(), `{}`); err != nil {
		t.Fatal(err)
	}

	repo := cli.RepoRef{Owner: "owner", Repo: "empty"}
	first, err := svc.RefreshClusters(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.RefreshClusters(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if first.Disposition != "committed" || second.Disposition != "unchanged" || first.Projection.RunID != second.Projection.RunID {
		t.Fatalf("empty refreshes: first=%+v second=%+v", first, second)
	}
	if second.Stats.CandidateCount != 0 || second.Stats.ClusterCount != 0 || second.Stats.RequiredPairs != 0 || second.Stats.ComparedPairs != 0 || second.Stats.CommitQueries != 0 || second.Stats.SnapshotQueries == 0 {
		t.Fatalf("unchanged empty stats = %+v", second.Stats)
	}
}

func TestCurrentProjectionClusterCountExcludesRetiredHistory(t *testing.T) {
	t.Parallel()
	clusters := []clustering.Cluster{{State: clustering.ClusterOpen}, {State: clustering.ClusterClosed}, {State: clustering.ClusterRetired}}
	if got := currentProjectionClusterCount(clusters); got != 2 {
		t.Fatalf("current cluster count = %d, want 2", got)
	}
}

func TestArchiveThreadsIsBoundedAndOffline(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := newTestServer("owner", "repo")
	defer srv.Close()
	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()
	seedRepoAndThreads(t, svc.corpus)

	result, err := svc.ArchiveThreads(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, "issue", "open", 2)
	if err != nil {
		t.Fatalf("archive threads: %v", err)
	}
	if len(result.Threads) != 2 || result.Threads[0].Number != 6 || result.Freshness == "" {
		t.Fatalf("result = %+v", result)
	}
	if _, err := svc.ArchiveThreads(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, "", "", 1001); err == nil {
		t.Fatal("expected hard-limit error")
	}
}

func TestServiceLensAndCollections(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := newTestServer("owner", "repo")
	defer srv.Close()

	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()

	def := lens.Definition{
		Name: "active-go",
		Filter: lens.Filter{
			Kinds:           []string{"issue"},
			States:          []string{"open"},
			Languages:       []string{"Go"},
			ExcludeArchived: true,
			Unassigned:      true,
			UpdatedWithin:   30 * 24 * time.Hour,
			MinStars:        20,
		},
		Weights:           map[string]float64{"relevance": 1},
		MaxResultsPerRepo: 3,
	}

	added, err := svc.AddLens(ctx, "active-go", def)
	if err != nil {
		t.Fatalf("add lens: %v", err)
	}
	if added.Name != "active-go" {
		t.Fatalf("lens name = %q", added.Name)
	}

	show, err := svc.ShowLens(ctx, "active-go")
	if err != nil {
		t.Fatalf("show lens: %v", err)
	}
	if show.Name != "active-go" || show.Definition.Filter.MinStars != 20 {
		t.Fatalf("unexpected lens: %+v", show)
	}

	list, err := svc.ListLenses(ctx)
	if err != nil {
		t.Fatalf("list lenses: %v", err)
	}
	if len(list.Lenses) != 1 || list.Lenses[0].Name != "active-go" {
		t.Fatalf("unexpected lenses: %+v", list)
	}

	col, err := svc.CreateCollection(ctx, "favorites")
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	if col.Name != "favorites" {
		t.Fatalf("collection name = %q", col.Name)
	}

	updated, err := svc.AddCollectionMembers(ctx, "favorites", []cli.CollectionMember{
		{Kind: "repository", Ref: "owner/repo"},
		{Kind: "issue", Ref: "owner/repo#1"},
	})
	if err != nil {
		t.Fatalf("add collection members: %v", err)
	}
	if updated.MemberCount != 2 {
		t.Fatalf("member count = %d", updated.MemberCount)
	}

	cols, err := svc.ListCollections(ctx)
	if err != nil {
		t.Fatalf("list collections: %v", err)
	}
	if len(cols.Collections) != 1 || cols.Collections[0].MemberCount != 2 {
		t.Fatalf("unexpected collections: %+v", cols)
	}
}

func TestCollectionServiceRejectsMalformedMembers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()
	if _, err := svc.CreateCollection(ctx, "favorites"); err != nil {
		t.Fatal(err)
	}

	invalid := []cli.CollectionMember{
		{Kind: "unknown", Ref: "o/r"},
		{Kind: "repository", Ref: "not-a-repo"},
		{Kind: "issue", Ref: "o/r#0"},
		{Kind: "pull_request", Ref: "o/r#abc"},
	}
	for _, member := range invalid {
		if _, err := svc.AddCollectionMembers(ctx, "favorites", []cli.CollectionMember{member}); err == nil {
			t.Fatalf("accepted malformed member %+v", member)
		}
	}
}

func TestArchiveSyncRejectsNegativeSince(t *testing.T) {
	t.Parallel()
	svc := newTestServiceNoNetwork(t)
	defer func() { _ = svc.Close() }()
	_, err := svc.ArchiveSync(context.Background(), cli.RepoRef{Owner: "o", Repo: "r"}, cli.ArchiveSyncOptions{Since: -time.Hour})
	if err == nil || err.Error() != "since duration cannot be negative" {
		t.Fatalf("expected negative duration error, got %v", err)
	}
}

func TestMCPReaderFindClustersAndCoverage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := newTestServer("owner", "repo")
	defer srv.Close()

	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()

	seedRepoAndThreads(t, svc.corpus)

	// Clusters must be computed before the MCP read tool can list them.
	if _, err := svc.RefreshClusters(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}); err != nil {
		t.Fatalf("compute clusters: %v", err)
	}

	reader := svc.MCPReader()
	clusters, err := reader.FindClusters(ctx, mcpserver.FindClustersInput{Owner: "owner", Repo: "repo", Limit: 10})
	if err != nil {
		t.Fatalf("find clusters: %v", err)
	}
	if clusters.Total == 0 {
		t.Fatal("expected clusters from MCP")
	}
	if clusters.RuleVersion != "duplicate-v1" {
		t.Fatalf("cluster rule version = %q", clusters.RuleVersion)
	}

	cov, err := reader.GetCoverage(ctx, mcpserver.GetCoverageInput{Targets: []mcpserver.CoverageTarget{{Owner: "owner", Repo: "repo"}}})
	if err != nil {
		t.Fatalf("get coverage: %v", err)
	}
	if len(cov.Items) != 1 || cov.Items[0].Value == nil || cov.Items[0].Value.Owner != "owner" || cov.Items[0].Value.Repo != "repo" {
		t.Fatalf("unexpected coverage: %+v", cov)
	}
}

func TestServiceReadsLensFromJSONFile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := newTestServer("owner", "repo")
	defer srv.Close()

	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()

	path := filepath.Join(t.TempDir(), "lens.json")
	data := []byte(`{
		"filter": {
			"kinds": ["issue"],
			"updated_within": "720h",
			"min_stars": 10
		},
		"weights": {"relevance": 1}
	}`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write lens file: %v", err)
	}

	fileData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read lens file: %v", err)
	}

	var def lens.Definition
	if err := json.Unmarshal(fileData, &def); err != nil {
		t.Fatalf("parse lens file: %v", err)
	}

	added, err := svc.AddLens(ctx, "from-file", def)
	if err != nil {
		t.Fatalf("add lens from file: %v", err)
	}
	if added.Definition.Filter.UpdatedWithin != 720*time.Hour {
		t.Fatalf("updated_within not parsed: %v", added.Definition.Filter.UpdatedWithin)
	}
	if added.Definition.Filter.MinStars != 10 {
		t.Fatalf("min_stars = %d", added.Definition.Filter.MinStars)
	}
}

func TestMCPReaderLensResource(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := newTestServer("owner", "repo")
	defer srv.Close()

	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()

	def := lens.Definition{
		Name:    "mcp-lens",
		Filter:  lens.Filter{Kinds: []string{"issue"}},
		Weights: map[string]float64{"freshness": 0.5},
	}
	if _, err := svc.AddLens(ctx, "mcp-lens", def); err != nil {
		t.Fatalf("add lens: %v", err)
	}

	reader := svc.MCPReader()
	out, err := reader.Lens(ctx, mcpserver.LensInput{Name: "mcp-lens"})
	if err != nil {
		t.Fatalf("get lens: %v", err)
	}
	if out.Name != "mcp-lens" {
		t.Fatalf("unexpected lens name: %q", out.Name)
	}

	if _, err := reader.Lens(ctx, mcpserver.LensInput{Name: "missing"}); err == nil {
		t.Fatal("expected error for missing lens")
	}
}
