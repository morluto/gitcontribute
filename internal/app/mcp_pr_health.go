package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpserver"
)

const pullRequestStatusWorkers = 4

// syncPullRequestStatusBatch preserves input order and isolates failures by PR.
// It uses the existing REST hydration path for details and reviews, then one
// typed GraphQL read for health facets unavailable from REST.
func (s *Service) syncPullRequestStatusBatch(ctx context.Context, in mcpserver.SyncPullRequestStatusInput, report func(string, string) error) (map[string]any, error) {
	results := make([]map[string]any, len(in.PullRequests))
	work := make(chan int)
	workers := min(pullRequestStatusWorkers, len(in.PullRequests))
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range work {
				ref := in.PullRequests[index]
				results[index] = s.syncOnePullRequestStatus(ctx, ref, in.MaxPages)
			}
		}()
	}
	for index := range in.PullRequests {
		select {
		case work <- index:
		case <-ctx.Done():
			close(work)
			wg.Wait()
			return nil, ctx.Err()
		}
	}
	close(work)
	wg.Wait()

	completed := 0
	status := "complete"
	for _, item := range results {
		if item["status"] == "complete" {
			completed++
		} else {
			status = "partial"
		}
	}
	if err := report("pull_request_status", jobProgressCounts(len(results), len(results))); err != nil {
		return nil, err
	}
	return map[string]any{"status": status, "items": results, "completed": completed, "total": len(results)}, nil
}

func (s *Service) syncOnePullRequestStatus(ctx context.Context, ref mcpserver.ThreadRef, maxPages int) map[string]any {
	key := fmt.Sprintf("%s/%s#%d", ref.Owner, ref.Repo, ref.Number)
	if ref.Number <= 0 {
		return map[string]any{"key": key, "status": "failed", "reason": "invalid_reference", "message": "pull request number must be positive"}
	}
	hydrated, err := s.HydrateThread(ctx, cli.RepoRef{Owner: ref.Owner, Repo: ref.Repo}, ref.Number, HydrateOptions{Facets: []string{FacetPRDetails, FacetPRReviews}, MaxPages: maxPages})
	if err != nil {
		status, reason, message, retry := githubBatchError(err)
		return map[string]any{"key": key, "status": status, "reason": reason, "message": message, "retry_after_ms": retry}
	}
	reader, err := s.githubReader() //nolint:contextcheck // The typed read below receives ctx.
	if err != nil {
		return map[string]any{"key": key, "status": "failed", "reason": "github_unavailable", "message": err.Error()}
	}
	statusReader, ok := reader.(github.PullRequestStatusReader)
	if !ok {
		return map[string]any{"key": key, "status": "unavailable", "reason": "status_adapter_unavailable", "next_action": "Configure a GitHub reader with pull-request status support."}
	}
	baselines, err := s.pullRequestHealthBaselines(ctx, ref)
	if err != nil {
		return map[string]any{"key": key, "status": "failed", "reason": "read_status_baseline_failed", "message": err.Error()}
	}
	remote, err := statusReader.GetPullRequestStatus(ctx, ref.Owner, ref.Repo, ref.Number, github.PullRequestStatusOptions{PageSize: 100, MaxPages: maxPages})
	if err != nil {
		itemStatus, reason, message, retry := githubBatchError(err)
		return map[string]any{"key": key, "status": itemStatus, "reason": reason, "message": message, "retry_after_ms": retry}
	}
	facets, err := s.persistPullRequestHealth(ctx, ref, remote, hydrated.Facets, baselines)
	if err != nil {
		return map[string]any{"key": key, "status": "failed", "reason": "persist_status_failed", "message": err.Error()}
	}
	itemStatus := "complete"
	for _, facet := range facets {
		if facet["status"] != "complete" {
			itemStatus = "retryable"
			break
		}
	}
	item := map[string]any{"key": key, "status": itemStatus, "facets": facets, "head_sha": remote.HeadSHA}
	if itemStatus == "retryable" {
		item["reason"] = "facet_incomplete"
		item["next_action"] = "Retry github.sync_pull_request_status for this pull request."
	}
	return item
}

func (s *Service) persistPullRequestHealth(ctx context.Context, ref mcpserver.ThreadRef, remote github.PullRequestStatus, hydrated []HydratedFacet, baselines map[string]int64) ([]map[string]any, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	repo, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil || repo == nil {
		if err == nil {
			err = errors.New("repository is not stored")
		}
		return nil, err
	}
	thread, err := c.GetThreadByNumber(ctx, repo.ID, ref.Number)
	if err != nil || thread == nil {
		if err == nil {
			err = errors.New("pull request is not stored")
		}
		return nil, err
	}
	sourceUpdatedAt := remote.SourceUpdatedAt
	if sourceUpdatedAt.IsZero() {
		sourceUpdatedAt = thread.SourceUpdatedAt
	}
	targets := []healthFacet{
		{name: FacetPRMergeState, value: remote.MergeState, coverage: remote.MergeStateCoverage},
		{name: FacetPRMergeQueue, value: remote.MergeQueue, coverage: remote.MergeQueueCoverage},
		{name: FacetPRChecks, value: remote.Checks.Items, coverage: remote.Checks.Coverage},
		{name: FacetPRReviewThreads, value: remote.ReviewThreads.Items, coverage: remote.ReviewThreads.Coverage},
		{name: FacetPRClosingIssues, value: remote.ClosingIssues.Items, coverage: remote.ClosingIssues.Coverage},
		{name: FacetPRFiles, value: remote.Files.Items, coverage: remote.Files.Coverage},
	}
	results := hydratedHealthResults(hydrated)
	for _, target := range targets {
		result, err := persistOneHealthFacet(ctx, c, *repo, *thread, sourceUpdatedAt, target, baselines[target.name], remote)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func hydratedHealthResults(facets []HydratedFacet) []map[string]any {
	results := make([]map[string]any, 0, len(facets))
	for _, facet := range facets {
		result := map[string]any{"facet": facet.Facet, "status": "complete", "complete": facet.Complete, "fetched": facet.Count, "pages": facet.Pages}
		if !facet.Complete {
			result["status"] = "retryable"
			result["next_action"] = "Retry this pull request with a larger max_pages bound."
		}
		results = append(results, result)
	}
	return results
}

func persistOneHealthFacet(ctx context.Context, c *corpus.Corpus, repo corpus.Repository, thread corpus.Thread, sourceUpdatedAt time.Time, target healthFacet, baseline int64, remote github.PullRequestStatus) (map[string]any, error) {
	applied, err := persistHealthFacet(ctx, c, repo.ID, thread.ID, sourceUpdatedAt, target, baseline)
	if err != nil {
		return nil, err
	}
	if applied && target.coverage.Complete {
		if err := persistPortfolioSignals(ctx, c, thread, sourceUpdatedAt, target); err != nil {
			return nil, err
		}
	}
	result := map[string]any{"facet": target.name, "complete": target.coverage.Complete, "fetched": target.coverage.Fetched, "total": target.coverage.Total, "status": "complete"}
	if !applied {
		result["status"], result["complete"] = "retryable", false
		result["next_action"] = "A concurrent refresh advanced this facet; retry for a coherent snapshot."
	}
	if target.name == FacetPRMergeState && !remote.MergeState.MergeableKnown {
		result["status"] = "retryable"
		result["next_action"] = "Retry after GitHub finishes computing mergeability."
	}
	if !target.coverage.Complete {
		result["status"] = "retryable"
		result["next_action"] = "Retry this pull request to complete the facet after the current cursor."
	}
	return result, nil
}

func persistPortfolioSignals(ctx context.Context, c *corpus.Corpus, thread corpus.Thread, sourceUpdatedAt time.Time, facet healthFacet) error {
	portfolioFacet := ""
	var signals []corpus.PortfolioSignal
	switch facet.name {
	case FacetPRFiles:
		portfolioFacet = corpus.PortfolioFacetChangedFiles
		files, ok := facet.value.([]github.PullRequestFile)
		if !ok {
			return fmt.Errorf("%s facet has unexpected value type %T", facet.name, facet.value)
		}
		for _, file := range files {
			signals = append(signals, corpus.PortfolioSignal{Kind: corpus.PortfolioSignalFilePath, Value: file.Path})
		}
	case FacetPRClosingIssues:
		portfolioFacet = corpus.PortfolioFacetLinkedIssues
		issues, ok := facet.value.([]github.PullRequestClosingIssue)
		if !ok {
			return fmt.Errorf("%s facet has unexpected value type %T", facet.name, facet.value)
		}
		for _, issue := range issues {
			signals = append(signals, corpus.PortfolioSignal{Kind: corpus.PortfolioSignalLinkedIssue, Value: fmt.Sprintf("%s#%d", issue.RepositoryFullName, issue.Number)})
		}
	default:
		return nil
	}
	observations, _, err := c.ListFacetObservationsBounded(ctx, thread.RepositoryID, &thread.ID, facet.name, 1)
	if err != nil {
		return err
	}
	if len(observations) == 0 {
		return fmt.Errorf("complete %s facet has no source observation", facet.name)
	}
	_, err = c.ReplacePortfolioSignals(ctx, corpus.PortfolioSignalSnapshot{
		Subject:               corpus.PortfolioSubject{Kind: corpus.PortfolioSubjectPullRequest, Ref: strconv.FormatInt(thread.ID, 10)},
		Facet:                 portfolioFacet,
		Signals:               signals,
		SourceUpdatedAt:       sourceUpdatedAt,
		SourceObservationRefs: []corpus.ObservationRef{{Kind: "facet", ID: observations[0].ID}},
	})
	return err
}

type healthFacet struct {
	name     string
	value    any
	coverage github.FacetCoverage
}

func persistHealthFacet(ctx context.Context, c *corpus.Corpus, repoID, threadID int64, sourceUpdatedAt time.Time, facet healthFacet, expectedSequence int64) (bool, error) {
	if !facet.coverage.Complete {
		// Preserve the last complete child snapshot while advancing explicit
		// incomplete coverage for this newer source revision.
		return c.AdvanceFacetCAS(ctx, repoID, &threadID, facet.name, sourceUpdatedAt, false, 0, expectedSequence)
	}
	payload, err := json.Marshal(facet.value)
	if err != nil {
		return false, fmt.Errorf("marshal %s: %w", facet.name, err)
	}
	pages := []corpus.FacetObservationInput{{SourceUpdatedAt: sourceUpdatedAt, Payload: string(payload)}}
	return c.ApplyFacetObservationSetCAS(ctx, repoID, &threadID, facet.name, sourceUpdatedAt, pages, true, 0, expectedSequence)
}

func (s *Service) pullRequestHealthBaselines(ctx context.Context, ref mcpserver.ThreadRef) (map[string]int64, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	repo, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil || repo == nil {
		if err == nil {
			err = errors.New("repository is not stored")
		}
		return nil, err
	}
	thread, err := c.GetThreadByNumber(ctx, repo.ID, ref.Number)
	if err != nil || thread == nil {
		if err == nil {
			err = errors.New("pull request is not stored")
		}
		return nil, err
	}
	baselines := make(map[string]int64, 6)
	for _, facet := range []string{FacetPRMergeState, FacetPRMergeQueue, FacetPRChecks, FacetPRReviewThreads, FacetPRClosingIssues, FacetPRFiles} {
		coverage, err := c.GetCoverage(ctx, repo.ID, &thread.ID, facet)
		if err != nil {
			return nil, err
		}
		if coverage != nil {
			baselines[facet] = coverage.ObservationSequence
		}
	}
	return baselines, nil
}
