package mcpcontract

import "testing"

func TestFixPatternReportValidateRejectsContradictoryCoverage(t *testing.T) {
	valid := FixPatternReport{Status: FixPatternReportComplete, Complete: true}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid report: %v", err)
	}

	tests := map[string]func(*FixPatternReport){
		"status": func(report *FixPatternReport) {
			report.Status = FixPatternReportStatus("unknown")
		},
		"complete flag": func(report *FixPatternReport) {
			report.Complete = false
		},
		"truncated flag": func(report *FixPatternReport) {
			report.Truncated = true
		},
		"unknown flag": func(report *FixPatternReport) {
			report.UnknownCoverage = true
		},
		"failure in complete report": func(report *FixPatternReport) {
			report.Failures = []FixPatternHydrationFailure{{Reason: "unavailable"}}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			report := valid
			mutate(&report)
			if err := report.Validate(); err == nil {
				t.Fatal("contradictory fix-pattern report was accepted")
			}
		})
	}
}

func TestFixPatternReportValidateAcceptsConsistentPartialCoverage(t *testing.T) {
	report := FixPatternReport{
		Status: FixPatternReportPartial,
		Coverage: FixPatternCoverage{
			CandidateTruncated: true,
			UnknownAfter:       1,
		},
		Truncated:       true,
		UnknownCoverage: true,
		Failures:        []FixPatternHydrationFailure{{Reason: "rate_limited"}},
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("consistent partial report: %v", err)
	}
}
