package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

type preflightReader struct {
	github.Reader
	identityErr   error
	authored      github.AuthoredPullRequestSearchResult
	related       github.ThreadSearchResult
	details       map[int]github.PullRequestDetails
	searchOptions github.AuthoredPullRequestSearchOptions
}

func (r *preflightReader) GetAuthenticatedIdentity(context.Context) (github.Identity, github.RateInfo, error) {
	if r.identityErr != nil {
		return github.Identity{}, github.RateInfo{}, r.identityErr
	}
	return github.Identity{Login: "morluto", ID: 1}, github.RateInfo{}, nil
}

func (r *preflightReader) SearchAuthoredPullRequests(_ context.Context, opts github.AuthoredPullRequestSearchOptions) (github.AuthoredPullRequestSearchResult, error) {
	r.searchOptions = opts
	return r.authored, nil
}

func (r *preflightReader) SearchThreads(context.Context, github.ThreadSearchOptions) (github.ThreadSearchResult, error) {
	return r.related, nil
}

func (r *preflightReader) GetPullRequestDetails(_ context.Context, _, _ string, number int) (github.PullRequestDetails, github.RateInfo, error) {
	details, ok := r.details[number]
	if !ok {
		return github.PullRequestDetails{}, github.RateInfo{}, errors.New("missing details")
	}
	return details, github.RateInfo{}, nil
}

func TestPreflightContributionRoutesExistingAuthoredPRAndLocalWorktree(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)

	worktree := t.TempDir()
	runGitApp(t, worktree, "init", "-b", "main")
	runGitApp(t, worktree, "config", "user.email", "test@example.com")
	runGitApp(t, worktree, "config", "user.name", "Test")
	writeAppFile(t, filepath.Join(worktree, "README.md"), "base\n")
	runGitApp(t, worktree, "add", ".")
	runGitApp(t, worktree, "commit", "-m", "base")
	runGitApp(t, worktree, "checkout", "-b", "fix/n01-mask-cu-seqlens-v2")
	runGitApp(t, worktree, "remote", "add", "origin", "https://github.com/morluto/flash-linear-attention.git")
	headSHA := runGitApp(t, worktree, "rev-parse", "HEAD")

	reader := &preflightReader{
		authored: github.AuthoredPullRequestSearchResult{Items: []github.Issue{{
			RepositoryOwner: "fla-org", RepositoryName: "flash-linear-attention", Kind: github.ThreadKindPullRequest,
			Number: 1088, Title: "Fix n01 mask cu seqlens v2", Author: "morluto",
		}}, Total: 1},
		related: github.ThreadSearchResult{Items: []github.Issue{
			{RepositoryOwner: "fla-org", RepositoryName: "flash-linear-attention", Kind: github.ThreadKindIssue, Number: 1086, Title: "Fix n01 mask cu seqlens v2"},
			{RepositoryOwner: "fla-org", RepositoryName: "flash-linear-attention", Kind: github.ThreadKindPullRequest, Number: 1088, Title: "Fix n01 mask cu seqlens v2"},
		}, Total: 2},
		details: map[int]github.PullRequestDetails{1088: {
			Number: 1088, HeadRef: "fix/n01-mask-cu-seqlens-v2", HeadSHA: headSHA,
			HeadOwner: "morluto", HeadRepo: "flash-linear-attention", HTMLURL: "https://github.com/fla-org/flash-linear-attention/pull/1088",
		}},
	}
	svc.SetGitHubReader(reader)
	beforeCorpus, err := svc.corpus.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}

	out, err := (&MCPReader{svc}).PreflightContribution(ctx, mcpcontract.ContributionPreflightInput{
		Repository: mcpcontract.RepositoryRef{Owner: "fla-org", Repo: "flash-linear-attention"},
		Candidate:  mcpcontract.ContributionPreflightCandidate{Title: "Fix n01 mask cu seqlens v2"}, WorkspacePaths: []string{worktree},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "existing_pr" || out.Coverage != "live_verified" || out.Existing == nil {
		t.Fatalf("preflight = %+v", out)
	}
	if out.Existing.Issue != 1086 || out.Existing.PullRequest != 1088 || out.Existing.Head != "morluto:fix/n01-mask-cu-seqlens-v2" {
		t.Fatalf("existing = %+v", out.Existing)
	}
	expectedWorktree, err := filepath.EvalSymlinks(worktree)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.LocalMatches) != 1 || out.LocalMatches[0].Path != expectedWorktree || out.LocalMatches[0].Branch != "fix/n01-mask-cu-seqlens-v2" {
		t.Fatalf("local matches = %+v", out.LocalMatches)
	}
	if reader.searchOptions.RepositoryOwner != "fla-org" || reader.searchOptions.RepositoryName != "flash-linear-attention" {
		t.Fatalf("authored search was not repository scoped: %+v", reader.searchOptions)
	}
	if out.NextAction != "review_or_follow_through" {
		t.Fatalf("next action = %q", out.NextAction)
	}
	afterCorpus, err := svc.corpus.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if beforeCorpus != afterCorpus {
		t.Fatalf("preflight mutated corpus status: before=%+v after=%+v", beforeCorpus, afterCorpus)
	}
}

func TestPreflightContributionNeverClaimsNewWorkWithIncompleteCoverage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)
	reader := &preflightReader{identityErr: errors.New("token unavailable")}
	svc.SetGitHubReader(reader)
	out, err := (&MCPReader{svc}).PreflightContribution(ctx, mcpcontract.ContributionPreflightInput{
		Repository: mcpcontract.RepositoryRef{Owner: "fla-org", Repo: "flash-linear-attention"},
		Candidate:  mcpcontract.ContributionPreflightCandidate{Title: "Unrelated change"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "coverage_unknown" || out.Coverage != "coverage_unknown" || out.NextAction != "complete_preflight_coverage" {
		t.Fatalf("incomplete preflight = %+v", out)
	}
	if len(out.CoverageReasons) == 0 {
		t.Fatal("missing coverage reason")
	}
}

func TestPreflightContributionReturnsNewWorkOnlyAfterLiveNegativeChecks(t *testing.T) {
	t.Parallel()
	svc := newSearchTestService(t)
	reader := &preflightReader{
		authored: github.AuthoredPullRequestSearchResult{Items: nil, Total: 0},
		related:  github.ThreadSearchResult{Items: nil, Total: 0},
	}
	svc.SetGitHubReader(reader)
	out, err := (&MCPReader{svc}).PreflightContribution(context.Background(), mcpcontract.ContributionPreflightInput{
		Repository: mcpcontract.RepositoryRef{Owner: "fla-org", Repo: "flash-linear-attention"},
		Candidate:  mcpcontract.ContributionPreflightCandidate{Title: "Unrelated change"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "new_work" || out.Coverage != "live_verified" || out.NextAction != "create_local_work" {
		t.Fatalf("negative preflight = %+v", out)
	}
}
