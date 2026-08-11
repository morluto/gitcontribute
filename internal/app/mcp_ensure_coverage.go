package app

import (
	"context"
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

type coverageSnapshotVersions struct {
	Coverage string `json:"coverage"`
}

type coverageSnapshotCompleteness struct {
	Unknown    bool `json:"unknown"`
	Incomplete bool `json:"incomplete"`
}

type coverageSnapshotProvenance struct {
	Producer string `json:"producer"`
	Workflow string `json:"workflow"`
}

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
	target, normalizedTarget, err := parseCoverageTarget(in.Target)
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	in.Target = normalizedTarget
	_, _, exactThread := target.thread()
	if !exactThread && len(in.Facets) > 0 {
		return mcpcontract.JobReference{}, errors.New("facets can be selected only for exact-thread coverage")
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
	id, err := r.submitJob(ctx, jobKindEnsureCoverage, in, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.ensureCoverage(ctx, in, target, report)
	})
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	return queuedJobReference(id, jobKindEnsureCoverage, "coverage workflow started"), nil
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
	return mcpcontract.CorpusSnapshotArtifact{
		SnapshotToken: value.Token, ContractVersion: value.ContractVersion,
		ObservationWatermark: value.ObservationWatermark, Scope: value.Scope,
		SourceManifestSHA256: value.SourceManifestSHA256, DerivedVersions: value.DerivedVersions,
		Completeness: value.Completeness, Provenance: value.Provenance, ArtifactKind: value.ArtifactKind,
		ArtifactDigest: value.ArtifactDigest, Payload: value.Payload, CreatedAt: value.CreatedAt.Format(time.RFC3339Nano),
	}, nil
}

func (r *MCPReader) ensureCoverage(ctx context.Context, in mcpcontract.EnsureCoverageInput, target parsedCoverageTarget, report func(string, string) error) (mcpcontract.EnsureCoverageJobResult, error) {
	c, err := r.openCorpus(ctx)
	if err != nil {
		return mcpcontract.EnsureCoverageJobResult{}, err
	}
	before, reason, err := readParsedCoverageTarget(ctx, c, target)
	if err != nil {
		return mcpcontract.EnsureCoverageJobResult{}, err
	}
	result := mcpcontract.EnsureCoverageJobResult{Status: "complete", PlannedStages: []string{"repository_context", "thread_headers", "selected_facets", "coverage_verification", "snapshot_materialization"}}
	if reason == "" {
		result.CoverageBefore = &before
	}
	remaining := in.MaxRequests
	repoRef := target.repository()
	repo := mcpcontract.RepositoryRef{Owner: repoRef.Owner(), Repo: repoRef.Repo()}
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
		request, err := newRepositoryContextSyncRequest([]domain.RepoRef{repoRef}, cost)
		if err != nil {
			return result, err
		}
		if _, err := r.syncRepositoryContext(ctx, request, report); err != nil {
			return result, err
		}
		remaining -= cost
		stage("repository_context", "complete", "repository identity and context synchronized")
	} else {
		stage("repository_context", "skipped", "repository identity already present")
	}
	headerRequests := 1
	kind, number, exactThread := target.thread()
	if !exactThread {
		headerRequests = 2 * ((in.LimitPerRepository + 99) / 100)
	}
	if remaining < headerRequests {
		return result, errors.New("max_requests exhausted before thread synchronization")
	}
	threadInput := mcpcontract.SyncThreadsInput{MaxRequests: headerRequests}
	if exactThread {
		threadInput.Selection = "threads"
		threadInput.Threads = []mcpcontract.ThreadRef{{Owner: repo.Owner, Repo: repo.Repo, Kind: string(kind), Number: number}}
	} else {
		threadInput.Selection, threadInput.Repositories, threadInput.Kind, threadInput.State, threadInput.LimitPerRepository = "repositories", []mcpcontract.RepositoryRef{repo}, "both", "all", in.LimitPerRepository
	}
	threadRequest, _, err := parseSyncThreadsInput(threadInput)
	if err != nil {
		return result, err
	}
	threadResult, err := r.syncThreadsBatch(ctx, threadRequest, report)
	if err != nil {
		return result, err
	}
	remaining -= headerRequests
	threadStatus := string(threadResult.Status)
	if threadResult.Status == batchOperationPartial {
		result.Status, result.Incomplete = "partial", true
	}
	stage("thread_headers", threadStatus, "thread headers synchronized after repository bootstrap")
	if exactThread && len(in.Facets) > 0 {
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
		facetStatus := string(facetResult.Status)
		if facetResult.Status == batchOperationPartial {
			result.Status, result.Incomplete = "partial", true
		}
		stage("selected_facets", facetStatus, "selected exact-thread facets synchronized")
	} else {
		stage("selected_facets", "skipped", "no exact-thread facets requested")
	}
	after, afterReason, err := readParsedCoverageTarget(ctx, c, target)
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
	materialization, err := corpus.NewSnapshotMaterialization(
		"coverage", target.wire(), after, coverageSnapshotVersions{Coverage: "v1"},
		coverageSnapshotCompleteness{Unknown: result.Unknown, Incomplete: result.Incomplete},
		coverageSnapshotProvenance{Producer: "gitcontribute", Workflow: jobKindEnsureCoverage}, after,
	)
	if err != nil {
		return result, err
	}
	snapshot, err := c.MaterializeReadSnapshot(ctx, materialization)
	if err != nil {
		return result, err
	}
	result.SnapshotToken, result.ArtifactDigest = snapshot.Token, snapshot.ArtifactDigest
	result.NextAction = mcpcontract.FollowUpActionFor(mcpcontract.SnapshotReadAction{SnapshotToken: snapshot.Token})
	stage("snapshot_materialization", "complete", "immutable coverage snapshot created")
	return result, nil
}
