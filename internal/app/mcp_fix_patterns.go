package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

type fixPatternSymptom struct {
	name  string
	terms []string
}

type fixPatternWindow struct {
	after  time.Time
	before *time.Time
}

func (w fixPatternWindow) beforeTime() time.Time {
	if w.before == nil {
		return time.Time{}
	}
	return *w.before
}

type fixPatternRequest struct {
	canonical           mcpcontract.MineRepositoryFixPatternsInput
	repository          domain.RepoRef
	window              fixPatternWindow
	symptoms            []fixPatternSymptom
	wantedOutcomes      map[mcpcontract.FixPatternOutcome]struct{}
	candidatePage       corpus.SearchPage
	hydrationLimit      int
	representativeLimit int
}

func (r fixPatternRequest) withHydrationLimit(limit int) fixPatternRequest {
	r.hydrationLimit = limit
	r.canonical.HydrationLimit = new(int)
	*r.canonical.HydrationLimit = limit
	return r
}

type fixPatternOperation uint8

const (
	fixPatternPreview fixPatternOperation = iota
	fixPatternWorkflow
)

type fixPatternSnapshotSource struct {
	ObservationWatermark int64  `json:"observation_watermark"`
	QueryDigest          string `json:"query_digest"`
}

type fixPatternSnapshotVersions struct {
	FixPatterns string `json:"fix_patterns"`
}

type fixPatternSnapshotCompleteness struct {
	Complete        bool `json:"complete"`
	Truncated       bool `json:"truncated"`
	UnknownCoverage bool `json:"unknown_coverage"`
}

type fixPatternSnapshotProvenance struct {
	Producer  string `json:"producer"`
	Operation string `json:"operation"`
}

// MineRepositoryFixPatterns submits one bounded GitHub-read/local-write
// workflow. It searches stored candidates first and hydrates only finalists
// whose merge outcome is unknown.
func (r *MCPReader) MineRepositoryFixPatterns(ctx context.Context, in mcpcontract.MineRepositoryFixPatternsInput) (mcpcontract.JobReference, error) {
	request, normalized, err := parseFixPatternInput(in)
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	id, err := r.submitJob(ctx, "mine_repository_fix_patterns", normalized, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.runFixPatternOperation(ctx, request, report, fixPatternWorkflow)
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
	request, _, err := parseFixPatternInput(mcpcontract.MineRepositoryFixPatternsInput(in))
	if err != nil {
		return mcpcontract.FixPatternReport{}, err
	}
	return r.runFixPatternOperation(ctx, request.withHydrationLimit(0), nil, fixPatternPreview)
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
	if job.Status != corpus.JobStatusSucceeded.String() {
		return mcpcontract.FixPatternReport{}, errors.New("fix-pattern report is not available until the job succeeds")
	}
	var report mcpcontract.FixPatternReport
	if err := json.Unmarshal([]byte(job.Result), &report); err != nil {
		return mcpcontract.FixPatternReport{}, fmt.Errorf("decode fix-pattern report: %w", err)
	}
	var identity struct {
		SnapshotToken string `json:"snapshot_token"`
	}
	if err := json.Unmarshal([]byte(job.Result), &identity); err != nil {
		return mcpcontract.FixPatternReport{}, fmt.Errorf("decode fix-pattern report identity: %w", err)
	}
	if identity.SnapshotToken == "" {
		var request mcpcontract.MineRepositoryFixPatternsInput
		var actions []mcpcontract.ToolCall
		if err := json.Unmarshal([]byte(job.Request), &request); err == nil {
			if _, canonical, parseErr := parseFixPatternInput(request); parseErr == nil {
				actions = append(actions, mcpcontract.RecoveryAction(canonical))
			}
		}
		return mcpcontract.FixPatternReport{}, mcpcontract.Unavailable(
			"legacy_artifact",
			"this persisted fix-pattern report predates immutable snapshot binding; rerun the fix-pattern workflow to regenerate it",
			actions...,
		)
	}
	if err := report.Validate(); err != nil {
		return mcpcontract.FixPatternReport{}, fmt.Errorf("parse fix-pattern report: %w", err)
	}
	report.Persisted = true
	return report, nil
}

func parseFixPatternInput(in mcpcontract.MineRepositoryFixPatternsInput) (fixPatternRequest, mcpcontract.MineRepositoryFixPatternsInput, error) {
	ref, err := domain.NewRepoRef(in.Repository.Owner, in.Repository.Repo)
	if err != nil {
		return fixPatternRequest{}, in, err
	}
	in.Repository.Owner = ref.Owner()
	in.Repository.Repo = ref.Repo()
	after, err := time.Parse(time.RFC3339, in.TimeWindow.UpdatedAfter)
	if err != nil {
		return fixPatternRequest{}, in, errors.New("time_window.updated_after must be RFC 3339")
	}
	var before *time.Time
	if in.TimeWindow.UpdatedBefore != "" {
		parsed, err := time.Parse(time.RFC3339, in.TimeWindow.UpdatedBefore)
		if err != nil {
			return fixPatternRequest{}, in, errors.New("time_window.updated_before must be RFC 3339")
		}
		if parsed.Before(after) {
			return fixPatternRequest{}, in, errors.New("time_window.updated_before must not be earlier than updated_after")
		}
		before = &parsed
	}
	if len(in.SymptomTaxonomy) < 1 || len(in.SymptomTaxonomy) > 12 {
		return fixPatternRequest{}, in, errors.New("symptom_taxonomy must contain 1 to 12 categories")
	}
	canonicalSymptoms := make([]mcpcontract.FixPatternSymptom, len(in.SymptomTaxonomy))
	parsedSymptoms := make([]fixPatternSymptom, len(in.SymptomTaxonomy))
	seenNames := make(map[string]struct{}, len(in.SymptomTaxonomy))
	for i := range in.SymptomTaxonomy {
		symptom := in.SymptomTaxonomy[i]
		symptom.Name = strings.TrimSpace(symptom.Name)
		if symptom.Name == "" {
			return fixPatternRequest{}, in, fmt.Errorf("symptom_taxonomy[%d].name is required", i)
		}
		key := strings.ToLower(symptom.Name)
		if _, exists := seenNames[key]; exists {
			return fixPatternRequest{}, in, fmt.Errorf("symptom_taxonomy[%d].name duplicates %q", i, symptom.Name)
		}
		seenNames[key] = struct{}{}
		if len(symptom.Terms) < 1 || len(symptom.Terms) > 12 {
			return fixPatternRequest{}, in, fmt.Errorf("symptom_taxonomy[%d].terms must contain 1 to 12 values", i)
		}
		terms := make([]string, len(symptom.Terms))
		seenTerms := make(map[string]struct{}, len(symptom.Terms))
		for j, value := range symptom.Terms {
			terms[j] = strings.TrimSpace(value)
			if terms[j] == "" {
				return fixPatternRequest{}, in, fmt.Errorf("symptom_taxonomy[%d].terms[%d] must not be empty", i, j)
			}
			key := strings.ToLower(terms[j])
			if _, exists := seenTerms[key]; exists {
				return fixPatternRequest{}, in, fmt.Errorf("symptom_taxonomy[%d].terms contains duplicate %q", i, terms[j])
			}
			seenTerms[key] = struct{}{}
		}
		canonicalSymptoms[i] = mcpcontract.FixPatternSymptom{Name: symptom.Name, Terms: append([]string(nil), terms...)}
		parsedSymptoms[i] = fixPatternSymptom{name: symptom.Name, terms: terms}
	}
	in.SymptomTaxonomy = canonicalSymptoms
	if in.CandidateLimit == 0 {
		in.CandidateLimit = mcpcontract.DefaultFixPatternCandidateLimit
	}
	if in.CandidateLimit < 1 || in.CandidateLimit > 100 {
		return fixPatternRequest{}, in, errors.New("candidate_limit must be between 1 and 100")
	}
	hydrationLimit := mcpcontract.DefaultFixPatternHydrationLimit
	if in.HydrationLimit != nil {
		hydrationLimit = *in.HydrationLimit
	}
	if hydrationLimit < 0 || hydrationLimit > 100 {
		return fixPatternRequest{}, in, errors.New("hydration_limit must be between 0 and 100")
	}
	in.HydrationLimit = &hydrationLimit
	if in.RepresentativeLimit == 0 {
		in.RepresentativeLimit = mcpcontract.DefaultFixPatternRepresentativeLimit
	}
	if in.RepresentativeLimit < 1 || in.RepresentativeLimit > 20 {
		return fixPatternRequest{}, in, errors.New("representative_limit must be between 1 and 20")
	}
	if len(in.MergeOutcomes) == 0 {
		in.MergeOutcomes = []mcpcontract.FixPatternOutcome{mcpcontract.FixPatternMerged}
	}
	seenOutcomes := make(map[mcpcontract.FixPatternOutcome]struct{}, len(in.MergeOutcomes))
	for _, outcome := range in.MergeOutcomes {
		switch outcome {
		case mcpcontract.FixPatternMerged, mcpcontract.FixPatternClosedUnmerged, mcpcontract.FixPatternSuperseded, mcpcontract.FixPatternOpen, mcpcontract.FixPatternUnknown:
		default:
			return fixPatternRequest{}, in, fmt.Errorf("unsupported merge outcome %q", outcome)
		}
		if _, exists := seenOutcomes[outcome]; exists {
			return fixPatternRequest{}, in, fmt.Errorf("duplicate merge outcome %q", outcome)
		}
		seenOutcomes[outcome] = struct{}{}
	}
	in.MergeOutcomes = append([]mcpcontract.FixPatternOutcome(nil), in.MergeOutcomes...)
	page, err := corpus.ParseSearchPage(in.CandidateLimit, "")
	if err != nil {
		return fixPatternRequest{}, in, err
	}
	return fixPatternRequest{
		canonical: in, repository: ref, window: fixPatternWindow{after: after, before: before}, symptoms: parsedSymptoms,
		wantedOutcomes: seenOutcomes, candidatePage: page, hydrationLimit: hydrationLimit, representativeLimit: in.RepresentativeLimit,
	}, in, nil
}

func collectFixPatternCandidates(ctx context.Context, c *corpus.Corpus, repo *corpus.Repository, request fixPatternRequest, progress func(string, string) error) (fixPatternAnalysis, error) {
	a := fixPatternAnalysis{
		clusters:   make([][]int64, len(request.symptoms)),
		candidates: make(map[int64]*fixPatternCandidate),
		orderedIDs: make([]int64, 0),
	}
	ref, err := domain.NewRepoRef(repo.Owner, repo.Name)
	if err != nil {
		return fixPatternAnalysis{}, fmt.Errorf("parse stored repository: %w", err)
	}
	repositoryScope, err := corpus.NewThreadRepositoryScope(ref, repo.ID)
	if err != nil {
		return fixPatternAnalysis{}, err
	}
	if err := progress("candidate_search", jobProgressCounts(0, len(request.symptoms))); err != nil {
		return fixPatternAnalysis{}, err
	}
	for symptomIndex, symptom := range request.symptoms {
		page, err := c.SearchThreadsPage(ctx, strings.Join(symptom.terms, " "), corpus.SearchFilter{
			Repository: repositoryScope,
			Kind:       corpus.PullRequestThreadKind(), UpdatedAfter: request.window.after, UpdatedBefore: request.window.beforeTime(),
			Page: request.candidatePage, TermMatch: corpus.MatchAnyTerm(),
		})
		if err != nil {
			return fixPatternAnalysis{}, fmt.Errorf("search symptom %q: %w", symptom.name, err)
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
		if err := progress("candidate_search", jobProgressCounts(symptomIndex+1, len(request.symptoms))); err != nil {
			return fixPatternAnalysis{}, err
		}
	}
	return a, nil
}

func selectFixPatternHydration(a fixPatternAnalysis, request fixPatternRequest) []mcpcontract.ThreadRef {
	unknown := countUnknownCandidates(a.candidates)
	refs := make([]mcpcontract.ThreadRef, 0, min(request.hydrationLimit, unknown))
	for _, id := range a.orderedIDs {
		candidate := a.candidates[id]
		if !needsMergeHydration(candidate.thread) || len(refs) >= request.hydrationLimit {
			continue
		}
		refs = append(refs, mcpcontract.ThreadRef{
			Owner: request.repository.Owner(), Repo: request.repository.Repo(), Kind: "pull_request", Number: candidate.thread.Number,
		})
	}
	return refs
}

func (r *MCPReader) runFixPatternOperation(ctx context.Context, request fixPatternRequest, progress func(string, string) error, operation fixPatternOperation) (mcpcontract.FixPatternReport, error) {
	in := request.canonical
	repoRef := request.repository
	var err error
	var c *corpus.Corpus
	if operation == fixPatternPreview {
		c, err = r.openReadOnlyCorpus(ctx)
	} else {
		c, err = r.openCorpus(ctx)
	}
	if err != nil {
		return mcpcontract.FixPatternReport{}, err
	}
	revision, err := beginCorpusRead(ctx, c, in.SnapshotToken)
	if err != nil {
		return mcpcontract.FixPatternReport{}, err
	}
	if progress == nil {
		progress = func(string, string) error { return nil }
	}
	repo, err := c.GetRepository(ctx, repoRef.Owner(), repoRef.Repo())
	if err != nil {
		return mcpcontract.FixPatternReport{}, err
	}
	if repo == nil {
		return mcpcontract.FixPatternReport{}, fmt.Errorf("repository %s has not been synced", repoRef)
	}
	analysis, err := collectFixPatternCandidates(ctx, c, repo, request, progress)
	if err != nil {
		return mcpcontract.FixPatternReport{}, err
	}
	unknownBefore := countUnknownCandidates(analysis.candidates)
	unknownBeforeByID := make(map[int64]bool, len(analysis.candidates))
	for id, candidate := range analysis.candidates {
		unknownBeforeByID[id] = candidate.unknownBefore
	}
	hydrationRefs := selectFixPatternHydration(analysis, request)

	hydrated, failures := 0, make([]mcpcontract.FixPatternHydrationFailure, 0)
	if operation == fixPatternWorkflow && len(hydrationRefs) > 0 {
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
		items := raw.Items
		for i, ref := range hydrationRefs {
			if i < len(items) && items[i].Status() == mcpcontract.BatchItemComplete {
				hydrated++
			} else {
				reason, message := "hydration_failed", "pull-request details were not refreshed"
				retryable := false
				if i < len(items) {
					reason = items[i].Reason()
					message = items[i].Message()
					retryable = items[i].Status() == mcpcontract.BatchItemRetryable
				}
				failures = append(failures, mcpcontract.FixPatternHydrationFailure{
					PullRequest: ref, Reason: reason, Message: message, Retryable: retryable,
				})
			}
		}
		revision, err = beginCorpusRead(ctx, c, "")
		if err != nil {
			return mcpcontract.FixPatternReport{}, err
		}
		analysis, err = collectFixPatternCandidates(ctx, c, repo, request, progress)
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

	reportClusters := make([]mcpcontract.FixPatternCluster, len(request.symptoms))
	for i, symptom := range request.symptoms {
		cluster := mcpcontract.FixPatternCluster{
			Name: symptom.name, Terms: append([]string(nil), symptom.terms...),
			CandidateCount: mcpcontract.NonNegativeInt(len(analysis.clusters[i])),
		}
		for _, id := range analysis.clusters[i] {
			thread := candidates[id].thread
			if candidates[id].unknownBefore {
				cluster.UnknownBefore++
			}
			classification := classifyFixPattern(thread, repoRef)
			outcome := fixPatternOutcome(thread, classification.superseded)
			incrementFixPatternOutcome(&cluster.Outcomes, outcome)
			if outcome == "unknown" {
				cluster.UnknownAfter++
			}
			if _, wanted := request.wantedOutcomes[outcome]; !wanted {
				continue
			}
			if len(cluster.Examples) >= request.representativeLimit {
				cluster.ExamplesTruncated = true
				continue
			}
			cluster.Examples = append(cluster.Examples, buildFixPatternExample(ctx, c, repo.ID, in.Repository, thread, outcome, classification))
		}
		reportClusters[i] = cluster
	}

	status := mcpcontract.FixPatternReportComplete
	if len(failures) > 0 || analysis.candidateTruncated || countUnknownCandidates(candidates) > 0 {
		status = mcpcontract.FixPatternReportPartial
	}
	limitations := []string{
		"Similarity-only examples are candidates, not proof that a pull request fixed a related issue.",
		"Accepted fixes require refreshed merged state and an explicit closing relationship in stored pull-request text.",
	}
	if analysis.candidateTruncated {
		limitations = append(limitations, "At least one symptom category exceeded candidate_limit; coverage is truncated.")
	}
	var recovery *mcpcontract.RecoveryPlan
	if analysis.candidateTruncated || countUnknownCandidates(candidates) > 0 || len(failures) > 0 {
		next := in
		if analysis.candidateTruncated {
			next.CandidateLimit = min(100, max(in.CandidateLimit*2, in.CandidateLimit+1))
		}
		if next.HydrationLimit != nil && (*next.HydrationLimit > 0 || countUnknownCandidates(candidates) > 0) {
			value := min(100, max(*next.HydrationLimit*2, *next.HydrationLimit+1))
			next.HydrationLimit = &value
		}
		if operation == fixPatternPreview && countUnknownCandidates(candidates) == 0 && len(failures) == 0 {
			recovery = recoveryPlan("fix_pattern_candidates_truncated", "The candidate population exceeded candidate_limit. Rerun the read-only preview with a larger candidate_limit.", mcpcontract.RecoveryAction(mcpcontract.PreviewRepositoryFixPatternsInput(next)))
		} else {
			recovery = recoveryPlan("fix_pattern_coverage_incomplete", "The report retained unknown or bounded candidate outcomes. Rerun the workflow with the returned larger bounds to hydrate and recompute the report.", mcpcontract.RecoveryAction(next))
		}
	}
	if err := finishCorpusRead(ctx, c, revision); err != nil {
		return mcpcontract.FixPatternReport{}, err
	}
	inputBytes, _ := json.Marshal(in)
	queryHash := sha256.Sum256(inputBytes)
	report := mcpcontract.FixPatternReport{
		Status: status, Repository: in.Repository, TimeWindow: in.TimeWindow,
		GeneratedAt: r.now().Format(time.RFC3339),
		Coverage: mcpcontract.FixPatternCoverage{
			CandidateMatches: mcpcontract.NonNegativeInt(analysis.candidateMatches), UniqueCandidates: mcpcontract.NonNegativeInt(len(candidates)),
			UnknownBefore: mcpcontract.NonNegativeInt(unknownBefore), SelectedForHydration: mcpcontract.NonNegativeInt(len(hydrationRefs)),
			Hydrated: mcpcontract.NonNegativeInt(hydrated), HydrationFailed: mcpcontract.NonNegativeInt(len(failures)),
			UnknownAfter: mcpcontract.NonNegativeInt(countUnknownCandidates(candidates)), CandidateTruncated: analysis.candidateTruncated,
		},
		Clusters: reportClusters, Failures: failures, Limitations: limitations, Persisted: operation == fixPatternWorkflow, SnapshotToken: snapshotIdentity(in.SnapshotToken, revision),
		ObservationWatermark: revision, QueryDigestSHA256: hex.EncodeToString(queryHash[:]),
		Complete: status == mcpcontract.FixPatternReportComplete, Truncated: analysis.candidateTruncated, UnknownCoverage: countUnknownCandidates(candidates) > 0, Recovery: recovery,
	}
	if err := report.Validate(); err != nil {
		return mcpcontract.FixPatternReport{}, err
	}
	if operation == fixPatternWorkflow {
		materialization, err := corpus.NewSnapshotMaterialization(
			"fix_pattern_report", in.Repository,
			fixPatternSnapshotSource{ObservationWatermark: revision, QueryDigest: report.QueryDigestSHA256},
			fixPatternSnapshotVersions{FixPatterns: "v1"},
			fixPatternSnapshotCompleteness{Complete: report.Complete, Truncated: report.Truncated, UnknownCoverage: report.UnknownCoverage},
			fixPatternSnapshotProvenance{Producer: "gitcontribute", Operation: "workflow.mine_repository_fix_patterns"}, report,
		)
		if err != nil {
			return mcpcontract.FixPatternReport{}, err
		}
		snapshot, err := c.MaterializeReadSnapshot(ctx, materialization)
		if err != nil {
			return mcpcontract.FixPatternReport{}, err
		}
		report.SnapshotToken, report.ArtifactDigest = snapshot.Token, snapshot.ArtifactDigest
	} else {
		tokenHash := sha256.Sum256([]byte(fmt.Sprintf("offline-fix-pattern\x00%d\x00%s", revision, report.QueryDigestSHA256)))
		report.SnapshotToken = "ephemeral:" + hex.EncodeToString(tokenHash[:])
		report.Limitations = append(report.Limitations, "Preview snapshot identity is transaction-bound and is not a durable resource token because preview performs no local writes.")
	}
	return report, nil
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
	return thread.State == "closed" && !thread.Merge.Known()
}

func fixPatternOutcome(thread corpus.Thread, superseded bool) mcpcontract.FixPatternOutcome {
	switch {
	case thread.Merge.IsMerged():
		return mcpcontract.FixPatternMerged
	case thread.State != "closed":
		return mcpcontract.FixPatternOpen
	case !thread.Merge.Known():
		return mcpcontract.FixPatternUnknown
	case superseded:
		return mcpcontract.FixPatternSuperseded
	default:
		return mcpcontract.FixPatternClosedUnmerged
	}
}

func incrementFixPatternOutcome(counts *mcpcontract.FixPatternOutcomeCounts, outcome mcpcontract.FixPatternOutcome) {
	switch outcome {
	case mcpcontract.FixPatternMerged:
		counts.Merged++
	case mcpcontract.FixPatternClosedUnmerged:
		counts.ClosedUnmerged++
	case mcpcontract.FixPatternSuperseded:
		counts.Superseded++
	case mcpcontract.FixPatternOpen:
		counts.Open++
	case mcpcontract.FixPatternUnknown:
		counts.Unknown++
	}
}

func buildFixPatternExample(ctx context.Context, c *corpus.Corpus, repoID int64, repository mcpcontract.RepositoryRef, thread corpus.Thread, outcome mcpcontract.FixPatternOutcome, classification fixPatternClassification) mcpcontract.FixPatternExample {
	example := mcpcontract.FixPatternExample{
		PullRequest: mcpcontract.ThreadRef{Owner: repository.Owner, Repo: repository.Repo, Kind: "pull_request", Number: thread.Number},
		Title:       thread.Title, Outcome: outcome, Relationship: classification.relationship, RelationshipEvidence: classification.evidence,
		AcceptedFix: outcome == mcpcontract.FixPatternMerged && classification.relationship == mcpcontract.FixPatternCloses,
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
			example.RelatedThread.Kind = string(related.Kind)
		}
	}
	return example
}

func classifyFixPattern(thread corpus.Thread, repository domain.RepoRef) fixPatternClassification {
	refs := relatedwork.Extract(thread.Body, repository)
	classification := fixPatternClassification{relationship: mcpcontract.FixPatternSimilarityOnly}
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
			Owner: ref.Repo.Owner(), Repo: ref.Repo.Repo(), Kind: string(ref.Kind), Number: ref.Number,
		}
		classification.evidence = ref.Evidence
		switch ref.Relation {
		case relatedwork.RelationClaimsToClose:
			classification.relationship = mcpcontract.FixPatternCloses
		case relatedwork.RelationReplaces, relatedwork.RelationSupersededBy:
			classification.relationship = mcpcontract.FixPatternExplicitReplacement
		default:
			classification.relationship = mcpcontract.FixPatternReferences
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
		{name: mcpcontract.FixPatternRegressionTest, terms: []string{"regression test", "unit test", "test coverage"}},
		{name: mcpcontract.FixPatternReproduction, terms: []string{"reproducer", "reproduction", "repro case"}},
		{name: mcpcontract.FixPatternBenchmark, terms: []string{"benchmark", "throughput", "latency"}},
		{name: mcpcontract.FixPatternBeforeAfter, terms: []string{"before and after", "before/after"}},
		{name: mcpcontract.FixPatternScreenshot, terms: []string{"screenshot"}},
	} {
		if slices.ContainsFunc(candidate.terms, func(term string) bool { return strings.Contains(lower, term) }) {
			styles = append(styles, candidate.name)
		}
	}
	return styles
}
