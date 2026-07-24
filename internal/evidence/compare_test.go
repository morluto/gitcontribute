package evidence

import (
	"testing"
)

func TestCompare(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		base       *ValidationRun
		candidate  *ValidationRun
		want       ComparisonClassification
		wantInExpl string
	}{
		{
			name:      "fixed when base fails and candidate passes",
			base:      &ValidationRun{Kind: RunKindBase, ExitCode: 1, Classification: RunClassificationFailing},
			candidate: &ValidationRun{Kind: RunKindCandidate, ExitCode: 0, Classification: RunClassificationPassing},
			want:      ComparisonFixed,
		},
		{
			name:      "not fixed when both fail",
			base:      &ValidationRun{Kind: RunKindBase, ExitCode: 1, Classification: RunClassificationFailing},
			candidate: &ValidationRun{Kind: RunKindCandidate, ExitCode: 2, Classification: RunClassificationFailing},
			want:      ComparisonNotFixed,
		},

		{
			name:      "no difference when both pass",
			base:      &ValidationRun{Kind: RunKindBase, ExitCode: 0, Classification: RunClassificationPassing},
			candidate: &ValidationRun{Kind: RunKindCandidate, ExitCode: 0, Classification: RunClassificationPassing},
			want:      ComparisonNoDifference,
		},
		{
			name:      "regression when base passes and candidate fails",
			base:      &ValidationRun{Kind: RunKindBase, ExitCode: 0, Classification: RunClassificationPassing},
			candidate: &ValidationRun{Kind: RunKindCandidate, ExitCode: 1, Classification: RunClassificationFailing},
			want:      ComparisonRegression,
		},
		{
			name:      "inconclusive when base cancelled",
			base:      &ValidationRun{Kind: RunKindBase, ExitCode: -1, Classification: RunClassificationCancelled},
			candidate: &ValidationRun{Kind: RunKindCandidate, ExitCode: 0, Classification: RunClassificationPassing},
			want:      ComparisonInconclusive,
		},
		{
			name:      "inconclusive when candidate errors",
			base:      &ValidationRun{Kind: RunKindBase, ExitCode: 0, Classification: RunClassificationPassing},
			candidate: &ValidationRun{Kind: RunKindCandidate, ExitCode: -1, Classification: RunClassificationError},
			want:      ComparisonInconclusive,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Compare(tc.base, tc.candidate)
			if err != nil {
				t.Fatalf("Compare returned error: %v", err)
			}
			if got.Classification != tc.want {
				t.Fatalf("classification: got %q, want %q", got.Classification, tc.want)
			}
		})
	}
}

func TestCompareInvalidKind(t *testing.T) {
	t.Parallel()
	base := &ValidationRun{Kind: RunKindCandidate, ExitCode: 0, Classification: RunClassificationPassing}
	candidate := &ValidationRun{Kind: RunKindCandidate, ExitCode: 0, Classification: RunClassificationPassing}
	if _, err := Compare(base, candidate); err == nil {
		t.Fatal("expected error for non-base base run")
	}
}
