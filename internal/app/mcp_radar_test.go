package app

import (
	"context"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func TestRankContributionCandidatesPreservesBoundsAndStableRanks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	svc.SetGitHubReader(panicRadarReader{})
	seedCandidateRankingRepository(ctx, t, svc, "rocket", 5, now)

	reader := &MCPReader{svc}
	bounded, err := reader.RankContributionCandidates(ctx, mcpcontract.RankContributionCandidatesInput{
		Repositories: []mcpcontract.RepositoryRef{{Owner: "acme", Repo: "rocket"}}, Limit: 2, MaxResultsPerRepository: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if bounded.Status != "partial" || bounded.Total != 5 || len(bounded.Candidates) != 2 || !bounded.Truncated || bounded.SnapshotToken == "" {
		t.Fatalf("bounded ranking = %+v", bounded)
	}
	if bounded.Recovery == nil || len(bounded.Recovery.Then) != 2 || bounded.Recovery.Then[0].Type() != "sync_threads" || bounded.Recovery.Then[1].Type() != "rank_contribution_candidates" {
		t.Fatalf("bounded recovery = %+v", bounded.Recovery)
	}
	if summary := bounded.Repositories[0].Value; summary == nil || summary.Considered != 5 || summary.Returned != 5 || summary.Truncated {
		t.Fatalf("repository summary = %+v", summary)
	}
	for index, candidate := range bounded.Candidates {
		if candidate.Rank != index+1 {
			t.Fatalf("candidate rank = %+v", candidate)
		}
	}
	if bounded.Provenance.QueryDigestSHA256 == "" || bounded.Provenance.Complete() {
		t.Fatalf("provenance = %+v", bounded.Provenance)
	}
}

func TestRankContributionCandidatesUsesOneEvaluationTime(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	clockCalls := 0
	svc.SetClock(func() time.Time {
		clockCalls++
		return now.Add(time.Duration(clockCalls) * time.Hour)
	})
	for _, name := range []string{"one", "two"} {
		seedCandidateRankingRepository(ctx, t, svc, name, 1, now)
	}
	out, err := (&MCPReader{svc}).RankContributionCandidates(ctx, mcpcontract.RankContributionCandidatesInput{
		Repositories: []mcpcontract.RepositoryRef{{Owner: "acme", Repo: "one"}, {Owner: "acme", Repo: "two"}}, Limit: 2, MaxResultsPerRepository: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if clockCalls != 1 || out.GeneratedAt != now.Add(time.Hour).Format(time.RFC3339) || len(out.Candidates) != 2 {
		t.Fatalf("evaluation = calls:%d output:%+v", clockCalls, out)
	}
}

func TestCandidateRankingRecoveryUsesFreshSnapshot(t *testing.T) {
	in := mcpcontract.RankContributionCandidatesInput{SnapshotToken: "snapshot:stale"}
	if next := freshCandidateRankingInput(in); next.SnapshotToken != "" {
		t.Fatalf("ranking recovery retained stale snapshot: %+v", next)
	}
}

func seedCandidateRankingRepository(ctx context.Context, t *testing.T, svc *Service, name string, candidates int, now time.Time) {
	t.Helper()
	repository, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: "acme", Name: name, SourceUpdatedAt: now}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	for number := 1; number <= candidates; number++ {
		if _, err := svc.corpus.UpsertThread(ctx, corpus.Thread{RepositoryID: repository.ID, Kind: domain.IssueKind, Number: number, State: domain.OpenState, Title: "same-score candidate", SourceUpdatedAt: now.Add(-time.Hour)}, `{}`); err != nil {
			t.Fatal(err)
		}
	}
}
