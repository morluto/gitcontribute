package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// SyncPortfolio submits one bounded job that discovers pull requests authored
// by the active credential and refreshes health for the resulting stored set.
func (r *MCPReader) SyncPortfolio(ctx context.Context, in mcpcontract.SyncPortfolioInput) (mcpcontract.JobReference, error) {
	request, normalized, err := parseSyncPortfolioInput(in)
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	id, err := r.submitJob(ctx, jobKindSyncPullRequestPortfolio, normalized, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.runPortfolioSync(ctx, request, report)
	})
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	return queuedJobReference(id, jobKindSyncPullRequestPortfolio, "portfolio synchronization job started"), nil
}

type syncPortfolioRequest struct {
	selection      syncPortfolioSelection
	statusMaxPages int
}

type syncPortfolioSelection interface {
	isSyncPortfolioSelection()
}

type explicitPortfolioSelection struct {
	pullRequests []mcpcontract.ThreadRef
}

func (explicitPortfolioSelection) isSyncPortfolioSelection() {}

type authoredPortfolioSelection struct {
	repository   *mcpcontract.RepositoryRef
	state        syncThreadState
	updatedAfter time.Time
	limit        int
	maxRequests  int
}

func (authoredPortfolioSelection) isSyncPortfolioSelection() {}

func parseSyncPortfolioInput(in mcpcontract.SyncPortfolioInput) (syncPortfolioRequest, mcpcontract.SyncPortfolioInput, error) {
	if in.Selection == "" {
		return syncPortfolioRequest{}, mcpcontract.SyncPortfolioInput{}, errors.New("selection is required: choose authored or explicit")
	}
	if in.Selection != "authored" && in.Selection != "explicit" {
		return syncPortfolioRequest{}, mcpcontract.SyncPortfolioInput{}, errors.New("selection must be authored or explicit")
	}
	if in.StatusMaxPages == 0 {
		in.StatusMaxPages = 3
	}
	if in.StatusMaxPages < 1 || in.StatusMaxPages > 20 {
		return syncPortfolioRequest{}, mcpcontract.SyncPortfolioInput{}, errors.New("status_max_pages must be between 1 and 20")
	}
	if in.Selection == "explicit" {
		selection, normalized, err := parseExplicitPortfolioSelection(in)
		if err != nil {
			return syncPortfolioRequest{}, mcpcontract.SyncPortfolioInput{}, err
		}
		return syncPortfolioRequest{selection: selection, statusMaxPages: in.StatusMaxPages}, normalized, nil
	}
	if len(in.PullRequests) > 0 {
		return syncPortfolioRequest{}, mcpcontract.SyncPortfolioInput{}, errors.New("pull_requests is only valid in explicit mode")
	}
	selection, normalized, err := parseAuthoredPortfolioSelection(in)
	if err != nil {
		return syncPortfolioRequest{}, mcpcontract.SyncPortfolioInput{}, err
	}
	return syncPortfolioRequest{selection: selection, statusMaxPages: in.StatusMaxPages}, normalized, nil
}

func parseExplicitPortfolioSelection(in mcpcontract.SyncPortfolioInput) (explicitPortfolioSelection, mcpcontract.SyncPortfolioInput, error) {
	if len(in.PullRequests) < 1 || len(in.PullRequests) > 100 {
		return explicitPortfolioSelection{}, mcpcontract.SyncPortfolioInput{}, errors.New("pull_requests must contain 1 to 100 items in explicit mode")
	}
	refs, err := parsePullRequestRefs(in.PullRequests, "pull_requests")
	if err != nil {
		return explicitPortfolioSelection{}, mcpcontract.SyncPortfolioInput{}, err
	}
	in.PullRequests = refs
	if in.State != "" || in.UpdatedAfter != "" || in.Limit != 0 || in.DiscoveryMaxRequests != 0 {
		return explicitPortfolioSelection{}, mcpcontract.SyncPortfolioInput{}, errors.New("state, updated_after, limit, and discovery_max_requests are only valid in authored mode")
	}
	if in.Repository != nil {
		return explicitPortfolioSelection{}, mcpcontract.SyncPortfolioInput{}, errors.New("repository is only valid in authored mode")
	}
	return explicitPortfolioSelection{pullRequests: append([]mcpcontract.ThreadRef(nil), in.PullRequests...)}, in, nil
}

func parseAuthoredPortfolioSelection(in mcpcontract.SyncPortfolioInput) (authoredPortfolioSelection, mcpcontract.SyncPortfolioInput, error) {
	if in.Repository != nil {
		ref, err := domain.NewRepoRef(in.Repository.Owner, in.Repository.Repo)
		if err != nil {
			return authoredPortfolioSelection{}, mcpcontract.SyncPortfolioInput{}, err
		}
		in.Repository = &mcpcontract.RepositoryRef{Owner: ref.Owner(), Repo: ref.Repo()}
	}
	if in.State == "" {
		in.State = "open"
	}
	state, err := parseSyncThreadState(in.State)
	if err != nil {
		return authoredPortfolioSelection{}, mcpcontract.SyncPortfolioInput{}, err
	}
	var updatedAfter time.Time
	if in.UpdatedAfter != "" {
		parsed, err := time.Parse(time.RFC3339, in.UpdatedAfter)
		if err != nil {
			return authoredPortfolioSelection{}, mcpcontract.SyncPortfolioInput{}, errors.New("updated_after must be RFC 3339")
		}
		updatedAfter = parsed
	}
	if in.Limit == 0 {
		in.Limit = 100
	}
	if in.Limit < 1 || in.Limit > 100 {
		return authoredPortfolioSelection{}, mcpcontract.SyncPortfolioInput{}, errors.New("limit must be between 1 and 100")
	}
	if in.DiscoveryMaxRequests == 0 {
		in.DiscoveryMaxRequests = defaultSyncBatchMaxRequests
	}
	if in.DiscoveryMaxRequests < 2 || in.DiscoveryMaxRequests > defaultSyncBatchMaxRequests {
		return authoredPortfolioSelection{}, mcpcontract.SyncPortfolioInput{}, fmt.Errorf("discovery_max_requests must be between 2 and %d", defaultSyncBatchMaxRequests)
	}
	var repository *mcpcontract.RepositoryRef
	if in.Repository != nil {
		copy := *in.Repository
		repository = &copy
	}
	return authoredPortfolioSelection{repository: repository, state: state, updatedAfter: updatedAfter, limit: in.Limit, maxRequests: in.DiscoveryMaxRequests}, in, nil
}

func (r *MCPReader) runPortfolioSync(ctx context.Context, request syncPortfolioRequest, report func(string, string) error) (syncPortfolioResult, error) {
	switch selection := request.selection.(type) {
	case explicitPortfolioSelection:
		return r.syncExplicitPortfolio(ctx, selection, request.statusMaxPages, report)
	case authoredPortfolioSelection:
		return r.syncAuthoredPortfolio(ctx, selection, request.statusMaxPages, report)
	default:
		return syncPortfolioResult{}, errors.New("unsupported parsed portfolio selection")
	}
}

func (r *MCPReader) syncExplicitPortfolio(ctx context.Context, selection explicitPortfolioSelection, statusMaxPages int, report func(string, string) error) (syncPortfolioResult, error) {
	refreshed, failures, status, err := r.syncPortfolioStatusBatches(ctx, selection.pullRequests, statusMaxPages, report)
	if err != nil {
		return syncPortfolioResult{}, err
	}
	return syncPortfolioResult{Status: status, Discovered: len(selection.pullRequests), Refreshed: refreshed, PullRequests: threadRefKeys(selection.pullRequests), Failures: failures, DiscoveryStatus: batchOperationComplete}, nil
}

func (r *MCPReader) syncAuthoredPortfolio(ctx context.Context, selection authoredPortfolioSelection, statusMaxPages int, report func(string, string) error) (syncPortfolioResult, error) {
	discovery, err := r.syncAuthoredPullRequests(ctx, authoredPullRequestSyncOptions{
		Repository: selection.repository, State: selection.state, UpdatedAfter: selection.updatedAfter, Limit: selection.limit, MaxRequests: selection.maxRequests,
	}, report)
	if err != nil {
		return syncPortfolioResult{}, err
	}
	refreshed, failures, status, err := r.syncPortfolioStatusBatches(ctx, discovery.PullRequestTargets, statusMaxPages, report)
	if err != nil {
		return syncPortfolioResult{}, err
	}
	if discovery.Status != batchOperationComplete || discovery.SearchIncomplete || discovery.RequestCapped {
		status = batchOperationPartial
	}
	return syncPortfolioResult{Status: status, Login: discovery.Login, Discovered: discovery.PullRequests, Refreshed: refreshed, PullRequests: append([]string(nil), discovery.PullRequestRefs...), Failures: failures, DiscoveryStatus: discovery.Status, SearchIncomplete: discovery.SearchIncomplete, RequestCapped: discovery.RequestCapped}, nil
}

func (r *MCPReader) syncPortfolioStatusBatches(ctx context.Context, refs []mcpcontract.ThreadRef, maxPages int, report func(string, string) error) (int, []pullRequestStatusFailure, batchOperationStatus, error) {
	refreshed := 0
	failures := make([]pullRequestStatusFailure, 0)
	status := batchOperationComplete
	for start := 0; start < len(refs); start += 50 {
		end := min(start+50, len(refs))
		batch, err := r.syncPullRequestStatusBatch(ctx, pullRequestStatusBatchInput{PullRequests: refs[start:end], MaxPages: maxPages}, report)
		if err != nil {
			return 0, nil, "", err
		}
		refreshed += batch.Completed
		failures = append(failures, batch.Failures...)
		if batch.Status != batchOperationComplete {
			status = batchOperationPartial
		}
	}
	return refreshed, failures, status, nil
}

type syncPortfolioResult struct {
	Status           batchOperationStatus       `json:"status"`
	Login            string                     `json:"login"`
	Discovered       int                        `json:"discovered"`
	Refreshed        int                        `json:"refreshed"`
	PullRequests     []string                   `json:"pull_requests"`
	Failures         []pullRequestStatusFailure `json:"failures,omitempty"`
	DiscoveryStatus  batchOperationStatus       `json:"discovery_status"`
	SearchIncomplete bool                       `json:"search_incomplete"`
	RequestCapped    bool                       `json:"request_capped"`
}
