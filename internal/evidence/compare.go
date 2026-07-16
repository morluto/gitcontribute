package evidence

import "fmt"

// Compare classifies the relationship between a base run and a candidate run.
// Both runs must be present and distinguishable by kind.
func Compare(base, candidate *ValidationRun) (*ComparisonResult, error) {
	if base == nil || candidate == nil {
		return nil, ErrInvalidComparison
	}
	if base.Kind != RunKindBase || candidate.Kind != RunKindCandidate {
		return nil, ErrInvalidComparison
	}

	classification, explanation := classify(base, candidate)
	return &ComparisonResult{
		Base:           base,
		Candidate:      candidate,
		Classification: classification,
		Explanation:    explanation,
	}, nil
}

func classify(base, candidate *ValidationRun) (ComparisonClassification, string) {
	if base.Classification == RunClassificationCancelled ||
		candidate.Classification == RunClassificationCancelled {
		return ComparisonInconclusive, fmt.Sprintf(
			"base=%s candidate=%s; one or both runs were cancelled",
			base.Classification, candidate.Classification,
		)
	}
	if base.Classification == RunClassificationError ||
		candidate.Classification == RunClassificationError {
		return ComparisonInconclusive, fmt.Sprintf(
			"base=%s candidate=%s; execution error prevents comparison",
			base.Classification, candidate.Classification,
		)
	}

	switch base.Classification {
	case RunClassificationFailing:
		switch candidate.Classification {
		case RunClassificationPassing:
			return ComparisonFixed, fmt.Sprintf(
				"base exited %d (failing) and candidate exited %d (passing): the issue is reproduced and fixed",
				base.ExitCode, candidate.ExitCode,
			)
		case RunClassificationFailing:
			return ComparisonNotFixed, fmt.Sprintf(
				"base exited %d and candidate exited %d; both fail",
				base.ExitCode, candidate.ExitCode,
			)
		}
	case RunClassificationPassing:
		switch candidate.Classification {
		case RunClassificationPassing:
			return ComparisonNoDifference, fmt.Sprintf(
				"base exited %d and candidate exited %d; both pass",
				base.ExitCode, candidate.ExitCode,
			)
		case RunClassificationFailing:
			return ComparisonRegression, fmt.Sprintf(
				"base exited %d (passing) and candidate exited %d (failing): regression introduced",
				base.ExitCode, candidate.ExitCode,
			)
		}
	}
	return ComparisonInconclusive, fmt.Sprintf(
		"base=%s (exit %d) candidate=%s (exit %d); comparison inconclusive",
		base.Classification, base.ExitCode, candidate.Classification, candidate.ExitCode,
	)
}
