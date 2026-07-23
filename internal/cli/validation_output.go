package cli

import (
	"fmt"
	"strings"
)

func validationHuman(r *ValidationResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Validation: %s (kind=%s, investigation=%s)\n", r.ID, r.Kind, r.InvestigationID)
	fmt.Fprintf(&b, "Command: %s\n", strings.Join(r.Command, " "))
	fmt.Fprintf(&b, "Working directory: %s\n", r.WorkingDir)
	if r.BaseWorkingDir != "" {
		fmt.Fprintf(&b, "Base working directory: %s\n", r.BaseWorkingDir)
	}
	if r.CandidateDir != "" {
		fmt.Fprintf(&b, "Candidate directory: %s\n", r.CandidateDir)
	}
	if r.Timeout != "" {
		fmt.Fprintf(&b, "Timeout: %s\n", r.Timeout)
	}
	if r.MaxOutputBytes > 0 {
		fmt.Fprintf(&b, "Max output bytes: %d\n", r.MaxOutputBytes)
	}
	if len(r.Env) > 0 {
		fmt.Fprintf(&b, "Environment allowlist: %s\n", strings.Join(r.Env, ", "))
	}
	if r.Observation != nil {
		fmt.Fprintf(&b, "Proof intent: %s\n", r.Observation.Intent)
		fmt.Fprintf(&b, "Expected observations: base=%d candidate=%d\n", len(r.Observation.Base), len(r.Observation.Candidate))
	}
	fmt.Fprintf(&b, "Created: %s", r.CreatedAt)
	return b.String()
}

func validationRunHuman(r *ValidationRunResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Validation run: %s (kind=%s, classification=%s, exit=%d)\n", r.ID, r.Kind, r.Classification, r.ExitCode)
	if r.ObservationStatus != "" {
		fmt.Fprintf(&b, "Observation status: %s\n", r.ObservationStatus)
	}
	for _, observation := range r.Observations {
		fmt.Fprintf(&b, "Observation %q: %s (%s %s %s)\n", observation.Name, observation.Status, observation.Source, observation.Matcher, observation.Occurrence)
		if observation.Excerpt != "" {
			fmt.Fprintf(&b, "Matched excerpt: %s\n", observation.Excerpt)
		}
		if observation.Error != "" {
			fmt.Fprintf(&b, "Observation error: %s\n", observation.Error)
		}
	}
	if r.Truncated {
		b.WriteString("Output truncated\n")
	}
	if r.Stdout != "" {
		fmt.Fprintf(&b, "--- stdout ---\n%s\n", r.Stdout)
	}
	if r.Stderr != "" {
		fmt.Fprintf(&b, "--- stderr ---\n%s\n", r.Stderr)
	}
	if r.Error != "" {
		fmt.Fprintf(&b, "Error: %s\n", r.Error)
	}
	fmt.Fprintf(&b, "Started: %s\n", r.StartedAt)
	fmt.Fprintf(&b, "Completed: %s", r.CompletedAt)
	return b.String()
}

func validationRunGroupHuman(group *ValidationRunGroupResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Validation group %s: %s (%d/%d runs)\n", group.ID, group.Classification, group.CompletedRuns, group.RequestedRuns)
	for _, aggregate := range group.Aggregates {
		fmt.Fprintf(&b, "  %s: %s; pass=%d fail=%d inconclusive=%d cancelled=%d resources=%s\n",
			aggregate.Kind, aggregate.Classification, aggregate.Passing, aggregate.Failing,
			aggregate.Inconclusive, aggregate.Cancelled, aggregate.ResourceClassification)
	}
	if group.Comparison != nil {
		fmt.Fprintf(&b, "  comparison: %s — %s\n", group.Comparison.Classification, group.Comparison.Explanation)
	}
	return b.String()
}

func validationComparisonHuman(r *ValidationComparisonResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Comparison: %s\n", r.Classification)
	fmt.Fprintf(&b, "Explanation: %s\n", r.Explanation)
	if r.Base != nil {
		fmt.Fprintf(&b, "Base run: %s (exit=%d, %s, observation=%s)\n", r.Base.ID, r.Base.ExitCode, r.Base.Classification, r.Base.ObservationStatus)
	}
	if r.Candidate != nil {
		fmt.Fprintf(&b, "Candidate run: %s (exit=%d, %s, observation=%s)\n", r.Candidate.ID, r.Candidate.ExitCode, r.Candidate.Classification, r.Candidate.ObservationStatus)
	}
	return b.String()
}
