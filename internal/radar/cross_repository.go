package radar

import "sort"

// CrossRepositoryRanking is the deterministic merge of already-computed
// repository reports. Acquisition and repository availability remain owned by
// the application layer.
type CrossRepositoryRanking struct {
	Candidates []Candidate
	Total      int
	Truncated  bool
}

// RankAcrossRepositories performs no I/O and applies one stable ordering to
// candidates produced under the same evaluation time and score version.
func RankAcrossRepositories(reports []*Report, limit int) CrossRepositoryRanking {
	var ranking CrossRepositoryRanking
	for _, report := range reports {
		if report == nil {
			continue
		}
		ranking.Total += report.CandidatePopulation
		ranking.Truncated = ranking.Truncated || report.PopulationCapped || len(report.Candidates) < report.CandidatePopulation
		ranking.Candidates = append(ranking.Candidates, report.Candidates...)
	}
	sort.SliceStable(ranking.Candidates, func(i, j int) bool {
		left, right := ranking.Candidates[i], ranking.Candidates[j]
		if left.Eligibility != right.Eligibility {
			return eligibilitySeverity(left.Eligibility) < eligibilitySeverity(right.Eligibility)
		}
		if left.Score != right.Score {
			return left.Score > right.Score
		}
		return left.Ref < right.Ref
	})
	if len(ranking.Candidates) > limit {
		ranking.Candidates = ranking.Candidates[:limit]
		ranking.Truncated = true
	}
	for index := range ranking.Candidates {
		ranking.Candidates[index].Rank = index + 1
	}
	return ranking
}
