package corpus

import (
	"context"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/evidence"
)

func TestValidationObservationPayloadRoundTrip(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	now := time.Now().UTC()
	definition := &evidence.ValidationDefinition{
		ID: "definition", Command: []string{"test"}, WorkingDir: "/tmp", CreatedAt: now,
		Observation: &evidence.ObservationContract{
			Intent: "observe artifact",
			Candidate: []evidence.ExpectedObservation{{
				Name: "generated", Source: evidence.ObservationArtifact, Path: "out.txt",
				Matcher: evidence.ObservationExact, Pattern: "fixed", Occurrence: evidence.ObservationPresent,
			}},
		},
	}
	if err := c.SaveValidationDefinition(ctx, definition); err != nil {
		t.Fatalf("save definition: %v", err)
	}
	run := &evidence.ValidationRun{
		ID: "run", DefinitionID: definition.ID, Kind: evidence.RunKindCandidate,
		Classification:    evidence.RunClassificationPassing,
		ObservationStatus: evidence.ObservationMatched,
		Observations: []evidence.ObservationResult{{
			ExpectedObservation: definition.Observation.Candidate[0],
			Status:              evidence.ObservationMatched, Excerpt: "fixed",
		}},
		StartedAt: now, CompletedAt: now,
	}
	if err := c.SaveValidationRun(ctx, run); err != nil {
		t.Fatalf("save run: %v", err)
	}

	gotDefinition, err := c.GetValidationDefinition(ctx, definition.ID)
	if err != nil {
		t.Fatalf("get definition: %v", err)
	}
	if gotDefinition.Observation == nil || gotDefinition.Observation.Candidate[0].Path != "out.txt" {
		t.Fatalf("definition observation = %#v", gotDefinition.Observation)
	}
	gotRun, err := c.GetValidationRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if gotRun.ObservationStatus != evidence.ObservationMatched || len(gotRun.Observations) != 1 || gotRun.Observations[0].Excerpt != "fixed" {
		t.Fatalf("run observation = %#v", gotRun)
	}
}
