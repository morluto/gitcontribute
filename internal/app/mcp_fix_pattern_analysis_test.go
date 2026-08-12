package app

import (
	"context"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func TestAnalyzeFixPatternsIsOfflineAndPreservesUnknownOutcomes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	repository, err := svc.corpus.ApplyRepositoryObservation(ctx, "owner", "repo", "R_1", now, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, thread := range []corpus.Thread{
		{RepositoryID: repository.ID, Kind: domain.PullRequestKind, Number: 2, State: domain.ClosedState, Title: "Fix numeric drift", Body: "Fixes #1. Regression test covers numeric drift.", Merge: domain.MergedStatus(time.Time{}), SourceUpdatedAt: now.Add(time.Hour)},
		{RepositoryID: repository.ID, Kind: domain.PullRequestKind, Number: 3, State: domain.ClosedState, Title: "Investigate numeric drift", Body: "Numeric drift reproduction.", Merge: domain.UnknownMergeStatus(), SourceUpdatedAt: now.Add(2 * time.Hour)},
	} {
		if _, err := svc.corpus.UpsertThread(ctx, thread, `{}`); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.corpus.ApplyFacetObservationSet(ctx, repository.ID, nil, "threads", now.Add(2*time.Hour), nil, true, 0); err != nil {
		t.Fatal(err)
	}
	beforeRevision, err := svc.corpus.CorpusRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	report, err := (&MCPReader{Service: svc}).AnalyzeFixPatterns(ctx, mcpcontract.AnalyzeFixPatternsInput{
		Repository:      mcpcontract.RepositoryRef{Owner: "owner", Repo: "repo"},
		TimeWindow:      mcpcontract.FixPatternTimeWindow{UpdatedAfter: now.Format(time.RFC3339)},
		SymptomTaxonomy: []mcpcontract.FixPatternSymptom{{Name: "numeric drift", Terms: []string{"numeric", "drift"}}},
		MergeOutcomes:   []mcpcontract.FixPatternOutcome{mcpcontract.FixPatternMerged, mcpcontract.FixPatternUnknown},
	})
	if err != nil {
		t.Fatal(err)
	}
	afterRevision, err := svc.corpus.CorpusRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if afterRevision != beforeRevision {
		t.Fatalf("offline analysis mutated corpus revision: %d -> %d", beforeRevision, afterRevision)
	}
	if report.Status != "partial" || report.Coverage.UniqueCandidates != 2 || report.Coverage.UnknownOutcomes != 1 || report.Coverage.HistoryStatus != "complete" {
		t.Fatalf("coverage = %+v status=%q", report.Coverage, report.Status)
	}
	if report.Recovery == nil || len(report.Recovery.Then) != 2 || report.Recovery.Then[0].Type() != "hydrate_threads" || report.Recovery.Then[1].Type() != "analyze_fix_patterns" {
		t.Fatalf("recovery = %+v", report.Recovery)
	}
	if len(report.Clusters) != 1 || len(report.Clusters[0].Examples) != 2 {
		t.Fatalf("clusters = %+v", report.Clusters)
	}
	examples := make(map[int]mcpcontract.FixPatternExample)
	for _, example := range report.Clusters[0].Examples {
		examples[example.PullRequest.Number] = example
	}
	if accepted := examples[2]; !accepted.AcceptedFix || accepted.Relationship != mcpcontract.FixPatternCloses || len(accepted.ProofStyles) != 1 {
		t.Fatalf("accepted example = %+v", accepted)
	}
	if unknown := examples[3]; unknown.Outcome != mcpcontract.FixPatternUnknown || unknown.AcceptedFix {
		t.Fatalf("unknown example = %+v", unknown)
	}
}

func TestAnalyzeFixPatternsReportsMissingHistoryRecovery(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	if _, err := svc.corpus.ApplyRepositoryObservation(ctx, "owner", "repo", "R_1", now, `{}`); err != nil {
		t.Fatal(err)
	}
	report, err := (&MCPReader{Service: svc}).AnalyzeFixPatterns(ctx, mcpcontract.AnalyzeFixPatternsInput{
		Repository:      mcpcontract.RepositoryRef{Owner: "owner", Repo: "repo"},
		TimeWindow:      mcpcontract.FixPatternTimeWindow{UpdatedAfter: now.Format(time.RFC3339)},
		SymptomTaxonomy: []mcpcontract.FixPatternSymptom{{Name: "panic", Terms: []string{"panic"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.UnknownCoverage || report.Coverage.HistoryStatus != "unknown" || report.Recovery == nil || report.Recovery.Then[0].Type() != "sync_threads" {
		t.Fatalf("report = %+v", report)
	}
}

func TestFixPatternRecoveryRerunsAfterAcquisitionWithoutStaleSnapshot(t *testing.T) {
	request := fixPatternAnalysisRequest{canonical: mcpcontract.AnalyzeFixPatternsInput{
		Repository: mcpcontract.RepositoryRef{Owner: "owner", Repo: "repo"}, SnapshotToken: "snapshot:stale", CandidateLimit: 10,
	}}
	plan := fixPatternRecovery(request, mcpcontract.AnalyzeFixPatternsOutput{Coverage: mcpcontract.FixPatternCoverage{HistoryStatus: "unknown"}})
	next, ok := mcpcontract.RecoveryInput[mcpcontract.AnalyzeFixPatternsInput](plan.Then[len(plan.Then)-1])
	if !ok || next.SnapshotToken != "" {
		t.Fatalf("analysis recovery retained stale snapshot: %+v", plan)
	}
}
