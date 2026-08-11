package app

import (
	"encoding/json"
	"errors"

	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

type batchOperationStatus string

const (
	batchOperationComplete batchOperationStatus = "complete"
	batchOperationPartial  batchOperationStatus = "partial"
	batchOperationFailed   batchOperationStatus = "failed"
)

type batchOperationSummary[T any] struct {
	Status    batchOperationStatus `json:"status"`
	Items     []T                  `json:"items"`
	Completed int                  `json:"completed"`
	Total     int                  `json:"total"`
}

type threadSyncBatchResult struct {
	batchOperationSummary[threadSyncItem]
	Requests        int `json:"requests"`
	RequestBudget   int `json:"request_budget"`
	PlannedRequests int `json:"planned_requests"`
}

type threadSyncOutcome interface {
	threadSyncOutcome()
	status() mcpcontract.BatchItemStatus
	requestsUsed() int
}

type threadSyncRepositorySuccess struct {
	updated       int
	requests      int
	requestCapped bool
	message       string
	threads       []mcpcontract.ThreadRef
}

func (threadSyncRepositorySuccess) threadSyncOutcome() {}
func (s threadSyncRepositorySuccess) status() mcpcontract.BatchItemStatus {
	if s.requestCapped {
		return mcpcontract.BatchItemPartial
	}
	return mcpcontract.BatchItemComplete
}
func (s threadSyncRepositorySuccess) requestsUsed() int { return s.requests }

type threadSyncExactSuccess struct {
	requestCapped bool
	message       string
	threads       []mcpcontract.ThreadRef
}

func (threadSyncExactSuccess) threadSyncOutcome() {}
func (s threadSyncExactSuccess) status() mcpcontract.BatchItemStatus {
	if s.requestCapped {
		return mcpcontract.BatchItemPartial
	}
	return mcpcontract.BatchItemComplete
}
func (threadSyncExactSuccess) requestsUsed() int { return 0 }

type threadSyncFailure struct {
	itemStatus   mcpcontract.BatchItemStatus
	reason       string
	message      string
	retryAfterMS *int
}

func (threadSyncFailure) threadSyncOutcome()                    {}
func (f threadSyncFailure) status() mcpcontract.BatchItemStatus { return f.itemStatus }
func (threadSyncFailure) requestsUsed() int                     { return 0 }

type threadSyncExactFailure struct {
	threadSyncFailure
	threads []mcpcontract.ThreadRef
}

type threadSyncItem struct {
	key     string
	outcome threadSyncOutcome
}

func successfulThreadSyncItem(key string, updated, requests int, requestCapped bool, message string, threads []mcpcontract.ThreadRef) threadSyncItem {
	return threadSyncItem{key: key, outcome: threadSyncRepositorySuccess{
		updated: updated, requests: requests, requestCapped: requestCapped, message: message, threads: threads,
	}}
}

func unavailableThreadSyncItem(key, reason, message string) threadSyncItem {
	return threadSyncItem{key: key, outcome: threadSyncFailure{itemStatus: mcpcontract.BatchItemUnavailable, reason: reason, message: message}}
}

func failedThreadSyncItem(key string, status mcpcontract.BatchItemStatus, reason, message string, retryAfterMS int) threadSyncItem {
	return threadSyncItem{key: key, outcome: threadSyncFailure{itemStatus: status, reason: reason, message: message, retryAfterMS: &retryAfterMS}}
}

func (i threadSyncItem) Status() mcpcontract.BatchItemStatus {
	if i.outcome == nil {
		return ""
	}
	return i.outcome.status()
}

func (i threadSyncItem) RequestsUsed() int {
	if i.outcome == nil {
		return 0
	}
	return i.outcome.requestsUsed()
}

func (i threadSyncItem) forExactThread(key string, fallback mcpcontract.ThreadRef) (threadSyncItem, error) {
	switch outcome := i.outcome.(type) {
	case threadSyncRepositorySuccess:
		return threadSyncItem{key: key, outcome: threadSyncExactSuccess{
			requestCapped: outcome.requestCapped, message: outcome.message, threads: outcome.threads,
		}}, nil
	case threadSyncFailure:
		return threadSyncItem{key: key, outcome: threadSyncExactFailure{
			threadSyncFailure: outcome, threads: []mcpcontract.ThreadRef{fallback},
		}}, nil
	default:
		return threadSyncItem{}, errors.New("thread sync item cannot be projected to an exact thread")
	}
}

func (i threadSyncItem) MarshalJSON() ([]byte, error) {
	switch outcome := i.outcome.(type) {
	case threadSyncRepositorySuccess:
		return json.Marshal(struct {
			Key           string                      `json:"key"`
			Status        mcpcontract.BatchItemStatus `json:"status"`
			Updated       int                         `json:"updated"`
			Requests      int                         `json:"requests"`
			RequestCapped bool                        `json:"request_capped"`
			Message       string                      `json:"message"`
			Threads       []mcpcontract.ThreadRef     `json:"threads"`
		}{i.key, outcome.status(), outcome.updated, outcome.requests, outcome.requestCapped, outcome.message, outcome.threads})
	case threadSyncExactSuccess:
		return json.Marshal(struct {
			Key           string                      `json:"key"`
			Status        mcpcontract.BatchItemStatus `json:"status"`
			RequestCapped bool                        `json:"request_capped"`
			Message       string                      `json:"message"`
			Threads       []mcpcontract.ThreadRef     `json:"threads"`
		}{i.key, outcome.status(), outcome.requestCapped, outcome.message, outcome.threads})
	case threadSyncFailure:
		return marshalThreadSyncFailure(i.key, outcome, nil)
	case threadSyncExactFailure:
		return marshalThreadSyncFailure(i.key, outcome.threadSyncFailure, outcome.threads)
	default:
		return nil, errors.New("thread sync item has no supported outcome")
	}
}

func marshalThreadSyncFailure(key string, failure threadSyncFailure, threads []mcpcontract.ThreadRef) ([]byte, error) {
	if failure.itemStatus == mcpcontract.BatchItemComplete || failure.itemStatus == mcpcontract.BatchItemPartial || failure.itemStatus == "" {
		return nil, errors.New("thread sync failure has a non-failure status")
	}
	if threads != nil {
		return json.Marshal(struct {
			Key          string                      `json:"key"`
			Status       mcpcontract.BatchItemStatus `json:"status"`
			Reason       string                      `json:"reason"`
			Message      string                      `json:"message"`
			RetryAfterMS *int                        `json:"retry_after_ms,omitempty"`
			Threads      []mcpcontract.ThreadRef     `json:"threads"`
		}{key, failure.itemStatus, failure.reason, failure.message, failure.retryAfterMS, threads})
	}
	return json.Marshal(struct {
		Key          string                      `json:"key"`
		Status       mcpcontract.BatchItemStatus `json:"status"`
		Reason       string                      `json:"reason"`
		Message      string                      `json:"message"`
		RetryAfterMS *int                        `json:"retry_after_ms,omitempty"`
	}{key, failure.itemStatus, failure.reason, failure.message, failure.retryAfterMS})
}

type threadHydrationBatchResult struct {
	Status    batchOperationStatus  `json:"status"`
	Items     []threadHydrationItem `json:"items"`
	Completed int                   `json:"completed"`
	Total     int                   `json:"total"`
}

type threadHydrationSuccess struct {
	kind     string
	requests int
	facets   []contracts.HydratedFacet
}

type threadHydrationFailure struct {
	reason       string
	message      string
	retryAfterMS int
}

type threadHydrationItem struct {
	key     string
	status  mcpcontract.BatchItemStatus
	success *threadHydrationSuccess
	failure *threadHydrationFailure
}

func completeThreadHydrationItem(key, kind string, requests int, facets []contracts.HydratedFacet) threadHydrationItem {
	return threadHydrationItem{
		key: key, status: mcpcontract.BatchItemComplete,
		success: &threadHydrationSuccess{kind: kind, requests: requests, facets: facets},
	}
}

func failedThreadHydrationItem(key string, status mcpcontract.BatchItemStatus, reason, message string, retryAfterMS int) threadHydrationItem {
	return threadHydrationItem{
		key: key, status: status,
		failure: &threadHydrationFailure{reason: reason, message: message, retryAfterMS: retryAfterMS},
	}
}

func (i threadHydrationItem) Status() mcpcontract.BatchItemStatus { return i.status }

func (i threadHydrationItem) Reason() string {
	if i.failure == nil {
		return ""
	}
	return i.failure.reason
}

func (i threadHydrationItem) Message() string {
	if i.failure == nil {
		return ""
	}
	return i.failure.message
}

func (i threadHydrationItem) MarshalJSON() ([]byte, error) {
	if i.success != nil && i.failure == nil {
		return json.Marshal(struct {
			Key             string                      `json:"key"`
			Status          mcpcontract.BatchItemStatus `json:"status"`
			Kind            string                      `json:"kind"`
			HeaderRefreshed bool                        `json:"header_refreshed"`
			Requests        int                         `json:"requests"`
			Facets          []contracts.HydratedFacet   `json:"facets"`
		}{i.key, i.status, i.success.kind, true, i.success.requests, i.success.facets})
	}
	if i.failure != nil && i.success == nil {
		return json.Marshal(struct {
			Key          string                      `json:"key"`
			Status       mcpcontract.BatchItemStatus `json:"status"`
			Reason       string                      `json:"reason"`
			Message      string                      `json:"message"`
			RetryAfterMS int                         `json:"retry_after_ms"`
		}{i.key, i.status, i.failure.reason, i.failure.message, i.failure.retryAfterMS})
	}
	return nil, errors.New("thread hydration item has no single outcome")
}

type repositoryIndexBatchResult struct {
	batchOperationSummary[repositoryIndexItem]
	SnapshotToken string `json:"snapshot_token"`
}

type repositoryIndexOutcome interface {
	repositoryIndexOutcome()
	status() mcpcontract.BatchItemStatus
}

type repositoryIndexSuccess struct{ result contracts.AcquisitionResult }

func (repositoryIndexSuccess) repositoryIndexOutcome() {}
func (repositoryIndexSuccess) status() mcpcontract.BatchItemStatus {
	return mcpcontract.BatchItemComplete
}

type repositoryIndexFailure struct {
	reason       string
	message      string
	retryAfterMS int
}

func (repositoryIndexFailure) repositoryIndexOutcome() {}
func (repositoryIndexFailure) status() mcpcontract.BatchItemStatus {
	return mcpcontract.BatchItemFailed
}

type repositoryIndexItem struct {
	key     string
	outcome repositoryIndexOutcome
}

func successfulRepositoryIndexItem(key string, result contracts.AcquisitionResult) repositoryIndexItem {
	return repositoryIndexItem{key: key, outcome: repositoryIndexSuccess{result: result}}
}

func failedRepositoryIndexItem(key, reason, message string, retryAfterMS int) repositoryIndexItem {
	return repositoryIndexItem{key: key, outcome: repositoryIndexFailure{reason: reason, message: message, retryAfterMS: retryAfterMS}}
}

func (i repositoryIndexItem) Status() mcpcontract.BatchItemStatus {
	if i.outcome == nil {
		return ""
	}
	return i.outcome.status()
}

func (i repositoryIndexItem) SnapshotToken() string {
	if outcome, ok := i.outcome.(repositoryIndexSuccess); ok {
		return outcome.result.SnapshotToken
	}
	return ""
}

func (i repositoryIndexItem) MarshalJSON() ([]byte, error) {
	switch outcome := i.outcome.(type) {
	case repositoryIndexSuccess:
		return json.Marshal(struct {
			Key            string                      `json:"key"`
			Status         mcpcontract.BatchItemStatus `json:"status"`
			CommitSHA      string                      `json:"commit_sha"`
			Files          int                         `json:"files"`
			Bytes          int                         `json:"bytes"`
			Inserted       bool                        `json:"inserted"`
			SnapshotToken  string                      `json:"snapshot_token"`
			IndexManifest  codeindex.Manifest          `json:"index_manifest"`
			ArtifactDigest string                      `json:"artifact_digest"`
			ManifestDigest string                      `json:"manifest_digest"`
		}{i.key, outcome.status(), outcome.result.CommitSHA, outcome.result.Files, outcome.result.Bytes, outcome.result.Inserted,
			outcome.result.SnapshotToken, outcome.result.IndexManifest, outcome.result.ArtifactDigest, outcome.result.ManifestDigest})
	case repositoryIndexFailure:
		return json.Marshal(struct {
			Key          string                      `json:"key"`
			Status       mcpcontract.BatchItemStatus `json:"status"`
			Reason       string                      `json:"reason"`
			Message      string                      `json:"message"`
			RetryAfterMS int                         `json:"retry_after_ms"`
		}{i.key, outcome.status(), outcome.reason, outcome.message, outcome.retryAfterMS})
	default:
		return nil, errors.New("repository index item has no supported outcome")
	}
}

type repositoryContextBatchResult struct {
	batchOperationSummary[repositoryContextItem]
	Requests        int `json:"requests"`
	RequestBudget   int `json:"request_budget"`
	PlannedRequests int `json:"planned_requests"`
}

type repositoryContextOutcome interface {
	repositoryContextOutcome()
	status() mcpcontract.BatchItemStatus
}

type repositoryContextSuccess struct {
	requests   int
	repository mcpcontract.RepositoryOutput
}

func (repositoryContextSuccess) repositoryContextOutcome() {}
func (repositoryContextSuccess) status() mcpcontract.BatchItemStatus {
	return mcpcontract.BatchItemComplete
}

type repositoryContextBudgetFailure struct {
	reason  string
	message string
}

func (repositoryContextBudgetFailure) repositoryContextOutcome() {}
func (repositoryContextBudgetFailure) status() mcpcontract.BatchItemStatus {
	return mcpcontract.BatchItemUnavailable
}

type repositoryContextRequestFailure struct {
	itemStatus   mcpcontract.BatchItemStatus
	reason       string
	message      string
	retryAfterMS int
	requests     int
}

func (repositoryContextRequestFailure) repositoryContextOutcome()             {}
func (f repositoryContextRequestFailure) status() mcpcontract.BatchItemStatus { return f.itemStatus }

type repositoryContextItem struct {
	key     string
	outcome repositoryContextOutcome
}

func successfulRepositoryContextItem(key string, requests int, repository mcpcontract.RepositoryOutput) repositoryContextItem {
	return repositoryContextItem{key: key, outcome: repositoryContextSuccess{requests: requests, repository: repository}}
}

func unavailableRepositoryContextItem(key, reason, message string) repositoryContextItem {
	return repositoryContextItem{key: key, outcome: repositoryContextBudgetFailure{reason: reason, message: message}}
}

func failedRepositoryContextItem(key string, status mcpcontract.BatchItemStatus, reason, message string, retryAfterMS, requests int) repositoryContextItem {
	return repositoryContextItem{key: key, outcome: repositoryContextRequestFailure{
		itemStatus: status, reason: reason, message: message, retryAfterMS: retryAfterMS, requests: requests,
	}}
}

func (i repositoryContextItem) Status() mcpcontract.BatchItemStatus {
	if i.outcome == nil {
		return ""
	}
	return i.outcome.status()
}

func (i repositoryContextItem) MarshalJSON() ([]byte, error) {
	switch outcome := i.outcome.(type) {
	case repositoryContextSuccess:
		type facet struct {
			Status mcpcontract.BatchItemStatus `json:"status"`
		}
		return json.Marshal(struct {
			Key        string                       `json:"key"`
			Status     mcpcontract.BatchItemStatus  `json:"status"`
			Requests   int                          `json:"requests"`
			Repository mcpcontract.RepositoryOutput `json:"repository"`
			Facets     struct {
				Metadata             facet `json:"metadata"`
				ContributionGuidance facet `json:"contribution_guidance"`
			} `json:"facets"`
		}{
			Key: i.key, Status: outcome.status(), Requests: outcome.requests, Repository: outcome.repository,
			Facets: struct {
				Metadata             facet `json:"metadata"`
				ContributionGuidance facet `json:"contribution_guidance"`
			}{Metadata: facet{Status: mcpcontract.BatchItemComplete}, ContributionGuidance: facet{Status: mcpcontract.BatchItemComplete}},
		})
	case repositoryContextBudgetFailure:
		return json.Marshal(struct {
			Key     string                      `json:"key"`
			Status  mcpcontract.BatchItemStatus `json:"status"`
			Reason  string                      `json:"reason"`
			Message string                      `json:"message"`
		}{i.key, outcome.status(), outcome.reason, outcome.message})
	case repositoryContextRequestFailure:
		if outcome.itemStatus == mcpcontract.BatchItemComplete || outcome.itemStatus == mcpcontract.BatchItemPartial || outcome.itemStatus == "" {
			return nil, errors.New("repository context failure has a non-failure status")
		}
		return json.Marshal(struct {
			Key          string                      `json:"key"`
			Status       mcpcontract.BatchItemStatus `json:"status"`
			Reason       string                      `json:"reason"`
			Message      string                      `json:"message"`
			RetryAfterMS int                         `json:"retry_after_ms"`
			Requests     int                         `json:"requests"`
		}{i.key, outcome.status(), outcome.reason, outcome.message, outcome.retryAfterMS, outcome.requests})
	default:
		return nil, errors.New("repository context item has no supported outcome")
	}
}

// SyncRepositoryContext submits a durable metadata and contribution-guidance
// GitHub read. It does not fetch threads, comments, reviews, or code.
