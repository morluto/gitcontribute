package clustering

import (
	"fmt"
	"math"
)

const defaultComparisonBudget ComparisonBudget = 10_000_000

// ComparisonBudget bounds the worst-case exact scores in one clustering run.
type ComparisonBudget uint64

// DefaultComparisonBudget returns the repository's supported scoring budget.
func DefaultComparisonBudget() ComparisonBudget { return defaultComparisonBudget }

// CapacityError reports a population whose worst-case work exceeds the budget.
type CapacityError struct {
	CandidateCount int
	PossiblePairs  uint64
	AllowedPairs   uint64
}

// Error describes the rejected scoring request.
func (e *CapacityError) Error() string {
	return fmt.Sprintf("%d candidates have %d possible pairs; limit is %d", e.CandidateCount, e.PossiblePairs, e.AllowedPairs)
}

// Possible returns the population's possible pair count or a CapacityError
// when worst-case scoring would exceed the budget.
func (b ComparisonBudget) Possible(candidateCount int) (uint64, error) {
	possible := possiblePairs(candidateCount)
	if possible > uint64(b) {
		return possible, &CapacityError{CandidateCount: candidateCount, PossiblePairs: possible, AllowedPairs: uint64(b)}
	}
	return possible, nil
}

// MaxCandidates returns the greatest population whose worst-case work fits the budget.
func (b ComparisonBudget) MaxCandidates() int {
	low, high := uint64(0), uint64(b)+2
	for low < high {
		mid := low + (high-low+1)/2
		if possiblePairsUint64(mid) <= uint64(b) {
			low = mid
		} else {
			high = mid - 1
		}
	}
	if low > uint64(math.MaxInt) {
		return math.MaxInt
	}
	return int(low)
}

func possiblePairs(candidateCount int) uint64 {
	if candidateCount < 2 {
		return 0
	}
	return possiblePairsUint64(uint64(candidateCount))
}

func possiblePairsUint64(candidateCount uint64) uint64 {
	if candidateCount < 2 {
		return 0
	}
	left, right := candidateCount, candidateCount-1
	if left%2 == 0 {
		left /= 2
	} else {
		right /= 2
	}
	if right != 0 && left > math.MaxUint64/right {
		return math.MaxUint64
	}
	return left * right
}
