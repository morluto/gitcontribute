package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/exporter"
)

// ArchiveSync adapts CLI archive options to the application sync contract.
func (s *Service) ArchiveSync(ctx context.Context, repo cli.RepoRef, opts cli.ArchiveSyncOptions) (*cli.SyncResult, error) {
	if opts.Since < 0 {
		return nil, errors.New("since duration cannot be negative")
	}
	syncOpts := SyncOptions{State: opts.State, Numbers: opts.Numbers, MaxPages: opts.MaxPages}
	if opts.Since > 0 {
		syncOpts.Since = s.now().Add(-opts.Since)
	}
	return s.SyncWithOptions(ctx, repo, syncOpts)
}

// Hydrate adapts the explicit CLI hydration contract to selective hydration.
func (s *Service) Hydrate(ctx context.Context, repo cli.RepoRef, number int, opts cli.HydrateOptions) (*cli.HydrateResult, error) {
	result, err := s.HydrateThread(ctx, repo, number, HydrateOptions{Facets: opts.Facets, MaxPages: opts.MaxPages})
	if err != nil {
		return nil, err
	}
	out := &cli.HydrateResult{
		Repo: result.Repo, Number: result.Number, Kind: result.Kind,
		Pages: result.Pages, Requests: result.Requests, Message: result.Message,
		Facets: make([]cli.HydratedFacet, len(result.Facets)),
	}
	for i, facet := range result.Facets {
		out.Facets[i] = cli.HydratedFacet{Facet: facet.Facet, Count: facet.Count, Pages: facet.Pages, Complete: facet.Complete}
	}
	return out, nil
}

// Coverage returns repository-level facet coverage without network access.
func (s *Service) Coverage(ctx context.Context, repo cli.RepoRef) (*cli.CoverageResult, error) {
	ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Repo}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	c, err := s.openCorpus(ctx)
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
	out := &cli.CoverageResult{Repo: repo, Facets: make([]cli.CoverageFacet, len(coverage))}
	for i, facet := range coverage {
		out.Facets[i] = cli.CoverageFacet{
			Facet: facet.Facet, Present: true, Complete: facet.Complete, UpdatedAt: formatTime(facet.UpdatedAt),
		}
	}
	return out, nil
}

// ArchiveThreads returns bounded current thread projections without network access.
func (s *Service) ArchiveThreads(ctx context.Context, repo cli.RepoRef, kind, state string, limit int) (*cli.ThreadListResult, error) {
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
	c, err := s.openCorpus(ctx)
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
	out := &cli.ThreadListResult{Repo: repo, Threads: make([]cli.ThreadListItem, 0, len(threads))}
	var freshest time.Time
	for _, thread := range threads {
		out.Threads = append(out.Threads, cli.ThreadListItem{
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
	out.Coverage = make([]cli.CoverageFacet, len(coverage))
	for i, facet := range coverage {
		out.Coverage[i] = cli.CoverageFacet{Facet: facet.Facet, Present: true, Complete: facet.Complete, UpdatedAt: formatTime(facet.UpdatedAt)}
	}
	return out, nil
}

// RunHistory returns bounded durable run metadata.
func (s *Service) RunHistory(ctx context.Context, limit int) (*cli.RunListResult, error) {
	if limit <= 0 || limit > 1000 {
		return nil, errors.New("run limit must be between 1 and 1000")
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	runs, err := c.ListRuns(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := &cli.RunListResult{Runs: make([]cli.RunResult, len(runs))}
	for i, run := range runs {
		out.Runs[i] = cli.RunResult{
			ID: run.ID, Kind: run.Kind, Status: run.Status, StartedAt: formatTime(run.StartedAt), Stats: run.Stats, Error: run.Error,
		}
		if run.CompletedAt != nil {
			out.Runs[i].CompletedAt = formatTime(*run.CompletedAt)
		}
	}
	return out, nil
}

// NeighborQuery returns transparent local nearest-thread results.
func (s *Service) NeighborQuery(ctx context.Context, repo cli.RepoRef, kind string, number, limit int) (*cli.NeighborListResult, error) {
	result, err := s.Neighbors(ctx, repo, kind, number, limit)
	if err != nil {
		return nil, err
	}
	out := &cli.NeighborListResult{
		Repo: repo, Kind: result.Kind, Number: result.Number, SourceRevision: result.SourceRevision,
		Neighbors: make([]cli.NeighborResult, len(result.Neighbors)),
	}
	for i, neighbor := range result.Neighbors {
		out.Neighbors[i] = cli.NeighborResult{
			Kind: neighbor.Kind, Repo: cli.RepoRef{Owner: neighbor.Owner, Repo: neighbor.Repo}, Number: neighbor.Number,
			Title: neighbor.Title, State: neighbor.State, Score: neighbor.Score, Reason: neighbor.Reason,
		}
	}
	return out, nil
}

// ExportDossier builds and renders a deterministic redacted dossier bundle.
func (s *Service) ExportDossier(ctx context.Context, repo cli.RepoRef, format string) (*cli.ExportResult, error) {
	ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Repo}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	if _, err := s.openCorpus(ctx); err != nil {
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
	return &cli.ExportResult{Kind: "dossier", Format: format, Content: b.String()}, nil
}

// ExportEvidence renders a deterministic redacted investigation evidence bundle.
func (s *Service) ExportEvidence(ctx context.Context, investigationID, format string) (*cli.ExportResult, error) {
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
	return &cli.ExportResult{Kind: "evidence", Format: format, Content: b.String()}, nil
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
