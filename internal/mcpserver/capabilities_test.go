package mcpserver

import (
	"context"

	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

type fakeOptionalCapabilities struct {
	base             *fakeReader
	syncThreadsInput mcpcontract.SyncThreadsInput
}

func (*fakeOptionalCapabilities) FindNeighbors(context.Context, mcpcontract.FindNeighborsInput) (mcpcontract.FindNeighborsOutput, error) {
	return mcpcontract.FindNeighborsOutput{}, nil
}
func (*fakeOptionalCapabilities) GetRepositories(context.Context, mcpcontract.GetRepositoriesInput) (mcpcontract.GetRepositoriesOutput, error) {
	return mcpcontract.GetRepositoriesOutput{Status: "complete"}, nil
}
func (*fakeOptionalCapabilities) GetThreads(context.Context, mcpcontract.GetThreadsInput) (mcpcontract.GetThreadsOutput, error) {
	return mcpcontract.GetThreadsOutput{Status: "complete"}, nil
}
func (*fakeOptionalCapabilities) RankOpportunities(context.Context, mcpcontract.RankOpportunitiesInput) (mcpcontract.RankOpportunitiesOutput, error) {
	return mcpcontract.RankOpportunitiesOutput{Status: "complete"}, nil
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
	return f.base.SearchGitHubRepositories(ctx, in)
}
func (*fakeOptionalCapabilities) SyncRepositoryMetadata(context.Context, mcpcontract.SyncRepositoryMetadataInput) (mcpcontract.JobReference, error) {
	return mcpcontract.JobReference{ID: "job-metadata", Status: "queued"}, nil
}
func (f *fakeOptionalCapabilities) SyncThreads(_ context.Context, in mcpcontract.SyncThreadsInput) (mcpcontract.JobReference, error) {
	f.syncThreadsInput = in
	return mcpcontract.JobReference{ID: "job-threads", Status: "queued"}, nil
}
func (*fakeOptionalCapabilities) HydrateThreads(context.Context, mcpcontract.HydrateThreadsInput) (mcpcontract.JobReference, error) {
	return mcpcontract.JobReference{ID: "job-hydrate", Status: "queued"}, nil
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
func (*fakeOptionalCapabilities) IndexRepositories(context.Context, mcpcontract.IndexRepositoriesInput) (mcpcontract.JobReference, error) {
	return mcpcontract.JobReference{ID: "job-index", Status: "queued"}, nil
}
func (*fakeOptionalCapabilities) CheckMergeConflicts(context.Context, mcpcontract.CheckMergeConflictsInput) (mcpcontract.CheckMergeConflictsOutput, error) {
	return mcpcontract.CheckMergeConflictsOutput{Status: "complete"}, nil
}
func (*fakeOptionalCapabilities) DeepWiki(context.Context, mcpcontract.DeepWikiInput) (mcpcontract.DeepWikiOutput, error) {
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
		PortfolioReader: optional, GitHubOperator: optional, CodeIndexer: optional,
		MergeConflictReader: optional, ResearchReader: optional,
		CommitPlannerReader: base,
		PortfolioOperator:   optional, Operator: base,
		ConcernReader: base, ConcernOperator: base,
		WorkspaceCreator: base, WorkspaceAdopter: base,
	}
}
