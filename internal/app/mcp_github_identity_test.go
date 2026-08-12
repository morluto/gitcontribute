package app

import (
	"context"
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

type githubFactsReader struct {
	github.Reader
	forkStatus string
}

func (*githubFactsReader) GetAuthenticatedIdentity(context.Context) (github.Identity, github.RateInfo, error) {
	return github.Identity{Login: "morluto", ID: 1}, github.RateInfo{}, nil
}

func (*githubFactsReader) GetRepository(_ context.Context, owner, repo string) (github.Repository, github.RateInfo, error) {
	result := github.Repository{Owner: owner, Name: repo, DefaultBranch: "main"}
	if !strings.EqualFold(owner, "fla-org") {
		result.Fork = true
		result.Parent = &github.RepositoryParent{Owner: "fla-org", Name: "flash-linear-attention"}
	}
	return result, github.RateInfo{}, nil
}

func (*githubFactsReader) GetBranch(_ context.Context, owner, _, branch string) (github.Branch, github.RateInfo, error) {
	sha := "upstream-sha"
	if !strings.EqualFold(owner, "fla-org") {
		sha = "fork-sha"
	}
	return github.Branch{Name: branch, CommitSHA: sha}, github.RateInfo{}, nil
}

func (r *githubFactsReader) CompareCommits(context.Context, string, string, string, string) (github.CommitComparison, github.RateInfo, error) {
	status := r.forkStatus
	if status == "" {
		status = "identical"
	}
	return github.CommitComparison{Status: status, BaseSHA: "upstream-sha", MergeBaseSHA: "merge-base"}, github.RateInfo{}, nil
}

func TestGetAuthenticatedIdentityReturnsOnlyCredentialIdentity(t *testing.T) {
	t.Parallel()
	svc := newSearchTestService(t)
	svc.SetGitHubReader(&githubFactsReader{})

	out, err := (&MCPReader{svc}).GetAuthenticatedIdentity(context.Background(), mcpcontract.GetAuthenticatedIdentityInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Login != "morluto" || out.DatabaseID != 1 || out.ObservedAt == "" {
		t.Fatalf("identity = %+v", out)
	}
}

func TestCompareForkUsesExplicitRepositories(t *testing.T) {
	t.Parallel()
	svc := newSearchTestService(t)
	svc.SetGitHubReader(&githubFactsReader{forkStatus: "behind"})

	out, err := (&MCPReader{svc}).CompareFork(context.Background(), mcpcontract.CompareForkInput{
		Upstream:           mcpcontract.RepositoryRef{Owner: "fla-org", Repo: "flash-linear-attention"},
		Fork:               mcpcontract.RepositoryRef{Owner: "morluto", Repo: "flash-linear-attention"},
		ContributionBranch: "fix/attention",
		ContributionSHA:    "candidate-sha",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "behind" || out.Coverage != "verified" || !out.EffectiveDiffRisk {
		t.Fatalf("fork comparison = %+v", out)
	}
	if out.Fork.Owner != "morluto" || out.ContributionBranch != "fix/attention" || out.ContributionSHA != "candidate-sha" {
		t.Fatalf("fork provenance = %+v", out)
	}
}
