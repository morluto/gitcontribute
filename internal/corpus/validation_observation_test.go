package corpus

import (
	"context"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/investigation"
)

func TestValidationObservationPayloadRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	now := time.Now().UTC()
	inv, err := investigation.NewService(c, c).StartInvestigation(ctx, domain.RepoRef{Owner: "owner", Repo: "repo"}, "abc123", "")
	if err != nil {
		t.Fatal(err)
	}
	definition := &evidence.ValidationDefinition{
		ID: "definition", InvestigationID: inv.ID, Command: []string{"test"}, WorkingDir: "/tmp", CreatedAt: now,
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
	group := &evidence.ValidationRunGroup{
		ID: "group", DefinitionID: definition.ID, InvestigationID: inv.ID, RequestedRuns: 1, CompletedRuns: 1,
		Concurrency: 1, PerRunTimeout: time.Second, OverallTimeout: time.Second,
		SampleInterval: 100 * time.Millisecond, Classification: evidence.RunGroupStablePass,
		Attempts: []evidence.ValidationAttempt{{Index: 1, Kind: evidence.RunKindCandidate, RunID: run.ID,
			Classification: evidence.RunClassificationPassing, Phases: evidence.RunPhases{InitializedAt: now}}},
		StartedAt: now, CompletedAt: now,
	}
	if err := c.SaveValidationRunGroup(ctx, group); err != nil {
		t.Fatalf("save validation group: %v", err)
	}
	gotGroup, err := c.GetValidationRunGroup(ctx, group.ID)
	if err != nil {
		t.Fatalf("get validation group: %v", err)
	}
	if len(gotGroup.Attempts) != 1 || !gotGroup.Attempts[0].Phases.InitializedAt.Equal(now) {
		t.Fatalf("validation group = %#v", gotGroup)
	}
}
