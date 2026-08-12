package radar

import "testing"

func TestRankAcrossRepositoriesUsesEligibilityThenScoreThenRef(t *testing.T) {
	reports := []*Report{
		{CandidatePopulation: 2, Candidates: []Candidate{{Ref: "issue:b/z#2", Eligibility: EligibilityNeedsDiagnosis, Score: 99}, {Ref: "issue:a/z#1", Eligibility: EligibilityReadyToCode, Score: 50}}},
		{CandidatePopulation: 1, Candidates: []Candidate{{Ref: "issue:c/z#3", Eligibility: EligibilityReadyToCode, Score: 80}}},
	}
	ranking := RankAcrossRepositories(reports, 2)
	if !ranking.Truncated || ranking.Total != 3 || len(ranking.Candidates) != 2 || ranking.Candidates[0].Ref != "issue:c/z#3" || ranking.Candidates[1].Ref != "issue:a/z#1" {
		t.Fatalf("ranking = %+v", ranking)
	}
	if ranking.Candidates[0].Rank != 1 || ranking.Candidates[1].Rank != 2 {
		t.Fatalf("ranks = %+v", ranking.Candidates)
	}
}
