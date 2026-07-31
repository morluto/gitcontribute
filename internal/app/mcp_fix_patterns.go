package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/failure"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
	"github.com/morluto/gitcontribute/internal/relatedwork"
)

type fixPatternCandidate struct {
	thread        corpus.Thread
	unknownBefore bool
	symptoms      map[int]struct{}
}

type fixPatternAnalysis struct {
	clusters           [][]int64
	candidates         map[int64]*fixPatternCandidate
	orderedIDs         []int64
	candidateMatches   int
	candidateTruncated bool
}

type fixPatternClassification struct {
	relationship mcpcontract.FixPatternRelationship
	related      *mcpcontract.ThreadRef
	evidence     string
	superseded   bool
}

// MineRepositoryFixPatterns submits one bounded GitHub-read/local-write
// workflow. It searches stored candidates first and hydrates only finalists
// whose merge outcome is unknown.
func (r *MCPReader) MineRepositoryFixPatterns(ctx context.Context, in mcpcontract.MineRepositoryFixPatternsInput) (mcpcontract.JobReference, error) {
	normalized, err := normalizeFixPatternInput(in)
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	id, err := r.submitJob(ctx, "mine_repository_fix_patterns", normalized, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.analyzeFixPatterns(ctx, normalized, report, false)
	})
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	return queuedJobReference(id, "mine_repository_fix_patterns", "repository fix-pattern mining job started"), nil
}

// PreviewRepositoryFixPatterns performs the same bounded corpus analysis as
// the durable workflow while keeping the entire operation read-only. Preview
// deliberately disables hydration: it is an offline planning read, not a
// hidden synchronization request.
func (r *MCPReader) PreviewRepositoryFixPatterns(ctx context.Context, in mcpcontract.PreviewRepositoryFixPatternsInput) (mcpcontract.FixPatternReport, error) {
	normalized, err := normalizeFixPatternInput(mcpcontract.MineRepositoryFixPatternsInput(in))
	if err != nil {
		return mcpcontract.FixPatternReport{}, err
	}
	zero := 0
	normalized.HydrationLimit = &zero
	return r.analyzeFixPatterns(ctx, normalized, nil, true)
}

// mineRepositoryFixPatterns remains the executor-local entry point used by
// focused tests and the durable job adapter.
func (r *MCPReader) mineRepositoryFixPatterns(ctx context.Context, in mcpcontract.MineRepositoryFixPatternsInput, progress func(string, string) error) (mcpcontract.FixPatternReport, error) {
	return r.analyzeFixPatterns(ctx, in, progress, false)
}

// GetFixPatternReport reads the typed terminal result of a pattern-mining job.
func (r *MCPReader) GetFixPatternReport(ctx context.Context, id string) (mcpcontract.FixPatternReport, error) {
	job, err := r.Service.GetJob(ctx, id)
	if err != nil {
		return mcpcontract.FixPatternReport{}, err
	}
	if job.Kind != "mine_repository_fix_patterns" {
		return mcpcontract.FixPatternReport{}, failure.NotFound(fmt.Errorf("job %s is not a fix-pattern report", id))
	}
	if job.Status != corpus.JobStatusSucceeded {
		return mcpcontract.FixPatternReport{}, errors.New("fix-pattern report is not available until the job succeeds")
	}
	var report mcpcontract.FixPatternReport
	if err := json.Unmarshal([]byte(job.Result), &report); err != nil {
		return mcpcontract.FixPatternReport{}, fmt.Errorf("decode fix-pattern report: %w", err)
	}
	var identity struct {
		CorpusRevision *int64 `json:"corpus_revision"`
	}
	if err := json.Unmarshal([]byte(job.Result), &identity); err != nil {
		return mcpcontract.FixPatternReport{}, fmt.Errorf("decode fix-pattern report identity: %w", err)
	}
	if identity.CorpusRevision == nil {
		return mcpcontract.FixPatternReport{}, mcpcontract.Unavailable(
			"legacy_artifact",
			"this persisted fix-pattern report predates corpus revision binding; rerun the fix-pattern workflow to regenerate it",
		)
	}
	report.Persisted = true
	return report, nil
}

func normalizeFixPatternInput(in mcpcontract.MineRepositoryFixPatternsInput) (mcpcontract.MineRepositoryFixPatternsInput, error) {
	ref := domain.RepoRef{Owner: in.Repository.Owner, Repo: in.Repository.Repo}
	if err := ref.Validate(); err != nil {
		return in, err
	}
	after, err := time.Parse(time.RFC3339, in.TimeWindow.UpdatedAfter)
	if err != nil {
		return in, errors.New("time_window.updated_after must be RFC 3339")
	}
	if in.TimeWindow.UpdatedBefore != "" {
		before, err := time.Parse(time.RFC3339, in.TimeWindow.UpdatedBefore)
		if err != nil {
			return in, errors.New("time_window.updated_before must be RFC 3339")
		}
		if before.Before(after) {
			return in, errors.New("time_window.updated_before must not be earlier than updated_after")
		}
	}
	if len(in.SymptomTaxonomy) < 1 || len(in.SymptomTaxonomy) > 12 {
		return in, errors.New("symptom_taxonomy must contain 1 to 12 categories")
	}
	seenNames := make(map[string]struct{}, len(in.SymptomTaxonomy))
	for i := range in.SymptomTaxonomy {
		symptom := &in.SymptomTaxonomy[i]
		symptom.Name = strings.TrimSpace(symptom.Name)
		if symptom.Name == "" {
			return in, fmt.Errorf("symptom_taxonomy[%d].name is required", i)
		}
		key := strings.ToLower(symptom.Name)
		if _, exists := seenNames[key]; exists {
			return in, fmt.Errorf("symptom_taxonomy[%d].name duplicates %q", i, symptom.Name)
		}
		seenNames[key] = struct{}{}
		if len(symptom.Terms) < 1 || len(symptom.Terms) > 12 {
			return in, fmt.Errorf("symptom_taxonomy[%d].terms must contain 1 to 12 values", i)
		}
		seenTerms := make(map[string]struct{}, len(symptom.Terms))
		for j := range symptom.Terms {
			symptom.Terms[j] = strings.TrimSpace(symptom.Terms[j])
			if symptom.Terms[j] == "" {
				return in, fmt.Errorf("symptom_taxonomy[%d].terms[%d] must not be empty", i, j)
			}
			key := strings.ToLower(symptom.Terms[j])
			if _, exists := seenTerms[key]; exists {
				return in, fmt.Errorf("symptom_taxonomy[%d].terms contains duplicate %q", i, symptom.Terms[j])
			}
			seenTerms[key] = struct{}{}
		}
	}
	if in.CandidateLimit == 0 {
		in.CandidateLimit = mcpcontract.DefaultFixPatternCandidateLimit
	}
	if in.CandidateLimit < 1 || in.CandidateLimit > 100 {
		return in, errors.New("candidate_limit must be between 1 and 100")
	}
	if in.HydrationLimit == nil {
		value := mcpcontract.DefaultFixPatternHydrationLimit
		in.HydrationLimit = &value
	}
	if *in.HydrationLimit < 0 || *in.HydrationLimit > 100 {
		return in, errors.New("hydration_limit must be between 0 and 100")
	}
	if in.RepresentativeLimit == 0 {
		in.RepresentativeLimit = mcpcontract.DefaultFixPatternRepresentativeLimit
	}
	if in.RepresentativeLimit < 1 || in.RepresentativeLimit > 20 {
		return in, errors.New("representative_limit must be between 1 and 20")
	}
	if len(in.MergeOutcomes) == 0 {
		in.MergeOutcomes = []mcpcontract.FixPatternOutcome{"merged"}
	}
	seenOutcomes := make(map[mcpcontract.FixPatternOutcome]struct{}, len(in.MergeOutcomes))
	for _, outcome := range in.MergeOutcomes {
		switch outcome {
		case "merged", "closed_unmerged", "superseded", "open", "unknown":
		default:
			return in, fmt.Errorf("unsupported merge outcome %q", outcome)
		}
		if _, exists := seenOutcomes[outcome]; exists {
			return in, fmt.Errorf("duplicate merge outcome %q", outcome)
		}
		seenOutcomes[outcome] = struct{}{}
	}
	return in, nil
}

func collectFixPatternCandidates(ctx context.Context, c *corpus.Corpus, repo *corpus.Repository, in mcpcontract.MineRepositoryFixPatternsInput, progress func(string, string) error) (fixPatternAnalysis, error) {
	a := fixPatternAnalysis{
		clusters:   make([][]int64, len(in.SymptomTaxonomy)),
		candidates: make(map[int64]*fixPatternCandidate),
		orderedIDs: make([]int64, 0),
	}
	after, _ := time.Parse(time.RFC3339, in.TimeWindow.UpdatedAfter)
	var before time.Time
	if in.TimeWindow.UpdatedBefore != "" {
		before, _ = time.Parse(time.RFC3339, in.TimeWindow.UpdatedBefore)
	}
	if err := progress("candidate_search", jobProgressCounts(0, len(in.SymptomTaxonomy))); err != nil {
		return fixPatternAnalysis{}, err
	}
	for symptomIndex, symptom := range in.SymptomTaxonomy {
		page, err := c.SearchThreadsPage(ctx, strings.Join(symptom.Terms, " "), corpus.SearchFilter{
			RepoID: repo.ID, Repo: in.Repository.Owner + "/" + in.Repository.Repo,
			Kind: corpus.ThreadKindPullRequest, UpdatedAfter: after, UpdatedBefore: before,
			Limit: in.CandidateLimit, Sort: "relevance", MatchMode: "any",
		})
		if err != nil {
			return fixPatternAnalysis{}, fmt.Errorf("search symptom %q: %w", symptom.Name, err)
		}
		a.candidateMatches += page.Total
		a.candidateTruncated = a.candidateTruncated || page.Total > len(page.Threads)
		for _, thread := range page.Threads {
			candidate := a.candidates[thread.ID]
			if candidate == nil {
				candidate = &fixPatternCandidate{thread: thread, unknownBefore: needsMergeHydration(thread), symptoms: make(map[int]struct{})}
				a.candidates[thread.ID] = candidate
				a.orderedIDs = append(a.orderedIDs, thread.ID)
			}
			if _, exists := candidate.symptoms[symptomIndex]; !exists {
				candidate.symptoms[symptomIndex] = struct{}{}
				a.clusters[symptomIndex] = append(a.clusters[symptomIndex], thread.ID)
			}
		}
		if err := progress("candidate_search", jobProgressCounts(symptomIndex+1, len(in.SymptomTaxonomy))); err != nil {
			return fixPatternAnalysis{}, err
		}
	}
	return a, nil
}

func selectFixPatternHydration(a fixPatternAnalysis, in mcpcontract.MineRepositoryFixPatternsInput) []mcpcontract.ThreadRef {
	unknown := countUnknownCandidates(a.candidates)
	refs := make([]mcpcontract.ThreadRef, 0, min(*in.HydrationLimit, unknown))
	for _, id := range a.orderedIDs {
		candidate := a.candidates[id]
		if !needsMergeHydration(candidate.thread) || len(refs) >= *in.HydrationLimit {
			continue
		}
		refs = append(refs, mcpcontract.ThreadRef{
			Owner: in.Repository.Owner, Repo: in.Repository.Repo, Kind: "pull_request", Number: candidate.thread.Number,
		})
	}
	return refs
}

func (r *MCPReader) analyzeFixPatterns(ctx context.Context, in mcpcontract.MineRepositoryFixPatternsInput, progress func(string, string) error, readOnly bool) (mcpcontract.FixPatternReport, error) {
	var c *corpus.Corpus
	var err error
	if readOnly {
		c, err = r.openReadOnlyCorpus(ctx)
	} else {
		c, err = r.openCorpus(ctx)
	}
	if err != nil {
		return mcpcontract.FixPatternReport{}, err
	}
	revision, err := beginCorpusRead(ctx, c, in.CorpusRevision)
	if err != nil {
		return mcpcontract.FixPatternReport{}, err
	}
	if progress == nil {
		progress = func(string, string) error { return nil }
	}
	repo, err := c.GetRepository(ctx, in.Repository.Owner, in.Repository.Repo)
	if err != nil {
		return mcpcontract.FixPatternReport{}, err
	}
	if repo == nil {
		return mcpcontract.FixPatternReport{}, fmt.Errorf("repository %s/%s has not been synced", in.Repository.Owner, in.Repository.Repo)
	}
	analysis, err := collectFixPatternCandidates(ctx, c, repo, in, progress)
	if err != nil {
		return mcpcontract.FixPatternReport{}, err
	}
	unknownBefore := countUnknownCandidates(analysis.candidates)
	unknownBeforeByID := make(map[int64]bool, len(analysis.candidates))
	for id, candidate := range analysis.candidates {
		unknownBeforeByID[id] = candidate.unknownBefore
	}
	hydrationRefs := selectFixPatternHydration(analysis, in)

	hydrated, failures := 0, make([]mcpcontract.FixPatternHydrationFailure, 0)
	if !readOnly && len(hydrationRefs) > 0 {
		// Validate the candidate read before making the workflow's deliberate
		// corpus mutation. The report is then assembled again from the
		// post-hydration state, rather than comparing the current revision with
		// itself after the write.
		if err := finishCorpusRead(ctx, c, revision); err != nil {
			return mcpcontract.FixPatternReport{}, err
		}
		raw, err := r.hydrateThreadsBatch(ctx, mcpcontract.HydrateThreadsInput{
			Threads: hydrationRefs, Facets: []string{FacetPRDetails}, MaxPages: 1,
		}, progress)
		if err != nil {
			return mcpcontract.FixPatternReport{}, err
		}
		items, _ := raw["items"].([]map[string]any)
		for i, ref := range hydrationRefs {
			if i < len(items) && items[i]["status"] == "complete" {
				hydrated++
			} else {
				reason, message := "hydration_failed", "pull-request details were not refreshed"
				retryable := false
				if i < len(items) {
					reason, _ = items[i]["reason"].(string)
					message, _ = items[i]["message"].(string)
					retryable = items[i]["status"] == "retryable"
				}
				failures = append(failures, mcpcontract.FixPatternHydrationFailure{
					PullRequest: ref, Reason: reason, Message: message, Retryable: retryable,
				})
			}
		}
		revision, err = beginCorpusRead(ctx, c, nil)
		if err != nil {
			return mcpcontract.FixPatternReport{}, err
		}
		analysis, err = collectFixPatternCandidates(ctx, c, repo, in, progress)
		if err != nil {
			return mcpcontract.FixPatternReport{}, err
		}
		for id, candidate := range analysis.candidates {
			if unknown, ok := unknownBeforeByID[id]; ok {
				candidate.unknownBefore = unknown
			}
		}
	}
	candidates := analysis.candidates

	wantedOutcomes := make(map[mcpcontract.FixPatternOutcome]struct{}, len(in.MergeOutcomes))
	for _, outcome := range in.MergeOutcomes {
		wantedOutcomes[outcome] = struct{}{}
	}
	reportClusters := make([]mcpcontract.FixPatternCluster, len(in.SymptomTaxonomy))
	for i, symptom := range in.SymptomTaxonomy {
		cluster := mcpcontract.FixPatternCluster{
			Name: symptom.Name, Terms: append([]string(nil), symptom.Terms...),
			CandidateCount: mcpcontract.NonNegativeInt(len(analysis.clusters[i])),
		}
		for _, id := range analysis.clusters[i] {
			thread := candidates[id].thread
			if candidates[id].unknownBefore {
				cluster.UnknownBefore++
			}
			classification := classifyFixPattern(thread, in.Repository)
			outcome := fixPatternOutcome(thread, classification.superseded)
			incrementFixPatternOutcome(&cluster.Outcomes, outcome)
			if outcome == "unknown" {
				cluster.UnknownAfter++
			}
			if _, wanted := wantedOutcomes[outcome]; !wanted {
				continue
			}
			if len(cluster.Examples) >= in.RepresentativeLimit {
				cluster.ExamplesTruncated = true
				continue
			}
			cluster.Examples = append(cluster.Examples, buildFixPatternExample(ctx, c, repo.ID, in.Repository, thread, outcome, classification))
		}
		reportClusters[i] = cluster
	}

	status := mcpcontract.FixPatternReportStatus("complete")
	if len(failures) > 0 || analysis.candidateTruncated || countUnknownCandidates(candidates) > 0 {
		status = "partial"
	}
	limitations := []string{
		"Similarity-only examples are candidates, not proof that a pull request fixed a related issue.",
		"Accepted fixes require refreshed merged state and an explicit closing relationship in stored pull-request text.",
	}
	if analysis.candidateTruncated {
		limitations = append(limitations, "At least one symptom category exceeded candidate_limit; coverage is truncated.")
	}
	if err := finishCorpusRead(ctx, c, revision); err != nil {
		return mcpcontract.FixPatternReport{}, err
	}
	return mcpcontract.FixPatternReport{
		Status: status, Repository: in.Repository, TimeWindow: in.TimeWindow,
		GeneratedAt: r.now().Format(time.RFC3339),
		Coverage: mcpcontract.FixPatternCoverage{
			CandidateMatches: mcpcontract.NonNegativeInt(analysis.candidateMatches), UniqueCandidates: mcpcontract.NonNegativeInt(len(candidates)),
			UnknownBefore: mcpcontract.NonNegativeInt(unknownBefore), SelectedForHydration: mcpcontract.NonNegativeInt(len(hydrationRefs)),
			Hydrated: mcpcontract.NonNegativeInt(hydrated), HydrationFailed: mcpcontract.NonNegativeInt(len(failures)),
			UnknownAfter: mcpcontract.NonNegativeInt(countUnknownCandidates(candidates)), CandidateTruncated: analysis.candidateTruncated,
		},
		Clusters: reportClusters, Failures: failures, Limitations: limitations, Persisted: !readOnly, CorpusRevision: revision,
	}, nil
}

func countUnknownCandidates(candidates map[int64]*fixPatternCandidate) int {
	count := 0
	for _, candidate := range candidates {
		if needsMergeHydration(candidate.thread) {
			count++
		}
	}
	return count
}

func needsMergeHydration(thread corpus.Thread) bool {
	return thread.State == "closed" && !thread.MergedKnown
}

func fixPatternOutcome(thread corpus.Thread, superseded bool) mcpcontract.FixPatternOutcome {
	switch {
	case thread.MergedKnown && thread.Merged:
		return "merged"
	case thread.State != "closed":
		return "open"
	case !thread.MergedKnown:
		return "unknown"
	case superseded:
		return "superseded"
	default:
		return "closed_unmerged"
	}
}

func incrementFixPatternOutcome(counts *mcpcontract.FixPatternOutcomeCounts, outcome mcpcontract.FixPatternOutcome) {
	switch outcome {
	case "merged":
		counts.Merged++
	case "closed_unmerged":
		counts.ClosedUnmerged++
	case "superseded":
		counts.Superseded++
	case "open":
		counts.Open++
	case "unknown":
		counts.Unknown++
	}
}

func buildFixPatternExample(ctx context.Context, c *corpus.Corpus, repoID int64, repository mcpcontract.RepositoryRef, thread corpus.Thread, outcome mcpcontract.FixPatternOutcome, classification fixPatternClassification) mcpcontract.FixPatternExample {
	example := mcpcontract.FixPatternExample{
		PullRequest: mcpcontract.ThreadRef{Owner: repository.Owner, Repo: repository.Repo, Kind: "pull_request", Number: thread.Number},
		Title:       thread.Title, Outcome: outcome, Relationship: classification.relationship, RelationshipEvidence: classification.evidence,
		AcceptedFix: outcome == "merged" && classification.relationship == "closes",
		ProofStyles: detectProofStyles(thread.Body), UpdatedAt: thread.SourceUpdatedAt.Format(time.RFC3339),
	}
	if classification.related != nil {
		example.RelatedThread = classification.related
		example.RelatedKind = mcpcontract.FixPatternRelatedKind(classification.related.Kind)
		relatedRepoID := repoID
		if classification.related.Owner != repository.Owner || classification.related.Repo != repository.Repo {
			relatedRepo, err := c.GetRepository(ctx, classification.related.Owner, classification.related.Repo)
			if err != nil || relatedRepo == nil {
				return example
			}
			relatedRepoID = relatedRepo.ID
		}
		related, err := c.GetThreadByNumber(ctx, relatedRepoID, classification.related.Number)
		if err == nil && related != nil {
			example.RelatedKind = mcpcontract.FixPatternRelatedKind(related.Kind)
			example.RelatedThread.Kind = related.Kind
		}
	}
	return example
}

func classifyFixPattern(thread corpus.Thread, repository mcpcontract.RepositoryRef) fixPatternClassification {
	refs := relatedwork.Extract(thread.Body, domain.RepoRef{Owner: repository.Owner, Repo: repository.Repo})
	classification := fixPatternClassification{relationship: "similarity_only"}
	bestPriority := 0
	for _, ref := range refs {
		if ref.Relation == relatedwork.RelationSupersededBy {
			classification.superseded = true
		}
		priority := relatedwork.Priority(ref.Relation)
		if priority <= bestPriority {
			continue
		}
		bestPriority = priority
		classification.related = &mcpcontract.ThreadRef{
			Owner: ref.Repo.Owner, Repo: ref.Repo.Repo, Kind: string(ref.Kind), Number: ref.Number,
		}
		classification.evidence = ref.Evidence
		switch ref.Relation {
		case relatedwork.RelationClaimsToClose:
			classification.relationship = "closes"
		case relatedwork.RelationReplaces, relatedwork.RelationSupersededBy:
			classification.relationship = "explicit_replacement"
		default:
			classification.relationship = "references"
		}
	}
	return classification
}

func detectProofStyles(body string) []mcpcontract.FixPatternProofStyle {
	lower := strings.ToLower(body)
	var styles []mcpcontract.FixPatternProofStyle
	for _, candidate := range []struct {
		name  mcpcontract.FixPatternProofStyle
		terms []string
	}{
		{name: "regression_test", terms: []string{"regression test", "unit test", "test coverage"}},
		{name: "reproduction", terms: []string{"reproducer", "reproduction", "repro case"}},
		{name: "benchmark", terms: []string{"benchmark", "throughput", "latency"}},
		{name: "before_after", terms: []string{"before and after", "before/after"}},
		{name: "screenshot", terms: []string{"screenshot"}},
	} {
		if slices.ContainsFunc(candidate.terms, func(term string) bool { return strings.Contains(lower, term) }) {
			styles = append(styles, candidate.name)
		}
	}
	return styles
}
