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
	"github.com/morluto/gitcontribute/internal/mcpserver"
)

func TestBuildAndGetRepositoryDossier(t *testing.T) {
	t.Parallel()
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
		MergedKnown:     true,
	}, prPayload(0, 0, 0)); err != nil {
		t.Fatalf("upsert closed pr: %v", err)
	}

	if _, err := upsertThread(ctx, svc.corpus, repo.ID, corpus.Thread{
		RepositoryID:    repo.ID,
		Kind:            corpus.ThreadKindPullRequest,
		Number:          8,
		State:           "closed",
		Title:           "header-only closed pull request",
		SourceCreatedAt: base,
		SourceUpdatedAt: base.Add(2 * time.Hour),
		ClosedAt:        base.Add(time.Hour),
	}, `{}`); err != nil {
		t.Fatalf("upsert unknown-merge pr: %v", err)
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
	if d.ClosedPullRequestUnknownCount != 1 || len(d.RecentClosedUnknownPullRequests) != 1 || d.RecentClosedUnknownPullRequests[0].Number != 8 {
		t.Fatalf("unexpected unknown-merge PRs: count=%d recent=%+v", d.ClosedPullRequestUnknownCount, d.RecentClosedUnknownPullRequests)
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

	if _, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{
		Owner: ref.Owner, Name: ref.Repo, Description: "A changed repo", Stars: 99,
		SourceUpdatedAt: time.Unix(3000, 0).UTC(),
	}, `{}`); err != nil {
		t.Fatalf("update repository after dossier build: %v", err)
	}
	mcpDossier, err := svc.MCPReader().Dossier(ctx, mcpserver.RepoInput{Owner: ref.Owner, Repo: ref.Repo})
	if err != nil {
		t.Fatalf("read persisted MCP dossier: %v", err)
	}
	if stars := mcpDossier.Sections["stars"]; stars != 10 {
		t.Fatalf("MCP dossier stars = %v, want persisted value 10", stars)
	}

	res, err := svc.Dossier(ctx, cli.RepoRef{Owner: ref.Owner, Repo: ref.Repo})
	if err != nil {
		t.Fatalf("dossier summary: %v", err)
	}
	if res.Stars != 99 || res.OpenIssues != 1 || res.Summary != "A changed repo" {
		t.Fatalf("unexpected dossier summary: %+v", res)
	}
}

func TestExtractSeeds(t *testing.T) {
	t.Parallel()
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
		MergedKnown:     true,
	}, prPayload(0, 0, 0)); err != nil {
		t.Fatalf("upsert closed pr: %v", err)
	}
	if _, err := upsertThread(ctx, svc.corpus, repo.ID, corpus.Thread{
		RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 3,
		State: "closed", Title: "header-only closed PR", SourceCreatedAt: base, SourceUpdatedAt: base.Add(2 * time.Hour),
	}, `{}`); err != nil {
		t.Fatalf("upsert unknown-merge pr: %v", err)
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
	if _, err := upsertThread(ctx, svc.corpus, repo.ID, corpus.Thread{
		RepositoryID:    repo.ID,
		Kind:            corpus.ThreadKindIssue,
		Number:          2,
		State:           "closed",
		StateReason:     "not_planned",
		Title:           "replace the parser",
		Body:            "This direction is outside the project scope.",
		Author:          "dana",
		SourceCreatedAt: base,
		SourceUpdatedAt: base.Add(6 * time.Hour),
		ClosedAt:        base.Add(6 * time.Hour),
	}, `{}`); err != nil {
		t.Fatalf("upsert not-planned issue: %v", err)
	}

	seeds, err := svc.ExtractSeeds(ctx, cli.RepoRef{Owner: ref.Owner, Repo: ref.Repo}, domain.ExtractSeedsOptions{})
	if err != nil {
		t.Fatalf("extract seeds: %v", err)
	}
	if len(seeds) != 3 {
		t.Fatalf("expected 3 positive/negative seeds, got %d: %+v", len(seeds), seeds)
	}
	for _, seed := range seeds {
		if seed.Polarity == domain.SeedPolarityContext {
			t.Fatalf("default extraction included contextual issue: %+v", seed)
		}
	}

	merged := findSeed(seeds, domain.SeedSourceClassMergedPR, 5)
	if merged == nil {
		t.Fatal("missing merged PR seed")
	}
	if merged.Polarity != domain.SeedPolarityPositive || !strings.Contains(merged.PolarityReason, "was merged") {
		t.Fatalf("merged PR polarity = %+v", merged)
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
	if closed.Polarity != domain.SeedPolarityNegative || !strings.Contains(closed.PolarityReason, "without merging") {
		t.Fatalf("closed PR polarity = %+v", closed)
	}
	if closed.Evidence.RejectionOrSupersession == "" {
		t.Fatalf("expected rejection/supersession context, got empty")
	}
	notPlanned := findSeed(seeds, domain.SeedSourceClassIssue, 2)
	if notPlanned == nil || notPlanned.Polarity != domain.SeedPolarityNegative || notPlanned.Evidence.RejectionOrSupersession != "GitHub state reason: not_planned" {
		t.Fatalf("not-planned issue polarity/evidence = %+v", notPlanned)
	}

	contextOnly, err := svc.ExtractSeeds(ctx, cli.RepoRef{Owner: ref.Owner, Repo: ref.Repo}, domain.ExtractSeedsOptions{
		Classes:    []domain.SeedSourceClass{domain.SeedSourceClassIssue},
		Polarities: []domain.SeedPolarity{domain.SeedPolarityContext},
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("extract context seeds: %v", err)
	}
	if len(contextOnly) != 1 || contextOnly[0].Number != 1 || contextOnly[0].Polarity != domain.SeedPolarityContext {
		t.Fatalf("expected open issue context, got %+v", contextOnly)
	}

	negativeIssues, err := svc.ExtractSeeds(ctx, cli.RepoRef{Owner: ref.Owner, Repo: ref.Repo}, domain.ExtractSeedsOptions{
		Classes:    []domain.SeedSourceClass{domain.SeedSourceClassIssue},
		Polarities: []domain.SeedPolarity{domain.SeedPolarityNegative},
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("extract negative issue seeds: %v", err)
	}
	if len(negativeIssues) != 1 || negativeIssues[0].Number != 2 {
		t.Fatalf("expected not-planned issue only, got %+v", negativeIssues)
	}
	empty, err := svc.ExtractSeeds(ctx, cli.RepoRef{Owner: ref.Owner, Repo: ref.Repo}, domain.ExtractSeedsOptions{
		Classes:    []domain.SeedSourceClass{domain.SeedSourceClassMergedPR},
		Polarities: []domain.SeedPolarity{domain.SeedPolarityContext},
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("extract empty seed selection: %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Fatalf("empty seed selection = %#v, want non-nil empty list", empty)
	}

	bounded, err := svc.ExtractSeeds(ctx, cli.RepoRef{Owner: ref.Owner, Repo: ref.Repo}, domain.ExtractSeedsOptions{Limit: 1})
	if err != nil {
		t.Fatalf("extract bounded: %v", err)
	}
	if len(bounded) != 1 {
		t.Fatalf("expected 1 seed with limit 1, got %d", len(bounded))
	}

	if _, err := svc.ExtractSeeds(ctx, cli.RepoRef{Owner: ref.Owner, Repo: ref.Repo}, domain.ExtractSeedsOptions{Polarities: []domain.SeedPolarity{"invented"}}); err == nil || !strings.Contains(err.Error(), "unknown seed polarity") {
		t.Fatalf("invalid polarity error = %v", err)
	}
	if _, err := svc.ExtractSeeds(ctx, cli.RepoRef{Owner: ref.Owner, Repo: ref.Repo}, domain.ExtractSeedsOptions{Classes: []domain.SeedSourceClass{"invented"}}); err == nil || !strings.Contains(err.Error(), "unknown seed source class") {
		t.Fatalf("invalid source class error = %v", err)
	}
	cliSeeds, err := svc.ExtractSeedsForCLI(ctx, cli.RepoRef{Owner: ref.Owner, Repo: ref.Repo}, []string{"issues"}, []string{"context"}, 10)
	if err != nil {
		t.Fatalf("extract CLI context seeds: %v", err)
	}
	typedCLISeeds, ok := cliSeeds.([]domain.Seed)
	if !ok || len(typedCLISeeds) != 1 || typedCLISeeds[0].Number != 1 {
		t.Fatalf("CLI context seeds = %#v", cliSeeds)
	}
	if _, err := svc.ExtractSeedsForCLI(ctx, cli.RepoRef{Owner: ref.Owner, Repo: ref.Repo}, nil, []string{"invented"}, 10); err == nil || !strings.Contains(err.Error(), "unknown seed polarity") {
		t.Fatalf("invalid CLI polarity error = %v", err)
	}
}

func TestSeedPolarityUsesOnlyStructuredOutcomeEvidence(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		thread     corpus.Thread
		class      domain.SeedSourceClass
		want       domain.SeedPolarity
		wantReason string
	}{
		{
			name:       "merged PR remains positive despite rejection text",
			thread:     corpus.Thread{Kind: corpus.ThreadKindPullRequest, State: "closed", Merged: true, Title: "rejected experiment"},
			class:      domain.SeedSourceClassMergedPR,
			want:       domain.SeedPolarityPositive,
			wantReason: "GitHub reports this pull request was merged",
		},
		{
			name:       "closed unmerged PR is negative",
			thread:     corpus.Thread{Kind: corpus.ThreadKindPullRequest, State: "closed", MergedKnown: true},
			class:      domain.SeedSourceClassClosedUnmergedPR,
			want:       domain.SeedPolarityNegative,
			wantReason: "GitHub reports this pull request was closed without merging",
		},
		{
			name:       "issue text cannot imply rejection",
			thread:     corpus.Thread{Kind: corpus.ThreadKindIssue, State: "closed", StateReason: "completed", Title: "rejected idea", Body: "superseded elsewhere"},
			class:      domain.SeedSourceClassIssue,
			want:       domain.SeedPolarityContext,
			wantReason: "issue evidence provides problem context, not an implementation outcome",
		},
		{
			name:       "open issue rejection label remains context",
			thread:     corpus.Thread{Kind: corpus.ThreadKindIssue, State: "open", Labels: []string{"duplicate"}},
			class:      domain.SeedSourceClassIssue,
			want:       domain.SeedPolarityContext,
			wantReason: "issue evidence provides problem context, not an implementation outcome",
		},
		{
			name:       "not planned issue is negative",
			thread:     corpus.Thread{Kind: corpus.ThreadKindIssue, State: "closed", StateReason: "not_planned"},
			class:      domain.SeedSourceClassIssue,
			want:       domain.SeedPolarityNegative,
			wantReason: "GitHub reports this issue was closed as not planned",
		},
		{
			name:       "closed duplicate issue is negative",
			thread:     corpus.Thread{Kind: corpus.ThreadKindIssue, State: "closed", StateReason: "completed", Labels: []string{"Duplicate"}},
			class:      domain.SeedSourceClassIssue,
			want:       domain.SeedPolarityNegative,
			wantReason: "closed issue has rejection or supersession label: Duplicate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := polarityForThread(tt.thread, tt.class)
			if got != tt.want || reason != tt.wantReason {
				t.Fatalf("polarity = %q (%q), want %q (%q)", got, reason, tt.want, tt.wantReason)
			}
		})
	}
}

func TestExtractSeedsRequiresNoNetwork(t *testing.T) {
	t.Parallel()
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

	seeds, err := svc.ExtractSeeds(ctx, cli.RepoRef{Owner: ref.Owner, Repo: ref.Repo}, domain.ExtractSeedsOptions{Polarities: []domain.SeedPolarity{domain.SeedPolarityContext}})
	if err != nil {
		t.Fatalf("extract seeds without network reader: %v", err)
	}
	if len(seeds) != 1 || seeds[0].Polarity != domain.SeedPolarityContext {
		t.Fatalf("offline context seeds = %+v", seeds)
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
