package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
)

func (s *Service) syncProvidedThreadHeaders(ctx context.Context, repo contracts.RepoRef, issues []github.Issue) (_ *contracts.SyncResult, resultErr error) {
	ref, err := domain.NewRepoRef(repo.Owner, repo.Repo)
	if err != nil {
		return nil, err
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	sourceUpdatedAt := time.Time{}
	for _, issue := range issues {
		if issue.UpdatedAt.After(sourceUpdatedAt) {
			sourceUpdatedAt = issue.UpdatedAt
		}
	}
	stored, err := c.GetRepository(ctx, ref.Owner(), ref.Repo())
	if err != nil {
		return nil, err
	}
	if stored == nil {
		payload, err := json.Marshal(struct {
			Source string `json:"source"`
			Owner  string `json:"owner"`
			Repo   string `json:"repo"`
		}{Source: "authored_pull_request_search", Owner: ref.Owner(), Repo: ref.Repo()})
		if err != nil {
			return nil, err
		}
		stored, err = c.UpsertRepository(ctx, corpus.Repository{
			Owner: ref.Owner(), Name: ref.Repo(),
		}, string(payload))
		if err != nil {
			return nil, fmt.Errorf("store authored repository identity: %w", err)
		}
	}
	run, err := c.StartRun(ctx, "sync_authored_pull_request_headers")
	if err != nil {
		return nil, err
	}
	defer failRunOnError(ctx, c, run.ID, &resultErr)
	writer := &syncThreadWriter{
		ctx: ctx, corpus: c, owner: ref.Owner(), repo: ref.Repo(), repositoryID: stored.ID, kind: syncPullRequests, sourceUpdatedAt: sourceUpdatedAt,
	}
	if err := writer.storeAll(issues); err != nil {
		return nil, err
	}
	if err := c.AdvanceFacet(ctx, stored.ID, nil, "threads", sourceUpdatedAt, false, run.ID); err != nil {
		return nil, err
	}
	if err := c.FinishRun(ctx, run.ID, `{"requests":0,"complete":false}`); err != nil {
		return nil, err
	}
	return &contracts.SyncResult{
		Repo: repo, Threads: writer.threads, Updated: writer.updated, Requests: 0, PlannedRequests: 0, RequestBudget: 0,
		Message: fmt.Sprintf("stored %d provided thread headers", writer.updated),
	}, nil
}

func (s *Service) syncThreadHeaders(ctx context.Context, repo contracts.RepoRef, input threadSyncInput) (*contracts.SyncResult, error) {
	ref, err := domain.NewRepoRef(repo.Owner, repo.Repo)
	if err != nil {
		return nil, err
	}
	request, plan, err := parseThreadSync(input)
	if err != nil {
		return nil, err
	}
	return s.executeThreadSync(ctx, ref, request, plan)
}

func (s *Service) executeThreadSync(ctx context.Context, ref domain.RepoRef, request threadSyncRequest, plan syncRequestPlan) (_ *contracts.SyncResult, resultErr error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	repoProjection, err := c.GetRepository(ctx, ref.Owner(), ref.Repo())
	if err != nil {
		return nil, fmt.Errorf("get repository: %w", err)
	}
	if repoProjection == nil {
		return nil, fmt.Errorf(
			"repository %s is not stored; run `gitcontribute archive sync-context %s`",
			ref, ref,
		)
	}
	reader, err := s.githubReader() //nolint:contextcheck // Client construction performs no request; operations below receive ctx.
	if err != nil {
		return nil, err
	}
	run, err := c.StartRun(ctx, "sync_threads")
	if err != nil {
		return nil, err
	}
	defer failRunOnError(ctx, c, run.ID, &resultErr)
	budget := newSyncRequestBudget(request.maxRequests)
	selection, err := syncThreadHeaderSelection(ctx, c, reader, ref, repoProjection.ID, repoProjection.SourceUpdatedAt, request, budget)
	if err != nil {
		return nil, err
	}
	if err := c.AdvanceFacet(ctx, repoProjection.ID, nil, "threads", selection.sourceUpdatedAt, selection.complete, run.ID); err != nil {
		return nil, fmt.Errorf("advance threads facet: %w", err)
	}
	requestCapped := selection.requestCapped || request.planCapped(plan)
	stats, err := json.Marshal(struct {
		Pages         int  `json:"pages"`
		Threads       int  `json:"threads"`
		Complete      bool `json:"complete"`
		Requests      int  `json:"requests"`
		RequestBudget int  `json:"request_budget"`
		RequestCapped bool `json:"request_capped"`
	}{
		Pages: selection.requests, Threads: selection.updated, Complete: selection.complete,
		Requests: budget.used, RequestBudget: budget.limit, RequestCapped: requestCapped,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal sync statistics: %w", err)
	}
	if err := c.FinishRun(ctx, run.ID, string(stats)); err != nil {
		return nil, err
	}
	return &contracts.SyncResult{
		Repo: contracts.RepoRef{Owner: ref.Owner(), Repo: ref.Repo()}, Threads: selection.threads, Updated: selection.updated, Requests: budget.used, PlannedRequests: plan.plannedRequests,
		RequestBudget: request.maxRequests, Capped: requestCapped,
		Message: fmt.Sprintf("fetched %d thread headers across %d thread requests", selection.updated, selection.requests),
	}, nil
}
