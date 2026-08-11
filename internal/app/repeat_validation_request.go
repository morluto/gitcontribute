package app

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

type validationRunTarget uint8

const (
	validationRunBase validationRunTarget = iota + 1
	validationRunCandidate
	validationRunBoth
)

func parseValidationRunKind(value string) (evidence.RunKind, error) {
	switch strings.TrimSpace(value) {
	case "base":
		return evidence.RunKindBase, nil
	case "candidate":
		return evidence.RunKindCandidate, nil
	default:
		return "", errors.New("validation kind must be base or candidate")
	}
}

func parseRepeatValidationTarget(value string) (validationRunTarget, error) {
	if strings.TrimSpace(value) == "both" {
		return validationRunBoth, nil
	}
	kind, err := parseValidationRunKind(value)
	if err != nil {
		return 0, errors.New("target must be base, candidate, or both")
	}
	if kind == evidence.RunKindBase {
		return validationRunBase, nil
	}
	return validationRunCandidate, nil
}

func (t validationRunTarget) String() string {
	switch t {
	case validationRunBase:
		return "base"
	case validationRunCandidate:
		return "candidate"
	case validationRunBoth:
		return "both"
	default:
		panic("invalid validation run target")
	}
}

func (t validationRunTarget) kinds() []evidence.RunKind {
	switch t {
	case validationRunBase:
		return []evidence.RunKind{evidence.RunKindBase}
	case validationRunCandidate:
		return []evidence.RunKind{evidence.RunKindCandidate}
	case validationRunBoth:
		return []evidence.RunKind{evidence.RunKindBase, evidence.RunKindCandidate}
	default:
		panic("invalid validation run target")
	}
}

type repeatValidationRequest struct {
	definitionID string
	target       validationRunTarget
	options      evidence.RepeatValidationOptions
}

type validationRunRequest struct {
	definitionID string
	kind         evidence.RunKind
}

func parseValidationRunOptions(definitionID string, opts contracts.RunValidationOptions) (validationRunRequest, error) {
	if !opts.Execute {
		return validationRunRequest{}, evidence.ErrExecutionNotAuthorized
	}
	definitionID = strings.TrimSpace(definitionID)
	if definitionID == "" {
		return validationRunRequest{}, errors.New("validation definition ID is required")
	}
	kind, err := parseValidationRunKind(opts.Kind)
	if err != nil {
		return validationRunRequest{}, err
	}
	return validationRunRequest{definitionID: definitionID, kind: kind}, nil
}

func parseMCPRepeatValidationInput(in mcpcontract.RunValidationInput) (repeatValidationRequest, mcpcontract.RunValidationInput, error) {
	if !in.Execute {
		return repeatValidationRequest{}, mcpcontract.RunValidationInput{}, errors.New("execute must be true to authorize host command execution")
	}
	definitionID := strings.TrimSpace(in.ID)
	if definitionID == "" {
		return repeatValidationRequest{}, mcpcontract.RunValidationInput{}, errors.New("validation definition ID is required")
	}
	target, err := parseRepeatValidationTarget(in.Target)
	if err != nil {
		return repeatValidationRequest{}, mcpcontract.RunValidationInput{}, err
	}
	if in.RunCount == 0 {
		in.RunCount = 1
	}
	if in.Concurrency == 0 {
		in.Concurrency = 1
	}
	if strings.TrimSpace(in.SampleInterval) == "" {
		in.SampleInterval = "100ms"
	}
	perRunTimeout, err := parseOptionalDuration(in.PerRunTimeout)
	if err != nil {
		return repeatValidationRequest{}, mcpcontract.RunValidationInput{}, fmt.Errorf("per_run_timeout: %w", err)
	}
	overallTimeout, err := parseOptionalDuration(in.OverallTimeout)
	if err != nil {
		return repeatValidationRequest{}, mcpcontract.RunValidationInput{}, fmt.Errorf("overall_timeout: %w", err)
	}
	sampleInterval, err := parseOptionalDuration(in.SampleInterval)
	if err != nil {
		return repeatValidationRequest{}, mcpcontract.RunValidationInput{}, fmt.Errorf("sample_interval: %w", err)
	}
	options, err := evidence.ParseRepeatValidationOptions(evidence.RepeatValidationOptions{
		Kinds: target.kinds(), RunCount: in.RunCount, Concurrency: in.Concurrency,
		PerRunTimeout: perRunTimeout, OverallTimeout: overallTimeout, SampleInterval: sampleInterval,
	})
	if err != nil {
		return repeatValidationRequest{}, mcpcontract.RunValidationInput{}, err
	}
	canonical := mcpcontract.RunValidationInput{
		ID: definitionID, Target: target.String(), RunCount: options.RunCount, Concurrency: options.Concurrency,
		PerRunTimeout: optionalDurationString(options.PerRunTimeout), OverallTimeout: optionalDurationString(options.OverallTimeout),
		SampleInterval: optionalDurationString(options.SampleInterval), Execute: true,
	}
	return repeatValidationRequest{definitionID: definitionID, target: target, options: options}, canonical, nil
}

func parseRepeatValidationOptions(definitionID string, opts contracts.RepeatValidationOptions) (repeatValidationRequest, error) {
	if !opts.Execute {
		return repeatValidationRequest{}, evidence.ErrExecutionNotAuthorized
	}
	definitionID = strings.TrimSpace(definitionID)
	if definitionID == "" {
		return repeatValidationRequest{}, errors.New("validation definition ID is required")
	}
	kinds := make([]evidence.RunKind, len(opts.Kinds))
	for i, value := range opts.Kinds {
		kind, err := parseValidationRunKind(value)
		if err != nil {
			return repeatValidationRequest{}, evidence.ErrMissingRunKind
		}
		kinds[i] = kind
	}
	options, err := evidence.ParseRepeatValidationOptions(evidence.RepeatValidationOptions{
		Kinds: kinds, RunCount: opts.RunCount, Concurrency: opts.Concurrency,
		PerRunTimeout: opts.PerRunTimeout, OverallTimeout: opts.OverallTimeout, SampleInterval: opts.SampleInterval,
	})
	if err != nil {
		return repeatValidationRequest{}, err
	}
	return repeatValidationRequest{definitionID: definitionID, options: options}, nil
}

func optionalDurationString(value time.Duration) string {
	if value == 0 {
		return ""
	}
	return value.String()
}
