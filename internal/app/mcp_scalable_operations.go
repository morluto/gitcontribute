package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/deepwiki"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
	"github.com/morluto/gitcontribute/internal/repositorycontext"
)

// SyncRepositoryContext submits a durable metadata and contribution-guidance
// GitHub read. It does not fetch threads, comments, reviews, or code.
func (r *MCPReader) SyncRepositoryContext(ctx context.Context, in mcpcontract.SyncRepositoryContextInput) (mcpcontract.JobReference, error) {
	if err := rejectDuplicateRepositoryRefs(in.Repositories); err != nil {
		return mcpcontract.JobReference{}, err
	}
	if len(in.Repositories) < 1 || len(in.Repositories) > 100 {
		return mcpcontract.JobReference{}, errors.New("repositories must contain 1 to 100 items")
	}
	for _, input := range in.Repositories {
		if err := (domain.RepoRef{Owner: input.Owner, Repo: input.Repo}).Validate(); err != nil {
			return mcpcontract.JobReference{}, err
		}
	}
	if in.MaxRequests == 0 {
		in.MaxRequests = defaultSyncBatchMaxRequests
	}
	if in.MaxRequests < repositorycontext.RequestCost() || in.MaxRequests > defaultSyncBatchMaxRequests {
		return mcpcontract.JobReference{}, fmt.Errorf(
			"max requests must be between %d and %d",
			repositorycontext.RequestCost(), defaultSyncBatchMaxRequests,
		)
	}
	id, err := r.submitJob(ctx, "sync_repository_context", in, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.syncRepositoryContext(ctx, in, report)
	})
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	return queuedJobReference(id, "sync_repository_context", "repository context sync job started"), nil
}

// SyncThreads submits a durable bounded GitHub read for thread headers in
// repositories that already have local identities.
func (r *MCPReader) SyncThreads(ctx context.Context, in mcpcontract.SyncThreadsInput) (mcpcontract.JobReference, error) {
	if in.Selection != "repositories" && in.Selection != "threads" {
		return mcpcontract.JobReference{}, errors.New("selection must be repositories or threads")
	}
	if err := rejectDuplicateRepositoryRefs(in.Repositories); err != nil {
		return mcpcontract.JobReference{}, err
	}
	if err := rejectDuplicateThreadRefs(in.Threads); err != nil {
		return mcpcontract.JobReference{}, err
	}
	if in.Selection == "repositories" && (len(in.Repositories) < 1 || len(in.Repositories) > 50) {
		return mcpcontract.JobReference{}, errors.New("repositories must contain 1 to 50 items")
	}
	if in.Selection == "threads" && (len(in.Threads) < 1 || len(in.Threads) > 100) {
		return mcpcontract.JobReference{}, errors.New("threads must contain 1 to 100 items")
	}
	if in.Selection == "repositories" {
		if in.LimitPerRepository == 0 {
			in.LimitPerRepository = 100
		}
		if in.LimitPerRepository < 1 || in.LimitPerRepository > 1000 {
			return mcpcontract.JobReference{}, errors.New("limit_per_repository must be between 1 and 1000")
		}
	} else if in.LimitPerRepository != 0 {
		return mcpcontract.JobReference{}, errors.New("limit_per_repository is only valid in repository selection mode")
	}
	if in.MaxRequests == 0 {
		in.MaxRequests = defaultSyncBatchMaxRequests
	}
	if in.MaxRequests < 1 || in.MaxRequests > defaultSyncBatchMaxRequests {
		return mcpcontract.JobReference{}, fmt.Errorf("max requests must be between 1 and %d", defaultSyncBatchMaxRequests)
	}
	id, err := r.submitJob(ctx, "sync_threads", in, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.syncThreadsBatch(ctx, in, report)
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
func (s *Service) syncThreadsBatch(ctx context.Context, in mcpcontract.SyncThreadsInput, report func(string, string) error) (map[string]any, error) {
	type task struct {
		key          string
		ref          contracts.RepoRef
		kind         string
		numbers      []int
		inputIndexes []int
		maxRequests  int
	}
	var tasks []task
	if in.Selection == "repositories" {
		for _, ref := range in.Repositories {
			tasks = append(tasks, task{key: ref.Owner + "/" + ref.Repo, ref: contracts.RepoRef{Owner: ref.Owner, Repo: ref.Repo}})
		}
	} else {
		grouped := make(map[string]int)
		for inputIndex, thread := range in.Threads {
			kind := thread.Kind
			if kind == "" {
				kind = "both"
			}
			key := thread.Owner + "/" + thread.Repo + "\x00" + kind
			index, ok := grouped[key]
			if !ok {
				grouped[key] = len(tasks)
				tasks = append(tasks, task{key: thread.Owner + "/" + thread.Repo + "/" + kind, kind: kind, ref: contracts.RepoRef{Owner: thread.Owner, Repo: thread.Repo}})
				index = len(tasks) - 1
			}
			tasks[index].numbers = append(tasks[index].numbers, thread.Number)
			tasks[index].inputIndexes = append(tasks[index].inputIndexes, inputIndex)
		}
	}
	resultCount := len(tasks)
	if in.Selection == "threads" {
		resultCount = len(in.Threads)
	}
	if err := report("thread_headers", jobProgressCounts(0, resultCount)); err != nil {
		return nil, err
	}
	state := in.State
	if state == "" {
		state = "open"
	}
	kind := in.Kind
	if kind == "" {
		kind = "both"
	}
	maxPages := 1
	if in.LimitPerRepository > 100 {
		maxPages = (in.LimitPerRepository + 99) / 100
	}
	var since time.Time
	if in.UpdatedAfter != "" {
		parsed, err := time.Parse(time.RFC3339, in.UpdatedAfter)
		if err != nil {
			return nil, errors.New("updated_after must be RFC 3339")
		}
		since = parsed
	}
	taskResults := make([]map[string]any, len(tasks))
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	remainingRequests := in.MaxRequests
	plannedRequests := 0
	runnable := make([]int, 0, len(tasks))
	for index := range tasks {
		stored, err := c.GetRepository(ctx, tasks[index].ref.Owner, tasks[index].ref.Repo)
		if err != nil {
			return nil, err
		}
		if stored == nil {
			taskResults[index] = map[string]any{
				"key": tasks[index].key, "status": "unavailable", "reason": "repository_not_indexed",
				"message": "repository is not stored; call github.sync_repository_context first",
			}
			continue
		}
		threadRequests := maxPages
		if len(tasks[index].numbers) > 0 {
			threadRequests = len(tasks[index].numbers)
			if threadRequests > remainingRequests {
				taskResults[index] = syncRequestBudgetUnavailable(tasks[index].key, threadRequests, remainingRequests)
				continue
			}
		} else if threadRequests > remainingRequests {
			threadRequests = remainingRequests
		}
		if threadRequests < 1 {
			taskResults[index] = syncRequestBudgetUnavailable(tasks[index].key, 1, remainingRequests)
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
				if in.Selection == "threads" {
					currentKind = current.kind
				}
				opts := SyncOptions{Kind: currentKind, State: state, Since: since, Numbers: current.numbers, MaxItems: in.LimitPerRepository, MaxPages: maxPages, MaxRequests: current.maxRequests}
				if len(current.numbers) > 0 {
					opts.State = "all"
					opts.Since = time.Time{}
				}
				res, err := s.syncThreadHeaders(ctx, current.ref, opts)
				if err != nil {
					status, reason, message, retry := githubBatchError(err)
					taskResults[index] = map[string]any{"key": current.key, "status": status, "reason": reason, "message": message, "retry_after_ms": retry}
					continue
				}
				status := "complete"
				if res.Capped {
					status = "partial"
				}
				taskResults[index] = map[string]any{"key": current.key, "status": status, "updated": res.Updated, "requests": res.Requests, "request_capped": res.Capped, "message": res.Message}
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
	if in.Selection == "threads" {
		results = make([]map[string]any, len(in.Threads))
		for taskIndex, current := range tasks {
			for _, inputIndex := range current.inputIndexes {
				item := maps.Clone(taskResults[taskIndex])
				delete(item, "requests")
				delete(item, "updated")
				thread := in.Threads[inputIndex]
				item["key"] = threadRefKey(thread)
				results[inputIndex] = item
			}
		}
	}
	status := "complete"
	completed := 0
	requests := 0
	for _, result := range taskResults {
		if count, ok := result["requests"].(int); ok {
			requests += count
		}
	}
	for _, result := range results {
		if result["status"] == "complete" {
			completed++
		} else {
			status = "partial"
		}
	}
	if err := report("thread_headers", jobProgressCounts(resultCount, resultCount)); err != nil {
		return nil, err
	}
	return map[string]any{
		"status": status, "items": results, "completed": completed, "total": resultCount,
		"requests": requests, "request_budget": in.MaxRequests, "planned_requests": plannedRequests,
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
	if err := rejectDuplicateIndexRepositoryInputs(in.Repositories); err != nil {
		return mcpcontract.JobReference{}, err
	}
	if len(in.Repositories) < 1 || len(in.Repositories) > 10 {
		return mcpcontract.JobReference{}, errors.New("repositories must contain 1 to 10 items")
	}
	for _, input := range in.Repositories {
		if err := (domain.RepoRef{Owner: input.Owner, Repo: input.Repo}).Validate(); err != nil {
			return mcpcontract.JobReference{}, err
		}
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
			Tool: mcpcontract.ToolGetJob, Arguments: &mcpcontract.ToolCallArguments{IDs: []string{id}}, Reason: "Poll this job ID after the suggested delay.",
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
					out.Items[index] = item
					continue
				}
				result, err := manager.CheckMergeWorkspace(ctx, ws, current.BaseOID, current.HeadOID)
				if err != nil {
					item.Status, item.Reason, item.Message = "failed", "merge_check_failed", err.Error()
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

func (s *Service) indexRepositoriesBatch(ctx context.Context, in mcpcontract.IndexRepositoriesInput, report func(string, string) error) (map[string]any, error) {
	if err := report("repository_indexing", jobProgressCounts(0, len(in.Repositories))); err != nil {
		return nil, err
	}
	results := make([]map[string]any, len(in.Repositories))
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
					results[index] = map[string]any{"key": key, "status": "failed", "reason": "acquisition_or_index_failed", "message": err.Error()}
					continue
				}
				results[index] = map[string]any{"key": key, "status": "complete", "commit_sha": result.CommitSHA, "files": result.Files, "bytes": result.Bytes, "inserted": result.Inserted, "index_manifest": result.IndexManifest}
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
	status := "complete"
	completed := 0
	for _, result := range results {
		if result["status"] == "complete" {
			completed++
		} else {
			status = "partial"
		}
	}
	if err := report("repository_indexing", jobProgressCounts(len(in.Repositories), len(in.Repositories))); err != nil {
		return nil, err
	}
	return map[string]any{"status": status, "items": results, "completed": completed, "total": len(in.Repositories)}, nil
}

func (s *Service) hydrateThreadsBatch(ctx context.Context, in mcpcontract.HydrateThreadsInput, report func(string, string) error) (map[string]any, error) {
	if err := report("thread_hydration", jobProgressCounts(0, len(in.Threads))); err != nil {
		return nil, err
	}
	results := make([]map[string]any, len(in.Threads))
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
					results[index] = map[string]any{"key": key, "status": status, "reason": reason, "message": message, "retry_after_ms": retry}
					continue
				}
				results[index] = map[string]any{
					"key": key, "status": "complete", "kind": res.Kind,
					"header_refreshed": true, "requests": res.Requests, "facets": res.Facets,
				}
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
	status := "complete"
	completed := 0
	for _, result := range results {
		if result["status"] == "complete" {
			completed++
		} else {
			status = "partial"
		}
	}
	if err := report("thread_hydration", jobProgressCounts(len(in.Threads), len(in.Threads))); err != nil {
		return nil, err
	}
	return map[string]any{"status": status, "items": results, "completed": completed, "total": len(in.Threads)}, nil
}

// This bounded worker loop keeps each repository's fetch, persistence, and
// ordered result mapping in one place to preserve item-level failure semantics.
//
//nolint:gocognit
func (s *Service) syncRepositoryContext(ctx context.Context, in mcpcontract.SyncRepositoryContextInput, report func(string, string) error) (map[string]any, error) {
	if err := report("repository_context", jobProgressCounts(0, len(in.Repositories))); err != nil {
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
	results := make([]map[string]any, len(in.Repositories))
	remaining := in.MaxRequests
	planned := 0
	requests := 0
	completed := 0
	for index, input := range in.Repositories {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		key := input.Owner + "/" + input.Repo
		required := repositorycontext.RequestCost()
		if required > remaining {
			results[index] = syncRequestBudgetUnavailable(key, required, remaining)
			continue
		}
		remaining -= required
		planned += required
		budget := newSyncRequestBudget(required)
		ref := domain.RepoRef{Owner: input.Owner, Repo: input.Repo}
		repo, syncErr := syncRepositoryContextItem(ctx, c, reader, ref, budget)
		requests += budget.used
		if syncErr != nil {
			status, reason, message, retry := githubBatchError(syncErr)
			results[index] = map[string]any{
				"key": key, "status": status, "reason": reason, "message": message,
				"retry_after_ms": retry, "requests": budget.used,
			}
			continue
		}
		results[index] = map[string]any{
			"key": key, "status": "complete", "requests": budget.used,
			"repository": typedRepository(&repo),
			"facets": map[string]any{
				"metadata":              map[string]any{"status": "complete"},
				"contribution_guidance": map[string]any{"status": "complete"},
			},
		}
		completed++
	}
	status := "complete"
	if completed != len(results) {
		status = "partial"
	}
	if err := report("repository_context", jobProgressCounts(len(results), len(results))); err != nil {
		return nil, err
	}
	return map[string]any{
		"status": status, "items": results, "completed": completed, "total": len(results),
		"requests": requests, "request_budget": in.MaxRequests, "planned_requests": planned,
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
	defer func() {
		if resultErr != nil {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = c.FailRun(cleanupCtx, run.ID, resultErr.Error())
		}
	}()
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

func githubBatchError(err error) (status, reason, message string, retryMS int) {
	message = err.Error()
	var primary *github.PrimaryRateLimitError
	var secondary *github.SecondaryRateLimitError
	var transient *github.TransientError
	var notFound *github.NotFoundError
	var denied *github.AccessDeniedError
	switch {
	case errors.As(err, &primary):
		return "retryable", "rate_limited", message, int(primary.RetryAfter.Milliseconds())
	case errors.As(err, &secondary):
		return "retryable", "rate_limited", message, int(secondary.RetryAfter.Milliseconds())
	case errors.As(err, &transient):
		return "retryable", "transient", message, 1000
	case errors.As(err, &notFound):
		return "unavailable", "not_found", message, 0
	case errors.As(err, &denied):
		return "unavailable", "access_denied", message, 0
	default:
		return "failed", "request_failed", message, 0
	}
}

// DeepWiki performs one external derived-knowledge read and does not persist its response.
func (r *MCPReader) DeepWiki(ctx context.Context, in mcpcontract.DeepWikiInput) (mcpcontract.DeepWikiOutput, error) {
	if in.Action != "structure" && in.Action != "contents" && in.Action != "question" {
		return mcpcontract.DeepWikiOutput{}, errors.New("action must be structure, contents, or question")
	}
	if (in.Action == "structure" || in.Action == "contents") && strings.TrimSpace(in.Repository) == "" {
		return mcpcontract.DeepWikiOutput{}, errors.New("repository is required for structure or contents")
	}
	if in.Action == "question" && (len(in.Repositories) < 1 || strings.TrimSpace(in.Question) == "") {
		return mcpcontract.DeepWikiOutput{}, errors.New("repositories and question are required for question")
	}
	repositories := append([]string(nil), in.Repositories...)
	if in.Repository != "" {
		repositories = []string{in.Repository}
	}
	if len(repositories) > 10 {
		return mcpcontract.DeepWikiOutput{}, errors.New("DeepWiki supports at most 10 repositories")
	}
	maxBytes := in.MaxOutputBytes
	if maxBytes == 0 {
		maxBytes = mcpcontract.DeepWikiDefaultOutputBytes
	}
	if maxBytes < mcpcontract.DeepWikiMinOutputBytes || maxBytes > mcpcontract.DeepWikiMaxOutputBytes {
		return mcpcontract.DeepWikiOutput{}, errors.New("max_output_bytes must be between 1024 and 1048576")
	}
	res, err := r.deepWiki().Read(ctx, deepwiki.Request{Action: in.Action, Repository: in.Repository, Repositories: repositories, Question: in.Question})
	if err != nil {
		return mcpcontract.DeepWikiOutput{}, err
	}
	out := mcpcontract.DeepWikiOutput{Status: "complete", Provider: "deepwiki", Action: in.Action, Repositories: repositories, Question: in.Question, Result: res.Text, SourceURL: res.SourceURL, RetrievedAt: formatTime(r.now()), Provenance: "derived_external"}
	if !res.Available {
		out.Status, out.Reason = "unavailable", "blocked"
		out.Recovery = recoveryPlan("blocked", "Use GitHub metadata, stored corpus data, or explicit code acquisition instead.")
		return out, nil
	}
	if len(out.Result) > maxBytes {
		out.Result = validUTF8Prefix(out.Result, maxBytes)
		out.Truncated = true
		out.Reason = "output_limit"
		if in.Action == "contents" {
			out.Recovery = recoveryPlan("blocked", "Call structure, then ask a focused question about the relevant section. Increase max_output_bytes only when the focused read is still incomplete.", mcpcontract.ToolCall{Tool: mcpcontract.ToolQueryDeepWiki, Arguments: &mcpcontract.ToolCallArguments{Action: "structure", Repository: in.Repository}})
		} else {
			out.Recovery = recoveryPlan("blocked", "Narrow the question or repository set. Increase max_output_bytes only when the focused read is still incomplete.")
		}
	}
	return out, nil
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

func validatePullRequestRefs(inputs []mcpcontract.ThreadRef, path string) error {
	for i, input := range inputs {
		itemPath := fmt.Sprintf("%s[%d]", path, i)
		if strings.TrimSpace(input.Owner) == "" {
			return mcpcontract.InvalidArgument(itemPath+".owner", "must not be blank", nil)
		}
		if strings.TrimSpace(input.Repo) == "" {
			return mcpcontract.InvalidArgument(itemPath+".repo", "must not be blank", nil)
		}
		if input.Number <= 0 {
			return mcpcontract.InvalidArgument(itemPath+".number", "must be positive", nil)
		}
		if input.Kind != "" && input.Kind != corpus.ThreadKindPullRequest {
			return mcpcontract.InvalidArgument(itemPath+".kind", "must be pull_request when provided", nil)
		}
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

func validUTF8Prefix(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	for maxBytes > 0 && !utf8.ValidString(value[:maxBytes]) {
		maxBytes--
	}
	return value[:maxBytes]
}
