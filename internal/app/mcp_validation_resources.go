package app

import (
	"context"
	"errors"

	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// ValidationGroupResource reads the persisted execution group without running
// validation again. Attempt IDs can be used to attach independently supplied reports.
func (r *MCPReader) ValidationGroupResource(ctx context.Context, id string) (mcpcontract.ValidationGroupResource, error) {
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.ValidationGroupResource{}, err
	}
	group, err := c.GetValidationRunGroup(ctx, id)
	if errors.Is(err, evidence.ErrNotFound) {
		return mcpcontract.ValidationGroupResource{}, mcpcontract.ErrNotFound
	}
	if err != nil {
		return mcpcontract.ValidationGroupResource{}, err
	}
	out := mcpcontract.ValidationGroupResource{
		ID: group.ID, DefinitionID: group.DefinitionID, InvestigationID: group.InvestigationID, ConfigurationSHA256: group.ConfigurationSHA256,
		RequestedRuns: group.RequestedRuns, CompletedRuns: group.CompletedRuns, Classification: string(group.Classification),
		StartedAt: formatTime(group.StartedAt), CompletedAt: formatTime(group.CompletedAt),
		Attempts: make([]mcpcontract.ValidationAttemptResource, len(group.Attempts)),
	}
	for i, attempt := range group.Attempts {
		out.Attempts[i] = mcpcontract.ValidationAttemptResource{
			Index: attempt.Index, Kind: string(attempt.Kind), RunID: attempt.RunID, Classification: string(attempt.Classification),
			ObservationStatus: string(attempt.ObservationStatus), ExitCode: attempt.ExitCode, TimeoutPhase: string(attempt.TimeoutPhase), FailurePhase: string(attempt.FailurePhase),
			StartedAt: formatTime(attempt.StartedAt), CompletedAt: formatTime(attempt.CompletedAt),
		}
	}
	return out, nil
}
