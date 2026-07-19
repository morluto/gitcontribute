package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/deepwiki"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpserver"
)

// SyncRepositoryMetadata submits a durable metadata-only GitHub read. It does
// not fetch threads, comments, reviews, or code.
func (r *MCPReader) SyncRepositoryMetadata(ctx context.Context, in mcpserver.SyncRepositoryMetadataInput) (mcpserver.JobReference, error) {
	if len(in.Repositories) < 1 || len(in.Repositories) > 100 {
		return mcpserver.JobReference{}, errors.New("repositories must contain 1 to 100 items")
	}
	for _, input := range in.Repositories {
		if err := (domain.RepoRef{Owner: input.Owner, Repo: input.Repo}).Validate(); err != nil {
			return mcpserver.JobReference{}, err
		}
	}
	id, err := r.submitJob(ctx, "sync_repository_metadata", in, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.syncRepositoryMetadata(ctx, in.Repositories, report)
	})
	if err != nil {
		return mcpserver.JobReference{}, err
	}
	return mcpserver.JobReference{ID: id, Kind: "sync_repository_metadata", Status: "queued", Message: "repository metadata sync job started"}, nil
}

// SearchGitHubRepositories performs one bounded live repository search and
// persists the returned metadata observations without fetching thread data.
func (r *MCPReader) SearchGitHubRepositories(ctx context.Context, in mcpserver.SearchGitHubRepositoriesInput) (mcpserver.SearchGitHubRepositoriesOutput, error) {
	in.Query = strings.TrimSpace(in.Query)
	if in.Query == "" {
		return mcpserver.SearchGitHubRepositoriesOutput{}, errors.New("query is required")
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpserver.SearchGitHubRepositoriesOutput{}, errors.New("limit must be between 1 and 100")
	}
	if in.Sort != "" && in.Sort != "stars" && in.Sort != "forks" && in.Sort != "help-wanted-issues" && in.Sort != "updated" {
		return mcpserver.SearchGitHubRepositoriesOutput{}, errors.New("sort must be stars, forks, help-wanted-issues, or updated")
	}
	if in.Order != "" && in.Order != "asc" && in.Order != "desc" {
		return mcpserver.SearchGitHubRepositoriesOutput{}, errors.New("order must be asc or desc")
	}
	reader, err := r.githubReader() //nolint:contextcheck // Client construction performs no request; operations below receive ctx.
	if err != nil {
		return mcpserver.SearchGitHubRepositoriesOutput{}, err
	}
	searcher, ok := reader.(github.RepositorySearcher)
	if !ok {
		return mcpserver.SearchGitHubRepositoriesOutput{}, errors.New("configured GitHub reader does not support repository search")
	}
	result, err := searcher.SearchRepositories(ctx, github.RepositorySearchOptions{Query: in.Query, Sort: in.Sort, Order: in.Order, PageOptions: github.PageOptions{Page: 1, PerPage: in.Limit}})
	if err != nil {
		return mcpserver.SearchGitHubRepositoriesOutput{}, err
	}
	c, err := r.openCorpus(ctx)
	if err != nil {
		return mcpserver.SearchGitHubRepositoriesOutput{}, err
	}
	out := mcpserver.SearchGitHubRepositoriesOutput{Status: "complete", Query: in.Query, Total: result.Total, Incomplete: result.Incomplete, Items: make([]mcpserver.BatchItem[mcpserver.TypedRepositoryOutput], len(result.Items))}
	if result.Incomplete {
		out.Status = "partial"
	}
	observedAt := r.now()
	for i, remote := range result.Items {
		key := remote.Owner + "/" + remote.Name
		item := mcpserver.BatchItem[mcpserver.TypedRepositoryOutput]{Key: key, Status: "complete"}
		payload, err := json.Marshal(remote)
		if err != nil {
			return mcpserver.SearchGitHubRepositoriesOutput{}, err
		}
		stored, err := c.UpsertRepository(ctx, corpusRepoFromGitHub(remote), string(payload))
		if err == nil {
			err = c.AdvanceFacet(ctx, stored.ID, nil, "metadata", remote.UpdatedAt, true, 0)
		}
		if err != nil {
			return mcpserver.SearchGitHubRepositoriesOutput{}, err
		}
		value := typedRepository(stored)
		value.Metadata = mcpserver.RepositoryMetadataOutput{Status: "complete", ObservedAt: formatTime(observedAt), SourceUpdatedAt: formatTime(remote.UpdatedAt)}
		item.Value = &value
		out.Items[i] = item
	}
	return out, nil
}

// SyncThreads submits a durable bounded GitHub read for thread headers only.
func (r *MCPReader) SyncThreads(ctx context.Context, in mcpserver.SyncThreadsInput) (mcpserver.JobReference, error) {
	if in.Selection != "repositories" && in.Selection != "threads" {
		return mcpserver.JobReference{}, errors.New("selection must be repositories or threads")
	}
	if in.Selection == "repositories" && (len(in.Repositories) < 1 || len(in.Repositories) > 50) {
		return mcpserver.JobReference{}, errors.New("repositories must contain 1 to 50 items")
	}
	if in.Selection == "threads" && (len(in.Threads) < 1 || len(in.Threads) > 100) {
		return mcpserver.JobReference{}, errors.New("threads must contain 1 to 100 items")
	}
	id, err := r.submitJob(ctx, "sync_threads", in, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.syncThreadsBatch(ctx, in, report)
	})
	if err != nil {
		return mcpserver.JobReference{}, err
	}
	return mcpserver.JobReference{ID: id, Kind: "sync_threads", Status: "queued", Message: "thread synchronization job started"}, nil
}

// This function keeps bounded worker orchestration and ordered result assembly
// together so cancellation and per-item failures remain consistent.
//
//nolint:gocognit
func (s *Service) syncThreadsBatch(ctx context.Context, in mcpserver.SyncThreadsInput, report func(string, string) error) (map[string]any, error) {
	type task struct {
		key     string
		ref     cli.RepoRef
		numbers []int
	}
	var tasks []task
	if in.Selection == "repositories" {
		for _, ref := range in.Repositories {
			tasks = append(tasks, task{key: ref.Owner + "/" + ref.Repo, ref: cli.RepoRef{Owner: ref.Owner, Repo: ref.Repo}})
		}
	} else {
		grouped := make(map[string]int)
		for _, thread := range in.Threads {
			key := thread.Owner + "/" + thread.Repo
			index, ok := grouped[key]
			if !ok {
				grouped[key] = len(tasks)
				tasks = append(tasks, task{key: key, ref: cli.RepoRef{Owner: thread.Owner, Repo: thread.Repo}})
				index = len(tasks) - 1
			}
			tasks[index].numbers = append(tasks[index].numbers, thread.Number)
		}
	}
	if err := report("thread_headers", jobProgressCounts(0, len(tasks))); err != nil {
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
	results := make([]map[string]any, len(tasks))
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
				opts := SyncOptions{Kind: kind, State: state, Since: since, Numbers: current.numbers, MaxPages: maxPages}
				if len(current.numbers) > 0 {
					opts.State = "all"
					opts.Since = time.Time{}
				}
				res, err := s.SyncWithOptions(ctx, current.ref, opts)
				if err != nil {
					status, reason, message, retry := githubBatchError(err)
					results[index] = map[string]any{"key": current.key, "status": status, "reason": reason, "message": message, "retry_after_ms": retry}
					continue
				}
				results[index] = map[string]any{"key": current.key, "status": "complete", "updated": res.Updated, "message": res.Message}
			}
		}()
	}
	for i := range tasks {
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
	if err := report("thread_headers", jobProgressCounts(len(tasks), len(tasks))); err != nil {
		return nil, err
	}
	return map[string]any{"status": status, "items": results, "completed": completed, "total": len(tasks)}, nil
}

// HydrateThreads submits a durable GitHub read for explicit child facets on
// selected threads; an empty facet set is rejected.
func (r *MCPReader) HydrateThreads(ctx context.Context, in mcpserver.HydrateThreadsInput) (mcpserver.JobReference, error) {
	if len(in.Threads) < 1 || len(in.Threads) > 100 {
		return mcpserver.JobReference{}, errors.New("threads must contain 1 to 100 items")
	}
	if len(in.Facets) == 0 {
		return mcpserver.JobReference{}, errors.New("facets must not be empty")
	}
	if in.MaxPages == 0 {
		in.MaxPages = 3
	}
	id, err := r.submitJob(ctx, "hydrate_threads", in, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.hydrateThreadsBatch(ctx, in, report)
	})
	if err != nil {
		return mcpserver.JobReference{}, err
	}
	return mcpserver.JobReference{ID: id, Kind: "hydrate_threads", Status: "queued", Message: "thread hydration job started"}, nil
}

// GetAuthenticatedIdentity reads the GitHub account associated with the active credential.
func (r *MCPReader) GetAuthenticatedIdentity(ctx context.Context) (mcpserver.AuthenticatedIdentityOutput, error) {
	reader, err := r.githubReader() //nolint:contextcheck // Client construction performs no request; operations below receive ctx.
	if err != nil {
		return mcpserver.AuthenticatedIdentityOutput{}, err
	}
	identityReader, ok := reader.(github.IdentityReader)
	if !ok {
		return mcpserver.AuthenticatedIdentityOutput{}, errors.New("GitHub reader does not support authenticated identity lookup")
	}
	identity, _, err := identityReader.GetAuthenticatedIdentity(ctx)
	if err != nil {
		return mcpserver.AuthenticatedIdentityOutput{}, err
	}
	return mcpserver.AuthenticatedIdentityOutput{Login: identity.Login, ID: identity.ID, NodeID: identity.NodeID, ObservedAt: formatTime(r.now())}, nil
}

// SyncAuthoredPullRequests submits a durable GitHub search and exact-header
// refresh for pull requests authored by the authenticated account.
func (r *MCPReader) SyncAuthoredPullRequests(ctx context.Context, in mcpserver.SyncAuthoredPullRequestsInput) (mcpserver.JobReference, error) {
	if in.State == "" {
		in.State = "open"
	}
	if in.State != "open" && in.State != "closed" && in.State != "all" {
		return mcpserver.JobReference{}, errors.New("state must be open, closed, or all")
	}
	if in.Limit == 0 {
		in.Limit = 500
	}
	if in.Limit < 1 || in.Limit > 500 {
		return mcpserver.JobReference{}, errors.New("limit must be between 1 and 500")
	}
	id, err := r.submitJob(ctx, "sync_authored_pull_requests", in, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.syncAuthoredPullRequests(ctx, in, report)
	})
	if err != nil {
		return mcpserver.JobReference{}, err
	}
	return mcpserver.JobReference{ID: id, Kind: "sync_authored_pull_requests", Status: "queued", Message: "authored pull-request synchronization job started"}, nil
}

// This bounded job combines pagination, repository grouping, and ordered worker
// results because those phases share a single discovery limit and status result.
//
//nolint:gocognit,cyclop,funlen
func (s *Service) syncAuthoredPullRequests(ctx context.Context, in mcpserver.SyncAuthoredPullRequestsInput, report func(string, string) error) (map[string]any, error) {
	if err := report("authored_pull_request_discovery", jobProgressCounts(0, in.Limit)); err != nil {
		return nil, err
	}
	reader, err := s.githubReader() //nolint:contextcheck // Client construction performs no request; operations below receive ctx.
	if err != nil {
		return nil, err
	}
	identityReader, ok := reader.(github.IdentityReader)
	if !ok {
		return nil, errors.New("GitHub reader does not support authenticated identity lookup")
	}
	searcher, ok := reader.(github.AuthoredPullRequestSearcher)
	if !ok {
		return nil, errors.New("GitHub reader does not support authored pull-request search")
	}
	identity, _, err := identityReader.GetAuthenticatedIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var updatedAfter time.Time
	if in.UpdatedAfter != "" {
		updatedAfter, err = time.Parse(time.RFC3339, in.UpdatedAfter)
		if err != nil {
			return nil, errors.New("updated_after must be RFC 3339")
		}
	}
	page := 1
	byRepo := make(map[string][]int)
	order := make([]string, 0)
	discovered := 0
	incomplete := false
	for discovered < in.Limit {
		perPage := 100
		if remaining := in.Limit - discovered; remaining < perPage {
			perPage = remaining
		}
		result, err := searcher.SearchAuthoredPullRequests(ctx, github.AuthoredPullRequestSearchOptions{Login: identity.Login, State: in.State, UpdatedAfter: updatedAfter, PageOptions: github.PageOptions{Page: page, PerPage: perPage}})
		if err != nil {
			return nil, err
		}
		incomplete = incomplete || result.Incomplete
		for _, pr := range result.Items {
			if pr.RepositoryOwner == "" || pr.RepositoryName == "" {
				continue
			}
			key := pr.RepositoryOwner + "/" + pr.RepositoryName
			if _, exists := byRepo[key]; !exists {
				order = append(order, key)
			}
			byRepo[key] = append(byRepo[key], pr.Number)
			discovered++
			if discovered >= in.Limit {
				break
			}
		}
		if !result.Page.HasNext || discovered >= in.Limit {
			break
		}
		page = result.Page.NextPage
	}
	type authoredTask struct {
		key, owner, repo string
		numbers          []int
	}
	var tasks []authoredTask
	for _, key := range order {
		owner, repo, _ := strings.Cut(key, "/")
		numbers := byRepo[key]
		for len(numbers) > 0 {
			size := min(100, len(numbers))
			tasks = append(tasks, authoredTask{key: key, owner: owner, repo: repo, numbers: append([]int(nil), numbers[:size]...)})
			numbers = numbers[size:]
		}
	}
	results := make([]map[string]any, len(tasks))
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
				res, err := s.SyncWithOptions(ctx, cli.RepoRef{Owner: current.owner, Repo: current.repo}, SyncOptions{Kind: "pull_request", State: "all", Numbers: current.numbers, MaxPages: 1})
				if err != nil {
					status, reason, message, retry := githubBatchError(err)
					results[index] = map[string]any{"key": current.key, "status": status, "reason": reason, "message": message, "retry_after_ms": retry}
					continue
				}
				results[index] = map[string]any{"key": current.key, "status": "complete", "updated": res.Updated}
			}
		}()
	}
	for i := range tasks {
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
	if err := report("authored_pull_request_headers", jobProgressCounts(len(tasks), len(tasks))); err != nil {
		return nil, err
	}
	return map[string]any{"status": status, "login": identity.Login, "pull_requests": discovered, "repositories": results, "search_incomplete": incomplete}, nil
}

// SyncPullRequestStatus submits a bounded source-backed refresh of PR details,
// reviews, checks, review conversations, merge state, queue state, closing
// issues, and changed paths. Each facet retains independent coverage.
func (r *MCPReader) SyncPullRequestStatus(ctx context.Context, in mcpserver.SyncPullRequestStatusInput) (mcpserver.JobReference, error) {
	if len(in.PullRequests) < 1 || len(in.PullRequests) > 50 {
		return mcpserver.JobReference{}, errors.New("pull_requests must contain 1 to 50 items")
	}
	if in.MaxPages == 0 {
		in.MaxPages = 3
	}
	id, err := r.submitJob(ctx, "sync_pull_request_status", in, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.syncPullRequestStatusBatch(ctx, in, report)
	})
	if err != nil {
		return mcpserver.JobReference{}, err
	}
	return mcpserver.JobReference{ID: id, Kind: "sync_pull_request_status", Status: "queued", Message: "pull-request status synchronization job started"}, nil
}

// IndexRepositories submits a durable Git acquisition and safe indexing job
// with at most two repositories processed concurrently.
func (r *MCPReader) IndexRepositories(ctx context.Context, in mcpserver.IndexRepositoriesInput) (mcpserver.JobReference, error) {
	if len(in.Repositories) < 1 || len(in.Repositories) > 10 {
		return mcpserver.JobReference{}, errors.New("repositories must contain 1 to 10 items")
	}
	for _, input := range in.Repositories {
		if err := (domain.RepoRef{Owner: input.Owner, Repo: input.Repo}).Validate(); err != nil {
			return mcpserver.JobReference{}, err
		}
	}
	id, err := r.submitJob(ctx, "index_repositories", in, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.indexRepositoriesBatch(ctx, in, report)
	})
	if err != nil {
		return mcpserver.JobReference{}, err
	}
	return mcpserver.JobReference{ID: id, Kind: "index_repositories", Status: "queued", Message: "repository indexing job started"}, nil
}

// CheckMergeConflicts compares already-fetched OIDs in managed workspaces
// without fetching or modifying refs, indexes, or worktrees.
func (r *MCPReader) CheckMergeConflicts(ctx context.Context, in mcpserver.CheckMergeConflictsInput) (mcpserver.CheckMergeConflictsOutput, error) {
	if len(in.Comparisons) < 1 || len(in.Comparisons) > 50 {
		return mcpserver.CheckMergeConflictsOutput{}, errors.New("comparisons must contain 1 to 50 items")
	}
	c, err := r.openCorpus(ctx)
	if err != nil {
		return mcpserver.CheckMergeConflictsOutput{}, err
	}
	manager, err := r.workspaceManager(ctx)
	if err != nil {
		return mcpserver.CheckMergeConflictsOutput{}, err
	}
	out := mcpserver.CheckMergeConflictsOutput{Status: "complete", Items: make([]mcpserver.BatchItem[mcpserver.MergeConflictOutput], len(in.Comparisons))}
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
				key := current.WorkspaceID + ":" + current.BaseOID + ".." + current.HeadOID
				item := mcpserver.BatchItem[mcpserver.MergeConflictOutput]{Key: key, Status: "complete"}
				ws, err := c.GetWorkspace(ctx, current.WorkspaceID)
				if err != nil {
					item.Status, item.Reason, item.Message = "failed", "workspace_not_found", err.Error()
					out.Items[index] = item
					continue
				}
				result, err := manager.CheckMerge(ctx, ws.Path, current.BaseOID, current.HeadOID)
				if err != nil {
					item.Status, item.Reason, item.Message = "failed", "merge_check_failed", err.Error()
					out.Items[index] = item
					continue
				}
				value := mcpserver.MergeConflictOutput{WorkspaceID: current.WorkspaceID, BaseOID: current.BaseOID, HeadOID: current.HeadOID, MergeBase: result.MergeBase, Conflicted: result.Conflicted, Summary: result.Summary}
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

func (s *Service) indexRepositoriesBatch(ctx context.Context, in mcpserver.IndexRepositoriesInput, report func(string, string) error) (map[string]any, error) {
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
				result, err := s.Acquire(ctx, cli.RepoRef{Owner: current.Owner, Repo: current.Repo}, current.Remote)
				if err != nil {
					results[index] = map[string]any{"key": key, "status": "failed", "reason": "acquisition_or_index_failed", "message": err.Error()}
					continue
				}
				results[index] = map[string]any{"key": key, "status": "complete", "commit_sha": result.CommitSHA, "files": result.Files, "bytes": result.Bytes, "inserted": result.Inserted}
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

func (s *Service) hydrateThreadsBatch(ctx context.Context, in mcpserver.HydrateThreadsInput, report func(string, string) error) (map[string]any, error) {
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
				key := fmt.Sprintf("%s/%s#%d", current.Owner, current.Repo, current.Number)
				res, err := s.Hydrate(ctx, cli.RepoRef{Owner: current.Owner, Repo: current.Repo}, current.Number, cli.HydrateOptions{Facets: in.Facets, MaxPages: in.MaxPages})
				if err != nil {
					status, reason, message, retry := githubBatchError(err)
					results[index] = map[string]any{"key": key, "status": status, "reason": reason, "message": message, "retry_after_ms": retry}
					continue
				}
				results[index] = map[string]any{"key": key, "status": "complete", "kind": res.Kind, "requests": res.Requests, "facets": res.Facets}
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
func (s *Service) syncRepositoryMetadata(ctx context.Context, refs []mcpserver.RepositoryRef, report func(string, string) error) (mcpserver.GetRepositoriesOutput, error) {
	if err := report("repository_metadata", jobProgressCounts(0, len(refs))); err != nil {
		return mcpserver.GetRepositoriesOutput{}, err
	}
	reader, err := s.githubReader() //nolint:contextcheck // Client construction performs no request; operations below receive ctx.
	if err != nil {
		return mcpserver.GetRepositoriesOutput{}, err
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return mcpserver.GetRepositoriesOutput{}, err
	}
	out := mcpserver.GetRepositoriesOutput{Status: "complete", Items: make([]mcpserver.BatchItem[mcpserver.TypedRepositoryOutput], len(refs))}
	type work struct {
		index int
		ref   mcpserver.RepositoryRef
	}
	jobs := make(chan work)
	var wg sync.WaitGroup
	workers := 8
	if len(refs) < workers {
		workers = len(refs)
	}
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for current := range jobs {
				if ctx.Err() != nil {
					return
				}
				key := current.ref.Owner + "/" + current.ref.Repo
				item := mcpserver.BatchItem[mcpserver.TypedRepositoryOutput]{Key: key, Status: "complete"}
				remote, _, err := reader.GetRepository(ctx, current.ref.Owner, current.ref.Repo)
				if err != nil {
					item.Status, item.Reason, item.Message, item.RetryAfterMS = githubBatchError(err)
					out.Items[current.index] = item
					continue
				}
				payload, err := json.Marshal(remote)
				if err != nil {
					item.Status, item.Reason, item.Message = "failed", "marshal", err.Error()
					out.Items[current.index] = item
					continue
				}
				stored, err := c.UpsertRepository(ctx, corpusRepoFromGitHub(remote), string(payload))
				if err == nil {
					err = c.AdvanceFacet(ctx, stored.ID, nil, "metadata", remote.UpdatedAt, true, 0)
				}
				if err != nil {
					item.Status, item.Reason, item.Message = "failed", "storage", err.Error()
					out.Items[current.index] = item
					continue
				}
				value := typedRepository(stored)
				value.Metadata = mcpserver.RepositoryMetadataOutput{Status: "complete", ObservedAt: formatTime(s.now()), SourceUpdatedAt: formatTime(remote.UpdatedAt)}
				item.Value = &value
				out.Items[current.index] = item
			}
		}()
	}
	for i, ref := range refs {
		select {
		case jobs <- work{i, ref}:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return out, ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	completed := 0
	for _, item := range out.Items {
		if item.Status == "complete" {
			completed++
		} else {
			out.Status = "partial"
		}
	}
	if err := report("repository_metadata", jobProgressCounts(len(refs), len(refs))); err != nil {
		return out, err
	}
	return out, nil
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
func (r *MCPReader) DeepWiki(ctx context.Context, in mcpserver.DeepWikiInput) (mcpserver.DeepWikiOutput, error) {
	if in.Action != "structure" && in.Action != "contents" && in.Action != "question" {
		return mcpserver.DeepWikiOutput{}, errors.New("action must be structure, contents, or question")
	}
	if (in.Action == "structure" || in.Action == "contents") && strings.TrimSpace(in.Repository) == "" {
		return mcpserver.DeepWikiOutput{}, errors.New("repository is required for structure or contents")
	}
	if in.Action == "question" && (len(in.Repositories) < 1 || strings.TrimSpace(in.Question) == "") {
		return mcpserver.DeepWikiOutput{}, errors.New("repositories and question are required for question")
	}
	repositories := append([]string(nil), in.Repositories...)
	if in.Repository != "" {
		repositories = []string{in.Repository}
	}
	if len(repositories) > 10 {
		return mcpserver.DeepWikiOutput{}, errors.New("DeepWiki supports at most 10 repositories")
	}
	res, err := r.deepWiki().Read(ctx, deepwiki.Request{Action: in.Action, Repository: in.Repository, Repositories: in.Repositories, Question: in.Question})
	if err != nil {
		return mcpserver.DeepWikiOutput{}, err
	}
	out := mcpserver.DeepWikiOutput{Status: "complete", Provider: "deepwiki", Action: in.Action, Repositories: repositories, Question: in.Question, Result: res.Text, SourceURL: res.SourceURL, RetrievedAt: formatTime(r.now()), Provenance: "derived_external"}
	if !res.Available {
		out.Status, out.Reason, out.NextAction = "unavailable", "not_indexed_or_unavailable", "Use GitHub metadata, stored corpus data, or explicit code acquisition instead."
		return out, nil
	}
	maxBytes := in.MaxOutputBytes
	if maxBytes == 0 {
		maxBytes = 131072
	}
	if maxBytes < 1024 || maxBytes > 1048576 {
		return mcpserver.DeepWikiOutput{}, errors.New("max_output_bytes must be between 1024 and 1048576")
	}
	if len(out.Result) > maxBytes {
		out.Result = validUTF8Prefix(out.Result, maxBytes)
		out.Truncated = true
	}
	return out, nil
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
