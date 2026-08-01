package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func TestMineRepositoryFixPatternsSeparatesAcceptedFixesFromSimilarity(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	repo, err := svc.corpus.ApplyRepositoryObservation(ctx, "owner", "repo", "R_1", now, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, thread := range []corpus.Thread{
		{RepositoryID: repo.ID, Kind: corpus.ThreadKindIssue, Number: 1, State: "open", Title: "Numeric drift on RDNA", Body: "split cumsum produces the wrong result", SourceUpdatedAt: now},
		{RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 2, State: "closed", Title: "Restrict barrier conversion to CDNA", Body: "Fixes #1.\n\nRegression test covers numeric drift.", Merged: true, MergedKnown: true, SourceUpdatedAt: now.Add(time.Hour)},
		{RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 3, State: "closed", Title: "Try a different barrier lowering", Body: "Similar numeric drift was observed, with a reproduction.", MergedKnown: true, SourceUpdatedAt: now.Add(2 * time.Hour)},
		{RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 4, State: "closed", Title: "Investigate numeric drift", Body: "Numeric drift investigation.", SourceUpdatedAt: now.Add(3 * time.Hour)},
		{RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 5, State: "closed", Title: "Earlier numeric drift attempt", Body: "Numeric drift attempt. Superseded by #2.", MergedKnown: true, SourceUpdatedAt: now.Add(3 * time.Hour)},
		{RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 6, State: "open", Title: "New numeric drift approach", Body: "Numeric drift work remains open.", SourceUpdatedAt: now.Add(3 * time.Hour)},
	} {
		if _, err := svc.corpus.UpsertThread(ctx, thread, `{}`); err != nil {
			t.Fatal(err)
		}
	}
	zero := 0
	input, err := normalizeFixPatternInput(mcpcontract.MineRepositoryFixPatternsInput{
		Repository: mcpcontract.RepositoryRef{Owner: "owner", Repo: "repo"},
		TimeWindow: mcpcontract.FixPatternTimeWindow{
			UpdatedAfter:  now.Add(-time.Hour).Format(time.RFC3339),
			UpdatedBefore: now.Add(4 * time.Hour).Format(time.RFC3339),
		},
		SymptomTaxonomy: []mcpcontract.FixPatternSymptom{{Name: "numeric drift", Terms: []string{"numeric", "drift"}}},
		MergeOutcomes:   []mcpcontract.FixPatternOutcome{"merged", "closed_unmerged", "superseded", "open", "unknown"},
		HydrationLimit:  &zero,
	})
	if err != nil {
		t.Fatal(err)
	}

	report, err := (&MCPReader{Service: svc}).mineRepositoryFixPatterns(ctx, input, func(string, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "partial" || report.Coverage.UniqueCandidates != 5 || report.Coverage.UnknownBefore != 1 || report.Coverage.UnknownAfter != 1 {
		t.Fatalf("coverage = %+v, status = %q", report.Coverage, report.Status)
	}
	if report.Recovery == nil || len(report.Recovery.Then) != 1 || report.Recovery.Then[0].Type != "mine_repository_fix_patterns" {
		t.Fatalf("fix-pattern recovery = %+v", report.Recovery)
	}
	if len(report.Clusters) != 1 || len(report.Clusters[0].Examples) != 5 {
		t.Fatalf("clusters = %+v", report.Clusters)
	}
	examples := make(map[int]mcpcontract.FixPatternExample)
	for _, example := range report.Clusters[0].Examples {
		examples[example.PullRequest.Number] = example
	}
	if got := examples[2]; got.PullRequest.Kind != "pull_request" || !got.AcceptedFix || got.Outcome != "merged" || got.Relationship != "closes" || got.RelatedThread == nil || got.RelatedThread.Number != 1 {
		t.Fatalf("accepted example = %+v", got)
	}
	if got := examples[3]; got.AcceptedFix || got.Outcome != "closed_unmerged" || got.Relationship != "similarity_only" {
		t.Fatalf("closed similarity example = %+v", got)
	}
	if got := examples[4]; got.Outcome != "unknown" || got.AcceptedFix {
		t.Fatalf("unknown example = %+v", got)
	}
	if got := examples[5]; got.Outcome != "superseded" || got.Relationship != "explicit_replacement" || got.RelatedThread == nil || got.RelatedThread.Number != 2 {
		t.Fatalf("superseded example = %+v", got)
	}
	if got := examples[6]; got.Outcome != "open" || got.AcceptedFix {
		t.Fatalf("open example = %+v", got)
	}
}

func TestMineRepositoryFixPatternsHydratesOnlyUnknownFinalists(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	repo, err := svc.corpus.ApplyRepositoryObservation(ctx, "owner", "repo", "R_1", now, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.corpus.UpsertThread(ctx, corpus.Thread{
		RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 2, State: "closed",
		Title: "Fix numeric drift", Body: "Fixes #1 with a regression test.", SourceUpdatedAt: now,
	}, `{}`); err != nil {
		t.Fatal(err)
	}
	mergedAt := now.Add(time.Hour)
	reader := &exactHydrationReader{
		fakeHydrationReader: &fakeHydrationReader{prDetails: github.PullRequestDetails{
			Number: 2, State: "closed", Title: "Fix numeric drift", Body: "Fixes #1 with a regression test.",
			Merged: true, MergedAt: &mergedAt, UpdatedAt: mergedAt,
		}},
		header: github.Issue{
			RepositoryOwner: "owner", RepositoryName: "repo", Number: 2, Kind: github.ThreadKindPullRequest,
			State: "closed", Title: "Fix numeric drift", Body: "Fixes #1 with a regression test.",
			UpdatedAt: mergedAt,
		},
	}
	svc.SetGitHubReader(reader)
	one := 1
	input, err := normalizeFixPatternInput(mcpcontract.MineRepositoryFixPatternsInput{
		Repository:      mcpcontract.RepositoryRef{Owner: "owner", Repo: "repo"},
		TimeWindow:      mcpcontract.FixPatternTimeWindow{UpdatedAfter: now.Add(-time.Hour).Format(time.RFC3339)},
		SymptomTaxonomy: []mcpcontract.FixPatternSymptom{{Name: "numeric drift", Terms: []string{"numeric drift"}}},
		HydrationLimit:  &one,
	})
	if err != nil {
		t.Fatal(err)
	}

	report, err := (&MCPReader{Service: svc}).mineRepositoryFixPatterns(ctx, input, func(string, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if report.Coverage.UnknownBefore != 1 || report.Coverage.SelectedForHydration != 1 || report.Coverage.Hydrated != 1 || report.Coverage.UnknownAfter != 0 {
		t.Fatalf("coverage = %+v, failures = %+v", report.Coverage, report.Failures)
	}
	if report.Status != "complete" || len(report.Clusters) != 1 || len(report.Clusters[0].Examples) != 1 || !report.Clusters[0].Examples[0].AcceptedFix {
		t.Fatalf("report = %+v", report)
	}
}

func TestPreviewRepositoryFixPatternsIsReadOnlyAndNeverHydrates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	repo, err := svc.corpus.ApplyRepositoryObservation(ctx, "owner", "repo", "R_1", now, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.corpus.UpsertThread(ctx, corpus.Thread{
		RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 2, State: "closed",
		Title: "Fix numeric drift", Body: "Numeric drift reproduction", SourceUpdatedAt: now,
	}, `{}`); err != nil {
		t.Fatal(err)
	}
	beforeRevision, err := svc.corpus.CorpusRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	beforeJobs, err := svc.corpus.ListJobs(ctx, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	report, err := (&MCPReader{Service: svc}).PreviewRepositoryFixPatterns(ctx, mcpcontract.PreviewRepositoryFixPatternsInput{
		Repository:      mcpcontract.RepositoryRef{Owner: "owner", Repo: "repo"},
		TimeWindow:      mcpcontract.FixPatternTimeWindow{UpdatedAfter: now.Add(-time.Hour).Format(time.RFC3339)},
		SymptomTaxonomy: []mcpcontract.FixPatternSymptom{{Name: "drift", Terms: []string{"drift"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Persisted || !strings.HasPrefix(report.SnapshotToken, "ephemeral:") || report.Coverage.SelectedForHydration != 0 || report.Coverage.Hydrated != 0 {
		t.Fatalf("preview report = %+v", report)
	}
	afterRevision, err := svc.corpus.CorpusRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	afterJobs, err := svc.corpus.ListJobs(ctx, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if afterRevision != beforeRevision || len(afterJobs) != len(beforeJobs) {
		t.Fatalf("preview mutated corpus: revision %d -> %d, jobs %d -> %d", beforeRevision, afterRevision, len(beforeJobs), len(afterJobs))
	}
}

func TestGetFixPatternReportRejectsLegacyUnboundArtifact(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)
	job, err := svc.corpus.CreateJob(ctx, "mine_repository_fix_patterns", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.StartJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.TransitionJob(ctx, job.ID, corpus.JobStatusRunning, corpus.JobStatusSucceeded, `{"status":"complete","persisted":true}`, ""); err != nil {
		t.Fatal(err)
	}
	_, err = (&MCPReader{svc}).GetFixPatternReport(ctx, job.ID)
	var toolErr *mcpcontract.ToolError
	if !errors.As(err, &toolErr) || toolErr.Code != "legacy_artifact" {
		t.Fatalf("legacy report error = %v", err)
	}
}

func TestNormalizeFixPatternInputRejectsInvalidWindow(t *testing.T) {
	t.Parallel()
	for _, window := range []mcpcontract.FixPatternTimeWindow{
		{UpdatedAfter: "not-a-date"},
		{UpdatedAfter: "2026-07-02T00:00:00Z", UpdatedBefore: "2026-07-01T00:00:00Z"},
	} {
		_, err := normalizeFixPatternInput(mcpcontract.MineRepositoryFixPatternsInput{
			Repository:      mcpcontract.RepositoryRef{Owner: "owner", Repo: "repo"},
			TimeWindow:      window,
			SymptomTaxonomy: []mcpcontract.FixPatternSymptom{{Name: "drift", Terms: []string{"drift"}}},
		})
		if err == nil {
			t.Fatalf("invalid time window was accepted: %+v", window)
		}
	}
}
