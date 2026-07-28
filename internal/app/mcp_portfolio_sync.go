package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// SyncPortfolio submits one bounded job that discovers pull requests authored
// by the active credential and refreshes health for the resulting stored set.
func (r *MCPReader) SyncPortfolio(ctx context.Context, in mcpcontract.SyncPortfolioInput) (mcpcontract.JobReference, error) {
	if in.State == "" {
		in.State = "open"
	}
	if in.State != "open" && in.State != "closed" && in.State != "all" {
		return mcpcontract.JobReference{}, errors.New("state must be open, closed, or all")
	}
	if in.UpdatedAfter != "" {
		if _, err := time.Parse(time.RFC3339, in.UpdatedAfter); err != nil {
			return mcpcontract.JobReference{}, errors.New("updated_after must be RFC 3339")
		}
	}
	if in.Limit == 0 {
		in.Limit = 100
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpcontract.JobReference{}, errors.New("limit must be between 1 and 100")
	}
	if in.DiscoveryMaxRequests == 0 {
		in.DiscoveryMaxRequests = defaultSyncBatchMaxRequests
	}
	if in.DiscoveryMaxRequests < 2 || in.DiscoveryMaxRequests > defaultSyncBatchMaxRequests {
		return mcpcontract.JobReference{}, fmt.Errorf("discovery_max_requests must be between 2 and %d", defaultSyncBatchMaxRequests)
	}
	if in.StatusMaxPages == 0 {
		in.StatusMaxPages = 3
	}
	if in.StatusMaxPages < 1 || in.StatusMaxPages > 20 {
		return mcpcontract.JobReference{}, errors.New("status_max_pages must be between 1 and 20")
	}

	id, err := r.submitJob(ctx, "sync_portfolio", in, func(ctx context.Context, report func(string, string) error) (any, error) {
		discovery, err := r.syncAuthoredPullRequests(ctx, mcpcontract.SyncAuthoredPullRequestsInput{
			State: in.State, UpdatedAfter: in.UpdatedAfter, Limit: in.Limit, MaxRequests: in.DiscoveryMaxRequests,
		}, report)
		if err != nil {
			return nil, err
		}
		refs := append([]mcpcontract.ThreadRef(nil), discovery.PullRequestTargets...)
		references := append([]string(nil), discovery.PullRequestRefs...)
		status := "complete"
		if discovery.Status != "complete" || discovery.SearchIncomplete || discovery.RequestCapped {
			status = "partial"
		}
		refreshed := 0
		failures := make([]pullRequestStatusFailure, 0)
		for start := 0; start < len(refs); start += 50 {
			end := min(start+50, len(refs))
			batch, err := r.syncPullRequestStatusBatch(ctx, mcpcontract.SyncPullRequestStatusInput{
				PullRequests: refs[start:end], MaxPages: in.StatusMaxPages,
			}, report)
			if err != nil {
				return nil, err
			}
			refreshed += batch.Completed
			failures = append(failures, batch.Failures...)
			if batch.Status != "complete" {
				status = "partial"
			}
		}
		return syncPortfolioResult{
			Status: status, Login: discovery.Login, Discovered: discovery.PullRequests, Refreshed: refreshed,
			PullRequests: references, Failures: failures,
			DiscoveryStatus: discovery.Status, SearchIncomplete: discovery.SearchIncomplete,
			RequestCapped: discovery.RequestCapped,
		}, nil
	})
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	return queuedJobReference(id, "sync_portfolio", "portfolio synchronization job started"), nil
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
