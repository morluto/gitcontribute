package research

import (
	"testing"
	"time"
)

func TestObservedFacetCoverageRejectsInvalidObservations(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	for name, testCase := range map[string]struct {
		facet string
		asOf  time.Time
		count int
	}{
		"missing facet":  {asOf: now},
		"missing as-of":  {facet: "issue_comments"},
		"negative count": {facet: "issue_comments", asOf: now, count: -1},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ObservedFacetCoverage(testCase.facet, true, testCase.asOf, testCase.count, SourceRef{Source: "github:rest"}); err == nil {
				t.Fatal("expected invalid facet observation to be rejected")
			}
		})
	}
	if _, err := ObservedFacetCoverage("issue_comments", true, now, 1, SourceRef{Source: "github:rest", AsOf: now.Add(-time.Second)}); err == nil {
		t.Fatal("coverage accepted provenance for a different observation time")
	}
}

func TestHealthEvidenceSeparatesUnavailableAndObservedMetrics(t *testing.T) {
	missing := MissingHealthEvidence("not computed")
	if _, available := missing.Metrics(); available || missing.UnknownReason() != "not computed" || len(missing.Sources()) != 0 {
		t.Fatalf("unexpected missing health evidence: %+v", missing)
	}

	rate := 0.5
	sources := []SourceRef{{Source: "local:health"}}
	observed, err := ObservedHealthEvidence(HealthMetrics{ExternalPRMergeRate: &rate, ExternalPRSampleSize: 2}, sources, "bounded")
	if err != nil {
		t.Fatal(err)
	}
	rate = 1
	sources[0].Source = "mutated"
	metrics, available := observed.Metrics()
	if !available || *metrics.ExternalPRMergeRate != 0.5 || observed.Sources()[0].Source != "local:health" {
		t.Fatalf("observed health evidence did not preserve ownership: metrics=%+v sources=%+v", metrics, observed.Sources())
	}
	if _, err := ObservedHealthEvidence(HealthMetrics{}, nil, ""); err == nil {
		t.Fatal("observed health evidence without provenance was accepted")
	}
}

func TestMissingFacetCoverageCannotBeTruncated(t *testing.T) {
	missing, err := MissingFacetCoverage("issue_comments")
	if err != nil {
		t.Fatalf("construct missing coverage: %v", err)
	}
	if missing.WithTruncated(true).Truncated() {
		t.Fatal("missing facet reported truncation")
	}
}

func TestTruncatedFacetCoverageCannotRemainComplete(t *testing.T) {
	now := time.Unix(1, 0).UTC()
	coverage, err := ObservedFacetCoverage("issue_comments", true, now, 10, SourceRef{Source: "github:rest", AsOf: now})
	if err != nil {
		t.Fatal(err)
	}
	coverage = coverage.WithTruncated(true)
	if !coverage.Present() || !coverage.Truncated() || coverage.Complete() {
		t.Fatalf("truncated coverage = present:%t complete:%t truncated:%t", coverage.Present(), coverage.Complete(), coverage.Truncated())
	}
}

func TestCodeEvidenceSeparatesMissingAndObservedSnapshots(t *testing.T) {
	queries := []string{"parser"}
	missing := MissingCodeEvidence(queries)
	queries[0] = "mutated"
	if missing.Present() || missing.CommitSHA() != "" || len(missing.Hits()) != 0 || missing.Queries()[0] != "parser" {
		t.Fatalf("unexpected missing evidence: present=%t commit=%q queries=%v hits=%v", missing.Present(), missing.CommitSHA(), missing.Queries(), missing.Hits())
	}

	source := SourceRef{Source: "local:code-index", CommitSHA: "abc123"}
	hits := []CodeHit{{
		Path: "parser.go", CommitSHA: "abc123", MatchedTerm: "parser",
		Source: SourceRef{Source: "local:code-index", CommitSHA: "abc123"},
	}}
	observed, err := ObservedCodeEvidence("abc123", []string{"parser"}, hits, source, true)
	if err != nil {
		t.Fatalf("construct observed evidence: %v", err)
	}
	hits[0].Path = "mutated.go"
	if !observed.Present() || observed.CommitSHA() != "abc123" || observed.Hits()[0].Path != "parser.go" || !observed.Truncated() {
		t.Fatalf("unexpected observed evidence: present=%t commit=%q hits=%v truncated=%t", observed.Present(), observed.CommitSHA(), observed.Hits(), observed.Truncated())
	}
}

func TestObservedCodeEvidenceRejectsContradictorySnapshotIdentity(t *testing.T) {
	source := SourceRef{Source: "local:code-index", CommitSHA: "other"}
	if _, err := ObservedCodeEvidence("abc123", nil, nil, source, false); err == nil {
		t.Fatal("expected contradictory source commit to be rejected")
	}
	hit := CodeHit{
		Path: "parser.go", CommitSHA: "other", MatchedTerm: "parser",
		Source: SourceRef{Source: "local:code-index", CommitSHA: "other"},
	}
	if _, err := ObservedCodeEvidence("abc123", nil, []CodeHit{hit}, SourceRef{Source: "local:code-index", CommitSHA: "abc123"}, false); err == nil {
		t.Fatal("expected contradictory hit commit to be rejected")
	}
}
