package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/facets"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// GetThreadFacets reads bounded facet coverage and resource identities from the
// local corpus. It never decodes or fetches large payloads in the tool result.
func (r *MCPReader) GetThreadFacets(ctx context.Context, in mcpcontract.GetThreadFacetsInput) (mcpcontract.GetThreadFacetsOutput, error) {
	if len(in.Threads) < 1 || len(in.Threads) > 100 {
		return mcpcontract.GetThreadFacetsOutput{}, errors.New("threads must contain 1 to 100 items")
	}
	if len(in.Facets) < 1 || len(in.Facets) > 10 {
		return mcpcontract.GetThreadFacetsOutput{}, errors.New("facets must contain 1 to 10 items")
	}
	if err := validateFacetNames(in.Facets); err != nil {
		return mcpcontract.GetThreadFacetsOutput{}, err
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.GetThreadFacetsOutput{}, err
	}
	revision, err := beginCorpusRead(ctx, c, in.SnapshotToken)
	if err != nil {
		return mcpcontract.GetThreadFacetsOutput{}, err
	}
	out := mcpcontract.GetThreadFacetsOutput{Status: "complete", Items: make([]mcpcontract.BatchItem[mcpcontract.ThreadFacetsOutput], len(in.Threads)), SnapshotToken: snapshotIdentity(in.SnapshotToken, revision)}
	parsed := make([]*parsedThreadReference, len(in.Threads))
	repositoryKeys := make([]corpus.RepositoryKey, 0, len(in.Threads))
	for i, input := range in.Threads {
		ref, parseErr := parseThreadReference(input)
		if parseErr != nil {
			continue
		}
		parsed[i] = &ref
		repositoryKeys = append(repositoryKeys, ref.repositoryKey())
	}
	repositories, err := c.GetRepositoriesBatch(ctx, repositoryKeys)
	if err != nil {
		return mcpcontract.GetThreadFacetsOutput{}, err
	}
	threadKeys := make([]corpus.ThreadKey, 0, len(in.Threads))
	for _, ref := range parsed {
		if ref == nil {
			continue
		}
		if repo := repositories[ref.repositoryKey()]; repo != nil {
			threadKeys = append(threadKeys, ref.threadKey(repo.ID))
		}
	}
	threads, err := c.GetThreadsBatch(ctx, threadKeys)
	if err != nil {
		return mcpcontract.GetThreadFacetsOutput{}, err
	}
	threadIDs := make([]int64, 0, len(threads))
	for _, thread := range threads {
		threadIDs = append(threadIDs, thread.ID)
	}
	coverage, err := c.ListThreadCoverageBatch(ctx, threadIDs, in.Facets)
	if err != nil {
		return mcpcontract.GetThreadFacetsOutput{}, err
	}
	observations, err := c.ListThreadFacetObservationsBatch(ctx, threadIDs, in.Facets, 100)
	if err != nil {
		return mcpcontract.GetThreadFacetsOutput{}, err
	}
	for i, input := range in.Threads {
		item := mcpcontract.BatchItem[mcpcontract.ThreadFacetsOutput]{Key: threadRefKey(input), Status: "complete"}
		if parsed[i] == nil {
			item.Status, item.Reason, item.Message = "failed", "blocked", "invalid thread reference"
			out.Status = "partial"
			out.Items[i] = item
			continue
		}
		ref := *parsed[i]
		wire := ref.wire()
		item.Key = threadRefKey(wire)
		repo := repositories[ref.repositoryKey()]
		if repo == nil {
			item.Status, item.Reason, item.Message = "unavailable", "repository_not_indexed", "repository is not present in the local corpus"
			item.Recovery = recoveryPlan("repository_not_indexed", item.Message, syncRepositoryContextCall(ref.repository.Owner(), ref.repository.Repo()))
			out.Status = "partial"
			out.Items[i] = item
			continue
		}
		thread := threads[ref.threadKey(repo.ID)]
		if thread == nil {
			item.Status, item.Reason, item.Message = "unavailable", "thread_not_indexed", "thread is not present in the local corpus"
			item.Recovery = recoveryPlan("thread_not_indexed", item.Message, syncThreadCall(wire))
			out.Status = "partial"
			out.Items[i] = item
			continue
		}
		value := mcpcontract.ThreadFacetsOutput{Owner: ref.repository.Owner(), Repo: ref.repository.Repo(), Kind: string(thread.Kind), Number: thread.Number, Facets: make([]mcpcontract.ThreadFacetOutput, 0, len(in.Facets))}
		for _, facet := range in.Facets {
			key := corpus.ThreadFacetKey{ThreadID: thread.ID, Facet: facet}
			entry := mcpcontract.ThreadFacetOutput{Facet: facet, Status: "not_observed", ResourceURI: threadFacetURI(ref.repository.Owner(), ref.repository.Repo(), string(thread.Kind), thread.Number, facet)}
			if cov := coverage[key]; cov != nil {
				entry.Complete, entry.SourceUpdatedAt = cov.Complete, formatTime(cov.SourceUpdatedAt)
				entry.Status = "complete"
				if !cov.Complete {
					entry.Status = "incomplete"
				}
			}
			if batch, ok := observations[key]; ok {
				entry.ObservationCount = mcpcontract.NonNegativeInt(len(batch.Observations))
			}
			switch entry.Status {
			case "not_observed":
				entry.Recovery = recoveryPlan("facet_not_observed", "Synchronize this facet, then read its coverage again.", syncFacetCall(input, facet))
			case "incomplete":
				entry.Recovery = recoveryPlan("facet_incomplete", "Synchronize this facet again to complete its bounded observation set.", syncFacetCall(input, facet))
			}
			value.Facets = append(value.Facets, entry)
		}
		item.Value = &value
		out.Items[i] = item
	}
	if err := finishCorpusRead(ctx, c, revision); err != nil {
		return mcpcontract.GetThreadFacetsOutput{}, err
	}
	return out, nil
}

// ThreadFacetResource is the canonical offline payload read for one stored
// facet. Resource reads are intentionally separate from bounded tool output.
func (r *MCPReader) ThreadFacetResource(ctx context.Context, owner, repo, kind string, number int, facet string) (mcpcontract.ThreadFacetResource, error) {
	if err := validateFacetNames([]string{facet}); err != nil {
		return mcpcontract.ThreadFacetResource{}, err
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.ThreadFacetResource{}, err
	}
	storedRepo, err := c.GetRepository(ctx, owner, repo)
	if err != nil || storedRepo == nil {
		if err == nil {
			err = errors.New("repository is not stored")
		}
		return mcpcontract.ThreadFacetResource{}, err
	}
	parsedKind, err := domain.ParseThreadKind(kind)
	if err != nil {
		return mcpcontract.ThreadFacetResource{}, err
	}
	thread, err := c.GetThread(ctx, storedRepo.ID, parsedKind, number)
	if err != nil || thread == nil {
		if err == nil {
			err = errors.New("thread is not stored")
		}
		return mcpcontract.ThreadFacetResource{}, err
	}
	observations, _, err := c.ListFacetObservationsBounded(ctx, storedRepo.ID, &thread.ID, facet, 1000)
	if err != nil {
		return mcpcontract.ThreadFacetResource{}, err
	}
	coverage, err := c.GetCoverage(ctx, storedRepo.ID, &thread.ID, facet)
	if err != nil {
		return mcpcontract.ThreadFacetResource{}, err
	}
	out := mcpcontract.ThreadFacetResource{
		SchemaVersion: "gitcontribute.thread-facet.v1", Owner: owner, Repo: repo, Kind: string(thread.Kind), Number: number, Facet: facet,
		Observations: make([]mcpcontract.ThreadFacetObservationResource, 0, len(observations)),
	}
	for _, observation := range observations {
		payload := json.RawMessage(observation.Payload)
		if !json.Valid(payload) {
			return mcpcontract.ThreadFacetResource{}, fmt.Errorf("decode %s observation: invalid JSON", facet)
		}
		out.Observations = append(out.Observations, mcpcontract.ThreadFacetObservationResource{
			SourceUpdatedAt: formatTime(observation.SourceUpdatedAt), ObservationSequence: observation.ObservationSequence, Payload: payload,
		})
	}
	if coverage != nil {
		out.Coverage = &mcpcontract.ResourceCoverage{Complete: coverage.Complete, SourceUpdatedAt: formatTime(coverage.SourceUpdatedAt)}
	}
	return out, nil
}

func validateFacetNames(values []string) error {
	allowed := make(map[string]struct{}, len(facets.AllNames()))
	for _, name := range facets.AllNames() {
		allowed[name] = struct{}{}
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return errors.New("facet names must not be blank")
		}
		if _, ok := allowed[value]; !ok {
			return fmt.Errorf("unknown facet %q", value)
		}
		if _, ok := seen[value]; ok {
			return fmt.Errorf("duplicate facet %q", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func threadFacetURI(owner, repo, kind string, number int, facet string) string {
	return fmt.Sprintf("gitcontribute://thread/%s/%s/%s/%d/facet/%s", owner, repo, kind, number, facet)
}

func recoveryPlan(reason, message string, calls ...mcpcontract.ToolCall) *mcpcontract.RecoveryPlan {
	return &mcpcontract.RecoveryPlan{Version: mcpcontract.RecoveryPlanVersion, Reason: reason, Message: message, Then: append([]mcpcontract.ToolCall(nil), calls...)}
}

func syncRepositoryContextCall(owner, repo string) mcpcontract.ToolCall {
	return mcpcontract.RecoveryAction(mcpcontract.SyncRepositoryContextInput{
		Repositories: []mcpcontract.RepositoryRef{{Owner: owner, Repo: repo}},
	})
}

func syncThreadCall(ref mcpcontract.ThreadRef) mcpcontract.ToolCall {
	return mcpcontract.RecoveryAction(mcpcontract.SyncThreadsInput{
		Selection: "threads", Threads: []mcpcontract.ThreadRef{ref},
	})
}

func syncThreadFacetsCall(ref mcpcontract.ThreadRef, facetNames []string) mcpcontract.ToolCall {
	return mcpcontract.RecoveryAction(mcpcontract.HydrateThreadsInput{
		Threads: []mcpcontract.ThreadRef{ref}, Facets: append([]string(nil), facetNames...),
	})
}

func syncFacetCall(ref mcpcontract.ThreadRef, facetName string) mcpcontract.ToolCall {
	switch facetName {
	case facets.PRChecks, facets.PRReviewThreads, facets.PRMergeState, facets.PRMergeQueue, facets.PRClosingIssues, facets.PRFiles:
		return syncPullRequestCalls([]mcpcontract.ThreadRef{ref})[0]
	default:
		return syncThreadFacetsCall(ref, []string{facetName})
	}
}

func syncPullRequestCalls(refs []mcpcontract.ThreadRef) []mcpcontract.ToolCall {
	return []mcpcontract.ToolCall{mcpcontract.RecoveryAction(mcpcontract.SyncPortfolioInput{
		Selection: "explicit", PullRequests: append([]mcpcontract.ThreadRef(nil), refs...),
	})}
}

func threadRefKey(ref mcpcontract.ThreadRef) string {
	kind := ref.Kind
	if kind == "" {
		kind = "any"
	}
	return fmt.Sprintf("%s/%s/%s#%d", ref.Owner, ref.Repo, kind, ref.Number)
}
