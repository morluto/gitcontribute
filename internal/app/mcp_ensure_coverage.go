package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/facets"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
	"github.com/morluto/gitcontribute/internal/repositorycontext"
)

const jobKindEnsureCoverage = "ensure_coverage"

// EnsureCoverage submits one durable workflow that owns repository bootstrap,
// header synchronization, selected facet hydration, verification, and an
// immutable offline handoff.
func (r *MCPReader) EnsureCoverage(ctx context.Context, in mcpcontract.EnsureCoverageInput) (mcpcontract.JobReference, error) {
	if in.MaxRequests == 0 {
		in.MaxRequests = 1000
	}
	if in.MaxPages == 0 {
		in.MaxPages = 3
	}
	if in.LimitPerRepository == 0 {
		in.LimitPerRepository = 100
	}
	if in.MaxRequests < 1 || in.MaxRequests > 1000 {
		return mcpcontract.JobReference{}, errors.New("max_requests must be between 1 and 1000")
	}
	if in.MaxPages < 1 || in.MaxPages > 100 {
		return mcpcontract.JobReference{}, errors.New("max_pages must be between 1 and 100")
	}
	if in.LimitPerRepository < 1 || in.LimitPerRepository > 1000 {
		return mcpcontract.JobReference{}, errors.New("limit_per_repository must be between 1 and 1000")
	}
	allowedFacets := make(map[string]struct{})
	for _, name := range facets.SelectableNames() {
		allowedFacets[name] = struct{}{}
	}
	seenFacets := make(map[string]struct{}, len(in.Facets))
	for _, name := range in.Facets {
		if _, ok := allowedFacets[name]; !ok {
			return mcpcontract.JobReference{}, fmt.Errorf("unsupported facet %q", name)
		}
		if _, duplicate := seenFacets[name]; duplicate {
			return mcpcontract.JobReference{}, fmt.Errorf("duplicate facet %q", name)
		}
		seenFacets[name] = struct{}{}
	}
	if err := validateEnsureCoverageTarget(in.Target); err != nil {
		return mcpcontract.JobReference{}, err
	}
	id, err := r.submitJob(ctx, jobKindEnsureCoverage, in, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.ensureCoverage(ctx, in, report)
	})
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	return queuedJobReference(id, jobKindEnsureCoverage, "coverage workflow started"), nil
}

func validateEnsureCoverageTarget(target mcpcontract.CoverageTarget) error {
	if err := (domain.RepoRef{Owner: target.Repository.Owner, Repo: target.Repository.Repo}).Validate(); err != nil {
		return err
	}
	if target.Type == mcpcontract.CoverageTargetRepository && target.Thread == nil {
		return nil
	}
	if target.Type == mcpcontract.CoverageTargetExactThread && target.Thread != nil && (target.Thread.Kind == "issue" || target.Thread.Kind == "pull_request") && target.Thread.Number > 0 {
		return nil
	}
	return errInvalidCoverageTarget
}

func (r *MCPReader) ReadSnapshot(ctx context.Context, token string) (mcpcontract.CorpusSnapshotArtifact, error) {
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.CorpusSnapshotArtifact{}, err
	}
	value, err := c.ResolveReadSnapshot(ctx, token)
	if err != nil {
		return mcpcontract.CorpusSnapshotArtifact{}, err
	}
	decode := func(raw json.RawMessage) (any, error) {
		var out any
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, err
		}
		return out, nil
	}
	scope, err := decode(value.Scope)
	if err != nil {
		return mcpcontract.CorpusSnapshotArtifact{}, fmt.Errorf("decode snapshot scope: %w", err)
	}
	derived, err := decode(value.DerivedVersions)
	if err != nil {
		return mcpcontract.CorpusSnapshotArtifact{}, fmt.Errorf("decode snapshot derived versions: %w", err)
	}
	completeness, err := decode(value.Completeness)
	if err != nil {
		return mcpcontract.CorpusSnapshotArtifact{}, fmt.Errorf("decode snapshot completeness: %w", err)
	}
	provenance, err := decode(value.Provenance)
	if err != nil {
		return mcpcontract.CorpusSnapshotArtifact{}, fmt.Errorf("decode snapshot provenance: %w", err)
	}
	payload, err := decode(value.Payload)
	if err != nil {
		return mcpcontract.CorpusSnapshotArtifact{}, fmt.Errorf("decode snapshot payload: %w", err)
	}
	return mcpcontract.CorpusSnapshotArtifact{
		SnapshotToken: value.Token, ContractVersion: value.ContractVersion,
		ObservationWatermark: value.ObservationWatermark, Scope: scope,
		SourceManifestSHA256: value.SourceManifestSHA256, DerivedVersions: derived,
		Completeness: completeness, Provenance: provenance, ArtifactKind: value.ArtifactKind,
		ArtifactDigest: value.ArtifactDigest, Payload: payload, CreatedAt: value.CreatedAt.Format(time.RFC3339Nano),
	}, nil
}

func (r *MCPReader) ensureCoverage(ctx context.Context, in mcpcontract.EnsureCoverageInput, report func(string, string) error) (mcpcontract.EnsureCoverageJobResult, error) {
	c, err := r.openCorpus(ctx)
	if err != nil {
		return mcpcontract.EnsureCoverageJobResult{}, err
	}
	before, reason, err := readCoverageTarget(ctx, c, in.Target)
	if err != nil {
		return mcpcontract.EnsureCoverageJobResult{}, err
	}
	result := mcpcontract.EnsureCoverageJobResult{Status: "complete", PlannedStages: []string{"repository_context", "thread_headers", "selected_facets", "coverage_verification", "snapshot_materialization"}}
	if reason == "" {
		result.CoverageBefore = &before
	}
	remaining := in.MaxRequests
	repo := in.Target.Repository
	stage := func(name, status, message string) {
		result.CompletedStages = append(result.CompletedStages, name)
		result.StageOutcomes = append(result.StageOutcomes, mcpcontract.CoverageStageOutcome{Stage: name, Status: status, Message: message})
	}
	if err := report("coverage_bootstrap", jobProgressCounts(0, len(result.PlannedStages))); err != nil {
		return result, err
	}
	if reason == "repository_not_indexed" {
		cost := repositorycontext.RequestCost()
		if remaining < cost {
			return result, fmt.Errorf("max_requests is too small for repository bootstrap: need at least %d", cost)
		}
		if _, err := r.syncRepositoryContext(ctx, mcpcontract.SyncRepositoryContextInput{Repositories: []mcpcontract.RepositoryRef{repo}, MaxRequests: cost}, report); err != nil {
			return result, err
		}
		remaining -= cost
		stage("repository_context", "complete", "repository identity and context synchronized")
	} else {
		stage("repository_context", "skipped", "repository identity already present")
	}
	headerRequests := 1
	if in.Target.Type == mcpcontract.CoverageTargetRepository {
		headerRequests = 2 * ((in.LimitPerRepository + 99) / 100)
	}
	if remaining < headerRequests {
		return result, errors.New("max_requests exhausted before thread synchronization")
	}
	threadInput := mcpcontract.SyncThreadsInput{MaxRequests: headerRequests}
	if in.Target.Type == mcpcontract.CoverageTargetExactThread {
		threadInput.Selection = "threads"
		threadInput.Threads = []mcpcontract.ThreadRef{{Owner: repo.Owner, Repo: repo.Repo, Kind: in.Target.Thread.Kind, Number: in.Target.Thread.Number}}
	} else {
		threadInput.Selection, threadInput.Repositories, threadInput.Kind, threadInput.State, threadInput.LimitPerRepository = "repositories", []mcpcontract.RepositoryRef{repo}, "both", "all", in.LimitPerRepository
	}
	threadResult, err := r.syncThreadsBatch(ctx, threadInput, report)
	if err != nil {
		return result, err
	}
	remaining -= headerRequests
	threadStatus, _ := threadResult["status"].(string)
	if threadStatus == "partial" {
		result.Status, result.Incomplete = "partial", true
	}
	stage("thread_headers", threadStatus, "thread headers synchronized after repository bootstrap")
	if in.Target.Type == mcpcontract.CoverageTargetExactThread && len(in.Facets) > 0 {
		pages := in.MaxPages
		if bound := remaining / len(in.Facets); bound < pages {
			pages = bound
		}
		if pages < 1 {
			return result, errors.New("max_requests exhausted before selected facet hydration")
		}
		facetResult, err := r.hydrateThreadsBatch(ctx, mcpcontract.HydrateThreadsInput{Threads: threadInput.Threads, Facets: append([]string(nil), in.Facets...), MaxPages: pages}, report)
		if err != nil {
			return result, err
		}
		facetStatus, _ := facetResult["status"].(string)
		if facetStatus == "partial" {
			result.Status, result.Incomplete = "partial", true
		}
		stage("selected_facets", facetStatus, "selected exact-thread facets synchronized")
	} else {
		stage("selected_facets", "skipped", "no exact-thread facets requested")
	}
	after, afterReason, err := readCoverageTarget(ctx, c, in.Target)
	if err != nil {
		return result, err
	}
	result.Unknown = afterReason != ""
	for _, coverage := range after.Facets {
		if !coverage.Complete {
			result.Incomplete = true
		}
	}
	if result.Unknown || result.Incomplete {
		result.Status = "partial"
	}
	stage("coverage_verification", result.Status, afterReason)
	snapshot, err := c.MaterializeReadSnapshot(ctx, corpus.SnapshotMaterialization{Kind: "coverage", Scope: in.Target, SourceManifest: after, DerivedVersions: map[string]string{"coverage": "v1"}, Completeness: map[string]bool{"unknown": result.Unknown, "incomplete": result.Incomplete}, Provenance: map[string]any{"producer": "gitcontribute", "workflow": jobKindEnsureCoverage}, Payload: after})
	if err != nil {
		return result, err
	}
	result.SnapshotToken, result.ArtifactDigest = snapshot.Token, snapshot.ArtifactDigest
	result.NextAction = mcpcontract.FollowUpAction{Type: "read_snapshot", ReadSnapshot: &mcpcontract.SnapshotReadAction{SnapshotToken: snapshot.Token}}
	stage("snapshot_materialization", "complete", "immutable coverage snapshot created")
	return result, nil
}
