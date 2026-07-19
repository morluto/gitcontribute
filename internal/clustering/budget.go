package clustering

import (
	"fmt"
	"math"
)

const defaultExactPairBudget ExactPairBudget = 10_000_000

// ExactPairBudget bounds the number of exact candidate comparisons in one clustering run.
type ExactPairBudget uint64

// DefaultExactPairBudget returns the repository's supported exact-work budget.
func DefaultExactPairBudget() ExactPairBudget { return defaultExactPairBudget }

// CapacityError reports candidate work that cannot execute within an exact pair budget.
type CapacityError struct {
	CandidateCount int
	RequiredPairs  uint64
	AllowedPairs   uint64
}

// Error describes the rejected exact-work request.
func (e *CapacityError) Error() string {
	return fmt.Sprintf("%d candidates require %d exact pairs; limit is %d", e.CandidateCount, e.RequiredPairs, e.AllowedPairs)
}

// Required returns the exact pair count or a CapacityError when it exceeds the budget.
func (b ExactPairBudget) Required(candidateCount int) (uint64, error) {
	required := requiredPairs(candidateCount)
	if required > uint64(b) {
		return required, &CapacityError{CandidateCount: candidateCount, RequiredPairs: required, AllowedPairs: uint64(b)}
	}
	return required, nil
}

// MaxCandidates returns the greatest population whose all-pairs work fits the budget.
func (b ExactPairBudget) MaxCandidates() int {
	low, high := uint64(0), uint64(b)+2
	for low < high {
		mid := low + (high-low+1)/2
		if requiredPairsUint64(mid) <= uint64(b) {
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

func requiredPairs(candidateCount int) uint64 {
	if candidateCount < 2 {
		return 0
	}
	return requiredPairsUint64(uint64(candidateCount))
}

func requiredPairsUint64(candidateCount uint64) uint64 {
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
