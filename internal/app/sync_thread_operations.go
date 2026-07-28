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
	ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Repo}
	if err := ref.Validate(); err != nil {
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
	stored, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return nil, err
	}
	if stored == nil {
		payload, err := json.Marshal(map[string]any{"source": "authored_pull_request_search", "owner": ref.Owner, "repo": ref.Repo})
		if err != nil {
			return nil, err
		}
		stored, err = c.UpsertRepository(ctx, corpus.Repository{
			Owner: ref.Owner, Name: ref.Repo,
		}, string(payload))
		if err != nil {
			return nil, fmt.Errorf("store authored repository identity: %w", err)
		}
	}
	run, err := c.StartRun(ctx, "sync_authored_pull_request_headers")
	if err != nil {
		return nil, err
	}
	defer func() {
		if resultErr != nil {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = c.FailRun(cleanupCtx, run.ID, resultErr.Error())
		}
	}()
	writer := &syncThreadWriter{
		ctx: ctx, corpus: c, repositoryID: stored.ID, kind: "pull_request", sourceUpdatedAt: sourceUpdatedAt,
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
		Repo: repo, Updated: writer.updated, Requests: 0, PlannedRequests: 0, RequestBudget: 0,
		Message: fmt.Sprintf("stored %d provided thread headers", writer.updated),
	}, nil
}

func (s *Service) syncThreadHeaders(ctx context.Context, repo contracts.RepoRef, syncOpts SyncOptions) (_ *contracts.SyncResult, resultErr error) {
	ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Repo}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	var plan syncRequestPlan
	var err error
	syncOpts, plan, err = planThreadSyncOptions(syncOpts)
	if err != nil {
		return nil, err
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	repoProjection, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
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
	defer func() {
		if resultErr != nil {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = c.FailRun(cleanupCtx, run.ID, resultErr.Error())
		}
	}()
	budget := newSyncRequestBudget(syncOpts.MaxRequests)
	selection, err := syncThreadHeaderSelection(ctx, c, reader, ref, repoProjection.ID, repoProjection.SourceUpdatedAt, syncOpts, nil, budget)
	if err != nil {
		return nil, err
	}
	if err := c.AdvanceFacet(ctx, repoProjection.ID, nil, "threads", selection.sourceUpdatedAt, selection.complete, run.ID); err != nil {
		return nil, fmt.Errorf("advance threads facet: %w", err)
	}
	requestCapped := selection.requestCapped || (len(syncOpts.Numbers) == 0 && plan.threadRequestCeiling < syncOpts.MaxPages)
	stats, err := json.Marshal(map[string]any{
		"pages": selection.requests, "threads": selection.updated, "complete": selection.complete,
		"requests": budget.used, "request_budget": budget.limit, "request_capped": requestCapped,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal sync statistics: %w", err)
	}
	if err := c.FinishRun(ctx, run.ID, string(stats)); err != nil {
		return nil, err
	}
	return &contracts.SyncResult{
		Repo: repo, Updated: selection.updated, Requests: budget.used, PlannedRequests: plan.plannedRequests,
		RequestBudget: syncOpts.MaxRequests, Capped: requestCapped,
		Message: fmt.Sprintf("fetched %d thread headers across %d thread requests", selection.updated, selection.requests),
	}, nil
}
