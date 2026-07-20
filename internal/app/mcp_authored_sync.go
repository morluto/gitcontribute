package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpserver"
)

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
	requests := 1
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
	byRepo := make(map[string][]github.Issue)
	order := make([]string, 0)
	discovered := 0
	incomplete := false
	requestCapped := false
	for discovered < in.Limit {
		if requests >= in.MaxRequests {
			requestCapped, incomplete = true, true
			break
		}
		perPage := min(100, in.Limit-discovered)
		requests++
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
			byRepo[key] = append(byRepo[key], pr)
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
		issues           []github.Issue
		maxRequests      int
	}
	tasks := make([]authoredTask, 0, len(order))
	for _, key := range order {
		owner, repo, _ := strings.Cut(key, "/")
		tasks = append(tasks, authoredTask{key: key, owner: owner, repo: repo, issues: append([]github.Issue(nil), byRepo[key]...)})
	}
	results := make([]map[string]any, len(tasks))
	remainingRequests := in.MaxRequests - requests
	plannedRequests := requests
	runnable := make([]int, 0, len(tasks))
	for index := range tasks {
		required := syncFixedRequestCost()
		if required > remainingRequests {
			results[index] = syncRequestBudgetUnavailable(tasks[index].key, required, remainingRequests)
			requestCapped = true
			continue
		}
		tasks[index].maxRequests = required
		remainingRequests -= required
		plannedRequests += required
		runnable = append(runnable, index)
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	workers := min(4, len(tasks))
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				current := tasks[index]
				res, err := s.syncProvidedThreadHeaders(ctx, cli.RepoRef{Owner: current.owner, Repo: current.repo}, current.issues, current.maxRequests)
				if err != nil {
					status, reason, message, retry := githubBatchError(err)
					results[index] = map[string]any{"key": current.key, "status": status, "reason": reason, "message": message, "retry_after_ms": retry}
					continue
				}
				results[index] = map[string]any{"key": current.key, "status": "complete", "updated": res.Updated, "requests": res.Requests}
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
	status := "complete"
	completed := 0
	for _, result := range results {
		if count, ok := result["requests"].(int); ok {
			requests += count
		}
		if result["status"] == "complete" {
			completed++
		} else {
			status = "partial"
		}
	}
	if err := report("authored_pull_request_headers", jobProgressCounts(len(tasks), len(tasks))); err != nil {
		return nil, err
	}
	return map[string]any{
		"status": status, "login": identity.Login, "pull_requests": discovered, "repositories": results,
		"search_incomplete": incomplete, "request_capped": requestCapped, "requests": requests,
		"request_budget": in.MaxRequests, "planned_requests": plannedRequests,
	}, nil
}
