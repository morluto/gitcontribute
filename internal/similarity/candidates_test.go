package similarity_test

import (
	"context"
	"errors"
	"testing"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/similarity"
)

func TestDuplicateCandidatesKeepEveryQualifyingPair(t *testing.T) {
	rule := similarity.DefaultDuplicateRule()
	repo := domain.RepoRef{Owner: "Owner", Repo: "Repo"}
	threads := []similarity.ThreadText{
		{Ref: similarity.ThreadRef{Repo: repo, Kind: domain.IssueKind, Number: 1}, Title: "login crash"},
		{Ref: similarity.ThreadRef{Repo: repo, Kind: domain.IssueKind, Number: 2}, Title: "login failure"},
		{Ref: similarity.ThreadRef{Repo: repo, Kind: domain.IssueKind, Number: 3}, Title: "different", Body: "duplicate of owner/repo#1"},
		{Ref: similarity.ThreadRef{Repo: repo, Kind: domain.PullRequestKind, Number: 4}, Title: "parser"},
		{Ref: similarity.ThreadRef{Repo: repo, Kind: domain.IssueKind, Number: 5}, Title: "unrelated", Body: "see github.com/owner/repo/pull/4"},
		{Ref: similarity.ThreadRef{Repo: repo, Kind: domain.IssueKind, Number: 6}, Title: "first", Body: "shared body", Labels: []string{"bug"}, Author: "alice"},
		{Ref: similarity.ThreadRef{Repo: repo, Kind: domain.IssueKind, Number: 7}, Title: "second", Body: "shared body", Labels: []string{"bug"}, Author: "alice"},
	}
	prepared := make([]similarity.PreparedDuplicate, len(threads))
	for i, thread := range threads {
		prepared[i] = rule.Prepare(thread)
	}

	indexed := candidatePairSet(t, rule, prepared)
	for i := range prepared {
		for j := i + 1; j < len(prepared); j++ {
			if rule.Score(prepared[i], prepared[j]) < rule.Threshold() {
				continue
			}
			if _, ok := indexed[[2]int{i, j}]; !ok {
				t.Fatalf("qualifying pair (%d, %d) was pruned", i, j)
			}
		}
	}
	if _, ok := indexed[[2]int{5, 6}]; ok {
		t.Fatal("pair with no title overlap or reference was not pruned")
	}
	if got, wantMax := len(indexed), len(prepared)*(len(prepared)-1)/2; got >= wantMax {
		t.Fatalf("candidate index did not prune: got %d of %d possible pairs", got, wantMax)
	}
}

func TestDuplicateCandidatesHonorCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := similarity.DefaultDuplicateRule().VisitCandidatePairs(ctx, []similarity.PreparedDuplicate{{}}, func(int, int) {})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Candidates error = %v, want context.Canceled", err)
	}
}

func FuzzDuplicateCandidatesKeepQualifyingPairs(f *testing.F) {
	f.Add("login crash", "login failure", "", "")
	f.Add("first", "second", "", "duplicate of #1")
	f.Add("parser", "renderer", "shared body", "shared body")
	f.Fuzz(func(t *testing.T, titleA, titleB, bodyA, bodyB string) {
		titleA, titleB = boundedText(titleA), boundedText(titleB)
		bodyA, bodyB = boundedText(bodyA), boundedText(bodyB)
		rule := similarity.DefaultDuplicateRule()
		repo := domain.RepoRef{Owner: "owner", Repo: "repo"}
		prepared := []similarity.PreparedDuplicate{
			rule.Prepare(similarity.ThreadText{
				Ref:   similarity.ThreadRef{Repo: repo, Kind: domain.IssueKind, Number: 1},
				Title: titleA, Body: bodyA,
			}),
			rule.Prepare(similarity.ThreadText{
				Ref:   similarity.ThreadRef{Repo: repo, Kind: domain.IssueKind, Number: 2},
				Title: titleB, Body: bodyB,
			}),
		}
		if rule.Score(prepared[0], prepared[1]) < rule.Threshold() {
			return
		}
		pairs := candidatePairSet(t, rule, prepared)
		if _, ok := pairs[[2]int{0, 1}]; !ok {
			t.Fatalf("qualifying pair was pruned: candidates=%v", pairs)
		}
	})
}

func candidatePairSet(t *testing.T, rule similarity.DuplicateRule, prepared []similarity.PreparedDuplicate) map[[2]int]struct{} {
	t.Helper()
	pairs := make(map[[2]int]struct{})
	err := rule.VisitCandidatePairs(context.Background(), prepared, func(left, right int) {
		pairs[[2]int{left, right}] = struct{}{}
	})
	if err != nil {
		t.Fatal(err)
	}
	return pairs
}

func boundedText(value string) string {
	const limit = 256
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
