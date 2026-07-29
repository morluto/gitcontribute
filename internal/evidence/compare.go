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
	if base.DefinitionID != candidate.DefinitionID {
		return inconclusiveComparison(base, candidate, "base and candidate do not share one validation definition"), nil
	}
	if reason := incompatibleRunIdentity(base, candidate); reason != "" {
		return inconclusiveComparison(base, candidate, reason), nil
	}

	classification, explanation := classify(base, candidate)
	return &ComparisonResult{
		Base:           base,
		Candidate:      candidate,
		Classification: classification,
		Explanation:    explanation,
	}, nil
}

func inconclusiveComparison(base, candidate *ValidationRun, explanation string) *ComparisonResult {
	return &ComparisonResult{Base: base, Candidate: candidate, Classification: ComparisonInconclusive, Explanation: explanation}
}

func incompatibleRunIdentity(base, candidate *ValidationRun) string {
	if base.ExecutionOrigin == "external" || candidate.ExecutionOrigin == "external" {
		if base.ExecutionOrigin != candidate.ExecutionOrigin {
			return "external and locally executed observations cannot establish a confirmatory comparison"
		}
		if base.External == nil || candidate.External == nil {
			return "external receipt provenance is missing"
		}
		if base.External.Incomplete || candidate.External.Incomplete {
			return "one or both external receipts are producer-declared incomplete"
		}
		if base.External.Repository == "" || candidate.External.Repository == "" ||
			base.External.Revision == "" || candidate.External.Revision == "" ||
			base.External.ArtifactSHA256 == "" || candidate.External.ArtifactSHA256 == "" {
			return "external source or artifact identity is incomplete"
		}
		if base.External.Repository != candidate.External.Repository {
			return "external receipts identify different repositories"
		}
	}
	for _, run := range []*ValidationRun{base, candidate} {
		if run.WorkspaceBindingStatus == "stale" || run.WorkspaceBindingStatus == "incompatible" {
			return fmt.Sprintf("%s workspace binding is %s", run.Kind, run.WorkspaceBindingStatus)
		}
	}
	return ""
}

func classify(base, candidate *ValidationRun) (ComparisonClassification, string) {
	baseObserved := base.ObservationStatus == ObservationMatched || base.ObservationStatus == ObservationMismatched || len(base.Observations) > 0
	candidateObserved := candidate.ObservationStatus == ObservationMatched || candidate.ObservationStatus == ObservationMismatched || len(candidate.Observations) > 0
	if base.ObservationStatus == ObservationMismatched || candidate.ObservationStatus == ObservationMismatched ||
		(baseObserved != candidateObserved) {
		return ComparisonInconclusive, fmt.Sprintf(
			"base observation=%s candidate observation=%s; expected validation symptom was not observed",
			base.ObservationStatus, candidate.ObservationStatus,
		)
	}
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
