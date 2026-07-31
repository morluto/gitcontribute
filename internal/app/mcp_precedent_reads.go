package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
	"github.com/morluto/gitcontribute/internal/precedent"
	"github.com/morluto/gitcontribute/internal/ranking"
	"github.com/morluto/gitcontribute/internal/similarity"
)

// FindPrecedents performs an offline similarity search over stored resolved threads.
// Each source thread is resolved and ranked independently while preserving
// input order and item-level recovery guidance in the bounded batch response.
//
//nolint:gocognit
func (r *MCPReader) FindPrecedents(ctx context.Context, in mcpcontract.FindPrecedentsInput) (mcpcontract.FindPrecedentsOutput, error) {
	if len(in.Threads) < 1 || len(in.Threads) > 20 {
		return mcpcontract.FindPrecedentsOutput{}, errors.New("threads must contain 1 to 20 items")
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpcontract.FindPrecedentsOutput{}, errors.New("limit must be between 1 and 100")
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.FindPrecedentsOutput{}, err
	}
	revision, err := beginCorpusRead(ctx, c, in.CorpusRevision)
	if err != nil {
		return mcpcontract.FindPrecedentsOutput{}, err
	}
	refs := make([]precedent.SourceRef, len(in.Threads))
	for i, input := range in.Threads {
		refs[i] = precedent.SourceRef{Repository: domain.RepoRef{Owner: input.Owner, Repo: input.Repo}, Number: input.Number}
	}
	snapshots, err := c.LoadPrecedentRepositories(ctx, refs, 2000)
	if err != nil {
		return mcpcontract.FindPrecedentsOutput{}, err
	}
	snapshotsByRepo := make(map[string]precedent.RepositorySnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		snapshotsByRepo[precedent.RepositoryKey(snapshot.Repository)] = snapshot
	}
	rule := similarity.DefaultPrecedentRule()
	preparedByRepo := make(map[string][]preparedPrecedent, len(snapshots))
	for _, snapshot := range snapshots {
		prepared := make([]preparedPrecedent, len(snapshot.Closed))
		for i, candidate := range snapshot.Closed {
			prepared[i] = preparedPrecedent{thread: candidate, text: rule.Prepare(candidate.Title + " " + candidate.Body)}
		}
		preparedByRepo[precedent.RepositoryKey(snapshot.Repository)] = prepared
	}
	out := mcpcontract.FindPrecedentsOutput{Status: "complete", Items: make([]mcpcontract.BatchItem[mcpcontract.PrecedentSet], len(in.Threads)), CorpusRevision: revision}
	for i, input := range in.Threads {
		if err := ctx.Err(); err != nil {
			return mcpcontract.FindPrecedentsOutput{}, err
		}
		key := threadRefKey(input)
		item := mcpcontract.BatchItem[mcpcontract.PrecedentSet]{Key: key, Status: "complete"}
		repoKey := precedent.RepositoryKey(refs[i].Repository)
		snapshot := snapshotsByRepo[repoKey]
		if !snapshot.Available {
			item.Status, item.Reason = "unavailable", "repository_not_indexed"
			out.Items[i] = item
			out.Status = "partial"
			continue
		}
		source, ok := snapshot.Sources[input.Number]
		if !ok {
			item.Status, item.Reason = "unavailable", "thread_not_indexed"
			out.Items[i] = item
			out.Status = "partial"
			continue
		}
		precedents := make([]mcpcontract.PrecedentOutput, 0, in.Limit)
		preparedSource := rule.Prepare(source.Title + " " + source.Body)
		for candidateIndex, prepared := range preparedByRepo[repoKey] {
			if candidateIndex%1024 == 0 {
				if err := ctx.Err(); err != nil {
					return mcpcontract.FindPrecedentsOutput{}, err
				}
			}
			candidate := prepared.thread
			if candidate.ID == source.ID {
				continue
			}
			score := rule.Compare(preparedSource, prepared.text)
			if score < 0.08 {
				continue
			}
			precedents = append(precedents, precedentToMCP(key, input.Owner, input.Repo, candidate, score))
		}
		qualifying := len(precedents)
		precedents = ranking.TopK(precedents, in.Limit, betterPrecedent)
		item.Value = &mcpcontract.PrecedentSet{Matches: precedents, Population: snapshot.ClosedTotal, Considered: len(snapshot.Closed), Truncated: snapshot.ClosedTruncated || len(precedents) < qualifying}
		if item.Value.Truncated {
			out.Status = "partial"
		}
		out.Total += len(precedents)
		out.Items[i] = item
	}
	if err := finishCorpusRead(ctx, c, revision); err != nil {
		return mcpcontract.FindPrecedentsOutput{}, err
	}
	truncated := false
	unknownCoverage := out.Status != "complete"
	for _, item := range out.Items {
		if item.Value == nil {
			unknownCoverage = true
			continue
		}
		truncated = truncated || item.Value.Truncated
	}
	out.Provenance, err = offlineReadProvenance("precedent_search", revision, in, !truncated && !unknownCoverage, truncated, unknownCoverage)
	if err != nil {
		return mcpcontract.FindPrecedentsOutput{}, err
	}
	return out, nil
}

type preparedPrecedent struct {
	thread precedent.Thread
	text   similarity.PreparedLexical
}

func betterPrecedent(a, b mcpcontract.PrecedentOutput) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	return a.Ref < b.Ref
}

func precedentToMCP(source, owner, repo string, t precedent.Thread, score float64) mcpcontract.PrecedentOutput {
	reasons := []string{"similar stored title or body"}
	if t.Merged {
		reasons = append(reasons, "pull request merged")
	}
	if t.StateReason != "" {
		reasons = append(reasons, "GitHub state reason: "+t.StateReason)
	}
	for _, label := range t.Labels {
		lower := strings.ToLower(label)
		if strings.Contains(lower, "duplicate") || strings.Contains(lower, "wontfix") || strings.Contains(lower, "invalid") {
			reasons = append(reasons, "label: "+label)
		}
	}
	return mcpcontract.PrecedentOutput{Source: source, Ref: fmt.Sprintf("%s/%s#%d", owner, repo, t.Number), Kind: t.Kind, State: t.State, StateReason: t.StateReason, Title: t.Title, Score: mcpcontract.SimilarityScore(score), RuleVersion: similarity.PrecedentV1, Reasons: reasons, ClosedAt: formatTime(t.ClosedAt), MergedAt: formatTime(t.MergedAt)}
}
