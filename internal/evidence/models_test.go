package evidence

import "testing"

func TestValidationRunParseStoredAcceptsCanonicalProducerValues(t *testing.T) {
	t.Parallel()
	run := &ValidationRun{
		ID: "run", DefinitionID: "definition", Kind: RunKindCandidate, Classification: RunClassificationPassing,
		WorkspaceBindingStatus: WorkspaceBindingBound, ExecutionOrigin: ExecutionOriginExternal,
		Cleanup: CleanupResult{Status: CleanupClean}, TimeoutPhase: ValidationPhaseExecution, FailurePhase: ValidationPhaseShutdown,
	}
	if err := run.ParseStored(); err != nil {
		t.Fatalf("canonical producer values rejected: %v", err)
	}
}

func TestValidationRunParseStoredRejectsUnknownDurableDiscriminators(t *testing.T) {
	newRun := func() *ValidationRun {
		return &ValidationRun{
			ID: "run", DefinitionID: "definition", Kind: RunKindCandidate,
			Classification: RunClassificationPassing,
		}
	}
	tests := []struct {
		name   string
		mutate func(*ValidationRun)
	}{
		{name: "workspace binding", mutate: func(run *ValidationRun) { run.WorkspaceBindingStatus = "impossible" }},
		{name: "execution origin", mutate: func(run *ValidationRun) { run.ExecutionOrigin = "impossible" }},
		{name: "cleanup", mutate: func(run *ValidationRun) { run.Cleanup.Status = "impossible" }},
		{name: "timeout phase", mutate: func(run *ValidationRun) { run.TimeoutPhase = "impossible" }},
		{name: "failure phase", mutate: func(run *ValidationRun) { run.FailurePhase = "impossible" }},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			run := newRun()
			testCase.mutate(run)
			if err := run.ParseStored(); err == nil {
				t.Fatal("unknown durable discriminator was accepted")
			}
		})
	}
}

func TestValidationRunGroupParseStoredRejectsUnknownAttemptAndAggregateDiscriminators(t *testing.T) {
	group := &ValidationRunGroup{
		ID: "group", DefinitionID: "definition", Classification: RunGroupStablePass,
		Attempts: []ValidationAttempt{{
			Kind: RunKindCandidate, Classification: RunClassificationPassing,
			Cleanup: CleanupResult{Status: CleanupClean},
		}},
		Aggregates: []ValidationAggregate{{
			Kind: RunKindCandidate, Classification: RunGroupStablePass,
			ResourceClassification: "impossible",
		}},
	}
	if err := group.ParseStored(); err == nil {
		t.Fatal("unknown aggregate resource classification was accepted")
	}
	group.Aggregates[0].ResourceClassification = ResourceAvailable
	group.Attempts[0].FailurePhase = "impossible"
	if err := group.ParseStored(); err == nil {
		t.Fatal("unknown attempt phase was accepted")
	}
}
