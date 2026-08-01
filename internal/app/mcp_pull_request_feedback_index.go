package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

const jobKindIndexPullRequestFeedback = "index_pull_request_feedback"

type pullRequestFeedbackIndexItem struct {
	Key           string                      `json:"key"`
	Status        mcpcontract.BatchItemStatus `json:"item_status"`
	FeedbackItems int                         `json:"feedback_items,omitempty"`
	HeadSHA       string                      `json:"head_sha,omitempty"`
	ResourceURI   string                      `json:"resource_uri,omitempty"`
	Code          string                      `json:"code,omitempty"`
	Message       string                      `json:"message,omitempty"`
	Recovery      *mcpcontract.RecoveryPlan   `json:"recovery,omitempty"`
	RetryAfterMS  int                         `json:"retry_after_ms,omitempty"`
}

type pullRequestFeedbackIndexResult struct {
	Status          string                         `json:"status"`
	DiscoveryStatus string                         `json:"discovery_status"`
	NextPage        int                            `json:"next_page,omitempty"`
	PullRequests    int                            `json:"pull_requests"`
	FeedbackItems   int                            `json:"feedback_items"`
	Requests        int                            `json:"requests"`
	Items           []pullRequestFeedbackIndexItem `json:"items"`
	Recovery        *mcpcontract.RecoveryPlan      `json:"recovery,omitempty"`
}

// IndexPullRequestFeedback submits a repository-scoped durable discovery and
// feedback job. Repeating the same repository resumes its stored provider
// page when the previous run was bounded or interrupted.
func (r *MCPReader) IndexPullRequestFeedback(ctx context.Context, in mcpcontract.IndexPullRequestFeedbackInput) (mcpcontract.JobReference, error) {
	ref := domain.RepoRef{Owner: in.Repository.Owner, Repo: in.Repository.Repo}
	if err := ref.Validate(); err != nil {
		return mcpcontract.JobReference{}, err
	}
	if len(in.Channels) == 0 {
		in.Channels = []string{"issue_comments", "submitted_reviews", "inline_comments", "review_threads"}
	}
	if err := validateFeedbackChannels(in.Channels); err != nil {
		return mcpcontract.JobReference{}, err
	}
	if in.ThreadState == "" {
		in.ThreadState = "all"
	}
	if in.ThreadState != "all" && in.ThreadState != "unresolved" {
		return mcpcontract.JobReference{}, errors.New("thread_state must be unresolved or all")
	}
	if in.MaxPullRequests == 0 {
		in.MaxPullRequests = 1000
	}
	if in.MaxPullRequests < 1 || in.MaxPullRequests > 1000 {
		return mcpcontract.JobReference{}, errors.New("max_pull_requests must be between 1 and 1000")
	}
	if in.MaxItemsPerChannel == 0 {
		in.MaxItemsPerChannel = 300
	}
	if in.MaxItemsPerChannel < 1 || in.MaxItemsPerChannel > 1000 {
		return mcpcontract.JobReference{}, errors.New("max_items_per_channel must be between 1 and 1000")
	}
	if in.MaxPages == 0 {
		in.MaxPages = 100
	}
	if in.MaxPages < 1 || in.MaxPages > 1000 {
		return mcpcontract.JobReference{}, errors.New("max_pages must be between 1 and 1000")
	}
	if in.MaxRequests == 0 {
		in.MaxRequests = 1000
	}
	if in.MaxRequests < 1 || in.MaxRequests > 1000 {
		return mcpcontract.JobReference{}, errors.New("max_requests must be between 1 and 1000")
	}
	id, err := r.submitJob(ctx, jobKindIndexPullRequestFeedback, in, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.indexPullRequestFeedback(ctx, in, report)
	})
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	return queuedJobReference(id, jobKindIndexPullRequestFeedback, "repository pull-request feedback indexing job started"), nil
}

func (r *MCPReader) indexPullRequestFeedback(ctx context.Context, in mcpcontract.IndexPullRequestFeedbackInput, report func(string, string) error) (pullRequestFeedbackIndexResult, error) {
	reader, err := r.githubReader() //nolint:contextcheck // Client construction performs no request; operations below receive ctx.
	if err != nil {
		return pullRequestFeedbackIndexResult{}, err
	}
	indexer, ok := reader.(github.PullRequestIndexer)
	if !ok {
		return pullRequestFeedbackIndexResult{}, errors.New("GitHub reader does not support repository pull-request discovery")
	}
	feedbackReader, ok := reader.(github.PullRequestFeedbackReader)
	if !ok {
		return pullRequestFeedbackIndexResult{}, errors.New("GitHub reader does not support pull-request feedback")
	}
	c, err := r.openCorpus(ctx)
	if err != nil {
		return pullRequestFeedbackIndexResult{}, err
	}
	repo, err := c.GetRepository(ctx, in.Repository.Owner, in.Repository.Repo)
	if err != nil {
		return pullRequestFeedbackIndexResult{}, err
	}
	if repo == nil {
		payload, marshalErr := json.Marshal(map[string]string{"source": jobKindIndexPullRequestFeedback, "owner": in.Repository.Owner, "repo": in.Repository.Repo})
		if marshalErr != nil {
			return pullRequestFeedbackIndexResult{}, marshalErr
		}
		repo, err = c.UpsertRepository(ctx, corpus.Repository{Owner: in.Repository.Owner, Name: in.Repository.Repo}, string(payload))
		if err != nil {
			return pullRequestFeedbackIndexResult{}, err
		}
	}
	discovery, err := c.GetFeedbackDiscovery(ctx, repo.ID)
	if err != nil {
		return pullRequestFeedbackIndexResult{}, err
	}
	if discovery == nil || discovery.Complete || !sameFeedbackSelection(discovery.Channels, in.Channels) || discovery.ThreadState != in.ThreadState {
		discovery = &corpus.FeedbackDiscovery{RepositoryID: repo.ID, State: "all", NextPage: 1, Channels: append([]string(nil), in.Channels...), ThreadState: in.ThreadState}
	} else {
		discovery.Channels = append([]string(nil), in.Channels...)
		discovery.ThreadState = in.ThreadState
	}
	if discovery.NextPage < 1 {
		discovery.NextPage = 1
	}
	budget := github.NewRequestBudget(in.MaxRequests)
	result := pullRequestFeedbackIndexResult{Status: "complete", DiscoveryStatus: "complete", Items: make([]pullRequestFeedbackIndexItem, 0, in.MaxPullRequests)}
	page := discovery.NextPage
	initialRequests := discovery.Requests
	pages := 0
	processed := 0
	stopReason := ""
	for processed < in.MaxPullRequests && pages < in.MaxPages {
		if err := budget.Take(); err != nil {
			stopReason = "request_budget_exhausted"
			break
		}
		listed, listErr := indexer.ListPullRequests(ctx, in.Repository.Owner, in.Repository.Repo, github.PullRequestListOptions{State: "all", Sort: "updated", Direction: "desc", PageOptions: github.PageOptions{Page: page, PerPage: min(100, in.MaxPullRequests-processed)}})
		if listErr != nil {
			stopReason = classifyIndexError(listErr)
			break
		}
		pages++
		for _, issue := range listed.Items {
			if processed >= in.MaxPullRequests {
				stopReason = "pull_request_item_bound"
				break
			}
			if issue.Kind != github.ThreadKindPullRequest || issue.Number < 1 {
				continue
			}
			processed++
			ref := mcpcontract.ThreadRef{Owner: in.Repository.Owner, Repo: in.Repository.Repo, Kind: "pull_request", Number: issue.Number}
			item := r.indexOnePullRequestFeedback(ctx, feedbackReader, ref, in, budget)
			result.Items = append(result.Items, item)
			result.PullRequests++
			result.FeedbackItems += item.FeedbackItems
			if item.Status != "complete" {
				result.Status = "partial"
			}
			if err := report("pull_request_feedback", jobProgressCounts(result.PullRequests, in.MaxPullRequests)); err != nil {
				return pullRequestFeedbackIndexResult{}, err
			}
			if errors.Is(budgetError(item), github.ErrRequestBudgetExhausted) {
				stopReason = "request_budget_exhausted"
				break
			}
		}
		if stopReason != "" {
			break
		}
		if !listed.Page.HasNext {
			discovery.NextPage, discovery.Complete, discovery.Truncated = page, true, false
			break
		}
		page = listed.Page.NextPage
		discovery.NextPage, discovery.Complete, discovery.Truncated = page, false, true
		if pages >= in.MaxPages {
			stopReason = "discovery_page_bound"
			break
		}
		discovery.DiscoveredPullRequests, err = c.CountThreadsFiltered(ctx, repo.ID, corpus.ThreadKindPullRequest, "")
		if err != nil {
			return pullRequestFeedbackIndexResult{}, err
		}
		discovery.Requests = initialRequests + budget.Completed()
		discovery.SourceUpdatedAt = time.Now().UTC()
		discovery.UpdatedAt = discovery.SourceUpdatedAt
		if err := c.UpsertFeedbackDiscovery(ctx, *discovery); err != nil {
			return pullRequestFeedbackIndexResult{}, err
		}
	}
	if stopReason != "" {
		result.Status, result.DiscoveryStatus = "partial", "partial"
		discovery.Complete, discovery.Truncated, discovery.LastError = false, true, stopReason
		result.NextPage = discovery.NextPage
		result.Recovery = mcpcontractRecoveryIndex(in)
	} else if !discovery.Complete {
		result.Status, result.DiscoveryStatus = "partial", "partial"
		result.NextPage = discovery.NextPage
		result.Recovery = mcpcontractRecoveryIndex(in)
	}
	if result.Recovery == nil {
		for _, item := range result.Items {
			if item.Recovery != nil {
				result.Recovery = item.Recovery
				break
			}
		}
	}
	discovery.DiscoveredPullRequests, err = c.CountThreadsFiltered(ctx, repo.ID, corpus.ThreadKindPullRequest, "")
	if err != nil {
		return pullRequestFeedbackIndexResult{}, err
	}
	discovery.Requests = initialRequests + budget.Completed()
	if result.Status == "complete" {
		discovery.LastError = ""
	}
	discovery.SourceUpdatedAt = time.Now().UTC()
	discovery.UpdatedAt = time.Now().UTC()
	if err := c.UpsertFeedbackDiscovery(ctx, *discovery); err != nil {
		return pullRequestFeedbackIndexResult{}, err
	}
	if _, err := c.RebuildPullRequestFeedbackProjection(ctx); err != nil {
		return pullRequestFeedbackIndexResult{}, err
	}
	result.Requests = budget.Completed()
	return result, nil
}

func (r *MCPReader) indexOnePullRequestFeedback(ctx context.Context, feedbackReader github.PullRequestFeedbackReader, ref mcpcontract.ThreadRef, in mcpcontract.IndexPullRequestFeedbackInput, budget *github.RequestBudget) pullRequestFeedbackIndexItem {
	item := pullRequestFeedbackIndexItem{Key: pullRequestKey(ref), Status: "complete"}
	snapshot, readErr := feedbackReader.GetPullRequestFeedback(ctx, ref.Owner, ref.Repo, ref.Number, github.PullRequestFeedbackOptions{Channels: in.Channels, ThreadState: in.ThreadState, MaxItemsPerChannel: in.MaxItemsPerChannel}, budget)
	item.FeedbackItems = len(snapshot.IssueComments) + len(snapshot.Reviews) + len(snapshot.InlineComments)
	for _, thread := range snapshot.ReviewThreads {
		item.FeedbackItems += len(thread.Comments)
	}
	item.HeadSHA = snapshot.HeadSHA
	if snapshot.Header.Number > 0 {
		if _, persistErr := r.persistPullRequestIdentity(ctx, ref, snapshot.Header, snapshot.SourceUpdatedAt); persistErr != nil {
			item.Status, item.Code, item.Message = "failed", "identity_persistence_failed", persistErr.Error()
			return item
		}
	}
	if readErr != nil {
		if len(snapshot.Coverage) > 0 {
			_ = r.persistPullRequestFeedback(ctx, ref, snapshot, coveredFeedbackChannels(in.Channels, snapshot.Coverage))
		}
		item = pullRequestFeedbackIndexFailure(ref, in, readErr)
		item.HeadSHA = snapshot.HeadSHA
		item.FeedbackItems = len(snapshot.IssueComments) + len(snapshot.Reviews) + len(snapshot.InlineComments)
		for _, thread := range snapshot.ReviewThreads {
			item.FeedbackItems += len(thread.Comments)
		}
		return item
	}
	if err := r.persistPullRequestFeedback(ctx, ref, snapshot, in.Channels); err != nil {
		item.Status, item.Code, item.Message = "failed", "feedback_persistence_failed", err.Error()
		return item
	}
	if !feedbackSnapshotComplete(snapshot, in.Channels) {
		item.Status, item.Code, item.Message = "retryable", "feedback_coverage_incomplete", "one or more feedback channels reached an item bound"
		item.Recovery = mcpcontractRecoveryExact(ref, in)
		return item
	}
	item.ResourceURI = fmt.Sprintf("gitcontribute://pull-request-feedback/%s/%s/%d", ref.Owner, ref.Repo, ref.Number)
	return item
}

func pullRequestFeedbackIndexFailure(ref mcpcontract.ThreadRef, in mcpcontract.IndexPullRequestFeedbackInput, err error) pullRequestFeedbackIndexItem {
	item := pullRequestFeedbackIndexItem{Key: pullRequestKey(ref), Status: "failed", Code: "provider_error", Message: err.Error()}
	if errors.Is(err, github.ErrRequestBudgetExhausted) || isRetryableGitHubError(err) {
		item.Status, item.Code, item.Recovery, item.RetryAfterMS = "retryable", "request_or_provider_retryable", mcpcontractRecoveryExact(ref, in), 1000
	}
	return item
}

func sameFeedbackSelection(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := make(map[string]struct{}, len(left))
	for _, value := range left {
		seen[value] = struct{}{}
	}
	for _, value := range right {
		if _, ok := seen[value]; !ok {
			return false
		}
	}
	return true
}

func isRetryableGitHubError(err error) bool {
	var transient *github.TransientError
	var primary *github.PrimaryRateLimitError
	var secondary *github.SecondaryRateLimitError
	return errors.As(err, &transient) || errors.As(err, &primary) || errors.As(err, &secondary)
}

func classifyIndexError(err error) string {
	if errors.Is(err, github.ErrRequestBudgetExhausted) {
		return "request_budget_exhausted"
	}
	if isRetryableGitHubError(err) {
		return "provider_retryable"
	}
	return "discovery_failed"
}

func budgetError(item pullRequestFeedbackIndexItem) error {
	if item.Code == "request_or_provider_retryable" || item.Code == "request_budget_exhausted" {
		return github.ErrRequestBudgetExhausted
	}
	return nil
}

func mcpcontractRecoveryIndex(in mcpcontract.IndexPullRequestFeedbackInput) *mcpcontract.RecoveryPlan {
	return recoveryPlan("feedback_discovery_incomplete", "Pull-request discovery was bounded or incomplete. Continue this repository index job before treating missing feedback as absence.", mcpcontract.RecoveryAction(in))
}

func mcpcontractRecoveryExact(ref mcpcontract.ThreadRef, in mcpcontract.IndexPullRequestFeedbackInput) *mcpcontract.RecoveryPlan {
	return recoveryPlan("feedback_facet_incomplete", "Retry the exact pull-request feedback synchronization for this incomplete facet.", mcpcontract.RecoveryAction(mcpcontract.SyncPullRequestFeedbackInput{PullRequests: []mcpcontract.ThreadRef{ref}, Channels: append([]string(nil), in.Channels...), ThreadState: in.ThreadState, MaxItemsPerChannel: in.MaxItemsPerChannel, MaxRequests: in.MaxRequests}))
}
