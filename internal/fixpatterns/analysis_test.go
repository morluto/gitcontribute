package fixpatterns

import (
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

func TestClassifyRequiresExplicitClosingRelationshipForAcceptedFix(t *testing.T) {
	repo, err := domain.NewRepoRef("acme", "rocket")
	if err != nil {
		t.Fatal(err)
	}
	merged := domain.MergedStatus(time.Unix(1, 0).UTC())
	classified := Classify(repo, PullRequest{State: domain.ClosedState, Merge: merged, Body: "Fixes #7\n\nIncludes a regression test and benchmark."})
	if classified.Outcome != Merged || classified.Relationship != Closes || classified.RelatedNumber != 7 {
		t.Fatalf("classification = %+v", classified)
	}
	if len(classified.ProofStyles) != 2 || classified.ProofStyles[0] != RegressionTest || classified.ProofStyles[1] != Benchmark {
		t.Fatalf("proof styles = %+v", classified.ProofStyles)
	}
}

func TestClassifyPreservesUnknownMergeOutcome(t *testing.T) {
	repo, _ := domain.NewRepoRef("acme", "rocket")
	classified := Classify(repo, PullRequest{State: domain.ClosedState, Merge: domain.UnknownMergeStatus(), Body: "Related to #9"})
	if classified.Outcome != Unknown || classified.Relationship != References {
		t.Fatalf("classification = %+v", classified)
	}
}
