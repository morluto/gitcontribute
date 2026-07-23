package mcpserver

import "context"

type fakeOptionalCapabilities struct {
	base             *fakeReader
	syncThreadsInput SyncThreadsInput
}

func (*fakeOptionalCapabilities) FindNeighbors(context.Context, FindNeighborsInput) (FindNeighborsOutput, error) {
	return FindNeighborsOutput{}, nil
}
func (*fakeOptionalCapabilities) GetRepositories(context.Context, GetRepositoriesInput) (GetRepositoriesOutput, error) {
	return GetRepositoriesOutput{Status: "complete"}, nil
}
func (*fakeOptionalCapabilities) GetThreads(context.Context, GetThreadsInput) (GetThreadsOutput, error) {
	return GetThreadsOutput{Status: "complete"}, nil
}
func (*fakeOptionalCapabilities) RankOpportunities(context.Context, RankOpportunitiesInput) (RankOpportunitiesOutput, error) {
	return RankOpportunitiesOutput{Status: "complete"}, nil
}
func (*fakeOptionalCapabilities) FindPrecedents(context.Context, FindPrecedentsInput) (FindPrecedentsOutput, error) {
	return FindPrecedentsOutput{Status: "complete"}, nil
}
func (f *fakeOptionalCapabilities) GetJobs(ctx context.Context, in GetJobsInput) (GetJobsOutput, error) {
	items := make([]BatchItem[GetJobOutput], len(in.IDs))
	for i, id := range in.IDs {
		job, err := f.base.GetJob(ctx, GetJobInput{ID: id})
		if err != nil {
			return GetJobsOutput{}, err
		}
		items[i] = BatchItem[GetJobOutput]{Key: id, Status: "complete", Value: &job}
	}
	return GetJobsOutput{Status: "complete", Items: items}, nil
}
func (*fakeOptionalCapabilities) ListPullRequestPortfolio(context.Context, ListPullRequestPortfolioInput) (ListPullRequestPortfolioOutput, error) {
	return ListPullRequestPortfolioOutput{Status: "complete"}, nil
}
func (*fakeOptionalCapabilities) FindPortfolioOverlaps(context.Context, FindPortfolioOverlapsInput) (FindPortfolioOverlapsOutput, error) {
	return FindPortfolioOverlapsOutput{Status: "complete"}, nil
}
func (f *fakeOptionalCapabilities) SearchGitHubRepositories(ctx context.Context, in SearchGitHubRepositoriesInput) (SearchGitHubRepositoriesOutput, error) {
	return f.base.SearchGitHubRepositories(ctx, in)
}
func (*fakeOptionalCapabilities) SyncRepositoryMetadata(context.Context, SyncRepositoryMetadataInput) (JobReference, error) {
	return JobReference{ID: "job-metadata", Status: "queued"}, nil
}
func (f *fakeOptionalCapabilities) SyncThreads(_ context.Context, in SyncThreadsInput) (JobReference, error) {
	f.syncThreadsInput = in
	return JobReference{ID: "job-threads", Status: "queued"}, nil
}
func (*fakeOptionalCapabilities) HydrateThreads(context.Context, HydrateThreadsInput) (JobReference, error) {
	return JobReference{ID: "job-hydrate", Status: "queued"}, nil
}
func (*fakeOptionalCapabilities) GetAuthenticatedIdentity(context.Context) (AuthenticatedIdentityOutput, error) {
	return AuthenticatedIdentityOutput{Login: "alice"}, nil
}
func (*fakeOptionalCapabilities) SyncAuthoredPullRequests(context.Context, SyncAuthoredPullRequestsInput) (JobReference, error) {
	return JobReference{ID: "job-authored", Status: "queued"}, nil
}
func (*fakeOptionalCapabilities) SyncPullRequestStatus(context.Context, SyncPullRequestStatusInput) (JobReference, error) {
	return JobReference{ID: "job-status", Status: "queued"}, nil
}
func (*fakeOptionalCapabilities) IndexRepositories(context.Context, IndexRepositoriesInput) (JobReference, error) {
	return JobReference{ID: "job-index", Status: "queued"}, nil
}
func (*fakeOptionalCapabilities) CheckMergeConflicts(context.Context, CheckMergeConflictsInput) (CheckMergeConflictsOutput, error) {
	return CheckMergeConflictsOutput{Status: "complete"}, nil
}
func (*fakeOptionalCapabilities) DeepWiki(context.Context, DeepWikiInput) (DeepWikiOutput, error) {
	return DeepWikiOutput{Status: "complete"}, nil
}
func (*fakeOptionalCapabilities) LinkPullRequest(context.Context, LinkPullRequestInput) (LinkPullRequestOutput, error) {
	return LinkPullRequestOutput{}, nil
}

type completeTestReader struct {
	Reader
	NeighborReader
	ScalableReader
	PortfolioReader
	GitHubOperator
	CodeIndexer
	MergeConflictReader
	CommitPlannerReader
	ResearchReader
	PortfolioOperator
	Operator
}

func completeFakeReader(base *fakeReader) Reader {
	optional := &fakeOptionalCapabilities{base: base}
	return completeTestReader{
		Reader: base, NeighborReader: optional, ScalableReader: optional,
		PortfolioReader: optional, GitHubOperator: optional, CodeIndexer: optional,
		MergeConflictReader: optional, ResearchReader: optional,
		CommitPlannerReader: base,
		PortfolioOperator:   optional, Operator: base,
	}
}
