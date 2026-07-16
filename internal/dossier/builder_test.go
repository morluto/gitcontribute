package dossier

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

// fakeReader is a deterministic Reader implementation for builder tests.
type fakeReader struct {
	repo         domain.Repository
	repoRefs     []domain.SourceRef
	coverage     domain.Coverage
	guidance     string
	guidanceRefs []domain.SourceRef
	threads      map[threadKey][]domain.Thread
	threadRefs   map[threadKey][]domain.SourceRef
}

type threadKey struct {
	kind   string
	state  string
	merged string
}

func (f *fakeReader) ReadRepository(_ context.Context, ref domain.RepoRef) (domain.Repository, []domain.SourceRef, error) {
	if err := ref.Validate(); err != nil {
		return domain.Repository{}, nil, err
	}
	return f.repo, f.repoRefs, nil
}

func (f *fakeReader) ReadThreads(_ context.Context, ref domain.RepoRef, q ThreadQuery) ([]domain.Thread, []domain.SourceRef, error) {
	if err := ref.Validate(); err != nil {
		return nil, nil, err
	}
	merged := "*"
	if q.Merged != nil {
		if *q.Merged {
			merged = "true"
		} else {
			merged = "false"
		}
	}
	k := threadKey{kind: string(q.Kind), state: string(q.State), merged: merged}
	threads := f.threads[k]
	refs := f.threadRefs[k]
	if q.Limit > 0 && len(threads) > q.Limit {
		return threads[:q.Limit], refs, nil
	}
	return threads, refs, nil
}

func (f *fakeReader) ReadCoverage(_ context.Context, ref domain.RepoRef) (domain.Coverage, error) {
	if err := ref.Validate(); err != nil {
		return domain.Coverage{}, err
	}
	return f.coverage, nil
}

func (f *fakeReader) ReadContributionGuidance(_ context.Context, ref domain.RepoRef) (string, []domain.SourceRef, error) {
	if err := ref.Validate(); err != nil {
		return "", nil, err
	}
	return f.guidance, f.guidanceRefs, nil
}

var now = time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)

func TestBuilderValidation(t *testing.T) {
	b := NewBuilder(&fakeReader{}, 5)
	_, err := b.Build(context.Background(), domain.RepoRef{Owner: "", Repo: "go"})
	if err == nil {
		t.Fatal("expected validation error for empty owner")
	}
}

func TestBuilderBuild(t *testing.T) {
	ref := domain.RepoRef{Owner: "owner", Repo: "repo"}
	repo := domain.Repository{
		RepoRef:                        ref,
		CommitSHA:                      "abc123",
		Description:                    "A test repository",
		OpenIssueCount:                 3,
		ClosedIssueCount:               7,
		OpenPullRequestCount:           2,
		MergedPullRequestCount:         5,
		ClosedUnmergedPullRequestCount: 1,
	}
	coverage := domain.Coverage{
		AsOf: now,
		Facets: []domain.FacetCoverage{
			{Facet: "metadata", Present: true, Complete: true, Freshness: domain.Freshness{Status: domain.Fresh, AsOf: now}},
			{Facet: "threads", Present: true, Complete: false, Freshness: domain.Freshness{Status: domain.Stale, AsOf: now.Add(-time.Hour)}},
		},
	}
	guidance := "Please open an issue first."
	repoRef := domain.SourceRef{Source: "github:rest", URL: "https://api.github.com/repos/owner/repo", ObservedAt: now, AsOf: now}
	guideRef := domain.SourceRef{Source: "github:raw", URL: "https://raw.githubusercontent.com/owner/repo/main/CONTRIBUTING.md", ObservedAt: now, AsOf: now}
	threadRef := domain.SourceRef{Source: "github:graphql", URL: "https://api.github.com/graphql", ObservedAt: now, AsOf: now}

	// Return threads unsorted to exercise stable ordering in the builder.
	mergedPRs := []domain.Thread{
		{Repo: ref, Kind: domain.PullRequestKind, Number: 9, Title: "Second merged", State: domain.ClosedState, UpdatedAt: now.Add(-2 * time.Hour), CreatedAt: now.Add(-10 * time.Hour), PullRequest: &domain.PullRequestDetails{Merged: true, MergedAt: now.Add(-3 * time.Hour)}},
		{Repo: ref, Kind: domain.PullRequestKind, Number: 5, Title: "First merged", State: domain.ClosedState, UpdatedAt: now.Add(-time.Hour), CreatedAt: now.Add(-12 * time.Hour), PullRequest: &domain.PullRequestDetails{Merged: true, MergedAt: now.Add(-2 * time.Hour)}},
	}
	openPRs := []domain.Thread{
		{Repo: ref, Kind: domain.PullRequestKind, Number: 11, Title: "Open PR", State: domain.OpenState, UpdatedAt: now, CreatedAt: now.Add(-time.Hour)},
	}
	closedUnmergedPRs := []domain.Thread{
		{Repo: ref, Kind: domain.PullRequestKind, Number: 3, Title: "Closed unmerged", State: domain.ClosedState, UpdatedAt: now.Add(-3 * time.Hour), CreatedAt: now.Add(-20 * time.Hour), PullRequest: &domain.PullRequestDetails{Merged: false}},
	}
	issues := []domain.Thread{
		{Repo: ref, Kind: domain.IssueKind, Number: 42, Title: "Recent issue", State: domain.OpenState, UpdatedAt: now.Add(-30 * time.Minute), CreatedAt: now.Add(-2 * time.Hour)},
		{Repo: ref, Kind: domain.IssueKind, Number: 7, Title: "Old issue", State: domain.ClosedState, UpdatedAt: now.Add(-4 * time.Hour), CreatedAt: now.Add(-24 * time.Hour)},
	}

	fr := &fakeReader{
		repo:         repo,
		repoRefs:     []domain.SourceRef{repoRef},
		coverage:     coverage,
		guidance:     guidance,
		guidanceRefs: []domain.SourceRef{guideRef},
		threads: map[threadKey][]domain.Thread{
			{kind: "pull_request", state: "closed", merged: "true"}:  mergedPRs,
			{kind: "pull_request", state: "open", merged: "*"}:       openPRs,
			{kind: "pull_request", state: "closed", merged: "false"}: closedUnmergedPRs,
			{kind: "issue", state: "", merged: "*"}:                  issues,
		},
		threadRefs: map[threadKey][]domain.SourceRef{
			{kind: "pull_request", state: "closed", merged: "true"}:  {threadRef},
			{kind: "pull_request", state: "open", merged: "*"}:       {threadRef},
			{kind: "pull_request", state: "closed", merged: "false"}: {threadRef},
			{kind: "issue", state: "", merged: "*"}:                  {threadRef},
		},
	}

	b := NewBuilder(fr, 5)
	dossier, err := b.Build(context.Background(), ref)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if dossier.Repo.String() != "owner/repo" {
		t.Fatalf("unexpected repo %s", dossier.Repo.String())
	}
	if dossier.CommitSHA != "abc123" {
		t.Fatalf("unexpected commit sha %s", dossier.CommitSHA)
	}
	if dossier.ContributionGuidance != guidance {
		t.Fatalf("unexpected guidance %q", dossier.ContributionGuidance)
	}
	if !reflect.DeepEqual(dossier.Coverage, coverage) {
		t.Fatalf("unexpected coverage: %+v", dossier.Coverage)
	}

	wantCounts := map[string]int{
		"open issues":         3,
		"closed issues":       7,
		"open PRs":            2,
		"merged PRs":          5,
		"closed unmerged PRs": 1,
	}
	gotCounts := map[string]int{
		"open issues":         dossier.OpenIssueCount,
		"closed issues":       dossier.ClosedIssueCount,
		"open PRs":            dossier.OpenPullRequestCount,
		"merged PRs":          dossier.MergedPullRequestCount,
		"closed unmerged PRs": dossier.ClosedUnmergedPullRequestCount,
	}
	if !reflect.DeepEqual(gotCounts, wantCounts) {
		t.Fatalf("counts mismatch: got %+v, want %+v", gotCounts, wantCounts)
	}

	// Recent merged PRs should be sorted by UpdatedAt descending.
	if len(dossier.RecentMergedPullRequests) != 2 {
		t.Fatalf("expected 2 merged PRs, got %d", len(dossier.RecentMergedPullRequests))
	}
	if dossier.RecentMergedPullRequests[0].Number != 5 {
		t.Fatalf("expected first merged PR number 5, got %d", dossier.RecentMergedPullRequests[0].Number)
	}
	if dossier.RecentMergedPullRequests[1].Number != 9 {
		t.Fatalf("expected second merged PR number 9, got %d", dossier.RecentMergedPullRequests[1].Number)
	}

	if len(dossier.RecentOpenPullRequests) != 1 || dossier.RecentOpenPullRequests[0].Number != 11 {
		t.Fatalf("unexpected open PRs: %+v", dossier.RecentOpenPullRequests)
	}

	if len(dossier.RecentClosedUnmergedPullRequests) != 1 || dossier.RecentClosedUnmergedPullRequests[0].Number != 3 {
		t.Fatalf("unexpected closed unmerged PRs: %+v", dossier.RecentClosedUnmergedPullRequests)
	}

	if len(dossier.RecentIssues) != 2 || dossier.RecentIssues[0].Number != 42 {
		t.Fatalf("unexpected issues order: %+v", dossier.RecentIssues)
	}

	// Source references should be collected and deduplicated.
	if len(dossier.SourceRefs) != 3 {
		t.Fatalf("expected 3 source refs (deduped), got %d: %+v", len(dossier.SourceRefs), dossier.SourceRefs)
	}
	if !dossier.AsOf.Equal(now) {
		t.Fatalf("expected AsOf %v, got %v", now, dossier.AsOf)
	}
}

func TestBuilderDeterministicSorting(t *testing.T) {
	ref := domain.RepoRef{Owner: "owner", Repo: "repo"}
	repo := domain.Repository{RepoRef: ref}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Threads with identical UpdatedAt must be ordered by CreatedAt, number, title.
	threads := []domain.Thread{
		{Repo: ref, Kind: domain.IssueKind, Number: 3, Title: "C", State: domain.OpenState, UpdatedAt: base, CreatedAt: base},
		{Repo: ref, Kind: domain.IssueKind, Number: 1, Title: "A", State: domain.OpenState, UpdatedAt: base, CreatedAt: base},
		{Repo: ref, Kind: domain.IssueKind, Number: 2, Title: "B", State: domain.OpenState, UpdatedAt: base, CreatedAt: base},
	}

	fr := &fakeReader{
		repo:     repo,
		coverage: domain.Coverage{},
		threads: map[threadKey][]domain.Thread{
			{kind: "issue", state: "", merged: "*"}: threads,
		},
	}

	b := NewBuilder(fr, 10)
	dossier, err := b.Build(context.Background(), ref)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	want := []int{3, 2, 1}
	got := make([]int, len(dossier.RecentIssues))
	for i, dt := range dossier.RecentIssues {
		got[i] = dt.Number
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("deterministic order mismatch: got %v, want %v", got, want)
	}
}

func TestBuilderReaderError(t *testing.T) {
	ref := domain.RepoRef{Owner: "owner", Repo: "repo"}
	fr := &fakeReader{
		repo: domain.Repository{RepoRef: ref},
	}
	fr.threads = nil // not relevant

	// Simulate reader error by wrapping fakeReader with a failing one.
	failing := &failingReader{err: errors.New("boom")}
	b := NewBuilder(failing, 5)
	_, err := b.Build(context.Background(), ref)
	if err == nil {
		t.Fatal("expected error from failing reader")
	}
}

// failingReader always returns an error.
type failingReader struct {
	err error
}

func (f *failingReader) ReadRepository(context.Context, domain.RepoRef) (domain.Repository, []domain.SourceRef, error) {
	return domain.Repository{}, nil, f.err
}

func (f *failingReader) ReadThreads(context.Context, domain.RepoRef, ThreadQuery) ([]domain.Thread, []domain.SourceRef, error) {
	return nil, nil, f.err
}

func (f *failingReader) ReadCoverage(context.Context, domain.RepoRef) (domain.Coverage, error) {
	return domain.Coverage{}, f.err
}

func (f *failingReader) ReadContributionGuidance(context.Context, domain.RepoRef) (string, []domain.SourceRef, error) {
	return "", nil, f.err
}
