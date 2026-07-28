package mcpserver

import (
	"context"

	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

type fakeOptionalCapabilities struct {
	base                  *fakeReader
	syncThreadsInput      mcpcontract.SyncThreadsInput
	fixPatternCalls       int
	lastFixPatternRequest mcpcontract.MineRepositoryFixPatternsInput
}

func (*fakeOptionalCapabilities) FindNeighbors(context.Context, mcpcontract.FindNeighborsInput) (mcpcontract.FindNeighborsOutput, error) {
	return mcpcontract.FindNeighborsOutput{}, nil
}
func (*fakeOptionalCapabilities) GetRepositories(_ context.Context, in mcpcontract.GetRepositoriesInput) (mcpcontract.GetRepositoriesOutput, error) {
	items := make([]mcpcontract.BatchItem[mcpcontract.TypedRepositoryOutput], len(in.Repositories))
	for i, repository := range in.Repositories {
		value := mcpcontract.TypedRepositoryOutput{
			Ref:           "repository:" + repository.Owner + "/" + repository.Repo,
			Owner:         repository.Owner,
			Repo:          repository.Repo,
			Metadata:      mcpcontract.RepositoryMetadataOutput{Status: "complete"},
			DossierStatus: "missing",
		}
		if repository.Repo == "rocket" {
			value.DossierStatus = "available"
			value.DossierAsOf = "2026-07-25T00:00:00Z"
		}
		items[i] = mcpcontract.BatchItem[mcpcontract.TypedRepositoryOutput]{
			Key: repository.Owner + "/" + repository.Repo, Status: "complete", Value: &value,
		}
	}
	return mcpcontract.GetRepositoriesOutput{Status: "complete", Items: items}, nil
}
func (*fakeOptionalCapabilities) GetThreads(context.Context, mcpcontract.GetThreadsInput) (mcpcontract.GetThreadsOutput, error) {
	return mcpcontract.GetThreadsOutput{Status: "complete"}, nil
}
func (f *fakeOptionalCapabilities) RankOpportunities(context.Context, mcpcontract.RankOpportunitiesInput) (mcpcontract.RankOpportunitiesOutput, error) {
	score := 87
	if f.base.radarScore != 0 {
		score = f.base.radarScore
	}
	return mcpcontract.RankOpportunitiesOutput{
		Status: "complete",
		Candidates: []mcpcontract.OpportunityCandidateOutput{{
			Rank:        1,
			Ref:         "thread:acme/rocket/issue/7",
			Repo:        "acme/rocket",
			Number:      7,
			Title:       "engine stalls",
			URL:         "https://github.com/acme/rocket/issues/7",
			Score:       mcpcontract.RadarScore(score),
			Eligibility: "needs_coordination",
			Confidence:  "medium",
		}},
		Total: 1,
	}, nil
}
func (*fakeOptionalCapabilities) FindPrecedents(context.Context, mcpcontract.FindPrecedentsInput) (mcpcontract.FindPrecedentsOutput, error) {
	return mcpcontract.FindPrecedentsOutput{Status: "complete"}, nil
}
func (*fakeOptionalCapabilities) PrepareIssueSet(context.Context, mcpcontract.PrepareIssueSetInput) (mcpcontract.PrepareIssueSetOutput, error) {
	return mcpcontract.PrepareIssueSetOutput{Status: "complete"}, nil
}
func (f *fakeOptionalCapabilities) GetJobs(ctx context.Context, in mcpcontract.GetJobsInput) (mcpcontract.GetJobsOutput, error) {
	items := make([]mcpcontract.BatchItem[mcpcontract.GetJobOutput], len(in.IDs))
	for i, id := range in.IDs {
		job, err := f.base.GetJob(ctx, mcpcontract.GetJobInput{ID: id})
		if err != nil {
			return mcpcontract.GetJobsOutput{}, err
		}
		items[i] = mcpcontract.BatchItem[mcpcontract.GetJobOutput]{Key: id, Status: "complete", Value: &job}
	}
	return mcpcontract.GetJobsOutput{Status: "complete", Items: items}, nil
}
func (*fakeOptionalCapabilities) ListPullRequestPortfolio(context.Context, mcpcontract.ListPullRequestPortfolioInput) (mcpcontract.ListPullRequestPortfolioOutput, error) {
	return mcpcontract.ListPullRequestPortfolioOutput{Status: "complete"}, nil
}
func (*fakeOptionalCapabilities) FindPortfolioOverlaps(context.Context, mcpcontract.FindPortfolioOverlapsInput) (mcpcontract.FindPortfolioOverlapsOutput, error) {
	return mcpcontract.FindPortfolioOverlapsOutput{Status: "complete"}, nil
}
func (f *fakeOptionalCapabilities) SearchGitHubRepositories(ctx context.Context, in mcpcontract.SearchGitHubRepositoriesInput) (mcpcontract.SearchGitHubRepositoriesOutput, error) {
	f.base.recordCall("search_github_repositories")
	return f.base.SearchGitHubRepositories(ctx, in)
}
func (*fakeOptionalCapabilities) SyncRepositoryContext(context.Context, mcpcontract.SyncRepositoryContextInput) (mcpcontract.JobReference, error) {
	return mcpcontract.JobReference{ID: "job-metadata", Status: "queued"}, nil
}
func (f *fakeOptionalCapabilities) SyncThreads(_ context.Context, in mcpcontract.SyncThreadsInput) (mcpcontract.JobReference, error) {
	f.base.recordCall("sync_threads")
	f.syncThreadsInput = in
	return mcpcontract.JobReference{ID: "job-threads", Status: "queued"}, nil
}
func (*fakeOptionalCapabilities) HydrateThreads(context.Context, mcpcontract.HydrateThreadsInput) (mcpcontract.JobReference, error) {
	return mcpcontract.JobReference{ID: "job-hydrate", Status: "queued"}, nil
}
func (f *fakeOptionalCapabilities) MineRepositoryFixPatterns(_ context.Context, in mcpcontract.MineRepositoryFixPatternsInput) (mcpcontract.JobReference, error) {
	f.fixPatternCalls++
	f.lastFixPatternRequest = in
	return mcpcontract.JobReference{ID: "job-fix-patterns", Kind: "mine_repository_fix_patterns", Status: "queued"}, nil
}
func (*fakeOptionalCapabilities) GetAuthenticatedIdentity(context.Context) (mcpcontract.AuthenticatedIdentityOutput, error) {
	return mcpcontract.AuthenticatedIdentityOutput{Login: "alice"}, nil
}
func (*fakeOptionalCapabilities) SyncAuthoredPullRequests(context.Context, mcpcontract.SyncAuthoredPullRequestsInput) (mcpcontract.JobReference, error) {
	return mcpcontract.JobReference{ID: "job-authored", Status: "queued"}, nil
}
func (*fakeOptionalCapabilities) SyncPullRequestStatus(context.Context, mcpcontract.SyncPullRequestStatusInput) (mcpcontract.JobReference, error) {
	return mcpcontract.JobReference{ID: "job-status", Status: "queued"}, nil
}
func (*fakeOptionalCapabilities) SyncPortfolio(context.Context, mcpcontract.SyncPortfolioInput) (mcpcontract.JobReference, error) {
	return mcpcontract.JobReference{ID: "job-portfolio", Kind: "sync_portfolio", Status: "queued"}, nil
}
func (*fakeOptionalCapabilities) IndexRepositories(context.Context, mcpcontract.IndexRepositoriesInput) (mcpcontract.JobReference, error) {
	return mcpcontract.JobReference{ID: "job-index", Status: "queued"}, nil
}
func (*fakeOptionalCapabilities) CheckMergeConflicts(context.Context, mcpcontract.CheckMergeConflictsInput) (mcpcontract.CheckMergeConflictsOutput, error) {
	return mcpcontract.CheckMergeConflictsOutput{Status: "complete"}, nil
}
func (f *fakeOptionalCapabilities) DeepWiki(context.Context, mcpcontract.DeepWikiInput) (mcpcontract.DeepWikiOutput, error) {
	f.base.recordCall("deepwiki")
	return mcpcontract.DeepWikiOutput{Status: "complete"}, nil
}
func (*fakeOptionalCapabilities) LinkPullRequest(context.Context, mcpcontract.LinkPullRequestInput) (mcpcontract.LinkPullRequestOutput, error) {
	return mcpcontract.LinkPullRequestOutput{}, nil
}

type completeTestReader struct {
	mcpcontract.Reader
	NeighborReader
	ScalableReader
	IssueSetReader
	PortfolioReader
	GitHubOperator
	FixPatternOperator
	FixPatternReader
	CodeIndexer
	MergeConflictReader
	CommitPlannerReader
	ResearchReader
	PortfolioOperator
	Operator
	ConcernReader
	ConcernOperator
	WorkspaceCreator
	WorkspaceAdopter
}

func completeFakeReader(base *fakeReader) mcpcontract.Reader {
	optional := &fakeOptionalCapabilities{base: base}
	return completeTestReader{
		Reader: base, NeighborReader: optional, ScalableReader: optional, IssueSetReader: optional,
		PortfolioReader: optional, GitHubOperator: optional, FixPatternOperator: optional, FixPatternReader: base, CodeIndexer: optional,
		MergeConflictReader: optional, ResearchReader: optional,
		CommitPlannerReader: base,
		PortfolioOperator:   optional, Operator: base,
		ConcernReader: base, ConcernOperator: base,
		WorkspaceCreator: base, WorkspaceAdopter: base,
	}
}
