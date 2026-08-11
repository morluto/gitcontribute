package evidence

import (
	"errors"
	"fmt"
)

// ParseStored parses validation-definition discriminators after
// durable JSON decoding.
func (d *ValidationDefinition) ParseStored() error {
	if d == nil || d.ID == "" {
		return errors.New("validation definition ID is required")
	}
	if d.Protocol != "" && d.Protocol != ValidationProtocolMCPStdio {
		return fmt.Errorf("unsupported validation protocol %q", d.Protocol)
	}
	if d.Observation != nil && d.Observation.intent == "" {
		return errors.New("stored observation contract was not parsed")
	}
	return nil
}

// ParseStored parses validation-run outcomes after durable JSON
// decoding.
func (r *ValidationRun) ParseStored() error {
	if r == nil || r.ID == "" || r.DefinitionID == "" {
		return errors.New("validation run identity is required")
	}
	if !validRunKind(r.Kind) {
		return fmt.Errorf("unsupported validation run kind %q", r.Kind)
	}
	if !validRunClassification(r.Classification) {
		return fmt.Errorf("unsupported validation run classification %q", r.Classification)
	}
	// Empty is the legacy representation of a run with no observation contract.
	if r.ObservationStatus == "" {
		r.ObservationStatus = ObservationNotEvaluated
	}
	if !validObservationStatus(r.ObservationStatus) {
		return fmt.Errorf("unsupported observation status %q", r.ObservationStatus)
	}
	if !validWorkspaceBindingStatus(r.WorkspaceBindingStatus) {
		return fmt.Errorf("unsupported workspace binding status %q", r.WorkspaceBindingStatus)
	}
	if !validExecutionOrigin(r.ExecutionOrigin) {
		return fmt.Errorf("unsupported execution origin %q", r.ExecutionOrigin)
	}
	if !validCleanupStatus(r.Cleanup.Status) {
		return fmt.Errorf("unsupported cleanup status %q", r.Cleanup.Status)
	}
	if !validValidationPhase(r.TimeoutPhase) || !validValidationPhase(r.FailurePhase) {
		return errors.New("validation run has an unsupported failure or timeout phase")
	}
	if r.JUnitReport != nil {
		if err := r.JUnitReport.ParseStored(); err != nil {
			return fmt.Errorf("stored JUnit report: %w", err)
		}
	}
	return nil
}

// ParseStored parses repeat-run classifications after durable JSON
// decoding.
func (g *ValidationRunGroup) ParseStored() error {
	if g == nil || g.ID == "" || g.DefinitionID == "" {
		return errors.New("validation run group identity is required")
	}
	if !validRunGroupClassification(g.Classification) {
		return fmt.Errorf("unsupported validation group classification %q", g.Classification)
	}
	for i := range g.Attempts {
		attempt := &g.Attempts[i]
		if attempt.ObservationStatus == "" {
			attempt.ObservationStatus = ObservationNotEvaluated
		}
		if !validRunKind(attempt.Kind) || !validRunClassification(attempt.Classification) || !validObservationStatus(attempt.ObservationStatus) || !validCleanupStatus(attempt.Cleanup.Status) || !validValidationPhase(attempt.TimeoutPhase) || !validValidationPhase(attempt.FailurePhase) {
			return fmt.Errorf("validation attempt %d has an unsupported discriminator", i)
		}
	}
	for i, aggregate := range g.Aggregates {
		if !validRunKind(aggregate.Kind) || !validRunGroupClassification(aggregate.Classification) || !validResourceClassification(aggregate.ResourceClassification) {
			return fmt.Errorf("validation aggregate %d has an unsupported discriminator", i)
		}
	}
	if g.Comparison != nil && !validComparisonClassification(g.Comparison.Classification) {
		return fmt.Errorf("unsupported validation comparison %q", g.Comparison.Classification)
	}
	return nil
}

// ParseStored parses evidence type and relation claims after
// durable JSON decoding.
func (e *Evidence) ParseStored() error {
	if e == nil || e.ID == "" {
		return errors.New("evidence ID is required")
	}
	if !isValidEvidenceType(e.Type) {
		return fmt.Errorf("unsupported evidence type %q", e.Type)
	}
	if !isValidRelation(e.Relation) {
		return fmt.Errorf("unsupported evidence relation %q", e.Relation)
	}
	if e.ValidationDefinition != nil {
		if err := e.ValidationDefinition.ParseStored(); err != nil {
			return fmt.Errorf("embedded validation definition: %w", err)
		}
	}
	if e.ValidationRun != nil {
		if err := e.ValidationRun.ParseStored(); err != nil {
			return fmt.Errorf("embedded validation run: %w", err)
		}
	}
	if e.External != nil {
		if _, err := ParseExternalEvidenceCompleteness(string(e.External.Completeness)); err != nil {
			return fmt.Errorf("unsupported external evidence completeness %q", e.External.Completeness)
		}
		if e.External.Integrity != ExternalEvidenceVerified && e.External.Integrity != ExternalEvidenceUnverified {
			return fmt.Errorf("unsupported external evidence integrity %q", e.External.Integrity)
		}
	}
	return nil
}

func validRunKind(kind RunKind) bool { return kind == RunKindBase || kind == RunKindCandidate }

func validRunClassification(classification RunClassification) bool {
	switch classification {
	case RunClassificationPassing, RunClassificationFailing, RunClassificationError, RunClassificationCancelled:
		return true
	default:
		return false
	}
}

func validObservationStatus(status ObservationStatus) bool {
	switch status {
	case ObservationNotEvaluated, ObservationMatched, ObservationMismatched:
		return true
	default:
		return false
	}
}

func validCleanupStatus(status CleanupStatus) bool {
	switch status {
	case CleanupUnknown, CleanupClean, CleanupFailed, CleanupUnavailable:
		return true
	default:
		return false
	}
}

func validValidationPhase(phase ValidationPhase) bool {
	switch phase {
	case ValidationPhaseNone, ValidationPhaseStartup, ValidationPhaseReadiness, ValidationPhaseExecution, ValidationPhaseShutdown:
		return true
	default:
		return false
	}
}

func validWorkspaceBindingStatus(status WorkspaceBindingStatus) bool {
	switch status {
	case WorkspaceBindingUnknown, WorkspaceBindingUnavailable, WorkspaceBindingIncomplete,
		WorkspaceBindingChanged, WorkspaceBindingBound, WorkspaceBindingStale, WorkspaceBindingIncompatible:
		return true
	default:
		return false
	}
}

func validExecutionOrigin(origin ExecutionOrigin) bool {
	return origin == ExecutionOriginLocal || origin == ExecutionOriginExternal
}

func validResourceClassification(classification ResourceClassification) bool {
	switch classification {
	case ResourceUnknown, ResourceAvailable, ResourceCleanupFailed, ResourceInconclusive:
		return true
	default:
		return false
	}
}

func validRunGroupClassification(classification RunGroupClassification) bool {
	switch classification {
	case RunGroupStablePass, RunGroupStableFail, RunGroupFlaky, RunGroupInconclusive, RunGroupCancelled:
		return true
	default:
		return false
	}
}

func validComparisonClassification(classification ComparisonClassification) bool {
	switch classification {
	case ComparisonFixed, ComparisonNotFixed, ComparisonRegression, ComparisonNoDifference, ComparisonInconclusive:
		return true
	default:
		return false
	}
}
