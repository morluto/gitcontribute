package cli_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/lens"
)

type fakeSurfacesService struct {
	*fakeService

	clustersCalled    bool
	refreshCalled     bool
	clusterCalled     bool
	addLensCalled     bool
	listLensCalled    bool
	showLensCalled    bool
	explainLensCalled bool
	createColCalled   bool
	addColCalled      bool
	listColCalled     bool
	archiveCalled     bool
	planCalled        bool
	hydrateCalled     bool
	threadsCalled     bool
	coverageCalled    bool
	runsCalled        bool
	neighborsCalled   bool
	exportCalled      bool

	lastClustersArg     contracts.RepoRef
	lastClusterID       string
	lastClusterLimit    int
	lastLensName        string
	lastLensExplainRef  string
	lastLensExplainOpts contracts.LensExplainOptions
	lastLensDef         lens.Definition
	syncEvents          []string
	lastCreateColName   string
	lastAddColName      string
	lastAddColMembers   []contracts.CollectionMember
	lastArchiveOpts     contracts.ArchiveSyncOptions
	lastHydrateOpts     contracts.HydrateOptions
}

func (f *fakeSurfacesService) ListClusters(_ context.Context, repo contracts.RepoRef, _ int) (*contracts.ClusterListResult, error) {
	f.clustersCalled = true
	f.lastClustersArg = repo
	return &contracts.ClusterListResult{
		Repo:      repo,
		Total:     2,
		Truncated: true,
		Clusters: []contracts.ClusterResult{
			{
				StableID:    "abc12345",
				State:       "open",
				Canonical:   contracts.ClusterMember{Kind: "issue", Owner: repo.Owner, Repo: repo.Repo, Number: 1},
				MemberCount: 3,
			},
		},
	}, f.err
}

func (f *fakeSurfacesService) RefreshClusters(_ context.Context, repo contracts.RepoRef) (*contracts.ClusterRefreshResult, error) {
	f.refreshCalled = true
	f.lastClustersArg = repo
	return &contracts.ClusterRefreshResult{Repo: repo, Disposition: "committed", Projection: contracts.ClusterProjectionIdentity{RuleVersion: "duplicate-v1"}}, f.err
}

func (f *fakeSurfacesService) Cluster(ctx context.Context, id string, limit int) (*contracts.ClusterResult, error) {
	f.clusterCalled = true
	f.lastClusterID = id
	f.lastClusterLimit = limit
	return &contracts.ClusterResult{
		StableID:    id,
		State:       "open",
		Canonical:   contracts.ClusterMember{Kind: "issue", Owner: "o", Repo: "r", Number: 1},
		MemberCount: 2,
		Members: []contracts.ClusterMember{
			{Kind: "issue", Owner: "o", Repo: "r", Number: 1, Title: "first", Score: 1.0, Reason: "canonical", Included: true},
			{Kind: "issue", Owner: "o", Repo: "r", Number: 2, Title: "second", Score: 0.9, Reason: "similar title", Included: true},
		},
	}, f.err
}

func (f *fakeSurfacesService) AddLens(ctx context.Context, name string, def lens.Definition) (*contracts.LensResult, error) {
	f.addLensCalled = true
	f.lastLensName = name
	f.lastLensDef = def
	return &contracts.LensResult{
		Name:       name,
		Definition: def,
		CreatedAt:  "2026-07-17T00:00:00Z",
		UpdatedAt:  "2026-07-17T00:00:00Z",
	}, f.err
}

func (f *fakeSurfacesService) ListLenses(ctx context.Context) (*contracts.LensListResult, error) {
	f.listLensCalled = true
	return &contracts.LensListResult{Lenses: []contracts.LensResult{{Name: "active-go"}}, Total: 2, Truncated: true}, f.err
}

func (f *fakeSurfacesService) ShowLens(ctx context.Context, name string) (*contracts.LensResult, error) {
	f.showLensCalled = true
	f.lastLensName = name
	return &contracts.LensResult{
		Name: name,
		Definition: lens.Definition{
			Name:    name,
			Filter:  lens.Filter{Kinds: []string{"issue"}},
			Weights: map[string]float64{"relevance": 1},
		},
		CreatedAt: "2026-07-17T00:00:00Z",
		UpdatedAt: "2026-07-17T00:00:00Z",
	}, f.err
}

func (f *fakeSurfacesService) ExplainLens(ctx context.Context, name, ref string, opts contracts.LensExplainOptions) (*contracts.LensExplainResult, error) {
	f.explainLensCalled = true
	f.lastLensName = name
	f.lastLensExplainRef = ref
	f.lastLensExplainOpts = opts
	return &contracts.LensExplainResult{
		Lens: contracts.LensResult{
			Name: name,
			Definition: lens.Definition{
				Name:    name,
				Filter:  lens.Filter{Kinds: []string{"issue"}},
				Weights: map[string]float64{"relevance": 1},
			},
		},
		Candidate: contracts.LensExplainCandidate{
			Kind:  "issue",
			Repo:  contracts.RepoRef{Owner: "o", Repo: "r"},
			Title: "fix it",
		},
		Score: 0.75,
		Signals: []contracts.LensExplainSignal{
			{Name: "relevance", Value: 0.8, Weight: 1, Contribution: 0.75},
		},
	}, f.err
}

func (f *fakeSurfacesService) CreateCollection(ctx context.Context, name string) (*contracts.CollectionResult, error) {
	f.createColCalled = true
	f.lastCreateColName = name
	return &contracts.CollectionResult{Name: name, MemberCount: 0}, f.err
}

func (f *fakeSurfacesService) AddCollectionMembers(ctx context.Context, name string, members []contracts.CollectionMember) (*contracts.CollectionResult, error) {
	f.addColCalled = true
	f.lastAddColName = name
	f.lastAddColMembers = members
	return &contracts.CollectionResult{Name: name, MemberCount: len(members)}, f.err
}

func (f *fakeSurfacesService) ListCollections(ctx context.Context) (*contracts.CollectionListResult, error) {
	f.listColCalled = true
	return &contracts.CollectionListResult{Collections: []contracts.CollectionResult{{Name: "favorites", MemberCount: 2}}, Total: 2, Truncated: true}, f.err
}

func (f *fakeSurfacesService) ArchiveSync(ctx context.Context, repo contracts.RepoRef, opts contracts.ArchiveSyncOptions) (*contracts.SyncResult, error) {
	f.archiveCalled = true
	f.syncEvents = append(f.syncEvents, "sync")
	f.lastArchiveOpts = opts
	return &contracts.SyncResult{Repo: repo, Updated: len(opts.Numbers), PlannedRequests: 11, RequestBudget: opts.MaxRequests, Message: "synced"}, f.err
}

func (f *fakeSurfacesService) PlanArchiveSync(_ context.Context, repo contracts.RepoRef, opts contracts.ArchiveSyncOptions) (*contracts.SyncPlanResult, error) {
	f.planCalled = true
	f.syncEvents = append(f.syncEvents, "plan")
	return &contracts.SyncPlanResult{
		Repo: repo, FixedRequests: 9, ThreadRequestCeiling: len(opts.Numbers), PlannedRequests: 9 + len(opts.Numbers),
		RequestBudget: opts.MaxRequests, MaxPages: opts.MaxPages, ExactThreads: len(opts.Numbers),
	}, f.err
}

func (f *fakeSurfacesService) Hydrate(ctx context.Context, repo contracts.RepoRef, number int, opts contracts.HydrateOptions) (*contracts.HydrateResult, error) {
	f.hydrateCalled = true
	f.lastHydrateOpts = opts
	return &contracts.HydrateResult{Repo: repo, Number: number, Kind: "issue", Requests: 1, Facets: []contracts.HydratedFacet{{Facet: "issue_comments", Complete: true}}}, f.err
}

func (f *fakeSurfacesService) Coverage(ctx context.Context, repo contracts.RepoRef) (*contracts.CoverageResult, error) {
	f.coverageCalled = true
	return &contracts.CoverageResult{Repo: repo, Facets: []contracts.CoverageFacet{{Facet: "threads", Present: true, Complete: true}}}, f.err
}

func (f *fakeSurfacesService) ArchiveThreads(ctx context.Context, repo contracts.RepoRef, kind, state string, limit int) (*contracts.ThreadListResult, error) {
	f.threadsCalled = true
	return &contracts.ThreadListResult{Repo: repo, Threads: []contracts.ThreadListItem{{Kind: "issue", Number: 1, State: "open", Title: "one"}}}, f.err
}

func (f *fakeSurfacesService) RunHistory(ctx context.Context, limit int) (*contracts.RunListResult, error) {
	f.runsCalled = true
	return &contracts.RunListResult{Runs: []contracts.RunResult{{ID: 1, Kind: "sync", Status: "completed"}}}, f.err
}

func (f *fakeSurfacesService) NeighborQuery(ctx context.Context, repo contracts.RepoRef, kind string, number, limit int) (*contracts.NeighborListResult, error) {
	f.neighborsCalled = true
	return &contracts.NeighborListResult{Repo: repo, Kind: kind, Number: number, SourceRevision: "rev", Neighbors: []contracts.NeighborResult{{Kind: "issue", Repo: repo, Number: 2, Score: .8, Reason: "similar title"}}}, f.err
}

func (f *fakeSurfacesService) ExportDossier(ctx context.Context, repo contracts.RepoRef, format string) (*contracts.ExportResult, error) {
	f.exportCalled = true
	return &contracts.ExportResult{Kind: "dossier", Format: format, Content: "# dossier\n"}, f.err
}

func (f *fakeSurfacesService) ExportEvidence(ctx context.Context, investigationID, format string) (*contracts.ExportResult, error) {
	f.exportCalled = true
	return &contracts.ExportResult{Kind: "evidence", Format: format, Content: "# evidence\n"}, f.err
}

func (f *fakeSurfacesService) ExportManifest(_ context.Context, opportunityID string, _ contracts.ManifestExportOptions) (*contracts.ExportResult, error) {
	f.exportCalled = true
	return &contracts.ExportResult{Kind: "manifest", Format: "json", Content: `{"manifest":"` + opportunityID + `"}`}, f.err
}

func newSurfacesCLI(svc *fakeSurfacesService) (*cli.CLI, *strings.Builder, *strings.Builder) {
	var stdout, stderr strings.Builder
	return cli.New(svc, nil, &stdout, &stderr), &stdout, &stderr
}

func TestClustersCommand(t *testing.T) {
	t.Parallel()
	svc := &fakeSurfacesService{fakeService: &fakeService{}}
	c, stdout, _ := newSurfacesCLI(svc)
	requireNoErr(t, c.Run(context.Background(), []string{"clusters", "list", "o/r"}))
	if !svc.clustersCalled || svc.lastClustersArg.String() != "o/r" {
		t.Fatalf("clusters not called: called=%v repo=%+v", svc.clustersCalled, svc.lastClustersArg)
	}
	if !strings.Contains(stdout.String(), "abc12345") || !strings.Contains(stdout.String(), "1 shown, truncated") {
		t.Fatalf("stdout = %q", stdout.String())
	}

	c2, stdout, _ := newSurfacesCLI(svc)
	requireNoErr(t, c2.Run(context.Background(), []string{"clusters", "list", "o/r", "--json"}))
	var got contracts.ClusterListResult
	if err := json.Unmarshal([]byte(stdout.String()), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if got.Total != 2 || !got.Truncated {
		t.Fatalf("unexpected JSON: %+v", got)
	}

	c3, _, _ := newSurfacesCLI(svc)
	requireNoErr(t, c3.Run(context.Background(), []string{"clusters", "refresh", "o/r"}))
	if !svc.refreshCalled || svc.lastClustersArg.String() != "o/r" {
		t.Fatalf("cluster refresh not called: called=%v repo=%+v", svc.refreshCalled, svc.lastClustersArg)
	}
}

func TestClusterShowCommand(t *testing.T) {
	t.Parallel()
	svc := &fakeSurfacesService{fakeService: &fakeService{}}
	c, stdout, _ := newSurfacesCLI(svc)
	requireNoErr(t, c.Run(context.Background(), []string{"cluster", "show", "abc12345"}))
	if !svc.clusterCalled || svc.lastClusterID != "abc12345" || svc.lastClusterLimit != 100 {
		t.Fatalf("cluster show not called: called=%v id=%q limit=%d", svc.clusterCalled, svc.lastClusterID, svc.lastClusterLimit)
	}
	if !strings.Contains(stdout.String(), "second") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestLensAddListShow(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := dir + "/lens.json"
	data := []byte(`{"filter":{"kinds":["issue"],"updated_within":"720h"},"weights":{"relevance":1}}`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write lens file: %v", err)
	}

	svc := &fakeSurfacesService{fakeService: &fakeService{}}
	c, stdout, _ := newSurfacesCLI(svc)
	requireNoErr(t, c.Run(context.Background(), []string{"lens", "add", "active-go", "--file", path}))
	if !svc.addLensCalled || svc.lastLensName != "active-go" || svc.lastLensDef.Filter.UpdatedWithin != 720*time.Hour {
		t.Fatalf("add lens not called: called=%v name=%q updated_within=%v", svc.addLensCalled, svc.lastLensName, svc.lastLensDef.Filter.UpdatedWithin)
	}
	if !strings.Contains(stdout.String(), "active-go") {
		t.Fatalf("stdout = %q", stdout.String())
	}

	c2, stdout, _ := newSurfacesCLI(svc)
	requireNoErr(t, c2.Run(context.Background(), []string{"lens", "list", "--json"}))
	var list contracts.LensListResult
	if err := json.Unmarshal([]byte(stdout.String()), &list); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if len(list.Lenses) != 1 || list.Total != 2 || !list.Truncated {
		t.Fatalf("unexpected list: %+v", list)
	}

	c3, stdout, _ := newSurfacesCLI(svc)
	requireNoErr(t, c3.Run(context.Background(), []string{"lens", "show", "active-go"}))
	if !svc.showLensCalled || svc.lastLensName != "active-go" {
		t.Fatalf("show lens not called: called=%v name=%q", svc.showLensCalled, svc.lastLensName)
	}
	if !strings.Contains(stdout.String(), "active-go") {
		t.Fatalf("stdout = %q", stdout.String())
	}

	c4, stdout, _ := newSurfacesCLI(svc)
	requireNoErr(t, c4.Run(context.Background(), []string{
		"lens", "explain", "active-go", "o/r#1", "--query", "fix", "--repo", "o/r",
		"--kind", "all", "--state", "open", "--author", "octo", "--association", "member",
		"--assignee", "hubot", "--label", "bug", "--updated-after", "2026-07-01T00:00:00Z",
	}))
	wantUpdatedAfter := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	if !svc.explainLensCalled || svc.lastLensName != "active-go" || svc.lastLensExplainRef != "o/r#1" ||
		svc.lastLensExplainOpts.Query != "fix" || svc.lastLensExplainOpts.Repo != "o/r" ||
		svc.lastLensExplainOpts.Kind != "all" || svc.lastLensExplainOpts.State != "open" ||
		svc.lastLensExplainOpts.Author != "octo" || svc.lastLensExplainOpts.Association != "member" ||
		svc.lastLensExplainOpts.Assignee != "hubot" || !slices.Equal(svc.lastLensExplainOpts.Labels, []string{"bug"}) ||
		!svc.lastLensExplainOpts.UpdatedAfter.Equal(wantUpdatedAfter) {
		t.Fatalf("explain lens not called: called=%v name=%q ref=%q", svc.explainLensCalled, svc.lastLensName, svc.lastLensExplainRef)
	}
	if !strings.Contains(stdout.String(), "Lens: active-go") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestLensExplainRequiresRef(t *testing.T) {
	t.Parallel()
	svc := &fakeSurfacesService{fakeService: &fakeService{}}
	c, _, _ := newSurfacesCLI(svc)
	err := c.Run(context.Background(), []string{"lens", "explain", "active-go"})
	requireCLIError(t, err, cli.ExitUsage)
}

func TestCollectionCreateAddList(t *testing.T) {
	t.Parallel()
	svc := &fakeSurfacesService{fakeService: &fakeService{}}
	c, stdout, _ := newSurfacesCLI(svc)
	requireNoErr(t, c.Run(context.Background(), []string{"collection", "create", "favorites"}))
	if !svc.createColCalled || svc.lastCreateColName != "favorites" {
		t.Fatalf("create collection not called: called=%v name=%q", svc.createColCalled, svc.lastCreateColName)
	}

	c2, stdout, _ := newSurfacesCLI(svc)
	requireNoErr(t, c2.Run(context.Background(), []string{"collection", "add", "favorites", "repo:o/r", "issue:o/r#1", "pr:o/r#2"}))
	if !svc.addColCalled || svc.lastAddColName != "favorites" || len(svc.lastAddColMembers) != 3 {
		t.Fatalf("add collection not called: called=%v name=%q members=%+v", svc.addColCalled, svc.lastAddColName, svc.lastAddColMembers)
	}
	want := []contracts.CollectionMember{
		{Kind: "repository", Ref: "o/r"},
		{Kind: "issue", Ref: "o/r#1"},
		{Kind: "pull_request", Ref: "o/r#2"},
	}
	for i, m := range svc.lastAddColMembers {
		if m != want[i] {
			t.Fatalf("member %d = %+v, want %+v", i, m, want[i])
		}
	}

	c3, stdout, _ := newSurfacesCLI(svc)
	requireNoErr(t, c3.Run(context.Background(), []string{"collection", "list"}))
	if !svc.listColCalled || !strings.Contains(stdout.String(), "favorites") || !strings.Contains(stdout.String(), "1 collections of 2 (truncated)") {
		t.Fatalf("list collections not called: called=%v stdout=%q", svc.listColCalled, stdout.String())
	}
}

func TestCollectionAddRejectsInvalidMember(t *testing.T) {
	t.Parallel()
	svc := &fakeSurfacesService{fakeService: &fakeService{}}
	c, _, _ := newSurfacesCLI(svc)
	err := c.Run(context.Background(), []string{"collection", "add", "favorites", "bad"})
	requireCLIError(t, err, cli.ExitUsage)
	if svc.addColCalled {
		t.Fatal("add collection should not be called for invalid member")
	}
}

func TestArchiveAndLocalQueryCommands(t *testing.T) {
	t.Parallel()
	svc := &fakeSurfacesService{fakeService: &fakeService{}}
	c, stdout, stderr := newSurfacesCLI(svc)

	requireNoErr(t, c.Run(context.Background(), []string{"archive", "sync", "o/r", "--numbers", "2,1", "--max-pages", "5", "--max-requests", "12", "--json"}))
	if !svc.archiveCalled || !svc.planCalled || len(svc.lastArchiveOpts.Numbers) != 2 || svc.lastArchiveOpts.MaxPages != 5 || svc.lastArchiveOpts.MaxRequests != 12 {
		t.Fatalf("archive options = %+v", svc.lastArchiveOpts)
	}
	if !slices.Equal(svc.syncEvents, []string{"plan", "sync"}) || !strings.Contains(stderr.String(), "planned sync for o/r: up to 11 requests (9 fixed + up to 2 thread requests; budget 12)") {
		t.Fatalf("sync was not planned before execution: events=%v stderr=%q", svc.syncEvents, stderr.String())
	}
	stdout.Reset()
	requireNoErr(t, c.Run(context.Background(), []string{"archive", "hydrate", "o/r#1", "--with", "issue_comments", "--json"}))
	if !svc.hydrateCalled || len(svc.lastHydrateOpts.Facets) != 1 {
		t.Fatalf("hydrate options = %+v", svc.lastHydrateOpts)
	}
	stdout.Reset()
	requireNoErr(t, c.Run(context.Background(), []string{"archive", "refresh", "o/r", "--json"}))
	requireNoErr(t, c.Run(context.Background(), []string{"archive", "threads", "o/r", "--kind", "issue", "--json"}))
	requireNoErr(t, c.Run(context.Background(), []string{"archive", "coverage", "o/r", "--json"}))
	if !svc.threadsCalled {
		t.Fatal("archive threads was not dispatched")
	}
	stdout.Reset()
	requireNoErr(t, c.Run(context.Background(), []string{"coverage", "o/r", "--json"}))
	requireNoErr(t, c.Run(context.Background(), []string{"runs", "--limit", "5", "--json"}))
	requireNoErr(t, c.Run(context.Background(), []string{"neighbors", "o/r#1", "--kind", "issue", "--json"}))
	if !svc.coverageCalled || !svc.runsCalled || !svc.neighborsCalled {
		t.Fatal("one or more local query commands were not dispatched")
	}
}

func TestArchiveSyncRejectsConflictingFiltersBeforeDispatch(t *testing.T) {
	t.Parallel()
	svc := &fakeSurfacesService{fakeService: &fakeService{}}
	c, _, stderr := newSurfacesCLI(svc)
	err := c.Run(context.Background(), []string{"archive", "sync", "o/r", "--numbers", "1", "--since", "1h"})
	requireCLIError(t, err, cli.ExitUsage)
	if svc.archiveCalled {
		t.Fatal("archive service was called for invalid input")
	}
	if stderr.Len() != 0 {
		t.Fatalf("progress was printed before validation: %q", stderr.String())
	}
}

func TestExportCommandWritesContent(t *testing.T) {
	t.Parallel()
	svc := &fakeSurfacesService{fakeService: &fakeService{}}
	c, stdout, _ := newSurfacesCLI(svc)
	requireNoErr(t, c.Run(context.Background(), []string{"export", "dossier", "o/r"}))
	if !svc.exportCalled || stdout.String() != "# dossier\n" {
		t.Fatalf("export output = %q", stdout.String())
	}
}

func TestExportDossierReturnsServiceErrorWithoutOutput(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("export failed")
	svc := &fakeSurfacesService{fakeService: &fakeService{err: wantErr}}
	c, stdout, _ := newSurfacesCLI(svc)

	err := c.Run(context.Background(), []string{"export", "dossier", "o/r"})

	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if stdout.Len() != 0 {
		t.Fatalf("output = %q, want empty", stdout.String())
	}
}
