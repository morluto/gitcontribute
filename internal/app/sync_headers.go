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
)

func syncRepositoryHeader(ctx context.Context, c *corpus.Corpus, reader github.Reader, ref domain.RepoRef, runID int64, budget *syncRequestBudget) (corpus.Repository, time.Time, error) {
	if err := budget.take(); err != nil {
		return corpus.Repository{}, time.Time{}, err
	}
	ghRepo, _, err := reader.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return corpus.Repository{}, time.Time{}, fmt.Errorf("get repository: %w", err)
	}
	repoPayload, err := json.Marshal(ghRepo)
	if err != nil {
		return corpus.Repository{}, time.Time{}, fmt.Errorf("marshal repository: %w", err)
	}
	repo, err := c.UpsertRepository(ctx, corpusRepoFromGitHub(ghRepo), string(repoPayload))
	if err != nil {
		return corpus.Repository{}, time.Time{}, fmt.Errorf("upsert repository: %w", err)
	}
	if err := c.AdvanceFacet(ctx, repo.ID, nil, "metadata", ghRepo.UpdatedAt, true, runID); err != nil {
		return corpus.Repository{}, time.Time{}, fmt.Errorf("advance metadata facet: %w", err)
	}
	if err := syncRepositoryGuidance(ctx, c, reader, *repo, ref, ghRepo.UpdatedAt, runID, budget); err != nil {
		return corpus.Repository{}, time.Time{}, err
	}
	return *repo, ghRepo.UpdatedAt, nil
}

type syncThreadSelection struct {
	updated         int
	requests        int
	sourceUpdatedAt time.Time
	complete        bool
	requestCapped   bool
}

type syncThreadWriter struct {
	ctx             context.Context
	corpus          *corpus.Corpus
	repositoryID    int64
	kind            string
	updated         int
	sourceUpdatedAt time.Time
}

func syncThreadHeaderSelection(ctx context.Context, c *corpus.Corpus, reader github.Reader, ref domain.RepoRef, repoID int64, sourceUpdatedAt time.Time, opts SyncOptions, provided []github.Issue, budget *syncRequestBudget) (syncThreadSelection, error) {
	writer := &syncThreadWriter{ctx: ctx, corpus: c, repositoryID: repoID, kind: opts.Kind, sourceUpdatedAt: sourceUpdatedAt}
	if provided != nil {
		if err := writer.storeAll(provided); err != nil {
			return syncThreadSelection{}, err
		}
		return writer.result(0, false, false), nil
	}
	if len(opts.Numbers) > 0 {
		requests, err := syncExactThreadHeaders(ctx, reader, ref, opts.Numbers, budget, writer)
		return writer.result(requests, false, false), err
	}
	return syncListedThreadHeaders(ctx, reader, ref, opts, budget, writer)
}

func (w *syncThreadWriter) storeAll(issues []github.Issue) error {
	for _, issue := range issues {
		if err := w.store(issue); err != nil {
			return err
		}
	}
	return nil
}

func (w *syncThreadWriter) store(issue github.Issue) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	if w.kind != "both" && string(issue.Kind) != w.kind {
		return nil
	}
	thread, payload, err := threadFromIssue(issue)
	if err != nil {
		return err
	}
	thread.RepositoryID = w.repositoryID
	if _, err := w.corpus.UpsertThread(w.ctx, thread, payload); err != nil {
		return fmt.Errorf("upsert thread: %w", err)
	}
	w.updated++
	if thread.SourceUpdatedAt.After(w.sourceUpdatedAt) {
		w.sourceUpdatedAt = thread.SourceUpdatedAt
	}
	return nil
}

func (w *syncThreadWriter) result(requests int, complete, requestCapped bool) syncThreadSelection {
	return syncThreadSelection{
		updated: w.updated, requests: requests, sourceUpdatedAt: w.sourceUpdatedAt,
		complete: complete, requestCapped: requestCapped,
	}
}

func syncExactThreadHeaders(ctx context.Context, reader github.Reader, ref domain.RepoRef, numbers []int, budget *syncRequestBudget, writer *syncThreadWriter) (int, error) {
	getter, ok := reader.(github.IssueGetter)
	if !ok {
		return 0, errors.New("GitHub reader does not support exact thread refresh")
	}
	requests := 0
	for _, number := range numbers {
		if err := ctx.Err(); err != nil {
			return requests, err
		}
		if err := budget.take(); err != nil {
			return requests, err
		}
		issue, _, err := getter.GetIssue(ctx, ref.Owner, ref.Repo, number)
		if err != nil {
			return requests, fmt.Errorf("get thread %d: %w", number, err)
		}
		requests++
		if err := writer.store(issue); err != nil {
			return requests, err
		}
	}
	return requests, nil
}

func syncListedThreadHeaders(ctx context.Context, reader github.Reader, ref domain.RepoRef, opts SyncOptions, budget *syncRequestBudget, writer *syncThreadWriter) (syncThreadSelection, error) {
	perPage := 100
	if opts.MaxItems > 0 {
		perPage = min(perPage, opts.MaxItems)
	}
	listOpts := github.ListIssueOptions{
		State: opts.State, Sort: "updated", Direction: "desc", Since: opts.Since,
		PageOptions: github.PageOptions{Page: 1, PerPage: perPage},
	}
	requests, truncated, requestCapped := 0, false, false
	for {
		if !budget.available() {
			truncated, requestCapped = true, true
			break
		}
		if err := budget.take(); err != nil {
			return syncThreadSelection{}, err
		}
		res, err := reader.ListIssues(ctx, ref.Owner, ref.Repo, listOpts)
		if err != nil {
			return syncThreadSelection{}, fmt.Errorf("list issues page %d: %w", listOpts.Page, err)
		}
		requests++
		reachedLimit := false
		for index, issue := range res.Items {
			if err := writer.store(issue); err != nil {
				return syncThreadSelection{}, err
			}
			if opts.MaxItems > 0 && writer.updated >= opts.MaxItems {
				truncated = res.Page.HasNext || index < len(res.Items)-1
				reachedLimit = true
				break
			}
		}
		if reachedLimit {
			break
		}
		if !res.Page.HasNext {
			break
		}
		if requests >= opts.MaxPages {
			truncated = true
			break
		}
		if !budget.available() {
			truncated, requestCapped = true, true
			break
		}
		listOpts.Page = res.Page.NextPage
		if opts.MaxItems > 0 {
			listOpts.PerPage = min(100, opts.MaxItems-writer.updated)
		}
	}
	complete := opts.Kind == "both" && opts.State == "all" && opts.Since.IsZero() && !truncated
	return writer.result(requests, complete, requestCapped), nil
}
