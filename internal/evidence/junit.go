package evidence

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	// JUnitReportSchemaV1 identifies the persisted shape of a parsed report.
	JUnitReportSchemaV1 = "gitcontribute.junit-report.v1"
	// DefaultJUnitReportMaxBytes bounds the XML document held in memory.
	DefaultJUnitReportMaxBytes int64 = 2 << 20
	// DefaultJUnitTestCaseLimit prevents a valid report from creating an
	// unbounded number of persisted test-case records.
	DefaultJUnitTestCaseLimit = 10000
	maxJUnitParseErrorBytes   = 512
)

var (
	ErrJUnitReportTooLarge   = errors.New("evidence: JUnit report exceeds the input bound")
	ErrJUnitTestCaseLimit    = errors.New("evidence: JUnit report exceeds the test-case bound")
	ErrJUnitInvalidRoot      = errors.New("evidence: JUnit report must contain a testsuite or testsuites root")
	ErrJUnitReportIncomplete = errors.New("evidence: JUnit report is incomplete")
)

// JUnitParseOptions bounds parser resource use. Zero values select the
// package defaults; negative values are invalid.
type JUnitParseOptions struct {
	MaxBytes     int64
	MaxTestCases int
}

// JUnitTestStatus is the normalized outcome of one JUnit testcase element.
type JUnitTestStatus string

const (
	JUnitTestPassed  JUnitTestStatus = "passed"
	JUnitTestFailed  JUnitTestStatus = "failed"
	JUnitTestSkipped JUnitTestStatus = "skipped"
	JUnitTestErrored JUnitTestStatus = "errored"
	JUnitTestUnknown JUnitTestStatus = "unknown"
)

// JUnitTestCase preserves producer-supplied identity and display names. An
// absent id remains absent; the parser does not invent identity from text.
type JUnitTestCase struct {
	ID        string
	Name      string
	Classname string
	Status    JUnitTestStatus
}

// JUnitCounts are derived from observed testcase elements, not from aggregate
// attributes supplied by the producer. Unknown cases are never counted as
// passing.
type JUnitCounts struct {
	Total   int
	Passed  int
	Failed  int
	Skipped int
	Errored int
	Unknown int
}

// JUnitReport is the bounded, structured representation persisted with a
// validation run. Incomplete reports are diagnostic evidence only and must not
// be treated as a complete passing test result.
type JUnitReport struct {
	SchemaVersion string
	Name          string
	Counts        JUnitCounts
	TestCases     []JUnitTestCase
	Incomplete    bool
	ParseError    string
	RawXML        string
	RawSHA256     string
}

type junitSuiteFrame struct {
	Declared         int
	DeclaredFailures int
	DeclaredErrors   int
	DeclaredSkipped  int
	Actual           int
	Failures         int
	Errors           int
	Skipped          int
}

// ParseJUnitReport parses one JUnit XML document and returns partial evidence
// together with an error when the document is malformed, exceeds a bound, or
// declares a different testcase count than it contains. A valid empty report
// is complete.
func ParseJUnitReport(input io.Reader, opts JUnitParseOptions) (JUnitReport, error) { //nolint:gocognit // The bounded token state machine owns XML shape, limits, and partial-report semantics.
	report := JUnitReport{SchemaVersion: JUnitReportSchemaV1}
	maxBytes, maxCases, err := normalizeJUnitParseOptions(opts)
	if err != nil {
		return report, err
	}
	if input == nil {
		return incompleteJUnitReport(report, errors.New("JUnit report input is nil"))
	}

	payload, err := io.ReadAll(io.LimitReader(input, maxBytes+1))
	if err != nil {
		return incompleteJUnitReport(report, fmt.Errorf("read JUnit report: %w", err))
	}
	if int64(len(payload)) > maxBytes {
		return incompleteJUnitReport(report, ErrJUnitReportTooLarge)
	}
	report.RawXML = string(payload)
	digest := sha256.Sum256(payload)
	report.RawSHA256 = hex.EncodeToString(digest[:])

	decoder := xml.NewDecoder(bytes.NewReader(payload))
	rootSeen := false
	rootClosed := false
	elementDepth := 0
	declaredTests := -1
	declaredFailures, declaredErrors, declaredSkipped := -1, -1, -1
	var suites []junitSuiteFrame

	for {
		token, tokenErr := decoder.Token()
		if errors.Is(tokenErr, io.EOF) {
			if !rootSeen {
				return incompleteJUnitReport(report, ErrJUnitInvalidRoot)
			}
			if !rootClosed {
				return incompleteJUnitReport(report, errors.New("JUnit report ended before the root element closed"))
			}
			if declaredTests >= 0 && declaredTests != report.Counts.Total {
				return incompleteJUnitReport(report, fmt.Errorf("JUnit report declares %d tests but contains %d", declaredTests, report.Counts.Total))
			}
			if (declaredFailures >= 0 && declaredFailures != report.Counts.Failed) || (declaredErrors >= 0 && declaredErrors != report.Counts.Errored) || (declaredSkipped >= 0 && declaredSkipped != report.Counts.Skipped) {
				return incompleteJUnitReport(report, errors.New("JUnit aggregate failure, error, or skipped counts do not match testcase outcomes"))
			}
			return report, nil
		}
		if tokenErr != nil {
			return incompleteJUnitReport(report, fmt.Errorf("parse JUnit XML: %w", tokenErr))
		}

		switch value := token.(type) {
		case xml.StartElement:
			if rootClosed {
				return incompleteJUnitReport(report, errors.New("JUnit report contains content after the root element"))
			}
			if !rootSeen {
				if value.Name.Local != "testsuite" && value.Name.Local != "testsuites" {
					return incompleteJUnitReport(report, ErrJUnitInvalidRoot)
				}
				rootSeen = true
				elementDepth = 1
				report.Name = xmlAttribute(value, "name")
				declaredTests, err = xmlIntAttribute(value, "tests")
				if err != nil {
					return incompleteJUnitReport(report, err)
				}
				declaredFailures, err = xmlIntAttribute(value, "failures")
				if err != nil {
					return incompleteJUnitReport(report, err)
				}
				declaredErrors, err = xmlIntAttribute(value, "errors")
				if err != nil {
					return incompleteJUnitReport(report, err)
				}
				declaredSkipped, err = xmlIntAttribute(value, "skipped")
				if err != nil {
					return incompleteJUnitReport(report, err)
				}
				if value.Name.Local == "testsuite" {
					suites = append(suites, junitSuiteFrame{Declared: declaredTests, DeclaredFailures: declaredFailures, DeclaredErrors: declaredErrors, DeclaredSkipped: declaredSkipped})
				}
				continue
			}
			if value.Name.Local == "testsuite" {
				declared, suiteErr := xmlIntAttribute(value, "tests")
				if suiteErr != nil {
					return incompleteJUnitReport(report, suiteErr)
				}
				failures, suiteErr := xmlIntAttribute(value, "failures")
				if suiteErr != nil {
					return incompleteJUnitReport(report, suiteErr)
				}
				errorsCount, suiteErr := xmlIntAttribute(value, "errors")
				if suiteErr != nil {
					return incompleteJUnitReport(report, suiteErr)
				}
				skipped, suiteErr := xmlIntAttribute(value, "skipped")
				if suiteErr != nil {
					return incompleteJUnitReport(report, suiteErr)
				}
				suites = append(suites, junitSuiteFrame{Declared: declared, DeclaredFailures: failures, DeclaredErrors: errorsCount, DeclaredSkipped: skipped})
				elementDepth++
				continue
			}
			if value.Name.Local == "testcase" {
				if len(report.TestCases) >= maxCases {
					return incompleteJUnitReport(report, ErrJUnitTestCaseLimit)
				}
				caseValue, caseErr := decodeJUnitTestCase(decoder, value)
				report.TestCases = append(report.TestCases, caseValue)
				addJUnitTestCase(&report.Counts, caseValue.Status)
				if len(suites) > 0 {
					suites[len(suites)-1].Actual++
					switch caseValue.Status {
					case JUnitTestFailed:
						suites[len(suites)-1].Failures++
					case JUnitTestErrored:
						suites[len(suites)-1].Errors++
					case JUnitTestSkipped:
						suites[len(suites)-1].Skipped++
					}
				}
				if caseErr != nil {
					return incompleteJUnitReport(report, fmt.Errorf("parse JUnit testcase: %w", caseErr))
				}
				continue
			}
			elementDepth++
		case xml.EndElement:
			elementDepth--
			if value.Name.Local == "testsuite" && len(suites) > 0 {
				frame := suites[len(suites)-1]
				suites = suites[:len(suites)-1]
				if (frame.Declared >= 0 && frame.Declared != frame.Actual) || (frame.DeclaredFailures >= 0 && frame.DeclaredFailures != frame.Failures) || (frame.DeclaredErrors >= 0 && frame.DeclaredErrors != frame.Errors) || (frame.DeclaredSkipped >= 0 && frame.DeclaredSkipped != frame.Skipped) {
					return incompleteJUnitReport(report, fmt.Errorf("JUnit testsuite declares %d tests but contains %d", frame.Declared, frame.Actual))
				}
			}
			if rootSeen && elementDepth == 0 {
				rootClosed = true
			}
		case xml.CharData:
			if (!rootSeen || rootClosed) && strings.TrimSpace(string(value)) != "" {
				return incompleteJUnitReport(report, errors.New("JUnit report contains non-whitespace data outside the root element"))
			}
		}
	}
}

func normalizeJUnitParseOptions(opts JUnitParseOptions) (int64, int, error) {
	maxBytes := opts.MaxBytes
	if maxBytes == 0 {
		maxBytes = DefaultJUnitReportMaxBytes
	}
	if maxBytes < 0 {
		return 0, 0, errors.New("JUnit report byte bound must not be negative")
	}
	maxCases := opts.MaxTestCases
	if maxCases == 0 {
		maxCases = DefaultJUnitTestCaseLimit
	}
	if maxCases < 0 {
		return 0, 0, errors.New("JUnit report test-case bound must not be negative")
	}
	return maxBytes, maxCases, nil
}

func decodeJUnitTestCase(decoder *xml.Decoder, start xml.StartElement) (JUnitTestCase, error) {
	baseStatus := junitAttributeStatus(xmlAttribute(start, "status"))
	caseValue := JUnitTestCase{
		ID:        xmlAttribute(start, "id"),
		Name:      xmlAttribute(start, "name"),
		Classname: xmlAttribute(start, "classname"),
		Status:    baseStatus,
	}
	depth := 1
	hasFailure, hasError, hasSkipped := false, false, false
	for depth > 0 {
		token, err := decoder.Token()
		if err != nil {
			caseValue.Status = JUnitTestUnknown
			return caseValue, err
		}
		switch value := token.(type) {
		case xml.StartElement:
			depth++
			switch value.Name.Local {
			case "failure":
				hasFailure = true
			case "error":
				hasError = true
			case "skipped":
				hasSkipped = true
			}
		case xml.EndElement:
			depth--
		}
	}
	caseValue.Status = junitTestStatus(hasFailure, hasError, hasSkipped, baseStatus)
	return caseValue, nil
}

func junitAttributeStatus(value string) JUnitTestStatus {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "passed", "pass", "success":
		return JUnitTestPassed
	case "failed", "failure":
		return JUnitTestFailed
	case "error", "errored":
		return JUnitTestErrored
	case "skipped", "ignored", "notrun", "not-run":
		return JUnitTestSkipped
	default:
		return JUnitTestUnknown
	}
}

func junitTestStatus(failure, testError, skipped bool, base JUnitTestStatus) JUnitTestStatus {
	switch {
	case testError:
		return JUnitTestErrored
	case failure:
		return JUnitTestFailed
	case skipped:
		return JUnitTestSkipped
	default:
		return base
	}
}

func addJUnitTestCase(counts *JUnitCounts, status JUnitTestStatus) {
	counts.Total++
	switch status {
	case JUnitTestPassed:
		counts.Passed++
	case JUnitTestFailed:
		counts.Failed++
	case JUnitTestSkipped:
		counts.Skipped++
	case JUnitTestErrored:
		counts.Errored++
	default:
		counts.Unknown++
	}
}

func xmlIntAttribute(element xml.StartElement, name string) (int, error) {
	value, present := xmlAttributeValue(element, name)
	if !present || value == "" {
		return -1, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return -1, fmt.Errorf("JUnit %s attribute %q is not a non-negative integer", name, value)
	}
	return parsed, nil
}

func xmlAttribute(element xml.StartElement, name string) string {
	value, _ := xmlAttributeValue(element, name)
	return value
}

func xmlAttributeValue(element xml.StartElement, name string) (string, bool) {
	for _, attr := range element.Attr {
		if attr.Name.Local == name {
			return strings.TrimSpace(attr.Value), true
		}
	}
	return "", false
}

func incompleteJUnitReport(report JUnitReport, err error) (JUnitReport, error) {
	report.Incomplete = true
	report.ParseError = truncateJUnitParseError(err.Error())
	return report, errors.Join(ErrJUnitReportIncomplete, err)
}

func truncateJUnitParseError(value string) string {
	if len(value) <= maxJUnitParseErrorBytes {
		return value
	}
	return value[:maxJUnitParseErrorBytes]
}
