package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/exporter"
	"github.com/morluto/gitcontribute/internal/repositorycontext"
)

func (s *Service) RepositoryContextSync(ctx context.Context, repo contracts.RepoRef, maxRequests int) (_ *contracts.RepositoryContextResult, resultErr error) {
	plan, err := planRepositoryContextSync(repo, maxRequests)
	if err != nil {
		return nil, err
	}
	ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Repo}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	reader, err := s.githubReader() //nolint:contextcheck // Client construction performs no request; operations below receive ctx.
	if err != nil {
		return nil, err
	}
	run, err := c.StartRun(ctx, "sync_repository_context")
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
	budget := newSyncRequestBudget(plan.RequestBudget)
	if _, _, err := syncRepositoryHeader(ctx, c, reader, ref, run.ID, budget); err != nil {
		return nil, err
	}
	stats, err := json.Marshal(map[string]int{"requests": budget.used, "request_budget": plan.RequestBudget})
	if err != nil {
		return nil, err
	}
	if err := c.FinishRun(ctx, run.ID, string(stats)); err != nil {
		return nil, err
	}
	return &contracts.RepositoryContextResult{
		Repo: repo, Requests: budget.used, PlannedRequests: plan.PlannedRequests, RequestBudget: plan.RequestBudget,
		Message: "refreshed repository metadata and contribution guidance",
	}, nil
}

func (s *Service) PlanRepositoryContextSync(_ context.Context, repo contracts.RepoRef, maxRequests int) (*contracts.SyncPlanResult, error) {
	return planRepositoryContextSync(repo, maxRequests)
}

func planRepositoryContextSync(repo contracts.RepoRef, maxRequests int) (*contracts.SyncPlanResult, error) {
	if err := (domain.RepoRef{Owner: repo.Owner, Repo: repo.Repo}).Validate(); err != nil {
		return nil, err
	}
	required := repositorycontext.RequestCost()
	if maxRequests == 0 {
		maxRequests = required
	}
	if maxRequests < required || maxRequests > maxSyncRequests {
		return nil, fmt.Errorf("max requests must be between %d and %d", required, maxSyncRequests)
	}
	return &contracts.SyncPlanResult{
		Repo: repo, FixedRequests: required, PlannedRequests: required, RequestBudget: maxRequests,
	}, nil
}

// ArchiveSync adapts CLI archive options to the application sync contract.
func (s *Service) ArchiveSync(ctx context.Context, repo contracts.RepoRef, opts contracts.ArchiveSyncOptions) (*contracts.SyncResult, error) {
	if opts.Since < 0 {
		return nil, errors.New("since duration cannot be negative")
	}
	syncOpts := SyncOptions{State: opts.State, Numbers: opts.Numbers, MaxPages: opts.MaxPages, MaxRequests: opts.MaxRequests}
	if opts.Since > 0 {
		syncOpts.Since = s.now().Add(-opts.Since)
	}
	return s.syncThreadHeaders(ctx, repo, syncOpts)
}

// PlanArchiveSync computes the conservative request ceiling before resolving a
// GitHub reader or opening the corpus.
func (s *Service) PlanArchiveSync(_ context.Context, repo contracts.RepoRef, opts contracts.ArchiveSyncOptions) (*contracts.SyncPlanResult, error) {
	ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Repo}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	if opts.Since < 0 {
		return nil, errors.New("since duration cannot be negative")
	}
	syncOpts := SyncOptions{State: opts.State, Numbers: opts.Numbers, MaxPages: opts.MaxPages, MaxRequests: opts.MaxRequests}
	if opts.Since > 0 {
		syncOpts.Since = s.now().Add(-opts.Since)
	}
	normalized, plan, err := planThreadSyncOptions(syncOpts)
	if err != nil {
		return nil, err
	}
	return &contracts.SyncPlanResult{
		Repo: repo, FixedRequests: 0, ThreadRequestCeiling: plan.threadRequestCeiling,
		PlannedRequests: plan.plannedRequests, RequestBudget: normalized.MaxRequests,
		MaxPages: normalized.MaxPages, ExactThreads: len(normalized.Numbers),
	}, nil
}

// Hydrate adapts the explicit CLI hydration contract to selective hydration.
func (s *Service) Hydrate(ctx context.Context, repo contracts.RepoRef, number int, opts contracts.HydrateOptions) (*contracts.HydrateResult, error) {
	if err := s.refreshHydrationThreadHeader(ctx, repo, opts.Kind, number); err != nil {
		return nil, fmt.Errorf("refresh thread header: %w", err)
	}
	result, err := s.HydrateThread(ctx, repo, number, HydrateOptions{Kind: opts.Kind, Facets: opts.Facets, MaxPages: opts.MaxPages})
	if err != nil {
		return nil, err
	}
	out := &contracts.HydrateResult{
		Repo: result.Repo, Number: result.Number, Kind: result.Kind,
		Pages: result.Pages, Requests: result.Requests + 1,
		Message: "refreshed thread header and " + result.Message,
		Facets:  make([]contracts.HydratedFacet, len(result.Facets)),
	}
	for i, facet := range result.Facets {
		out.Facets[i] = contracts.HydratedFacet{Facet: facet.Facet, Count: facet.Count, Pages: facet.Pages, Complete: facet.Complete}
	}
	return out, nil
}

// Coverage returns repository-level facet coverage without network access.
func (s *Service) Coverage(ctx context.Context, repo contracts.RepoRef) (*contracts.CoverageResult, error) {
	ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Repo}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return nil, err
	}
	stored, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return nil, err
	}
	if stored == nil {
		return nil, fmt.Errorf("%w: %s", errRepositoryNotFound, ref)
	}
	coverage, err := c.ListCoverage(ctx, stored.ID, nil)
	if err != nil {
		return nil, err
	}
	out := &contracts.CoverageResult{Repo: repo, Facets: make([]contracts.CoverageFacet, len(coverage))}
	for i, facet := range coverage {
		out.Facets[i] = contracts.CoverageFacet{
			Facet: facet.Facet, Present: true, Complete: facet.Complete, UpdatedAt: formatTime(facet.UpdatedAt),
		}
	}
	return out, nil
}

// ArchiveThreads returns bounded current thread projections without network access.
func (s *Service) ArchiveThreads(ctx context.Context, repo contracts.RepoRef, kind, state string, limit int) (*contracts.ThreadListResult, error) {
	if limit <= 0 || limit > 1000 {
		return nil, errors.New("thread limit must be between 1 and 1000")
	}
	if kind == "pr" {
		kind = "pull_request"
	}
	if kind == "all" {
		kind = ""
	}
	if kind != "" && kind != "issue" && kind != "pull_request" {
		return nil, fmt.Errorf("unsupported thread kind %q", kind)
	}
	if state != "" && state != "all" && state != "open" && state != "closed" {
		return nil, fmt.Errorf("unsupported thread state %q", state)
	}
	ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Repo}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return nil, err
	}
	stored, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return nil, err
	}
	if stored == nil {
		return nil, fmt.Errorf("%w: %s", errRepositoryNotFound, ref)
	}
	// Apply both kind and state filters at the corpus boundary so the bounded
	// limit is applied to already-matching rows.
	threads, err := c.ListThreadsFiltered(ctx, stored.ID, kind, state, limit)
	if err != nil {
		return nil, err
	}
	out := &contracts.ThreadListResult{Repo: repo, Threads: make([]contracts.ThreadListItem, 0, len(threads))}
	var freshest time.Time
	for _, thread := range threads {
		out.Threads = append(out.Threads, contracts.ThreadListItem{
			Kind: thread.Kind, Number: thread.Number, State: thread.State, Title: thread.Title,
			Author: thread.Author, Labels: thread.Labels, UpdatedAt: formatTime(thread.SourceUpdatedAt),
		})
		if thread.SourceUpdatedAt.After(freshest) {
			freshest = thread.SourceUpdatedAt
		}
	}
	if !freshest.IsZero() {
		out.Freshness = formatTime(freshest)
	}
	coverage, err := c.ListCoverage(ctx, stored.ID, nil)
	if err != nil {
		return nil, err
	}
	out.Coverage = make([]contracts.CoverageFacet, len(coverage))
	for i, facet := range coverage {
		out.Coverage[i] = contracts.CoverageFacet{Facet: facet.Facet, Present: true, Complete: facet.Complete, UpdatedAt: formatTime(facet.UpdatedAt)}
	}
	return out, nil
}

// RunHistory returns bounded durable run metadata.
func (s *Service) RunHistory(ctx context.Context, limit int) (*contracts.RunListResult, error) {
	if limit <= 0 || limit > 1000 {
		return nil, errors.New("run limit must be between 1 and 1000")
	}
	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return nil, err
	}
	runs, err := c.ListRuns(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := &contracts.RunListResult{Runs: make([]contracts.RunResult, len(runs))}
	for i, run := range runs {
		out.Runs[i] = contracts.RunResult{
			ID: run.ID, Kind: run.Kind, Status: run.Status, StartedAt: formatTime(run.StartedAt), Stats: run.Stats, Error: run.Error,
		}
		if run.CompletedAt != nil {
			out.Runs[i].CompletedAt = formatTime(*run.CompletedAt)
		}
	}
	return out, nil
}

// NeighborQuery returns transparent local nearest-thread results.
func (s *Service) NeighborQuery(ctx context.Context, repo contracts.RepoRef, kind string, number, limit int) (*contracts.NeighborListResult, error) {
	result, err := s.Neighbors(ctx, repo, kind, number, limit)
	if err != nil {
		return nil, err
	}
	out := &contracts.NeighborListResult{
		Repo: repo, Kind: result.Kind, Number: result.Number, SourceRevision: result.SourceRevision,
		Neighbors: make([]contracts.NeighborResult, len(result.Neighbors)),
	}
	for i, neighbor := range result.Neighbors {
		out.Neighbors[i] = contracts.NeighborResult{
			Kind: neighbor.Kind, Repo: contracts.RepoRef{Owner: neighbor.Owner, Repo: neighbor.Repo}, Number: neighbor.Number,
			Title: neighbor.Title, State: neighbor.State, Score: neighbor.Score, Reason: neighbor.Reason,
		}
	}
	return out, nil
}

// ExportDossier builds and renders a deterministic redacted dossier bundle.
func (s *Service) ExportDossier(ctx context.Context, repo contracts.RepoRef, format string) (*contracts.ExportResult, error) {
	ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Repo}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	if _, err := s.openReadOnlyCorpus(ctx); err != nil {
		return nil, err
	}
	d, err := s.buildDossier(ctx, ref)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	format, err = normalizeExportFormat(format)
	if err != nil {
		return nil, err
	}
	if format == "json" {
		err = exporter.ExportDossierJSON(&b, d)
	} else {
		err = exporter.ExportDossierMarkdown(&b, d)
	}
	if err != nil {
		return nil, err
	}
	return &contracts.ExportResult{Kind: "dossier", Format: format, Content: b.String()}, nil
}

// ExportEvidence renders a deterministic redacted investigation evidence bundle.
func (s *Service) ExportEvidence(ctx context.Context, investigationID, format string) (*contracts.ExportResult, error) {
	evidence, err := s.ShowEvidence(ctx, investigationID)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	format, err = normalizeExportFormat(format)
	if err != nil {
		return nil, err
	}
	if format == "json" {
		err = exporter.ExportEvidenceJSON(&b, evidence)
	} else {
		err = exporter.ExportEvidenceMarkdown(&b, evidence)
	}
	if err != nil {
		return nil, err
	}
	return &contracts.ExportResult{Kind: "evidence", Format: format, Content: b.String()}, nil
}

func normalizeExportFormat(format string) (string, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "md" {
		format = "markdown"
	}
	if format != "json" && format != "markdown" {
		return "", errors.New("export format must be json or markdown")
	}
	return format, nil
}
