package cli

import "fmt"

func threadInvestigationHuman(result *ThreadInvestigationResult) string {
	if result == nil || result.Investigation == nil || result.Hypothesis == nil {
		return "Thread investigation result is incomplete."
	}
	action := "Created"
	if !result.Created {
		action = "Reused open"
	}
	output := fmt.Sprintf("%s investigation %s with seed hypothesis %s.", action, result.Investigation.ID, result.Hypothesis.ID)
	if baseline := result.Investigation.ThreadBaseline; baseline != nil {
		output += fmt.Sprintf("\nBaseline: %s observation %d, sequence %d", baseline.Ref, baseline.ObservationID, baseline.ObservationSequence)
		if baseline.SourceUpdatedAt != "" {
			output += ", source updated " + baseline.SourceUpdatedAt
		}
		output += "."
	}
	return output
}
