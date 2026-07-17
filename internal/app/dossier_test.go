package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
)

func TestBuildAndGetRepositoryDossier(t *testing.T) {
	ctx := context.Background()
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	if _, err := svc.Init(ctx); err != nil {
		t.Fatal(err)
	}

	ref := domain.RepoRef{Owner: "owner", Repo: "repo"}

	if _, _, err := svc.corpus.StoreCodeSnapshot(ctx, ref, codeindex.Snapshot{
		RepoPath:   "/repo",
		Commit:     "deadbeef",
		CreatedAt:  time.Unix(500, 0).UTC(),
		TotalBytes: 10,
	}); err != nil {
		t.Fatalf("store code snapshot: %v", err)
	}

	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{
		Owner:           ref.Owner,
		Name:            ref.Repo,
		Description:     "A test repo",
		Language:        "Go",
		DefaultBranch:   "main",
		Stars:           10,
		OpenIssues:      3,
		SourceUpdatedAt: time.Unix(1000, 0).UTC(),
	}, `{}`)
	if err != nil {
		t.Fatalf("upsert repository: %v", err)
	}

	base := time.Unix(2000, 0).UTC()
	if _, err := upsertThread(ctx, svc.corpus, repo.ID, corpus.Thread{
		RepositoryID:    repo.ID,
		Kind:            corpus.ThreadKindPullRequest,
		Number:          10,
		State:           "closed",
		Title:           "fix(pkg/parser): resolve crash",
		Body:            "Closes #1. Adds tests.",
		Author:          "alice",
		Labels:          []string{"bug"},
		SourceCreatedAt: base,
		SourceUpdatedAt: base.Add(4 * time.Hour),
		ClosedAt:        base.Add(2 * time.Hour),
		MergedAt:        base.Add(2 * time.Hour),
		Merged:          true,
	}, prPayload(2, 120, 45)); err != nil {
		t.Fatalf("upsert merged pr: %v", err)
	}

	if _, err := upsertThread(ctx, svc.corpus, repo.ID, corpus.Thread{
		RepositoryID:    repo.ID,
		Kind:            corpus.ThreadKindPullRequest,
		Number:          9,
		State:           "closed",
		Title:           "feat(ui): add button",
		Body:            "Superseded by #10.",
		Author:          "bob",
		Labels:          []string{"duplicate", "enhancement"},
		SourceCreatedAt: base,
		SourceUpdatedAt: base.Add(3 * time.Hour),
		ClosedAt:        base.Add(1 * time.Hour),
		Merged:          false,
	}, prPayload(0, 0, 0)); err != nil {
		t.Fatalf("upsert closed pr: %v", err)
	}

	if _, err := upsertThread(ctx, svc.corpus, repo.ID, corpus.Thread{
		RepositoryID:    repo.ID,
		Kind:            corpus.ThreadKindIssue,
		Number:          1,
		State:           "open",
		Title:           "parser crashes on empty input",
		Body:            "The pkg/parser module panics. See #2.",
		Author:          "carol",
		Labels:          []string{"bug"},
		SourceCreatedAt: base,
		SourceUpdatedAt: base.Add(5 * time.Hour),
	}, `{}`); err != nil {
		t.Fatalf("upsert issue: %v", err)
	}

	d, err := svc.BuildRepositoryDossier(ctx, cli.RepoRef{Owner: ref.Owner, Repo: ref.Repo})
	if err != nil {
		t.Fatalf("build dossier: %v", err)
	}
	if d.CommitSHA != "deadbeef" {
		t.Fatalf("commit sha = %q, want deadbeef", d.CommitSHA)
	}
	if d.Repository.Description != "A test repo" {
		t.Fatalf("unexpected repository: %+v", d.Repository)
	}
	if len(d.RecentMergedPullRequests) != 1 || d.RecentMergedPullRequests[0].Number != 10 {
		t.Fatalf("unexpected merged PRs: %+v", d.RecentMergedPullRequests)
	}

	got, err := svc.GetRepositoryDossier(ctx, cli.RepoRef{Owner: ref.Owner, Repo: ref.Repo})
	if err != nil {
		t.Fatalf("get dossier: %v", err)
	}
	if got.CommitSHA != "deadbeef" {
		t.Fatalf("got commit sha = %q", got.CommitSHA)
	}
	if diff := cmp.Diff(d.CommitSHA, got.CommitSHA); diff != "" {
		t.Fatalf("dossier mismatch: %s", diff)
	}
	if len(got.SourceRefs) == 0 {
		t.Fatal("expected source refs in dossier")
	}

	res, err := svc.Dossier(ctx, cli.RepoRef{Owner: ref.Owner, Repo: ref.Repo})
	if err != nil {
		t.Fatalf("legacy Dossier: %v", err)
	}
	if res.Stars != 10 || res.OpenIssues != 1 || res.Summary != "A test repo" {
		t.Fatalf("unexpected legacy dossier result: %+v", res)
	}
}

func TestExtractSeeds(t *testing.T) {
	ctx := context.Background()
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	if _, err := svc.Init(ctx); err != nil {
		t.Fatal(err)
	}

	ref := domain.RepoRef{Owner: "owner", Repo: "repo"}
	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{
		Owner:           ref.Owner,
		Name:            ref.Repo,
		SourceUpdatedAt: time.Unix(1000, 0).UTC(),
	}, `{}`)
	if err != nil {
		t.Fatalf("upsert repository: %v", err)
	}

	base := time.Unix(2000, 0).UTC()
	if _, err := upsertThread(ctx, svc.corpus, repo.ID, corpus.Thread{
		RepositoryID:    repo.ID,
		Kind:            corpus.ThreadKindPullRequest,
		Number:          5,
		State:           "closed",
		Title:           "fix(pkg/parser): resolve crash",
		Body:            "Closes #1. Adds tests.",
		Author:          "alice",
		Labels:          []string{"bug"},
		SourceCreatedAt: base,
		SourceUpdatedAt: base.Add(4 * time.Hour),
		ClosedAt:        base.Add(2 * time.Hour),
		MergedAt:        base.Add(2 * time.Hour),
		Merged:          true,
	}, prPayload(2, 120, 45)); err != nil {
		t.Fatalf("upsert merged pr: %v", err)
	}

	if _, err := upsertThread(ctx, svc.corpus, repo.ID, corpus.Thread{
		RepositoryID:    repo.ID,
		Kind:            corpus.ThreadKindPullRequest,
		Number:          4,
		State:           "closed",
		Title:           "feat(ui): add button",
		Body:            "Superseded by #5.",
		Author:          "bob",
		Labels:          []string{"duplicate"},
		SourceCreatedAt: base,
		SourceUpdatedAt: base.Add(3 * time.Hour),
		ClosedAt:        base.Add(1 * time.Hour),
		Merged:          false,
	}, prPayload(0, 0, 0)); err != nil {
		t.Fatalf("upsert closed pr: %v", err)
	}

	if _, err := upsertThread(ctx, svc.corpus, repo.ID, corpus.Thread{
		RepositoryID:    repo.ID,
		Kind:            corpus.ThreadKindIssue,
		Number:          1,
		State:           "open",
		Title:           "parser crashes on empty input",
		Body:            "The pkg/parser module panics. See #2.",
		Author:          "carol",
		Labels:          []string{"bug"},
		SourceCreatedAt: base,
		SourceUpdatedAt: base.Add(5 * time.Hour),
	}, `{}`); err != nil {
		t.Fatalf("upsert issue: %v", err)
	}

	seeds, err := svc.ExtractSeeds(ctx, cli.RepoRef{Owner: ref.Owner, Repo: ref.Repo}, domain.ExtractSeedsOptions{})
	if err != nil {
		t.Fatalf("extract seeds: %v", err)
	}
	if len(seeds) != 3 {
		t.Fatalf("expected 3 seeds, got %d: %+v", len(seeds), seeds)
	}

	if seeds[0].SourceClass != domain.SeedSourceClassIssue || seeds[0].Number != 1 {
		t.Fatalf("expected issue #1 first, got %+v", seeds[0])
	}

	merged := findSeed(seeds, domain.SeedSourceClassMergedPR, 5)
	if merged == nil {
		t.Fatal("missing merged PR seed")
	}
	if merged.Evidence.ApproximateScope != "small" {
		t.Fatalf("merged scope = %q, want small", merged.Evidence.ApproximateScope)
	}
	if !contains(merged.Evidence.ValidationIndicators, "test") && !contains(merged.Evidence.ValidationIndicators, "tests") {
		t.Fatalf("expected validation indicator 'test' or 'tests', got %v", merged.Evidence.ValidationIndicators)
	}
	if !contains(merged.Evidence.IssueLinkages, "Closes #1") && !contains(merged.Evidence.IssueLinkages, "closes #1") {
		t.Fatalf("expected issue linkage 'Closes #1', got %v", merged.Evidence.IssueLinkages)
	}
	if !contains(merged.Evidence.ProblemAreas, "pkg/parser") {
		t.Fatalf("expected problem area pkg/parser, got %v", merged.Evidence.ProblemAreas)
	}

	closed := findSeed(seeds, domain.SeedSourceClassClosedUnmergedPR, 4)
	if closed == nil {
		t.Fatal("missing closed unmerged PR seed")
	}
	if closed.Evidence.RejectionOrSupersession == "" {
		t.Fatalf("expected rejection/supersession context, got empty")
	}

	issueOnly, err := svc.ExtractSeeds(ctx, cli.RepoRef{Owner: ref.Owner, Repo: ref.Repo}, domain.ExtractSeedsOptions{
		Classes: []domain.SeedSourceClass{domain.SeedSourceClassIssue},
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("extract issue seeds: %v", err)
	}
	if len(issueOnly) != 1 || issueOnly[0].SourceClass != domain.SeedSourceClassIssue {
		t.Fatalf("expected 1 issue seed, got %+v", issueOnly)
	}

	bounded, err := svc.ExtractSeeds(ctx, cli.RepoRef{Owner: ref.Owner, Repo: ref.Repo}, domain.ExtractSeedsOptions{Limit: 1})
	if err != nil {
		t.Fatalf("extract bounded: %v", err)
	}
	if len(bounded) != 1 {
		t.Fatalf("expected 1 seed with limit 1, got %d", len(bounded))
	}
}

func TestExtractSeedsRequiresNoNetwork(t *testing.T) {
	ctx := context.Background()
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	if _, err := svc.Init(ctx); err != nil {
		t.Fatal(err)
	}

	ref := domain.RepoRef{Owner: "owner", Repo: "repo"}
	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{
		Owner:           ref.Owner,
		Name:            ref.Repo,
		SourceUpdatedAt: time.Unix(1000, 0).UTC(),
	}, `{}`)
	if err != nil {
		t.Fatalf("upsert repository: %v", err)
	}

	if _, err := upsertThread(ctx, svc.corpus, repo.ID, corpus.Thread{
		RepositoryID:    repo.ID,
		Kind:            corpus.ThreadKindIssue,
		Number:          1,
		State:           "open",
		Title:           "a bug",
		Body:            "details",
		Author:          "dev",
		SourceCreatedAt: time.Unix(2000, 0).UTC(),
		SourceUpdatedAt: time.Unix(3000, 0).UTC(),
	}, `{}`); err != nil {
		t.Fatalf("upsert thread: %v", err)
	}

	_, err = svc.ExtractSeeds(ctx, cli.RepoRef{Owner: ref.Owner, Repo: ref.Repo}, domain.ExtractSeedsOptions{})
	if err != nil {
		t.Fatalf("extract seeds without network reader: %v", err)
	}
}

func upsertThread(ctx context.Context, c *corpus.Corpus, repoID int64, thread corpus.Thread, payload any) (*corpus.Thread, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	thread.RepositoryID = repoID
	return c.UpsertThread(ctx, thread, string(payloadJSON))
}

func prPayload(changed, added, deleted int) prPayloadStruct {
	return prPayloadStruct{ChangedFiles: changed, Additions: added, Deletions: deleted}
}

type prPayloadStruct struct {
	ChangedFiles int `json:"ChangedFiles"`
	Additions    int `json:"Additions"`
	Deletions    int `json:"Deletions"`
}

func findSeed(seeds []domain.Seed, class domain.SeedSourceClass, number int) *domain.Seed {
	for i := range seeds {
		if seeds[i].SourceClass == class && seeds[i].Number == number {
			return &seeds[i]
		}
	}
	return nil
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if strings.EqualFold(s, needle) {
			return true
		}
	}
	return false
}
