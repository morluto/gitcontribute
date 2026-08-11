package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

const pullRequestStatusWorkers = 4

type pullRequestStatusBatchInput struct {
	PullRequests []mcpcontract.ThreadRef
	MaxPages     int
}

// syncPullRequestStatusBatch preserves input order and isolates failures by PR.
// It uses the existing REST hydration path for details and reviews, then one
// typed GraphQL read for health facets unavailable from REST.
func (s *Service) syncPullRequestStatusBatch(ctx context.Context, in pullRequestStatusBatchInput, report func(string, string) error) (pullRequestStatusBatchResult, error) {
	results := make([]pullRequestStatusItem, len(in.PullRequests))
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
			return pullRequestStatusBatchResult{}, ctx.Err()
		}
	}
	close(work)
	wg.Wait()

	completed := 0
	status := batchOperationComplete
	failures := make([]pullRequestStatusFailure, 0)
	for index, item := range results {
		if item.Status() == mcpcontract.BatchItemComplete {
			completed++
		} else {
			status = batchOperationPartial
			ref := in.PullRequests[index]
			if ref.Kind == "" {
				ref.Kind = string(domain.PullRequestKind)
			}
			failure := pullRequestStatusFailure{
				Reference: threadRefKey(ref),
				Status:    item.Status(),
				Reason:    item.Reason(),
				Message:   item.Message(),
			}
			if retryAfter := item.RetryAfterMS(); retryAfter != nil && *retryAfter > 0 {
				failure.RetryAfterMS = *retryAfter
			}
			failures = append(failures, failure)
		}
	}
	if err := report("pull_request_status", jobProgressCounts(len(results), len(results))); err != nil {
		return pullRequestStatusBatchResult{}, err
	}
	return pullRequestStatusBatchResult{Status: status, Items: results, Failures: failures, Completed: completed, Total: len(results)}, nil
}

type pullRequestStatusBatchResult struct {
	Status    batchOperationStatus       `json:"status"`
	Items     []pullRequestStatusItem    `json:"items"`
	Failures  []pullRequestStatusFailure `json:"failures,omitempty"`
	Completed int                        `json:"completed"`
	Total     int                        `json:"total"`
}

type pullRequestStatusItem struct {
	key      string
	status   mcpcontract.BatchItemStatus
	snapshot *pullRequestStatusSnapshot
	failure  *pullRequestStatusItemFailure
}

type pullRequestStatusSnapshot struct {
	reason   string
	recovery *mcpcontract.RecoveryPlan
	facets   []pullRequestHealthFacet
	headSHA  string
}

type pullRequestStatusItemFailure struct {
	reason       string
	message      string
	retryAfterMS *int
	recovery     *mcpcontract.RecoveryPlan
}

func pullRequestStatusSnapshotItem(key string, status mcpcontract.BatchItemStatus, reason string, recovery *mcpcontract.RecoveryPlan, facets []pullRequestHealthFacet, headSHA string) pullRequestStatusItem {
	return pullRequestStatusItem{
		key: key, status: status,
		snapshot: &pullRequestStatusSnapshot{reason: reason, recovery: recovery, facets: facets, headSHA: headSHA},
	}
}

func pullRequestStatusFailureItem(key string, status mcpcontract.BatchItemStatus, reason, message string, retryAfterMS *int, recovery *mcpcontract.RecoveryPlan) pullRequestStatusItem {
	return pullRequestStatusItem{
		key: key, status: status,
		failure: &pullRequestStatusItemFailure{reason: reason, message: message, retryAfterMS: retryAfterMS, recovery: recovery},
	}
}

func (i pullRequestStatusItem) Status() mcpcontract.BatchItemStatus { return i.status }

func (i pullRequestStatusItem) Reason() string {
	if i.snapshot != nil {
		return i.snapshot.reason
	}
	if i.failure != nil {
		return i.failure.reason
	}
	return ""
}

func (i pullRequestStatusItem) Message() string {
	if i.failure == nil {
		return ""
	}
	return i.failure.message
}

func (i pullRequestStatusItem) RetryAfterMS() *int {
	if i.failure == nil {
		return nil
	}
	return i.failure.retryAfterMS
}

func (i pullRequestStatusItem) MarshalJSON() ([]byte, error) {
	if i.snapshot != nil && i.failure == nil {
		return json.Marshal(struct {
			Key      string                      `json:"key"`
			Status   mcpcontract.BatchItemStatus `json:"status"`
			Reason   string                      `json:"reason,omitempty"`
			Recovery *mcpcontract.RecoveryPlan   `json:"recovery,omitempty"`
			Facets   []pullRequestHealthFacet    `json:"facets"`
			HeadSHA  string                      `json:"head_sha"`
		}{i.key, i.status, i.snapshot.reason, i.snapshot.recovery, i.snapshot.facets, i.snapshot.headSHA})
	}
	if i.failure != nil && i.snapshot == nil {
		return json.Marshal(struct {
			Key          string                      `json:"key"`
			Status       mcpcontract.BatchItemStatus `json:"status"`
			Reason       string                      `json:"reason,omitempty"`
			Message      string                      `json:"message,omitempty"`
			RetryAfterMS *int                        `json:"retry_after_ms,omitempty"`
			Recovery     *mcpcontract.RecoveryPlan   `json:"recovery,omitempty"`
		}{i.key, i.status, i.failure.reason, i.failure.message, i.failure.retryAfterMS, i.failure.recovery})
	}
	return nil, errors.New("pull request status item has no single outcome")
}

type pullRequestHealthFacet struct {
	Facet    string                      `json:"facet"`
	Status   mcpcontract.BatchItemStatus `json:"status"`
	Complete bool                        `json:"complete"`
	Fetched  int                         `json:"fetched"`
	Total    *int                        `json:"total,omitempty"`
	Pages    *int                        `json:"pages,omitempty"`
	Recovery *mcpcontract.RecoveryPlan   `json:"recovery,omitempty"`
}

type pullRequestStatusFailure struct {
	Reference    string                      `json:"reference"`
	Status       mcpcontract.BatchItemStatus `json:"status"`
	Reason       string                      `json:"reason,omitempty"`
	Message      string                      `json:"message,omitempty"`
	RetryAfterMS int                         `json:"retry_after_ms,omitempty"`
}

func (s *Service) syncOnePullRequestStatus(ctx context.Context, ref mcpcontract.ThreadRef, maxPages int) pullRequestStatusItem {
	if ref.Number <= 0 {
		return pullRequestStatusFailureItem(threadRefKey(ref), mcpcontract.BatchItemFailed, "invalid_reference", "pull request number must be positive", nil, nil)
	}
	if ref.Kind == "" {
		ref.Kind = string(domain.PullRequestKind)
	}
	key := threadRefKey(ref)
	hydrated, err := s.hydrateStoredThread(ctx, contracts.RepoRef{Owner: ref.Owner, Repo: ref.Repo}, ref.Number, hydrateThreadInput{Kind: ref.Kind, Facets: []string{FacetPRDetails, FacetPRReviews}, MaxPages: maxPages})
	if err != nil {
		status, reason, message, retry := githubBatchError(err)
		return pullRequestStatusFailureItem(key, status, reason, message, intPointer(retry), nil)
	}
	reader, err := s.githubReader() //nolint:contextcheck // The typed read below receives ctx.
	if err != nil {
		return pullRequestStatusFailureItem(key, mcpcontract.BatchItemFailed, "github_unavailable", err.Error(), nil, nil)
	}
	statusReader, ok := reader.(github.PullRequestStatusReader)
	if !ok {
		message := "Configure a GitHub reader with pull-request status support."
		return pullRequestStatusFailureItem(key, mcpcontract.BatchItemUnavailable, "blocked", message, nil, recoveryPlan("blocked", message))
	}
	baselines, err := s.pullRequestHealthBaselines(ctx, ref)
	if err != nil {
		return pullRequestStatusFailureItem(key, mcpcontract.BatchItemFailed, "read_status_baseline_failed", err.Error(), nil, nil)
	}
	remote, err := statusReader.GetPullRequestStatus(ctx, ref.Owner, ref.Repo, ref.Number, github.PullRequestStatusOptions{PageSize: 100, MaxPages: maxPages})
	if err != nil {
		itemStatus, reason, message, retry := githubBatchError(err)
		return pullRequestStatusFailureItem(key, itemStatus, reason, message, intPointer(retry), nil)
	}
	facets, err := s.persistPullRequestHealth(ctx, ref, remote, hydrated.Facets, baselines)
	if err != nil {
		return pullRequestStatusFailureItem(key, mcpcontract.BatchItemFailed, "persist_status_failed", err.Error(), nil, nil)
	}
	itemStatus := mcpcontract.BatchItemComplete
	for _, facet := range facets {
		if facet.Status != mcpcontract.BatchItemComplete {
			itemStatus = mcpcontract.BatchItemRetryable
			break
		}
	}
	var reason string
	var recovery *mcpcontract.RecoveryPlan
	if itemStatus == mcpcontract.BatchItemRetryable {
		reason = "facet_incomplete"
		recovery = recoveryPlan("facet_incomplete", "Retry this pull request in explicit mode to complete its facets.", syncPullRequestCalls([]mcpcontract.ThreadRef{ref})...)
	}
	return pullRequestStatusSnapshotItem(key, itemStatus, reason, recovery, facets, remote.HeadSHA)
}

func (s *Service) persistPullRequestHealth(ctx context.Context, ref mcpcontract.ThreadRef, remote github.PullRequestStatus, hydrated []contracts.HydratedFacet, baselines map[string]int64) ([]pullRequestHealthFacet, error) {
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
	thread, err := c.GetThread(ctx, repo.ID, domain.PullRequestKind, ref.Number)
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
	targets, err := parseHealthFacets(remote)
	if err != nil {
		return nil, err
	}
	results := hydratedHealthResults(hydrated, mcpcontract.ThreadRef{Owner: repo.Owner, Repo: repo.Name, Kind: string(thread.Kind), Number: thread.Number})
	for _, target := range targets {
		result, err := persistOneHealthFacet(ctx, c, *repo, *thread, ref, sourceUpdatedAt, target, baselines[target.name], remote)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func hydratedHealthResults(facets []contracts.HydratedFacet, ref mcpcontract.ThreadRef) []pullRequestHealthFacet {
	results := make([]pullRequestHealthFacet, 0, len(facets))
	for _, facet := range facets {
		result := pullRequestHealthFacet{Facet: facet.Facet, Status: mcpcontract.BatchItemComplete, Complete: facet.Complete, Fetched: facet.Count, Pages: intPointer(facet.Pages)}
		if !facet.Complete {
			result.Status = mcpcontract.BatchItemRetryable
			result.Recovery = recoveryPlan("facet_incomplete", "Retry this pull request with a larger max_pages bound.", syncPullRequestCalls([]mcpcontract.ThreadRef{ref})...)
		}
		results = append(results, result)
	}
	return results
}

func persistOneHealthFacet(ctx context.Context, c *corpus.Corpus, repo corpus.Repository, thread corpus.Thread, ref mcpcontract.ThreadRef, sourceUpdatedAt time.Time, target healthFacet, baseline int64, remote github.PullRequestStatus) (pullRequestHealthFacet, error) {
	applied, err := persistHealthFacet(ctx, c, repo.ID, thread.ID, sourceUpdatedAt, target, baseline)
	if err != nil {
		return pullRequestHealthFacet{}, err
	}
	if applied && target.portfolio != nil {
		if err := persistPortfolioSignals(ctx, c, thread, sourceUpdatedAt, target.name, *target.portfolio); err != nil {
			return pullRequestHealthFacet{}, err
		}
	}
	result := pullRequestHealthFacet{Facet: target.name, Status: mcpcontract.BatchItemComplete, Complete: target.coverage.Complete, Fetched: target.coverage.Fetched, Total: intPointer(target.coverage.Total)}
	if !applied {
		result.Status, result.Complete = mcpcontract.BatchItemRetryable, false
		result.Recovery = recoveryPlan("coverage_stale", "A concurrent refresh advanced this facet; retry for a coherent snapshot.", syncPullRequestCalls([]mcpcontract.ThreadRef{ref})...)
	}
	if _, known := remote.MergeState.Mergeability(); target.name == FacetPRMergeState && !known {
		result.Status = mcpcontract.BatchItemRetryable
		result.Recovery = recoveryPlan("facet_incomplete", "Retry after GitHub finishes computing mergeability.", syncPullRequestCalls([]mcpcontract.ThreadRef{ref})...)
	}
	if !target.coverage.Complete {
		result.Status = mcpcontract.BatchItemRetryable
		result.Recovery = recoveryPlan("facet_incomplete", "Retry this pull request to complete the facet after the current cursor.", syncPullRequestCalls([]mcpcontract.ThreadRef{ref})...)
	}
	return result, nil
}

type portfolioSignalProjection struct {
	facet   string
	signals []corpus.PortfolioSignal
}

func persistPortfolioSignals(ctx context.Context, c *corpus.Corpus, thread corpus.Thread, sourceUpdatedAt time.Time, sourceFacet string, projection portfolioSignalProjection) error {
	observations, _, err := c.ListFacetObservationsBounded(ctx, thread.RepositoryID, &thread.ID, sourceFacet, 1)
	if err != nil {
		return err
	}
	if len(observations) == 0 {
		return fmt.Errorf("complete %s facet has no source observation", sourceFacet)
	}
	subject, err := corpus.NewPullRequestPortfolioSubject(thread.ID)
	if err != nil {
		return err
	}
	sourceRef, err := corpus.NewFacetObservationRef(observations[0].ID)
	if err != nil {
		return err
	}
	_, err = c.ReplacePortfolioSignals(ctx, corpus.PortfolioSignalSnapshot{
		Subject:               subject,
		Facet:                 projection.facet,
		Signals:               projection.signals,
		SourceUpdatedAt:       sourceUpdatedAt,
		SourceObservationRefs: []corpus.ObservationRef{sourceRef},
	})
	return err
}

type healthFacet struct {
	name      string
	payload   json.RawMessage
	coverage  github.FacetCoverage
	portfolio *portfolioSignalProjection
}

func newHealthFacet[T any](name string, value T, coverage github.FacetCoverage, portfolio *portfolioSignalProjection) (healthFacet, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return healthFacet{}, fmt.Errorf("marshal %s: %w", name, err)
	}
	return healthFacet{name: name, payload: payload, coverage: coverage, portfolio: portfolio}, nil
}

func appendHealthFacet[T any](targets []healthFacet, name string, value T, coverage github.FacetCoverage, portfolio *portfolioSignalProjection) ([]healthFacet, error) {
	target, err := newHealthFacet(name, value, coverage, portfolio)
	if err != nil {
		return nil, err
	}
	return append(targets, target), nil
}

func parseHealthFacets(remote github.PullRequestStatus) ([]healthFacet, error) {
	var files, issues *portfolioSignalProjection
	if remote.Files.Coverage.Complete {
		parsed, err := parseFilePortfolioSignals(remote.Files.Items)
		if err != nil {
			return nil, err
		}
		files = &parsed
	}
	if remote.ClosingIssues.Coverage.Complete {
		parsed, err := parseLinkedIssuePortfolioSignals(remote.ClosingIssues.Items)
		if err != nil {
			return nil, err
		}
		issues = &parsed
	}
	targets := make([]healthFacet, 0, 6)
	var err error
	if targets, err = appendHealthFacet(targets, FacetPRMergeState, remote.MergeState, remote.MergeStateCoverage, nil); err != nil {
		return nil, err
	}
	if targets, err = appendHealthFacet(targets, FacetPRMergeQueue, remote.MergeQueue, remote.MergeQueueCoverage, nil); err != nil {
		return nil, err
	}
	if targets, err = appendHealthFacet(targets, FacetPRChecks, remote.Checks.Items, remote.Checks.Coverage, nil); err != nil {
		return nil, err
	}
	if targets, err = appendHealthFacet(targets, FacetPRReviewThreads, remote.ReviewThreads.Items, remote.ReviewThreads.Coverage, nil); err != nil {
		return nil, err
	}
	if targets, err = appendHealthFacet(targets, FacetPRClosingIssues, remote.ClosingIssues.Items, remote.ClosingIssues.Coverage, issues); err != nil {
		return nil, err
	}
	if targets, err = appendHealthFacet(targets, FacetPRFiles, remote.Files.Items, remote.Files.Coverage, files); err != nil {
		return nil, err
	}
	return targets, nil
}

func parseFilePortfolioSignals(files []github.PullRequestFile) (portfolioSignalProjection, error) {
	projection := portfolioSignalProjection{facet: corpus.PortfolioFacetChangedFiles, signals: make([]corpus.PortfolioSignal, 0, len(files))}
	for _, file := range files {
		signal, err := corpus.NewPortfolioFilePathSignal(file.Path)
		if err != nil {
			return portfolioSignalProjection{}, fmt.Errorf("parse changed file signal: %w", err)
		}
		projection.signals = append(projection.signals, signal)
	}
	return projection, nil
}

func parseLinkedIssuePortfolioSignals(issues []github.PullRequestClosingIssue) (portfolioSignalProjection, error) {
	projection := portfolioSignalProjection{facet: corpus.PortfolioFacetLinkedIssues, signals: make([]corpus.PortfolioSignal, 0, len(issues))}
	for _, issue := range issues {
		signal, err := corpus.NewPortfolioLinkedIssueSignal(fmt.Sprintf("%s#%d", issue.RepositoryFullName, issue.Number))
		if err != nil {
			return portfolioSignalProjection{}, fmt.Errorf("parse linked issue signal: %w", err)
		}
		projection.signals = append(projection.signals, signal)
	}
	return projection, nil
}

func persistHealthFacet(ctx context.Context, c *corpus.Corpus, repoID, threadID int64, sourceUpdatedAt time.Time, facet healthFacet, expectedSequence int64) (bool, error) {
	if !facet.coverage.Complete {
		// Preserve the last complete child snapshot while advancing explicit
		// incomplete coverage for this newer source revision.
		return c.AdvanceFacetCAS(ctx, repoID, &threadID, facet.name, sourceUpdatedAt, false, 0, expectedSequence)
	}
	pages := []corpus.FacetObservationInput{{SourceUpdatedAt: sourceUpdatedAt, Payload: string(facet.payload)}}
	return c.ApplyFacetObservationSetCAS(ctx, repoID, &threadID, facet.name, sourceUpdatedAt, pages, true, 0, expectedSequence)
}

func (s *Service) pullRequestHealthBaselines(ctx context.Context, ref mcpcontract.ThreadRef) (map[string]int64, error) {
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
	if ref.Kind == "" {
		ref.Kind = string(domain.PullRequestKind)
	}
	thread, err := c.GetThread(ctx, repo.ID, domain.PullRequestKind, ref.Number)
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
