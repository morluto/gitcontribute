package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// This bounded job combines pagination, repository grouping, and ordered worker
// results because those phases share a single discovery limit and status result.
//
//nolint:gocognit,cyclop,funlen
func (s *Service) syncAuthoredPullRequests(ctx context.Context, in mcpcontract.SyncAuthoredPullRequestsInput, report func(string, string) error) (*authoredPullRequestSyncResult, error) {
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
	pullRequestRefs := make([]string, 0, in.Limit)
	pullRequestTargets := make([]mcpcontract.ThreadRef, 0, in.Limit)
	discovered := 0
	incomplete := false
	requestCapped := false
	for discovered < in.Limit {
		if requests+1 > in.MaxRequests {
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
			pullRequestRefs = append(pullRequestRefs, fmt.Sprintf("%s#%d", key, pr.Number))
			pullRequestTargets = append(pullRequestTargets, mcpcontract.ThreadRef{
				Owner: pr.RepositoryOwner, Repo: pr.RepositoryName, Kind: "pull_request", Number: pr.Number,
			})
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
	}
	tasks := make([]authoredTask, 0, len(order))
	for _, key := range order {
		owner, repo, _ := strings.Cut(key, "/")
		tasks = append(tasks, authoredTask{key: key, owner: owner, repo: repo, issues: append([]github.Issue(nil), byRepo[key]...)})
	}
	results := make([]authoredRepositorySyncResult, len(tasks))
	plannedRequests := requests
	runnable := make([]int, len(tasks))
	for index := range tasks {
		runnable[index] = index
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
				res, err := s.syncProvidedThreadHeaders(ctx, contracts.RepoRef{Owner: current.owner, Repo: current.repo}, current.issues)
				if err != nil {
					status, reason, message, retry := githubBatchError(err)
					results[index] = authoredRepositorySyncResult{Key: current.key, Status: status, Reason: reason, Message: message, RetryAfterMS: retry}
					continue
				}
				results[index] = authoredRepositorySyncResult{Key: current.key, Status: "complete", Updated: res.Updated, Requests: res.Requests}
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
		requests += result.Requests
		if result.Status == "complete" {
			completed++
		} else {
			status = "partial"
		}
	}
	if err := report("authored_pull_request_headers", jobProgressCounts(len(tasks), len(tasks))); err != nil {
		return nil, err
	}
	return &authoredPullRequestSyncResult{
		Status: status, Login: identity.Login, PullRequests: discovered,
		PullRequestRefs: pullRequestRefs, PullRequestTargets: pullRequestTargets, Repositories: results,
		SearchIncomplete: incomplete, RequestCapped: requestCapped, Requests: requests,
		RequestBudget: in.MaxRequests, PlannedRequests: plannedRequests,
	}, nil
}

type authoredPullRequestSyncResult struct {
	Status             string                         `json:"status"`
	Login              string                         `json:"login"`
	PullRequests       int                            `json:"pull_requests"`
	PullRequestRefs    []string                       `json:"pull_request_refs"`
	PullRequestTargets []mcpcontract.ThreadRef        `json:"-"`
	Repositories       []authoredRepositorySyncResult `json:"repositories"`
	SearchIncomplete   bool                           `json:"search_incomplete"`
	RequestCapped      bool                           `json:"request_capped"`
	Requests           int                            `json:"requests"`
	RequestBudget      int                            `json:"request_budget"`
	PlannedRequests    int                            `json:"planned_requests"`
}

type authoredRepositorySyncResult struct {
	Key          string `json:"key"`
	Status       string `json:"status"`
	Reason       string `json:"reason,omitempty"`
	Message      string `json:"message,omitempty"`
	RetryAfterMS int    `json:"retry_after_ms,omitempty"`
	Updated      int    `json:"updated,omitempty"`
	Requests     int    `json:"requests,omitempty"`
}
