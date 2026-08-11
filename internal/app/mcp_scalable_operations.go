package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
	"github.com/morluto/gitcontribute/internal/repositorycontext"
)

func (r *MCPReader) SyncRepositoryContext(ctx context.Context, in mcpcontract.SyncRepositoryContextInput) (mcpcontract.JobReference, error) {
	request, canonical, err := parseRepositoryContextSyncInput(in)
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	id, err := r.submitJob(ctx, "sync_repository_context", canonical, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.syncRepositoryContext(ctx, request, report)
	})
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	return queuedJobReference(id, "sync_repository_context", "repository context sync job started"), nil
}

// SyncThreads submits a durable bounded GitHub read for thread headers in
// repositories that already have local identities.
func (r *MCPReader) SyncThreads(ctx context.Context, in mcpcontract.SyncThreadsInput) (mcpcontract.JobReference, error) {
	request, normalized, err := parseSyncThreadsInput(in)
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	id, err := r.submitJob(ctx, "sync_threads", normalized, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.syncThreadsBatch(ctx, request, report)
	})
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	return queuedJobReference(id, "sync_threads", "thread synchronization job started"), nil
}

// This function keeps bounded worker orchestration and ordered result assembly
// together so cancellation and per-item failures remain consistent.
//
//nolint:gocognit
func (s *Service) syncThreadsBatch(ctx context.Context, request syncThreadsRequest, report func(string, string) error) (*threadSyncBatchResult, error) {
	type task struct {
		key          string
		ref          domain.RepoRef
		kind         syncThreadKind
		numbers      []int
		inputIndexes []int
		maxRequests  int
	}
	var (
		tasks              []task
		exactThreads       []exactThreadTarget
		kind               = syncAllThreads
		state              = syncAllStates
		since              time.Time
		limitPerRepository int
	)
	switch selection := request.selection.(type) {
	case repositoryThreadSelection:
		kind, state, since, limitPerRepository = selection.kind, selection.state, selection.updatedAfter, selection.limitPerRepository
		for _, ref := range selection.repositories {
			tasks = append(tasks, task{key: ref.String(), ref: ref})
		}
	case exactThreadSelection:
		exactThreads = selection.threads
		grouped := make(map[string]int)
		for inputIndex, thread := range selection.threads {
			key := thread.repository.String() + "\x00" + thread.kind.String()
			index, ok := grouped[key]
			if !ok {
				grouped[key] = len(tasks)
				tasks = append(tasks, task{key: thread.repository.String() + "/" + thread.kind.String(), kind: thread.kind, ref: thread.repository})
				index = len(tasks) - 1
			}
			tasks[index].numbers = append(tasks[index].numbers, thread.number)
			tasks[index].inputIndexes = append(tasks[index].inputIndexes, inputIndex)
		}
	default:
		return nil, errors.New("unsupported parsed thread selection")
	}
	resultCount := len(tasks)
	if exactThreads != nil {
		resultCount = len(exactThreads)
	}
	if err := report("thread_headers", jobProgressCounts(0, resultCount)); err != nil {
		return nil, err
	}
	maxPages := 1
	if limitPerRepository > 100 {
		maxPages = (limitPerRepository + 99) / 100
	}
	taskResults := make([]threadSyncItem, len(tasks))
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	remainingRequests := request.maxRequests
	plannedRequests := 0
	runnable := make([]int, 0, len(tasks))
	for index := range tasks {
		stored, err := c.GetRepository(ctx, tasks[index].ref.Owner(), tasks[index].ref.Repo())
		if err != nil {
			return nil, err
		}
		if stored == nil {
			taskResults[index] = unavailableThreadSyncItem(tasks[index].key, "repository_not_indexed", "repository is not stored; call github.sync_repository_context first")
			continue
		}
		threadRequests := maxPages
		if len(tasks[index].numbers) > 0 {
			threadRequests = len(tasks[index].numbers)
			if threadRequests > remainingRequests {
				taskResults[index] = unavailableThreadSyncItem(tasks[index].key, "request_budget_exceeded", syncRequestBudgetMessage(threadRequests, remainingRequests))
				continue
			}
		} else if threadRequests > remainingRequests {
			threadRequests = remainingRequests
		}
		if threadRequests < 1 {
			taskResults[index] = unavailableThreadSyncItem(tasks[index].key, "request_budget_exceeded", syncRequestBudgetMessage(1, remainingRequests))
			continue
		}
		required := threadRequests
		tasks[index].maxRequests = required
		remainingRequests -= required
		plannedRequests += required
		runnable = append(runnable, index)
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	workers := 4
	if len(tasks) < workers {
		workers = len(tasks)
	}
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				current := tasks[index]
				currentKind := kind
				currentState := state
				currentSince := since
				if exactThreads != nil {
					currentKind = current.kind
				}
				if len(current.numbers) > 0 {
					currentState = syncAllStates
					currentSince = time.Time{}
				}
				threadRequest, plan, err := newThreadSyncRequest(currentKind, currentState, currentSince, current.numbers, limitPerRepository, maxPages, current.maxRequests)
				if err != nil {
					status, reason, message, retry := githubBatchError(err)
					taskResults[index] = failedThreadSyncItem(current.key, status, reason, message, retry)
					continue
				}
				res, err := s.executeThreadSync(ctx, current.ref, threadRequest, plan)
				if err != nil {
					status, reason, message, retry := githubBatchError(err)
					taskResults[index] = failedThreadSyncItem(current.key, status, reason, message, retry)
					continue
				}
				taskResults[index] = successfulThreadSyncItem(current.key, res.Updated, res.Requests, res.Capped, res.Message, syncThreadRefsToMCP(res.Threads))
			}
		}()
	}
	for _, i := range runnable {
		select {
		case jobs <- i:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return nil, ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	results := taskResults
	if exactThreads != nil {
		results = make([]threadSyncItem, len(exactThreads))
		for taskIndex, current := range tasks {
			for _, inputIndex := range current.inputIndexes {
				thread := exactThreads[inputIndex].wire()
				item, err := taskResults[taskIndex].forExactThread(threadRefKey(thread), thread)
				if err != nil {
					return nil, err
				}
				results[inputIndex] = item
			}
		}
	}
	status := batchOperationComplete
	completed := 0
	requests := 0
	for _, result := range taskResults {
		requests += result.RequestsUsed()
	}
	for _, result := range results {
		if result.Status() == mcpcontract.BatchItemComplete {
			completed++
		} else {
			status = batchOperationPartial
		}
	}
	if err := report("thread_headers", jobProgressCounts(resultCount, resultCount)); err != nil {
		return nil, err
	}
	return &threadSyncBatchResult{
		batchOperationSummary: batchOperationSummary[threadSyncItem]{Status: status, Items: results, Completed: completed, Total: resultCount},
		Requests:              requests, RequestBudget: request.maxRequests, PlannedRequests: plannedRequests,
	}, nil
}

// HydrateThreads submits a durable GitHub read for explicit child facets on
// selected threads; an empty facet set is rejected.
func (r *MCPReader) HydrateThreads(ctx context.Context, in mcpcontract.HydrateThreadsInput) (mcpcontract.JobReference, error) {
	if err := rejectDuplicateThreadRefs(in.Threads); err != nil {
		return mcpcontract.JobReference{}, err
	}
	if len(in.Threads) < 1 || len(in.Threads) > 100 {
		return mcpcontract.JobReference{}, errors.New("threads must contain 1 to 100 items")
	}
	if len(in.Facets) == 0 {
		return mcpcontract.JobReference{}, errors.New("facets must not be empty")
	}
	if in.MaxPages == 0 {
		in.MaxPages = 3
	}
	if in.MaxPages < 1 || in.MaxPages > 100 {
		return mcpcontract.JobReference{}, errors.New("max_pages must be between 1 and 100")
	}
	id, err := r.submitJob(ctx, jobKindSyncThreadFacets, in, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.hydrateThreadsBatch(ctx, in, report)
	})
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	return queuedJobReference(id, jobKindSyncThreadFacets, "thread facet synchronization job started"), nil
}

// IndexRepositories submits a durable Git acquisition and safe indexing job
// with at most two repositories processed concurrently.
func (r *MCPReader) IndexRepositories(ctx context.Context, in mcpcontract.IndexRepositoriesInput) (mcpcontract.JobReference, error) {
	if len(in.Repositories) < 1 || len(in.Repositories) > 10 {
		return mcpcontract.JobReference{}, errors.New("repositories must contain 1 to 10 items")
	}
	canonical := make([]mcpcontract.IndexRepositoryInput, len(in.Repositories))
	for i, input := range in.Repositories {
		ref, err := domain.NewRepoRef(input.Owner, input.Repo)
		if err != nil {
			return mcpcontract.JobReference{}, err
		}
		canonical[i] = mcpcontract.IndexRepositoryInput{Owner: ref.Owner(), Repo: ref.Repo(), Remote: strings.TrimSpace(input.Remote)}
	}
	in.Repositories = canonical
	if err := rejectDuplicateIndexRepositoryInputs(in.Repositories); err != nil {
		return mcpcontract.JobReference{}, err
	}
	id, err := r.submitJob(ctx, "index_repositories", in, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.indexRepositoriesBatch(ctx, in, report)
	})
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	return queuedJobReference(id, "index_repositories", "repository indexing job started"), nil
}

func queuedJobReference(id, kind, message string) mcpcontract.JobReference {
	return mcpcontract.JobReference{
		ID: id, Ref: "job:" + id, Kind: kind, Status: "queued", Message: message, PollAfterMS: 1000,
		FollowUp: &mcpcontract.JobFollowUp{
			Action: mcpcontract.FollowUpActionFor(mcpcontract.GetJobsInput{IDs: []string{id}}), RetryAfterMS: 1000, Reason: "Poll this job ID after the suggested delay.",
		},
	}
}

// CheckMergeConflicts compares already-fetched OIDs in managed workspaces
// without fetching or modifying refs, indexes, or worktrees.
func (r *MCPReader) CheckMergeConflicts(ctx context.Context, in mcpcontract.CheckMergeConflictsInput) (mcpcontract.CheckMergeConflictsOutput, error) {
	if len(in.Comparisons) < 1 || len(in.Comparisons) > 50 {
		return mcpcontract.CheckMergeConflictsOutput{}, errors.New("comparisons must contain 1 to 50 items")
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.CheckMergeConflictsOutput{}, err
	}
	manager, err := r.workspaceReader()
	if err != nil {
		return mcpcontract.CheckMergeConflictsOutput{}, err
	}
	out := mcpcontract.CheckMergeConflictsOutput{Status: "complete", Items: make([]mcpcontract.BatchItem[mcpcontract.MergeConflictOutput], len(in.Comparisons))}
	jobs := make(chan int)
	var wg sync.WaitGroup
	workers := 4
	if len(in.Comparisons) < workers {
		workers = len(in.Comparisons)
	}
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				current := in.Comparisons[index]
				key := current.WorkspaceID
				item := mcpcontract.BatchItem[mcpcontract.MergeConflictOutput]{Key: key, Status: "complete"}
				ws, err := c.GetWorkspace(ctx, current.WorkspaceID)
				if err != nil {
					item.Status, item.Reason, item.Message = "failed", "workspace_not_found", err.Error()
					item.Recovery = recoveryPlan("workspace_not_found", "Inspect or recreate the managed workspace, then retry this exact comparison.", mcpcontract.RecoveryAction(mcpcontract.InspectCommitChangesInput{WorkspaceID: current.WorkspaceID}), mcpcontract.RecoveryAction(mcpcontract.CheckMergeConflictsInput{Comparisons: []mcpcontract.MergeConflictInput{current}}))
					out.Items[index] = item
					continue
				}
				if current.BaseOID == "" {
					current.BaseOID = ws.BaseSHA
				}
				if current.HeadOID == "" {
					current.HeadOID = ws.CandidateSHA
				}
				item.Key = current.WorkspaceID + ":" + current.BaseOID + ".." + current.HeadOID
				if current.BaseOID == "" || current.HeadOID == "" {
					item.Status, item.Reason, item.Message = "unavailable", "missing_objects", "workspace does not record both base and head OIDs"
					item.Recovery = recoveryPlan("missing_objects", item.Message, mcpcontract.RecoveryAction(mcpcontract.InspectCommitChangesInput{WorkspaceID: current.WorkspaceID}), mcpcontract.RecoveryAction(mcpcontract.CheckMergeConflictsInput{Comparisons: []mcpcontract.MergeConflictInput{current}}))
					out.Items[index] = item
					continue
				}
				result, err := manager.CheckMergeWorkspace(ctx, ws, current.BaseOID, current.HeadOID)
				if err != nil {
					item.Status, item.Reason, item.Message = "failed", "merge_check_failed", err.Error()
					item.Recovery = recoveryPlan("merge_check_failed", "Retry the same comparison after inspecting the managed workspace state.", mcpcontract.RecoveryAction(mcpcontract.InspectCommitChangesInput{WorkspaceID: current.WorkspaceID}), mcpcontract.RecoveryAction(mcpcontract.CheckMergeConflictsInput{Comparisons: []mcpcontract.MergeConflictInput{current}}))
					out.Items[index] = item
					continue
				}
				value := mcpcontract.MergeConflictOutput{WorkspaceID: current.WorkspaceID, BaseOID: current.BaseOID, HeadOID: current.HeadOID, MergeBase: result.MergeBase, Conflicted: result.Conflicted, Summary: result.Summary}
				item.Value = &value
				out.Items[index] = item
			}
		}()
	}
	for i := range in.Comparisons {
		select {
		case jobs <- i:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return out, ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	for _, item := range out.Items {
		if item.Status != "complete" {
			out.Status = "partial"
			break
		}
	}
	return out, nil
}

func (s *Service) indexRepositoriesBatch(ctx context.Context, in mcpcontract.IndexRepositoriesInput, report func(string, string) error) (*repositoryIndexBatchResult, error) {
	if err := report("repository_indexing", jobProgressCounts(0, len(in.Repositories))); err != nil {
		return nil, err
	}
	results := make([]repositoryIndexItem, len(in.Repositories))
	jobs := make(chan int)
	var wg sync.WaitGroup
	workers := 2
	if len(in.Repositories) < workers {
		workers = len(in.Repositories)
	}
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				current := in.Repositories[index]
				key := current.Owner + "/" + current.Repo
				result, err := s.Acquire(ctx, contracts.RepoRef{Owner: current.Owner, Repo: current.Repo}, current.Remote)
				if err != nil {
					results[index] = failedRepositoryIndexItem(key, "acquisition_or_index_failed", err.Error(), 0)
					continue
				}
				results[index] = successfulRepositoryIndexItem(key, *result)
			}
		}()
	}
	for i := range in.Repositories {
		select {
		case jobs <- i:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return nil, ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	status := batchOperationComplete
	completed := 0
	for _, result := range results {
		if result.Status() == mcpcontract.BatchItemComplete {
			completed++
		} else {
			status = batchOperationPartial
		}
	}
	// Each completed acquisition owns its immutable artifact token. Do not
	// replace those scoped identities with a mutable database watermark.
	snapshotToken := ""
	for _, result := range results {
		if token := result.SnapshotToken(); token != "" && snapshotToken == "" {
			snapshotToken = token
		}
	}
	if err := report("repository_indexing", jobProgressCounts(len(in.Repositories), len(in.Repositories))); err != nil {
		return nil, err
	}
	return &repositoryIndexBatchResult{
		batchOperationSummary: batchOperationSummary[repositoryIndexItem]{Status: status, Items: results, Completed: completed, Total: len(in.Repositories)},
		SnapshotToken:         snapshotToken,
	}, nil
}

func syncThreadRefsToMCP(values []contracts.SyncThreadRef) []mcpcontract.ThreadRef {
	refs := make([]mcpcontract.ThreadRef, 0, len(values))
	for _, value := range values {
		refs = append(refs, mcpcontract.ThreadRef{Owner: value.Owner, Repo: value.Repo, Kind: value.Kind, Number: value.Number})
	}
	return refs
}

func (s *Service) hydrateThreadsBatch(ctx context.Context, in mcpcontract.HydrateThreadsInput, report func(string, string) error) (*threadHydrationBatchResult, error) {
	if err := report("thread_hydration", jobProgressCounts(0, len(in.Threads))); err != nil {
		return nil, err
	}
	results := make([]threadHydrationItem, len(in.Threads))
	jobs := make(chan int)
	var wg sync.WaitGroup
	workers := 4
	if len(in.Threads) < workers {
		workers = len(in.Threads)
	}
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				current := in.Threads[index]
				key := threadRefKey(current)
				res, err := s.Hydrate(ctx, contracts.RepoRef{Owner: current.Owner, Repo: current.Repo}, current.Number, contracts.HydrateOptions{Kind: current.Kind, Facets: in.Facets, MaxPages: in.MaxPages})
				if err != nil {
					status, reason, message, retry := githubBatchError(err)
					results[index] = failedThreadHydrationItem(key, status, reason, message, retry)
					continue
				}
				results[index] = completeThreadHydrationItem(key, res.Kind, res.Requests, res.Facets)
			}
		}()
	}
	for i := range in.Threads {
		select {
		case jobs <- i:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return nil, ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	status := batchOperationComplete
	completed := 0
	for _, result := range results {
		if result.Status() == mcpcontract.BatchItemComplete {
			completed++
		} else {
			status = batchOperationPartial
		}
	}
	if err := report("thread_hydration", jobProgressCounts(len(in.Threads), len(in.Threads))); err != nil {
		return nil, err
	}
	return &threadHydrationBatchResult{Status: status, Items: results, Completed: completed, Total: len(in.Threads)}, nil
}

// This bounded worker loop keeps each repository's fetch, persistence, and
// ordered result mapping in one place to preserve item-level failure semantics.
//
//nolint:gocognit
func (s *Service) syncRepositoryContext(ctx context.Context, request repositoryContextSyncRequest, report func(string, string) error) (*repositoryContextBatchResult, error) {
	if err := report("repository_context", jobProgressCounts(0, len(request.repositories))); err != nil {
		return nil, err
	}
	reader, err := s.githubReader() //nolint:contextcheck // Client construction performs no request; operations below receive ctx.
	if err != nil {
		return nil, err
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	results := make([]repositoryContextItem, len(request.repositories))
	remaining := request.maxRequests
	planned := 0
	requests := 0
	completed := 0
	for index, ref := range request.repositories {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		key := ref.String()
		required := repositorycontext.RequestCost()
		if required > remaining {
			results[index] = unavailableRepositoryContextItem(key, "request_budget_exceeded", syncRequestBudgetMessage(required, remaining))
			continue
		}
		remaining -= required
		planned += required
		budget := newSyncRequestBudget(required)
		repo, syncErr := syncRepositoryContextItem(ctx, c, reader, ref, budget)
		requests += budget.used
		if syncErr != nil {
			status, reason, message, retry := githubBatchError(syncErr)
			results[index] = failedRepositoryContextItem(key, status, reason, message, retry, budget.used)
			continue
		}
		results[index] = successfulRepositoryContextItem(key, budget.used, repositoryOutput(&repo))
		completed++
	}
	status := batchOperationComplete
	if completed != len(results) {
		status = batchOperationPartial
	}
	if err := report("repository_context", jobProgressCounts(len(results), len(results))); err != nil {
		return nil, err
	}
	return &repositoryContextBatchResult{
		batchOperationSummary: batchOperationSummary[repositoryContextItem]{Status: status, Items: results, Completed: completed, Total: len(results)},
		Requests:              requests, RequestBudget: request.maxRequests, PlannedRequests: planned,
	}, nil
}

func syncRepositoryContextItem(
	ctx context.Context,
	c *corpus.Corpus,
	reader github.Reader,
	ref domain.RepoRef,
	budget *syncRequestBudget,
) (_ corpus.Repository, resultErr error) {
	run, err := c.StartRun(ctx, "sync_repository_context")
	if err != nil {
		return corpus.Repository{}, err
	}
	defer failRunOnError(ctx, c, run.ID, &resultErr)
	repo, _, err := syncRepositoryHeader(ctx, c, reader, ref, run.ID, budget)
	if err != nil {
		return corpus.Repository{}, err
	}
	stats, err := json.Marshal(map[string]int{"requests": budget.used, "request_budget": budget.limit})
	if err != nil {
		return corpus.Repository{}, err
	}
	if err := c.FinishRun(ctx, run.ID, string(stats)); err != nil {
		return corpus.Repository{}, err
	}
	return repo, nil
}

func githubBatchError(err error) (status mcpcontract.BatchItemStatus, reason, message string, retryMS int) {
	message = err.Error()
	var primary *github.PrimaryRateLimitError
	var secondary *github.SecondaryRateLimitError
	var transient *github.TransientError
	var notFound *github.NotFoundError
	var denied *github.AccessDeniedError
	switch {
	case errors.As(err, &primary):
		return mcpcontract.BatchItemRetryable, "rate_limited", message, int(primary.RetryAfter.Milliseconds())
	case errors.As(err, &secondary):
		return mcpcontract.BatchItemRetryable, "rate_limited", message, int(secondary.RetryAfter.Milliseconds())
	case errors.As(err, &transient):
		return mcpcontract.BatchItemRetryable, "transient", message, 1000
	case errors.As(err, &notFound):
		return mcpcontract.BatchItemUnavailable, "not_found", message, 0
	case errors.As(err, &denied):
		return mcpcontract.BatchItemUnavailable, "access_denied", message, 0
	default:
		return mcpcontract.BatchItemFailed, "request_failed", message, 0
	}
}

func rejectDuplicateRepositoryRefs(inputs []mcpcontract.RepositoryRef) error {
	seen := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		key := strings.ToLower(input.Owner + "\x00" + input.Repo)
		if _, ok := seen[key]; ok {
			return mcpcontract.InvalidArgument("repositories", fmt.Sprintf("duplicate repository %s/%s", input.Owner, input.Repo), nil)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func rejectDuplicateThreadRefs(inputs []mcpcontract.ThreadRef) error {
	seen := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		key := strings.ToLower(fmt.Sprintf("%s\x00%s\x00%s\x00%d", input.Owner, input.Repo, input.Kind, input.Number))
		if _, ok := seen[key]; ok {
			return mcpcontract.InvalidArgument("threads", fmt.Sprintf("duplicate thread %s/%s/%s#%d", input.Owner, input.Repo, input.Kind, input.Number), nil)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func rejectDuplicateIndexRepositoryInputs(inputs []mcpcontract.IndexRepositoryInput) error {
	seen := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		key := strings.ToLower(input.Owner + "\x00" + input.Repo)
		if _, ok := seen[key]; ok {
			return mcpcontract.InvalidArgument("repositories", fmt.Sprintf("duplicate repository %s/%s; submit one remote per repository", input.Owner, input.Repo), nil)
		}
		seen[key] = struct{}{}
	}
	return nil
}
