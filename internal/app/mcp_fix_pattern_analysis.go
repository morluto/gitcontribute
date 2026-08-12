package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/fixpatterns"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

type fixPatternAnalysisRequest struct {
	canonical           mcpcontract.AnalyzeFixPatternsInput
	repository          domain.RepoRef
	after               time.Time
	before              time.Time
	wantedOutcomes      map[mcpcontract.FixPatternOutcome]struct{}
	candidatePage       corpus.SearchPage
	representativeLimit int
}

func (r *MCPReader) AnalyzeFixPatterns(ctx context.Context, in mcpcontract.AnalyzeFixPatternsInput) (mcpcontract.AnalyzeFixPatternsOutput, error) {
	request, err := parseFixPatternAnalysisInput(in)
	if err != nil {
		return mcpcontract.AnalyzeFixPatternsOutput{}, err
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.AnalyzeFixPatternsOutput{}, err
	}
	revision, err := beginCorpusRead(ctx, c, request.canonical.SnapshotToken)
	if err != nil {
		return mcpcontract.AnalyzeFixPatternsOutput{}, err
	}
	repository, err := c.GetRepository(ctx, request.repository.Owner(), request.repository.Repo())
	if err != nil {
		return mcpcontract.AnalyzeFixPatternsOutput{}, err
	}
	if repository == nil {
		return mcpcontract.AnalyzeFixPatternsOutput{}, mcpcontract.Unavailable(
			"repository_not_indexed", fmt.Sprintf("Repository %s is not present in the local corpus.", request.repository),
			syncRepositoryContextCall(request.repository.Owner(), request.repository.Repo()),
		)
	}
	scope, err := corpus.NewThreadRepositoryScope(request.repository, repository.ID)
	if err != nil {
		return mcpcontract.AnalyzeFixPatternsOutput{}, err
	}
	out := mcpcontract.AnalyzeFixPatternsOutput{
		Status: "complete", Repository: request.canonical.Repository, TimeWindow: request.canonical.TimeWindow,
		Clusters: make([]mcpcontract.FixPatternCluster, len(request.canonical.SymptomTaxonomy)),
		Limitations: []string{
			"Similarity-only examples are candidates, not proof that a pull request fixed a related issue.",
			"Accepted fixes require observed merged state and an explicit closing relationship in stored pull-request text.",
		},
		SnapshotToken: snapshotIdentity(request.canonical.SnapshotToken, revision),
	}
	unique := make(map[int64]struct{})
	unknown := make(map[int64]struct{})
	for index, symptom := range request.canonical.SymptomTaxonomy {
		if err := ctx.Err(); err != nil {
			return mcpcontract.AnalyzeFixPatternsOutput{}, err
		}
		page, err := c.SearchThreadsPage(ctx, strings.Join(symptom.Terms, " "), corpus.SearchFilter{
			Repository: scope, Kind: corpus.PullRequestThreadKind(), UpdatedAfter: request.after, UpdatedBefore: request.before,
			Page: request.candidatePage, TermMatch: corpus.MatchAnyTerm(),
		})
		if err != nil {
			return mcpcontract.AnalyzeFixPatternsOutput{}, fmt.Errorf("search symptom %q: %w", symptom.Name, err)
		}
		cluster := mcpcontract.FixPatternCluster{Name: symptom.Name, Terms: append([]string(nil), symptom.Terms...), CandidateCount: mcpcontract.NonNegativeInt(page.Total)}
		out.Coverage.CandidateMatches += mcpcontract.NonNegativeInt(page.Total)
		out.Coverage.CandidateTruncated = out.Coverage.CandidateTruncated || page.Total > len(page.Threads)
		for _, thread := range page.Threads {
			unique[thread.ID] = struct{}{}
			classification := fixpatterns.Classify(request.repository, fixpatterns.PullRequest{State: thread.State, Body: thread.Body, Merge: thread.Merge})
			incrementFixPatternOutcome(&cluster.Outcomes, mcpcontract.FixPatternOutcome(classification.Outcome))
			if classification.Outcome == fixpatterns.Unknown {
				if _, exists := unknown[thread.ID]; !exists {
					unknown[thread.ID] = struct{}{}
					if len(out.Coverage.UnknownPullRequests) < 100 {
						out.Coverage.UnknownPullRequests = append(out.Coverage.UnknownPullRequests, mcpcontract.ThreadRef{Owner: request.repository.Owner(), Repo: request.repository.Repo(), Kind: string(domain.PullRequestKind), Number: thread.Number})
					}
				}
			}
			if _, wanted := request.wantedOutcomes[mcpcontract.FixPatternOutcome(classification.Outcome)]; !wanted {
				continue
			}
			if len(cluster.Examples) == request.representativeLimit {
				cluster.ExamplesTruncated = true
				continue
			}
			cluster.Examples = append(cluster.Examples, fixPatternExample(request.repository, thread, classification))
		}
		out.Clusters[index] = cluster
	}
	out.Coverage.UniqueCandidates = mcpcontract.NonNegativeInt(len(unique))
	out.Coverage.UnknownOutcomes = mcpcontract.NonNegativeInt(len(unknown))
	out.Coverage.UnknownRefsTruncated = len(unknown) > len(out.Coverage.UnknownPullRequests)
	history, err := c.GetCoverage(ctx, repository.ID, nil, "threads")
	if err != nil {
		return mcpcontract.AnalyzeFixPatternsOutput{}, err
	}
	switch {
	case history == nil:
		out.Coverage.HistoryStatus = "unknown"
	case history.Complete:
		out.Coverage.HistoryStatus = "complete"
	default:
		out.Coverage.HistoryStatus = "partial"
	}
	out.Truncated = out.Coverage.CandidateTruncated
	out.UnknownCoverage = out.Coverage.HistoryStatus != "complete" || out.Coverage.UnknownOutcomes > 0
	if out.Truncated || out.UnknownCoverage {
		out.Status = "partial"
		out.Recovery = fixPatternRecovery(request, out)
	}
	if err := finishCorpusRead(ctx, c, revision); err != nil {
		return mcpcontract.AnalyzeFixPatternsOutput{}, err
	}
	out.Provenance, err = offlineReadProvenance("fix_pattern_analysis", revision, request.canonical, out.Truncated, out.UnknownCoverage)
	if err != nil {
		return mcpcontract.AnalyzeFixPatternsOutput{}, err
	}
	out.Provenance.Recovery = out.Recovery
	return out, nil
}

func parseFixPatternAnalysisInput(in mcpcontract.AnalyzeFixPatternsInput) (fixPatternAnalysisRequest, error) {
	repository, err := domain.NewRepoRef(in.Repository.Owner, in.Repository.Repo)
	if err != nil {
		return fixPatternAnalysisRequest{}, err
	}
	in.Repository = mcpcontract.RepositoryRef{Owner: repository.Owner(), Repo: repository.Repo()}
	after, err := time.Parse(time.RFC3339, in.TimeWindow.UpdatedAfter)
	if err != nil {
		return fixPatternAnalysisRequest{}, errors.New("time_window.updated_after must be RFC 3339")
	}
	var before time.Time
	if in.TimeWindow.UpdatedBefore != "" {
		before, err = time.Parse(time.RFC3339, in.TimeWindow.UpdatedBefore)
		if err != nil {
			return fixPatternAnalysisRequest{}, errors.New("time_window.updated_before must be RFC 3339")
		}
		if before.Before(after) {
			return fixPatternAnalysisRequest{}, errors.New("time_window.updated_before must not be earlier than updated_after")
		}
	}
	if len(in.SymptomTaxonomy) < 1 || len(in.SymptomTaxonomy) > 12 {
		return fixPatternAnalysisRequest{}, errors.New("symptom_taxonomy must contain 1 to 12 categories")
	}
	seenNames := make(map[string]struct{}, len(in.SymptomTaxonomy))
	for i := range in.SymptomTaxonomy {
		in.SymptomTaxonomy[i].Name = strings.TrimSpace(in.SymptomTaxonomy[i].Name)
		if in.SymptomTaxonomy[i].Name == "" || len(in.SymptomTaxonomy[i].Terms) < 1 || len(in.SymptomTaxonomy[i].Terms) > 12 {
			return fixPatternAnalysisRequest{}, fmt.Errorf("symptom_taxonomy[%d] requires a name and 1 to 12 terms", i)
		}
		nameKey := strings.ToLower(in.SymptomTaxonomy[i].Name)
		if _, duplicate := seenNames[nameKey]; duplicate {
			return fixPatternAnalysisRequest{}, fmt.Errorf("symptom_taxonomy[%d].name duplicates %q", i, in.SymptomTaxonomy[i].Name)
		}
		seenNames[nameKey] = struct{}{}
		seenTerms := make(map[string]struct{}, len(in.SymptomTaxonomy[i].Terms))
		for j := range in.SymptomTaxonomy[i].Terms {
			term := strings.TrimSpace(in.SymptomTaxonomy[i].Terms[j])
			if term == "" {
				return fixPatternAnalysisRequest{}, fmt.Errorf("symptom_taxonomy[%d].terms[%d] is required", i, j)
			}
			key := strings.ToLower(term)
			if _, duplicate := seenTerms[key]; duplicate {
				return fixPatternAnalysisRequest{}, fmt.Errorf("symptom_taxonomy[%d].terms contains duplicate %q", i, term)
			}
			seenTerms[key] = struct{}{}
			in.SymptomTaxonomy[i].Terms[j] = term
		}
	}
	if in.CandidateLimit == 0 {
		in.CandidateLimit = mcpcontract.DefaultFixPatternCandidateLimit
	}
	if in.CandidateLimit < 1 || in.CandidateLimit > 100 {
		return fixPatternAnalysisRequest{}, errors.New("candidate_limit must be between 1 and 100")
	}
	if in.RepresentativeLimit == 0 {
		in.RepresentativeLimit = mcpcontract.DefaultFixPatternRepresentativeLimit
	}
	if in.RepresentativeLimit < 1 || in.RepresentativeLimit > 20 {
		return fixPatternAnalysisRequest{}, errors.New("representative_limit must be between 1 and 20")
	}
	if len(in.MergeOutcomes) == 0 {
		in.MergeOutcomes = []mcpcontract.FixPatternOutcome{mcpcontract.FixPatternMerged}
	}
	wanted := make(map[mcpcontract.FixPatternOutcome]struct{}, len(in.MergeOutcomes))
	for _, outcome := range in.MergeOutcomes {
		switch outcome {
		case mcpcontract.FixPatternMerged, mcpcontract.FixPatternClosedUnmerged, mcpcontract.FixPatternSuperseded, mcpcontract.FixPatternOpen, mcpcontract.FixPatternUnknown:
		default:
			return fixPatternAnalysisRequest{}, fmt.Errorf("unsupported merge outcome %q", outcome)
		}
		if _, duplicate := wanted[outcome]; duplicate {
			return fixPatternAnalysisRequest{}, fmt.Errorf("duplicate merge outcome %q", outcome)
		}
		wanted[outcome] = struct{}{}
	}
	page, err := corpus.ParseSearchPage(in.CandidateLimit, "")
	if err != nil {
		return fixPatternAnalysisRequest{}, err
	}
	return fixPatternAnalysisRequest{canonical: in, repository: repository, after: after, before: before, wantedOutcomes: wanted, candidatePage: page, representativeLimit: in.RepresentativeLimit}, nil
}

func fixPatternExample(repository domain.RepoRef, thread corpus.Thread, classification fixpatterns.Classification) mcpcontract.FixPatternExample {
	out := mcpcontract.FixPatternExample{
		PullRequest: mcpcontract.ThreadRef{Owner: repository.Owner(), Repo: repository.Repo(), Kind: string(domain.PullRequestKind), Number: thread.Number},
		Title:       thread.Title, Outcome: mcpcontract.FixPatternOutcome(classification.Outcome), Relationship: mcpcontract.FixPatternRelationship(classification.Relationship),
		RelationshipEvidence: classification.RelationshipEvidence, AcceptedFix: classification.Outcome == fixpatterns.Merged && classification.Relationship == fixpatterns.Closes,
		UpdatedAt: formatTime(thread.SourceUpdatedAt),
	}
	for _, style := range classification.ProofStyles {
		out.ProofStyles = append(out.ProofStyles, mcpcontract.FixPatternProofStyle(style))
	}
	if classification.RelatedRepository.IsValid() {
		out.RelatedThread = &mcpcontract.ThreadRef{Owner: classification.RelatedRepository.Owner(), Repo: classification.RelatedRepository.Repo(), Kind: string(classification.RelatedKind), Number: classification.RelatedNumber}
	}
	return out
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

func fixPatternRecovery(request fixPatternAnalysisRequest, out mcpcontract.AnalyzeFixPatternsOutput) *mcpcontract.RecoveryPlan {
	next := request.canonical
	next.SnapshotToken = ""
	if out.Truncated {
		next.CandidateLimit = min(100, max(next.CandidateLimit*2, next.CandidateLimit+1))
	}
	var actions []mcpcontract.ToolCall
	if out.Coverage.HistoryStatus != "complete" {
		actions = append(actions, mcpcontract.RecoveryAction(mcpcontract.SyncThreadsInput{
			Selection: "repositories", Repositories: []mcpcontract.RepositoryRef{next.Repository}, Kind: "pull_request", State: "all", UpdatedAfter: next.TimeWindow.UpdatedAfter,
		}))
	}
	if out.Coverage.UnknownOutcomes > 0 {
		actions = append(actions, mcpcontract.RecoveryAction(mcpcontract.HydrateThreadsInput{Threads: append([]mcpcontract.ThreadRef(nil), out.Coverage.UnknownPullRequests...), Facets: []string{FacetPRDetails}, MaxPages: 1}))
	}
	actions = append(actions, mcpcontract.RecoveryAction(next))
	return recoveryPlan("fix_pattern_coverage_incomplete", "Acquire the explicitly listed missing history or pull-request details, then repeat the same offline analysis before treating the result as exhaustive.", actions...)
}
