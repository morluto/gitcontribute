package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// SyncPortfolio submits one bounded job that discovers pull requests authored
// by the active credential and refreshes health for the resulting stored set.
func (r *MCPReader) SyncPortfolio(ctx context.Context, in mcpcontract.SyncPortfolioInput) (mcpcontract.JobReference, error) {
	in, err := normalizeSyncPortfolioInput(in)
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	id, err := r.submitJob(ctx, jobKindSyncPullRequestPortfolio, in, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.runPortfolioSync(ctx, in, report)
	})
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	return queuedJobReference(id, jobKindSyncPullRequestPortfolio, "portfolio synchronization job started"), nil
}

func normalizeSyncPortfolioInput(in mcpcontract.SyncPortfolioInput) (mcpcontract.SyncPortfolioInput, error) {
	if in.Selection == "" {
		return mcpcontract.SyncPortfolioInput{}, errors.New("selection is required: choose authored or explicit")
	}
	if in.Selection != "authored" && in.Selection != "explicit" {
		return mcpcontract.SyncPortfolioInput{}, errors.New("selection must be authored or explicit")
	}
	if in.Selection == "explicit" {
		return normalizeExplicitPortfolioInput(in)
	}
	if len(in.PullRequests) > 0 {
		return mcpcontract.SyncPortfolioInput{}, errors.New("pull_requests is only valid in explicit mode")
	}
	return normalizeAuthoredPortfolioInput(in)
}

func normalizeExplicitPortfolioInput(in mcpcontract.SyncPortfolioInput) (mcpcontract.SyncPortfolioInput, error) {
	if len(in.PullRequests) < 1 || len(in.PullRequests) > 100 {
		return mcpcontract.SyncPortfolioInput{}, errors.New("pull_requests must contain 1 to 100 items in explicit mode")
	}
	in.PullRequests = canonicalPullRequestRefs(in.PullRequests)
	if err := rejectDuplicateThreadRefs(in.PullRequests); err != nil {
		return mcpcontract.SyncPortfolioInput{}, err
	}
	if err := validatePullRequestRefs(in.PullRequests, "pull_requests"); err != nil {
		return mcpcontract.SyncPortfolioInput{}, err
	}
	if in.State != "" || in.UpdatedAfter != "" || in.Limit != 0 || in.DiscoveryMaxRequests != 0 {
		return mcpcontract.SyncPortfolioInput{}, errors.New("state, updated_after, limit, and discovery_max_requests are only valid in authored mode")
	}
	if in.Repository != nil {
		return mcpcontract.SyncPortfolioInput{}, errors.New("repository is only valid in authored mode")
	}
	return normalizePortfolioStatusMaxPages(in)
}

func normalizeAuthoredPortfolioInput(in mcpcontract.SyncPortfolioInput) (mcpcontract.SyncPortfolioInput, error) {
	if in.Repository != nil {
		in.Repository.Owner = strings.TrimSpace(in.Repository.Owner)
		in.Repository.Repo = strings.TrimSpace(in.Repository.Repo)
		if err := (domain.RepoRef{Owner: in.Repository.Owner, Repo: in.Repository.Repo}).Validate(); err != nil {
			return mcpcontract.SyncPortfolioInput{}, err
		}
	}
	if in.State == "" {
		in.State = "open"
	}
	if in.State != "open" && in.State != "closed" && in.State != "all" {
		return mcpcontract.SyncPortfolioInput{}, errors.New("state must be open, closed, or all")
	}
	if in.UpdatedAfter != "" {
		if _, err := time.Parse(time.RFC3339, in.UpdatedAfter); err != nil {
			return mcpcontract.SyncPortfolioInput{}, errors.New("updated_after must be RFC 3339")
		}
	}
	if in.Limit == 0 {
		in.Limit = 100
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpcontract.SyncPortfolioInput{}, errors.New("limit must be between 1 and 100")
	}
	if in.DiscoveryMaxRequests == 0 {
		in.DiscoveryMaxRequests = defaultSyncBatchMaxRequests
	}
	if in.DiscoveryMaxRequests < 2 || in.DiscoveryMaxRequests > defaultSyncBatchMaxRequests {
		return mcpcontract.SyncPortfolioInput{}, fmt.Errorf("discovery_max_requests must be between 2 and %d", defaultSyncBatchMaxRequests)
	}
	return normalizePortfolioStatusMaxPages(in)
}

func normalizePortfolioStatusMaxPages(in mcpcontract.SyncPortfolioInput) (mcpcontract.SyncPortfolioInput, error) {
	if in.StatusMaxPages == 0 {
		in.StatusMaxPages = 3
	}
	if in.StatusMaxPages < 1 || in.StatusMaxPages > 20 {
		return mcpcontract.SyncPortfolioInput{}, errors.New("status_max_pages must be between 1 and 20")
	}
	return in, nil
}

func (r *MCPReader) runPortfolioSync(ctx context.Context, in mcpcontract.SyncPortfolioInput, report func(string, string) error) (syncPortfolioResult, error) {
	if in.Selection == "explicit" {
		return r.syncExplicitPortfolio(ctx, in, report)
	}
	return r.syncAuthoredPortfolio(ctx, in, report)
}

func (r *MCPReader) syncExplicitPortfolio(ctx context.Context, in mcpcontract.SyncPortfolioInput, report func(string, string) error) (syncPortfolioResult, error) {
	refreshed, failures, status, err := r.syncPortfolioStatusBatches(ctx, in.PullRequests, in.StatusMaxPages, report)
	if err != nil {
		return syncPortfolioResult{}, err
	}
	return syncPortfolioResult{Status: status, Discovered: len(in.PullRequests), Refreshed: refreshed, PullRequests: threadRefKeys(in.PullRequests), Failures: failures, DiscoveryStatus: "complete"}, nil
}

func (r *MCPReader) syncAuthoredPortfolio(ctx context.Context, in mcpcontract.SyncPortfolioInput, report func(string, string) error) (syncPortfolioResult, error) {
	discovery, err := r.syncAuthoredPullRequests(ctx, authoredPullRequestSyncOptions{
		Repository: in.Repository, State: in.State, UpdatedAfter: in.UpdatedAfter, Limit: in.Limit, MaxRequests: in.DiscoveryMaxRequests,
	}, report)
	if err != nil {
		return syncPortfolioResult{}, err
	}
	refreshed, failures, status, err := r.syncPortfolioStatusBatches(ctx, discovery.PullRequestTargets, in.StatusMaxPages, report)
	if err != nil {
		return syncPortfolioResult{}, err
	}
	if discovery.Status != "complete" || discovery.SearchIncomplete || discovery.RequestCapped {
		status = "partial"
	}
	return syncPortfolioResult{Status: status, Login: discovery.Login, Discovered: discovery.PullRequests, Refreshed: refreshed, PullRequests: append([]string(nil), discovery.PullRequestRefs...), Failures: failures, DiscoveryStatus: discovery.Status, SearchIncomplete: discovery.SearchIncomplete, RequestCapped: discovery.RequestCapped}, nil
}

func (r *MCPReader) syncPortfolioStatusBatches(ctx context.Context, refs []mcpcontract.ThreadRef, maxPages int, report func(string, string) error) (int, []pullRequestStatusFailure, string, error) {
	refreshed := 0
	failures := make([]pullRequestStatusFailure, 0)
	status := "complete"
	for start := 0; start < len(refs); start += 50 {
		end := min(start+50, len(refs))
		batch, err := r.syncPullRequestStatusBatch(ctx, pullRequestStatusBatchInput{PullRequests: refs[start:end], MaxPages: maxPages}, report)
		if err != nil {
			return 0, nil, "", err
		}
		refreshed += batch.Completed
		failures = append(failures, batch.Failures...)
		if batch.Status != "complete" {
			status = "partial"
		}
	}
	return refreshed, failures, status, nil
}

type syncPortfolioResult struct {
	Status           string                     `json:"status"`
	Login            string                     `json:"login"`
	Discovered       int                        `json:"discovered"`
	Refreshed        int                        `json:"refreshed"`
	PullRequests     []string                   `json:"pull_requests"`
	Failures         []pullRequestStatusFailure `json:"failures,omitempty"`
	DiscoveryStatus  string                     `json:"discovery_status"`
	SearchIncomplete bool                       `json:"search_incomplete"`
	RequestCapped    bool                       `json:"request_capped"`
}
