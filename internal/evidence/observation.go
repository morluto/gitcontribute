package evidence

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	maxObservationIntentBytes  = 1024
	maxObservationsPerRun      = 8
	maxObservationNameBytes    = 128
	maxObservationPatternBytes = 4096
	maxObservationExcerptBytes = 1024
)

// ParseExpectedObservation converts an untrusted observation specification into
// the representation used by execution and persistence.
func ParseExpectedObservation(spec ExpectedObservationSpec) (ExpectedObservation, error) {
	observation, err := parseExpectedObservation(spec)
	if err != nil {
		return ExpectedObservation{}, fmt.Errorf("%w: %w", ErrInvalidObservation, err)
	}
	return observation, nil
}

func parseExpectedObservation(spec ExpectedObservationSpec) (ExpectedObservation, error) {
	name := strings.TrimSpace(spec.Name)
	if name == "" || len(name) > maxObservationNameBytes {
		return ExpectedObservation{}, fmt.Errorf("name is required and must be at most %d bytes", maxObservationNameBytes)
	}
	if spec.Source != ObservationStdout && spec.Source != ObservationStderr && spec.Source != ObservationArtifact {
		return ExpectedObservation{}, errors.New("source must be stdout, stderr, or artifact")
	}
	path := spec.Path
	if spec.Source == ObservationArtifact {
		if err := validateArtifactPath(path); err != nil {
			return ExpectedObservation{}, fmt.Errorf("artifact path: %w", err)
		}
		path = filepath.Clean(path)
	} else if path != "" {
		return ExpectedObservation{}, errors.New("path is only valid for artifact observations")
	}
	if spec.Matcher != ObservationExact && spec.Matcher != ObservationRegexp {
		return ExpectedObservation{}, errors.New("matcher must be exact or regexp")
	}
	if spec.Pattern == "" || len(spec.Pattern) > maxObservationPatternBytes {
		return ExpectedObservation{}, fmt.Errorf("pattern is required and must be at most %d bytes", maxObservationPatternBytes)
	}
	occurrence := spec.Occurrence
	if occurrence == "" {
		occurrence = ObservationPresent
	}
	if occurrence != ObservationPresent && occurrence != ObservationAbsent {
		return ExpectedObservation{}, errors.New("occurrence must be present or absent")
	}
	var compiled *regexp.Regexp
	if spec.Matcher == ObservationRegexp {
		var err error
		compiled, err = regexp.Compile(spec.Pattern)
		if err != nil {
			return ExpectedObservation{}, fmt.Errorf("regexp: %w", err)
		}
	}
	return ExpectedObservation{
		name: name, source: spec.Source, matcher: spec.Matcher, pattern: spec.Pattern,
		occurrence: occurrence, path: path, compiled: compiled,
	}, nil
}

// ParseObservationContract establishes the complete proof-contract invariant.
func ParseObservationContract(spec ObservationContractSpec) (*ObservationContract, error) {
	intent := strings.TrimSpace(spec.Intent)
	if intent == "" || len(intent) > maxObservationIntentBytes {
		return nil, fmt.Errorf("%w: intent is required and must be at most %d bytes", ErrInvalidObservation, maxObservationIntentBytes)
	}
	if len(spec.Base) == 0 || len(spec.Candidate) == 0 {
		return nil, fmt.Errorf("%w: at least one base and candidate observation is required", ErrInvalidObservation)
	}
	base, err := parseExpectedObservations("base", spec.Base)
	if err != nil {
		return nil, err
	}
	candidate, err := parseExpectedObservations("candidate", spec.Candidate)
	if err != nil {
		return nil, err
	}
	return &ObservationContract{intent: intent, base: base, candidate: candidate}, nil
}

func parseExpectedObservations(kind string, specs []ExpectedObservationSpec) ([]ExpectedObservation, error) {
	if len(specs) > maxObservationsPerRun {
		return nil, fmt.Errorf("%w: %s has more than %d observations", ErrInvalidObservation, kind, maxObservationsPerRun)
	}
	observations := make([]ExpectedObservation, len(specs))
	for i, spec := range specs {
		observation, err := ParseExpectedObservation(spec)
		if err != nil {
			return nil, fmt.Errorf("%w: %s[%d]: %w", ErrInvalidObservation, kind, i, err)
		}
		observations[i] = observation
	}
	return observations, nil
}

// Spec returns the canonical boundary representation of an observation.
func (o ExpectedObservation) Spec() ExpectedObservationSpec {
	return ExpectedObservationSpec{
		Name: o.name, Source: o.source, Matcher: o.matcher, Pattern: o.pattern,
		Occurrence: o.occurrence, Path: o.path,
	}
}

// Name identifies the assertion in human-facing evidence.
func (o ExpectedObservation) Name() string { return o.name }

// Intent describes the proof established by the contract.
func (c *ObservationContract) Intent() string { return c.intent }

// Base returns the parsed observations expected from the base revision.
func (c *ObservationContract) Base() []ExpectedObservation {
	return append([]ExpectedObservation(nil), c.base...)
}

// Candidate returns the parsed observations expected from the candidate revision.
func (c *ObservationContract) Candidate() []ExpectedObservation {
	return append([]ExpectedObservation(nil), c.candidate...)
}

// Spec returns the canonical boundary representation of a contract.
func (c *ObservationContract) Spec() ObservationContractSpec {
	return ObservationContractSpec{Intent: c.intent, Base: observationSpecs(c.base), Candidate: observationSpecs(c.candidate)}
}

func observationSpecs(observations []ExpectedObservation) []ExpectedObservationSpec {
	specs := make([]ExpectedObservationSpec, len(observations))
	for i, observation := range observations {
		specs[i] = observation.Spec()
	}
	return specs
}

// MarshalJSON persists only the canonical boundary representation.
func (o ExpectedObservation) MarshalJSON() ([]byte, error) {
	if o.name == "" {
		return nil, fmt.Errorf("%w: unparsed expected observation", ErrInvalidObservation)
	}
	return json.Marshal(o.Spec())
}

// UnmarshalJSON reparses persisted input instead of trusting stored fields.
func (o *ExpectedObservation) UnmarshalJSON(data []byte) error {
	var spec ExpectedObservationSpec
	if err := decodeObservationJSON(data, &spec); err != nil {
		return err
	}
	parsed, err := ParseExpectedObservation(spec)
	if err != nil {
		return err
	}
	*o = parsed
	return nil
}

// MarshalJSON persists only the canonical boundary representation.
func (c ObservationContract) MarshalJSON() ([]byte, error) {
	if c.intent == "" {
		return nil, fmt.Errorf("%w: unparsed observation contract", ErrInvalidObservation)
	}
	return json.Marshal(c.Spec())
}

// UnmarshalJSON reparses persisted input instead of trusting stored fields.
func (c *ObservationContract) UnmarshalJSON(data []byte) error {
	var spec ObservationContractSpec
	if err := decodeObservationJSON(data, &spec); err != nil {
		return err
	}
	parsed, err := ParseObservationContract(spec)
	if err != nil {
		return err
	}
	*c = *parsed
	return nil
}

type observationResultJSON struct {
	ExpectedObservationSpec
	Status  ObservationStatus
	Excerpt string
	Error   string
}

// MarshalJSON keeps the historical flattened result shape while preventing
// the embedded observation's marshaler from swallowing result fields.
func (r ObservationResult) MarshalJSON() ([]byte, error) {
	if r.name == "" {
		return nil, fmt.Errorf("%w: result contains an unparsed observation", ErrInvalidObservation)
	}
	return json.Marshal(observationResultJSON{
		ExpectedObservationSpec: r.Spec(),
		Status:                  r.Status, Excerpt: r.Excerpt, Error: r.Error,
	})
}

// UnmarshalJSON reparses the embedded observation before exposing a stored result.
func (r *ObservationResult) UnmarshalJSON(data []byte) error {
	var stored observationResultJSON
	if err := decodeObservationJSON(data, &stored); err != nil {
		return err
	}
	observation, err := ParseExpectedObservation(stored.ExpectedObservationSpec)
	if err != nil {
		return fmt.Errorf("stored result: %w", err)
	}
	*r = ObservationResult{
		ExpectedObservation: observation,
		Status:              stored.Status, Excerpt: stored.Excerpt, Error: stored.Error,
	}
	return nil
}

func decodeObservationJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode observation: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("decode observation: multiple JSON values")
		}
		return fmt.Errorf("decode observation: %w", err)
	}
	return nil
}

func evaluateObservations(ctx context.Context, contract *ObservationContract, kind RunKind, workingDir, stdout, stderr string, maxBytes int64) (ObservationStatus, []ObservationResult) {
	if contract == nil {
		return ObservationNotEvaluated, nil
	}
	expected := contract.candidate
	if kind == RunKindBase {
		expected = contract.base
	}
	if len(expected) == 0 {
		return ObservationNotEvaluated, nil
	}
	results := make([]ObservationResult, 0, len(expected))
	status := ObservationMatched
	for _, observation := range expected {
		output, readErr := observationOutput(ctx, observation, workingDir, stdout, stderr, maxBytes)
		if readErr != nil {
			status = ObservationMismatched
			results = append(results, ObservationResult{
				ExpectedObservation: observation,
				Status:              ObservationMismatched, Error: readErr.Error(),
			})
			continue
		}
		matched, excerpt := matchObservation(output, observation)
		if observation.occurrence == ObservationAbsent {
			matched = !matched
		}
		resultStatus := ObservationMatched
		if !matched {
			resultStatus = ObservationMismatched
			status = ObservationMismatched
		}
		results = append(results, ObservationResult{
			ExpectedObservation: observation,
			Status:              resultStatus,
			Excerpt:             excerpt,
		})
	}
	return status, results
}

func validateArtifactPath(path string) error {
	if path == "" {
		return errors.New("path is required")
	}
	if filepath.IsAbs(path) {
		return errors.New("path must be relative")
	}
	clean := filepath.Clean(path)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return errors.New("path must stay within the validation workspace")
	}
	return nil
}

func observationOutput(ctx context.Context, observation ExpectedObservation, workingDir, stdout, stderr string, maxBytes int64) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	switch observation.source {
	case ObservationStdout:
		return stdout, nil
	case ObservationStderr:
		return stderr, nil
	case ObservationArtifact:
		return readObservationArtifact(workingDir, observation.path, maxBytes)
	default:
		return "", fmt.Errorf("unparsed observation source %q", observation.source)
	}
}

func readObservationArtifact(workingDir, path string, maxBytes int64) (string, error) {
	root, err := filepath.EvalSymlinks(workingDir)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %w", err)
	}
	target, err := filepath.EvalSymlinks(filepath.Join(root, path))
	if err != nil {
		return "", fmt.Errorf("resolve artifact: %w", err)
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("artifact path escapes validation workspace")
	}
	f, err := os.Open(target)
	if err != nil {
		return "", fmt.Errorf("open artifact: %w", err)
	}
	if maxBytes <= 0 {
		maxBytes = defaultMaxOutputBytes
	}
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	closeErr := f.Close()
	if err != nil {
		return "", fmt.Errorf("read artifact: %w", err)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close artifact: %w", closeErr)
	}
	if int64(len(data)) > maxBytes {
		return "", fmt.Errorf("artifact exceeds %d-byte observation bound", maxBytes)
	}
	return string(data), nil
}

func matchObservation(output string, observation ExpectedObservation) (bool, string) {
	start, end := -1, -1
	if observation.matcher == ObservationExact {
		start = strings.Index(output, observation.pattern)
		if start >= 0 {
			end = start + len(observation.pattern)
		}
	} else {
		location := observation.compiled.FindStringIndex(output)
		if location != nil {
			start, end = location[0], location[1]
		}
	}
	if start < 0 {
		return false, ""
	}
	return true, boundedObservationExcerpt(output, start, end)
}

func boundedObservationExcerpt(output string, start, end int) string {
	if end-start >= maxObservationExcerptBytes {
		return output[start : start+maxObservationExcerptBytes]
	}
	remaining := maxObservationExcerptBytes - (end - start)
	left := min(start, remaining/2)
	right := min(len(output)-end, remaining-left)
	return output[start-left : end+right]
}
