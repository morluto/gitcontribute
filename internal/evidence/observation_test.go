package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunValidationEvaluatesObservationContract(t *testing.T) {
	t.Parallel()
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
		Observation: mustObservationContract(t, ObservationContractSpec{
			Intent: "the base pipeline buffer has three slots",
			Base: []ExpectedObservationSpec{{
				Name: "undersized buffer", Source: ObservationStdout,
				Matcher: ObservationRegexp, Pattern: `!buffer<3>`, Occurrence: ObservationPresent,
			}},
			Candidate: []ExpectedObservationSpec{{
				Name: "corrected buffer", Source: ObservationStdout,
				Matcher: ObservationRegexp, Pattern: `!buffer<4>`, Occurrence: ObservationPresent,
			}},
		}),
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
	t.Parallel()
	contract := mustObservationContract(t, ObservationContractSpec{
		Intent: "candidate removes the undersized buffer",
		Base: []ExpectedObservationSpec{{
			Name: "undersized buffer present", Source: ObservationStdout,
			Matcher: ObservationExact, Pattern: "!buffer<3>", Occurrence: ObservationPresent,
		}},
		Candidate: []ExpectedObservationSpec{{
			Name: "undersized buffer absent", Source: ObservationStdout,
			Matcher: ObservationExact, Pattern: "!buffer<3>", Occurrence: ObservationAbsent,
		}},
	})
	status, results := evaluateObservations(context.Background(), contract, RunKindCandidate, "", "generated !buffer<4>\n", "", 1024)
	if status != ObservationMatched || len(results) != 1 || results[0].Status != ObservationMatched {
		t.Fatalf("status=%q results=%#v, want matched absence", status, results)
	}
}

func TestObservationContractMatchesBoundedArtifact(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pipeline.mlir"), []byte("!buffer<4>\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	contract := mustObservationContract(t, ObservationContractSpec{
		Intent: "candidate generates a four-slot buffer",
		Base: []ExpectedObservationSpec{{
			Name: "generated buffer", Source: ObservationArtifact, Path: "pipeline.mlir",
			Matcher: ObservationExact, Pattern: "!buffer<3>", Occurrence: ObservationPresent,
		}},
		Candidate: []ExpectedObservationSpec{{
			Name: "generated buffer", Source: ObservationArtifact, Path: "pipeline.mlir",
			Matcher: ObservationExact, Pattern: "!buffer<4>", Occurrence: ObservationPresent,
		}},
	})
	status, results := evaluateObservations(context.Background(), contract, RunKindCandidate, dir, "", "", 1024)
	if status != ObservationMatched || len(results) != 1 || results[0].Excerpt == "" {
		t.Fatalf("status=%q results=%#v, want matched artifact", status, results)
	}
}

func TestObservationArtifactRejectsWorkspaceEscape(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	contract := mustObservationContract(t, ObservationContractSpec{
		Intent: "artifact remains inside workspace",
		Base: []ExpectedObservationSpec{{
			Name: "escaped", Source: ObservationArtifact, Path: "escape/secret",
			Matcher: ObservationExact, Pattern: "secret", Occurrence: ObservationPresent,
		}},
		Candidate: []ExpectedObservationSpec{{
			Name: "escaped", Source: ObservationArtifact, Path: "escape/secret",
			Matcher: ObservationExact, Pattern: "secret", Occurrence: ObservationPresent,
		}},
	})
	status, results := evaluateObservations(context.Background(), contract, RunKindBase, root, "", "", 1024)
	if status != ObservationMismatched || len(results) != 1 || !strings.Contains(results[0].Error, "escapes") {
		t.Fatalf("status=%q results=%#v, want escape rejection", status, results)
	}
}

func TestParseObservationContractRejectsInvalidRegexp(t *testing.T) {
	t.Parallel()
	_, err := ParseObservationContract(ObservationContractSpec{
		Intent: "observe output",
		Base: []ExpectedObservationSpec{{
			Name: "invalid", Source: ObservationStderr,
			Matcher: ObservationRegexp, Pattern: "[", Occurrence: ObservationPresent,
		}},
		Candidate: []ExpectedObservationSpec{{
			Name: "valid", Source: ObservationStdout,
			Matcher: ObservationExact, Pattern: "ok", Occurrence: ObservationPresent,
		}},
	})
	if !errors.Is(err, ErrInvalidObservation) {
		t.Fatalf("error = %v, want ErrInvalidObservation", err)
	}
}

func TestParseExpectedObservationEstablishesCanonicalInvariants(t *testing.T) {
	t.Parallel()
	parsed, err := ParseExpectedObservation(ExpectedObservationSpec{
		Name: "  generated artifact  ", Source: ObservationArtifact, Path: "build/../out.txt",
		Matcher: ObservationExact, Pattern: "fixed",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := ExpectedObservationSpec{
		Name: "generated artifact", Source: ObservationArtifact, Path: "out.txt",
		Matcher: ObservationExact, Pattern: "fixed", Occurrence: ObservationPresent,
	}
	if got := parsed.Spec(); got != want {
		t.Fatalf("spec = %#v, want %#v", got, want)
	}

	invalid := []ExpectedObservationSpec{
		{Name: "unknown source", Source: "network", Matcher: ObservationExact, Pattern: "x"},
		{Name: "stdout path", Source: ObservationStdout, Path: "out.txt", Matcher: ObservationExact, Pattern: "x"},
		{Name: "escape", Source: ObservationArtifact, Path: "../out.txt", Matcher: ObservationExact, Pattern: "x"},
		{Name: "unknown matcher", Source: ObservationStdout, Matcher: "glob", Pattern: "x"},
		{Name: "unknown occurrence", Source: ObservationStdout, Matcher: ObservationExact, Pattern: "x", Occurrence: "sometimes"},
		{Name: "empty pattern", Source: ObservationStdout, Matcher: ObservationExact},
	}
	for _, spec := range invalid {
		if _, err := ParseExpectedObservation(spec); !errors.Is(err, ErrInvalidObservation) {
			t.Errorf("ParseExpectedObservation(%q) error = %v, want ErrInvalidObservation", spec.Name, err)
		}
	}
}

func TestObservationContractJSONRejectsCorruptPersistedRegexp(t *testing.T) {
	t.Parallel()
	data := []byte(`{"Intent":"persisted contract may be corrupt","Base":[{"Name":"invalid","Source":"stdout","Matcher":"regexp","Pattern":"[","Occurrence":"present","Path":""}],"Candidate":[{"Name":"valid","Source":"stdout","Matcher":"exact","Pattern":"ok","Occurrence":"present","Path":""}]}`)
	var contract ObservationContract
	err := json.Unmarshal(data, &contract)
	if !errors.Is(err, ErrInvalidObservation) {
		t.Fatalf("error = %v, want ErrInvalidObservation", err)
	}
}

func TestObservationContractJSONRejectsUnknownFields(t *testing.T) {
	t.Parallel()
	data := []byte(`{"Intent":"proof","Base":[],"Candidate":[],"Unexpected":true}`)
	var contract ObservationContract
	if err := json.Unmarshal(data, &contract); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v, want unknown-field rejection", err)
	}
}

func TestParseObservationContractRequiresBaseAndCandidateObservations(t *testing.T) {
	t.Parallel()
	_, err := ParseObservationContract(ObservationContractSpec{
		Intent: "observe the intended behavior on both runs",
		Base: []ExpectedObservationSpec{{
			Name: "base symptom", Source: ObservationStderr,
			Matcher: ObservationExact, Pattern: "failure", Occurrence: ObservationPresent,
		}},
	})
	if !errors.Is(err, ErrInvalidObservation) {
		t.Fatalf("error = %v, want ErrInvalidObservation", err)
	}
}

func TestDefineValidationRejectsUnparsedObservationContract(t *testing.T) {
	t.Parallel()
	svc := NewService(newFakeRepo(), &fakeRunner{})
	err := svc.DefineValidation(context.Background(), &ValidationDefinition{
		Command: []string{"test"}, WorkingDir: "/tmp", Observation: &ObservationContract{},
	})
	if !errors.Is(err, ErrInvalidObservation) {
		t.Fatalf("error = %v, want ErrInvalidObservation", err)
	}
}

func mustObservationContract(t *testing.T, spec ObservationContractSpec) *ObservationContract {
	t.Helper()
	contract, err := ParseObservationContract(spec)
	if err != nil {
		t.Fatalf("parse observation contract: %v", err)
	}
	return contract
}

func TestCompareValidationObservationMismatchIsInconclusive(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
