package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/lens"
)

type fakeSurfacesService struct {
	*fakeService

	clustersCalled  bool
	clusterCalled   bool
	addLensCalled   bool
	listLensCalled  bool
	showLensCalled  bool
	createColCalled bool
	addColCalled    bool
	listColCalled   bool

	lastClustersArg   cli.RepoRef
	lastClusterID     string
	lastClusterLimit  int
	lastLensName      string
	lastLensDef       lens.Definition
	lastCreateColName string
	lastAddColName    string
	lastAddColMembers []cli.CollectionMember
}

func (f *fakeSurfacesService) Clusters(ctx context.Context, repo cli.RepoRef, limit int) (*cli.ClusterListResult, error) {
	f.clustersCalled = true
	f.lastClustersArg = repo
	return &cli.ClusterListResult{
		Repo:  repo,
		Total: 2,
		Clusters: []cli.ClusterResult{
			{
				StableID:    "abc12345",
				State:       "open",
				Canonical:   cli.ClusterMember{Kind: "issue", Owner: repo.Owner, Repo: repo.Repo, Number: 1},
				MemberCount: 3,
			},
		},
	}, f.err
}

func (f *fakeSurfacesService) Cluster(ctx context.Context, id string, limit int) (*cli.ClusterResult, error) {
	f.clusterCalled = true
	f.lastClusterID = id
	f.lastClusterLimit = limit
	return &cli.ClusterResult{
		StableID:    id,
		State:       "open",
		Canonical:   cli.ClusterMember{Kind: "issue", Owner: "o", Repo: "r", Number: 1},
		MemberCount: 2,
		Members: []cli.ClusterMember{
			{Kind: "issue", Owner: "o", Repo: "r", Number: 1, Title: "first", Score: 1.0, Reason: "canonical", Included: true},
			{Kind: "issue", Owner: "o", Repo: "r", Number: 2, Title: "second", Score: 0.9, Reason: "similar title", Included: true},
		},
	}, f.err
}

func (f *fakeSurfacesService) AddLens(ctx context.Context, name string, def lens.Definition) (*cli.LensResult, error) {
	f.addLensCalled = true
	f.lastLensName = name
	f.lastLensDef = def
	return &cli.LensResult{
		Name:       name,
		Definition: def,
		CreatedAt:  "2026-07-17T00:00:00Z",
		UpdatedAt:  "2026-07-17T00:00:00Z",
	}, f.err
}

func (f *fakeSurfacesService) ListLenses(ctx context.Context) (*cli.LensListResult, error) {
	f.listLensCalled = true
	return &cli.LensListResult{Lenses: []cli.LensResult{{Name: "active-go"}}}, f.err
}

func (f *fakeSurfacesService) ShowLens(ctx context.Context, name string) (*cli.LensResult, error) {
	f.showLensCalled = true
	f.lastLensName = name
	return &cli.LensResult{
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

func (f *fakeSurfacesService) CreateCollection(ctx context.Context, name string) (*cli.CollectionResult, error) {
	f.createColCalled = true
	f.lastCreateColName = name
	return &cli.CollectionResult{Name: name, MemberCount: 0}, f.err
}

func (f *fakeSurfacesService) AddCollectionMembers(ctx context.Context, name string, members []cli.CollectionMember) (*cli.CollectionResult, error) {
	f.addColCalled = true
	f.lastAddColName = name
	f.lastAddColMembers = members
	return &cli.CollectionResult{Name: name, MemberCount: len(members)}, f.err
}

func (f *fakeSurfacesService) ListCollections(ctx context.Context) (*cli.CollectionListResult, error) {
	f.listColCalled = true
	return &cli.CollectionListResult{Collections: []cli.CollectionResult{{Name: "favorites", MemberCount: 2}}}, f.err
}

func newSurfacesCLI(svc *fakeSurfacesService) (*cli.CLI, *strings.Builder, *strings.Builder) {
	var stdout, stderr strings.Builder
	return cli.New(svc, nil, &stdout, &stderr), &stdout, &stderr
}

func TestClustersCommand(t *testing.T) {
	svc := &fakeSurfacesService{fakeService: &fakeService{}}
	c, stdout, _ := newSurfacesCLI(svc)
	requireNoErr(t, c.Run(context.Background(), []string{"clusters", "o/r"}))
	if !svc.clustersCalled || svc.lastClustersArg.String() != "o/r" {
		t.Fatalf("clusters not called: called=%v repo=%+v", svc.clustersCalled, svc.lastClustersArg)
	}
	if !strings.Contains(stdout.String(), "abc12345") {
		t.Fatalf("stdout = %q", stdout.String())
	}

	c2, stdout, _ := newSurfacesCLI(svc)
	requireNoErr(t, c2.Run(context.Background(), []string{"clusters", "o/r", "--json"}))
	var got cli.ClusterListResult
	if err := json.Unmarshal([]byte(stdout.String()), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if got.Total != 2 {
		t.Fatalf("unexpected JSON: %+v", got)
	}
}

func TestClusterShowCommand(t *testing.T) {
	svc := &fakeSurfacesService{fakeService: &fakeService{}}
	c, stdout, _ := newSurfacesCLI(svc)
	requireNoErr(t, c.Run(context.Background(), []string{"cluster", "abc12345"}))
	if !svc.clusterCalled || svc.lastClusterID != "abc12345" || svc.lastClusterLimit != 100 {
		t.Fatalf("cluster show not called: called=%v id=%q limit=%d", svc.clusterCalled, svc.lastClusterID, svc.lastClusterLimit)
	}
	if !strings.Contains(stdout.String(), "second") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestLensAddListShow(t *testing.T) {
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
	var list cli.LensListResult
	if err := json.Unmarshal([]byte(stdout.String()), &list); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if len(list.Lenses) != 1 {
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
}

func TestCollectionCreateAddList(t *testing.T) {
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
	want := []cli.CollectionMember{
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
	if !svc.listColCalled || !strings.Contains(stdout.String(), "favorites") {
		t.Fatalf("list collections not called: called=%v stdout=%q", svc.listColCalled, stdout.String())
	}
}

func TestCollectionAddRejectsInvalidMember(t *testing.T) {
	svc := &fakeSurfacesService{fakeService: &fakeService{}}
	c, _, _ := newSurfacesCLI(svc)
	err := c.Run(context.Background(), []string{"collection", "add", "favorites", "bad"})
	requireCLIError(t, err, cli.ExitUsage)
	if svc.addColCalled {
		t.Fatal("add collection should not be called for invalid member")
	}
}
