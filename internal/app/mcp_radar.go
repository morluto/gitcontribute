package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
	"github.com/morluto/gitcontribute/internal/radar"
)

// RankContributionCandidates projects the application-owned cross-repository
// Radar use case as one snapshot-bound offline MCP read.
func (r *MCPReader) RankContributionCandidates(ctx context.Context, in mcpcontract.RankContributionCandidatesInput) (mcpcontract.RankContributionCandidatesOutput, error) {
	if len(in.Repositories) < 1 || len(in.Repositories) > 50 {
		return mcpcontract.RankContributionCandidatesOutput{}, errors.New("repositories must contain 1 to 50 items")
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.MaxResultsPerRepository == 0 {
		in.MaxResultsPerRepository = 10
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpcontract.RankContributionCandidatesOutput{}, errors.New("limit must be between 1 and 100")
	}
	if in.MaxResultsPerRepository < 1 || in.MaxResultsPerRepository > 100 {
		return mcpcontract.RankContributionCandidatesOutput{}, errors.New("max_results_per_repository must be between 1 and 100")
	}
	repositories := make([]contracts.RepoRef, len(in.Repositories))
	seen := make(map[string]struct{}, len(in.Repositories))
	for index, input := range in.Repositories {
		ref, err := domain.NewRepoRef(input.Owner, input.Repo)
		if err != nil {
			return mcpcontract.RankContributionCandidatesOutput{}, fmt.Errorf("repositories[%d]: %w", index, err)
		}
		key := ref.String()
		if _, duplicate := seen[key]; duplicate {
			return mcpcontract.RankContributionCandidatesOutput{}, fmt.Errorf("repositories contains duplicate %s", key)
		}
		seen[key] = struct{}{}
		in.Repositories[index] = mcpcontract.RepositoryRef{Owner: ref.Owner(), Repo: ref.Repo()}
		repositories[index] = contracts.RepoRef{Owner: ref.Owner(), Repo: ref.Repo()}
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.RankContributionCandidatesOutput{}, err
	}
	revision, err := beginCorpusRead(ctx, c, in.SnapshotToken)
	if err != nil {
		return mcpcontract.RankContributionCandidatesOutput{}, err
	}
	result, err := r.application().RankContributionCandidates(ctx, contracts.RadarBatchOptions{Repositories: repositories, Limit: in.Limit, MaxResultsPerRepository: in.MaxResultsPerRepository})
	if err != nil {
		return mcpcontract.RankContributionCandidatesOutput{}, err
	}
	out := mcpcontract.RankContributionCandidatesOutput{
		Status: "complete", GeneratedAt: formatTime(result.GeneratedAt), Total: result.Ranking.Total, Truncated: result.Ranking.Truncated,
		SnapshotToken: snapshotIdentity(in.SnapshotToken, revision),
		Repositories:  make([]mcpcontract.BatchItem[mcpcontract.RepositoryCandidateRankingOutput], len(result.Repositories)),
		Candidates:    make([]mcpcontract.ContributionCandidateOutput, len(result.Ranking.Candidates)),
	}
	for index, item := range result.Repositories {
		key := item.Repository.Owner + "/" + item.Repository.Repo
		projected := mcpcontract.BatchItem[mcpcontract.RepositoryCandidateRankingOutput]{Key: key, Status: "complete"}
		if item.Err != nil {
			if !errors.Is(item.Err, errRepositoryNotFound) {
				return mcpcontract.RankContributionCandidatesOutput{}, item.Err
			}
			projected.Status, projected.Reason, projected.Message = "unavailable", "repository_not_indexed", item.Err.Error()
			projected.Recovery = recoveryPlan(projected.Reason, projected.Message, syncRepositoryContextCall(item.Repository.Owner, item.Repository.Repo), mcpcontract.RecoveryAction(mcpcontract.SyncThreadsInput{Selection: "repositories", Repositories: []mcpcontract.RepositoryRef{{Owner: item.Repository.Owner, Repo: item.Repository.Repo}}, Kind: "issue", State: "open"}))
			out.Repositories[index] = projected
			out.Status = "partial"
			continue
		}
		report := item.Report
		summary := mcpcontract.RepositoryCandidateRankingOutput{
			Repo: report.Repo, TotalOpenIssues: report.TotalOpenIssues, Considered: report.CandidatePopulation,
			Returned: len(report.Candidates), Truncated: len(report.Candidates) < report.CandidatePopulation, PopulationCapped: report.PopulationCapped,
		}
		if summary.Truncated || summary.PopulationCapped {
			projected.Status, projected.Reason, projected.Message = "partial", "ranking_population_truncated", "the repository ranking population exceeded the requested bound"
			out.Status = "partial"
			next := freshCandidateRankingInput(in)
			next.Repositories = []mcpcontract.RepositoryRef{{Owner: item.Repository.Owner, Repo: item.Repository.Repo}}
			next.MaxResultsPerRepository = min(100, max(in.MaxResultsPerRepository*2, in.MaxResultsPerRepository+1))
			summary.Recovery = recoveryPlan(projected.Reason, "Refresh issue headers, then rerun with a larger per-repository bound before treating the ranking as exhaustive.", mcpcontract.RecoveryAction(mcpcontract.SyncThreadsInput{Selection: "repositories", Repositories: next.Repositories, Kind: "issue", State: "open"}), mcpcontract.RecoveryAction(next))
		}
		projected.Value = &summary
		out.Repositories[index] = projected
	}
	for index, candidate := range result.Ranking.Candidates {
		out.Candidates[index] = radarCandidateToMCP(candidate)
	}
	if out.Truncated {
		out.Status = "partial"
		next := freshCandidateRankingInput(in)
		next.Limit = min(100, max(in.Limit*2, in.Limit+1))
		out.Recovery = recoveryPlan("ranking_truncated", "The cross-repository ranking is bounded. Refresh issue headers, then rerun with a larger result limit before treating the returned candidates as exhaustive.", mcpcontract.RecoveryAction(mcpcontract.SyncThreadsInput{Selection: "repositories", Repositories: append([]mcpcontract.RepositoryRef(nil), in.Repositories...), Kind: "issue", State: "open"}), mcpcontract.RecoveryAction(next))
	}
	if err := finishCorpusRead(ctx, c, revision); err != nil {
		return mcpcontract.RankContributionCandidatesOutput{}, err
	}
	out.Provenance, err = offlineReadProvenance("contribution_candidate_ranking", revision, in, out.Truncated, out.Status != "complete")
	if err != nil {
		return mcpcontract.RankContributionCandidatesOutput{}, err
	}
	out.Provenance.Recovery = out.Recovery
	return out, nil
}

func freshCandidateRankingInput(in mcpcontract.RankContributionCandidatesInput) mcpcontract.RankContributionCandidatesInput {
	in.SnapshotToken = ""
	return in
}

func radarCandidateToMCP(candidate radar.Candidate) mcpcontract.ContributionCandidateOutput {
	out := mcpcontract.ContributionCandidateOutput{Rank: candidate.Rank, Ref: candidate.Ref, Repo: candidate.Repo, Number: candidate.Number, Title: candidate.Title, URL: candidate.URL, Score: mcpcontract.RadarScore(candidate.Score), Eligibility: string(candidate.Eligibility), Confidence: candidate.Confidence, SourceUpdatedAt: formatTime(candidate.SourceUpdatedAt)}
	for _, signal := range candidate.PositiveSignals {
		out.PositiveSignals = append(out.PositiveSignals, signal.Summary)
	}
	for _, signal := range candidate.Risks {
		out.Risks = append(out.Risks, signal.Summary)
	}
	for _, signal := range candidate.Blockers {
		out.Blockers = append(out.Blockers, signal.Summary)
	}
	for _, unknown := range candidate.Unknowns {
		out.Unknowns = append(out.Unknowns, unknown.Summary)
	}
	for _, linked := range candidate.LinkedPullRequests {
		out.LinkedPullRequests = append(out.LinkedPullRequests, linked.Number)
	}
	for _, work := range candidate.RelatedWork {
		out.RelatedWork = append(out.RelatedWork, mcpcontract.OpportunityRelatedWorkOutput{Ref: work.Ref, Relation: string(work.Relation), Direction: string(work.Direction), State: work.State})
	}
	return out
}
