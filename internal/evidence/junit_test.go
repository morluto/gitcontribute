package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestParseJUnitReportCountsCasesAndPreservesIdentity(t *testing.T) {
	report, err := ParseJUnitReport(strings.NewReader(`
<testsuites tests="4" failures="1" errors="1" skipped="1">
  <testsuite name="unit">
    <testcase id="pass-1" name="passes" classname="pkg.Test"/>
    <testcase id="fail-1" name="fails" classname="pkg.Test"><failure message="nope"/></testcase>
    <testcase id="skip-1" name="skips"><skipped/></testcase>
  </testsuite>
  <testsuite name="integration">
    <testcase id="error-1" name="errors"><error type="panic"/></testcase>
  </testsuite>
</testsuites>`), JUnitParseOptions{})
	if err != nil {
		t.Fatalf("ParseJUnitReport: %v", err)
	}
	if report.Incomplete || report.ParseError != "" {
		t.Fatalf("report completeness = %+v", report)
	}
	if report.Counts != (JUnitCounts{Total: 4, Passed: 1, Failed: 1, Skipped: 1, Errored: 1}) {
		t.Fatalf("counts = %+v", report.Counts)
	}
	if report.TestCases[1].ID != "fail-1" || report.TestCases[1].Name != "fails" || report.TestCases[1].Status != JUnitTestFailed {
		t.Fatalf("failed case = %+v", report.TestCases[1])
	}
	if report.TestCases[3].Status != JUnitTestErrored {
		t.Fatalf("errored case = %+v", report.TestCases[3])
	}
}

func TestParseJUnitReportMarksMalformedInputIncompleteWithPartialCounts(t *testing.T) {
	report, err := ParseJUnitReport(strings.NewReader(`<testsuite tests="2"><testcase id="one" name="one"/><testcase id="two"`), JUnitParseOptions{})
	if err == nil {
		t.Fatal("malformed report unexpectedly parsed")
	}
	if !report.Incomplete || report.ParseError == "" {
		t.Fatalf("incomplete report = %+v", report)
	}
	if report.Counts.Total != 1 || report.Counts.Passed != 1 {
		t.Fatalf("partial counts = %+v", report.Counts)
	}
}

func TestParseJUnitReportBoundsBytesAndCases(t *testing.T) {
	_, err := ParseJUnitReport(strings.NewReader(`<testsuite><testcase/></testsuite>`), JUnitParseOptions{MaxBytes: 8})
	if !errors.Is(err, ErrJUnitReportTooLarge) {
		t.Fatalf("size error = %v, want ErrJUnitReportTooLarge", err)
	}

	report, err := ParseJUnitReport(strings.NewReader(`<testsuite><testcase id="one"/><testcase id="two"/></testsuite>`), JUnitParseOptions{MaxTestCases: 1})
	if !errors.Is(err, ErrJUnitTestCaseLimit) {
		t.Fatalf("case error = %v, want ErrJUnitTestCaseLimit", err)
	}
	if !report.Incomplete || report.Counts.Total != 1 {
		t.Fatalf("bounded report = %+v", report)
	}
}

func TestParseJUnitReportRejectsTrailingNonXMLData(t *testing.T) {
	report, err := ParseJUnitReport(strings.NewReader(`<testsuite><testcase id="one"/></testsuite>garbage`), JUnitParseOptions{})
	if !errors.Is(err, ErrJUnitReportIncomplete) || !report.Incomplete {
		t.Fatalf("trailing data result = %+v, error = %v", report, err)
	}
}

func TestParseJUnitReportValidatesNestedSuiteCounts(t *testing.T) {
	report, err := ParseJUnitReport(strings.NewReader(`<testsuites><testsuite name="unit" tests="2"><testcase id="one"/></testsuite></testsuites>`), JUnitParseOptions{})
	if !errors.Is(err, ErrJUnitReportIncomplete) || !report.Incomplete {
		t.Fatalf("nested count result = %+v, error = %v", report, err)
	}
}

func TestAttachJUnitReportPersistsStructuredReportAndParseState(t *testing.T) {
	repo := newFakeRepo()
	repo.runs["run-1"] = &ValidationRun{ID: "run-1", Kind: RunKindCandidate}
	service := NewService(repo, nil)

	run, err := service.AttachJUnitReport(context.Background(), "run-1", strings.NewReader(`<testsuite tests="1"><testcase id="one" name="one"/></testsuite>`), JUnitParseOptions{})
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	if run.JUnitReport == nil || run.JUnitReport.Counts.Passed != 1 {
		t.Fatalf("stored report = %+v", run.JUnitReport)
	}

	payload, err := json.Marshal(run)
	if err != nil {
		t.Fatalf("marshal stored run: %v", err)
	}
	var roundTrip ValidationRun
	if err := json.Unmarshal(payload, &roundTrip); err != nil {
		t.Fatalf("unmarshal stored run: %v", err)
	}
	if roundTrip.JUnitReport == nil || roundTrip.JUnitReport.TestCases[0].ID != "one" {
		t.Fatalf("round-tripped report = %+v", roundTrip.JUnitReport)
	}
}

func TestAttachJUnitReportPersistsIncompleteReportOnParseError(t *testing.T) {
	repo := newFakeRepo()
	repo.runs["run-1"] = &ValidationRun{ID: "run-1", Kind: RunKindCandidate}
	service := NewService(repo, nil)

	run, err := service.AttachJUnitReport(context.Background(), "run-1", strings.NewReader(`<testsuite tests="2"><testcase id="one"/>`), JUnitParseOptions{})
	if !errors.Is(err, ErrJUnitReportIncomplete) {
		t.Fatalf("attach error = %v, want ErrJUnitReportIncomplete", err)
	}
	if run == nil || run.JUnitReport == nil || !run.JUnitReport.Incomplete {
		t.Fatalf("attached incomplete report = %+v", run)
	}
	if stored := repo.runs["run-1"].JUnitReport; stored == nil || !stored.Incomplete {
		t.Fatalf("persisted incomplete report = %+v", stored)
	}
}
