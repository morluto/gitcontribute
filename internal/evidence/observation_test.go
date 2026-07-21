package evidence

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunValidationEvaluatesObservationContract(t *testing.T) {
	repo := newFakeRepo()
	runner := &fakeRunner{result: &RunResult{
		ExitCode:       1,
		Stdout:         "generated !buffer<3> but expected !buffer<4>\n",
		Stderr:         "check failed\n",
		Classification: RunClassificationFailing,
	}}
	svc := NewService(repo, runner)
	def := &ValidationDefinition{
		ID:         "def",
		Command:    []string{"lit", "pipeline.mlir"},
		WorkingDir: "/tmp",
		Observation: &ObservationContract{
			Intent: "the base pipeline buffer has three slots",
			Base: []ExpectedObservation{{
				Name: "undersized buffer", Source: ObservationStdout,
				Matcher: ObservationRegexp, Pattern: `!buffer<3>`, Occurrence: ObservationPresent,
			}},
			Candidate: []ExpectedObservation{{
				Name: "corrected buffer", Source: ObservationStdout,
				Matcher: ObservationRegexp, Pattern: `!buffer<4>`, Occurrence: ObservationPresent,
			}},
		},
	}
	if err := svc.DefineValidation(context.Background(), def); err != nil {
		t.Fatalf("define: %v", err)
	}
	run, err := svc.RunValidation(context.Background(), def.ID, RunKindBase)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if run.ObservationStatus != ObservationMatched {
		t.Fatalf("observation status = %q, want matched", run.ObservationStatus)
	}
	if len(run.Observations) != 1 || run.Observations[0].Excerpt == "" {
		t.Fatalf("observations = %#v, want bounded matched excerpt", run.Observations)
	}
}

func TestObservationContractSupportsExpectedAbsence(t *testing.T) {
	contract := &ObservationContract{
		Intent: "candidate removes the undersized buffer",
		Candidate: []ExpectedObservation{{
			Name: "undersized buffer absent", Source: ObservationStdout,
			Matcher: ObservationExact, Pattern: "!buffer<3>", Occurrence: ObservationAbsent,
		}},
	}
	status, results := evaluateObservations(context.Background(), contract, RunKindCandidate, "", "generated !buffer<4>\n", "", 1024)
	if status != ObservationMatched || len(results) != 1 || results[0].Status != ObservationMatched {
		t.Fatalf("status=%q results=%#v, want matched absence", status, results)
	}
}

func TestObservationContractMatchesBoundedArtifact(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pipeline.mlir"), []byte("!buffer<4>\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	contract := &ObservationContract{
		Intent: "candidate generates a four-slot buffer",
		Candidate: []ExpectedObservation{{
			Name: "generated buffer", Source: ObservationArtifact, Path: "pipeline.mlir",
			Matcher: ObservationExact, Pattern: "!buffer<4>", Occurrence: ObservationPresent,
		}},
	}
	status, results := evaluateObservations(context.Background(), contract, RunKindCandidate, dir, "", "", 1024)
	if status != ObservationMatched || len(results) != 1 || results[0].Excerpt == "" {
		t.Fatalf("status=%q results=%#v, want matched artifact", status, results)
	}
}

func TestObservationArtifactRejectsWorkspaceEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	contract := &ObservationContract{
		Intent: "artifact remains inside workspace",
		Base: []ExpectedObservation{{
			Name: "escaped", Source: ObservationArtifact, Path: "escape/secret",
			Matcher: ObservationExact, Pattern: "secret", Occurrence: ObservationPresent,
		}},
	}
	status, results := evaluateObservations(context.Background(), contract, RunKindBase, root, "", "", 1024)
	if status != ObservationMismatched || len(results) != 1 || !strings.Contains(results[0].Error, "escapes") {
		t.Fatalf("status=%q results=%#v, want escape rejection", status, results)
	}
}

func TestDefineValidationRejectsInvalidObservationRegexp(t *testing.T) {
	svc := NewService(newFakeRepo(), &fakeRunner{})
	err := svc.DefineValidation(context.Background(), &ValidationDefinition{
		Command: []string{"test"}, WorkingDir: "/tmp",
		Observation: &ObservationContract{
			Intent: "observe output",
			Base: []ExpectedObservation{{
				Name: "invalid", Source: ObservationStderr,
				Matcher: ObservationRegexp, Pattern: "[", Occurrence: ObservationPresent,
			}},
			Candidate: []ExpectedObservation{{
				Name: "valid", Source: ObservationStdout,
				Matcher: ObservationExact, Pattern: "ok", Occurrence: ObservationPresent,
			}},
		},
	})
	if !errors.Is(err, ErrInvalidObservation) {
		t.Fatalf("error = %v, want ErrInvalidObservation", err)
	}
}

func TestEvaluateObservationsReportsCorruptPersistedRegexp(t *testing.T) {
	contract := &ObservationContract{
		Intent: "persisted contract may be corrupt",
		Base: []ExpectedObservation{{
			Name: "invalid", Source: ObservationStdout,
			Matcher: ObservationRegexp, Pattern: "[", Occurrence: ObservationPresent,
		}},
	}
	status, results := evaluateObservations(context.Background(), contract, RunKindBase, "", "output", "", 1024)
	if status != ObservationMismatched || len(results) != 1 || !strings.Contains(results[0].Error, "compile observation regexp") {
		t.Fatalf("status=%q results=%#v, want persisted-regexp error", status, results)
	}
}

func TestDefineValidationRequiresBaseAndCandidateObservations(t *testing.T) {
	svc := NewService(newFakeRepo(), &fakeRunner{})
	err := svc.DefineValidation(context.Background(), &ValidationDefinition{
		Command: []string{"test"}, WorkingDir: "/tmp",
		Observation: &ObservationContract{
			Intent: "observe the intended behavior on both runs",
			Base: []ExpectedObservation{{
				Name: "base symptom", Source: ObservationStderr,
				Matcher: ObservationExact, Pattern: "failure", Occurrence: ObservationPresent,
			}},
		},
	})
	if !errors.Is(err, ErrInvalidObservation) {
		t.Fatalf("error = %v, want ErrInvalidObservation", err)
	}
}

func TestCompareValidationObservationMismatchIsInconclusive(t *testing.T) {
	base := &ValidationRun{
		Kind: RunKindBase, ExitCode: 1, Classification: RunClassificationFailing,
		ObservationStatus: ObservationMismatched,
	}
	candidate := &ValidationRun{
		Kind: RunKindCandidate, ExitCode: 0, Classification: RunClassificationPassing,
		ObservationStatus: ObservationMatched,
	}
	result, err := Compare(base, candidate)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if result.Classification != ComparisonInconclusive {
		t.Fatalf("classification = %q, want inconclusive", result.Classification)
	}
}

func TestCompareValidationPartialObservationIsInconclusive(t *testing.T) {
	base := &ValidationRun{
		Kind: RunKindBase, ExitCode: 1, Classification: RunClassificationFailing,
		ObservationStatus: ObservationMatched,
		Observations:      []ObservationResult{{Status: ObservationMatched}},
	}
	candidate := &ValidationRun{
		Kind: RunKindCandidate, ExitCode: 0, Classification: RunClassificationPassing,
		ObservationStatus: ObservationNotEvaluated,
	}
	result, err := Compare(base, candidate)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if result.Classification != ComparisonInconclusive {
		t.Fatalf("classification = %q, want inconclusive", result.Classification)
	}
}
