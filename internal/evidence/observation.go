package evidence

import (
	"context"
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

func validateObservationContract(contract *ObservationContract) error {
	if contract == nil {
		return nil
	}
	if strings.TrimSpace(contract.Intent) == "" || len(contract.Intent) > maxObservationIntentBytes {
		return fmt.Errorf("%w: intent is required and must be at most %d bytes", ErrInvalidObservation, maxObservationIntentBytes)
	}
	if len(contract.Base) == 0 || len(contract.Candidate) == 0 {
		return fmt.Errorf("%w: at least one base and candidate observation is required", ErrInvalidObservation)
	}
	if err := validateExpectedObservations("base", contract.Base); err != nil {
		return err
	}
	return validateExpectedObservations("candidate", contract.Candidate)
}

func validateExpectedObservations(kind string, observations []ExpectedObservation) error {
	if len(observations) > maxObservationsPerRun {
		return fmt.Errorf("%w: %s has more than %d observations", ErrInvalidObservation, kind, maxObservationsPerRun)
	}
	for i := range observations {
		observation := &observations[i]
		if strings.TrimSpace(observation.Name) == "" || len(observation.Name) > maxObservationNameBytes {
			return fmt.Errorf("%w: %s[%d] name is required and must be at most %d bytes", ErrInvalidObservation, kind, i, maxObservationNameBytes)
		}
		if observation.Source != ObservationStdout && observation.Source != ObservationStderr && observation.Source != ObservationArtifact {
			return fmt.Errorf("%w: %s[%d] source must be stdout, stderr, or artifact", ErrInvalidObservation, kind, i)
		}
		if observation.Source == ObservationArtifact {
			if err := validateArtifactPath(observation.Path); err != nil {
				return fmt.Errorf("%w: %s[%d] artifact path: %v", ErrInvalidObservation, kind, i, err)
			}
		} else if observation.Path != "" {
			return fmt.Errorf("%w: %s[%d] path is only valid for artifact observations", ErrInvalidObservation, kind, i)
		}
		if observation.Matcher != ObservationExact && observation.Matcher != ObservationRegexp {
			return fmt.Errorf("%w: %s[%d] matcher must be exact or regexp", ErrInvalidObservation, kind, i)
		}
		if observation.Pattern == "" || len(observation.Pattern) > maxObservationPatternBytes {
			return fmt.Errorf("%w: %s[%d] pattern is required and must be at most %d bytes", ErrInvalidObservation, kind, i, maxObservationPatternBytes)
		}
		if observation.Occurrence == "" {
			observation.Occurrence = ObservationPresent
		}
		if observation.Occurrence != ObservationPresent && observation.Occurrence != ObservationAbsent {
			return fmt.Errorf("%w: %s[%d] occurrence must be present or absent", ErrInvalidObservation, kind, i)
		}
		if observation.Matcher == ObservationRegexp {
			if _, err := regexp.Compile(observation.Pattern); err != nil {
				return fmt.Errorf("%w: %s[%d] regexp: %v", ErrInvalidObservation, kind, i, err)
			}
		}
	}
	return nil
}

func evaluateObservations(ctx context.Context, contract *ObservationContract, kind RunKind, workingDir, stdout, stderr string, maxBytes int64) (ObservationStatus, []ObservationResult) {
	if contract == nil {
		return ObservationNotEvaluated, nil
	}
	expected := contract.Candidate
	if kind == RunKindBase {
		expected = contract.Base
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
		matched, excerpt, matchErr := matchObservation(output, observation)
		if matchErr != nil {
			status = ObservationMismatched
			results = append(results, ObservationResult{
				ExpectedObservation: observation,
				Status:              ObservationMismatched, Error: matchErr.Error(),
			})
			continue
		}
		if observation.Occurrence == ObservationAbsent {
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
		return fmt.Errorf("path is required")
	}
	if filepath.IsAbs(path) {
		return fmt.Errorf("path must be relative")
	}
	clean := filepath.Clean(path)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path must stay within the validation workspace")
	}
	return nil
}

func observationOutput(ctx context.Context, observation ExpectedObservation, workingDir, stdout, stderr string, maxBytes int64) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	switch observation.Source {
	case ObservationStdout:
		return stdout, nil
	case ObservationStderr:
		return stderr, nil
	case ObservationArtifact:
		return readObservationArtifact(workingDir, observation.Path, maxBytes)
	default:
		return "", fmt.Errorf("unsupported observation source %q", observation.Source)
	}
}

func readObservationArtifact(workingDir, path string, maxBytes int64) (string, error) {
	if err := validateArtifactPath(path); err != nil {
		return "", err
	}
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
		return "", fmt.Errorf("artifact path escapes validation workspace")
	}
	f, err := os.Open(target)
	if err != nil {
		return "", fmt.Errorf("open artifact: %w", err)
	}
	defer f.Close()
	if maxBytes <= 0 {
		maxBytes = defaultMaxOutputBytes
	}
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return "", fmt.Errorf("read artifact: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return "", fmt.Errorf("artifact exceeds %d-byte observation bound", maxBytes)
	}
	return string(data), nil
}

func matchObservation(output string, observation ExpectedObservation) (bool, string, error) {
	start, end := -1, -1
	if observation.Matcher == ObservationExact {
		start = strings.Index(output, observation.Pattern)
		if start >= 0 {
			end = start + len(observation.Pattern)
		}
	} else {
		matcher, err := regexp.Compile(observation.Pattern)
		if err != nil {
			return false, "", fmt.Errorf("compile observation regexp: %w", err)
		}
		location := matcher.FindStringIndex(output)
		if location != nil {
			start, end = location[0], location[1]
		}
	}
	if start < 0 {
		return false, "", nil
	}
	return true, boundedObservationExcerpt(output, start, end), nil
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
