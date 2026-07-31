package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// FindPortfolioOverlaps is a pure corpus read over complete normalized signal
// snapshots. It preserves candidate order and never treats absent coverage as
// proof that no overlap exists.
func (r *MCPReader) FindPortfolioOverlaps(ctx context.Context, in mcpcontract.FindPortfolioOverlapsInput) (mcpcontract.FindPortfolioOverlapsOutput, error) {
	if len(in.Candidates) < 1 || len(in.Candidates) > 50 {
		return mcpcontract.FindPortfolioOverlapsOutput{}, errors.New("candidates must contain 1 to 50 items")
	}
	if len(in.PullRequests) < 1 || len(in.PullRequests) > 100 {
		return mcpcontract.FindPortfolioOverlapsOutput{}, errors.New("pull_requests must contain 1 to 100 items")
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.FindPortfolioOverlapsOutput{}, err
	}
	out := mcpcontract.FindPortfolioOverlapsOutput{Status: "complete", Items: make([]mcpcontract.BatchItem[mcpcontract.PortfolioOverlapOutput], len(in.Candidates))}
	candidates, candidateIndexes := collectPortfolioCandidates(in.Candidates, &out)
	pullRequests := append([]mcpcontract.ThreadRef(nil), in.PullRequests...)
	for i := range pullRequests {
		if pullRequests[i].Kind == "" {
			pullRequests[i].Kind = corpus.ThreadKindPullRequest
		}
	}
	prIDs, missingPullRequests, err := resolvePortfolioPullRequests(ctx, c, pullRequests)
	if err != nil {
		return mcpcontract.FindPortfolioOverlapsOutput{}, err
	}
	if len(candidates) == 0 {
		return out, nil
	}
	if len(prIDs) == 0 {
		for _, index := range candidateIndexes {
			out.Items[index] = mcpcontract.BatchItem[mcpcontract.PortfolioOverlapOutput]{Key: in.Candidates[index].Kind + ":" + in.Candidates[index].Ref, Status: "unavailable", Reason: "thread_not_indexed", Message: "none of the requested pull requests are available in the local corpus", Recovery: recoveryPlan("thread_not_indexed", "Sync the exact authored pull requests, then retry this comparison.", syncPullRequestCalls(pullRequests)...)}
		}
		out.Status = "partial"
		return out, nil
	}
	results, err := c.FindPortfolioOverlaps(ctx, candidates, prIDs)
	if err != nil {
		return mcpcontract.FindPortfolioOverlapsOutput{}, err
	}
	for resultIndex, result := range results {
		i := candidateIndexes[resultIndex]
		value := portfolioOverlapOutput(result)
		batch := mcpcontract.BatchItem[mcpcontract.PortfolioOverlapOutput]{Key: result.Candidate.Kind + ":" + result.Candidate.Ref, Status: "complete", Value: &value}
		if missingPullRequests {
			out.Status = "partial"
			batch.Status, batch.Reason, batch.Recovery = "retryable", "thread_not_indexed", recoveryPlan("thread_not_indexed", "Sync the missing pull requests, then retry this comparison.", syncPullRequestCalls(pullRequests)...)
		} else if result.Status == "unknown" {
			out.Status = "partial"
			batch.Status, batch.Reason, batch.Recovery = "unavailable", "candidate_signal_unavailable", recoveryPlan("candidate_signal_unavailable", "Sync pull-request status and record candidate overlap signals before retrying.", syncPullRequestCalls(pullRequests)...)
		}
		out.Items[i] = batch
	}
	return out, nil
}

func collectPortfolioCandidates(inputs []mcpcontract.PortfolioSubjectInput, out *mcpcontract.FindPortfolioOverlapsOutput) ([]corpus.PortfolioSubject, []int) {
	var candidates []corpus.PortfolioSubject
	var indexes []int
	for i, candidate := range inputs {
		item := mcpcontract.BatchItem[mcpcontract.PortfolioOverlapOutput]{Key: candidate.Kind + ":" + candidate.Ref}
		if !validPortfolioSubjectInput(candidate) {
			item.Status, item.Reason, item.Message = "failed", "invalid_candidate", "kind must be opportunity, workspace, or pull_request and ref must be a valid local ID"
			out.Status, out.Items[i] = "partial", item
			continue
		}
		candidates = append(candidates, corpus.PortfolioSubject{Kind: candidate.Kind, Ref: strings.TrimSpace(candidate.Ref)})
		indexes = append(indexes, i)
	}
	return candidates, indexes
}

func resolvePortfolioPullRequests(ctx context.Context, c *corpus.Corpus, refs []mcpcontract.ThreadRef) ([]int64, bool, error) {
	var ids []int64
	missing := false
	for _, ref := range refs {
		repo, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
		if err != nil {
			return nil, false, err
		}
		if repo == nil {
			missing = true
			continue
		}
		thread, err := c.GetThread(ctx, repo.ID, ref.Kind, ref.Number)
		if err != nil {
			return nil, false, err
		}
		if thread == nil {
			missing = true
			continue
		}
		ids = append(ids, thread.ID)
	}
	return ids, missing, nil
}

func portfolioOverlapOutput(result corpus.PortfolioOverlapResult) mcpcontract.PortfolioOverlapOutput {
	value := mcpcontract.PortfolioOverlapOutput{Candidate: mcpcontract.PortfolioSubjectInput{Kind: result.Candidate.Kind, Ref: result.Candidate.Ref}, Status: result.Status, Coverage: result.Coverage}
	for _, match := range result.Matches {
		converted := mcpcontract.PortfolioOverlapMatchOutput{PullRequestThreadID: match.PullRequestThreadID}
		for _, evidence := range match.Evidence {
			item := mcpcontract.PortfolioOverlapEvidenceOutput{Kind: evidence.Kind, Value: evidence.Value, Score: mcpcontract.SimilarityScore(evidence.Score)}
			for _, ref := range evidence.SourceObservationRefs {
				item.SourceRefs = append(item.SourceRefs, ref.Kind+":"+strconv.FormatInt(ref.ID, 10))
			}
			converted.Evidence = append(converted.Evidence, item)
		}
		value.Matches = append(value.Matches, converted)
	}
	return value
}

func validPortfolioSubjectInput(candidate mcpcontract.PortfolioSubjectInput) bool {
	ref := strings.TrimSpace(candidate.Ref)
	if ref == "" {
		return false
	}
	switch candidate.Kind {
	case corpus.PortfolioSubjectOpportunity, corpus.PortfolioSubjectWorkspace:
		return true
	case corpus.PortfolioSubjectPullRequest:
		id, err := strconv.ParseInt(ref, 10, 64)
		return err == nil && id > 0
	default:
		return false
	}
}

// LinkPullRequest records an explicit local relationship without mutating GitHub.
func (r *MCPReader) LinkPullRequest(ctx context.Context, in mcpcontract.LinkPullRequestInput) (mcpcontract.LinkPullRequestOutput, error) {
	c, err := r.openCorpus(ctx)
	if err != nil {
		return mcpcontract.LinkPullRequestOutput{}, err
	}
	thread, err := resolveStoredPullRequest(ctx, c, in.PullRequest)
	if err != nil {
		return mcpcontract.LinkPullRequestOutput{}, err
	}
	link, err := c.SavePortfolioLink(ctx, corpus.PortfolioLink{PullRequestThreadID: thread.ID, OpportunityID: strings.TrimSpace(in.OpportunityID), WorkspaceID: strings.TrimSpace(in.WorkspaceID)})
	if err != nil {
		return mcpcontract.LinkPullRequestOutput{}, err
	}
	return mcpcontract.LinkPullRequestOutput{ID: link.ID, PullRequestThreadID: link.PullRequestThreadID, OpportunityID: link.OpportunityID, WorkspaceID: link.WorkspaceID, CreatedAt: formatTime(link.CreatedAt)}, nil
}

func resolveStoredPullRequest(ctx context.Context, c *corpus.Corpus, ref mcpcontract.ThreadRef) (*corpus.Thread, error) {
	repo, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return nil, err
	}
	if repo == nil {
		return nil, fmt.Errorf("repository %s/%s is not stored", ref.Owner, ref.Repo)
	}
	if ref.Kind == "" {
		ref.Kind = corpus.ThreadKindPullRequest
	}
	thread, err := c.GetThread(ctx, repo.ID, ref.Kind, ref.Number)
	if err != nil {
		return nil, err
	}
	if thread == nil {
		return nil, fmt.Errorf("pull request %s/%s#%d is not stored", ref.Owner, ref.Repo, ref.Number)
	}
	return thread, nil
}
