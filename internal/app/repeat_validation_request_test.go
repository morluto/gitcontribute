package app

import (
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func TestParseMCPRepeatValidationInputProducesCanonicalAuthorizedRequest(t *testing.T) {
	request, canonical, err := parseMCPRepeatValidationInput(mcpcontract.RunValidationInput{
		ID: " definition-1 ", Target: " both ", RunCount: 2, Concurrency: 2,
		PerRunTimeout: " 30s ", OverallTimeout: " 2m ", Execute: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.definitionID != "definition-1" || request.target != validationRunBoth {
		t.Fatalf("parsed request identity/target = %q/%v", request.definitionID, request.target)
	}
	if len(request.options.Kinds) != 2 || request.options.Kinds[0] != evidence.RunKindBase || request.options.Kinds[1] != evidence.RunKindCandidate {
		t.Fatalf("parsed run kinds = %v", request.options.Kinds)
	}
	if request.options.PerRunTimeout != 30*time.Second || request.options.OverallTimeout != 2*time.Minute || request.options.SampleInterval != 100*time.Millisecond {
		t.Fatalf("parsed durations = %+v", request.options)
	}
	if canonical.ID != "definition-1" || canonical.Target != "both" || canonical.PerRunTimeout != "30s" || canonical.OverallTimeout != "2m0s" || canonical.SampleInterval != "100ms" || !canonical.Execute {
		t.Fatalf("canonical durable request = %+v", canonical)
	}
}

func TestParseMCPRepeatValidationInputRejectsInvalidRequestBeforeSubmission(t *testing.T) {
	for name, in := range map[string]mcpcontract.RunValidationInput{
		"unauthorized": {ID: "definition-1", Target: "candidate"},
		"target":       {ID: "definition-1", Target: "production", Execute: true},
		"run count":    {ID: "definition-1", Target: "candidate", RunCount: 101, Concurrency: 1, Execute: true},
		"concurrency":  {ID: "definition-1", Target: "candidate", RunCount: 1, Concurrency: 2, Execute: true},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := parseMCPRepeatValidationInput(in); err == nil {
				t.Fatalf("invalid request was accepted: %+v", in)
			}
		})
	}
}
